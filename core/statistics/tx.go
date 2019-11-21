package statistics

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
)

// handelNew 处理新的单元，将其中的交易处理到所有用户下
// 主要是记录一笔交易所涉及的多方：
// 	1. 交易发送方(from)
//  2. 交易接收方(to)
//  3. 交易收款方(receipt)
func (ts *Statistic) afterNewUnit(ev types.ChainEvent) error {
	unit := ev.Unit

	if len(unit.Transactions()) == 0 || unit.Number() == 0 {
		return nil
	}

	bt := time.Now()

	records, err := ts.getRecodeFromUnit(unit.Timestamp(), unit.Transactions(), ev.Receipts)
	if err != nil {
		return err
	}

	err = ts.db.Exec(func(conn *sql.Conn) error {
		translation, err := conn.BeginTx(context.TODO(), nil)
		if err != nil {
			return err
		}

		stmt, err := translation.Prepare("INSERT OR IGNORE INTO txs(addr,tx,tx_timestamp,tx_type) values(?,?,?,?)")
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, r := range records {
			_, err := stmt.Exec(r.User, r.TxHash, r.TxTimestamp, r.TxType)
			if isUniqueError(err) {
				continue
			} else if err != nil {
				translation.Rollback()
				return err
			}
		}
		//更新处理位置
		_, err = translation.Exec("REPLACE INTO wkat (chain,height) values(?,?)", unit.MC().Hex(), unit.Number())
		if err != nil {
			translation.Rollback()
			return err
		}
		err = translation.Commit()
		return nil
	})
	if err == nil {
		log.Trace("write about unit", "cost", common.PrettyDuration(time.Now().Sub(bt)))
	} else {
		log.Error("failed to write unit transaction info", "err", err)
	}
	return err
}

func (ts *Statistic) getRecodeFromUnit(unitTimestamp uint64, txs types.TxExecs, receipts types.Receipts) ([]TxRecord, error) {
	var list []TxRecord
	for _, tx := range txs {
		sl, err := ts.newTx(tx.Tx)
		if err != nil {
			return nil, err
		}
		if len(sl) > 0 {
			list = append(list, sl...)
		}
	}

	for _, r := range receipts {
		// 如果存在交易接收方，则需要为此方记录信息
		// 如果存在交易接收方，则需要为此方记录信息
		//该交易实际有结果,此时收付款记录的时间不已交易时间为准儿是记录对应的单元创建时间
		for _, v := range r.Coinflows {
			if !ts.isLocalAcct(v.To) {
				continue
			}
			list = append(list, TxRecord{
				User:        v.To.Hex(),
				TxHash:      r.TxHash.Hex(),
				TxTimestamp: int64(unitTimestamp / uint64(time.Second)),
				TxType:      types.TransactionTypeContractRun,
			})
		}
	}

	return list, nil
}

// AfterNewTx 保留所有交易信息
// 将保留所有全网交易记录，sqlite单表可以支持到上亿条记录，同时还需要
// TODO:定期删除已过期的交易记录，但不能过期后立即删除而需要等待到一定时间后
func (ts *Statistic) AfterNewTx(_ bool, tx *types.Transaction) error {
	//log.Info("add tx", "clen", len(ts.jobc))
	ts.jobc <- tx
	return nil
}

func (ts *Statistic) saveNewTx(tx *types.Transaction) error {
	bt := time.Now()
	records, err := ts.newTx(tx)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
	}
	err = ts.db.Exec(func(conn *sql.Conn) error {
		stmt, err := conn.PrepareContext(context.Background(),
			"INSERT OR IGNORE INTO txs(addr,tx,tx_timestamp,tx_type) values(?,?,?,?)")
		if err != nil {
			return err
		}
		for _, r := range records {
			_, err := stmt.Exec(r.User, r.TxHash, r.TxTimestamp, r.TxType)
			if isUniqueError(err) {
				continue
			} else {
				stmt.Close()
				return err
			}
		}
		stmt.Close()
		return nil
	})
	if err == nil {
		log.Trace("write about tx", "txhash", tx.Hash(), "cost", common.PrettyDuration(time.Now().Sub(bt)))
	}
	return err
}

func (ts *Statistic) newTx(tx *types.Transaction) (list []TxRecord, err error) {
	if tx == nil {
		return nil, nil
	}
	fromOk, toOk := ts.isLocalAcct(tx.SenderAddr()), ts.isLocalAcct(tx.To())
	if !fromOk && !toOk {
		return nil, nil
	}
	from := tx.SenderAddr() //需要确保交易已经在外部获得校验，这里不关注签名正确性。因为交易中已经包含了交易发起方信息，直接使用
	txHex := tx.Hash().Hex()
	typ := sc.GetTxType(tx)
	txTime := tx.Time2().Unix()

	if fromOk {
		list = append(list, TxRecord{
			User:        from.Hex(),
			TxHash:      txHex,
			TxTimestamp: txTime,
			TxType:      typ,
		})
	}

	if toOk && from != tx.To() { //记录对手方
		list = append(list, TxRecord{
			User:        tx.To().Hex(),
			TxHash:      txHex,
			TxTimestamp: txTime,
			TxType:      typ,
		})
	}
	return list, nil
}

func (ts *Statistic) GetTxs(t types.TransactionType, pageIndex, pageSize uint64, address ...common.Address) (sum int, txs []common.Hash, err error) {
	//确认总数
	condition := bytes.NewBufferString("1=1")
	var args []interface{}

	if t < types.TransactionTypeAll {
		condition.WriteString(" and tx_type=?")
		args = append(args, t)
	}

	if len(address) > 0 {
		condition.WriteString(" and ")
		if len(address) == 1 {
			condition.WriteString(" addr=?")
			args = append(args, address[0].Hex())
		} else {
			condition.WriteString(" addr in (")
			for i, a := range address {
				if i == len(address)-1 {
					condition.WriteString("?")
				} else {
					condition.WriteString("?,")
				}
				args = append(args, a.Hex())
			}
			condition.WriteString(")")
		}
	}

	//sum
	//因为主键是：addr+txHash,因此根据单个地址查找时，交易不会重复。
	//但当多个地址一起查询是会存在交易重复，因为一笔交易涉及多个人（from,to,receipt）等等。
	//但这里不去重的原因是，当查询结果中的数据量有上百万时，会因为去重而影响时间，而实际上重复的交易非常少。
	//那么这里的代价是没必要的，仅仅是影响了总数量的判断。
	//而下方取一页 20 条数据时已进行去重处理，所以这里不进行去重。
	// 为何要一个账户记录一笔？这是因为一笔交易会涉及两个以上的交易方。
	// 		没有考虑在一个字段中用逗号分隔多个账户的做法。如：addrs="a,b,c,d,". 这里没法控制总量
	query := fmt.Sprintf("SELECT count(*) FROM txs WHERE %s", condition.String())

	row := ts.db.QueryRow(query, args...)
	if err = row.Scan(&sum); err != nil {
		log.Error("failed to exec sql", "sql", query, "err", err)
		return
	}
	if sum == 0 {
		return
	}
	//page
	rows, err := ts.db.Query(fmt.Sprintf("SELECT DISTINCT tx FROM txs WHERE %s ORDER BY tx_timestamp DESC LIMIT %d OFFSET %d", condition.String(), pageSize, pageIndex*pageSize), args...)
	if err != nil {
		return
	}
	txs = make([]common.Hash, 0, pageSize)
	for rows.Next() {
		var tx string
		if err = rows.Scan(&tx); err != nil {
			return
		}
		txs = append(txs, common.BytesToHash(common.FromHex(tx)))
	}
	return
}

func (ts *Statistic) GetLastTxHashByType(address common.Address, t types.TransactionType) (common.Hash, error) {
	row := ts.db.QueryRow("SELECT tx FROM txs WHERE addr=? and tx_type=? ORDER BY tx_timestamp desc limit 1",
		address.Hex(), t)
	var tx string
	if err := row.Scan(&tx); err != nil {
		if err == sql.ErrNoRows {
			return common.Hash{}, nil
		}
		return common.Hash{}, err
	}
	return common.HexToHash(tx), nil
}

func (ts *Statistic) QueryTxs(user common.Address, afterTime time.Time) ([]common.Hash, error) {
	//page
	rows, err := ts.db.Query("SELECT tx FROM txs WHERE addr=? and tx_timestamp>=?  ORDER BY tx_timestamp desc",
		user.Hex(), afterTime.Unix())
	if err != nil {
		return nil, err
	}
	var list []common.Hash
	for rows.Next() {
		var tx string
		if err = rows.Scan(&tx); err != nil {
			return nil, err
		}
		list = append(list, common.BytesToHash(common.FromHex(tx)))
	}
	return list, nil
}

// AutoFix 自动修复维护新数据
// 依次检查本地所有单元数据中的交易是否有实时维护在数据库中，
// 从最后依次记录的位置，分别检查所有链数据 分链遍历所有单元。
// 为了不影响现有数据，先将数据存入临时表，再转入当前表中。
func (ts *Statistic) autoFix() error {
	logger := log.New("module", "sta", "task", "autofix")
	logger.Info("checking local statistics data")
	defer log.Info("check done")

	ts.working.Add(1)
	defer ts.working.Done()

	if ts.reader == nil {
		logger.Warn("reader is nil")
		return nil
	}

	//检查所有链
	ctx := context.Background()

	//加载所有已记录数据
	rows, err := ts.db.Query("SELECT chain,height FROM wkat")
	if err != nil {
		return err
	}
	//to map
	workatMap, err := toWorkAtMap(rows)
	if err != nil {
		return err
	}
	var (
		execCount    int
		insertStmt   *sql.Stmt
		wkupdateStmt *sql.Stmt
	)
	defer func() {
		if insertStmt != nil {
			insertStmt.Close()
		}
		if wkupdateStmt != nil {
			wkupdateStmt.Close()
		}
	}()
	return ts.db.Exec(func(conn *sql.Conn) error {

		curTx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}

		initStmt := func() error {
			insertStmt, err = curTx.Prepare("INSERT INTO txs_tmp(addr,tx,tx_timestamp,tx_type) values(?,?,?,?)")
			if err != nil {
				return err
			}
			wkupdateStmt, err = curTx.Prepare("REPLACE INTO wkat_tmp (chain,height) values(?,?)")
			if err != nil {
				return err
			}
			return nil
		}
		if err := initStmt(); err != nil {
			curTx.Rollback()

			return err
		}
		tryCommit := func() error {
			if curTx == nil {
				return nil
			}
			if execCount < 1000 {
				return nil
			}
			execCount = 0
			err := curTx.Commit()
			if err != nil {
				return err
			}
			curTx, err = conn.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			return initStmt()
		}

		var foundErr error

		checkError := func(err error) error {
			if err == nil {
				return nil
			}
			if isUniqueError(err) {
				return nil
			}
			logger.Error("failed check", "err", err)
			foundErr = err
			return err
		}

		//TODO: 当链数量非常大时，存在性能问题
		ts.reader.WakChildren(ts.reader.GenesisHash(), func(children common.Hash) bool {
			//不同链单元
			//判断是否存在，如果不存则表示为0
			//高度加一开始工作
			header := ts.reader.GetHeaderByHash(children)
			if header == nil {
				return false
			}
			height := workatMap[header.MC]
			logger.Trace("check one chain", "chain", header.MC, "current", height)
			for {
				//从此高度开始依次处理
				brotherHash, err := ts.reader.GetBrothers(header.MC, height+1)
				if err != nil {
					break
				}
				height++
				h := ts.reader.GetHeader(brotherHash)
				if h == nil {
					continue
				}

				body := ts.reader.GetBody(h.MC, h.Number, brotherHash)
				if body == nil || len(body.Txs) == 0 {
					continue
				}
				//将交易依次处理
				records, err := ts.getRecodeFromUnit(h.Timestamp, body.Txs, nil)
				if checkError(err) != nil {
					return true
				}
				for _, r := range records {
					_, err := insertStmt.Exec(r.User, r.TxHash, r.TxTimestamp, r.TxType)
					if checkError(err) != nil {
						return true
					}
					execCount++
					if checkError(tryCommit()) != nil {
						return true
					}
				}

			}
			//更新高度
			if height > workatMap[header.MC] {
				_, err = wkupdateStmt.Exec(header.MC.Hex(), height)
				if checkError(err) != nil {
					return true
				}
				err = tryCommit()
				if checkError(err) != nil {
					return true
				}
				logger.Debug("check one chain done", "chain", header.MC, "old", workatMap[header.MC], "new", height)
			}
			return false
		})
		if foundErr != nil {
			curTx.Rollback()
			return foundErr
		}
		err = curTx.Commit()
		if err != nil {
			return err
		}
		//批量从临时表中加入当前表
		_, err = conn.ExecContext(ctx, `
REPLACE INTO wkat SELECT chain,height FROM wkat_tmp;
REPLACE INTO txs SELECT addr,tx,tx_timestamp,tx_type FROM txs_tmp;
DELETE FROM txs_tmp;
DELETE FROM wkat_tmp;  
`)
		return err
	})
}

func isUniqueError(err error) bool {
	if err == nil {
		return false
	}
	return strings.HasPrefix(err.Error(), "UNIQUE constraint failed") // sqlite UNIQUE constraint failed error
}
