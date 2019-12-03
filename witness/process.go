// Author: @kulics
package witness

import (
	"fmt"

	"gitee.com/nerthus/nerthus/core/txpool"
	"github.com/pkg/errors"

	"gitee.com/nerthus/nerthus/common/tracetime"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"
)

type ErrMissUnit struct {
	unit common.Hash
}

func (e *ErrMissUnit) Error() string {
	return fmt.Sprintf("miss unit %s", e.unit.Hex())
}

// ProcessIrreversibleEvent 处理收到的新单元
// 循环处理单元中的交易,出下一个单元
func (t *WitnessSchedule) ProcessIrreversibleEvent(ev types.ChainEvent) error {

	tr := tracetime.New()
	defer tr.Stop()
	unit := ev.Unit
	tr.Tag()
	log.Debug("WitnessSchedule.runReceivedIrreversibleUnit",
		"mc", unit.MC(), "number",
		unit.Number(), "hash", unit.Hash(),
		"generator", unit.Proposer())

	unitMC := unit.MC()

	//需要优先检查更换见证情况。
	// 因为一种情况是交易处理和更换操作出现在同一个单元中，因此需要提交检查
	if unitMC == params.SCAccount {
		if err := t.processCheckReplaceWitness(ev); err != nil {
			log.Error("failed process check", "uhash", unit.Hash(), "err", err)
		}
	}
	var otx *types.Transaction

	unitID := unit.ID()
	for i, tx := range unit.Transactions() {
		log.Trace("process irreversible unit tx", "uhash", unit.Hash(), "tx", tx.TxHash, "action", tx.Action)

		//已完成，则删除
		t.nts.TxPool().TODOPool().Del(unitMC, tx.TxHash, tx.Action)

		otx = tx.Tx
		if otx == nil {
			otx = t.nts.TxPool().Get(tx.TxHash)
			if otx == nil {
				otx = t.nts.DagChain().GetTransaction(tx.TxHash)
			}
		}
		if otx == nil {
			log.Crit("not found the transaction", "tx", tx.TxHash)
		}

		core.GetTxNextActions(unitMC, tx.Action, tx.TxHash, ev.Receipts[i], otx, func(chain common.Address, txhash common.Hash, tx *types.Transaction, action types.TxAction) {
			t.nts.TxPool().TODOPool().Push(chain, txhash, tx, action)

			if t.MyIsUserWitness(chain) != nil {
				t.logger.Trace("skip tx next action", "txhash", txhash, "chain", chain, "action", action)
			} else {
				t.addExecMessage(action, tx, unitID, chain)
			}
		})
	}
	return nil
}

// processCheckReplaceWitenss 处理申请,更换和注销见证人
func (t *WitnessSchedule) processCheckReplaceWitness(ev types.ChainEvent) error {

	var hasUpdateWitnessGroup bool
	t.logger.Debug("process check replace witness event", "uhash", ev.UnitHash, "hasUpdateGroup", hasUpdateWitnessGroup, "logs", len(ev.Logs))
	for _, event := range ev.Logs {
		if event.Removed {
			continue
		}
		//有可能因为自己被踢出而停止
		select {
		case <-t.quite:
		default:
		}
		switch {
		case event.ContainsTopic(sc.GetEventTopicHash(sc.ChainStatusChangedEvent)):
			if t.isSys {
				continue
			}
			info, err := sc.UnpackChainStatusChangedEvent(event.Data)
			if err != nil {
				log.Error("call UnpackChainStatusChangedEvent failed", "txHash", event.TxHash, "err", err)
				continue
			}
			log.Trace("chain status changed", "chain", info.Chain, "status", info.Status, "group", info.Group)
			switch info.Status {
			case sc.ChainStatusWitnessReplaceUnderway:
				//停止链操作，并移除地址
				if t.stopChain(info.Chain) {
					t.updateMyUser(info.Chain, false)
				}
			case sc.ChainStatusNormal:
				//链正常状态，如果是我的见证人则开启，否则关闭
				isMyUser := info.Group == t.curWitnessGroup
				t.updateMyUser(info.Chain, isMyUser)
				if isMyUser {
					if !info.First {
						t.jobCenter.RemoveBlack(info.Chain)
					}
					go t.addOldTx(info.Chain)
				} else if !info.First {
					t.stopChain(info.Chain)
				}
			}
		case event.ContainsTopic(sc.GetEventTopicHash(sc.WitnessChangeEvent)):
			t.logger.Debug("process witness change event", "uhash", ev.UnitHash, "updated", hasUpdateWitnessGroup)
			if hasUpdateWitnessGroup {
				continue
			}
			wlog, err := sc.UnpackWitnessChangeLog(event.Data)
			if err != nil {
				log.Error("failed to unpack", "txHash", event.TxHash, "err", err)
				continue
			}
			// 只处理相关组
			if wlog.Group != t.curWitnessGroup {
				continue
			}
			//只需要更新一次即可
			if err := t.updateWitnessGroup(); err != nil {
				log.Error("updateWitnessGroup failed", "txHash", event.TxHash, "err", err)
			}
			hasUpdateWitnessGroup = true
		case event.ContainsTopic(sc.GetEventTopicHash(sc.WitnessGroupChangedEvent)):
			if t.isSys {
				continue
			}

			w, oldGp, newGp, err := sc.UnpackWitnessGroupChangedEvent(event.Data)
			if err != nil {
				log.Error("failed to unpack", "txHash", event.TxHash, "err", err)
				continue
			}
			t.logger.Info("witness group changed", "witness", w, "old", oldGp, "new", newGp)
			if w == t.curWitness {
				//此时说明本人的见证发送变化，需要清理数据重新开始
				if err := t.initOrReset(true); err != nil {
					//必须退出
					t.logger.Crit("reset failed", "err", err)
				}
			} else if newGp == t.curWitnessGroup { //如果有人加入此组，需要更新见证人任意
				if err := t.reloadWitnessList(); err != nil {
					t.logger.Crit("reload witness list failed", "err", err)
				}
			}
		}

	}
	return nil
}
func (t *WitnessSchedule) updateWitnessGroup() error {
	stateDB, err := t.nts.DagChain().GetChainTailState(params.SCAccount)
	if err != nil {
		return err
	}
	info, err := sc.GetWitnessInfo(stateDB, t.nts.GetMainAccount())
	if err != nil {
		return err
	}
	// 当前已不是见证人
	if info.Status != sc.WitnessNormal {
		// TODO 可以在此发送见证人退出见证组信号,见证组就不必定时检测见证组成员状态
		log.Warn("you are not witness now. stop witness service")
		go t.nts.StopWitness()
		return errors.New("you are not witness now")
	}
	// TODO: 有重复reload

	// 修改当前见证组缓存
	oldList := t.chainLastWitness.list
	// 刷新当前见证组
	if err = t.reloadWitnessList(); err != nil {
		return err
	}

	//如果我是由备系统见证人成为主见证人，则立即干活
	if t.isSys && oldList.HaveIdx(t.curWitness) >= params.ConfigParamsSCWitness && t.IsSCMainWitness() {
		//err = t.runMinne()
		err = t.initOrReset(true)
		t.logger.Info("reserve witness become officially", "err", err)
	}
	// 主见证人变成备选
	if t.isSys && oldList.HaveIdx(t.curWitness) < params.ConfigParamsSCWitness && !t.IsSCMainWitness() {
		//err = t.stopMinne()
		err = t.initOrReset(true)
		t.logger.Info("officially witness become secondary", "err", err)
	}
	return nil
}

// addOldTx 查找用户的未处理交易
// 从用户的最后一个单元时间开始,查找到当前时间的所有交易
func (t *WitnessSchedule) addOldTx(from common.Address) error {
	txs := make(map[common.Hash]struct{})

	needCheck := from.Empty()

	cb := func(msg txpool.ExecMsg) bool {
		myUser := t.MyIsUserWitness(msg.Chain) == nil

		t.logger.Trace("check todo", "chain", msg.Chain, "tx", msg.TxHash, "action", msg.Action, "myUser", myUser)

		if needCheck && !myUser {
			return true
		}
		if _, ok := txs[msg.TxHash]; ok { //不重复处理
			return true
		}
		txs[msg.TxHash] = struct{}{}

		//优先从交易池中查找
		tx := t.nts.TxPool().Get(msg.TxHash)
		if tx == nil {
			tx = t.nts.DagChain().GetTransaction(msg.TxHash)
		}
		if tx == nil {
			//TODO:可以广播拉取
			t.logger.Debug("can not get tx info", "tx", msg.TxHash)
		} else {
			t.ProcessNewTx(tx, nil)
		}
		return true
	}

	if from.Empty() {
		return t.nts.TxPool().TODOPool().Range(cb)
	}
	return t.nts.TxPool().TODOPool().RangeChain(from, cb)
}

// ProcessNewTx 处理新交易
func (t *WitnessSchedule) ProcessNewTx(tx *types.Transaction, tr *tracetime.TraceTime) {
	t.logger.Debug("process new tx", "txHash", tx.Hash())
	err := t.processTransactionEvent(tx, tr)
	if err != nil {
		t.logger.Debug("failed to process tx", "tx", tx.Hash(), "err", err)
		//失败则主动移除交易.
		// 本地交易则不需要清除 TODO
		t.nts.TxPool().Remove(tx.Hash())
	}
}

// processTransactionEvent 处理交易
func (t *WitnessSchedule) processTransactionEvent(tx *types.Transaction, tr *tracetime.TraceTime) error {
	nexts, err := t.nts.DagReader().GetTxNextAction(tx.Hash(), tx, tr)
	if err != nil {
		return fmt.Errorf("failed replay transaction,%v", err)
	}
	if len(nexts) == 0 {
		return errors.New("failed replay transaction,next action nil")
	}
	for _, v := range nexts {
		if err := t.MyIsUserWitness(v.MC); err == nil {
			//tr.Tag()
			t.addExecMessage(v.Action, tx, v.PreStep, v.MC, tr)
		} else {
			t.logger.Trace("skip tx",
				"tx", tx.Hash(), "action", v.Action, "chain", v.MC, "witnessGp", t.curWitnessGroup)
		}
	}
	return nil
}

// includeTxAction 判断交易的action是否已经处理
func (t *WitnessSchedule) includeTxAction(mc common.Address, txHash common.Hash, action types.TxAction) bool {
	unitHash := t.nts.DagChain().GetChainTailHash(mc)
	header := t.nts.DagChain().GetHeaderByHash(unitHash)
	if header == nil {
		log.Warn("not found the header", "unitHash", unitHash)
		return false
	}
	db, err := t.nts.DagChain().StateAt(header.MC, header.StateRoot)
	if err != nil {
		log.Warn("read state db fail", "err", err)
		return false
	}
	return core.CheckTxActionInChain(db, mc, txHash, action) != nil
}

// getOriginTx 从交易执行体中获取原始交易
func (t *WitnessSchedule) getOriginTx(tx *types.TransactionExec) *types.Transaction {
	if tx == nil {
		return nil
	}
	if tx.Tx != nil {
		return tx.Tx
	}
	//TODO:可以优化从 DagChain 中取交易
	return core.GetTransaction(t.nts.ChainDb(), tx.TxHash)
}
