package sc

import (
	"math/big"
	"sync"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/core/rawdb"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"
)

var (
	settleUpdateTODOPrefix = []byte("SETTLE_TODO_")
)

// 结算计算服务
type SettleCalc struct {
	db     ntsdb.Database
	dag    DagChain
	locker sync.Mutex

	quit chan struct{}
}

func NewSettleSer(db ntsdb.Database, dag DagChain) *SettleCalc {
	return &SettleCalc{
		db:   db,
		dag:  dag,
		quit: make(chan struct{}),
	}
}

func getSettleTODOKey(utime uint64, uhash common.Hash) []byte {
	return MakeKey(settleUpdateTODOPrefix, utime, uhash)
}

// 在新单元落地后需要实时登录，以便进行结算计算
func (s *SettleCalc) Write(db ntsdb.Putter, unit *types.Unit) error {
	if unit.IsBlackUnit() {
		return nil
	}
	return db.Put(getSettleTODOKey(unit.Timestamp(), unit.Hash()), nil)
}

func (s *SettleCalc) Start() error {
	log.Info("start settle calculate server")
	if err := s.processOld(); err != nil {
		return err
	}
	go s.listen()
	return nil
}
func (s *SettleCalc) Stop() {
	close(s.quit)
	log.Info("stopped settle calculate server")
}

// 一次性处理所有在排队中需要处理的记录
func (s *SettleCalc) processOld() error {
	var printed bool

	return s.db.Iterator(settleUpdateTODOPrefix, func(key, value []byte) bool {
		if !printed {
			log.Info("process unit settle")
			printed = true
		}
		uhash := common.BytesToHash(key[len(key)-common.HashLength:])
		unit := rawdb.GetUnit(s.db, uhash)
		if unit == nil {
			s.db.Delete(key)
			log.Trace("missing unit", "uhash", uhash)
		} else {
			s.process(unit, nil)
		}
		return true
	})
}

func (s *SettleCalc) listen() {
	units := make(chan types.ChainEvent, 200)
	sub := s.dag.SubscribeChainEvent(units)
	defer sub.Unsubscribe()

	tk := time.NewTicker(time.Hour * 24)
	for {
		select {
		case <-s.quit:
			//释放资源，不需要立即处理，下次重启时将被处理
			for len(units) > 0 {
				<-units
			}
			return
		case <-tk.C:
			s.processOld()
		case ev := <-units:
			s.process(ev.Unit, ev.Receipts)
		}
	}
}
func (s *SettleCalc) process(unit *types.Unit, receipts types.Receipts) {
	lib, err := s.dag.GetChainWitnessLib(unit.MC(), unit.SCHash())
	if err != nil {
		log.Error("failed to get chain witness lib", "chain", unit.MC(), "err", err)
	} else if err := s.writeWorkFee(unit, lib, s.calcUnitFee(unit, receipts)); err != nil {
		log.Error("failed to write work fee", "chain", unit.MC(), "err", err)
	} else {
		log.Trace("update unit settle", "chain", unit.MC(), "number", unit.Number(), "uhash", unit.Number())
	}
}

// WriteWorkFee 写入见证费用。
// 使用32把锁控制同周期内的数据更新控制。
func (s *SettleCalc) writeWorkFee(unit *types.Unit, chainWitness []common.Address, workFee *big.Int) (err error) {
	if unit.IsBlackUnit() {
		return nil
	}
	period := GetSettlePeriod(unit.SCNumber())
	list, err := unit.Witnesses()
	if err != nil {
		log.Crit("storage witness work fee failed", "err", err)
	}

	batch := s.db.NewBatch()
	proposer := unit.Proposer()

	for _, w := range chainWitness {
		info := GetWitnessPeriodWorkReport(s.db, period, w)
		if info == nil {
			info = &types.WitnessPeriodWorkInfo{
				Fee:   new(big.Int),
				IsSys: unit.MC() == params.SCAccount,
			}
		}

		info.ShouldVote++
		if w == proposer { //单元生产者
			if info.VotedLastUnit == unit.Hash() {
				// 防止重复计数
				break
			}
			// 累加当前费用和单元个数
			info.Fee.Add(info.Fee, workFee)
			info.CreateUnits++
			info.Votes++
			info.VotedTxs += uint64(unit.TxCount())
			info.VotedLastUnit = unit.Hash()
		} else {
			//如果已投票则更新
			for _, votedw := range list {
				if votedw == w {
					info.Votes++
					info.VotedTxs += uint64(unit.TxCount())
					info.VotedLastUnit = unit.Hash()
					break
				}
			}
		}
		updateWitnessWorkReport(batch, getSettlementPeriodLogKey(period, w), info)
	}
	batch.Delete(getSettleTODOKey(unit.Timestamp(), unit.Hash()))
	return batch.Write()
}

func (s *SettleCalc) calcUnitFee(unit *types.Unit, receipts types.Receipts) *big.Int {
	if receipts == nil {
		receipts = rawdb.GetUnitReceipts(s.db, unit.MC(), unit.Hash(), unit.Number())
		if receipts == nil {
			log.Crit("missing unit receipts", "chain", unit.MC(), "number", unit.Number(), "uhash", unit.Number())
		}
	}
	// 获取打包将军
	total := new(big.Int)
	// 统计单元内所有的费用
	for i, txs := range unit.Transactions() {
		if receipts[i].GasUsed == 0 {
			continue
		}
		if txs.Action == types.ActionSCIdleContractDeal {
			continue
		}
		originTx := txs.Tx
		if originTx == nil {
			originTx = s.dag.GetTransaction(txs.TxHash)
			if originTx == nil {
				log.Crit("failed to find Transaction info", "tx", txs.TxHash)
			}
		}
		if originTx.GasPrice() == 0 {
			continue
		}
		fee := math.Mul(receipts[i].GasUsed, originTx.GasPrice())
		total.Add(total, fee)
	}
	return total
}

func (s *SettleCalc) RollbackWorkFee(db ntsdb.PutGetter, unit *types.Unit) {
	if unit.IsBlackUnit() {
		return
	}
	period := GetSettlePeriod(unit.SCNumber())

	proposer := unit.Proposer()

	//全局锁，必须保证无影响
	s.locker.Lock()
	defer s.locker.Unlock()

	//因为投票信息记录本身是不准确的，因此在回滚时只需要对涉及权益部分进行回退
	// 包括：单元提案者
	//记录单元创建者投票与见证费
	info := GetWitnessPeriodWorkReport(s.db, period, proposer)
	if info == nil {
		return
	}
	// 累加当前费用和单元个数
	info.Fee = info.Fee.Sub(info.Fee, s.calcUnitFee(unit, nil))
	info.CreateUnits = math.ForceSafeSub(info.CreateUnits, 1)
	info.Votes = math.ForceSafeSub(info.Votes, 1)
	info.VotedTxs = math.ForceSafeSub(info.Votes, uint64(unit.TxCount()))
	info.ShouldVote = math.ForceSafeSub(info.ShouldVote, 1)

	updateWitnessWorkReport(db, getSettlementPeriodLogKey(period, proposer), info)
	return
}
