package core

import (
	"math/big"
	"testing"

	"gitee.com/nerthus/nerthus/consensus/protocol"

	"github.com/stretchr/testify/require"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/consensus/bft"
)

func TestRoundChangeSet(t *testing.T) {
	vset := bft.NewSet(1, common.Hash{}, generateValidators(4))
	rc := newRoundChangeSet(vset, nil)

	view := &bft.View{
		Sequence: big.NewInt(1),
		Round:    big.NewInt(1),
	}
	r := &bft.Subject{
		View:   view,
		Digest: common.Hash{},
	}
	m, _ := Encode(r)

	// Test Add()
	// Add message from all validators
	for i, v := range vset.List() {
		msg := &protocol.Message{
			Code:    protocol.MsgRoundCh,
			Msg:     m,
			Address: v.Address(),
		}
		rc.Add(view.Round, msg)
		require.Equal(t, i+1, rc.roundChanges[view.Round.Uint64()].Size(),
			"the size of round change messages mismatch: have %v, want %v", rc.roundChanges[view.Round.Uint64()].Size(), i+1)
	}

	// Add message with again from all validators, but the size should be the same
	for _, v := range vset.List() {
		msg := &protocol.Message{
			Code:    protocol.MsgRoundCh,
			Msg:     m,
			Address: v.Address(),
		}
		rc.Add(view.Round, msg)
		require.Equal(t, rc.roundChanges[view.Round.Uint64()].Size(), vset.Size(),
			"the size of round change messages mismatch: have %v, want %v", rc.roundChanges[view.Round.Uint64()].Size(), vset.Size())
	}

	// Test MaxRound
	for i := 0; i < 10; i++ {
		maxRound := rc.MaxRound(i)
		if i <= vset.Size() {
			if maxRound == nil || maxRound.Cmp(view.Round) != 0 {
				t.Fatalf("max round mismatch: have %v, want %v", maxRound, view.Round)
			}
		} else if maxRound != nil {
			t.Fatalf("max round mismatch: have %v, want nil", maxRound)
		}
	}

	// Test Clear()
	for i := int64(0); i < 2; i++ {
		rc.Clear(big.NewInt(i))
		require.Equal(t, rc.roundChanges[view.Round.Uint64()].Size(), vset.Size(),
			"the size of round change message mismatch: have %v, want %v", rc.roundChanges[view.Round.Uint64()].Size(), vset.Size())
	}
	rc.Clear(big.NewInt(2))
	require.Nil(t, rc.roundChanges[view.Round.Uint64()], "the change messages mismatch: have %v, want nil", rc.roundChanges[view.Round.Uint64()])
}
