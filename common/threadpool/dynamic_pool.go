package threadpool

import (
	"container/list"
	"sync"
	"time"
)

//const globalMaxParallel = 10000
//
////全局线层池,处理异步操作
//var globalThreadPool *dynamicPool = NewDynamicPool(globalMaxParallel)
//
//func AsyncWork(f task, params []interface{}) {
//	globalThreadPool.PushTask(f, params)
//}

type task func([]interface{})

type sendFun struct {
	f      task
	params []interface{}
}

type worker struct {
	pool *dynamicPool
	task chan *sendFun
}

func (w *worker) run() {
	go func() {
		for v := range w.task {
			v.f(v.params)
			w.pool.putTask(w)
		}
	}()
}

func (w *worker) pending() {
	pending := list.New()
	var mux sync.Mutex
	go func() {
		for {
			select {
			case fun := <-w.task:
				mux.Lock()
				pending.PushBack(fun)
				mux.Unlock()
			}
		}
	}()
	go func() {
		for {
			mux.Lock()
			front := pending.Front()
			if front == nil {
				mux.Unlock()
				time.Sleep(time.Millisecond * 10)
				continue
			}
			fun := pending.Remove(front).(*sendFun)
			mux.Unlock()
			w.pool.PushTask(fun.f, fun.params)
		}
	}()
}

type dynamicPool struct {
	capacity uint32 // 最大开启的任务数
	running  uint32 // 当前运行的任务数
	//freeSign *sync.Cond // 任务处理完成发送信号
	workers  []*worker // 可复用的任务
	lock     sync.Mutex
	once     sync.Once
	waitTask *worker
}

func NewDynamicPool(maxSize uint32) *dynamicPool {
	pool := &dynamicPool{
		capacity: maxSize,
	}
	//pool.freeSign = sync.NewCond(&pool.lock)
	pool.waitTask = &worker{
		pool: pool,
		task: make(chan *sendFun),
	}
	pool.waitTask.pending()
	return pool
}

func (p *dynamicPool) getWork() *worker {
	var w *worker
	p.lock.Lock()
	defer p.lock.Unlock()
	//for {
	if len(p.workers) > 0 {
		w = p.workers[0]
		p.workers = p.workers[1:]
	} else if p.capacity > p.running {
		p.running++
		w = &worker{
			pool: p,
			task: make(chan *sendFun),
		}
		w.run()
	}
	if w != nil {
		return w
	}
	return p.waitTask
	//p.freeSign.Wait()
	//}
}

func (p *dynamicPool) PushTask(f task, params []interface{}) {
	p.getWork().task <- &sendFun{f, params}
}

func (p *dynamicPool) putTask(work *worker) {
	p.lock.Lock()
	p.workers = append(p.workers, work)
	p.lock.Unlock()
	//p.freeSign.Signal()
}
