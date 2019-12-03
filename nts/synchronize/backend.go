package synchronize

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/nts/synchronize/bysolt"
	"gitee.com/nerthus/nerthus/nts/synchronize/message"
)

func NewSyncBackend(m *SyncMain) *SyncBackend {
	return &SyncBackend{
		m: m,
	}
}

type SyncBackend struct {
	m *SyncMain
}

func (t *SyncBackend) BroadcastStatus(event *bysolt.RealtimeEvent) {
	t.m.SendMessageWitCode(message.CodeRealTimeMsg, event)
}
func (t *SyncBackend) SendMessage(event *bysolt.RealtimeEvent, peer message.PeerID) {
	t.m.SendMessageWitCode(message.CodeRealTimeMsg, event, peer)
}
func (t *SyncBackend) FetchUnitByHash(hashes []common.Hash, peer ...message.PeerID) error {
	return t.m.FetchUnitByHash(hashes, peer...)
}
func (t *SyncBackend) SubChainEvent() (chan types.ChainEvent, event.Subscription) {
	return t.m.createChainEvent()
}
