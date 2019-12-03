package protocol

import (
	"testing"

	"gitee.com/nerthus/nerthus/rlp"
	"github.com/stretchr/testify/require"
)

func TestNewTTLMsg(t *testing.T) {

	input := []byte("testinfodata")
	msg, err := NewTTLMsg(1, input)
	require.NoError(t, err)

	_, err = rlp.EncodeToBytes(msg)
	require.NoError(t, err)
}
