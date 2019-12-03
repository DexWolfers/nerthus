package rwit

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/event"
)

type Chainer interface {
	SubscribeChainEvent(ch chan<- types.ChainEvent) event.Subscription
	GetChainTailNumber(chain common.Address) uint64
	GetChainTailHead(chain common.Address) *types.Header
	GetChainTailState(chain common.Address) (*state.StateDB, error)
	GetHeaderByNumber(chain common.Address, number uint64) *types.Header
}

type Backend interface {
	//当前运行中的见证人
	CurrWitness() common.Address
	//给定地址是否是当前的有效见证人，如果无效将不接收此地址消息
	IsWitness(common.Address) bool
	//签名
	Sign(types.SignHelper) error

	WitnessSet() WitnessSet
}

type WitnessSet interface {
	Len() int
	AllWitness() []common.Address
	SubChange(c chan types.WitnessConnectStatusEvent) event.Subscription
	GossipTo(code uint8, msg interface{}, witness ...common.Address) int
	Gossip(code uint8, msg interface{}) (peers int)
}
