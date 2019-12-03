package ucache

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/nts/synchronize/message"
	"gitee.com/nerthus/nerthus/nts/synchronize/status"
)

type UnitEventSubRead interface {
	SubscribeChainEvent(ch chan<- types.ChainEvent) event.Subscription
	SubscribeUnitStatusEvent(ch chan<- types.RealtimeSyncEvent) event.Subscription
}

type ChainWrite interface {
	InsertChain(chain types.Blocks) (int, error)
}

type ChainReader interface {
	UnitEventSubRead
	GetChainTailHead(chain common.Address) *types.Header
	GetChainTailHash(mc common.Address) common.Hash
	GetChainTailNumber(mc common.Address) uint64
	GetHeaderByHash(hash common.Hash) *types.Header
	GetUnitByHash(hash common.Hash) *types.Unit
	GetUnitNumber(uhash common.Hash) (mc common.Address, number uint64)
	GetStableHash(addr common.Address, number uint64) common.Hash
}

type ChainWriteRead interface {
	ChainWrite
	ChainReader
}

type MainSync interface {
	PeerStatusSet() *status.PeerStatusSet
	SendMessageWitCode(code message.Code, msg interface{}, peers ...message.PeerID) error
	FetchUnitByID(uid types.UnitID) error
}
