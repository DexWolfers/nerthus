package bysolt

import (
	"fmt"
	"sync"
	"time"

	"gitee.com/nerthus/nerthus/core/types"

	"gitee.com/nerthus/nerthus/core/rawdb"
	"gitee.com/nerthus/nerthus/params"

	"gitee.com/nerthus/nerthus/ntsdb"

	"gitee.com/nerthus/nerthus/common/sync/cmap"

	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/nts/synchronize/message"
	"gitee.com/nerthus/nerthus/rlp"
)

const (
	TimeoutDiffCmp      = time.Second * 30 // 对比超时时间
	IntervalStatusSend  = time.Minute * 10 // 定时发送状态间隔
	timeoutFetchUnit    = time.Second * 10 // 拉取单元等待超时时间
	intervalSamePeerCmp = time.Second * 10 // 同一个节点参与比较间隔时间
	maxFetchMisses      = 100
)

func NewRealTimeSync(backend Backend, chainr ChainReader, db ntsdb.Database) *RealTimeSync {
	return &RealTimeSync{
		backend: backend,
		cmping:  false,
		started: false,
		db:      db,
		chainr:  chainr,
	}
}

type RealTimeSync struct {
	mux       sync.Mutex
	backend   Backend
	status    *statusSwitch // 状态交换
	cmptor    *treeCmptor   // 差异对比
	cmping    bool          //是否正在对比中
	quit      chan struct{}
	started   bool
	blackList cmap.ConcurrentMap

	chainr      ChainReader
	db          ntsdb.Database
	unitTreeSer *Storage
}

func (t *RealTimeSync) Start() {
	begin := time.Now()

	t.mux.Lock()
	defer t.mux.Unlock()

	t.started = true
	t.blackList = cmap.NewWith(1)
	t.quit = make(chan struct{})

	if t.db != nil {
		first := t.chainr.GetHeaderByNumber(params.SCAccount, 1)
		if first == nil {
			panic("must have number 1 unit")
		}
		t.unitTreeSer = NewStorage(t.db, first.Timestamp, rawdb.RangRangeUnits)
		t.unitTreeSer.Start()
	} else {
		log.Warn("didn't have start one server")
	}
	t.status = newSwitcher(t.backend, t.CurrentStatus, t.startCmpWithPeer)
	t.status.start()
	go t.loopBlacklist()

	log.Info("started real time sync", "startup.time", time.Now().Sub(begin))
}
func (t *RealTimeSync) Stop() {
	t.mux.Lock()
	defer t.mux.Unlock()
	if !t.started {
		return
	}
	close(t.quit)
	t.started = false
	if !t.cmping {
		t.status.stop()
	}
	if t.unitTreeSer != nil {
		t.unitTreeSer.Stop()
	}

	t.cmptor = nil
	t.status = nil
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

// 主动触发一次广播状态
func (t *RealTimeSync) NewPointEvent() {
	t.status.resetRequester()
}

// 当前节点状态
func (t *RealTimeSync) CurrentStatus(number uint64) NodeStatus {
	if number == 0 {
		return t.unitTreeSer.LastPointStatus()
	} else {
		return t.unitTreeSer.GetPointStatus(number)
	}
}

// handler
func (t *RealTimeSync) handleEvent(ev *RealtimeEvent) error {
	// 判断是否在黑名单 * 给同一个节点提供服务需要间隔时间，防止恶意
	if t.blackList.Has(ev.From.String()) {
		log.Debug("peer at blacklist", "peer", ev.From)
		return nil
	}
	switch ev.Code {
	default:
		return fmt.Errorf("invalid message code %d", ev.Code)
	case RealtimeStatus, RealtimeRequest, RealtimeReponse:
		if t.isCmping() {
			return nil
		}
		t.status.handleResponse(ev)
		return nil
	case RealtimeCmpHash, RealtimeCmpDiff, RealtimeCmpEnd:
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
		select {
		case <-wait:
			// 差异对比结束 更新状态
			t.unitTreeSer.UpdateNow(ev.RemoteStatus.Number)
			t.stopCmp(true)
		case <-t.quit:
			return
		case <-time.After(TimeoutDiffCmp):
			// 超时，强行终止差异对比
			log.Debug("sync cmp timeout")
			t.stopCmp(true)
		}

		// 广播一次状态
		t.status.sendStatus(RealtimeStatus, t.CurrentStatus(ev.RemoteStatus.Number), message.PeerID{})
		// 一次对比结束后，peer加入黑名单
		t.blackList.Set(ev.From.String(), time.Now())
	}()
}
func (t *RealTimeSync) activateCmp(ev *RealtimeEvent, waitCh chan struct{}) {
	t.mux.Lock()
	defer t.mux.Unlock()
	if t.cmping {
		return
	}
	if tree, err := t.generateTree(ev.RemoteStatus.Number); err == nil {
		t.cmptor = newTreeCmptor(tree, t.CurrentStatus, t.backend, t.unitTreeSer)
	} else {
		log.Error("generate cmp tree err", "number", ev.RemoteStatus.Number, "err", err)
		return
	}
	t.status.stop() // 停止状态交换
	t.cmptor.start(ev, waitCh)
	t.cmping = true
}

// 创建指定位置的树结构
func (t *RealTimeSync) generateTree(number uint64) (Node, error) {
	return t.unitTreeSer.GetPointTree(number)
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
	if t.started {
		t.status.start()
	}
}
func (t *RealTimeSync) isCmping() bool {
	t.mux.Lock()
	defer t.mux.Unlock()
	return t.cmping
}
func (t *RealTimeSync) loopBlacklist() {
	t.blackList = cmap.New()

	tk := time.NewTicker(intervalSamePeerCmp / 2)
	for {
		select {
		case <-t.quit:
			return
		case <-tk.C:
			// 定时从黑名单移除peer
			now := time.Now()
			needRemove := make([]string, 0)
			t.blackList.IterCb(func(key string, v interface{}) bool {
				if now.Sub(v.(time.Time)) > intervalSamePeerCmp {
					needRemove = append(needRemove, key)
				}
				return true
			})
			if len(needRemove) == 0 {
				continue
			}
			for _, k := range needRemove {
				t.blackList.Remove(k)
			}
			log.Debug("recover sync cmp permission ", "recovered", len(needRemove), "blacklist", t.blackList.Count())
		}
	}
}

func (t *RealTimeSync) OnNewUnit(unit *types.Unit) {
	t.unitTreeSer.NewUnitDone(unit)
}
