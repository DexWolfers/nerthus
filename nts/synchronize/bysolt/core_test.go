package bysolt

import (
	"hash"
	"hash/fnv"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/nts/synchronize/message"
	"gitee.com/nerthus/nerthus/p2p/discover"
)

func startCluster() *peerCluster {
	c := &peerCluster{
		make(map[message.PeerID]*TestBackend),
		fnv.New32(),
		make(chan []interface{}, 1024),
	}
	go c.loop()
	return c
}

type peerCluster struct {
	peers  map[message.PeerID]*TestBackend
	hasher hash.Hash
	ch     chan []interface{}
}

func (t *peerCluster) hash(b []byte) []byte {
	t.hasher.Reset()
	t.hasher.Write(b)
	return t.hasher.Sum(nil)
}
func (t *peerCluster) register(peer *TestBackend) {
	t.peers[peer.id] = peer
}
func (t *peerCluster) msg(ev *RealtimeEvent, to message.PeerID) {
	t.ch <- []interface{}{ev, to}
}
func (t *peerCluster) loop() {
	for {
		arg := <-t.ch
		ev, to := arg[0].(*RealtimeEvent), arg[1].(message.PeerID)
		if err := t.peers[to].handle(ev); err != nil {
			log.Error("handle msg err", "msg", ev, "to", to, "err", err)
		}
	}
}

func newPeer(id int, c *peerCluster) *TestBackend {
	peer := discover.NodeID{byte(id)}
	bk := &TestBackend{
		id:      peer,
		cluster: c,
	}
	c.register(bk)
	return bk
}

type TestBackend struct {
	points  [][]common.Hash
	id      message.PeerID
	handle  func(event *RealtimeEvent) error
	cluster *peerCluster
}

func (t *TestBackend) BroadcastStatus(event *RealtimeEvent) {
	for k, _ := range t.cluster.peers {
		if t.id == k {
			continue
		}
		t.SendMessage(event, k)
	}
}
func (t *TestBackend) SendMessage(event *RealtimeEvent, peer message.PeerID) {
	event.From = t.id
	t.cluster.msg(event, peer)
}
func (t *TestBackend) LastPointStatus() NodeStatus {
	index := len(t.points)
	return t.GetPointStatus(uint64(index))
}
func (t *TestBackend) GetPointStatus(number uint64) NodeStatus {
	index := number - 1
	var bytes []byte
	var parent Hash
	hashes := t.points[index]
	for i := range hashes {
		bytes = append(bytes, hashes[i].Bytes()...)
	}
	if index > 0 {
		parent = t.GetPointStatus(index).Root
	}
	bytes = append(parent.Bytes())

	root := CalcHash(bytes)
	//log.Trace("generate hash", "number", number, "len", len(hashes), "hash", root, "bytes", bytes)
	return NodeStatus{
		Root:   root,
		Parent: parent,
		Number: number,
	}
}
func (t *TestBackend) GetPeriodHashes(number, from uint64, to uint64) []common.Hash {
	return t.points[number-1]
}
func (t *TestBackend) SyncUnit(hash common.Hash, peer message.PeerID) {
	index := 0
	ps := t.points[len(t.points)-1]
	for i := range ps {
		b := ps[i].Big()
		if hash.Big().Cmp(b) > 0 {
			index++
		} else {
			break
		}
	}
	ps = append(ps[:index+1], ps[index:]...)
	ps[index] = hash
	t.points[len(t.points)-1] = ps
	log.Debug("----sync hash >>>>>>>", "from", peer, "hash", hash, "index", index, "len", len(ps))
}
func (t *TestBackend) GetTimestamp(number uint64) uint64 {
	return 0
}
func (t *TestBackend) GetPointTree(number uint64) (Node, error) {
	h := t.GetPointStatus(number)
	nd := testNode(h.Root.Bytes())
	return nd, nil
}

type testNode []byte

func (t testNode) Hash() Hash {
	h := Hash{}
	copy(h[:], t)
	return h
}
func (t testNode) Children() (Node, Node) {
	return nil, nil
}
func (t testNode) IsLeaf() bool {
	return true
}
func (t testNode) Parent() Node {
	return nil
}
func (t testNode) Timestamp() TimeSize {
	return TimeSize{}
}
