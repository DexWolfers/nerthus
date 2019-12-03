package witness

import (
	"fmt"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/mclock"
	"gitee.com/nerthus/nerthus/common/sort/sortslice"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/types"
)

type (
	WReqKey []byte

	TxMsgVotes struct {
		MsgID string
		Votes types.Votes
	}

	TxExecMsg struct {
		id     msgID
		Action types.TxAction
		TXHash common.Hash
		// 在合适的时候补充
		MC        common.Address
		PreAction types.UnitID
		// 存储
		Tx *types.Transaction

		traceCreated mclock.AbsTime
	}
	ChainPQueueItem struct {
		Chain   common.Address
		TxQueue MsgQueue
	}
)

// 值越大，优先级越高
var actionPriority = [...]int{
	// 特殊交易
	types.ActionPowTxContractDeal:  1,
	types.ActionTransferPayment:    2,
	types.ActionContractDeal:       3,
	types.ActionContractFreeze:     4,
	types.ActionContractRefund:     5,
	types.ActionTransferReceipt:    6,
	types.ActionContractReceipt:    6,
	types.ActionSCIdleContractDeal: 10,
}

// 判断 t 的优先级是否高于 o
// 1. 高等 Action 优先、价格优先、时间优先
func (t *TxExecMsg) Compare(o sortslice.Item) int {
	if t == nil {
		return -1
	}
	if o == nil {
		return 1
	}
	other := o.(*TxExecMsg)
	if other == nil {
		return 1
	}
	if t.Action != other.Action {
		// 返回 1 表示，t 的优先级高于 other
		if l1, l2 := actionPriority[int(t.Action)], actionPriority[int(other.Action)]; l1 > l2 {
			return 1
		} else if l1 < l2 {
			return -1
		}
	}
	if t.Tx == nil {
		return 0
	}
	//	1. 价格优先，
	// 2. 时间优先
	if p1, p2 := t.Tx.GasPrice(), other.Tx.GasPrice(); p1 > p2 {
		return 1
	} else if p1 < p2 {
		return -1
	}
	if n1, n2 := t.Tx.Time(), other.Tx.Time(); n1 < n2 {
		return 1
	} else if n1 == n2 {
		return 0
	}
	return -1
}
func (t *TxExecMsg) Key() interface{} {
	if t.id == emptyID {
		t.id = GetMsgID(t.TXHash, t.Action)
	}
	return t.id
}

func (t *TxExecMsg) ConvertToTxExec() *types.TransactionExec {
	var txExec = new(types.TransactionExec)
	txExec.Action = t.Action
	txExec.TxHash = t.Tx.Hash()
	txExec.Tx = t.Tx
	switch t.Action {
	case types.ActionTransferPayment, types.ActionContractFreeze:
		//skip it
	case types.ActionPowTxContractDeal, types.ActionSCIdleContractDeal:
		// skip it
	case types.ActionContractDeal, types.ActionTransferReceipt:
		// 系统空转单元，跳过第一个单元以及单三个单元
		// first step unit hash
		if !sc.IsSCIdleContract(t.Tx.Data()) {
			txExec.PreStep = t.PreAction
		}
	case types.ActionContractReceipt, types.ActionContractRefund:
		// second step unit hash
		txExec.PreStep = t.PreAction
	default:
		panic(fmt.Sprintf("no the action %v", t.Action))
	}
	return txExec
}

func (item *ChainPQueueItem) Compare(other sortslice.Item) int {
	atop := item.TxQueue.Peek()
	if atop == nil {
		return -1
	}
	return atop.Compare(other.(*ChainPQueueItem).TxQueue.Peek())
}

func (item *ChainPQueueItem) Key() interface{} {
	return item.Chain
}

// newTask 基于交易和Action创建交易任务
func newTask(action types.TxAction, mc common.Address, tx *types.Transaction) *TxExecMsg {
	task := TxExecMsg{
		traceCreated: mclock.Now(),
		Action:       action,
		TXHash:       tx.Hash(),
		Tx:           tx,
		MC:           mc,
	}
	return &task
}

type msgID [1 + common.HashLength]byte

var emptyID msgID

func GetMsgID(tx common.Hash, action types.TxAction) msgID {
	var id msgID
	id[0] = byte(action)
	copy(id[1:], tx.Bytes())
	return id
}
