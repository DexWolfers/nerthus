package types

import (
	"testing"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/rlp"

	"github.com/stretchr/testify/require"
)

func TestSyncDagMsgRLP(t *testing.T) {

	data := SyncDagMsg{
		Type:         MODE_SINGLE_BEHIND,
		Address:      []common.Address{common.StringToAddress("addr1"), common.StringToAddress("addr2 ")},
		TargetHash:   common.StringToHash("target"),
		TargetNumber: 1234567890,
	}

	b, err := rlp.EncodeToBytes(data)
	require.NoError(t, err)

	var got SyncDagMsg
	err = rlp.DecodeBytes(b, &got)
	require.NoError(t, err)
	require.Equal(t, got, data)
}
