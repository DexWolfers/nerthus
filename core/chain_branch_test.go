package core

import (
	"testing"

	"gitee.com/nerthus/nerthus/common"
	witvote "gitee.com/nerthus/nerthus/consensus/bft/backend"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/ntsdb"
	"github.com/stretchr/testify/require"
)

func TestDagChain_SubscribeReportUnitEvent(t *testing.T) {

	db, _ := ntsdb.NewMemDatabase()
	gspec, err := LoadGenesisConfig()
	require.NoError(t, err)
	gspec.MustCommit(db)

	dagchain, _ := NewDagChain(db, gspec.Config, witvote.NewFaker(), vm.Config{})
	defer dagchain.Stop()

	// 测试订阅接收事情
	data := types.FoundBadWitnessEvent{
		Bad: types.Header{MC: common.StringToAddress("test")},
	}

	// 订阅
	ch := make(chan types.FoundBadWitnessEvent, 1)
	dagchain.SubscribeReportUnitEvent(ch)

	// 发送
	dagchain.PostChainEvents([]interface{}{data}, nil)

	// 读取
	select {
	case got := <-ch:
		require.Equal(t, data, got)
	default:
		require.Fail(t, "expect got data")
	}

}
