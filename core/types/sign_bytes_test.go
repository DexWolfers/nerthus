package types

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitee.com/nerthus/nerthus/rlp"
)

func TestSignBytes_Bytes(t *testing.T) {
	var sign = NewSignBytes([]byte{19})
	b, err := rlp.EncodeToBytes(sign)
	require.Nil(t, err)
	var sign2 = NewSignBytes(nil)
	rlp.DecodeBytes(b, sign2)
	require.Nil(t, err)
	require.NotEqual(t, sign2.data, sign.data)
}
