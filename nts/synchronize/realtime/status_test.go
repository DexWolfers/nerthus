package realtime

import (
	"fmt"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/p2p/discover"
	"testing"
)

func TestChainStatus_Cmp(t *testing.T) {
	ch := make(chan *RealtimeEvent, 1024)
	bk := &TestBackend{msgCh: ch}
	currStatus := NodeStatus{Root: common.BytesToHash([]byte{1, 1, 1, 1}), Chains: 10}

	quit := make(chan struct{})
	swer := newSwitcher(bk, func() NodeStatus {
		return currStatus
	}, func(event *RealtimeEvent) {
		t.Log(event)
		close(quit)
	})
	// 收消息
	times := 0
	pr := discover.NodeID{0x1}
	go func() {
		for {
			ev := <-ch
			fmt.Println("p2 handle>>>>", ev)
			switch ev.Code {
			case RealtimeStatus:
				state := &RealtimeEvent{Code: RealtimeStatus, From: pr, RemoteStatus: currStatus}
				if times > 3 {
					state.RemoteStatus = NodeStatus{Root: common.BytesToHash([]byte{1, 2, 1, 1, 3, 4}), Chains: 6}
					state.Code = RealtimeRequest
				}
				swer.handleResponse(state)
				times++
			case RealtimeReponse:

			}
		}
	}()
	swer.start()
	<-quit
}
