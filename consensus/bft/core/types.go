package core

import (
	"math/big"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/objectpool"
	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/consensus/protocol"
	"gitee.com/nerthus/nerthus/rlp"
)

type EngineOption func(engine Engine)

// 共识核心引擎接口
type Engine interface {
	objectpool.Object
	CurrentState() (RunState, bft.ConsensusInfo)
	CurrentProposal() *bft.MiningResult
	SetTimer(timer Timer)
	IsActive() bool
	// 消息插队
	PriorHandle(msg []interface{}) bool
	RollbackPackedTx(newUnit common.Hash)
}

// 超时器接口
type Timer interface {
	SetTimeout(view *RoundView)
	StopTimer(mc common.Address)
}
type RoundView struct {
	Chain    common.Address
	Round    *big.Int
	Sequence *big.Int
	DeadLine time.Time
	SendTime time.Time
}

//共识运行状态

//go:generate enumer -type=RunState -json -transform=snake

type RunState uint8

const (
	RunStateNormal RunState = iota
	//提案被错误清理掉了
	RunStateNilRequest
	//Sequence没有正常变更
	RunStateOldSequence
	//RoundTooBig轮次太高
	RunRoundTooBig
)

//go:generate enumer -type=State -json -transform=snake

type State uint64

const (
	StateAcceptRequest State = iota
	StatePreprepared
	StatePrepared
	StateCommitted
)

// Cmp compares s and y and returns:
//   -1 if s is the previous state of y
//    0 if s and y are the same state
//   +1 if s is the next state of y
func (s State) Cmp(y State) int {
	if uint64(s) < uint64(y) {
		return -1
	}
	if uint64(s) > uint64(y) {
		return 1
	}
	return 0
}

type MockMessage = protocol.Message

// ==============================================
//
// helper functions

func Encode(val interface{}) ([]byte, error) {
	return rlp.EncodeToBytes(val)
}
