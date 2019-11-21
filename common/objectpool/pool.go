package objectpool

import (
	"time"

	"gitee.com/nerthus/nerthus/core/config"

	"gitee.com/nerthus/nerthus/common/sync/lfreequeue"
	"gitee.com/nerthus/nerthus/log"

	"gitee.com/nerthus/nerthus/common/sync/cmap"
)

func NewObjPool(reuse bool, feeDelay time.Duration, newObj func(key ObjectKey) Object) *ObjectPool {
	return &ObjectPool{
		upperLimit:     0,
		autoFeeTime:    feeDelay,
		reuse:          reuse,
		newObj:         newObj,
		waitQueueUpper: 0,
	}
}

// 对象池
// 统一管理对象实例，实现上限排队和自动释放
type ObjectPool struct {
	upperLimit     int
	autoFeeTime    time.Duration
	reuse          bool
	container      cmap.ConcurrentMap
	newObj         func(key ObjectKey) Object
	freeChan       cmap.ConcurrentMap
	waitQueue      cmap.ConcurrentMap
	waitQueueCh    *lfreequeue.Queue
	waitQueueUpper int
	nextSelector   func(exist func(key ObjectKey) bool) ObjectKey
	quit           chan struct{}
}

func (this *ObjectPool) Start() {
	this.container = cmap.New()
	this.freeChan = cmap.New()
	this.waitQueue = cmap.New()
	this.waitQueueCh = lfreequeue.NewQueue()
	this.quit = make(chan struct{})
	go this.dispatch()
}
func (this *ObjectPool) SetUpperLimit(upper int) {
	this.upperLimit = upper
}
func (this *ObjectPool) SetWaitUpper(upper int) {
	this.waitQueueUpper = upper
}
func (pool *ObjectPool) SetSelect(f func(exist func(key ObjectKey) bool) ObjectKey) {
	pool.nextSelector = f
}

func (this *ObjectPool) Handle(key ObjectKey, values ...interface{}) {
	keyStr := key.Text()
	if item, ok := this.container.Get(keyStr); ok {
		item.(Object).Handle(values)
		return
	}
	if this.upperLimit == 0 || this.container.Count() < this.upperLimit {
		item, _ := this.container.LoadOrStore(keyStr, func() interface{} {
			obj := this.newObj(key)
			obj.Start()
			return obj
		})
		item.(Object).Handle(values)
		return
	}

	// 容量已达上限
	l, ok := this.waitQueue.LoadOrStore(keyStr, func() interface{} {
		return lfreequeue.NewQueue()
	})
	lfq := l.(*lfreequeue.Queue)
	lfq.Enqueue(values)
	// 排队
	if !ok {
		this.waitQueueCh.Enqueue(key)
	}
	// 堆积消息太多，插队处理
	if ok && this.waitQueueUpper > 0 && lfq.Len() > int64(this.waitQueueUpper) {
		go func(k ObjectKey, v *lfreequeue.Queue) {
			newObj, exist := this.container.LoadOrStore(k.Text(), func() interface{} {
				obj := this.newObj(key)
				obj.Start()
				return obj
			})
			this.waitQueue.Remove(k.Text())
			if exist {
				return
			}
			for {
				if msg, b := v.Dequeue(); !b {
					break
				} else {
					newObj.(Object).Handle(msg.([]interface{}))
				}
			}
		}(key, lfq)
	}
}
func (this *ObjectPool) Get(key ObjectKey) Object {
	if this.container == nil {
		return nil
	}
	obj, ok := this.container.Get(key.Text())
	if !ok {
		return nil
	}
	return obj.(Object)
}
func (this *ObjectPool) Keys() []string {
	return this.container.Keys()
}
func (this *ObjectPool) Stop() {
	all := this.container.Items()
	for _, v := range all {
		v.(Object).Stop()
	}
	close(this.quit)

}
func (this *ObjectPool) Count() int {
	return this.container.Count()
}
func (this *ObjectPool) WaitCount() int {
	return this.waitQueue.Count()
}

// 发出释放信号
// 等待释放期间，有新消息触发会取消释放
func (this *ObjectPool) Free(key ObjectKey, callback func(string), immediately bool) {
	if immediately {
		item, ok := this.container.Get(key.Text())
		if !ok {
			return
		}
		this.container.Remove(key.Text())
		item.(Object).Stop()
		callback(key.Text())
		return
	}
	freeTime := time.Now().Add(this.autoFeeTime)

	// 达到时间后，释放
	this.freeChan.SetIfAbsent(key.Text(), []interface{}{freeTime, callback})

}

// 取消释放，在真正释放之前有效
func (this *ObjectPool) CancelFree(key ObjectKey) {
	if key == nil {
		return
	}
	this.freeChan.Remove(key.Text())

}
func (this *ObjectPool) Reset() {
	this.Stop()
	this.Start()
}
func (this *ObjectPool) Range(fn func(key string, v interface{}) bool) {
	this.container.IterCb(fn)
}

func (this *ObjectPool) dispatch() {
	freeFuc := func(key string) {
		var (
			removed bool
			addKey  string
		)
		if config.InStress {
			defer func() {
				log.Trace("dispatch", "key", key, "removed", removed, "add", addKey,
					"len1", this.container.Count(), "len2", this.waitQueue.Count())
			}()
		}

		item, ok := this.container.Get(key)
		if !ok {
			return
		}
		//停止前先询问是否能删除
		if asker, ok := item.(StopAsk); ok && !asker.CanStop() {
			return
		}
		obj := item.(Object)
		this.container.Remove(key)
		removed = true

		if this.container.Count() >= this.upperLimit {
			obj.Stop()
			return
		}
	checkline:
		// 是否有排队的任务
		nextKey := this.selectNext()
		//log.Info("select next obj key", "curr", key, "next", nextKey, "objs", this.Count(), "wait", this.waitQueueCh.Len())
		if nextKey == nil || nextKey.Text() == "" {
			// 没有排队的任务，直接关闭
			obj.Stop()
			return
		}

		nextKeyStr := nextKey.Text()
		nextQueu, ok := this.waitQueue.Get(nextKeyStr)
		if !ok {
			obj.Stop()
			goto checkline
		}
		this.waitQueue.Remove(nextKeyStr)
		var newObjFunc func() interface{}
		if this.reuse {
			newObjFunc = func() interface{} {
				obj.Reset(nextKey)
				return obj
			}
		} else {
			obj.Stop()
			newObjFunc = func() interface{} {
				obj := this.newObj(nextKey)
				obj.Start()
				return obj
			}
		}
		addKey = nextKeyStr
		this.container.LoadOrStore(nextKeyStr, newObjFunc)
		go this.waitQueueGoHandle(nextKey, nextQueu.(*lfreequeue.Queue))

	}
	for {
		select {
		case <-this.quit:
			return
		default:
		}
		now := time.Now()
		freeMap := this.freeChan.Items()
		for ikey, ival := range freeMap {
			arg := ival.([]interface{})
			if now.After(arg[0].(time.Time)) {
				if !this.freeChan.Has(ikey) {
					continue
				}
				freeFuc(ikey)
				this.freeChan.Remove(ikey)
				// 回调
				if len(arg) == 2 {
					call, ok := arg[1].(func(string))
					if ok && call != nil {
						call(ikey)
					}
				}
			}
		}
		// 如果存在排队任务，按需提前释放
		if wait := this.waitQueue.Count(); this.waitQueueCh.Len() > 0 && wait > 0 {

			for ikey, ival := range freeMap {
				if wait <= 0 {
					break
				}
				if !this.freeChan.Has(ikey) {
					continue
				}
				arg := ival.([]interface{})
				freeFuc(ikey)
				this.freeChan.Remove(ikey)
				// 回调
				if len(arg) == 2 {
					call, ok := arg[1].(func(string))
					if ok && call != nil {
						call(ikey)
					}
				}
				wait--
			}
		}
		time.Sleep(time.Second)
	}

	this.container = nil
	this.freeChan = nil
	this.waitQueue = nil
	this.waitQueueCh = nil
}

func (this *ObjectPool) waitQueueGoHandle(nextKey ObjectKey, queue *lfreequeue.Queue) {
	for {
		if e, b := queue.Dequeue(); b {
			this.Handle(nextKey, e.([]interface{})...)
		} else {
			break
		}
	}
}

func (pool *ObjectPool) selectNext() ObjectKey {
	if pool.waitQueueCh.Len() == 0 {
		return nil
	}
	if pool.nextSelector != nil {
		next := pool.nextSelector(func(key ObjectKey) bool {
			//需要确保在container中不存在
			if pool.container.Has(key.Text()) { //去除已存在
				return true
			}
			//需要确保在waitQueue中存在
			if !pool.waitQueue.Has(key.Text()) {
				return true
			}
			return false
		})
		if next != nil {
			return next
		}
	}
	for {
		select {
		case task := <-pool.waitQueueCh.DequeueChan():
			key := task.(ObjectKey)
			if pool.waitQueue.Has(key.Text()) {
				return key
			}
			continue
		default:
			return nil
		}
	}
}

type StopAsk interface {
	CanStop() bool
}
