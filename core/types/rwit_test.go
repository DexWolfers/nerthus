package types

import (
	"math/big"
	"testing"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/rlp"
	"github.com/stretchr/testify/require"
)

func TestChooseVote(t *testing.T) {

	vote := ChooseVote{
		Chain:       common.StringToAddress("chainA"),
		ApplyNumber: 1<<63 - 1,
		Choose: UnitID{
			ChainID: common.StringToAddress("chainA"),
			Height:  1<<63 - 1,
			Hash:    common.StringToHash("unitMax"),
		},
	}

	t.Run("sign", func(t *testing.T) {
		signder := NewSigner(big.NewInt(100))
		key, _ := crypto.GenerateKey()

		err := SignBySignHelper(&vote, signder, key)
		require.NoError(t, err)

		sender, err := Sender(signder, &vote)
		require.NoError(t, err)

		user := crypto.ForceParsePubKeyToAddress(key.PublicKey)
		require.Equal(t, user, sender)

		t.Run("rlp", func(t *testing.T) {

			b, err := rlp.EncodeToBytes(&vote)
			require.NoError(t, err)

			var got ChooseVote
			err = rlp.DecodeBytes(b, &got)
			require.NoError(t, err)
			require.Equal(t, vote, got)
		})

	})

}
