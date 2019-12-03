package bysolt

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/nts/synchronize/message"
	"gitee.com/nerthus/nerthus/rlp"
)

func newTreeCmptor(tree Node, currentStatus func(number uint64) NodeStatus, backend Backend, unitPointReader UnitPointReader) *treeCmptor {
	return &treeCmptor{
		started:   new(int32),
		tree:      tree,
		backend:   backend,
		getStatus: currentStatus,
		upReader:  unitPointReader,
		logger:    log.Root().New("module", "realtime sync"),
	}
}

type treeCmptor struct {
	started     *int32
	tree        Node
	backend     Backend
	peer        message.PeerID
	peerStatus  NodeStatus
	getStatus   func(number uint64) NodeStatus
	endNodePath []byte
	tasker      *syncTasker
	quit        chan struct{}
	logger      log.Logger
	mux         sync.Mutex
	isEnd       bool

	upReader UnitPointReader
}

func (t *treeCmptor) start(ev *RealtimeEvent, waitCh chan struct{}) {
	if !atomic.CompareAndSwapInt32(t.started, 0, 1) {
		return
	}
	t.mux.Lock()
	t.quit = waitCh
	if ev.RemoteStatus.Root == t.currentStatus().Root {
		t.mux.Unlock()
		t.stop()
		return
	}
	defer t.mux.Unlock()
	t.peer = ev.From
	t.peerStatus = ev.RemoteStatus
	t.logger.Debug("cmptor starting", "msg code", ev.Code, "number", ev.RemoteStatus.Number, "remote root", ev.RemoteStatus.Root, "peer", t.peer)
	if ev.Code == RealtimeReponse {
		t.sendFirstCmp() // 由一方开启
	}
}
func (t *treeCmptor) stop() {
	if !atomic.CompareAndSwapInt32(t.started, 1, 0) {
		return
	}
	t.mux.Lock()
	defer t.mux.Unlock()
	t.tree = nil
	t.endNodePath = nil
	if t.tasker != nil {
		t.tasker.stop()
	}
	close(t.quit)
	t.logger.Debug("cmptor stop", "peer", t.peer)
}
func (t *treeCmptor) currentStatus() NodeStatus {
	return t.getStatus(t.peerStatus.Number)
}
func (t *treeCmptor) remoteRoot() Hash {
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
		if t.currentStatus().Root == t.peerStatus.Root {
			t.stop()
			return nil
		}
	}
	t.mux.Lock()
	defer t.mux.Unlock()
	t.logger.Trace("handle response", "code", ev.Code, "from", ev.From, "endPath", t.endNodePath)
	switch ev.Code {
	default:
		return fmt.Errorf("invalid message code %d", ev.Code)
	case RealtimeCmpHash:
		data := new(CmpMessage)
		if err := t.toInterface(ev.Data, data); err != nil {
			return err
		}
		node := t.tree
		endPath := data.EndPath
		for i, v := range data.Path {
			if node.IsLeaf() {
				endPath = endPath[:i+1]
				break
			}
			if v == 0 {
				node, _ = node.Children()
			} else {
				_, node = node.Children()
			}
		}
		t.logger.Trace("received cmp hash", "endPath", t.endNodePath, "msg_path", data.Path, "msg_endPath", data.EndPath)
		// 记录终结点
		if t.cmpWithEndPath(endPath) > 0 {
			t.endNodePath = endPath
		}
		// 已对比到叶子节点
		if node.IsLeaf() {
			t.findDiff(node)
			if t.cmpWithEndPath(data.Path) >= 0 {
				// 对比结束
				t.sendCmpEnd()
			}
			return nil
		}
		diffs := make([]struct {
			Node Node
			Path []byte
		}, 0)

		l, r := node.Children()
		for i, v := range data.Children {
			nd := l
			if i > 0 {
				nd = r
			}
			if !bytes.Equal(v, nd.Hash().Bytes()) {
				// hash不一致，继续比对下一级
				ph := append(data.Path, byte(i))
				diffs = append(diffs, struct {
					Node Node
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
	case RealtimeCmpDiff: // 发现不一致
		data := new(DiffMessage)
		if err := t.toInterface(ev.Data, data); err != nil {
			return err
		}
		// hashes是否超过上限
		if len(data.Hashes) > maxSlotUnits {
			return errors.New("to much unit hashes in period")
		}
		return t.handleDiff(data)
	case RealtimeCmpEnd: // 结束
		// 提醒对方结束
		t.sendCmpEnd()

		go func() {
			// 等待本地补全缺失
			t.tasker.wait()
			// 结束
			t.stop()
		}()
	}
	return nil
}
func (t *treeCmptor) handleDiff(msg *DiffMessage) error {
	mine := t.upReader.GetPeriodHashes(t.peerStatus.Number, msg.From, msg.To)
	miss := make([]common.Hash, 0)
loop:
	for i := range msg.Hashes {
		for m := range mine {
			if msg.Hashes[i] == mine[m] {
				continue loop
			}
		}
		// 记录缺失
		t.logger.Trace("discover miss hash", "hash", msg.Hashes[i])
		miss = append(miss, msg.Hashes[i])
	}
	t.logger.Trace("handle diff", "mine", len(mine), "remote", len(msg.Hashes), "miss", len(miss), "from", msg.From, "to", msg.To)
	if len(miss) == 0 {
		return nil
	}
	// 去拉取缺失的单元
	t.syncHashes(miss)
	return nil
}
func (t *treeCmptor) sendFirstCmp() {
	curr := t.currentStatus()
	t.logger.Trace("send first cmp", "current_root", curr.Root, "remote_root", t.peerStatus.Root)
	//从根节点开始
	t.sendDiffNode(t.tree, []byte{})
}
func (t *treeCmptor) sendDiffNode(node Node, path []byte) {
	data := CmpMessage{Path: path, EndPath: t.endNodePath}
	if c1, c2 := node.Children(); c1 != nil {
		if c2 == nil {
			data.Children = [][]byte{c1.Hash().Bytes()}
		} else {
			data.Children = [][]byte{c1.Hash().Bytes(), c2.Hash().Bytes()}
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
		t.findDiff(node)
		if t.cmpWithEndPath(data.Path) >= 0 {
			// 对比结束
			isOver = true
		}
	}
	t.logger.Trace("send cmp hash", "endPath", t.endNodePath, "path", data.Path)
	t.sendMsg(msg)
	if isOver {
		t.sendCmpEnd()
	}
}
func (t *treeCmptor) sendCmpEnd() {
	// 只发送一次end
	if t.isEnd {
		return
	}
	msg := &RealtimeEvent{
		Code: RealtimeCmpEnd,
	}
	t.sendMsg(msg)
	t.isEnd = true
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
func (t *treeCmptor) findDiff(node Node) {

	period := node.Timestamp()
	from, to := uint64(period.Begin.UnixNano()), uint64(period.End.UnixNano())
	diffs := t.upReader.GetPeriodHashes(t.peerStatus.Number, from, to)
	t.logger.Trace("find diff", "hash", node.Hash(), "from", period.Begin, "to", period.End, "diffs", len(diffs))

	data, err := t.toBytes(DiffMessage{
		From:   from,
		To:     to,
		Hashes: diffs,
	})
	if err != nil {
		t.logger.Error("rlp failed", "err", err)
	}
	msg := &RealtimeEvent{
		Code: RealtimeCmpDiff,
		Data: data,
	}
	t.sendMsg(msg)
}

func (t *treeCmptor) sendMsg(event *RealtimeEvent) {
	t.backend.SendMessage(event, t.peer)
	//t.logger.Debug("send msg", "code", event.Code)
}
func (t *treeCmptor) syncHashes(hashes []common.Hash) {
	if t.tasker == nil {
		t.tasker = startSyncTasker(t.peer, t.backend)
	}
	t.tasker.add(hashes)
	//for i := range hashes {
	//	t.tasker.add(hashes[i])
	//}
}

func (t *treeCmptor) toBytes(v interface{}) ([]byte, error) {
	return rlp.EncodeToBytes(v)
}

func (t *treeCmptor) toInterface(b []byte, v interface{}) error {
	return rlp.DecodeBytes(b, v)
}
