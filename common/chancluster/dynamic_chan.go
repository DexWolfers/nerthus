package chancluster

import "context"

func NewDychan(max int) *Dychan {
	if max == 0 {
		max = 1024
	}
	d := &Dychan{
		make(chan interface{}, max),
		int64(max),
	}
	return d
}

type Dychan struct {
	container  chan interface{}
	upperLimit int64
}

func (this *Dychan) In(m interface{}) {
	this.container <- m
}
func (this *Dychan) InWithContext(ctx context.Context, m interface{}) {
	select {
	case <-ctx.Done():
		break
	case this.container <- m:
	}
}
func (this *Dychan) Out() <-chan interface{} {
	return this.container
}
func (this *Dychan) Len() int64 {
	return int64(len(this.container))
}
func (this *Dychan) Close() {
	close(this.container)
}
func (this *Dychan) InChan() chan<- interface{} {
	return this.container
}

//
//// 构建一个带无堵塞的chan
//// clen表示channel的长度
//func NewDychan(max int) *Dychan {
//	d := &Dychan{
//		lfreequeue.NewQueue(),
//		int64(max),
//		make(chan struct{}),
//	}
//	return d
//}
//
//type Dychan struct {
//	container  *lfreequeue.Queue
//	upperLimit int64
//	waitLock   chan struct{}
//}
//
//func (this *Dychan) In(m interface{}) {
//	if this.upperLimit > 0 {
//		// 最大值限制
//	upper:
//		if this.container.Len() >= this.upperLimit {
//			this.waitLock <- struct{}{}
//			goto upper
//		}
//	}
//	this.container.Enqueue(m)
//}
//func (this *Dychan) Out() <-chan interface{} {
//	// 出队，释放一次排队
//	select {
//	case <-this.waitLock:
//	default:
//
//	}
//	return this.container.DequeueChan()
//}
//func (this *Dychan) Len() int64 {
//	return this.container.Len()
//}
//func (this *Dychan) Close() {
//	this.container.Clear()
//	close(this.waitLock)
//}
