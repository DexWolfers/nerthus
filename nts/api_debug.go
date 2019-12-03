package nts

import (
	"bytes"
	"context"
	"fmt"
	"gitee.com/nerthus/nerthus/trie"
	"math/big"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/hexutil"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/clean"
	"gitee.com/nerthus/nerthus/core/rawdb"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/nts/synchronize/bysolt"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/rlp"

	"github.com/pkg/errors"
)

// DebugNerthusAPI 用于开发调式程序时所开启的API
type DebugNerthusAPI struct {
	n *Nerthus
}

func NewDebugNerthusAPI(e *Nerthus) *DebugNerthusAPI {
	return &DebugNerthusAPI{e}
}

// 根据单元哈希获取单元RLP编码信息
func (api *DebugNerthusAPI) GetEncodedUnit(hash common.Hash) (hexutil.Bytes, error) {
	u := api.n.dagchain.GetUnitByHash(hash)
	if u == nil {
		return nil, errors.New("not found unit by hash")
	}
	b, err := rlp.EncodeToBytes(u)
	if err != nil {
		return nil, err
	}
	return hexutil.Bytes(b), nil
}

// 向网络广播单元
func (api *DebugNerthusAPI) BroadcastRawUnit(encodedUnit hexutil.Bytes) error {
	var unit types.Unit
	if err := rlp.DecodeBytes(encodedUnit, &unit); err != nil {
		return err
	}
	api.n.ProtocolHandler.BroadcastUnit(&unit)
	return nil
}

// 往本地插入一个单元
func (api *DebugNerthusAPI) InsertRawUnit(encodedUnit hexutil.Bytes) error {
	var unit types.Unit
	if err := rlp.DecodeBytes(encodedUnit, &unit); err != nil {
		return err
	}
	return api.n.dagchain.InsertUnit(&unit)
}

// 查看level db内存使用情况
func (api *DebugNerthusAPI) GetLevelDBReport(pro string) (string, error) {
	db, ok := api.n.ChainDb().(*ntsdb.LDBDatabase)
	if !ok {
		return "", errors.New("db convert err")
	}
	return db.LDB().GetProperty(pro)
}

type RangeUnit struct {
	Last  string
	Items []string
}

func (api *DebugNerthusAPI) GetUnitTimeline(start uint64, count int) *RangeUnit {
	list := rawdb.UnitsByTimeLine(api.n.ChainDb(), start, count)
	toInfo := func(h *types.Header, hash common.Hash) string {
		if h == nil {
			return fmt.Sprintf("missing %s", hash.String())
		}
		return fmt.Sprintf("%s %s %d : %d,%s",
			hash.String(),
			h.MC, h.Number,
			h.Timestamp,
			time.Unix(0, int64(h.Timestamp)).Format("2006-01-02 15:04:05.999999999"),
		)
	}

	h, _ := rawdb.LastUnitHash(api.n.ChainDb())
	info := RangeUnit{
		Last:  toInfo(api.n.dagchain.GetHeader(h), h),
		Items: make([]string, len(list)),
	}
	for i, v := range list {
		info.Items[i] = toInfo(api.n.dagchain.GetHeader(v), v)
	}

	return &info
}

func (api *DebugNerthusAPI) GetUnitsReport() (interface{}, error) {
	var result struct {
		Units int64
		Sizes int64
	}
	db := api.n.chainDb
	rawdb.RangUnitByTimline(db, 0, func(hash common.Hash) bool {
		//api.n.DagChain().GetUnit(hash)
		chain, number := api.n.DagChain().GetUnitNumber(hash)
		b := rawdb.GetRawUnit(db, chain, hash, number)
		result.Units += 1
		result.Sizes += int64(len(b))
		return true
	})
	return &result, nil
}

func (api *DebugNerthusAPI) GetLevelDBMemory() (interface{}, error) {
	db, ok := api.n.ChainDb().(*ntsdb.LDBDatabase)
	if !ok {
		return "", errors.New("db convert err")
	}
	rs := make(map[string][]int64)
	rs["h"] = []int64{0, 0}
	rs["H"] = []int64{0, 0}
	rs["b"] = []int64{0, 0}
	rs["ud_"] = []int64{0, 0}
	rs["vote_"] = []int64{0, 0}
	rs["utx_"] = []int64{0, 0}
	rs["others"] = []int64{0, 0}
	var result struct {
		Keys      int64
		Sizes     int64
		KeyValues map[string][]int64
	}

	db.Iterator(nil, func(key, value []byte) bool {
		other := true
		for k := range rs {
			if bytes.HasPrefix(key, []byte(k)) {
				rs[k][0]++
				rs[k][1] += int64(len(value) + len(key))
				other = false
				break
			}
		}
		if other {
			rs["others"][0]++
			rs["others"][1] += int64(len(value) + len(key))
		}
		result.Keys++
		result.Sizes += int64(len(value) + len(key))
		return true
	})
	result.KeyValues = rs
	return result, nil
}

// 压力测试数据报告
func (api *DebugNerthusAPI) TxReport() (map[string]interface{}, error) {

	db := api.n.chainDb

	//链数量
	var chains int
	rawdb.RangeChains(db, func(chain common.Address, status rawdb.ChainStatus) bool {
		chains++
		return true
	})

	//交易总笔数
	var (
		txs int
		gp  = make(map[common.Address]int)
		err error
	)
	rawdb.RangeTxs(db, func(tx []byte) bool {
		txs++

		info, err2 := rawdb.DecodeTx(tx)
		if err2 != nil {
			err = err2
			return false
		}
		if info == nil {
			err = errors.New("bad info")
			return false
		}
		gp[info.SenderAddr()] = gp[info.SenderAddr()] + 1
		return true
	})
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"chains":        chains,
		"pending":       api.n.txPool.PendingCount(),
		"txs":           txs,
		"senders":       len(gp),
		"groupBySender": gp,
	}, nil
}

// GetChildrenUnits 获取指定单元的所有子单元
func (api *DebugNerthusAPI) GetChildrenUnits(uhash common.Hash) []common.Hash {
	var list []common.Hash
	api.n.dagchain.WakChildren(uhash, func(children common.Hash) bool {
		list = append(list, children)
		return false
	})
	return list
}
func (api *DebugNerthusAPI) GetActiveWitnessList() ([]common.Address, error) {

	addr := make([]common.Address, 0)

	stateDB, err := api.n.DagChain().GetChainTailState(params.SCAccount)

	if err != nil {
		return addr, err
	}
	addrInfo, err := sc.GetActivWitnessLib(stateDB)
	if err != nil {
		return addr, err
	}
	for _, v := range addrInfo {
		addr = append(addr, v.Address)
	}
	return addr, nil
}

// SendRawTransactions 发送交易
// encodedTx:接收签完名的交易
func (s *DebugNerthusAPI) SendRawTransactions(ctx context.Context, encodedTx hexutil.Bytes) error {
	var txs []*types.Transaction
	if err := rlp.DecodeBytes(encodedTx, &txs); err != nil {
		return err
	}

	var txHashes = make([]common.Hash, len(txs))
	var errs = make([]error, len(txs))
	for _, tx := range txs {
		txHashes = append(txHashes, tx.Hash())
		errs = append(errs, nil)
	}
	go func() {
		if err := s.n.txPool.AddRemotes(txs); err != nil {
			log.Error("Send remote transaction happen error", "err", err)
		}
	}()
	return nil
}

func (s *DebugNerthusAPI) ClearUnit(hash common.Hash) error {
	return s.n.dagchain.ClearUnit(hash, true)
}

// 在指定链的尾部开始创建多个单元，其中单元数量有encodedTxs决定。一个单元一笔交易
func (s *DebugNerthusAPI) CreateNewUnits(ctx context.Context,
	chain common.Address, encodedTxs hexutil.Bytes) (hexutil.Bytes, error) {
	var txs []*types.Transaction
	if err := rlp.DecodeBytes(encodedTxs, &txs); err != nil {
		return nil, err
	}

	tail := s.n.DagChain().GetChainTailHead(chain)
	tail = types.CopyHeader(tail)

	state, err := s.n.DagChain().GetUnitState(tail.Hash())
	if err != nil {
		return nil, err
	}
	gasLimit := s.n.DagChain().Config().Get(params.ConfigParamsUcGasLimit)
	var units []*types.Unit
	for _, tx := range txs {
		if err = s.n.txPool.ValidateTx(tx.SenderAddr(),
			false, tx, math.MaxBig256, true); err != nil {
			return nil, err
		}
		if tx.SenderAddr() != chain {
			return nil, errors.New("the tx sender should be equal chain")
		}

		h := types.CopyHeader(tail)
		h.Number += 1
		h.Timestamp += uint64(time.Duration(params.OneNumberHaveSecondByUc) * time.Second)
		h.ParentHash = tail.Hash()

		pool := core.GasPool(*big.NewInt(0).SetUint64(gasLimit))

		msg := types.NewMessage(tx, tx.Gas(), tx.SenderAddr(), tx.To(), false)

		txExec := &types.TransactionExec{
			Action: types.ActionTransferPayment,
			Tx:     tx,
			TxHash: tx.Hash(),
		}
		r, err := core.ProcessTransaction(s.n.DagChain().Config(),
			s.n.DagChain(), state, h, &pool, txExec, big.NewInt(0), vm.Config{}, msg)
		if err != nil {
			return nil, err
		}
		h.StateRoot, _ = state.IntermediateRoot(h.MC)
		h.GasUsed = r.GasUsed
		h.GasLimit = gasLimit

		receipts := types.Receipts{r}
		// 插入部分结果数据
		h.ReceiptRoot = types.DeriveSha(receipts)
		h.Bloom = types.CreateBloom(receipts)

		units = append(units, types.NewUnit(h, types.TxExecs{txExec}))

		tail = h //next
	}

	b, err := rlp.EncodeToBytes(units)
	if err != nil {
		return nil, err
	}
	return hexutil.Bytes(b), nil
}

// 获取周期内的单元
func (s *DebugNerthusAPI) GetUnitsInPeriod(period uint64) interface{} {

	startNumber, endNumber := sc.GetPeriodRange(period)

	_, header := s.n.DagChain().GetHeaderAndHash(params.SCAccount, startNumber)

	db := s.n.ChainDb()

	type Info struct {
		UID  types.UnitID
		Time uint64
	}

	var list []Info
	rawdb.RangUnitByTimline(db, header.Timestamp, func(hash common.Hash) bool {
		h := s.n.DagChain().GetHeader(hash)
		if h.SCNumber > endNumber {
			return false
		}
		if h.SCNumber < startNumber {
			return true
		}
		list = append(list, Info{
			UID:  h.ID(),
			Time: h.Timestamp,
		})
		return true
	})
	return list
}
func (s *DebugNerthusAPI) ClearMemory(seq uint64) error {
	switch seq {
	case 1:
		s.n.ProtocolHandler.realTimeSync.RealtimeEngine().Stop()
	}
	return nil
}

// 清理state db 磁盘空间
func (s *DebugNerthusAPI) ClearStateDB(chain common.Address, remain uint64) (int64, error) {
	tail := s.n.dagchain.GetChainTailHead(chain)
	if tail.Number <= remain {
		return 0, errors.New("tail number <= remain number")
	}
	start := tail.Number - remain
	deadline := s.n.dagchain.GetHeaderByNumber(chain, start+1)

	stateDB, err := s.n.dagchain.GetStateByNumber(chain, deadline.Number)
	if err != nil {
		log.Error("get stateDB err when process clean")
		return 0, err
	}
	now := time.Now()
	clner := state.NewCleaner(s.n.chainDb, chain, 1, deadline.Number, stateDB,
		func(chain common.Address, number uint64) common.Hash {
			return s.n.dagchain.GetHeaderByNumber(chain, number).StateRoot
		})
	totalRemoved := clner.StartClear()

	k := rawdb.CleanerKey(chain)
	s.n.chainDb.Put(k, big.NewInt(int64(deadline.Number)).Bytes())
	log.Info("clear state space", "chain", chain, "from", 1, "to", deadline.Number, "removed", totalRemoved, "cost", time.Now().Sub(now))
	return int64(totalRemoved), nil
}

func (s *DebugNerthusAPI) GetStateDBKey(chain common.Address, number uint64, key string) (common.Hash, error) {
	var value common.Hash
	statedb, err := s.n.dagchain.GetStateByNumber(chain, number)
	if err != nil {
		return value, err
	}
	tr := statedb.GetTrie(params.SCAccount)
	if tr == nil {
		return value, errors.New("trie is nil")
	}
	k := common.Hex2Bytes(key)
	orgTr := tr.Trie()
	if orgTr == nil {
		return value, errors.New("origin trie is nil")
	}
	v, err := new(trie.TrieCleaner).GetByOriginKey(k, orgTr)
	if err != nil {
		return value, err
	}
	value = common.BytesToHash(v)
	fmt.Printf("%x\n", v)
	return value, err
}
func (s *DebugNerthusAPI) RecoverStateDB(chain common.Address, from, deadline uint64) error {
	return clean.Recover(s.n.dagchain, s.n.chainDb, chain, from, deadline)
}
func (s *DebugNerthusAPI) FindStateNodeKey(chain common.Address, from, to uint64, node common.Hash) error {
	for i := from; i < to; i++ {
		db, err := s.n.dagchain.GetStateByNumber(chain, i)
		if err != nil {
			log.Error("get state db err", "number", i, "err", err)
			return err
		}
		yes := false
		state.RangeTrieKey(s.n.chainDb, chain, db.Root().Bytes(), func(key []byte) {
			fmt.Println(common.BytesToHash(key).Hex())
			if common.BytesToHash(key) == node {
				//fmt.Println(">>>>>>>>>>>>>>", "number", i, "root", db.Root().String())
				yes = true
			}
		})
		fmt.Println("------------------------------------", "number", i, "have", yes)
	}
	return nil
}
func (s *DebugNerthusAPI) GetStateDB(chain common.Address) interface{} {

	tail := s.n.dagchain.GetChainTailNumber(chain)

	result := make([]error, tail+1)
	for i := uint64(0); i <= tail; i++ {

		fmt.Println("-----------------------------number >", i)
		db, err := s.n.dagchain.GetStateByNumber(chain, i)
		fmt.Println(db)
		result[i] = err
	}
	return result
}

func (s *DebugNerthusAPI) GetTimePoint(number uint64) interface{} {
	first := s.n.dagchain.GetHeaderByNumber(params.SCAccount, 1)
	if first == nil {
		return nil
	}
	ns := bysolt.NewStorage(s.n.chainDb, first.Timestamp, rawdb.RangRangeUnits)
	var status bysolt.NodeStatus
	if number == 0 {
		status = ns.LastPointStatus()
	} else {
		status = ns.GetPointStatus(number)
	}
	timepoint := ns.PointTimeRange(status.Number)
	units := ns.GetPeriodHashes(status.Number, uint64(timepoint.Begin.UnixNano()), uint64(timepoint.End.UnixNano()))
	return struct {
		Begin  time.Time
		End    time.Time
		Status bysolt.NodeStatus
		Units  common.HashList
	}{
		timepoint.Begin,
		timepoint.End,
		status,
		common.HashList(units),
	}
}

func (s *DebugNerthusAPI) GetWitnessPeriodWorkReports(period uint64) map[common.Address]types.WitnessPeriodWorkInfo {
	result := sc.GetWitnessPeriodWorkReports(s.n.ChainDb(), period)
	count := uint64(0)
	for k, v := range result {
		count += v.CreateUnits
		v.Votes = 0
		v.VotedTxs = 0
		v.ShouldVote = 0
		v.VotedLastUnit = common.Hash{}
		result[k] = v
	}
	return result
}

func (s DebugNerthusAPI) GetUnitStateKeys(hash common.Hash) []common.Hash {
	result := clean.FindAllKeys(s.n.dagchain, s.n.chainDb, hash)
	repeated := make([]common.Hash, 0)
	for k, _ := range result {
		repeated = append(repeated, k)
	}
	return repeated
}
