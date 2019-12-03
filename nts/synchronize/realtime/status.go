package realtime

import (
	"bytes"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/nts/synchronize/message"
	"time"
)

func newSwitcher(backend Backend, currentStatus func() NodeStatus, discoverDiff func(event *RealtimeEvent)) *statusSwitch {
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
	currentStatus func() NodeStatus
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
	if t.requestTimer != nil {
		t.requestTimer.Stop()
	}
	t.requestTimer = time.AfterFunc(t.interval, func() {
		t.sendStatus(RealtimeStatus, message.PeerID{})
		t.resetRequester()
	})

}
func (t *statusSwitch) handleResponse(ev *RealtimeEvent) {
	// 和自己的状态对比
	curr := t.currentStatus()
	t.logger.Trace("handle node status", "curr_root", curr.Root, "curr_chains", curr.Chains, "remote_root", ev.RemoteStatus.Root, "remote_chains", ev.RemoteStatus.Chains)
	// 只有当状态不一致才响应
	if ev.RemoteStatus.Chains == curr.Chains && bytes.Equal(ev.RemoteStatus.Root, curr.Root) {
		return
	}
	switch ev.Code {
	case RealtimeStatus:
		// 如果状态不一致，发送一个请求
		t.sendStatus(RealtimeRequest, ev.From)
	case RealtimeRequest:
		// 回应一个请求
		t.sendStatus(RealtimeReponse, ev.From)
		t.discoverDiff(ev)
	case RealtimeReponse:
		// 收到一个回应，立即进入差异对比
		t.discoverDiff(ev)
	}
}

func (t *statusSwitch) sendStatus(code uint8, peer message.PeerID) {
	// 发送自己的状态
	curr := &RealtimeEvent{
		Code:         code,
		RemoteStatus: t.currentStatus(),
	}
	t.logger.Trace("send node status", "code", curr.Code, "root", curr.RemoteStatus.Root, "chains", curr.RemoteStatus.Chains)
	if peer.IsEmpty() {
		t.backend.BroadcastStatus(curr)
	} else {
		t.backend.SendMessage(curr, peer)
	}
}
