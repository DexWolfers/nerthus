package core

import (
	"fmt"
	"io"
	"math/big"
	"sync"

	"gitee.com/nerthus/nerthus/params"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/rlp"
)

func newRoundState(view *bft.View, validatorSet bft.ValidatorSet, chaindAddress common.Address, polcRound *polcRound, preprepare *bft.Preprepare,
	pendingRequest *bft.Request) *roundState {
	return &roundState{
		chainAddress:   chaindAddress,
		round:          view.Round,
		sequence:       view.Sequence,
		Preprepare:     preprepare,
		Prepares:       newMessageSet(validatorSet),
		Commits:        newMessageSet(validatorSet),
		polcRound:      polcRound,
		lock:           new(sync.RWMutex),
		pendingRequest: pendingRequest,
	}
}

// 存储共识状态
type roundState struct {
	chainAddress   common.Address  // For trace
	round          *big.Int        // 轮次，从0开始
	sequence       *big.Int        // 序列号，即区块的高度
	Preprepare     *bft.Preprepare // 预备阶段
	Prepares       *messageSet     // 准备阶段
	Commits        *messageSet     // 提交阶段
	polcRound      *polcRound
	pendingRequest *bft.Request

	lock *sync.RWMutex
}

type polcRound struct {
	view       *bft.View
	lockedHash common.Hash
	signBytes  [][]byte
}

func (p *polcRound) View() *bft.View {
	if p == nil {
		return nil
	}
	return &bft.View{Sequence: new(big.Int).Set(p.view.Sequence), Round: new(big.Int).Set(p.view.Round)}
}

func (p *polcRound) LockHash() common.Hash {
	if p == nil {
		return common.Hash{}
	}
	return p.lockedHash
}

func (p *polcRound) SignBytes() [][]byte {
	if p == nil {
		return nil
	}
	return p.signBytes
}

func (p *polcRound) String() string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprintf("lockHash:%v, lockSeq:%v, lockRound:%v", p.LockHash().TerminalString(), p.view.Sequence.Uint64(), p.view.Round.Uint64())
}

func (s *roundState) GetPrepareOrCommitSize() int {
	s.lock.RLock()
	result := s.Prepares.Size() + s.Commits.Size()
	// 去重
	for _, m := range s.Prepares.Values() {
		if s.Commits.Get(m.Address) != nil {
			result--
		}
	}
	s.lock.RUnlock()
	return result
}

func (s *roundState) GetPrepareOrCommitBytes() [][]byte {
	s.lock.RLock()
	var (
		minVoteCount = params.GetChainMinVoting(s.chainAddress)
		flag         = make(map[common.Address]struct{})
		buf          = make([][]byte, 0, minVoteCount)
	)
	for _, msg := range s.Prepares.Values() {
		if len(buf) >= minVoteCount {
			break
		}
		if _, ok := flag[msg.Address]; ok {
			continue
		}
		flag[msg.Address] = struct{}{}
		buf = append(buf, msg.PacketProveTreeBytes())
	}
	for _, msg := range s.Commits.Values() {
		if len(buf) >= minVoteCount {
			break
		}
		if _, ok := flag[msg.Address]; ok {
			continue
		}
		flag[msg.Address] = struct{}{}
		buf = append(buf, msg.PacketProveTreeBytes())
	}
	s.lock.RUnlock()
	return buf
}

func (s *roundState) Subject() *bft.Subject {
	s.lock.RLock()

	round, seq := big.NewInt(0).Set(s.round), big.NewInt(0).Set(s.sequence)

	var proHash common.Hash
	if s.Preprepare != nil && s.Preprepare.Proposal != nil {
		proHash = s.Preprepare.Proposal.Hash()
	}
	s.lock.RUnlock()

	return &bft.Subject{
		View: &bft.View{
			Round:    round,
			Sequence: seq,
		},
		Digest: proHash,
	}
}

func (s *roundState) SetPreprepare(preprepare *bft.Preprepare) {
	//if s.Preprepare != nil && s.Preprepare.Mining != nil {
	//	s.Preprepare.Mining.StateDB.Clear()
	//}
	s.lock.Lock()
	s.Preprepare = preprepare
	s.lock.Unlock()
}

func (s *roundState) Proposal() (proposal bft.Proposal) {
	s.lock.RLock()
	if s.Preprepare != nil {
		proposal = s.Preprepare.Proposal
	}
	s.lock.RUnlock()
	return proposal
}

// 设置轮次
func (s *roundState) SetRound(r *big.Int) {
	s.lock.Lock()
	s.round = new(big.Int).Set(r)
	s.lock.Unlock()
}

func (s *roundState) Round() (round *big.Int) {
	s.lock.RLock()
	round = s.round
	s.lock.RUnlock()
	return
}

// 设置序列号，即高度
func (s *roundState) SetSequence(seq *big.Int) {
	s.lock.Lock()
	s.sequence = seq
	s.lock.Unlock()
}

func (s *roundState) Sequence() (seq *big.Int) {
	s.lock.RLock()
	seq = s.sequence
	s.lock.RUnlock()
	return
}

// view is the new lock's view, it should greater than local lock
func (s *roundState) LockHash(view *bft.View, payload [][]byte) {
	if view == nil {
		bft.PanicSanity(view)
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.Preprepare != nil {
		var oldLockSeq, oldLockRound *big.Int
		if s.polcRound != nil {
			oldLockSeq, oldLockRound = s.polcRound.view.Sequence, s.polcRound.view.Round
		}
		//if s.polcRound != nil && s.polcRound.View().Cmp(view) >= 0 {
		//	log.Warn("Trigger lock mechanism fail, new view should greater than old view",
		//		"old-lock-seq", oldLockSeq, "old-lock-round", oldLockRound, "new-lock-seq", view.Sequence, "new-lock-round", view.Round)
		//	return
		//}
		pRNumber, prepNumber := uint64(0), s.Preprepare.Proposal.Number()
		if s.pendingRequest != nil {
			pRNumber = s.pendingRequest.Proposal.Number()
		}
		log.Debug("Trigger lock mechanism successfully", "cAddr", s.chainAddress.ShortString(), "oldLock", s.polcRound.LockHash(), "new_lock", s.Preprepare.Proposal.Hash(),
			"old-lock-seq", oldLockSeq, "old-lock-round", oldLockRound, "new-lock-seq", view.Sequence, "new-lock-round", view.Round,
			"n1", pRNumber, "n2", prepNumber, "pSize", s.Prepares.Size(), "cSize", s.Commits.Size(), "payload-size", len(payload))
		s.polcRound = &polcRound{
			lockedHash: s.Preprepare.Proposal.Hash(),
			view:       view,
			signBytes:  payload}
	}
}

// 解锁
func (s *roundState) UnlockHash() {
	s.lock.Lock()
	s.polcRound = nil
	s.lock.Unlock()
}

// 检查是否已锁定
func (s *roundState) IsHashLocked() (ok bool) {
	s.lock.RLock()
	ok = !common.EmptyHash(s.polcRound.LockHash())
	s.lock.RUnlock()
	return
}

func (s *roundState) GetPolcRound() (polcRound *polcRound) {
	s.lock.RLock()
	polcRound = s.polcRound
	s.lock.RUnlock()
	return
}

// rlp序列化
func (s *roundState) DecodeRLP(stream *rlp.Stream) error {
	var ss struct {
		Round          *big.Int
		Sequence       *big.Int
		Preprepare     *bft.Preprepare
		Prepares       *messageSet
		Commits        *messageSet
		PolcRound      *polcRound
		PendingRequest *bft.Request
	}

	if err := stream.Decode(&ss); err != nil {
		return err
	}
	s.round = ss.Round
	s.sequence = ss.Sequence
	s.Preprepare = ss.Preprepare
	s.Prepares = ss.Prepares
	s.Commits = ss.Commits
	s.polcRound = ss.PolcRound
	s.pendingRequest = ss.PendingRequest
	s.lock = new(sync.RWMutex)

	return nil
}

// rlp 反序列化
func (s *roundState) EncodeRLP(w io.Writer) error {
	s.lock.RLock()
	defer s.lock.RUnlock()

	return rlp.Encode(w, []interface{}{
		s.chainAddress,
		s.round,
		s.sequence,
		s.Preprepare,
		s.Prepares,
		s.Commits,
		s.polcRound,
		s.pendingRequest,
	})
}

func (s *roundState) String(state State, wait bool) string {
	if s == nil {
		return fmt.Sprintf("{csstate=%v, wait=%v, other=Nil}", state, wait)
	}
	var (
		pSize, cSize       = 0, 0
		seq, round         = "", ""
		pendingRequestInfo = "nil"
		lockHash           = s.GetPolcRound().LockHash().TerminalString()
		isLocked           = s.IsHashLocked()
	)
	if s.Prepares != nil {
		pSize = s.Prepares.Size()
	}
	if s.Commits != nil {
		cSize = s.Commits.Size()
	}
	if s.sequence != nil {
		seq = s.sequence.String()
	}
	if s.round != nil {
		round = s.round.String()
	}
	if s.pendingRequest != nil {
		pendingRequestInfo = fmt.Sprintf("{proposalHash:%v,proposalNumber:%v}", s.pendingRequest.Proposal.Hash().TerminalString(), s.pendingRequest.Proposal.Number())
	}
	return fmt.Sprintf("csstate=%v, wait=%v, pSize=%v,cSize=%v,seq=%v,round=%v,pendingR=%v,isLock=%v,lockHash=%v", state, wait, pSize, cSize, seq, round, pendingRequestInfo, isLocked, lockHash)
}

func (s *roundState) Report(state State, wait bool) []interface{} {
	ctx := []interface{}{
		"csstate", state,
		"wait", wait}
	if s == nil {
		ctx = append(ctx, "other", nil)
		return ctx
	}
	if s.Prepares != nil {
		ctx = append(ctx, "pSize", s.Prepares.Size())
	} else {
		ctx = append(ctx, "pSize", 0)
	}
	if s.Commits != nil {
		ctx = append(ctx, "cSize", s.Commits.Size())
	} else {
		ctx = append(ctx, "cSize", 0)
	}
	if s.sequence != nil {
		ctx = append(ctx, "seq", s.sequence)
	} else {
		ctx = append(ctx, "seq", 0)
	}
	if s.round != nil {
		ctx = append(ctx, "round", s.round)
	} else {
		ctx = append(ctx, "round", 0)
	}

	if s.Proposal() != nil {
		ctx = append(ctx,
			"proposal.hash", s.Proposal().Hash(),
			"proposal.number", s.Proposal().Number())
	} else {
		ctx = append(ctx, "proposal", nil)
	}
	ctx = append(ctx, "isLock", s.IsHashLocked(), "lockHash", s.GetPolcRound().LockHash())
	return ctx
}
