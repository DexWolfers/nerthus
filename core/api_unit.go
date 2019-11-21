// Author: @kulics
package core

import (
	"errors"
	"fmt"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/tracetime"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"
)

type Unit = types.Unit

// GetGenesis 获取创世单元
func (self *DAG) GetGenesis() *Unit {
	return self.dagchain.genesisUnit
}

// GetUnitByHash 根据单元的哈希地址获取单元
func (self *DAG) GetUnitByHash(hash common.Hash) (*Unit, error) {
	unit := self.dagchain.GetUnitByHash(hash)
	if unit == nil {
		return nil, ErrUnitNotFind
	}
	return unit, nil
}

// GetLatestUnitByAddress 根据地址获取最新的单元
// 稳定
func (self *DAG) GetLatestUnitByAddress(mcAccount common.Address) (*Unit, error) {
	return self.dagchain.GetChainTailUnit(mcAccount), nil
}

// GetUnitByIterator 使用迭代器获取单元
func (self *DAG) GetUnitByIterator(account common.Address, fn func(it *Unit) bool) error {
	// 获取用户链最后的位置
	hash := GetChainTailHash(self.db, account)
	if hash.Empty() {
		return ErrLastUnitNotFind
	}
	unit, err := self.GetUnitByHash(hash)
	if err != nil {
		return ErrLastUnitNotFind
	}
	number := unit.Number()
	for {
		// 找到创世则自动退出
		if number == 0 {
			return nil
		}
		// 获取单元
		hash := GetStableHash(self.db, account, number)
		number--
		if hash.Empty() {
			return nil
		}
		u, err := self.GetUnitByHash(hash)
		if err != nil {
			return err
		}
		// 执行迭代方法
		if fn(u) {
			return nil
		}
	}
}

type TxExecMsg struct {
	MC      common.Address
	Action  types.TxAction
	PreStep types.UnitID
}

// GetTxFirstAction 获取指定交易的第一个Action
func (self *DAG) GetTxFirstAction(tx *types.Transaction) (mc common.Address, action types.TxAction, err error) {
	sender, err := tx.Sender()
	if err != nil {
		err = fmt.Errorf("invalid sender,%v", err)
		return
	}

	if tx.To().IsContract() {
		// 创建合约
		if sc.IsCallCreateContractTx(tx.To(), tx.Data()) {
			return sender, types.ActionContractFreeze, nil
		}
		// 执行合约-> 1、系统合约，普通合约
		// 1.系统合约，普通合约
		if tx.To() == params.SCAccount {
			// 系统合约
			if sc.IsSCIdleContract(tx.Data()) {
				log.Debug("is sc idel", "tx", tx.Hash(), "sender", sender)
				// 如果是空转合约，需要校验发送方
				db, err := self.dagchain.GetChainTailState(params.SCAccount)
				if err == nil {
					if s, isW := sc.GetWitnessStatus(db, sender); !isW {
						if isC := sc.IsActiveCouncil(db, sender); !isC {
							err = ErrInvalidScTxSender
						}
					} else if s != sc.WitnessNormal {
						err = ErrInvalidScTxSender
					}
				}
				return params.SCAccount, types.ActionSCIdleContractDeal, err
			} else if tx.Seed() > 0 { //已在交易池中交易，>0则说明是Pow交易
				// powtx
				return params.SCAccount, types.ActionPowTxContractDeal, nil
			}
		}
		// 普通合约
		return sender, types.ActionContractFreeze, nil
	}
	//转账
	return sender, types.ActionTransferPayment, nil
}

// GetTxNextAction 获取指定交易的下一阶段执行内容
func (self *DAG) GetTxNextAction(thash common.Hash, tx *types.Transaction, trs ...*tracetime.TraceTime) (actions []TxExecMsg, err error) {
	var tr *tracetime.TraceTime
	if len(trs) > 0 && trs[0] != nil {
		tr = trs[0]
	} else {
		tr = tracetime.New()
	}
	if tx == nil || thash.Empty() {
		return nil, errors.New("invalid tx or thash")
	}
	if tx.Hash() != thash {
		return nil, errors.New("invalid thash")
	}
	tr.Tag()
	sender, err := tx.Sender()
	if err != nil {
		err = fmt.Errorf("invalid sender,%v", err)
		return
	}
	tr.Tag()
	lastStatus, err := self.GetLastTransactionStatus(thash)
	tr.Tag("io")
	//更加交易情况获取下一个节点
	if err == ErrMissingTransaction { //尚未开始
		// 检查交易是否过期
		if tx.Expired(time.Now()) {
			return nil, ErrTxExpired
		}
		mc, action, err := self.GetTxFirstAction(tx)
		if err != nil {
			return nil, err
		}
		tr.Tag("first")
		//转账
		actions = append(actions, TxExecMsg{MC: mc, Action: action})
		return actions, nil
	} else if err != nil {
		return
	}
	switch lastStatus.Action {
	case types.ActionTransferReceipt, //忽略：转账收款
		types.ActionPowTxContractDeal: // 特殊合约
		return
	case types.ActionContractRefund, types.ActionContractReceipt:
		err = self.GetTransactionStatus(thash, func(info TxStatusInfo) (bool, error) {
			if info.Action == types.ActionContractDeal {
				unitMC, number := GetUnitNumber(self.db, info.UnitHash)
				preStep := types.UnitID{ChainID: unitMC, Height: number, Hash: info.UnitHash}
				// 是否处理合约返回
				stateDB, err := self.GetDagChain().GetChainTailState(sender)
				if err != nil {
					return false, err
				}
				if CheckTxActionInChain(stateDB, sender, thash, types.ActionContractRefund) == nil {
					actions = append(actions, TxExecMsg{MC: sender, Action: types.ActionContractRefund, PreStep: preStep})
				}
				//获取 receipt 查看
				receipt := GetUnitReceiptAt(self.db, info.UnitHash, number, uint16(info.TxExecIndex-1))
				if receipt == nil {
					return false, fmt.Errorf("missing receipt[%d] of %s", info.TxExecIndex-1, info.UnitHash.Hex())
				}
				// 是否存在合约转账
				for _, coinFlow := range receipt.Coinflows {
					if coinFlow.To == unitMC {
						continue
					}
					stateDB, err = self.GetDagChain().GetChainTailState(coinFlow.To)
					if err != nil {
						return false, err
					}
					// 合约转账是否执行完成
					if CheckTxActionInChain(stateDB, coinFlow.To, thash, types.ActionContractReceipt) == nil {
						actions = append(actions, TxExecMsg{MC: coinFlow.To, Action: types.ActionContractReceipt, PreStep: preStep})
					}
				}
				return false, nil
			}
			return true, nil
		})
		return
	}
	unitMC, number := GetUnitNumber(self.db, lastStatus.UnitHash)
	if number == MissingNumber {
		err = fmt.Errorf("not found header by hash(%s)", lastStatus.UnitHash.Hex())
		return
	}
	preStep := types.UnitID{ChainID: unitMC, Height: number, Hash: lastStatus.UnitHash}
	// 从单元体中获取交易执行信息
	lastExecUnit := self.dagchain.GetUnit(lastStatus.UnitHash)
	if lastExecUnit == nil {
		err = fmt.Errorf("not found unit by hash(%s)", lastStatus.UnitHash.Hex())
		return
	}
	lastExecInfo, err := self.GetTxReceiptAtUnit(lastStatus.UnitHash, thash)
	if err != nil {
		return
	}

	GetTxNextActions(unitMC, lastStatus.Action, tx.Hash(), lastExecInfo, tx, func(chain common.Address, txhash common.Hash, tx *types.Transaction, action types.TxAction) {
		actions = append(actions, TxExecMsg{MC: chain, Action: action, PreStep: preStep})
	})
	return
}

func GetTxNextActions(unitMC common.Address, action types.TxAction, txHash common.Hash,
	receipt *types.Receipt, otx *types.Transaction,
	nextAction func(chain common.Address, txhash common.Hash, tx *types.Transaction, action types.TxAction)) {

	// 判断单元是 交易单元还是 合约单元
	switch action {
	default:
		panic(fmt.Errorf("not handle action %s", action))
	case types.ActionTransferPayment:
		// 如果原始交易已经失败，则不需要处理
		if receipt.Failed || otx.To() == otx.SenderAddr() {
			return
		}
		nextAction(otx.To(), txHash, otx, types.ActionTransferReceipt)

	case types.ActionTransferReceipt, //忽略：转账收款
		types.ActionContractReceipt, types.ActionContractRefund, // 合约收款
		types.ActionPowTxContractDeal: // 特殊合约

	case types.ActionContractFreeze:
		// 如果原始交易已经失败，则不需要处理
		if receipt.Failed {
			return
		}

		if otx.To() == params.SCAccount && sc.IsCallCreateContractTx(otx.To(), otx.Data()) {
			//部署合约
			nextAction(params.SCAccount, txHash, otx, types.ActionContractDeal)
		} else {
			//执行合约
			nextAction(otx.To(), txHash, otx, types.ActionContractDeal)
		}

	case types.ActionSCIdleContractDeal:
		// 处理Result coinFlow
		for _, coinFlow := range receipt.Coinflows {
			// 如果流水发送在当前链，则说明已经过处理无效再次处理
			if coinFlow.To == unitMC {
				continue
			}
			nextAction(coinFlow.To, txHash, otx, types.ActionContractReceipt)
		}
	case types.ActionContractDeal:
		// 合约部署(创建合约的第2个单元)，此时已分配新合约地址和合约见证人，此时需要
		if unitMC == params.SCAccount && sc.IsCallCreateContractTx(otx.To(), otx.Data()) {
			// 如果失败，则跳过第2个单元，直接到第三个单元
			// 如果成功，则继续在合约链上执行合约初始化
			if !receipt.Failed {
				contractInfo, err := sc.GetContractOutput(sc.FuncCreateContract, receipt.Output)
				if err != nil {
					panic(err)
					return
				}
				ctWit := contractInfo.(sc.NewCtWit)
				nextAction(ctWit.Contract, txHash, otx, types.ActionContractDeal)
				return
			}
		}

		// 只有是付费交易才需要执行 refund
		if otx.Gas() > 0 {
			// 处理Origin mc
			nextAction(otx.SenderAddr(), txHash, otx, types.ActionContractRefund)
		}
		// 处理Result coinFlow
		for _, coinFlow := range receipt.Coinflows {
			// 如果流水发送在当前链，则说明已经过处理无效再次处理
			if coinFlow.To == unitMC {
				continue
			}
			nextAction(coinFlow.To, txHash, otx, types.ActionContractReceipt)
		}
	}
}

// GetStableHash 获取链指定位置的hash
func (d *DAG) GetStableHash(addr common.Address, number uint64) common.Hash {
	return d.dagchain.GetStableHash(addr, number)
}

// TxExecInfo 根据单元hash和交易hash获取交易回执
func (d *DAG) GetTxReceiptAtUnit(uHash common.Hash, txHash common.Hash) (*types.Receipt, error) {
	var local TxStatusInfo
	err := d.GetTransactionStatus(txHash, func(info TxStatusInfo) (b bool, e error) {
		if info.UnitHash == uHash {
			local = info
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	if local.TxExecIndex == 0 {
		return nil, errors.New("not found transactionExec")
	}
	_, number := GetUnitNumber(d.db, uHash)
	r := GetUnitReceiptAt(d.db, uHash, number, uint16(local.TxExecIndex-1))
	if r == nil {
		return nil, fmt.Errorf("missing receipt[%d] of %s", local.TxExecIndex-1, local.UnitHash.Hex())
	}
	return r, nil
}

// writeBadUnit 记录已坐实的作恶单元
func (dag *DAG) writeBadUnit(uhash common.Hash) {
	dag.badUnitCache.Add(uhash.Hex())
	err := dag.db.Put(badUnitKey(uhash), []byte{1})
	if err != nil {
		log.Crit("failed to save bad unit", "uhash", uhash, "err", err)
	}
}

// removeBad 删除指定坏单元
func (dag *DAG) removeBad(uhash common.Hash) {
	dag.badUnitCache.Delete(uhash.Hex())
	dag.db.Delete(badUnitKey(uhash))
}

// badUnitKey 返回坏单元db key
func badUnitKey(uhash common.Hash) []byte {
	return makeKey(badUnitPrefix, uhash)
}

// IsBadUnit 是否是已登记的非法单元，一般是见证人作恶后标记
func (dag *DAG) IsBadUnit(uhash common.Hash) bool {
	return dag.badUnitCache.Has(uhash.Hex())
}

// loadBadUnit 迭代返回是否存在坏单元
func (dag *DAG) loadBadUnit() {
	dag.db.Iterator(badUnitPrefix, func(key, value []byte) bool {
		dag.badUnitCache.Add(common.BytesToHash(key[len(badUnitPrefix):]).Hex())
		return true
	})
}
