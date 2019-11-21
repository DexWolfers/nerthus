package arbitration

import (
	"errors"
	"sync"

	"gitee.com/nerthus/nerthus/log"

	"gitee.com/nerthus/nerthus/consensus/arbitration/validtor"

	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/nts/protocol"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"
)

type ChainReader interface {
	Config() *params.ChainConfig
	GetChainTailHead(chain common.Address) *types.Header
	GetHeaderByNumber(mcaddr common.Address, number uint64) *types.Header
	StateAt(chain common.Address, root common.Hash) (*state.StateDB, error)
	GetChainTailState(mcaddr common.Address) (*state.StateDB, error)
	PostChainEvent(events ...interface{})
	GetUnitNumber(hash common.Hash) (common.Address, uint64)
	WakChildren(parentHash common.Hash, cb func(children common.Hash) bool)
	GetUnitState(unitHash common.Hash) (*state.StateDB, error)
}

// 测试使用
var mockCheckProofWithState func(signer types.Signer, rep []types.AffectedUnit) ([]common.Address, error)
var (
	handlLock       = sync.Mutex{}
	badUnitEventBus = event.Feed{} //这是内部使用的一个快速感知作恶事件的订阅推送，一般给系统见证人启动见证服务后使用。见 Worker。
)

//当发现作恶时激活如下操作：
//
//1. 根据本节点视角，选择出一个合法单元
//2. 将非法单元数据全部删除
//3. 保留作恶证据
//    + badChain_%chain% = {%number%,%proof_id%}
//    + badWitnessProof_%id%=%info%
//4. 广播作恶证据到网络
//5. 停止和此链相关联的见证服务，并记录有哪些链受影响
//   + 遍历查找所有关联链的起始单元（unit.chain, unit.number）
//   + 标记受影响链：stopedChain_%chain% = %number%
//
//6. 系统见证人选择和合法单元进行仲裁
func OnFoundBad(cr ChainReader, db ntsdb.Database, emux *event.TypeMux, rep []types.AffectedUnit, checkWitness func(db *state.StateDB, address common.Address) error) error {
	//检查数据合法性
	if len(rep) <= 1 {
		return errors.New("the report info must more")
	}
	handlLock.Lock()
	defer handlLock.Unlock()

	SortProof(rep)

	id := common.SafeHash256(rep)
	logger := log.New("module", "onFoundBad", "chain", rep[0].Header.MC, "number", rep[0].Header.Number, "pid", id)

	if ExistProof(db, id) {
		logger.Trace("repeat")
		return nil //如果已存在，则不继续。且不返回错误信息
	}

	signer := types.MakeSigner(*cr.Config())
	if mockCheckProofWithState != nil { //测试使用
		_, err := mockCheckProofWithState(signer, rep)
		if err != nil {
			return err
		}
	} else {
		_, err := validtor.CheckProofWithState(signer, rep, func(uhash common.Hash) (*state.StateDB, error) {
			return cr.GetUnitState(uhash)
		}, checkWitness)

		logger.Trace("checkProofWithState", "err", err)

		if err != nil {
			return err
		}
	}

	//立刻广播作恶证据
	if emux != nil {
		logger.Trace("broadcastProof")
		broadcastBadWitnessProof(emux, rep)
	}
	//2. 存储
	batch := newBatch(db)
	defer func() {
		batch.Write()
		batch.Discard()
	}()

	writeBadWitneeProof(batch, rep)
	StopChain(cr, batch, rep[0].Header.MC, rep[0].Header.Number)
	log.Warn("found report bad unit", "chain", rep[0].Header.MC, "number", rep[0].Header.Number)
	//4.系统见证人处理作恶，通知已发现作恶
	badUnitEventBus.Send(rep[0].Header.MC)
	return nil
}

func broadcastBadWitnessProof(emux *event.TypeMux, p []types.AffectedUnit) {
	emux.Post(&types.SendDirectEvent{Data: p, Code: protocol.BadWitnessProof})
}

// 停止链并记录停止和波及信息
func StopChain(cr ChainReader, db ntsdb.PutGetter, chain common.Address, number uint64) {
	//3. 停止链操
	chains := stopChain(cr, chain, number, nil)
	// 在停止并收集需要停止的链后
	for chain, number := range chains {
		writeStoppedChain(db, chain, number)
	}
	//登记波及关系
	writeStoppedByWho(db, chain, number, chains)
}

// 恢复链
// 在仲裁接受后，需要恢复所有相关的链
// 根据链找出所有波及到的链，
func RecoverStartChain(cr ChainReader, db ntsdb.Database, chain common.Address, number uint64) error {
	handlLock.Lock()
	defer handlLock.Unlock()

	batch := newBatch(db)
	defer func() {
		batch.Write()
		batch.Discard()
	}()

	tryRecover := func(c ChainNumber) {
		//恢复
		chainCanStart := clearStoppedChainFlag(batch, c.Chain, c.Number)
		if chainCanStart {
			//通知 Start
			cr.PostChainEvent(types.ChainMCProcessStatusChangedEvent{MC: c.Chain, Status: types.ChainCanStart})
		}
	}

	//恢复自身
	tryRecover(ChainNumber{chain, number})

	//获取受影响的链
	chains := GetStoppedChainByBad(batch, chain, number)
	for _, c := range chains {
		tryRecover(c)
	}
	//删除标记
	clearBadChainFlag(batch, chain, number)

	return nil
}

func stopChain(cr ChainReader, badChain common.Address, badNumber uint64, rep []types.AffectedUnit) map[common.Address]uint64 {
	if len(rep) == 0 {
		return nil
	}

	//在用户链中，查找影响面
	//分析last位置：
	// 1.  last-10 > bad 说明是对历史高度重复生成，则不会影响现有
	// 2.  否则这让很有可能不同节点的视图不一样，短期内出现这种情况，需要降低影响面。争取和系统见证人判断的合法性相同
	last := cr.GetChainTailHead(badChain)

	//检索子单元
	mcs := make(map[common.Address]uint64)

	//处理
	stop := func(mc common.Address, number uint64) {
		if mc.Empty() || mc == params.SCAccount || number == 0 { // 不应该停止系统链
			return
		}
		if oldNum, ok := mcs[mc]; ok {
			if oldNum > number { //保持最小位置
				mcs[mc] = number
			}
			return
		}
		mcs[mc] = number
		log.Warn("stop chain", "chain", mc, "number", number, "badChain", badChain, "badNumber", badNumber)
		//通知休眠
		cr.PostChainEvent([]interface{}{types.ChainMCProcessStatusChangedEvent{MC: mc, Status: types.ChainNeedStop}})
	}

	if last.Number > badNumber+10 { //10个高度前的单元视为历史单元
		stop(badChain, badNumber) //对历史前单元作恶时，只需要停止本链即可
		return mcs
	}
	//否则需要停止所有向关联的链
	var lookup func(hash common.Hash)
	lookup = func(hash common.Hash) {
		mc, number := cr.GetUnitNumber(hash)
		if mc.Empty() {
			return
		}
		stop(mc, number)

		//遍历所有子单元，寻找影响
		cr.WakChildren(hash, func(children common.Hash) bool {
			lookup(children)
			return false //continue
		})
	}
	// 开始遍历,也需要根据证据中的单元哈希，停用相关联的链
	if localUnit := cr.GetHeaderByNumber(badChain, badNumber); localUnit != nil {
		lookup(localUnit.Hash())
	}

	for _, b := range rep {
		lookup(b.Header.Hash())
	}
	return mcs
}
