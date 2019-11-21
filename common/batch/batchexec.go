package batch

import (
	"sync"
	"sync/atomic"
)

type execFunc func(gp interface{}, args ...interface{})

type ChannelMsg struct {
	group interface{}
	args  []interface{}
	f     execFunc
}
type ExecChannel struct {
	queuedCount int64    //队列中数量
	queuedGroup sync.Map // key=group ,value =count
	ch          chan ChannelMsg
}

func (e *ExecChannel) Add(group interface{}, n int64) {
	atomic.AddInt64(&e.queuedCount, n)

	data, ok := e.queuedGroup.Load(group)
	if !ok {
		data = new(int64)
	}
	count := atomic.AddInt64(data.(*int64), n)
	e.queuedGroup.Store(group, &count)
}
func (e *ExecChannel) Done(group interface{}) {
	e.Add(group, -1)
	data, ok := e.queuedGroup.Load(group)
	if ok {
		if v := data.(*int64); *v <= 0 {
			e.queuedGroup.Delete(group)
		}
	}
}

type BatchExec struct {
	max        int
	chs        []*ExecChannel
	runningLoc sync.Map // key= id, value = work channel index
	quit       chan struct{}
	status     int32
	mut        sync.Mutex

	testMode bool // 测试使用
}

func NewBatchExec(max int) *BatchExec {
	b := &BatchExec{
		chs:  make([]*ExecChannel, max),
		quit: make(chan struct{}),
	}
	b.Start()
	return b
}

func (b *BatchExec) Start() {
	b.mut.Lock()
	defer b.mut.Unlock()
	if !atomic.CompareAndSwapInt32(&b.status, 0, 1) {
		return
	}

	for i := 0; i < len(b.chs); i++ {
		b.chs[i] = &ExecChannel{ch: make(chan ChannelMsg, 100)}
		go func(i int) {
			ch := b.chs[i]
			for {
				select {
				case msg, ok := <-ch.ch:
					if !ok {
						return
					}
					if b.testMode {
						msg.f(msg.group, append([]interface{}{i}, msg.args...)...) //测试模式记录运行位置
					} else {
						msg.f(msg.group, msg.args...)
					}
					// 减1
					ch.Done(msg.group)

				case <-b.quit:
					return
				}
			}
		}(i)
	}
}
func (b *BatchExec) Stop() {
	b.mut.Lock()
	defer b.mut.Unlock()
	if !atomic.CompareAndSwapInt32(&b.status, 1, 0) {
		return
	}
	close(b.quit)
	for _, c := range b.chs {
		if c.ch != nil {
			for {
				if len(c.ch) == 0 {
					break
				}
				<-c.ch //clear
			}
		}
	}
}

func (b *BatchExec) Push(f execFunc, group interface{}, args ...interface{}) {
	select {
	case <-b.quit:
		return
	default:
		b.mut.Lock()
		ch := b.nextCh(group)
		b.mut.Unlock()
		select {
		case <-b.quit:
			return
		default:
			ch.Add(group, 1)
			ch.ch <- ChannelMsg{group, args, f}
		}
	}
}

func (b *BatchExec) nextCh(group interface{}) *ExecChannel {
	var next *ExecChannel
	for _, c := range b.chs {
		// 如果在单个ch中存在此分组，则直接使用此分组
		if count, ok := c.queuedGroup.Load(group); ok && *(count.(*int64)) > 0 {
			return c
		}
		if next == nil || c.queuedCount < next.queuedCount {
			next = c
		}
	}
	return next
}
