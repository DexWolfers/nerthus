package chancluster

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
)

var (
	errExistChann = errors.New("exist this identify")
	ErrPanic      = errors.New("panic happened")
)

func NewChanCluster() *ChanCluster {
	return &ChanCluster{
		currentParallel: new(int64),
		waitQueue:       make(chan struct{}),
	}
}

// 并行处理器,按给定标识区分，每个并行自拥有一个channel，不相互影响
type ChanCluster struct {
	channels sync.Map
	quit     chan struct{}
	//createMux       sync.Mutex
	testOnNew       func(identify interface{})
	maxParallel     int64
	currentParallel *int64
	waitQueue       chan struct{}
}

func (this *ChanCluster) ParallelCount() int {
	return int(atomic.LoadInt64(this.currentParallel))
}

// 静态注册 Start之前调用
func (this *ChanCluster) Register(identify interface{}, parallel int, sizeLimit int, deal func([]interface{}) error, errDeal func(error, []interface{})) error {
	_, err := this.registerOrLoad(identify, parallel, sizeLimit, deal, errDeal)
	return err
}

// 判断是否已注册
func (this *ChanCluster) HadRegister(identify interface{}) bool {
	_, ok := this.channels.Load(identify)
	return ok
}

// 热注册，可在Start之后掉用
func (this *ChanCluster) HotRegister(identify interface{}, parallel int, sizeLimit int, deal func([]interface{}) error, errDeal func(error, []interface{})) error {
	c, err := this.registerOrLoad(identify, parallel, sizeLimit, deal, errDeal)
	if err != nil && err != errExistChann {
		return err
	} else {
		c.start(this.quit)
		return nil
	}
}

// 热注册并处理，如果没有注册则注册，已注册则自己处理
func (this *ChanCluster) HandleOrRegister(identify interface{}, parallel int, sizeLimit int, deal func([]interface{}) error, errDeal func(error, []interface{}), args ...interface{}) error {
	c, err := this.registerOrLoad(identify, parallel, sizeLimit, deal, errDeal)
	if err == errExistChann {
		c.put(args)
		return nil
	} else if err == nil {
		// 动态注册 需要设置释放
		c.setFree(10 * time.Second)
		c.start(this.quit)
		c.put(args)
	}
	return err
}
func (this *ChanCluster) Unregister(identify interface{}) {
	c, ok := this.channels.Load(identify)
	if !ok {
		return
	}
	c.(*channelController).stop()
	this.channels.Delete(identify)
}

// 设置并行上限
func (this *ChanCluster) SetUpperLimit(upper int64) {
	this.maxParallel = upper
}

func (this *ChanCluster) Start() {
	this.quit = make(chan struct{})
	this.channels.Range(func(key, value interface{}) bool {
		v, ok := value.(*channelController)
		if ok {
			v.start(this.quit)
		}
		return true
	})
}

// 处理入口
func (this *ChanCluster) Handle(identify interface{}, value ...interface{}) error {
	return this.HandleWithContext(context.Background(), identify, value...)
}

// 处理入口
func (this *ChanCluster) HandleWithContext(ctx context.Context, identify interface{}, value ...interface{}) error {
	v, ok := this.channels.Load(identify)
	if !ok {
		return errors.New("not found this channel identity")
	}
	controller := v.(*channelController)
	controller.putWithContext(ctx, value)
	return nil
}
func (this *ChanCluster) Stop() {
	if this.quit == nil {
		return
	}
	close(this.quit)
	// 等待全部释放
	for {
		if atomic.LoadInt64(this.currentParallel) <= 0 {
			break
		}
		time.Sleep(time.Millisecond * 100)
	}
	//this.channels.Range(func(key, value interface{}) bool {
	//	v, ok := value.(*channelController)
	//	if ok {
	//		v.stop()
	//	}
	//	return true
	//})
}

// channel 长度
func (this *ChanCluster) Len(identify interface{}) int64 {
	ch, ok := this.channels.Load(identify)
	if !ok {
		return 0
	} else {
		return ch.(*channelController).ch.Len()
	}
}

// 查找注册实例，不存在则先注册
func (this *ChanCluster) registerOrLoad(identify interface{}, parallel int, sizeLimit int, deal func([]interface{}) error, errDeal func(error, []interface{})) (*channelController, error) {
allover:
	if v, ok := this.channels.Load(identify); ok {
		return v.(*channelController), errExistChann
	}

	//不允许并发创建
	//this.createMux.Lock()
	//defer this.createMux.Unlock()
	if v, ok := this.channels.Load(identify); ok {
		return v.(*channelController), errExistChann
	}

	if identify == nil {
		return nil, errors.New("invalid identity")
	}
	if parallel <= 0 {
		return nil, errors.New("invalid parallel value")
	}
	if deal == nil {
		return nil, errors.New("invalid deal func")
	}
	// 是否达到并行处理上限
	if this.maxParallel > 0 {
		cur := atomic.LoadInt64(this.currentParallel)
		if cur+int64(parallel) > this.maxParallel {
			// 达到上限 等待
			this.waitQueue <- struct{}{}
			goto allover
		}
	}
	callback := func(c *channelController) {
		atomic.AddInt64(this.currentParallel, -int64(c.parallel))
		this.channels.Delete(c.name)
		// 取出排队的任务
		select {
		case <-this.waitQueue:

		default:

		}
		//fmt.Printf("---------%v channel controller stop-------\n", c.name)

	}
	ch := NewDychan(sizeLimit)
	c := &channelController{
		identify,
		ch,
		deal,
		errDeal,
		parallel,
		false,
		0,
		nil,
		new(int64),
		callback,
		this.waitQueue,
	}
	// 处理并发
	_, ok := this.channels.LoadOrStore(identify, c)
	if ok {
		// 清理资源
		ch.Close()
		c = nil
		goto allover
	}
	atomic.AddInt64(this.currentParallel, int64(parallel))
	if this.testOnNew != nil {
		this.testOnNew(identify)
	}
	return c, nil
}

// 单个处理实例，处理任务执行、goroutine启动和停止
type channelController struct {
	name         interface{}
	ch           *Dychan
	deal         func(ev []interface{}) error
	onerr        func(err error, args []interface{})
	parallel     int
	autoFree     bool //是否自动释放
	freeDelay    time.Duration
	quit         <-chan struct{}
	sequence     *int64
	quitCallback func(*channelController)
	waitQueue    chan struct{}
}

// 设置自动释放，指定时间间隔没有任务则自动释放
func (this *channelController) setFree(delay time.Duration) {
	this.autoFree = true
	this.freeDelay = delay
}
func (this *channelController) start(q <-chan struct{}) {
	if this.deal == nil {
		return
	}
	if this.parallel <= 0 {
		return
	}
	this.quit = q
	for i := 0; i < this.parallel; i++ {
		go func(t *channelController) {
			atomic.AddInt64(t.sequence, 1) //计数
			defer func() {
				//if t.onerr != nil {
				//	arg := recover()
				//	if arg != nil {
				//		t.onerr(ErrPanic, []interface{}{arg})
				//	}
				//}
				if rest := atomic.AddInt64(t.sequence, -1); rest <= 0 {
					// 全部并行结束
					t.stop()
				}
			}()
			if !t.autoFree {

			loop:
				for {
					select {
					case ev, ok := <-t.ch.Out():
						if ok {
							e := ev.([]interface{})
							err := t.deal(e)
							if err != nil && t.onerr != nil {
								t.onerr(err, e)
							}
						} else {
							break loop
						}
					case <-t.quit:
						break loop
					}

				}
			} else {
				dealFunc := func(ev interface{}) {
					e := ev.([]interface{})
					err := t.deal(e)
					if err != nil && t.onerr != nil {
						t.onerr(err, e)
					}
				}
			loopFree: // 需要自动释放节约性能
				for {
					select {
					case ev, ok := <-t.ch.Out():
						if ok {
							dealFunc(ev)
						} else {
							break loopFree
						}
					case <-t.quit:
						break loopFree
					default:
						// 后面有排队任务，立即释放
						if len(t.waitQueue) > 0 {
							break loopFree
						}
						select {
						case ev, ok := <-t.ch.Out(): // 期间有工作进来 立即处理
							if !ok {
								break loopFree
							}
							dealFunc(ev)
						case <-t.waitQueue: // 有排队，立即释放
							break loopFree
						case <-time.After(t.freeDelay): // 时间到，释放
							break loopFree
						case <-t.quit:
							break loopFree
						}
					}

				}
			}
		}(this)
	}
}
func (this *channelController) stop() {
	this.quitCallback(this)
	// 清理未完成任务
lp:
	for {
		select {
		case ev, ok := <-this.ch.Out():
			if !ok {
				return
			}
			e := ev.([]interface{})
			err := this.deal(e)
			if err != nil && this.onerr != nil {
				this.onerr(err, e)
			}
		default:
			break lp
		}
	}
	this.ch.Close()
}
func (this *channelController) put(m []interface{}) {
	this.putWithContext(context.Background(), m)
}

func (this *channelController) putWithContext(ctx context.Context, m []interface{}) {
	defer func() {
		recover()
	}()
	this.ch.InWithContext(ctx, m)
}
