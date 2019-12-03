package wconn

import (
	"sync"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/nts/protocol"
	"gitee.com/nerthus/nerthus/p2p/discover"
	"gitee.com/nerthus/nerthus/params"
)

type WitnessSet struct {
	mut         sync.RWMutex
	changedFeed event.Feed
	peers       map[common.Address]protocol.Peer
	peerIds     map[discover.NodeID]common.Address

	logger log.Logger
}

func NewWitnessSet(logger log.Logger) *WitnessSet {
	return &WitnessSet{
		peers:   make(map[common.Address]protocol.Peer, params.ConfigParamsSCWitness),
		peerIds: make(map[discover.NodeID]common.Address, params.ConfigParamsSCWitness),
		logger:  logger,
	}
}

// 订阅见证人连接变更
func (ws *WitnessSet) SubChange(c chan types.WitnessConnectStatusEvent) event.Subscription {
	return ws.changedFeed.Subscribe(c)
}
func (ws *WitnessSet) AllWitness() []common.Address {
	l := make([]common.Address, 0, len(ws.peers))
	ws.mut.RLock()
	for w := range ws.peers {
		l = append(l, w)
	}
	ws.mut.RUnlock()
	return l
}

func (ws *WitnessSet) AddWitness(witness common.Address, peer protocol.Peer) {
	ws.mut.Lock()
	ws.peers[witness] = peer
	ws.peerIds[peer.ID()] = witness

	ws.mut.Unlock()
	peer.SetWitnessType()

	ws.changedFeed.Send(types.WitnessConnectStatusEvent{Witness: witness, Connected: true})
	ws.logger.Trace("witness connected", "witness", witness, "peer", peer.ID())
}
func (ws *WitnessSet) RemoveWitness(witness common.Address) {
	ws.mut.Lock()

	p := ws.peers[witness]
	if p != nil {
		delete(ws.peers, witness)
		delete(ws.peerIds, p.ID())
		ws.logger.Trace("remove witness connect info", "witness", witness, "peer", p.ID())
	}

	ws.mut.Unlock()

	ws.changedFeed.Send(types.WitnessConnectStatusEvent{Witness: witness, Connected: false})

}
func (ws *WitnessSet) GetPeer(witness common.Address) protocol.Peer {
	ws.mut.RLock()
	p := ws.peers[witness]
	ws.mut.RUnlock()
	return p
}

func (ws *WitnessSet) GetWitnessById(pid discover.NodeID) common.Address {
	ws.mut.RLock()
	w, _ := ws.peerIds[pid]
	ws.mut.RUnlock()
	return w
}

func (ws *WitnessSet) Len() int {
	ws.mut.RLock()
	l := len(ws.peers)
	ws.mut.RUnlock()
	return l
}
func (ws *WitnessSet) Peers() []protocol.Peer {
	ws.mut.RLock()
	list := make([]protocol.Peer, 0, len(ws.peers))
	for _, p := range ws.peers {
		list = append(list, p)
	}
	ws.mut.RUnlock()
	return list
}

// Gossip 广播消息给所有已连接的见证人
// peers 标识发送的见证人数
func (ws *WitnessSet) Gossip(code uint8, msg interface{}) (peers int) {
	peers = len(ws.peers)

	// TODO use stack instead of stack
	var peersConn = make([]protocol.Peer, 0, peers)
	var witness = make([]common.Address, 0, peers)

	ws.mut.RLock()
	for addr, peer := range ws.peers {
		peersConn = append(peersConn, peer)
		witness = append(witness, addr)
	}
	ws.mut.RUnlock()

	for i, conn := range peersConn {
		if err := conn.AsyncSendMessage(code, msg); err != nil {
			ws.logger.Trace("send message to witness done", "witness", witness[i], "peer", conn.ID(), "code", code, "err", err)
		}
	}
	return peers
}

func (ws *WitnessSet) GossipTo(code uint8, msg interface{}, witness ...common.Address) (sends int) {
	for _, w := range witness {
		p := ws.GetPeer(w)
		if p != nil {
			if err := p.AsyncSendMessage(code, msg); err != nil {
				ws.logger.Trace("send message to witness done", "witness", w, "peer", p.ID(), "code", code, "err", err)
			} else {
				sends++
			}
		}
	}
	return
}
