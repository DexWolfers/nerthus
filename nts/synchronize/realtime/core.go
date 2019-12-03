package realtime

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/nts/synchronize/message"
	"gitee.com/nerthus/nerthus/nts/synchronize/realtime/merkle"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/rlp"
)

const (
	TimeoutDiffCmp     = time.Second * 10 // 对比超时时间
	IntervalStatusSend = time.Minute * 1  // 定时发送状态间隔
)

func NewRealTimeSync(backend Backend) *RealTimeSync {
	return &RealTimeSync{
		backend: backend,
		cmping:  false,
		started: false,
	}
}

type RealTimeSync struct {
	tree    *merkle.Tree
	mux     sync.Mutex
	backend Backend
	status  *statusSwitch // 状态交换
	cmptor  *treeCmptor   // 差异对比
	cmping  bool          //是否正在对比中
	quit    chan struct{}
	started bool
}

func (t *RealTimeSync) Start() {
	begin := time.Now()

	t.mux.Lock()
	defer t.mux.Unlock()
	// 获取已知链地址
	chains := t.backend.GetChainsStatus()
	// 组成树
	t.tree = merkle.NewTreeAppend(chains, func(content merkle.Content) int {
		addr := content.(ChainStatus).Addr
		// 系统链永远在第一位
		if addr == params.SCAccount {
			return 0
		}
		index, ok := t.backend.ChainStatus(addr)
		if ok {
			return int(index)
		} else {
			log.Debug("can't find chain index", "chain", addr)
			return -1
		}
	}) // 注入查找方法，数据库查出链所在下标地址
	t.started = true
	t.quit = make(chan struct{})
	log.Debug("realtime sync start", "chain len", len(chains), "sc number", chains[0].(ChainStatus).Number)

	t.status = newSwitcher(t.backend, t.CurrentStatus, t.startCmpWithPeer)
	t.status.start()

	log.Info("started real time sync", "startup.time", time.Now().Sub(begin))
}
func (t *RealTimeSync) Stop() {
	t.mux.Lock()
	defer t.mux.Unlock()
	if !t.started {
		return
	}
	t.started = false
	if !t.cmping {
		t.status.stop()
	} else if t.cmptor != nil {
		t.cmptor.stop()
	}
	t.cmptor = nil
	t.status = nil
	t.tree.Clear()
	t.tree = nil
	close(t.quit)
	log.Info("stop real time sync")
}

// 消息handler
func (t *RealTimeSync) Handle(msg *message.Message) error {
	t.mux.Lock()
	if !t.started {
		t.mux.Unlock()
		return nil
	}
	t.mux.Unlock()
	ev := new(RealtimeEvent)
	if err := rlp.DecodeBytes(msg.Playload, ev); err != nil {
		return err
	}
	ev.From = msg.Src
	return t.handleEvent(ev)
}

// 链有新的事件，激活链时number==0
func (t *RealTimeSync) NewChainEvent(ev ChainStatus) {
	t.mux.Lock()
	defer t.mux.Unlock()
	if !t.started {
		return
	}
	if ev.Number == 0 {
		// 新的链地址
		t.tree.Append(ev)
	} else {
		if err := t.tree.Update(ev); err != nil {
			log.Debug("tree update failed,restart sync server", "err", err)
			go func() {
				t.Stop()
				t.Start()
			}()
		}
	}
	// 如果正在差异对比，检查
	if t.cmping {
		// 新单元补入后，是否已经一致
		log.Debug("new chain event when sync tree cmping", "ev_number", ev.Number, "chain", ev.Addr)
		if bytes.Equal(t.tree.Root().Hash(), t.cmptor.remoteRoot()) {
			t.stopCmp(false)
		}
	} else {
		// 重置发送状态tick时间
		//t.status.resetRequester()
	}
}

// 当前节点状态
func (t *RealTimeSync) CurrentStatus() NodeStatus {
	t.mux.Lock()
	defer t.mux.Unlock()
	return NodeStatus{
		t.tree.Root().Hash(),
		uint64(t.tree.LeafCount()),
	}
}

// handler
func (t *RealTimeSync) handleEvent(ev *RealtimeEvent) error {
	switch ev.Code {
	default:
		return fmt.Errorf("invalid message code %d", ev.Code)
	case RealtimeStatus, RealtimeRequest, RealtimeReponse:
		if t.isCmping() {
			return nil
		}
		t.status.handleResponse(ev)
		return nil
	case RealtimePushChains, RealtimeCmpHash, RealtimeCmpEnd:
		if !t.isCmping() {
			return nil
		}
		return t.cmptor.handleResponse(ev)
	}
}

// 开始与某个节点进行差异对比
func (t *RealTimeSync) startCmpWithPeer(ev *RealtimeEvent) {
	// 进入差异对比
	wait := make(chan struct{})
	t.activateCmp(ev, wait)
	go func() {
		// 计算超时时间 链数每一百万加超时时间加一秒 *最大不能超过状态消息间隔
		timeout := TimeoutDiffCmp
		if c := t.CurrentStatus().Chains / 10e5; c > 0 {
			timeout += time.Duration(c) * time.Second
		}
		if timeout > IntervalStatusSend {
			timeout = IntervalStatusSend
		}
		select {
		case <-wait:
			// 差异对比结束
			t.stopCmp(true)
		case <-t.quit:
			return
		case <-time.After(timeout):
			// 超时，强行终止差异对比  TODO 需要特殊手段处理
			log.Debug("sync cmp timeout", "t", timeout)
			t.stopCmp(true)

		}
	}()
}
func (t *RealTimeSync) activateCmp(ev *RealtimeEvent, waitCh chan struct{}) {
	t.mux.Lock()
	defer t.mux.Unlock()
	if t.cmping {
		return
	}
	t.status.stop() // 停止状态交换
	t.cmptor = newTreeCmptor(t.tree.Copy(), t.backend)
	t.cmptor.start(ev, waitCh)
	t.cmping = true
}
func (t *RealTimeSync) stopCmp(lock bool) {
	if lock {
		t.mux.Lock()
		defer t.mux.Unlock()
	}
	if t.cmptor != nil {
		t.cmptor.stop()
	}
	t.cmping = false
	t.cmptor = nil
	t.status.start()
}
func (t *RealTimeSync) isCmping() bool {
	t.mux.Lock()
	defer t.mux.Unlock()
	return t.cmping
}
