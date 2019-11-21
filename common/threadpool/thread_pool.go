package threadpool

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"gitee.com/nerthus/nerthus/common/chancluster"
)

var ErrThreadPoolDone = errors.New("thread pool is done")

type CallBackFn = func(ctx context.Context, args []interface{})

const (
	threadPoolState uint8 = iota
	WaitStartThreadPoolState
	StartedThreadPoolState
	WaitStopThreadPoolState
	StoppedThreadPoolState
)

type ThreadPool interface {
	Start() <-chan error
	PushTask(callBackFn CallBackFn, args []interface{}) (err error)
	Stop()
	Len() int64
}

type queueItem struct {
	callBackFn CallBackFn
	args       []interface{}
}

// 线程池，共享给定的goroutine
type BaseThreadPool struct {
	//queue      chan queueItem
	queue      *chancluster.Dychan
	startOnce  *sync.Once
	startCh    chan error
	stopOnce   *sync.Once
	ctx        context.Context
	cancelFn   context.CancelFunc
	gWait      *sync.WaitGroup
	gSize      int
	state      uint8
	callBackFn CallBackFn
}

func NewBaseTreadPoll(qSize, gSize int) ThreadPool {
	ctx, cancelFn := context.WithCancel(context.TODO())
	return &BaseThreadPool{
		queue:     chancluster.NewDychan(qSize),
		gSize:     gSize,
		startOnce: new(sync.Once),
		stopOnce:  new(sync.Once),
		gWait:     new(sync.WaitGroup),
		ctx:       ctx,
		cancelFn:  cancelFn,
		startCh:   make(chan error, 1),
		state:     WaitStartThreadPoolState}
}

func (tp *BaseThreadPool) Start() <-chan error {
	tp.startOnce.Do(func() {
		for i := 0; i < tp.gSize; i++ {
			go tp.work(fmt.Sprintf("%v", i))
		}
		tp.state = StartedThreadPoolState
		tp.startCh <- nil
	})
	return tp.startCh
}

func (tp *BaseThreadPool) Stop() {
	tp.stopOnce.Do(func() {
		tp.state = WaitStopThreadPoolState
		tp.cancelFn()
		close(tp.startCh)
		tp.gWait.Wait()
		// do residual work
		for {
			select {
			case t := <-tp.queue.Out():
				task := t.(queueItem)
				task.callBackFn(tp.ctx, task.args)
			default:
				tp.state = StoppedThreadPoolState
				return
			}
		}
	})
}

// it is not thread safe function, so avoid panic that recover it
func (tp *BaseThreadPool) PushTask(callBackFn CallBackFn, args []interface{}) (err error) {
	defer func() {
		if errRecover := recover(); errRecover != nil {
			err = ErrThreadPoolDone
		}
	}()

	if tp.state != StartedThreadPoolState {
		return fmt.Errorf("threapool at %v state", tp.state)
	}
	tp.queue.In(queueItem{callBackFn: callBackFn, args: args})
	//select {
	//case tp.queue.InChan() <- queueItem{callBackFn: callBackFn, args: args}:
	//	err = nil
	//case <-tp.ctx.Done():
	//	err = tp.ctx.Err()
	//	//default:
	//	//	// 满了就新开goroutine
	//	//	go tp.work("")
	//}
	return
}
func (tp *BaseThreadPool) Len() int64 {
	return tp.queue.Len()
}
func (tp *BaseThreadPool) work(tid string) {
	tp.gWait.Add(1)
	defer func() {
		tp.gWait.Done()
	}()
	for {
		select {
		case t := <-tp.queue.Out():

			task := t.(queueItem)
			task.callBackFn(tp.ctx, task.args)
		case <-tp.ctx.Done():
			return
			//default:
			//	// 空了就释放
			//	if tid == "" {
			//		return
			//	} else {
			//		t := <-tp.queue.Out()
			//		tp.queue.In(t)
			//	}
		}
	}
}
