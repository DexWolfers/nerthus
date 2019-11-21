package statistics

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/event"
)

// ChainReader 定义一组在单元校验是所需要访问单元信息的方法集
type ChainReader interface {
	GetHeader(hash common.Hash) *types.Header
	// GetHeaderByHash 通过单元头Hash获取单元头
	GetHeaderByHash(hash common.Hash) *types.Header
	GetBrothers(addr common.Address, number uint64) (common.Hash, error)
	GetBody(addr common.Address, number uint64, hash common.Hash) *types.Body
	WakChildren(parentHash common.Hash, cb func(children common.Hash) bool)
	GenesisHash() common.Hash

	SubscribeChainEvent(ch chan<- types.ChainEvent) event.Subscription
}
