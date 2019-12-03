package realtime

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/nts/synchronize/message"
	"gitee.com/nerthus/nerthus/nts/synchronize/realtime/merkle"
	"gitee.com/nerthus/nerthus/rlp"
	"github.com/hashicorp/golang-lru"
)

func newTreeCmptor(tree *merkle.Tree, backend Backend) *treeCmptor {
	return &treeCmptor{
		started: new(int32),
		tree:    tree,
		backend: backend,
		currentStatus: func() NodeStatus {
			return NodeStatus{
				tree.Root().Hash(),
				uint64(tree.LeafCount()),
			}
		},
		logger: log.Root().New("module", "realtime sync"),
	}
}

type treeCmptor struct {
	started       *int32
	tree          *merkle.Tree
	backend       Backend
	peer          message.PeerID
	peerStatus    NodeStatus
	currentStatus func() NodeStatus
	diffNode      []uint64
	endNodePath   []byte
	quit          chan struct{}
	msgCache      *lru.Cache
	logger        log.Logger
	mux           sync.Mutex

	curChainMsgBatchNo uint64 //接收到对方节点下发的链数据，分批的批次号。
}

func (t *treeCmptor) start(ev *RealtimeEvent, waitCh chan struct{}) {
	if !atomic.CompareAndSwapInt32(t.started, 0, 1) {
		return
	}
	t.mux.Lock()
	t.quit = waitCh
	if bytes.Equal(ev.RemoteStatus.Root, t.currentStatus().Root) {
		t.mux.Unlock()
		t.stop()
		return
	}
	defer t.mux.Unlock()
	t.curChainMsgBatchNo = 0
	t.peer = ev.From
	t.peerStatus = ev.RemoteStatus
	t.logger.Debug("cmptor starting", "msg code", ev.Code, "remote root", ev.RemoteStatus.Root, "remote chains", ev.RemoteStatus.Chains, "peer", t.peer)
	t.sendFirstCmp(ev.Code == RealtimeReponse)
}
func (t *treeCmptor) stop() {
	if !atomic.CompareAndSwapInt32(t.started, 1, 0) {
		return
	}
	t.mux.Lock()
	defer t.mux.Unlock()
	t.tree.Clear()
	t.tree = nil
	t.diffNode = nil
	t.endNodePath = nil
	t.curChainMsgBatchNo = 0
	close(t.quit)
	t.logger.Debug("cmptor stop", "peer", t.peer)
}
func (t *treeCmptor) remoteRoot() []byte {
	return t.peerStatus.Root
}
func (t *treeCmptor) handleResponse(ev *RealtimeEvent) error {
	if atomic.LoadInt32(t.started) == 0 {
		return nil
	}
	if ev.From != t.peer {
		return nil
	}
	if !ev.RemoteStatus.Empty() {
		t.peerStatus = ev.RemoteStatus
		// 更新之后如果已经一致，退出
		if bytes.Equal(t.currentStatus().Root, t.peerStatus.Root) {
			t.stop()
			return nil
		}
	}
	t.mux.Lock()
	defer t.mux.Unlock()
	t.logger.Trace("handle response", "code", ev.Code, "from", ev.From.String(), "endPath", t.endNodePath)
	switch ev.Code {
	default:
		return fmt.Errorf("invalid message code %d", ev.Code)
	case RealtimePushChains:
		err := t.handleChainsMsg(ev)
		if err != nil {
			return err
		}
	case RealtimeCmpHash:
		data := new(CmpMessage)
		if err := t.toInterface(ev.Data, data); err != nil {
			return err
		}
		node := t.tree.Root()
		for _, v := range data.Path {
			if v == 0 {
				node, _ = node.Children()
			} else {
				_, node = node.Children()
			}
			if node == nil {
				return errors.New("different tree struct")
			}
		}
		t.logger.Trace("received cmp hash", "diff", len(t.diffNode), "endPath", t.endNodePath, "msg_path", data.Path, "msg_endPath", data.EndPath)
		// 记录终结点
		if t.cmpWithEndPath(data.EndPath) > 0 {
			t.endNodePath = data.EndPath
		}
		// 已对比到叶子节点
		if node.IsLeaf() {
			t.cacheDiff(node.Index())
			if t.cmpWithEndPath(data.Path) >= 0 {
				// 对比结束
				t.sendCmpEnd()
			}
			return nil
		}
		diffs := make([]struct {
			Node merkle.Node
			Path []byte
		}, 0)

		for i, v := range data.Children {
			var nd merkle.Node
			if i == 0 {
				nd, _ = node.Children()
			} else {
				_, nd = node.Children()
			}
			if nd == nil {
				return errors.New("different tree struct")
			}
			if bytes.Equal(v, nd.Hash()) {
				// hash不一致，继续比对下一级
				ph := append(data.Path, byte(i))
				diffs = append(diffs, struct {
					Node merkle.Node
					Path []byte
				}{Node: nd, Path: ph})
				t.logger.Trace("discover diff", "path", ph)
				// 记录最大终结点
				if t.cmpWithEndPath(ph) > 0 {
					t.endNodePath = ph
				}
			}
		}
		for _, v := range diffs {
			t.sendDiffNode(v.Node, v.Path)
		}
	case RealtimeCmpEnd: // 结束
		var data []ChainStatus
		if err := t.toInterface(ev.Data, &data); err != nil {
			return err
		}
		t.logger.Trace("receive cmp before", "local_diffs", len(t.diffNode), "remote_diffs", len(data))
		// 取交集
		needPush := make(map[common.Address]uint64)
		for _, v := range data {
			for _, d := range t.diffNode {
				if v.Index == d {
					addr, number, ok := t.backend.ChainStatusByIndex(d)
					if !ok {
						continue
					}
					if v.Addr.Equal(addr) && number > v.Number {
						needPush[v.Addr] = v.Number
						break
					}
				}
			}
			//t.logger.Debug("*********remote", "addr", v.Addr, "number", v.Number)
		}
		t.logger.Trace("received cmp end", "local", len(t.diffNode), "remote", len(data), "needPush", len(needPush))
		// 将对方节点缺少的链单元推送
		if len(needPush) > 0 {
			t.backend.SyncChain(needPush, t.peer)
		}
		// 结束
		go t.stop()
	}
	return nil
}
func (t *treeCmptor) sendFirstCmp(isMain bool) {
	curr := t.currentStatus()
	t.logger.Trace("send first cmp", "current_root", curr.Root, "current_chains", curr.Chains, "remote_root", t.peerStatus.Root, "remote_chains", t.peerStatus.Chains, "isMain", isMain)
	// 如果链数量不一致，由链多的一方发起
	if t.peerStatus.Chains < curr.Chains {
		t.pushChains(func(f func(chain common.Address) bool) {
			t.rangeChainsFromIndex(t.peerStatus.Chains, f)
		})
		return
	} else if t.peerStatus.Chains == curr.Chains && isMain { // 如果链数量一致，只由一方发起
		//从根节点开始
		t.sendDiffNode(t.tree.Root(), []byte{})
	}
}
func (t *treeCmptor) sendCmpEnd() {

	if len(t.diffNode) == 0 {
		return
	}
	data := make([]ChainStatus, 0)
	for i := range t.diffNode {
		if chain, number, ok := t.backend.ChainStatusByIndex(t.diffNode[i]); ok {
			data = append(data, ChainStatus{chain, number, t.diffNode[i]})
		}
	}
	bytes, err := t.toBytes(data)
	if err != nil {
		t.logger.Error("rlp encode error", "err", err)
		return
	}
	t.logger.Trace("send cmp end", "diff", len(t.diffNode), "endPath", t.endNodePath, "data", len(data))
	t.sendMsg(&RealtimeEvent{
		Code: RealtimeCmpEnd,
		Data: bytes,
	})
}
func (t *treeCmptor) sendDiffNode(node merkle.Node, path []byte) {
	data := CmpMessage{Path: path, EndPath: t.endNodePath}
	if c1, c2 := node.Children(); c1 != nil {
		if c2 == nil {
			data.Children = [][]byte{c1.Hash()}
		} else {
			data.Children = [][]byte{c1.Hash(), c2.Hash()}
		}
	}
	bytes, err := t.toBytes(data)
	if err != nil {
		t.logger.Error("rlp encode error", "err", err)
		return
	}
	msg := &RealtimeEvent{
		Code: RealtimeCmpHash,
		Data: bytes,
	}
	// 第一次需要带上自己的状态
	if len(path) == 0 {
		msg.RemoteStatus = t.currentStatus()
	}

	isOver := false
	if node.IsLeaf() {
		// 这里已经是叶子节点，自己记录差异
		t.cacheDiff(node.Index())
		if t.cmpWithEndPath(data.Path) >= 0 {
			// 对比结束
			isOver = true
		}
	}
	t.logger.Trace("send cmp hash", "diff", len(t.diffNode), "endPath", t.endNodePath, "path", data.Path)
	t.sendMsg(msg)
	if isOver {
		t.sendCmpEnd()
	}
}

func (t *treeCmptor) toBytes(v interface{}) ([]byte, error) {
	return rlp.EncodeToBytes(v)
}

func (t *treeCmptor) toInterface(b []byte, v interface{}) error {
	return rlp.DecodeBytes(b, v)
}

func (t *treeCmptor) cmpWithEndPath(path []byte) int {
	for i, v := range t.endNodePath {
		if i >= len(path) {
			return -1
		}
		if v > path[i] {
			return -1
		} else if v < path[i] {
			return 1
		}
	}
	if len(path) > len(t.endNodePath) {
		return 1
	}
	return 0
}
func (t *treeCmptor) sendMsg(event *RealtimeEvent) {
	t.backend.SendMessage(event, t.peer)
	//t.logger.Debug("send msg", "code", event.Code)
}
func (t *treeCmptor) cacheDiff(leafIndex uint64) {
	t.diffNode = append(t.diffNode, leafIndex)
	t.logger.Trace("cache diff chain", "v", leafIndex)
}
func (t *treeCmptor) getChainStatus(index uint64) (s ChainStatus, ok bool) {
	s.Addr, s.Number, ok = t.backend.ChainStatusByIndex(index)
	return
}
func (t *treeCmptor) chainsFromIndex(index uint64) (ls []common.Address) {
	for {
		if chain, _, ok := t.backend.ChainStatusByIndex(index); ok {
			ls = append(ls, chain)
			index++
		} else {
			return
		}
	}
}
func (t *treeCmptor) rangeChainsFromIndex(index uint64, f func(chain common.Address) bool) {
	for {
		chain, _, ok := t.backend.ChainStatusByIndex(index)
		if !ok {
			return
		}
		if !f(chain) {
			return
		}
		index++
	}
}
