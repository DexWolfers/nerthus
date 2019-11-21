package backend

import (
	"context"
	"expvar"
	"fmt"
	"sync"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/chancluster"
	"gitee.com/nerthus/nerthus/common/objectpool"
	"gitee.com/nerthus/nerthus/common/tracetime"
	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/consensus/bft/core"
	"gitee.com/nerthus/nerthus/consensus/protocol"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"
)

const (
	autoFeeChainDelay = time.Minute * 10 //链不工作自动释放间隔
	chainChanLen      = 128              //单链消息channel缓冲 * 等于0无上限
)

func newEngineSchedule(backend bft.Backend, config *bft.Config, packUnit packUnitFunc, existTx func(mc common.Address) bool, maxParallelChain int) *engineSchedule {
	eng := &engineSchedule{
		packUnit:            packUnit,
		existValidTx:        existTx,
		clt:                 chancluster.NewChanCluster(),
		logger:              log.New("module", "bft schedule"),
		maxParallelChainRun: maxParallelChain,
		started:             false,
	}
	timer := newRoundTimer(func(rv *core.RoundView) {
		eng.handle(rv.Chain, bft.NewChangeRound, rv)
	})
	objs := objectpool.NewObjPool(false, autoFeeChainDelay, func(key objectpool.ObjectKey) objectpool.Object {
		return core.New(backend, config, key.(common.Address), eng.prePackUnit, eng.existValidTx, eng.setChainStatus, eng.timer)
	})
	// 设置上限
	objs.SetUpperLimit(maxParallelChain)
	objs.SetWaitUpper(int(params.ConfigParamsUCMinVotings * 3))
	eng.timer = timer
	eng.doing = objs
	expvar.Publish(fmt.Sprintf("bft_chains_%d", time.Now().Unix()), &activeChainsVar{objs: eng.doing, clt: eng.clt})
	return eng
}

type packUnitFunc func(ctx context.Context, chain common.Address) (*bft.MiningResult, error)
type engineSchedule struct {
	max                 int
	doing               *objectpool.ObjectPool
	timer               *timerSchedule
	packUnit            packUnitFunc
	existValidTx        func(mc common.Address) bool
	clt                 *chancluster.ChanCluster
	localMsgChan        *chancluster.Dychan
	logger              log.Logger
	maxParallelChainRun int
	started             bool
	validators          validatorCache
	quit                chan struct{}
}

func (this *engineSchedule) start() {
	if this.started {
		return
	}
	this.started = true
	this.quit = make(chan struct{})
	//设置上限
	this.clt.SetUpperLimit(int64(this.maxParallelChainRun))
	this.clt.Start()
	//this.doing.SetUpperLimit(maxParallelChainRun)
	this.doing.Start()
	go this.timer.loop()
	go this.loopLocalMsg()
	// object gc 定时扫无效的对象并释放
	go this.loopObjectGc()

	this.logger.Debug("start bft engine schedule")
}
func (this *engineSchedule) stop() {
	if !this.started {
		return
	}
	close(this.quit)
	this.started = false
	this.timer.stop()
	this.clt.Stop()
	this.doing.Stop()
	this.localMsgChan.Close()
	this.logger.Debug("stop bft engine schedule")
}
func (this *engineSchedule) getEngine(mc common.Address) core.Engine {
	if this == nil || this.doing == nil {
		return nil
	}
	if e := this.doing.Get(mc); e != nil {
		return e.(core.Engine)
	}
	return nil
}

// 新交易 激活这条链
func (this *engineSchedule) activate(mc common.Address, activate bool) {
	if !this.started {
		return
	}
	// 系统链不用激活
	if mc == params.SCAccount {
		return
	}
	if eng := this.getEngine(mc); eng != nil {
		if !eng.IsActive() {
			this.handle(mc, bft.NewTx)
		}
	} else {
		this.handle(mc, bft.NewTx)
	}
}
func (this *engineSchedule) setChainStatus(mc common.Address, status bool) {
	// 是激活 取消释放
	if status {
		this.tryCancelFreeChain(mc)
	} else {
		this.freeChain(mc, false)
	}
}
func (this *engineSchedule) handleMsg(ev protocol.MessageEvent) {
	if ev.FromMe {
		this.handleLocal(ev.Chain, bft.NewMessage, ev.Message)
		return
	}
	this.handle(ev.Chain, bft.NewMessage, ev.Message)
}
func (this *engineSchedule) newHeadNumber(chain common.Address, number uint64, uhash common.Hash) {
	tr := tracetime.New("chain", chain, "number", number, "uhash", uhash).SetMin(0)
	defer tr.Stop()
	if !this.started {
		return
	}
	this.logger.Debug("new number", "chain", chain, "number", number, "uhash", uhash)
	tr.Tag()
	// 尝试插队
	if this.clt.Len(chain) > 0 {
		engine := this.getEngine(chain)
		if engine == nil || !engine.PriorHandle([]interface{}{bft.NewNumber, number, uhash}) {
			this.handle(chain, bft.NewNumber, number, uhash)
			tr.Tag()
		}
	} else {
		this.handle(chain, bft.NewNumber, number, uhash)
	}

}

// 将打包单元送入共识
func (this *engineSchedule) seal(mc common.Address, request *bft.Request) error {
	// 尝试插队
	if this.clt.Len(mc) > 0 {
		engine := this.getEngine(mc)
		if engine != nil && engine.PriorHandle([]interface{}{bft.NewProposal, request}) {
			return nil
		}
	}
	this.handleLocal(mc, bft.NewProposal, request)
	return nil
}

// 打包单元
func (this *engineSchedule) prePackUnit(mc common.Address) error {
	result, err := this.packUnit(context.Background(), mc)

	if err != nil {
		return err
	}
	if err = this.seal(mc, &bft.Request{result}); err != nil {
		return err
	}
	return nil
}

// 取消正在等待的释放任务
func (this *engineSchedule) tryCancelFreeChain(mc common.Address) {
	if mc == params.SCAccount {
		return
	}
	this.doing.CancelFree(mc)
}

// 释放链内存
func (this *engineSchedule) freeChain(mc common.Address, immediately bool) {
	if mc == params.SCAccount {
		return
	}
	this.doing.Free(mc, func(key string) {
		this.logger.Debug("no work any more,free chain", "chain", key)
	}, immediately)
}

// 共识handler
func (this *engineSchedule) handle(mc common.Address, args ...interface{}) {
	//tr := tracetime.New("chain", mc, "clen", this.clt.Len(mc), "more", args)
	//defer tr.Stop()
	if !this.started {
		return
	}
	this.clt.HandleOrRegister(mc, 1, chainChanLen, func(i []interface{}) error {
		this.doing.Handle(i[0].(common.Address), i[1].([]interface{})...)
		return nil
	}, nil, mc, args)
}

// 本地消息处理
func (this *engineSchedule) handleLocal(chain common.Address, args ...interface{}) {
	if !this.started {
		return
	}
	select {
	case this.localMsgChan.InChan() <- []interface{}{chain, args}:
	default:
		go this.localMsgChan.In([]interface{}{chain, args})
	}
}

// 本地消息handle
func (this *engineSchedule) loopLocalMsg() {
	this.localMsgChan = chancluster.NewDychan(this.maxParallelChainRun * 2)
	for {
		select {
		case ev, ok := <-this.localMsgChan.Out():
			if !ok {
				return
			}
			mc := ev.([]interface{})[0].(common.Address)
			this.clt.HandleOrRegister(mc, 1, chainChanLen, func(i []interface{}) error {
				this.doing.Handle(i[0].(common.Address), i[1].([]interface{})...)
				return nil
			}, nil, ev.([]interface{})...)
		}
	}
}

// 定时清理无效共识对象
func (this *engineSchedule) loopObjectGc() {
	for {
		time.Sleep(time.Minute)
		select {
		case <-this.quit:
			return
		default:

		}
		invalidObjs := make([]objectpool.ObjectKey, 0)
		this.doing.Range(func(key string, v interface{}) bool {
			eng := v.(core.Engine)
			if !eng.IsActive() {
				invalidObjs = append(invalidObjs, eng.Key())
			}
			return true
		})
		if len(invalidObjs) == 0 {
			continue
		}
		// 检查goroutine队列
		for i := range invalidObjs {
			if this.clt.Len(invalidObjs[i]) == 0 {
				this.logger.Debug("discover free chain", "chain", invalidObjs[i])
				this.freeChain(invalidObjs[i].(common.Address), false)
			}
		}
	}
}

func newRoundTimer(do func(rv *core.RoundView)) *timerSchedule {
	return &timerSchedule{
		doFunc: do,
	}
}

// timeout调度器
// 管理所有链引擎的超时时钟
type timerSchedule struct {
	tasks  sync.Map
	doFunc func(rv *core.RoundView)
	quit   chan struct{}
}

func (t *timerSchedule) SetTimeout(view *core.RoundView) {
	t.tasks.Store(view.Chain, view)
}
func (t *timerSchedule) StopTimer(mc common.Address) {
	t.tasks.Delete(mc)
}
func (t *timerSchedule) stop() {
	close(t.quit)
}
func (t *timerSchedule) loop() {
	t.quit = make(chan struct{})
	for {
		select {
		case <-t.quit:
			return
		default:

		}
		tr := tracetime.New().SetMin(0)
		var needDo []*core.RoundView
		now := time.Now()
		t.tasks.Range(func(key, value interface{}) bool {
			rv := value.(*core.RoundView)
			if now.After(rv.DeadLine) {
				needDo = append(needDo, rv)
			}
			return true
		})
		tr.AddArgs("more", needDo)
		for i := 0; i < len(needDo); i++ {
			t.doFunc(needDo[i])
			t.tasks.Delete(needDo[i].Chain)
		}
		tr.Stop()
		time.Sleep(500 * time.Millisecond)
	}
}

type validatorCache struct {
	height    uint64
	validator bft.Validators
	sync.RWMutex
}
