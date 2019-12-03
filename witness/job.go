package witness

import (
	"sync"
	"sync/atomic"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/mclock"
	"gitee.com/nerthus/nerthus/common/objectpool"
	"gitee.com/nerthus/nerthus/common/sort/sortslice"
	"gitee.com/nerthus/nerthus/core/config"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"

	"github.com/pkg/errors"
)

// MiningJobCenter 挖矿任务管控中心，管理所有待处理的交易
type MiningJobCenter struct {
	size              int64         //实时计算的待处理交易总量
	sizeCap           int64         //允许存储的待处理交易上限
	declinec          chan struct{} //等待信号
	blackChains       sync.Map      //链黑名单
	chainJobs         *objectpool.ObjectPool
	chains            *sortslice.List //所有链
	onPush            func(msg *TxExecMsg)
	api               Backend
	stopped           bool
	chainLastSortTime mclock.AbsTime //记录链排序时间戳
	quite             chan struct{}
}

func NewMiningJobCenter(api Backend, queueUpper int, onPush func(msg *TxExecMsg)) *MiningJobCenter {
	c := &MiningJobCenter{
		api:   api,
		quite: make(chan struct{}),
		//chainJobs: cmap.NewWith(2000),
		declinec: make(chan struct{}, 1),
		onPush:   onPush,
		chains:   sortslice.NewList(2000),
		sizeCap:  int64(config.MustInt("runtime.witness_job_queue", 10000000)),
	}
	c.chainJobs = objectpool.NewObjPool(false, time.Minute, c.newChainJobQueue)
	c.chainJobs.SetUpperLimit(queueUpper)
	c.chainJobs.Start()
	log.Debug("start mining job center", "upper limit", queueUpper)
	return c
}

// 所有交易任务数
// 注意：该数据不精准
func (center *MiningJobCenter) Txs() int64 {
	return center.size
}
func (center *MiningJobCenter) RangeChain(f func(chain common.Address) bool) {
	//5 秒内不进行排序
	if time.Duration(mclock.Now()-center.chainLastSortTime) > time.Second*5 {
		center.chains.Sort()
		center.chainLastSortTime = mclock.Now()
	}
	center.chains.Range(func(item sortslice.Item) bool {
		return f(item.(*ChainPQueueItem).Chain)
	})
}

func (center *MiningJobCenter) onJobIsEmpty(chain common.Address) {
	// 当一条链需要处理的交易为空时，清理数据
	center.removeChainDone(chain)
	//	这里不需要清理center.chainJobs,将由内部自行管理
}
func (center *MiningJobCenter) removeChainDone(chain common.Address) {
	center.chains.Remove(chain)
}
func (center *MiningJobCenter) onJobPushed(chain common.Address) {
	if center.chains.Exist(chain) {
		return
	}
	center.chains.Add(&ChainPQueueItem{
		Chain:   chain,
		TxQueue: center.Load(chain),
	})
}

// 移除任务
// 移除任务常见情况是交易已经处理完成，本节点可以放弃无效处理。
func (center *MiningJobCenter) Remove(chain common.Address, txhash common.Hash, action types.TxAction) {
	if q := center.Load(chain); q != nil {
		q.Remove(txhash, action)
		// 队列数据已空，等待自动释放
		if q.Len() == 0 {
			log.Debug("try to free job queue",
				"chain", chain, "len1", center.chainJobs.Count(), "len2", center.size)
			center.freeChain(chain, false)
		}
	}
}

func (center *MiningJobCenter) ChainTxs(chain common.Address) int64 {
	q := center.Load(chain)
	if q == nil {
		return 0
	}
	return q.Len()
}

func (center *MiningJobCenter) Push(chain common.Address, tx *types.Transaction, action types.TxAction, pre types.UnitID) error {
	msg := newTask(action, chain, tx)
	msg.PreAction = pre

	if !center.beforePush(msg) {
		return errors.New("ignore new job")
	}
	// 如果正在等待释放，取消。必须提前取消
	center.chainJobs.CancelFree(msg.MC)
	center.chainJobs.Handle(msg.MC, msg)

	//如果是新链将被记录
	center.onJobPushed(msg.MC)
	return nil
}

func (center *MiningJobCenter) beforePush(msg *TxExecMsg) bool {
	if center.stopped {
		return false
	}
	//已在黑名单
	if _, ok := center.blackChains.Load(msg.MC); ok {
		return false
	}
	//如果消息已存在则忽略
	//暂时不开启检查，因为消息进入后一般不存在重复
	//if q := center.Load(msg.MC); q != nil && q.Exist(msg) {
	//	return false
	//}

	// 不走原子取数检查，因为不加锁操作情况下无法保证Size的准确性。
	// 但为了性能考虑，允许 Size 浮动。
	if center.sizeCap > 0 && center.size >= center.sizeCap {
		//lock
		center.addCount(1)
		defer center.addCount(-1)
		//等待
		select {
		case <-center.quite:
			return false
		case <-center.declinec:
			return true
		}
	}
	return true
}

//func (center *MiningJobCenter) Pop(chain common.Address) *TxExecMsg {
//	if center.stopped {
//		return nil
//	}
//	q := center.Load(chain)
//	if q == nil {
//		return nil
//	}
//
//	return q.Pop()
//}

func (center *MiningJobCenter) Chains() []common.Address {
	keys := center.chainJobs.Keys()
	list := make([]common.Address, 0, len(keys))
	for _, v := range keys {
		addr, err := common.DecodeAddress(v)
		if err != nil {
			log.Error("decode address failed", "chain", v, "err", err)
		} else {
			list = append(list, addr)
		}
	}
	return list
}

func (center *MiningJobCenter) LoadOrStore(chain common.Address) MsgQueue {
	q := center.Load(chain)
	if q != nil {
		return q
	}
	center.chainJobs.Handle(chain)
	return center.Load(chain)
}

func (center *MiningJobCenter) Load(chain common.Address) MsgQueue {
	v, ok := center.chainJobs.Get(chain).(MsgQueue)
	if !ok {
		return nil
	}
	return v
}

func (center *MiningJobCenter) newChainJobQueue(chain objectpool.ObjectKey) objectpool.Object {
	return &ChainJobQueue{
		msgList: sortslice.Get(),
		ser:     center,
		chain:   chain.(common.Address),
	}
}

func (center *MiningJobCenter) addCount(n int) {
	// 数量降低，给出信号
	atomic.AddInt64(&center.size, int64(n))
	if n < 0 {
		select {
		case center.declinec <- struct{}{}:
		default:
		}
	}
}
func (center *MiningJobCenter) freeChain(chain common.Address, immediately bool) {
	center.chainJobs.Free(chain, func(s string) {
		center.removeChainDone(chain)
		log.Debug("auto free mining queue", "chain", s)
	}, immediately)
}

func (center *MiningJobCenter) ClearChainForce(chain common.Address) {
	center.freeChain(chain, true)
}

func (center *MiningJobCenter) PushBlack(chain common.Address) bool {
	_, loaded := center.blackChains.LoadOrStore(chain, struct{}{})
	return !loaded
}
func (center *MiningJobCenter) RemoveBlack(chain common.Address) {
	center.blackChains.Delete(chain)
}
func (center *MiningJobCenter) IsInBlack(chain common.Address) bool {
	_, ok := center.blackChains.Load(chain)
	return ok
}

func (center *MiningJobCenter) Stop() {
	center.stopped = true
	close(center.quite)

	//清理数据
	center.chainJobs.Stop()
}
func (center *MiningJobCenter) Reset() {
	center.stopped = true
	//清理数据
	center.chains.Release()
	center.chainJobs.Reset()
	center.blackChains.Range(func(key, value interface{}) bool {
		center.blackChains.Delete(key)
		return true
	})
	center.chainLastSortTime = mclock.Now()
	atomic.AddInt64(&center.size, 0)
	center.stopped = false
}
