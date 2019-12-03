package status

import (
	"fmt"
	"sync"

	"gitee.com/nerthus/nerthus/core/types"

	"gitee.com/nerthus/nerthus/log"

	"gitee.com/nerthus/nerthus/event"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/nts/synchronize/message"
	"gitee.com/nerthus/nerthus/params"
)

type PeerID = message.PeerID

const CodePeerStatus message.Code = 20

type UintStatus struct {
	Number uint64      //单元高度
	UHash  common.Hash //单元哈希
}

// 节点数据状态
type NodeStatus struct {
	LastUintTime  uint64      //最后一个单元时间戳
	LastUintHash  common.Hash //最后一个单元哈希
	SysTailNumber uint64      //单元高度
	SysTailHash   common.Hash //单元哈希
}

func (a NodeStatus) Compare(b NodeStatus) int {
	return compareStatus(a, b)
}
func (a NodeStatus) String() string {
	return fmt.Sprintf("Last(%d,%s)+Tail(%d,%s)",
		a.LastUintTime, a.LastUintHash.Hex(),
		a.SysTailNumber, a.SysTailHash.Hex())
}

type PeerStatusSet struct {
	cr       ChainReader
	myStatus *NodeStatus
	feed     event.Feed

	locker sync.RWMutex
	others *sync.Map
}

func NewPeerStatus(cr ChainReader) *PeerStatusSet {
	ps := &PeerStatusSet{
		cr:     cr,
		others: &sync.Map{},
	}
	ps.updateMyStatus()
	return ps
}
func (s *PeerStatusSet) MyStatus() NodeStatus {
	s.updateMyStatus()
	s.locker.RLock()
	defer s.locker.RUnlock()
	return *s.myStatus
}

// 根据单元检测状态
func (s *PeerStatusSet) CheckMyStatus(unit *types.Unit) {
	var changed bool
	s.locker.Lock()
	if status, ye := haveNewStatus(*s.myStatus, unit); ye {
		s.myStatus = &status
		changed = true
	}
	s.locker.Unlock()

	//如果单元是来自某个节点，则自动更新节点的数据状态，如果有单元则必然已链接
	if p, ok := unit.ReceivedFrom.(PeerID); ok && !p.IsEmpty() {
		if status, ye := haveNewStatus(s.Get(p), unit); ye {
			s.Update(p, status)
		}
	}

	if changed {
		s.feed.Send(PeerID{})
	}

	return
}

func haveNewStatus(status NodeStatus, unit *types.Unit) (NodeStatus, bool) {
	var changed bool
	if status.LastUintTime < unit.Timestamp() {
		status.LastUintTime = unit.Timestamp()
		status.LastUintHash = unit.Hash()
		changed = true
	}
	if status.SysTailNumber < unit.SCNumber() {
		status.SysTailNumber = unit.SCNumber()
		status.SysTailHash = unit.SCHash()
		changed = true
	}
	return status, changed
}

func (s *PeerStatusSet) updateMyStatus() {
	s.locker.Lock()
	defer s.locker.Unlock()

	if s.myStatus == nil {
		s.myStatus = &NodeStatus{}
	}
	s.myStatus.LastUintHash, s.myStatus.LastUintTime = s.cr.GetLastUintHash()
	tail := s.cr.GetChainTailHead(params.SCAccount)
	s.myStatus.SysTailHash = tail.Hash()
	s.myStatus.SysTailNumber = tail.Number
}

func (set *PeerStatusSet) SubChange(c chan PeerID) event.Subscription {
	return set.feed.Subscribe(c)
}

func (set *PeerStatusSet) Update(peer PeerID, status NodeStatus) {
	set.others.Store(peer, &status)
	log.Trace("get peer chain status", "peer", peer, "status", status)
	set.feed.Send(peer) //发送更新事件
}
func (set *PeerStatusSet) Get(peer PeerID) NodeStatus {
	v, ok := set.others.Load(peer)
	if !ok {
		return NodeStatus{}
	}
	return *(v.(*NodeStatus))
}

// 找出比给定的状态更好的节点
func (set *PeerStatusSet) Rich(vs NodeStatus, skip func(id PeerID) bool) (p PeerID, ok bool) {
	set.Range(func(key, value interface{}) bool {
		if skip != nil && skip(key.(PeerID)) {
			return true
		}
		if v := *value.(*NodeStatus); compareStatus(v, vs) > 0 {
			p = key.(PeerID)
			ok = true

			return false
		}
		return true
	})
	return
}

// 从所有节点中选出最佳节点
func (set *PeerStatusSet) Best(skip func(id PeerID) bool) (p PeerID) {
	var s NodeStatus
	set.Range(func(key, value interface{}) bool {
		if skip != nil && skip(key.(PeerID)) {
			return true
		}
		if v := *value.(*NodeStatus); s.LastUintTime == 0 || compareStatus(v, s) > 0 {
			s = v
			p = key.(PeerID)
		}
		return true
	})
	return
}
func (set *PeerStatusSet) Range(f func(key, value interface{}) bool) {
	set.others.Range(f)
}
func (set *PeerStatusSet) Delete(p interface{}) {
	set.others.Delete(p)
}

func (set *PeerStatusSet) Empty() bool {
	var exist bool
	set.others.Range(func(key, value interface{}) bool {
		exist = true
		return false
	})
	return !exist
}

// 比较两个状态，A 状态好与 B 时返回1，B 状态好于 A时返回-1
// 状态好的条件：高度更高或者时间更大
func compareStatus(a, b NodeStatus) int {
	if a.SysTailNumber == b.SysTailNumber && a.LastUintTime == b.LastUintTime {
		return 0
	}

	if a.SysTailNumber > b.SysTailNumber || a.LastUintTime > b.LastUintTime {
		return 1
	}
	return -1
}
