package realtime

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/nts/synchronize/message"
	"gitee.com/nerthus/nerthus/nts/synchronize/realtime/merkle"
)

type Backend interface {
	GetChainsStatus() []merkle.Content
	BroadcastStatus(event *RealtimeEvent)
	SendMessage(event *RealtimeEvent, peer message.PeerID)
	SyncChain(map[common.Address]uint64, message.PeerID)
	ChainStatus(chain common.Address) (uint64, bool)
	ChainStatusByIndex(index uint64) (common.Address, uint64, bool)
}

type NodeStatus struct {
	Root   []byte
	Chains uint64
}

func (n NodeStatus) Empty() bool {
	return len(n.Root) == 0
}

type CmpMessage struct {
	Path     []byte // 从树root到node的路径，0：左，1：右
	EndPath  []byte // 终结点路径，当对比到达这个点，则可以结束对比
	Children [][]byte
}
type RealtimeEvent struct {
	Code         uint8
	From         message.PeerID `rlp:"-"`
	RemoteStatus NodeStatus
	Data         []byte
}

const (
	RealtimeStatus uint8 = iota
	RealtimeRequest
	RealtimeReponse

	RealtimePushChains
	RealtimeCmpHash
	RealtimeCmpEnd
)

// 链的状态信息
type ChainStatus struct {
	Addr   common.Address
	Number uint64
	Index  uint64
}

func (t ChainStatus) Hash(hasher func([]byte) []byte) []byte {
	return hasher(append(common.Uint64ToBytes(t.Index), common.Uint64ToBytes(t.Number)...))
}
func (t ChainStatus) Set(content merkle.Content) merkle.Content {
	v := content.(ChainStatus)
	t.Number = v.Number
	return t
}
func (t ChainStatus) Copy() merkle.Content {
	return t
}
func (t ChainStatus) Cmp(n merkle.Content) int {
	v := n.(ChainStatus)
	if t.Addr.Equal(v.Addr) {
		return 0
	} else {
		return 1
	}
}
