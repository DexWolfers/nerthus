package bytime

import (
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/nts/synchronize/message"
	"gitee.com/nerthus/nerthus/nts/synchronize/status"
)

type ChainWriteRead interface {
}

type MainSync interface {
	SendMessage(msg *message.Message, peers ...message.PeerID) error
	FetchUnitByID(uid types.UnitID) error
	PeerStatusSet() *status.PeerStatusSet
	HandleFetchUnitResponse(msg *message.Message) (success int, uints []*types.Unit, err error)
}
