package sc

import (
	"fmt"
	"math/big"
	"strconv"
	"testing"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/params"
	"github.com/stretchr/testify/require"
)

func TestSetupDefaultWitnessForUser(t *testing.T) {
	cfg := *params.TestnetChainConfig

	ctx, _ := newTestContext(t, params.SCAccount, &cfg)

	defaultWitness := make([]common.Address, 11)
	// 准备用户见证人
	for i := 0; i < 11; i++ {
		defaultWitness[i] = common.StringToAddress(fmt.Sprintf("us_%d", i))
		require.NoError(t, applyFunc(ctx.state, defaultWitness[i], false))
	}

	var users []common.Address
	for i := 0; i < 2000; i++ {
		users = append(users, common.StringToAddress(fmt.Sprintf("user_%d", i)))
	}
	err := SetupDefaultWitnessForUser(ctx.state, users...)
	require.NoError(t, err)

	//检查用户见证人
	for _, u := range users {
		lib, err := GetChainWitness(ctx.state, u)
		require.NoError(t, err)
		for i := 0; i < len(defaultWitness); i++ {
			require.Equal(t, defaultWitness[i], lib[i], "should be default")
		}
	}
	//检查见证人的用户数
	for _, w := range defaultWitness {
		info, err := GetWitnessInfo(ctx.state, w)
		require.NoError(t, err)
		GetChainsAtGroup(ctx.State(), info.GroupIndex, func(index uint64, chain common.Address) bool {
			require.Equal(t, users[index], chain)
			return true
		})

	}
}

func TestSetupCouncilMember(t *testing.T) {
	statedb := newTestState()

	before := statedb.GetBalance(params.SCAccount)
	require.Equal(t, 0, before.Sign())

	members := make([]common.Address, 33)
	for i := 0; i < 33; i++ {
		members[i] = common.StringToAddress(strconv.Itoa(i))
		statedb.SetBalance(members[i], new(big.Int).SetUint64(params.ConfigParamsCouncilMargin*2))
	}
	err := SetupCouncilMember(statedb, members, 33)
	require.NoError(t, err)

	after := statedb.GetBalance(params.SCAccount)
	require.Equal(t, new(big.Int).SetUint64(params.ConfigParamsCouncilMargin*33), after)

	//每个理事均有保证金
	for _, w := range members {
		info, err := GetCouncilInfo(statedb, w)
		require.NoError(t, err)
		require.Equal(t, info.Margin, params.ConfigParamsCouncilMargin)
	}
}

func TestSetupSystemWitness(t *testing.T) {
	statedb := newTestState()

	before := statedb.GetBalance(params.SCAccount)
	require.Equal(t, 0, before.Sign())

	systemWitness := make([]List, 55)
	for i := 0; i < 55; i++ {
		systemWitness[i] = List{
			Address: common.StringToAddress(strconv.Itoa(i)),
			PubKey:  []byte{1},
		}
		statedb.SetBalance(systemWitness[i].Address, new(big.Int).SetUint64(params.ConfigParamsSCWitnessMinMargin))
	}
	err := SetupSystemWitness(statedb, systemWitness)
	require.NoError(t, err)

	after := statedb.GetBalance(params.SCAccount)
	margin := new(big.Int).SetUint64(params.ConfigParamsSCWitnessMinMargin)
	margin.Mul(margin, big.NewInt(55))
	require.Equal(t, margin, after)

	//每个系统见证人均有保证金
	for _, w := range systemWitness {
		info, err := GetWitnessInfo(statedb, w.Address)
		require.NoError(t, err)
		require.Equal(t, info.Margin, params.ConfigParamsSCWitnessMinMargin)
	}
}

func TestSetupUserWitness(t *testing.T) {
	statedb := newTestState()

	before := statedb.GetBalance(params.SCAccount)
	require.Equal(t, 0, before.Sign())

	witness := make([]List, 55)
	for i := 0; i < 55; i++ {
		witness[i] = List{
			Address: common.StringToAddress(strconv.Itoa(i)),
			PubKey:  []byte{1},
		}
		statedb.SetBalance(witness[i].Address, new(big.Int).SetUint64(params.ConfigParamsUCWitnessMargin))
	}
	err := SetupUserWitness(statedb, witness)
	require.NoError(t, err)

	after := statedb.GetBalance(params.SCAccount)
	margin := new(big.Int).SetUint64(params.ConfigParamsSCWitnessMinMargin)
	margin.Mul(margin, big.NewInt(55))
	require.Equal(t, margin, after)

	//每个见证人均有保证金
	for _, w := range witness {
		info, err := GetWitnessInfo(statedb, w.Address)
		require.NoError(t, err)
		require.Equal(t, info.Margin, params.ConfigParamsUCWitnessMargin)
	}
}
