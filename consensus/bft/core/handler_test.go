package core

import (
	"context"
	"math/big"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/consensus/protocol"
	"gitee.com/nerthus/nerthus/core/types"
)

// notice: the normal case have been tested in integration tests.
func TestHandleMsg(t *testing.T) {
	N := uint64(4)
	F := uint64(1)
	sys := NewTestSystemWithBackend(N, F)

	closer := sys.Run(true)
	defer closer()

	v0 := sys.backends[0]
	r0 := v0.engine.(*core)

	m, _ := Encode(&bft.Subject{
		View: &bft.View{
			Sequence: big.NewInt(0),
			Round:    big.NewInt(0),
		},
		Digest: common.StringToHash("1234567890"),
	})
	// with a matched signBytes. MsgPreprepare should match with *istanbul.Preprepare in normal case.
	msg := &protocol.Message{
		Code:          protocol.MsgPreprepare,
		Msg:           m,
		Address:       v0.Address(),
		Sign:          types.NewSignContent(),
		CommittedSeal: &types.WitenssVote{},
	}

	bStart := time.Now()
	ctx := context.WithValue(context.Background(), "trace", handleTrace(func(key string) interface{} {
		switch key {
		case "cost":
			return time.Since(bStart)
		case "mid":
			return "0x001"
		default:
			return nil
		}
	}))

	vset, _ := v0.Validators(nil)
	_, val := vset.GetByAddress(v0.Address())
	if err := r0.handleCheckedMsg(ctx, msg, val); err != errFailedDecodePreprepare {
		t.Errorf("error mismatch: have %v, want %v", err, errFailedDecodePreprepare)
	}

	m, _ = Encode(&bft.Preprepare{
		View: &bft.View{
			Sequence: big.NewInt(0),
			Round:    big.NewInt(0),
		},
		Proposal: makeUnit(common.Address{}, common.Hash{}, common.Hash{}, 1),
	})
	// with a unmatched signBytes. MsgPrepare should match with *istanbul.Subject in normal case.
	msg = &protocol.Message{
		Code:          protocol.MsgPrepare,
		Msg:           m,
		Address:       v0.Address(),
		Sign:          types.NewSignContent(),
		CommittedSeal: &types.WitenssVote{},
	}

	_, val = vset.GetByAddress(v0.Address())
	if err := r0.handleCheckedMsg(ctx, msg, val); err != errFailedDecodePrepare {
		t.Errorf("error mismatch: have %v, want %v", err, errFailedDecodePreprepare)
	}

	m, _ = Encode(&bft.Preprepare{
		View: &bft.View{
			Sequence: big.NewInt(0),
			Round:    big.NewInt(0),
		},
		Proposal: makeUnit(common.Address{}, common.Hash{}, common.Hash{}, 2),
	})
	// with a unmatched signBytes. istanbul.MsgCommit should match with *istanbul.Subject in normal case.
	msg = &protocol.Message{
		Code:          protocol.MsgCommit,
		Msg:           m,
		Address:       v0.Address(),
		Sign:          types.NewSignContent(),
		CommittedSeal: &types.WitenssVote{},
	}

	_, val = vset.GetByAddress(v0.Address())
	if err := r0.handleCheckedMsg(ctx, msg, val); err != errFailedDecodeCommit {
		t.Errorf("error mismatch: have %v, want %v", err, errFailedDecodeCommit)
	}

	m, _ = Encode(&bft.Preprepare{
		View: &bft.View{
			Sequence: big.NewInt(0),
			Round:    big.NewInt(0),
		},
		Proposal: makeUnit(common.Address{}, common.Hash{}, common.Hash{}, 3),
	})
	// invalid message code. message code is not exists in list
	msg = &protocol.Message{
		Code:          protocol.ConsensusType(99),
		Msg:           m,
		Address:       v0.Address(),
		Sign:          types.NewSignContent(),
		CommittedSeal: &types.WitenssVote{},
	}

	_, val = vset.GetByAddress(v0.Address())
	if err := r0.handleCheckedMsg(ctx, msg, val); err == nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	// with malicious signBytes
	if err := r0.handleMsg([]byte{1}); err == nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
}
