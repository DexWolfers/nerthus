package parallel

import (
	"sync/atomic"
)

func NewParalleler() *Paralleler {
	return &Paralleler{
		err:      make(chan error, 1),
		state:    new(int32),
		sequence: new(int32),
		AsyncFunc: func(f func()) {
			go f()
		},
	}
}

// 并行执行多个任务
type Paralleler struct {
	err       chan error
	errBuffer error
	state     *int32 // 0 未启动 1 等待 2 已停止
	sequence  *int32
	AsyncFunc func(f func())
}

// 异步执行
func (this *Paralleler) Async(f func(args ...interface{}), args ...interface{}) {
	// 如果已取消
	if len(this.err) > 0 {
		return
	}
	atomic.AddInt32(this.sequence, 1)
	ff := func() {
		defer func() {
			atomic.AddInt32(this.sequence, -1)
			if atomic.LoadInt32(this.sequence) <= 0 && atomic.LoadInt32(this.state) == 1 {

				select {
				case this.err <- nil:
				default:

				}
			}
		}()
		f(args...)
	}
	this.AsyncFunc(ff)
}

// 终止
func (this *Paralleler) Abort(err error) {
	if this.err == nil {
		return
	}
	select {
	case this.err <- err:
	default:

	}
	atomic.StoreInt32(this.state, 2)
}

// 等待执行完毕
func (this *Paralleler) Wait() error {
	state := atomic.LoadInt32(this.state)
	if state == 1 {
		return nil
	}
	defer func() {
		atomic.StoreInt32(this.state, 2)
		close(this.err)
		this.err = nil
	}()
	switch state {
	case 0:
		atomic.AddInt32(this.state, 1)
		// allready done
		if atomic.LoadInt32(this.sequence) <= 0 {
			return nil
		}
		err, ok := <-this.err
		if !ok {
			return nil
		}
		return err
	case 2: //被终止
		if len(this.err) > 0 {
			return <-this.err
		}
	}
	return nil
}
func (this *Paralleler) Canceled() bool {
	return atomic.LoadInt32(this.state) == 2
}
