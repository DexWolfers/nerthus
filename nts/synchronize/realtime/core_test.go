package realtime

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/nts/synchronize/message"
	"gitee.com/nerthus/nerthus/nts/synchronize/realtime/merkle"
)

type TestBackend struct {
	chains    []ChainStatus
	peer      message.PeerID
	syncChain func(map[common.Address]uint64, message.PeerID)
	msgCh     chan *RealtimeEvent
}

func (t *TestBackend) GetChainsStatus() []merkle.Content {
	cts := make([]merkle.Content, 0)
	for _, v := range t.chains {
		cts = append(cts, ChainStatus{Addr: v.Addr, Number: v.Number})
	}
	return cts
}
func (t *TestBackend) BroadcastStatus(event *RealtimeEvent) {
	log.Debug("broadcast status")
	event.From = t.peer
	t.msgCh <- event
}
func (t *TestBackend) SendMessage(event *RealtimeEvent, peer message.PeerID) {
	event.From = t.peer
	t.msgCh <- event
}
func (t *TestBackend) SyncChain(chains map[common.Address]uint64, peer message.PeerID) {
	t.syncChain(chains, peer)
	//syncChains(chains, t.peer, peer)
}
func (t *TestBackend) ChainStatus(chain common.Address) (uint64, bool) {
	return 0, false
}
