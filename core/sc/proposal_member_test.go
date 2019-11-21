package sc

import (
	"math/big"
	"testing"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"
	assert "github.com/stretchr/testify/require"
)

func TestCoin(t *testing.T) {
	//使用特定的sc地址
	sc := common.StringToAddress("systemChainAddr")
	old := params.SCAccount
	defer func() { params.SCAccount = old }()
	params.SCAccount = sc

	cfg := *params.TestnetChainConfig
	ctx, _ := newTestContext(t, sc, &cfg)
	ctx.ctx.UnitParent = common.StringToHash("test1")

	tu := common.StringToAddress("test user")

	ctx.ctx.Chain = &ChainContext{
		getUnitState: func(uhash common.Hash) (*state.StateDB, error) {
			if uhash == common.StringToHash("test1") {
				db, _ := ntsdb.NewMemDatabase()
				statedb, _ := state.New([]byte{1, 2, 3}, common.Hash{}, state.NewDatabase(db))
				statedb.SetBalance(tu, big.NewInt(10000))
				return statedb, nil
			}
			return nil, nil
		},
	}
	actionState, err := ctx.Ctx().Chain.GetUnitState(ctx.Ctx().UnitParent)
	assert.Nil(t, err)
	assert.NotNil(t, actionState)
	balance := actionState.GetBalance(tu)
	t.Log(balance)
}
