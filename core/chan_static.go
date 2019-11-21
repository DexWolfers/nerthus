package core

import (
	"errors"
	"sync"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/consensus/arbitration"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"
)

type ChainDataStatic struct {
	dagchain     *DagChain
	db           ntsdb.Database
	chainEventCh chan types.ChainEvent

	quit    chan struct{}
	wg      sync.WaitGroup
	running bool
}

func NewChainDataStatic(dc *DagChain) *ChainDataStatic {
	s := ChainDataStatic{
		dagchain:     dc,
		db:           dc.chainDB,
		quit:         make(chan struct{}),
		chainEventCh: make(chan types.ChainEvent, 10240),
	}
	return &s
}

func (cds *ChainDataStatic) Start() error {
	if cds.running {
		return errors.New("does not allow restart")
	}
	cds.running = true
	cds.listenChain()
	return nil
}

func (cds *ChainDataStatic) Stop() {
	if cds.running == false {
		return
	}
	close(cds.quit)
	cds.wg.Wait()
	cds.running = false
}

func (cds *ChainDataStatic) listenChain() {
	cds.wg.Add(1)
	defer cds.wg.Done()

	sub := cds.dagchain.SubscribeChainEvent(cds.chainEventCh)
	if sub == nil {
		return
	}
	defer sub.Unsubscribe()

	for {
		select {
		case <-cds.quit:
			return
		case <-sub.Err():
			return
		case ev, ok := <-cds.chainEventCh:
			if !ok {
				return
			}
			//单元已存在到，则清理
			cds.unitStabled(ev)
		}
	}
}

func (cds *ChainDataStatic) unitStabled(ev types.ChainEvent) {
	cds.checkSpecialTx(ev) //优先处理
}

func (cds *ChainDataStatic) checkSpecialTx(ev types.ChainEvent) error {

	//如果是系统链，则对特殊交易进行处理
	if ev.Unit.MC() == params.SCAccount {
		for i, t := range ev.Unit.Transactions() {
			cds.scChainTxDone(ev.Unit, t, ev.Receipts[i])
		}
	}
	return nil
}

func (cds *ChainDataStatic) scChainTxDone(unit *types.Unit, v *types.TransactionExec, r *types.Receipt) {
	log.Trace("tx action done on system chain", "txhash", v.TxHash, "action", v.Action, "failed", r.Failed)
	// 判断是否更换系统见证人执行合约，需要发通知给监听服务更新排行榜
	if r.Failed {
		return
	}
	// 非此两中类型的Action可以过滤
	if !(v.Action == types.ActionContractDeal || v.Action == types.ActionPowTxContractDeal || v.Action == types.ActionSCIdleContractDeal) {
		return
	}

	orignTx := v.Tx
	if orignTx == nil {
		orignTx = cds.dagchain.GetTransaction(v.TxHash)
	}
	if orignTx == nil { //不应该发生
		log.Crit("failed to get transaction info when process tx", "txhash", v.TxHash)
		return
	}
	switch sc.SCContractString(orignTx.Data()) {
	case sc.FuncIdSetConfigFinalize:
		// 获取对应提案
		p, err := sc.GetContractInput(orignTx.Data())
		if err != nil {
			log.Crit("listenSCUnit", "err", err)
			return
		}

		stateDB, err := cds.dagchain.StateAt(unit.MC(), unit.Root())
		if err != nil {
			log.Crit("listenSCUnit", "err", err)
			return
		}
		// 读取提案数据
		proposal, err := sc.GetProposalInfoByHash(stateDB, p.(common.Hash), unit.Number())
		if err != nil {
			log.Trace("listenSCUnit", "err", err)
			return
		}
		switch proposal.Type {
		case sc.ProposalTypeCouncilAdd, sc.ProposalTypeCouncilFire:
			// 通知成员业务
			cds.dagchain.dag.eventMux.Post(types.SystemChainUnitEvent{})
		case sc.ProposalTypeConfigChange:
			if proposal.Status == sc.ProposalStatusPassed {
				// 读取提案数据
				// 更新设置
				log.Trace("Chain configuration modification has taken effect",
					"key", proposal.ConfigKey,
					"before", cds.dagchain.config.Get(proposal.ConfigKey), "after", proposal.ConfigValue)

				cds.dagchain.config.Set(proposal.ConfigKey, proposal.ConfigValue)
			}
		}
		return
		//case sc.FuncReportDishonest:
		//	//如果作恶已经坐实，则开始纠正链
		//
		//case sc.FuncApplyWitnessID: // 申请见证人
		//	sender, err := orignTx.Sender()
		//	log.Debug("send witness apply event", "sender", sender)
		//	if err != nil {
		//		log.Error("failed to get tx sender", "txHash", v.TxHash, "err", err)
		//		return
		//	}
		//	who, err := sc.GetReplaseWitnessTraget(sender, orignTx.Data())
		//	state, err := cds.dagchain.StateAt(header.MC,header.StateRoot)
		//	if err != nil {
		//		log.Error("failed to load chain state", "chain", header.MC, "number", header.Number)
		//		return
		//	}
		//	uid := types.UnitID{
		//		ChainID: header.MC,
		//		Hash:    header.Hash(),
		//		Height:  header.Number,
		//	}
		//	index, err := sc.GetGroupIndexFromUser(state, who)
		//	cds.dagchain.dag.eventMux.Post(types.WitnessWorkChainEvent{
		//		WitnessGroup: index,
		//		Chain:        who,
		//		UID:          uid,
		//		Kind:         types.WitnessWorkChainAdd,
		//	})
		//case sc.FuncReplaceWitnessID: // 更换见证人申请
		//	sender, err := orignTx.Sender()
		//	log.Debug("send witness replace event", "sender", sender)
		//	if err != nil {
		//		log.Error("failed to get tx sender", "txHash", v.TxHash, "err", err)
		//		return
		//	}
		//	if err != nil {
		//		log.Error("failed to get witness replace output", "data", common.Bytes2Hex(v.Result.CallContractOutput))
		//		return
		//	}
		//	uid := types.UnitID{
		//		ChainID: header.MC,
		//		Hash:    header.Hash(),
		//		Height:  header.Number,
		//	}
		//	who, err := sc.GetReplaseWitnessTraget(sender, orignTx.Data())
		//	if err != nil {
		//		log.Error("failed to load chain address", "txHash", v.TxHash, "err", err)
		//		return
		//	}
		//	// 发送更换见证人事件
		//	cds.dagchain.PostChainEvent(types.WitnessReplaceEvent{
		//		Hash:   header.Hash(),
		//		Chain:  who,
		//		Status: types.WitnessReplaceUnderway,
		//	})
		//	// drop
		//	{
		//		parent := cds.dagchain.GetHeader(header.ParentHash)
		//		if parent == nil {
		//			return
		//		}
		//		state, err := cds.dagchain.StateAt(parent.MC,parent.StateRoot)
		//		if err != nil {
		//			log.Error("failed to load chain state", "chain", parent.MC, "number", parent.Number)
		//			return
		//		}
		//		oldGroupIndex, err := sc.GetPreGroupIndexFromUser(state, who)
		//		if err != nil {
		//			if err != sc.ErrChainMissingWitness {
		//				log.Error("failed to load chain address", "txHash", v.TxHash, "err", err)
		//			}
		//		} else {
		//			cds.dagchain.dag.eventMux.Post(types.WitnessWorkChainEvent{
		//				WitnessGroup: oldGroupIndex,
		//				Chain:        who,
		//				UID:          uid,
		//				Kind:         types.WitnessWorkChainDrop,
		//			})
		//		}
		//	}
		//case sc.FuncReplaceWitnessExecuteID: // 更换见证人
		//	//成功更换见证人
		//	//不再属于旧的见证人，属于新的见证人
		//	//拉取旧的见证人用户
		//	input, err := sc.UnpackReplaceWitnessExecuteInput(orignTx.Data())
		//	if err != nil {
		//		log.Error("failed to get tx sender", "txHash", v.TxHash, "err", err)
		//		return
		//	}
		//	// 发送更换见证人事件
		//	cds.dagchain.PostChainEvent(types.WitnessReplaceEvent{
		//		Hash:   header.Hash(),
		//		Chain:  input.ID.ChainID,
		//		Status: types.WitnessReplaceDone,
		//	})
		//	// add
		//	{
		//		uid := types.UnitID{
		//			ChainID: header.MC,
		//			Hash:    header.Hash(),
		//			Height:  header.Number,
		//		}
		//		state, err := cds.dagchain.StateAt(header.MC,header.StateRoot)
		//		if err != nil {
		//			log.Error("failed to load chain state", "chain", header.MC, "number", header.Number)
		//			return
		//		}
		//		index, err := sc.GetGroupIndexFromUser(state, input.ID.ChainID)
		//		cds.dagchain.dag.eventMux.Post(types.WitnessWorkChainEvent{
		//			WitnessGroup: index,
		//			Chain:        input.ID.ChainID,
		//			UID:          uid,
		//			Kind:         types.WitnessWorkChainAdd,
		//		})
		//	}
		//case sc.FuncIdRemoveInvalid:
		//	return
		//	// TODO: 需要采用事件方式跟踪变化
		//	sender, err := orignTx.Sender()
		//	if err != nil {
		//		log.Error("failed to get tx sender", "txHash", v.TxHash, "err", err)
		//		return
		//	}
		//
		//	uid := types.UnitID{
		//		ChainID: header.MC,
		//		Hash:    header.Hash(),
		//		Height:  header.Number,
		//	}
		//	// drop
		//	{
		//		parent := cds.dagchain.GetHeader(header.ParentHash)
		//		if parent == nil {
		//			return
		//		}
		//		state, err := cds.dagchain.StateAt(parent.MC,parent.StateRoot)
		//		if err != nil {
		//			log.Error("failed to load chain state", "chain", parent.MC, "number", parent.Number)
		//			return
		//		}
		//
		//		who, err := sc.GetReplaseWitnessTraget(sender, orignTx.Data())
		//		if err != nil {
		//			log.Error("failed to load chain address", "txHash", v.TxHash, "err", err)
		//			return
		//		}
		//
		//		oldGroupIndex, err := sc.GetPreGroupIndexFromUser(state, who)
		//		if err != nil {
		//			if err != sc.ErrChainMissingWitness {
		//				log.Error("failed to load chain address", "txHash", v.TxHash, "err", err)
		//			}
		//		} else {
		//			cds.dagchain.dag.eventMux.Post(types.WitnessWorkChainEvent{
		//				WitnessGroup: oldGroupIndex,
		//				Chain:        who,
		//				UID:          uid,
		//				Kind:         types.WitnessWorkChainDrop,
		//			})
		//		}
		//	}
		//	// add
		//	{
		//		state, err := cds.dagchain.StateAt(header.MC,header.StateRoot)
		//		if err != nil {
		//			log.Error("failed to load chain state", "chain", header.MC, "number", header.Number)
		//			return
		//		}
		//
		//		who, err := sc.GetReplaseWitnessTraget(sender, orignTx.Data())
		//		if err != nil {
		//			log.Error("failed to load chain address", "txHash", v.TxHash, "err", err)
		//			return
		//		}
		//		index, err := sc.GetGroupIndexFromUser(state, who)
		//		cds.dagchain.dag.eventMux.Post(types.WitnessWorkChainEvent{
		//			WitnessGroup: index,
		//			Chain:        who,
		//			UID:          uid,
		//			Kind:         types.WitnessWorkChainAdd,
		//		})
		//	}
	}
}

func (dc *DagChain) processSystemLogs(unit *types.Unit, logs []*types.Log) {
	if unit.MC() != params.SCAccount || len(logs) == 0 {
		return
	}
	for _, logInfo := range logs {
		if logInfo.Removed {
			continue
		}
		if logInfo.ContainsTopic(sc.GetEventTopicHash(sc.ChainArbitrationEvent)) { //链仲裁事件

			chain, number, err := sc.UnpackChainArbitrationEventLog(logInfo.Data)
			if err != nil {
				log.Crit("failed to unpack event data", "topic", sc.ChainArbitrationEvent, "err", err)
			}

			log.Warn("new dishonest process", "chain", chain, "number", number)
			//记录正在仲裁中，系统链不记录
			if chain != params.SCAccount {
				dc.dag.writeChainInInArbitration(chain, number)
				arbitration.StopChain(dc, dc.chainDB, chain, number)
			}
		} else if logInfo.ContainsTopic(sc.GetEventTopicHash(sc.ChainArbitrationEndEvent)) { //恢复链
			//以此已经完成惩罚见证人，需要恢复链操作
			//先登记非法单元
			info, err := sc.UnpackChainArbitrationEndEventLog(logInfo.Data)
			if err != nil {
				log.Crit("failed to unpack event data", "topic", sc.ChainArbitrationEndEvent, "err", err)
			}
			log.Info("dishonest process done", "chain", info.ChainID, "number", info.Height, "uhash", info.Hash)

			//完成仲裁，恢复链
			if err := dc.recoveryChain(info); err != nil {
				log.Crit("failed recovery chain after seen system witness punish bad witness", "err", err)
			}
		} else if logInfo.ContainsTopic(sc.GetEventTopicHash(sc.ChainStatusChangedEvent)) {
			info, err := sc.UnpackChainStatusChangedEvent(logInfo.Data)
			if err != nil {
				log.Crit("failed to call UnpackChainStatusChangedEvent", "err", err)
			}
			if info.First && info.Status == sc.ChainStatusNormal {
				dc.PostChainEvent(types.ChainMCProcessStatusChangedEvent{MC: info.Chain, Status: types.ChainActivate})
			}
		} else if logInfo.ContainsTopic(sc.GetEventTopicHash(sc.UserWitnessChangeEvent)) {
			info, err := sc.UnpackUserWitnessChangeLog(logInfo.Data)
			if err != nil {
				log.Crit("failed to call UnpackUserWitnessChangeLog", "err", err)
			}
			db, err := dc.GetUnitState(unit.Hash())
			if err != nil {
				log.Crit("failed to get unit state db", "err", err)
			}
			startNumber := sc.GetChainWitnessStartNumber(db, info.User)
			//此时本地不应该存在任何在此高度开始的单元。
			//如果存在则需要抹除
			uhash := dc.GetStableHash(info.User, startNumber)
			log.Trace("chain new witness", "chain", info.User, "group", info.NewWitnessGroup,
				"oldGroup", info.OldWitnessGroup, "start", startNumber)

			if !uhash.Empty() {
				log.Warn("found need remove unit", "chain", info.User, "number", startNumber, "uhash", uhash)
				dc.ClearUnit(uhash, true)
			}
			//检查上一个单元的特殊性
			if startNumber > 1 {
				if sh := sc.GetChainSpecialUnit(db, info.User, startNumber-1); !sh.Empty() {
					uhash := dc.GetStableHash(info.User, startNumber-1)
					if !uhash.Empty() && uhash != sh {
						log.Warn("found need remove unit", "chain", info.User, "number", startNumber, "uhash", uhash)
						dc.ClearUnit(uhash, true)
					}
				}
			}
		}
	}
}
