package bft

import (
	"fmt"
	"io"
	"math/big"
	"time"

	"gitee.com/nerthus/nerthus/core/state"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/rlp"
)

// Proposal supports retrieving height and serialized block to be used during Istanbul consensus.
type Proposal interface {
	// Number retrieves the sequence number of this proposal.
	Number() uint64

	// Hash retrieves the hash of this proposal.
	Hash() common.Hash

	EncodeRLP(w io.Writer) error

	DecodeRLP(s *rlp.Stream) error

	String() string
}

//
type Request struct {
	//Proposal Proposal
	Proposal *MiningResult
}

//go:generate enumer -type=BftEventCode -json -transform=snake

type BftEventCode uint8

const (
	NewNumber BftEventCode = iota
	NewProposal
	NewTx
	NewMessage
	NewChangeRound
)

type MiningResult struct {
	CreateAt time.Time
	Unit     Proposal
	Receipts types.Receipts
	Logs     []*types.Log
	StateDB  *state.StateDB
}

func (this *MiningResult) Hash() common.Hash {
	return this.Unit.Hash()
}
func (this *MiningResult) Number() uint64 {
	return this.Unit.Number()
}

// View includes a round number and a sequence number.
// Sequence is the block number we'd like to commit.
// Each round has a number and is composed by 3 steps: preprepare, prepare and commit.
//
// If the given block is not accepted by validators, a round change will occur
// and the validators start a new round with round+1.
type View struct {
	Round    *big.Int
	Sequence *big.Int
}

// EncodeRLP serializes b into the RLP format.
func (v *View) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{v.Round, v.Sequence})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (v *View) DecodeRLP(s *rlp.Stream) error {
	var view struct {
		Round    *big.Int
		Sequence *big.Int
	}

	if err := s.Decode(&view); err != nil {
		return err
	}
	v.Round, v.Sequence = view.Round, view.Sequence
	return nil
}

func (v *View) String() string {
	if v == nil {
		return fmt.Sprintf("{Round: nil, Sequence: nil}")
	}
	return fmt.Sprintf("{Round: %d, Sequence: %d}", v.Round.Uint64(), v.Sequence.Uint64())
}

// Cmp compares v and y and returns:
//   -1 if v <  y
//    0 if v == y
//   +1 if v >  y
func (v *View) Cmp(y *View) int {
	if c := v.Sequence.Cmp(y.Sequence); c != 0 {
		return c
	}
	if c := v.Round.Cmp(y.Round); c != 0 {
		return c
	}
	return 0
}

type Preprepare struct {
	View     *View
	Proposal Proposal      `rlp:"nil"`
	Mining   *MiningResult `rlp:"nil"`
}

// EncodeRLP serializes b into the RLP format.
func (b *Preprepare) EncodeRLP(w io.Writer) (err error) {
	var prorlp []byte
	if b.Proposal != nil {
		prorlp, err = rlp.EncodeToBytes(b.Proposal)
		if err != nil {
			return err
		}
	}
	return rlp.Encode(w, []interface{}{b.View, prorlp})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (b *Preprepare) DecodeRLP(s *rlp.Stream) error {
	var preprepare struct {
		View     *View
		Proposal []byte `rlp:"nil"`
	}

	if err := s.Decode(&preprepare); err != nil {
		return err
	}
	if len(preprepare.Proposal) > 0 {
		var unit *types.Unit
		if err := rlp.DecodeBytes(preprepare.Proposal, &unit); err != nil {
			return err
		}
		b.Proposal = unit
	}
	b.View = preprepare.View

	return nil
}

type Subject struct {
	View   *View
	Digest common.Hash // 提案的Hash
	Extra  []byte      // 额外字段
}

func (b *Subject) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{b.View, b.Digest, b.Extra})
}

// DecodeRLP implements rlp.Decoder, and load the consensus fields from a RLP stream.
func (b *Subject) DecodeRLP(s *rlp.Stream) error {
	var subject struct {
		View   *View
		Digest common.Hash
		Extra  []byte
	}

	if err := s.Decode(&subject); err != nil {
		return err
	}
	b.View, b.Digest, b.Extra = subject.View, subject.Digest, subject.Extra
	return nil
}

// not include `Extra`, it just extra attribute
func (b *Subject) Cmp(other *Subject) bool {
	return b.View.Cmp(other.View) == 0 && b.Digest == other.Digest
}

func (b *Subject) String() string {
	return fmt.Sprintf("{View: %v, Digest: %v, %v}", b.View.String(), b.Digest.String(), b.Extra)
}

type ConsensusInfo struct {
	Node      common.Address `json:"node"`
	ChainAddr common.Address `json:"chain_address"`
	View      *View          `json:"view"`
	Wait      bool           `json:"wait"`
	Timer     bool           `json:"timer"`
	State     struct {
		ConsensusState string      `json:"consensus_state"`
		Req            common.Hash `json:"request"`
		Preprepare     common.Hash `json:"pre_prepare"`
		Prepares       int         `json:"prepare"`
		Commits        int         `json:"commits"`
		LockedHash     common.Hash `json:"locked_hash"`
	}
	LastHeader *types.Header `json:"last_header"`
}

type ProofTreeMessage struct {
	Code          uint8
	Msg           []byte
	Address       common.Address
	Extra         []byte             `rlp:"nil"`
	Sign          *types.SignContent `rlp:"nil"`
	CommittedSeal *types.WitenssVote `rlp:"nil"`
}

// Impl sign interface
func (pm *ProofTreeMessage) GetSignStruct() []interface{} {
	var packet = []interface{}{
		pm.Code,
		pm.Msg,
		pm.Address,
		pm.Extra,
	}
	if pm.CommittedSeal == nil {
		packet = append(packet, types.NewSignContent())
	} else {
		packet = append(packet, pm.CommittedSeal)
	}

	return packet
}

func (pm *ProofTreeMessage) GetSignStructBytes() []byte {
	buf, err := rlp.EncodeToBytes(pm.GetSignStruct())
	if err != nil {
		PanicSanity(err)
	}
	return buf
}

func (pm *ProofTreeMessage) GetRSVBySignatureHash() []byte {
	return pm.Sign.Get()
}

func (pm *ProofTreeMessage) Hash() common.Hash {
	return common.SafeHash256(pm.GetSignStruct())
}

func (pm *ProofTreeMessage) SetSign(sign []byte) {
	pm.Sign.Set(sign)
}

// ==============================================
//
// define the functions that needs to be provided for core.
func (pm *ProofTreeMessage) FromPayload(b []byte) error {
	// Decode message
	var err error

	if err = rlp.DecodeBytes(b, pm); err != nil {
		return err
	}
	// Still return the message even the err is not nil
	return err
}

func (pm *ProofTreeMessage) Sender(sign types.Signer) (common.Address, error) {
	return types.Sender(sign, pm)
}

func (pm *ProofTreeMessage) Payload() ([]byte, error) {
	return rlp.EncodeToBytes(pm)
}

func (pm *ProofTreeMessage) PayloadNoSig() ([]byte, error) {
	return rlp.EncodeToBytes(&ProofTreeMessage{
		Code:          pm.Code,
		Msg:           pm.Msg,
		Address:       pm.Address,
		Extra:         pm.Extra,
		CommittedSeal: pm.CommittedSeal,
		Sign:          nil,
	})
}

func (pm *ProofTreeMessage) Decode(val interface{}) error {
	return rlp.DecodeBytes(pm.Msg, val)
}

func (pm *ProofTreeMessage) Trace() string {
	return fmt.Sprintf("{code: %v, from: %v, extra_size:%v}", pm.Code, pm.Address.ShortString(), len(pm.Extra))
}

func (pm *ProofTreeMessage) String() string {
	return fmt.Sprintf("{code:%v, from:%v}", pm.Code, pm.Address.ShortString())
}
