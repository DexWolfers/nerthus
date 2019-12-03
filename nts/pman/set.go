package pman

import (
	"bytes"
	"context"
	"sync"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/crypto/sha3"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/nts/protocol"
	"gitee.com/nerthus/nerthus/p2p"
	"gitee.com/nerthus/nerthus/p2p/discover"
	"gitee.com/nerthus/nerthus/rlp"
)

// PeerSet represents the collection of active peers currently participating in
// the Nerthus sub-protocol.
type PeerSet struct {
	peers  map[discover.NodeID]*Peer
	lock   sync.RWMutex
	closed bool

	peerFeed event.Feed //Peer add or drop event
}

// NewPeerSet creates a new Peer set to track the active participants.
func NewPeerSet() *PeerSet {
	return &PeerSet{
		peers: make(map[discover.NodeID]*Peer),
	}
}

// Register injects a new Peer into the working set, or returns an error if the
// Peer is already known. If a new Peer it registered, its broadcast loop is also
// started.
func (ps *PeerSet) Register(p *Peer) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	if ps.closed {
		return errClosed
	}
	if _, ok := ps.peers[p.id]; ok {
		return RrrAlreadyRegistered
	}
	ps.peers[p.Peer.ID()] = p
	log.Trace("register", "id", p.Peer.ID(), "remote", p.Peer.RemoteAddr())

	go ps.peerFeed.Send(protocol.PeerEvent{
		Connected: true,
		PeerID:    p.ID(),
		Peer:      p,
	})

	go p.broadcast()

	return nil
}

// Unregister removes a remote Peer from the active set, disabling any further
// actions to/from that particular entity.
func (ps *PeerSet) Unregister(id discover.NodeID) error {
	return ps.Disconnect(id, p2p.DiscQuitting)
}

// 断开给定的Peer并告知断开原因
func (ps *PeerSet) Disconnect(id discover.NodeID, reason p2p.DiscReason) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	p, ok := ps.peers[id]
	if !ok {
		return errNotRegistered
	}
	log.Trace("disconnect", "id", id, "reason", reason, "remote", p.Peer.RemoteAddr())
	p.Close(reason)

	delete(ps.peers, id)

	go ps.peerFeed.Send(protocol.PeerEvent{
		Connected: false,
		PeerID:    p.ID(),
	})
	return nil
}

// SubscribePeers subscribes the given channel to Peer events
// 只支持订阅：Peer 进入、退出事件
func (ps *PeerSet) SubscribePeerEvent(ch chan protocol.PeerEvent) event.Subscription {
	return ps.peerFeed.Subscribe(ch)
}

// Peer retrieves the registered Peer with the given id.
func (ps *PeerSet) Peer(id discover.NodeID) *Peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return ps.peers[id]
}

// Len returns if the current number of peers in the set.
func (ps *PeerSet) Len() int {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return len(ps.peers)
}

// PeersWithoutBlock retrieves a list of peers that do not have a given block in
// their set of known hashes.
func (ps *PeerSet) PeersWithoutBlock(hash common.Hash) []*Peer {
	ps.lock.RLock()
	list := make([]*Peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.knownBlocks.Has(hash.String()) {
			list = append(list, p)
		}
	}
	ps.lock.RUnlock()
	return list
}

// PeersWithoutTx retrieves a list of peers that do not have a given transaction
// in their set of known hashes.
func (ps *PeerSet) PeersWithoutTx(hash common.Hash) []*Peer {
	ps.lock.RLock()
	list := make([]*Peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.knownTxs.Has(hash.String()) {
			list = append(list, p)
		}
	}
	ps.lock.RUnlock()
	return list
}

// PeersWithoutPayloadHash 无此广播消息的节点
func (ps *PeerSet) PeersWithoutPayloadHash(hash common.Hash) []*Peer {
	ps.lock.RLock()
	list := make([]*Peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.knownTTLMsg.Has(hash.String()) {
			list = append(list, p)
		}
	}
	ps.lock.RUnlock()
	return list
}

func (ps *PeerSet) Peers() []*Peer {
	ps.lock.RLock()
	list := make([]*Peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		list = append(list, p)
	}
	ps.lock.RUnlock()
	return list
}

// BestPeer retrieves the known Peer with the currently highest total difficulty.
func (ps *PeerSet) BestPeer() *Peer {
	ps.lock.RLock()
	var (
		bestPeer *Peer
	)
	for _, p := range ps.peers {
		if heade := p.SysChainHead(); bestPeer == nil || heade.Height > bestPeer.SysChainHead().Height {
			bestPeer = p
		}
	}
	ps.lock.RUnlock()
	return bestPeer
}

// Close disconnects all peers.
// No new peers can be registered after Close has returned.
func (ps *PeerSet) Close() {
	ps.lock.Lock()
	for _, p := range ps.peers {
		p.Close(p2p.DiscQuitting)
	}
	ps.closed = true
	ps.lock.Unlock()
}

func (ps *PeerSet) BroadcastWithTTL(code uint8, ttl uint32, data interface{}) error {
	switch code {
	case protocol.NewUnitMsg:
		unit := data.(*types.Unit)
		for _, p := range ps.PeersWithoutBlock(unit.Hash()) {
			if err := p.SendNewBlock(unit); err != nil {
				p.Log().Trace("failed to send unit", "uhash", unit.Hash(), "err", err)
			}
		}
	case protocol.TxMsg:
		tx := data.(*types.Transaction)
		for _, p := range ps.PeersWithoutTx(tx.Hash()) {
			if err := p.SendTransactions([]*types.Transaction{tx}); err != nil {
				p.Log().Trace("failed to send tx", "txhash", tx.Hash(), "err", err)
			}
		}
	default:
		// rlp
		b, err := rlp.EncodeToBytes(data)
		if err != nil {
			return err
		}
		// 获取哈希
		hash, err := messagePayloadHash(b)
		if err != nil {
			return err
		}
		for _, p := range ps.Peers() {
			p.Log().Trace("send message with ttl", "code", protocol.CodeString(code))
			// 将执行POW计算
			msg := protocol.TTLMsg{Msg: p2p.Msg{
				Code:    uint64(code),
				Size:    uint32(len(b)),
				Expiry:  uint32(time.Now().Add(time.Second * time.Duration(ttl)).Unix()),
				TTL:     ttl,
				Payload: bytes.NewReader(b)},
				PayloadHash: hash,
			}
			if err := p.SendTTLMsg(msg); err != nil {
				p.Log().Trace("failed to send message", "code", code, "err", err)
			}
		}
	}
	return nil
}

// 广播交易，交易只广播给尚未登记此交易的节点中一部分人。
// 以满足网络消息减半降低。
func (ps *PeerSet) BroadcastTxs(ctx context.Context, txs types.Transactions) error {
	//每笔交易均需要广播到不同节点，因此在广播时，采用单条依次发送
	for _, tx := range txs {
		s := ps.PeersWithoutTx(tx.Hash())
		if len(s) == 0 {
			continue
		}
		//分别发送
		//for _, p := range s[:int(math.Sqrt(float64(len(ps.peers))))] {
		//	// TODO(ysqi): 出现p为空情况，但不清楚原因
		//	if p != nil {
		//		p.AsyncSendTransactions([]*types.Transaction{tx})
		//	}
		//}
		for _, p := range s {
			// TODO(ysqi): 出现p为空情况，但不清楚原因
			if p != nil {
				p.AsyncSendTransactions([]*types.Transaction{tx})
			}
		}
	}
	return nil
}

func messagePayloadHash(payload []byte) (h common.Hash, err error) {
	hw := sha3.NewKeccak256()
	_, err = hw.Write(payload)
	if err != nil {
		return h, err
	}
	hw.Sum(h[:0])
	sha3.PutKeccak256(hw)
	return h, nil
}
