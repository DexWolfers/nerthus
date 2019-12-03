package status

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/event"
)

//lastHash, lastTime := s.api.DagChain().GetLastUintHash()
//	tail := s.api.DagChain().GetChainTailHead(params.SCAccount)

type ChainReader interface {
	GetLastUintHash() (common.Hash, uint64)
	GetChainTailHead(chain common.Address) *types.Header

	SubscribeChainEvent(ch chan<- types.ChainEvent) event.Subscription
}
