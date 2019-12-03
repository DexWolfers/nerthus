package bysolt

import (
	"hash"
	"hash/fnv"
	"sync"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/nts/synchronize/message"
)

type ChainReader interface {
	GetHeaderByNumber(mcaddr common.Address, number uint64) *types.Header
}
type UnitPointReader interface {
	LastPointStatus() NodeStatus
	GetPointStatus(number uint64) NodeStatus
	GetPeriodHashes(number, from uint64, to uint64) []common.Hash
	GetTimestamp(number uint64) uint64
	GetPointTree(number uint64) (Node, error)
}

type Backend interface {
	BroadcastStatus(event *RealtimeEvent)
	SendMessage(event *RealtimeEvent, peer message.PeerID)
	FetchUnitByHash(hashes []common.Hash, peer ...message.PeerID) error
	SubChainEvent() (chan types.ChainEvent, event.Subscription)
}
type Node interface {
	Hash() Hash
	Children() (Node, Node)
	IsLeaf() bool
	Timestamp() TimeSize
}

type NodeStatus struct {
	Root   Hash
	Parent Hash
	Number uint64
}

func (n NodeStatus) Empty() bool {
	return len(n.Root) == 0
}

type CmpMessage struct {
	Path     []byte // 从树root到node的路径，0：左，1：右
	EndPath  []byte // 终结点路径，当对比到达这个点，则可以结束对比
	Children [][]byte
}
type DiffMessage struct {
	From   uint64
	To     uint64
	Hashes []common.Hash
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

	RealtimeCmpHash
	RealtimeCmpDiff
	RealtimeCmpEnd
)

const HashLength = 4

var emptyHash Hash

type Hash [HashLength]byte

func (h Hash) String() string {
	return common.Bytes2Hex(h[:])
}
func (h Hash) Bytes() []byte {
	return h[:]
}

// 按时间分隔的单元统计点
type Point struct {
	Number   uint64
	Parent   Hash
	TreeRoot Hash //单元树根 Root
}

func (p *Point) Hash() Hash {
	//TODO: 将更加情况确定是否需要进行哈希缓存
	return CalcHash(p.Parent[:], p.TreeRoot[:], common.Uint64ToBytes(p.Number))
}
func (p Point) Empty() bool {
	return p.Number == 0 && p.Parent == emptyHash && p.TreeRoot == emptyHash
}
func (p *Point) ToNodeStauts() NodeStatus {
	if p == nil {
		return NodeStatus{}
	}
	return NodeStatus{
		Root:   p.Hash(),
		Parent: p.Parent,
		Number: p.Number,
	}
}

// 时间点统计下的小分片
type Slot struct {
	Depth uint64 //顶部是 0
	Hash  Hash
}

type UnitInfo struct {
	Time uint64
	Hash common.Hash
}
type SlotNode struct {
	Chains    []UnitInfo
	TimeRange TimeSize
}

type SlotStorage struct {
	Begin, End uint64
	UnitCount  uint64
}

var hasherPool = sync.Pool{
	New: func() interface{} {
		return fnv.New32()
	},
}

func CalcHash(b ...[]byte) (h Hash) {
	hasher := hasherPool.Get().(hash.Hash)
	for _, v := range b {
		hasher.Write(v)
	}
	hasher.Sum(h[:0])

	hasher.Reset()
	hasherPool.Put(hasher)
	return
}

func CalcHashMore(next func() ([]byte, bool)) (h Hash) {
	hasher := hasherPool.Get().(hash.Hash)
	for {
		b, ok := next()
		if !ok {
			break
		}
		hasher.Write(b)
	}
	hasher.Sum(h[:0])
	hasher.Reset()
	hasherPool.Put(hasher)
	return
}
