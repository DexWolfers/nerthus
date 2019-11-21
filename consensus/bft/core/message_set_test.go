package core

import (
	"math/big"
	"testing"

	"gitee.com/nerthus/nerthus/rlp"
	"github.com/stretchr/testify/require"

	"gitee.com/nerthus/nerthus/common"

	"gitee.com/nerthus/nerthus/consensus/bft"
)

func TestMessageSetWitPreprepare(t *testing.T) {
	valSet := newTestValidatorSet(4)

	ms := newMessageSet(valSet)
	view := &bft.View{
		Round:    new(big.Int),
		Sequence: new(big.Int),
	}

	pp := &bft.Preprepare{
		View:     view,
		Proposal: makeUnit(common.StringToAddress("0123456789"), common.Hash{}, common.Hash{}, 1),
	}

	rawPP, err := rlp.EncodeToBytes(pp)
	require.Nil(t, err)
	msg := &message{
		Code:    MsgPreprepare,
		Msg:     rawPP,
		Address: valSet.GetProposer().Address(),
	}

	err = ms.Add(msg)
	require.Nil(t, err)

	err = ms.Add(msg)
	require.Nil(t, err)

	require.Equal(t, 1, ms.Size())
}

func TestMessageSetWithSubject(t *testing.T) {
	valSet := newTestValidatorSet(4)

	ms := newMessageSet(valSet)

	view := &bft.View{
		Round:    new(big.Int),
		Sequence: new(big.Int),
	}

	sub := &bft.Subject{
		View:   view,
		Digest: common.StringToHash("1234567890"),
	}

	rawSub, err := rlp.EncodeToBytes(sub)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	msg := &message{
		Code:    MsgPrepare,
		Msg:     rawSub,
		Address: valSet.GetProposer().Address(),
	}

	err = ms.Add(msg)
	require.Nil(t, err)

	err = ms.Add(msg)
	require.Nil(t, err)

	require.Equal(t, 1, ms.Size())
}
