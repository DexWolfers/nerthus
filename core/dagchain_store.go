package core

import (
	"encoding/binary"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/mclock"
	"gitee.com/nerthus/nerthus/common/tracetime"
	"gitee.com/nerthus/nerthus/core/rawdb"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"
)

type needStoreUnit struct {
	unit     *types.Unit
	receipts types.Receipts
	sdb      *state.StateDB
	logs     []*types.Log
}
type dbHelp struct {
	put ntsdb.Putter
	get ntsdb.Getter
}

func (db dbHelp) Put(key []byte, value []byte) error {
	return db.put.Put(key, value)
}
func (db dbHelp) Get(key []byte) ([]byte, error) {
	return db.get.Get(key)
}

func (db dbHelp) Write() error {
	if w, ok := db.put.(ntsdb.Batch); ok {
		return w.Write()
	}
	return nil
}

func (db dbHelp) Discard() {
	if w, ok := db.put.(ntsdb.Batch); ok {
		w.Discard()
	}
}

func createWriter(db ntsdb.Database, useBatch bool) dbHelp {
	w := dbHelp{get: db}
	if useBatch {
		w.put = db.NewBatch()
	} else {
		w.put = db
	}
	return w
}

// writeRightUnit 单元落地
func (dc *DagChain) writeRightUnit(state *state.StateDB, unit *Unit, receipts types.Receipts, logs []*types.Log) error {
	bstart := mclock.Now()

	tr := tracetime.New("uhash", unit.Hash(),
		"utime", unit.Timestamp(), "number", unit.Number(), "chain", unit.MC(), "createat", unit.ReceivedAt,
		"proposer", unit.Proposer(), "txs", len(unit.Transactions())).SetMin(0)

	if len(unit.WitnessVotes()) > 0 {
		tr.AddArgs("round", binary.LittleEndian.Uint64(unit.WitnessVotes()[0].Extra[1:]))
	}
	logger := log.New("uhash", unit.Hash(), "utime", unit.Timestamp(), "number", unit.Number(), "chain", unit.MC())

	defer tr.Stop()

	dc.wg.Add(1)
	defer dc.wg.Done()

	// 特殊单元处理
	if unit.Proposer() == params.PublicPrivateKeyAddr {
		if uhash := dc.GetStableHash(unit.MC(), unit.Number()); !uhash.Empty() {
			dc.ClearUnit(uhash, true) // 如果已存在，清理旧单元
		}
	}
	tr.Tag()
	uhash, header := unit.Hash(), unit.Header()
	tr.Tag()
	//只锁住写入
	locker := dc.chainLocker(unit.MC())
	tr.Tag()
	var unlocked = true
	unlock := func() {
		//只解锁一次
		if unlocked {
			locker.Unlock()
			unlocked = false
		}
	}
	locker.Lock()
	tr.Tag("getLock")
	defer unlock()
	{
		//检查是否已存在
		if _, number := GetUnitNumber(dc.chainDB, uhash); number != MissingNumber {
			return ErrExistStableUnit
		}
		tr.Tag()
		// 批量提交，保证数据提交的原子性，
		db := createWriter(dc.chainDB, true)
		defer db.Discard()

		tr.Tag()

		// Store the header too, signaling full unit ownership
		rawdb.WriteHeader(db, header)
		tr.Tag()
		//不允许分支存在，将直接将当期区块作为合法稳定单元存储
		WriteStableHash(db, uhash, unit.MC(), unit.Number())
		tr.Tag()
		if err := WriteChainTailHash(db, uhash, unit.MC()); err != nil { //最后稳定位置
			return err
		}
		tr.Tag()

		if dc.enableAsyncWriteBody {
			writePendingUnit(db, uhash, unit.MC())
			dc.asyncWriteBody(state, unit, receipts, logs)
		} else {
			dc.writeUnitBody(dc.chainDB, db, state, unit, receipts, logs)
		}
		tr.Tag()

		if err := db.Write(); err != nil {
			log.Crit("failed to store unit",
				"chain", unit.MC(),
				"number", unit.Number(),
				"uhash", unit.Hash(),
				"err", err)
		}
		tr.Tag("batchW")
	}
	//及时解锁
	unlock()

	tr.AddArgs("status", "ok")
	dc.unitHeadCache.Add(uhash, header)
	tr.Tag()
	dc.cacheUnit(unit)
	tr.Tag()

	//成功后缓存,不需要重复缓存创世信息
	if header.Number > 0 {
		dc.updateChainTailInfo(unit.MC(), state, unit)
	}
	dc.processSystemLogs(unit, logs)
	// 如果不缓存state，需要清理
	if dc.enableAsyncWriteBody {
		dc.goodStateCache.Add(unit.Root().Hex(), state)

		//异步发送事件，异步发送的风险是乱序
		go dc.PostChainEvent(types.ChainEvent{Unit: unit, UnitHash: unit.Hash(), Logs: logs, Send: time.Now(), Receipts: receipts})
		go dc.PostChainEvent(types.RealtimeSyncEvent{Unit: unit, SyncType: types.RealtimeSyncBeta})
	} else {
		if header.MC != params.SCAccount {
			state.Clear()
		}

		//无需异步发送
		dc.PostChainEvent(
			types.ChainEvent{Unit: unit, UnitHash: unit.Hash(), Logs: logs, Send: time.Now(), Receipts: receipts},
			types.RealtimeSyncEvent{Unit: unit, SyncType: types.RealtimeSyncBeta},
		)
	}

	sender, _ := unit.Sender() //err 单元已合法

	logger.Debug("Inserted new unit", "mc", unit.MC(), "number", unit.Number(),
		"uhash", uhash, "utime", unit.Timestamp(),
		//"utime.format", time.Unix(0, int64(unit.Timestamp())).Format("15:04:05"),
		"txs", len(unit.Transactions()), "creator", sender,
		"write.cost", common.PrettyDuration(mclock.Now()-bstart),
		"gas", unit.GasUsed())

	tr.Tag()

	return nil
}

// asyncWriteBody 异步写入body
func (dc *DagChain) asyncWriteBody(sdb *state.StateDB, unit *Unit, receipts types.Receipts, logs []*types.Log) {
	//检查
	dc.checkWritenUnits()

	key := unit.Hash().Hex()

	// 内存暂存数据
	dc.pendingUnits.Set(key, &needStoreUnit{
		unit:     unit,
		sdb:      sdb,
		receipts: receipts,
	})
	// 发送信号
	dc.goodUnitq.Enqueue(key)
}

func (dc *DagChain) getPendingUnit(uhash common.Hash) *Unit {
	data, ok := dc.pendingUnits.Get(uhash.Hex())
	if !ok {
		return nil
	}
	return data.(*needStoreUnit).unit

}
func (dc *DagChain) pendingStored(uhash common.Hash) {
	dc.pendingUnits.Remove(uhash.Hex())
	deletePendingUnit(dc.chainDB, uhash)
}

func (dc *DagChain) goodUnitLookup() {
	// 不能并发写，必须保证按照校验顺序依次落地单元，否则乱序将引起关系数据写入不符合预期。
	for {
		select {
		case <-dc.quit:
			return
		case uhash := <-dc.goodUnitq.DequeueChan():
			data, ok := dc.pendingUnits.Get(uhash.(string))
			if ok {
				if dc.testPopUnit == nil {
					dc.writeUnitData(data.(*needStoreUnit))
				} else {
					dc.testPopUnit(data)
				}
			}
		}
	}
}

func (dc *DagChain) writeUnitData(info *needStoreUnit) {
	db := createWriter(dc.chainDB, false)
	dc.writeUnitBody(dc.chainDB, db, info.sdb, info.unit, info.receipts, info.logs)
	db.Write()
}

// writeUnitBody 写body
func (dc *DagChain) writeUnitBody(db ntsdb.Database, batch dbHelp, state *state.StateDB, unit *Unit, receipts types.Receipts, logs []*types.Log) {
	root, err := state.CommitToChain(dc.chainDB, unit.MC())
	if err != nil {
		log.Crit("failed to commit state", "chain", unit.MC(), "number", unit.Number(), "uhash", unit.Hash(), "err", err)
	}

	if root != unit.Root() {
		log.Crit("failed to commit state", "chain", unit.MC(), "number", unit.Number(), "uhash", unit.Hash(), "root", unit.Root(), "saved", root)
	}

	// 存储引用关系
	if err := WriteChildrens(batch, unit); err != nil {
		panic(err)
	}

	if err := dc.settleCalc.Write(batch, unit); err != nil {
		log.Crit("failed to write settle", "chain", unit.MC(), "number", unit.Number(), "uhash", unit.Hash(), "err", err)
	}

	db2 := createWriter(db, true)

	// Store the body first to retain database consistency
	if err := WriteBody(db2, unit.MC(), unit.Number(), unit.Hash(), unit.Body(), receipts); err != nil {
		panic(err)
	}
	if err := db2.Write(); err != nil {
		panic(err)
	}
	db2.Discard()

	handleSpecialLogs(db, unit, logs)

	if receipts != nil {
		WriteUnitReceipts(db, unit.Hash(), unit.MC(), unit.Number(), receipts)
	}
	// 写入bloom过滤信息
	if err := WriteMipmapBloom(db, unit.MC(), unit.Number(), receipts); err != nil {
		panic(err)
	}

	// 写入镜像 debug
	if err := WritePreimages(db, unit.MC(), unit.Number(), state.Preimages()); err != nil {
		panic(err)
	}

	//清理已处理内容
	dc.pendingStored(unit.Hash())
}

func (dc *DagChain) checkData() {
	db, ok := dc.chainDB.(ntsdb.Finder)
	if !ok {
		return
	}

	dc.wg.Add(1)
	defer dc.wg.Done()

	chains := make(map[common.Address]struct{})

	//获取所有
	db.Iterator(pendingUnit, func(key, value []byte) bool {
		if len(value) != len(common.StringToAddress("a").Bytes()) {
			log.Crit("bad data", "key", string(key), "keyBytes", key)
		}
		chains[common.BytesToAddress(value)] = struct{}{}
		return true
	})
	if len(chains) == 0 {
		return
	}

	log.Warn("Found incomplete unit data,recovery...", "chains", len(chains))
	for c := range chains {
		log.Info("check chain data", "chain", c)
		//针对每一条链进行递减检查，删除无单元部分
		var tail *types.Header
		for {
			tail = dc.GetChainTailHead(c)
			if tail == nil {
				log.Crit("chain tail is empty", "chain", c)
			}
			if tail.Number == 0 {
				break
			}
			tailHash := tail.Hash()
			//存在数据
			if ExistBody(dc.chainDB, tailHash) {
				break
			}
			//否则删除
			dc.clearUnit(tailHash, true)
			deletePendingUnit(dc.chainDB, tailHash)
		}
		log.Warn("one chain recovery done", "chain", c, "tail.number", tail.Number)
	}
}

var pendingUnit = []byte("pending_unit_") //标记已完成单元头落地，但是单元Body缺尚未成功落地的标记

func writePendingUnit(db ntsdb.Putter, uhash common.Hash, chain common.Address) error {
	return db.Put(makeKey(pendingUnit, uhash), chain.Bytes())
}
func deletePendingUnit(db ntsdb.Deleter, uhash common.Hash) error {
	return db.Delete(makeKey(pendingUnit, uhash))
}

func (dc *DagChain) writenNotOverloaded() bool {
	return dc.writtenInUnitLimit == 0 || dc.goodUnitq.Len() < dc.writtenInUnitLimit
}

func (dc *DagChain) checkWritenUnits() {
	if dc.writenNotOverloaded() {
		return
	}
	log.Warn("written in units is too more,wait a moment", "current", dc.goodUnitq.Len(), "limit", dc.writtenInUnitLimit)
	// 每两秒检查一次，指定低于限制
	ticker := time.NewTicker(time.Second * 1)
	defer ticker.Stop()
	for !dc.writenNotOverloaded() {
		select {
		case <-dc.quit:
			return
		case <-ticker.C:
		}
	}
}
