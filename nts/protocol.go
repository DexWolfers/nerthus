package nts

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/event"
)

const (
	// Official short name of the protocol used during capability negotiation.
	ProtocolName = "nts"
)

type txPool interface {
	// AddRemotes 将给定的交易tian
	AddRemotes([]*types.Transaction) error

	// Pending should return pending transactions.
	// The slice should be modifiable by the caller.
	Pending() (types.Transactions, error)

	// SubscribeTxPreEvent should return an event subscription of
	// TxPreEvent and send events to the given channel.
	SubscribeTxPreEvent(chan<- types.TxPreEvent) event.Subscription

	Get(thash common.Hash) *types.Transaction
}
