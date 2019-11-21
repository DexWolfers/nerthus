package sc

import (
	"math/big"
	"testing"

	"gitee.com/nerthus/nerthus/core/types"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/params"
	"github.com/stretchr/testify/require"
)

func TestMemberHeartbeat(t *testing.T) {

	for i := 0; i < 3; i++ {
		ctx, _ := newTestContext(t, params.SCAccount, params.TestnetChainConfig)
		ctx.ctx.UnitNumber = big.NewInt(15)

		members := []common.Address{
			common.StringToAddress("m1"),
			common.StringToAddress("m2"),
		}

		for _, m := range members {
			ctx.state.AddBalance(m, math.MaxBig63)
		}

		err := SetupCouncilMember(ctx.state, members, 11)
		require.NoError(t, err)

		for i := 0; i < 8; i++ {
			for _, m := range members {
				contract := Contract{caller: m, gaspool: math.MaxBig256, value: big.NewInt(0)}

				_, err = MemberHeartbeat(ctx, &contract)
				require.NoError(t, err)
			}
		}
		root, _ := ctx.state.IntermediateRoot(common.Address{})
		t.Log(root.Hex())
	}

}

// 测试场景：测试在写入心跳数据后，是否能正确读取到轮次信息
func TestQueryHeartbeatLastRound(t *testing.T) {

	ctx, _ := newTestContext(t, params.SCAccount, params.TestChainConfig)

	caces := []struct {
		role  types.AccountRole
		addr  common.Address
		round uint64
	}{
		{types.AccCouncil, common.StringToAddress("c1"), 10},
		{types.AccCouncil, common.StringToAddress("c2"), 11},
		{types.AccMainSystemWitness, common.StringToAddress("sys1"), 20},
		{types.AccMainSystemWitness, common.StringToAddress("sys2"), 25},
	}

	for _, c := range caces {
		switch c.role {
		case types.AccCouncil:
			query, err := ReadCouncilHeartbeat(ctx.State())
			require.NoError(t, err)
			query.Insert(c.addr, c.round, 10, 1, 1, 1).CommitCache()
		case types.AccMainSystemWitness:
			query, err := ReadSystemHeartbeat(ctx.State())
			require.NoError(t, err)
			query.Insert(c.addr, c.round, 10, 1, 1, 1).CommitCache()
		}
	}
	//再取值判断
	for _, c := range caces {
		got := QueryHeartbeatLastRound(ctx.State(), c.role, c.addr)
		require.Equal(t, int(c.round), int(got))
	}
}
