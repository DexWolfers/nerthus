package arbitration

import (
	"encoding/binary"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/consensus/protocol"
	"gitee.com/nerthus/nerthus/core/types"
)

type Backend interface {
	GenerateProposal(scTail *types.Header) (Proposal, error)
	GetCouncils() ([]common.Address, error)
	Commit(pro Proposal, seal []types.WitenssVote) error
	ScTail() *types.Header
	GetUnitByHash(hash common.Hash) *types.Unit
	Sign(sign types.SignHelper) error
	VerifySign(sign types.SignHelper) (common.Address, error)
	Broadcast(msg *CouncilMessage)
}

type CouncilMessage struct {
	From      common.Address
	ScTail    uint64
	Dist      common.Hash
	Status    State
	Sign      types.SignContent `rlp:"nil"`
	TimeStamp uint64
}

func (t *CouncilMessage) GetSignStruct() []interface{} {
	return []interface{}{t.Dist, t.Extra()}
}
func (t *CouncilMessage) GetRSVBySignatureHash() []byte {
	return t.Sign.Get()
}
func (t *CouncilMessage) SetSign(b []byte) {
	t.Sign.Set(b)
}
func (t *CouncilMessage) Extra() []byte {
	var buffer = make([]byte, types.ExtraSealSize)
	buffer[0] = byte(protocol.MsgCommit)
	binary.LittleEndian.PutUint64(buffer[1:types.ExtraSealSize], t.ScTail)
	return buffer
}

type CouncilMsgEvent struct {
	Message *CouncilMessage
}

type Proposal interface {
	Hash() common.Hash
}

type State uint8

const (
	StateRunning State = iota
	StateStop
	StatePrepare
	StateCommited
)
