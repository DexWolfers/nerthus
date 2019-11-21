package core

import (
	"math/big"
	"sync"
	"testing"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/consensus/bft"
)

func newTestRoundState(view *bft.View, validatorSet bft.ValidatorSet) *roundState {
	return &roundState{
		round:    view.Round,
		sequence: view.Sequence,
		//Preprepare: newTestPreprepare(view),
		Preprepare: &bft.Preprepare{
			View:     view,
			Proposal: newTestZeroProposal(),
		},
		Prepares: newMessageSet(validatorSet),
		Commits:  newMessageSet(validatorSet),
		lock:     new(sync.RWMutex),
		hasBadProposal: func(hash common.Hash) bool {
			return false
		},
	}
}

func TestLockHash(t *testing.T) {
	sys := NewTestSystemWithBackend(1, 0)
	rs := newTestRoundState(
		&bft.View{
			Round:    big.NewInt(0),
			Sequence: big.NewInt(0),
		},
		sys.backends[0].peers,
	)
	t.Log("locked hash", rs.polcRound.LockHash().String())
	if !common.EmptyHash(rs.polcRound.LockHash()) {
		t.Errorf("error mismatch: have %v, want empty", rs.polcRound.LockHash())
	}
	if rs.IsHashLocked() {
		t.Error("IsHashLocked should return false")
	}

	// Lock
	expected := rs.Proposal().Hash()
	rs.LockHash(nil, nil)
	if expected != rs.polcRound.LockHash() {
		t.Errorf("error mismatch: have %v, want %v", rs.polcRound.LockHash(), expected)
	}
	if !rs.IsHashLocked() {
		t.Error("IsHashLocked should return true")
	}

	// Unlock
	rs.UnlockHash()
	if !common.EmptyHash(rs.polcRound.LockHash()) {
		t.Errorf("error mismatch: have %v, want empty", rs.polcRound.LockHash())
	}
	if rs.IsHashLocked() {
		t.Error("IsHashLocked should return false")
	}
}

//func newTestRoundState(view *bft.View, validatorSet bft.ValidatorSet) *roundState {
//	return &roundState{
//		round:      view.Round,
//		sequence:   view.Sequence,
//		Preprepare: newTestPreprepare(view),
//		Prepares:   newMessageSet(validatorSet),
//		Commits:    newMessageSet(validatorSet),
//		mu:         new(sync.RWMutex),
//		hasBadProposal: func(hash common.Hash) bool {
//			return false
//		},
//	}
//}
//
//func TestLockHash(t *testing.T) {
//	sys := NewTestSystemWithBackend(1, 0)
//	rs := newTestRoundState(
//		&bft.View{
//			Round:    big.NewInt(0),
//			Sequence: big.NewInt(0),
//		},
//		sys.backends[0].peers,
//	)
//	t.Log("locked hash", rs.GetLockedHash().String())
//	if !common.EmptyHash(rs.GetLockedHash()) {
//		t.Errorf("error mismatch: have %v, want empty", rs.GetLockedHash())
//	}
//	if rs.IsHashLocked() {
//		t.Error("IsHashLocked should return false")
//	}
//
//	// Lock
//	expected := rs.Proposal().Hash()
//	rs.LockHash()
//	if expected != rs.GetLockedHash() {
//		t.Errorf("error mismatch: have %v, want %v", rs.GetLockedHash(), expected)
//	}
//	if !rs.IsHashLocked() {
//		t.Error("IsHashLocked should return true")
//	}
//
//	// Unlock
//	rs.UnlockHash()
//	if !common.EmptyHash(rs.GetLockedHash()) {
//		t.Errorf("error mismatch: have %v, want empty", rs.GetLockedHash())
//	}
//	if rs.IsHashLocked() {
//		t.Error("IsHashLocked should return false")
//	}
//}
