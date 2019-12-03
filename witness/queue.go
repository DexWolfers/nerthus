package witness

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/objectpool"
	"gitee.com/nerthus/nerthus/common/sort/sortslice"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/config"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"
)

type MsgQueue interface {
	Push(msg *TxExecMsg) bool
	Remove(txhash common.Hash, action types.TxAction)
	Len() int64
	NeedWork() bool
	Range(f func(msg *TxExecMsg) bool)
	Pop() *TxExecMsg
	Peek() *TxExecMsg
	Exist(msg *TxExecMsg) bool
}

type ChainJobQueue struct {
	chain   common.Address
	msgList *sortslice.List
	ser     *MiningJobCenter
}

func (cj *ChainJobQueue) Remove(txhash common.Hash, action types.TxAction) {
	if cj.msgList == nil {
		return
	}
	if cj.msgList.Remove(GetMsgID(txhash, action)) {
		if cj.ser != nil {
			cj.ser.addCount(-1)
		}
	}

	if config.InStress {
		log.Trace("delete tx from queue", "chain", cj.chain, "tx", txhash, "action", action)
	}
}

func (cj *ChainJobQueue) Push(m *TxExecMsg) bool {
	if cj.msgList == nil {
		return false
	}
	ok := cj.msgList.Add(m)
	if ok && cj.ser != nil {
		cj.ser.addCount(1)
	}
	if config.InStress {
		log.Trace("push tx to queue", "tx", m.TXHash, "action", m.Action, "repeat", !ok)
	}
	return ok
}
func (cj *ChainJobQueue) Pop() *TxExecMsg {
	if cj.msgList == nil {
		return nil
	}
	item := cj.msgList.Pop()
	if item != nil {
		if cj.ser != nil {
			cj.ser.addCount(-1)
			if cj.Len() == 0 {
				cj.ser.onJobIsEmpty(cj.chain)
			}
		}
		return item.(*TxExecMsg)
	}
	return nil
}
func (cj *ChainJobQueue) Len() int64 {
	if cj.msgList == nil {
		return 0
	}
	return int64(cj.msgList.Len())
}

func (cj *ChainJobQueue) Peek() *TxExecMsg {
	if cj.msgList == nil {
		return nil
	}
	top := cj.msgList.Get(0)
	if top == nil {
		return nil
	}
	return top.(*TxExecMsg)
}
func (cj *ChainJobQueue) Exist(msg *TxExecMsg) bool {
	if cj.msgList == nil {
		return false
	}
	return cj.msgList.Exist(msg.Key())
}

//遍历时，将会锁住列表
func (cj *ChainJobQueue) Range(f func(msg *TxExecMsg) bool) {
	if cj.msgList == nil {
		return
	}
	cj.msgList.Range(func(item sortslice.Item) bool {
		return f(item.(*TxExecMsg))
	})
}

func (cj *ChainJobQueue) NeedWork() bool {
	sdb, err := cj.ser.api.DagChain().GetChainTailState(cj.chain)
	if err != nil {
		log.Crit("failed to load chain tail stats", "chain", cj.chain, "err", err)
		return false
	}
	var ok bool
	needRemove := make([]*TxExecMsg, 0)
	if cj.msgList == nil {
		return false
	}
	cj.msgList.Range(func(item sortslice.Item) bool {
		msg := item.(*TxExecMsg)
		// find one ok
		if core.CheckTxActionInChain(sdb, cj.chain, msg.TXHash, msg.Action) == nil {
			log.Trace("job queue check need work", "chain", cj.chain, "tx", msg.TXHash, "action", msg.Action)
			ok = true
			return false
		}
		needRemove = append(needRemove, msg)

		return true
	})
	for _, msg := range needRemove {
		cj.Remove(msg.TXHash, msg.Action)
	}
	return ok
}

func (cj *ChainJobQueue) Start() {
	if cj.msgList != nil {
		cj.msgList = sortslice.Get()
	}
}
func (cj *ChainJobQueue) Stop() {
	log.Trace("clear chain msg queue", "chain", cj.chain, "len", cj.Len())
	cj.msgList.Range(func(item sortslice.Item) bool {
		msg := item.(*TxExecMsg)
		log.Trace("remove msg from chain msg queue", "chain", cj.chain,
			"tx", msg.TXHash,
			"action", msg.Action)
		return true
	})
	if cj.ser != nil {
		cj.ser.addCount(-cj.msgList.Len())
	}
	sortslice.Put(cj.msgList)
}
func (cj *ChainJobQueue) Key() objectpool.ObjectKey {
	return cj.chain
}
func (cj *ChainJobQueue) Reset(key objectpool.ObjectKey, args ...interface{}) {
	panic("disable call this Reset function")
}
func (cj *ChainJobQueue) Handle(args []interface{}) {
	if len(args) == 0 {
		return
	}
	tx := args[0].(*TxExecMsg)
	cj.Push(tx)
	if cj.ser.onPush != nil {
		cj.ser.onPush(tx)
	}
}

// 除非强制停止，否则不允许停止链，防止清理数据
func (cj *ChainJobQueue) CanStop() bool {
	select {
	case <-cj.ser.quite:
		return true
	default:
	}
	if cj.ser.stopped {
		return true
	}
	//如果是系统链，则不能停止
	if cj.chain == params.SCAccount {
		return false
	}
	//如果是在黑名单中，则可以Stop
	if cj.ser.IsInBlack(cj.chain) {
		return true
	}
	return cj.Len() == 0
}
