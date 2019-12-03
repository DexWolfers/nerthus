package synchronize

import (
	"gitee.com/nerthus/nerthus/accounts"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/nts/protocol"
	"gitee.com/nerthus/nerthus/p2p/discover"
)

type UnitEventSubRead interface {
	SubscribeChainEvent(ch chan<- types.ChainEvent) event.Subscription
	SubscribeUnitStatusEvent(ch chan<- types.RealtimeSyncEvent) event.Subscription
	SubscribeChainProcessStatusCHangedEvent(ch chan<- types.ChainMCProcessStatusChangedEvent) event.Subscription
}

type ChainWrite interface {
	InsertChain(chain types.Blocks) (int, error)
}

type ChainReader interface {
	UnitEventSubRead
	GetChainTailHash(mc common.Address) common.Hash
	GetHeaderByHash(hash common.Hash) *types.Header
	GetUnitByHash(hash common.Hash) *types.Unit
	GetUnitNumber(uhash common.Hash) (mc common.Address, number uint64)
	GetStableHash(addr common.Address, number uint64) common.Hash
	GetChainTailHead(chain common.Address) *types.Header
	GetLastUintHash() (common.Hash, uint64)
	GetChainTailNumber(mc common.Address) uint64
	GetUnitByNumber(mcaddr common.Address, number uint64) *types.Unit
}

type ChainWriteRead interface {
	ChainWrite
	ChainReader
}

type APIBackend interface {
	DisconnectPeer(peerID []byte)
	SubscribePeerEvent(ch chan protocol.PeerEvent) event.Subscription
	RemoveWhitelist(peerID []byte)
	SetWhitelist(peerID []byte)
	AddPeer(id *discover.Node) error
	PeerSelf() *discover.Node

	// 本地所有账户
	Accounts() []common.Address
	// 订阅本地账户变动事件，可以实时感知到账户的添加与删除
	SubscribeAccount(c chan<- accounts.WalletEvent) event.Subscription

	// 当前节点所正在运行的矿工（见证人）
	CurrentMiner() common.Address

	// 获取指定见证人所需参与见证的链
	GetWitnessChains(witness common.Address) ([]common.Address, error)

	// 当前见证人是否存在见证组内
	IsJoinWitnessGroup(address common.Address) bool
	DagChain() *core.DagChain
}
