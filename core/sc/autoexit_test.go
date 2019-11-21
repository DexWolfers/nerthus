package sc

import (
	"math/big"
	"testing"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/params"

	"github.com/stretchr/testify/require"
)

func TestGetTimeoutCouncil(t *testing.T) {
	ctx, _ := newTestContext(t, params.SCAccount, params.TestChainConfig)

	query, err := ReadCouncilHeartbeat(ctx.State())
	require.NoError(t, err)

	list, _ := GetCouncilList(ctx.state)
	require.NotEmpty(t, list)

	h := query.Insert(list[0], 1, 10, 1, 1, 1)
	h.CommitCache()

	//如果此时高度已经超过时间

	round1 := uint64(params.ConfigParamsSCWitness * params.MaxHeartbeatRound) //人数

	invalid := GetTimeoutCouncil(ctx.state, round1)
	//在第一个轮次，不存在被踢
	require.Empty(t, invalid)

	//一旦超过第一个周期，则存在被踢
	invalid2 := GetTimeoutCouncil(ctx.state, round1+1)
	//在第一个轮次，不存在被踢
	require.NotEmpty(t, invalid2)

	//如果某人已经发生心跳，则不会被踢
	newNumber := round1 + 1
	query.Insert(list[3], 2, 10, 1, 1, 1).CommitCache()
	invalid3 := GetTimeoutCouncil(ctx.state, newNumber)
	require.NotEmpty(t, invalid2)
	require.NotContains(t, invalid3, list[3])

	t.Run("isTimeout", func(t *testing.T) {
		for _, c := range invalid3 {
			role, yes, err := IsTimeout(ctx.State(), c, newNumber)
			require.NoError(t, err)
			require.Equal(t, types.AccCouncil, role)
			require.True(t, yes)
		}
	})

	t.Run("remove", func(t *testing.T) {
		ctx.ctx.UnitNumber = new(big.Int).SetUint64(newNumber)
		ctx.ctx.Coinbase = common.StringToAddress("creator")
		contract := Contract{caller: ctx.ctx.Coinbase, gaspool: math.MaxBig256}
		err := handleRemoveTimeout(ctx, &contract, invalid3)
		require.NoError(t, err)

		//查看需要应该有成功移除
		t.Run("removed", func(t *testing.T) {
			for _, c := range invalid3 {
				active := IsActiveCouncil(ctx.State(), c)
				require.False(t, active)
			}
		})
		//如果再次移除，则会提示成功移除0个
		t.Run("again", func(t *testing.T) {
			err := handleRemoveTimeout(ctx, &contract, invalid3)
			require.EqualError(t, err, "no one removed")
		})
	})
}

func TestGetTimeoutSecondSystemWitness(t *testing.T) {
	ctx, _ := newTestContext(t, params.SCAccount, params.TestChainConfig)
	db := ctx.State()
	round1 := uint64(params.ConfigParamsSCWitness * params.MaxHeartbeatRound) //人数

	list, err := ReadSystemWitness(db)
	require.NoError(t, err)
	_, mainList, err := GetSystemMainWitness(db)
	require.NoError(t, err)
	secondList := list[len(mainList):]

	//所有人均超期
	newNumber := round1 + 1
	invalid, err := GetTimeoutSecondSystemWitness(db, newNumber)
	require.NoError(t, err)
	//所有备选均过期
	require.Equal(t, len(secondList), len(invalid))
	for _, v := range secondList {
		require.Contains(t, invalid, v)

		role, yes, err := IsTimeout(db, v, newNumber)
		require.NoError(t, err)
		require.Equal(t, role, types.AccSecondSystemWitness)
		require.True(t, yes)
	}

	//有三个备选发送心跳，此时过期则不应该包含他
	{
		query, err := ReadSystemHeartbeat(ctx.State())
		require.NoError(t, err)

		good := 3
		for i := 0; i < good; i++ {
			query.Insert(secondList[i], params.CalcRound(newNumber), newNumber, 1, 1, 1).CommitCache()
		}
		invalid2, err := GetTimeoutSecondSystemWitness(db, newNumber)
		require.NoError(t, err)
		t.Run("someInvalid", func(t *testing.T) {

			require.Equal(t, len(secondList)-good, len(invalid2))
			//后面三个超时
			for _, v := range secondList[good:] {
				require.Contains(t, invalid, v)
				role, yes, err := IsTimeout(db, v, newNumber)
				require.NoError(t, err)
				require.Equal(t, role, types.AccSecondSystemWitness)
				require.True(t, yes)
			}
		})

		//已发送心跳则，未超时
		t.Run("okItems", func(t *testing.T) {
			//尚未超时
			for _, v := range secondList[:good] {
				require.NotContains(t, invalid, v)

				role, yes, err := IsTimeout(db, v, newNumber)
				require.NoError(t, err)
				require.Equal(t, role, types.AccSecondSystemWitness)
				require.False(t, yes)
			}
		})

		//可以成功移除过时见证人
		t.Run("remove", func(t *testing.T) {
			ctx.ctx.UnitNumber = new(big.Int).SetUint64(newNumber)
			ctx.ctx.Coinbase = common.StringToAddress("creator")
			contract := Contract{caller: ctx.ctx.Coinbase, gaspool: math.MaxBig256}
			err := handleRemoveTimeout(ctx, &contract, invalid2)
			require.NoError(t, err)

			//查看需要应该有成功移除
			t.Run("removed", func(t *testing.T) {
				for _, c := range invalid2 {
					status, is := GetWitnessStatus(ctx.State(), c)
					require.True(t, is)
					require.Equal(t, WitnessLoggedOut, status)
				}
			})
			//如果再次移除，则会提示成功移除0个
			t.Run("again", func(t *testing.T) {
				err := handleRemoveTimeout(ctx, &contract, invalid2)
				require.EqualError(t, err, "no one removed")
			})
		})
	}
}
