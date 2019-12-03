package bysolt

import (
	"time"

	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/nts/synchronize/message"
)

func newSwitcher(backend Backend, currentStatus func(number uint64) NodeStatus, discoverDiff func(event *RealtimeEvent)) *statusSwitch {
	return &statusSwitch{
		interval:      IntervalStatusSend,
		backend:       backend,
		currentStatus: currentStatus,
		discoverDiff:  discoverDiff,
		logger:        log.Root().New("module", "realtime sync"),
	}
}

type statusSwitch struct {
	interval      time.Duration
	backend       Backend
	quit          chan struct{}
	currentStatus func(number uint64) NodeStatus
	discoverDiff  func(event *RealtimeEvent)
	logger        log.Logger
	requestTimer  *time.Timer
}

func (t *statusSwitch) start() {
	t.quit = make(chan struct{})
	t.resetRequester()
	t.logger.Debug("start status sender")
}
func (t *statusSwitch) stop() {
	close(t.quit)
	t.requestTimer.Stop()
	t.logger.Debug("stop status sender")
}
func (t *statusSwitch) resetRequester() {
	// 广播当前最新状态
	t.sendStatus(RealtimeStatus, t.currentStatus(0), message.PeerID{})
	if t.requestTimer != nil {
		t.requestTimer.Stop()
	}
	t.requestTimer = time.AfterFunc(t.interval, func() {
		t.resetRequester()
	})

}
func (t *statusSwitch) handleResponse(ev *RealtimeEvent) error {
	// 和自己的状态对比
	curr := t.currentStatus(ev.RemoteStatus.Number)
	t.logger.Trace("handle node status", "code", ev.Code, "curr_number", curr.Number, "remote_number", ev.RemoteStatus.Number, "curr_root", curr.Root, "remote_root", ev.RemoteStatus.Root, "from", ev.From)
	if curr.Number != ev.RemoteStatus.Number {
		// 自己没有，不响应
		return nil
	}
	// 只有当前状态不一致才响应
	if ev.RemoteStatus.Root == curr.Root {
		return nil
	}
	// 是历史状态不一致 继续往上
	if ev.RemoteStatus.Parent != curr.Parent {
		t.logger.Trace("parent different", "number", curr.Number, "curr_p", curr.Parent, "remote_p", ev.RemoteStatus.Parent)
		t.sendStatus(RealtimeStatus, t.currentStatus(curr.Number-1), ev.From)
		return nil
	}
	switch ev.Code {
	case RealtimeStatus:
		// 如果状态不一致，发送一个请求
		t.sendStatus(RealtimeRequest, curr, ev.From)
	case RealtimeRequest:
		// 回应一个请求
		t.sendStatus(RealtimeReponse, curr, ev.From)
		t.discoverDiff(ev)
	case RealtimeReponse:
		// 收到一个回应，立即进入差异对比
		t.discoverDiff(ev)
	}
	return nil
}

func (t *statusSwitch) sendStatus(code uint8, status NodeStatus, peer message.PeerID) {
	// 发送自己的状态
	curr := &RealtimeEvent{
		Code:         code,
		RemoteStatus: status,
	}
	t.logger.Trace("send node status", "code", curr.Code, "root", curr.RemoteStatus.Root, "number", curr.RemoteStatus.Number, "parent", curr.RemoteStatus.Parent)
	if peer.IsEmpty() {
		t.backend.BroadcastStatus(curr)
	} else {
		t.backend.SendMessage(curr, peer)
	}
}
