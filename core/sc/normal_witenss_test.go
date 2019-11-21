package sc

import (
	"errors"
	"math/big"
	"strconv"

	"testing"

	"fmt"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func applyFunc(stateDB vm.StateDB, address common.Address, isSys bool) error {
	var (
		margin   uint64
		maxLimit uint64
	)
	if isSys {
		margin, maxLimit = params.ConfigParamsUCWitnessMargin, 0
	} else {
		margin, maxLimit = params.ConfigParamsUCWitnessMargin, params.TestnetChainConfig.Get(params.ConfigParamsUCWitnessCap)
	}
	_, err := applyWitness(stateDB, address, margin, maxLimit, []byte("123"), isSys)
	return err
}

func TestCalcPercentMoney(t *testing.T) {
	caces := []struct {
		Money   uint64
		Percent float64
		Want    uint64
	}{
		{21000000000000000, 0.15, 17850000000000000},
		{21000000000000000, 0.85, 3.15e15},
		{11000000000000000, 0.8533, 1.6137e15},
	}
	for _, c := range caces {
		v := calcPercentMoney(c.Money, c.Percent)
		require.Equal(t, c.Want, v, "test %d * %f ", c.Money, c.Percent)
	}
}

func TestUcWitnessGroup(t *testing.T) {
	stateDB := newTestState()
	var addrList common.AddressList
	for i := 0; i < params.ConfigParamsUCWitness*2; i++ {
		addr := common.StringToAddress("addr" + string(i))
		addrList = append(addrList, addr)
		require.Nil(t, applyFunc(stateDB, addr, false))
	}

	for _, v := range addrList {
		info, err := GetWitnessInfo(stateDB, v)
		require.Nil(t, err)
		groupIndex, err := GetWitnessGroupIndex(stateDB, info.Address)
		require.Nil(t, err)
		count := GetWitnessCountAt(stateDB, groupIndex)
		require.Equal(t, params.ConfigParamsUCWitness, int(count), "group %d should be full", groupIndex)
		list := GetWitnessListAt(stateDB, groupIndex)
		require.True(t, common.AddressList(list).Have(v))
	}
}

func TestUpdateAcitvieWitenssLib(t *testing.T) {
	statedb := newTestState()
	var sum uint64
	margin := params.ConfigParamsSCWitnessMinMargin
	// 添加系统见证人
	for i := 0; i < params.ConfigParamsSCWitness; i++ {
		sum++
		addr := common.StringToAddress("system" + string(i))
		assert.Nil(t, applyFunc(statedb, addr, true))
	}
	margin = params.ConfigParamsUCWitnessMargin
	var ucLimit uint64
	//添加用户见证人
	for i := 0; i < params.ConfigParamsUCWitness*2; i++ {
		sum++
		ucLimit++
		addr := common.BytesToAddress([]byte{byte(i + 1)})
		_, err := applyWitness(statedb, addr, margin, 0, []byte("1234"), false)
		require.Nil(t, err)
	}
	// 数量应该是见证人和：
	count := GetActiveWitnessCount(statedb)
	assert.Equal(t, sum, count, "the number of active witness should be equal created")
	//能正常获取到所有人
	list, err := GetActivWitnessLib(statedb)
	require.NoError(t, err)
	require.Len(t, list, int(sum))

	t.Run("cancelWitness", func(t *testing.T) {
		needCount := uint64(len(list))
		for _, w := range list {
			needCount--

			w.Status = WitnessLoggedOut
			w.CancelHeight = 100
			err := onWitnessExit(statedb, 100, &w)
			require.NoError(t, err)

			//此时有效见证人库总人数减少
			count := GetActiveWitnessCount(statedb)
			require.Equal(t, needCount, count)

			list, err := GetActivWitnessLib(statedb)
			require.NoError(t, err)
			require.Len(t, list, int(needCount))
		}

	})
}

func TestSystemWitnessCount(t *testing.T) {
	statedb := newTestState()
	count := params.ConfigParamsSCWitnessCap
	for i := 0; i < params.ConfigParamsSCWitnessCap; i++ {
		addr := common.StringToAddress(strconv.Itoa(i) + "system")
		err := applyFunc(statedb, addr, true)
		require.Nil(t, err)
	}
	require.Equal(t, count, int(GetWitnessCountAt(statedb, DefSysGroupIndex)))
}

func TestReplaceWitness(t *testing.T) {
	statedb := newTestState()
	context := vm.Context{
		UnitNumber: big.NewInt(0),
		UnitMC:     params.SCAccount,
		Transfer: func(db vm.StateDB, addresses common.Address, addresses2 common.Address, i *big.Int) {
			t.Log("transfer", "addr1", addresses.Hex(), "addr2", addresses2.Hex(), "margin", i.Uint64())
		},
	}

	// 初始化系统见证人
	for i := 0; i < params.ConfigParamsSCWitness; i++ {
		addr := common.StringToAddress(strconv.Itoa(i) + "system")
		err := applyFunc(statedb, addr, true)
		require.Nil(t, err)
	}

	for i := 0; i < 10; i++ {
		AddCouncil(statedb, &Council{
			Address: common.StringToAddress(strconv.Itoa(i) + "member"),
			Status:  CouncilStatusValid,
			Margin:  params.ConfigParamsCouncilMargin,
		}, 40)
	}

	ctx := TestContext{state: statedb, ctx: context, cfg: params.TestnetChainConfig}
	ucNum := params.ConfigParamsUCWitness * 5
	// 初始化见证人
	for i := 0; i < ucNum*2; i++ {
		addr := common.BytesToAddress([]byte{byte(i + 1)})
		err := applyFunc(statedb, addr, false)
		require.Nil(t, err)
	}

	execFunc := func(addr common.Address, afterCount int) error {
		//	申请见证人
		contract := Contract{caller: addr, gaspool: math.MaxBig256}
		_, err := replaceWitness(&ctx, &contract, contract.Caller(), false)

		if err != nil {
			return err
		}
		//成功后，获取链的见证人
		lib, err := GetChainWitness(statedb, addr)
		if err != nil {
			return err
		}
		// 数量需要同配置一致
		if lib.Len() != params.ConfigParamsUCWitness {
			return fmt.Errorf("the number of chain witness should be equal witness config value, len=%v, %v", lib.Len(), params.ConfigParamsUCWitness)
		}
		//且每个见证人的用户数需要增加
		for _, w := range lib {
			info, err := GetWitnessInfo(statedb, w)
			if err != nil {
				return err
			}
			items := make(map[common.Address]struct{})
			GetChainsAtGroup(statedb, info.GroupIndex, func(index uint64, chain common.Address) bool {
				items[chain] = struct{}{}
				return true
			})
			if afterCount != len(items) {
				return errors.New("the number of users under the witness should increase by 1")
			}
			if _, ok := items[addr]; !ok {
				return errors.New("the users under the witness should contain current user")
			}
		}
		return nil
	}

	t.Run("replace witness again", func(t *testing.T) {
		//	//	申请见证人
		addr := common.StringToAddress(fmt.Sprintf("testaccount_1"))
		err := execFunc(addr, 1)
		require.Nil(t, err)
		err = execFunc(addr, 1)
		require.NotNil(t, err)
		ctx.ctx.UnitNumber = new(big.Int).SetUint64(ctx.Ctx().UnitNumber.Uint64() + params.ReplaceWitnessInterval + 1)
		err = execFunc(addr, 1)
		require.Nil(t, err)
		chainStatus, number := GetChainStatus(ctx.State(), addr)
		require.Equal(t, chainStatus, ChainStatusWitnessReplaceUnderway)
		require.Equal(t, number, ctx.ctx.UnitNumber.Uint64())
		err = execFunc(addr, 1)
		require.NotNil(t, err)
		ctx.ctx.UnitNumber = new(big.Int).SetUint64(ctx.Ctx().UnitNumber.Uint64() + 100)
		err = execFunc(addr, 1)
		require.Nil(t, err)
		chainStatus, number = GetChainStatus(ctx.State(), addr)
		require.Equal(t, chainStatus, ChainStatusWitnessReplaceUnderway)
		require.Equal(t, number, ctx.ctx.UnitNumber.Uint64())
	})

	t.Run("remove system witness", func(t *testing.T) {
		sysWitmess, err := ReadSystemWitness(statedb)
		require.Nil(t, err)
		require.Equal(t, params.ConfigParamsSCWitness, len(sysWitmess))
		removeSysWitness := sysWitmess[0]

		info, err := GetWitnessInfo(statedb, removeSysWitness)
		require.Nil(t, err)

		t.Log("remove system witness", "addr", removeSysWitness.Hex(), "margin", info.Margin)
		contract := Contract{caller: removeSysWitness, gaspool: math.MaxBig256, value: new(big.Int)}
		err = removeChainWitness(&ctx, &contract, removeSysWitness, false)
		require.Nil(t, err)
	})

}

func TestCancelWitness(t *testing.T) {
	ctx, _ := newTestContext(t, common.StringToAddress("root"), params.TestChainConfig)
	addr := common.StringToAddress("addr1")
	require.Nil(t, applyFunc(ctx.State(), addr, false))
	require.Equal(t, GetActiveWitnessCount(ctx.State()), uint64(1))
	contract := Contract{caller: addr, gaspool: math.MaxBig256, value: math.MaxBig256}
	_, err := CancelWitness(ctx, &contract)
	require.Nil(t, err)
	info, err := GetWitnessInfo(ctx.State(), addr)
	require.Nil(t, err)
	require.Equal(t, WitnessLoggedOut, info.Status)
	//require.Equal(t, GetWitnessSumCount(ctx.State()), uint64(len(sysList))+1)
	require.Equal(t, GetActiveWitnessCount(ctx.State()), uint64(0))
}

func newTestState() *state.StateDB {
	db, _ := ntsdb.NewMemDatabase()

	statedb, _ := state.New([]byte{1, 2, 3}, common.Hash{}, state.NewDatabase(db))
	return statedb
}

//
//func getAddressStrList(lib []WitnessInfo) []string {
//	s := make([]string, len(lib))
//	for i, v := range lib {
//		s[i] = v.Address.Hex()
//	}
//	return s
//}
//
type Contract struct {
	caller  common.Address
	gaspool *big.Int
	value   *big.Int
}

func (c *Contract) Caller() common.Address {
	return c.caller
}
func (c *Contract) UseGas(gas uint64) (ok bool) {
	use := new(big.Int).SetUint64(gas)
	if c.gaspool.Cmp(use) < 0 {
		return false
	}
	c.gaspool.Sub(c.gaspool, use)
	return true
}
func (c *Contract) Address() common.Address {
	return c.caller
}
func (c *Contract) Value() *big.Int {
	return c.value
}

type TestContext struct {
	state *state.StateDB
	ctx   vm.Context
	cfg   *params.ChainConfig
}

func (c *TestContext) Ctx() vm.Context {
	return c.ctx
}
func (c *TestContext) State() vm.StateDB {
	return c.state
}

// ChainConfig returns the environment's chain configuration
func (c *TestContext) ChainConfig() *params.ChainConfig {
	return c.cfg
}

func (c *TestContext) Cancel() {}

func (c *TestContext) Create(caller vm.ContractRef, code []byte, gas uint64, value *big.Int) (ret []byte, contractAddr common.Address, leftOverGas uint64, err error) {
	panic("Not yet implemented")
}

func (c *TestContext) Call(caller vm.ContractRef, addr common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error) {
	panic("Not yet implemented")
}

func (c *TestContext) CallCode(caller vm.ContractRef, addr common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error) {
	panic("Not yet implemented")
}

func (c *TestContext) DelegateCall(caller vm.ContractRef, addr common.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error) {
	panic("Not yet implemented")
}

func (c *TestContext) StaticCall(caller vm.ContractRef, addr common.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error) {
	panic("Not yet implemented")
}
func (c *TestContext) VMConfig() vm.Config {
	return vm.Config{}
}

type ChainContext struct {
	getWitenssVoteCount func(witenss common.Address) uint64
	getNeedVoteCount    func(address common.Address) uint64
	getUnitState        func(uhash common.Hash) (*state.StateDB, error)
}

func (ctx *ChainContext) GetStateByNumber(chain common.Address, number uint64) (*state.StateDB, error) {
	panic("implement me")
}

func (ctx *ChainContext) GetHeaderByHash(hash common.Hash) *types.Header {
	return nil
}
func (ctx *ChainContext) GetHeader(hash common.Hash) *types.Header {
	return nil
}

func (ctx *ChainContext) ValidateUnit(unit *types.Unit) error {
	return nil
}

func (ctx *ChainContext) GetBalance(address common.Address) *big.Int {
	return common.Big0
}

func (ctx *ChainContext) GetAvailableWorkFee(vm.StateDB, common.Address, uint64) (*big.Int, *big.Int, error) {
	return common.Big0, common.Big0, nil
}

func (ctx *ChainContext) GetTxExecCountByWitenss(address common.Address) (*big.Int, error) {
	return common.Big0, nil
}

func (ctx *ChainContext) GetUserTxCount(address common.Address) (*big.Int, error) {
	return common.Big0, nil
}

func (ctx *ChainContext) GetStatisticalMustVoting(witness common.Address) *big.Int {
	//return big.NewInt(0)
	return big.NewInt(100).Sub(big.NewInt(100), witness.Big())
}

func (ctx *ChainContext) GetWitnessWorkReport(period uint64) map[common.Address]types.WitnessPeriodWorkInfo {
	return nil
}
func (ctx *ChainContext) GetWitnessReport(witness common.Address, period uint64) types.WitnessPeriodWorkInfo {
	return types.WitnessPeriodWorkInfo{}
}

// GetWitenssVotes 获取指定见证人参与的投票数
func (ctx *ChainContext) GetWitenssVoteCount(witenss common.Address) *big.Int {
	//return big.NewInt(0), nil
	return witenss.Big()
}

func (ctx *ChainContext) GetUnitState(hash common.Hash) (*state.StateDB, error) {
	if ctx.getUnitState != nil {
		return ctx.getUnitState(hash)
	}
	return nil, nil
}
func (ctx *ChainContext) GenesisHash() common.Hash {
	return common.Hash{}
}

func (ctx *ChainContext) GetChainTailHash(chain common.Address) common.Hash {
	return common.Hash{}
}

//
//func TestGenerateWitness(t *testing.T) {
//	db, _ := ntsdb.NewMemDatabase()
//
//	chain := ChainContext{}
//	statedb, _ := state.New([]byte{1,2,3},common.Hash{}, state.NewDatabase(db))
//	context := vm.Context{
//		UnitNumber: big.NewInt(1),
//		UnitMC:     params.GenesisUnitMCAccount,
//		Chain:      &chain,
//	}
//	ctx := TestContext{state: statedb, ctx: context, cfg: params.TestnetChainConfig}
//
//	statedb.SetState(params.SCAccount, common.BytesToHash([]byte("genesis")), common.HexToHash("0x1243251246514"))
//
//	var ws []string
//	// 初始化见证人
//	for i := 0; i < 15; i++ {
//		addr := common.BytesToAddress([]byte{byte(i + 1)})
//		margin := new(big.Int).SetUint64(params.ConfigParamsUCWitnessMargin)
//
//		statedb.SetBalance(addr, margin)
//		contract := Contract{caller: addr, gaspool: math.MaxBig256, value: margin}
//		ws = append(ws, contract.caller.Hex())
//
//		_, err := ApplyWitness(&ctx, &contract, contract.Value())
//
//		assert.NoError(t, err)
//		if t.Failed() {
//			return
//		}
//	}
//
//	addr := common.StringToAddress(fmt.Sprintf("testaccount_%d", 1))
//
//	replaceWitnessExec := func(addr common.Address) []common.Address {
//		contract := Contract{caller: addr, gaspool: math.MaxBig256}
//		ws = append(ws, contract.caller.Hex())
//		_, err := ReplaceWitness(&ctx, &contract, nil)
//		if err != nil {
//			t.Fatalf("fail to replace witness:%v", err)
//		}
//
//		lib, err := GetChainWitness(statedb, addr)
//		if err != nil {
//			t.Fatalf("fail to get chain witness:%v", err)
//		}
//
//		for _, v := range lib {
//			t.Log("=>", v.String())
//		}
//		t.Log("----------")
//		return lib
//
//	}
//
//	expected := [][]common.Address{
//		{
//			common.BytesToAddress([]byte{byte(1)}),
//			common.BytesToAddress([]byte{byte(2)}),
//			common.BytesToAddress([]byte{byte(3)}),
//			common.BytesToAddress([]byte{byte(4)}),
//			common.BytesToAddress([]byte{byte(5)}),
//			common.BytesToAddress([]byte{byte(6)}),
//			common.BytesToAddress([]byte{byte(7)}),
//		},
//		{
//			common.BytesToAddress([]byte{byte(8)}),
//			common.BytesToAddress([]byte{byte(9)}),
//			common.BytesToAddress([]byte{byte(10)}),
//			common.BytesToAddress([]byte{byte(11)}),
//			common.BytesToAddress([]byte{byte(12)}),
//			common.BytesToAddress([]byte{byte(13)}),
//			common.BytesToAddress([]byte{byte(14)}),
//		},
//		{
//			common.BytesToAddress([]byte{byte(15)}),
//			common.BytesToAddress([]byte{byte(1)}),
//			common.BytesToAddress([]byte{byte(2)}),
//			common.BytesToAddress([]byte{byte(3)}),
//			common.BytesToAddress([]byte{byte(4)}),
//			common.BytesToAddress([]byte{byte(5)}),
//			common.BytesToAddress([]byte{byte(6)}),
//		},
//		{
//			common.BytesToAddress([]byte{byte(7)}),
//			common.BytesToAddress([]byte{byte(8)}),
//			common.BytesToAddress([]byte{byte(9)}),
//			common.BytesToAddress([]byte{byte(10)}),
//			common.BytesToAddress([]byte{byte(11)}),
//			common.BytesToAddress([]byte{byte(12)}),
//			common.BytesToAddress([]byte{byte(13)}),
//		},
//	}
//
//	//compare := func(expect []common.Address, actual []common.Address) bool {
//	//	if len(expect) != len(actual) {
//	//		t.Fatalf("expect result length is not match")
//	//	}
//	//	num := len(expect)
//	//	for i := 0; i < num; i++ {
//	//		if expect[i] != actual[i] {
//	//			return false
//	//		}
//	//	}
//	//	return true
//	//}
//
//	for _, result := range expected {
//		getWits := replaceWitnessExec(addr)
//		require.Equal(t, result, getWits)
//	}
//}

func TestWitnessUserInfo(t *testing.T) {

	db, _ := ntsdb.NewMemDatabase()
	chain := ChainContext{}
	statedb, _ := state.New([]byte{1, 2, 3}, common.Hash{}, state.NewDatabase(db))
	context := vm.Context{
		UnitNumber: big.NewInt(1),
		UnitMC:     params.SCAccount,
		Chain:      &chain,
	}
	ctx := TestContext{state: statedb, ctx: context, cfg: params.TestnetChainConfig}

	margin := new(big.Int).SetUint64(params.ConfigParamsUCWitnessMargin)

	statedb.SetState(params.SCAccount, common.BytesToHash([]byte("genesis")), common.HexToHash("0x1243251246514"))
	var ws []string
	// 初始化见证人
	num := params.ConfigParamsUCWitness + 2
	for i := 0; i < num; i++ {
		addr := common.BytesToAddress([]byte{byte(i + 1)})

		statedb.SetBalance(addr, margin)
		ws = append(ws, addr.Hex())

		err := applyFunc(statedb, addr, false)

		assert.NoError(t, err)
		if t.Failed() {
			return
		}
		//check load
		info, err := GetWitnessInfo(statedb, addr)
		require.NoError(t, err)
		require.Equal(t, WitnessNormal, info.Status)
		require.Equal(t, margin.Uint64(), info.Margin)

		got, err := GetWitnessInfo(statedb, info.Address)
		require.Nil(t, err)
		require.Equal(t, addr, got.Address)
	}

	replaceWitnessExec := func(addr common.Address) []common.Address {
		contract := Contract{caller: addr, gaspool: math.MaxBig256}
		ws = append(ws, contract.caller.Hex())
		_, err := createWitness(&ctx, &contract, common.Address{}, nil)
		require.Nil(t, err)

		lib, err := GetChainWitness(statedb, addr)
		require.Nil(t, err)
		require.Len(t, lib, int(params.ConfigParamsUCWitness))

		// in list
		for _, w := range lib {
			require.True(t, w.Big().Int64() <= int64(num), "w=%s,v=%d", w.Hex(), w.Big())
		}

		//replace will failed
		_, err = replaceWitness(&ctx, &contract, contract.Caller(), false)
		require.NotNil(t, err)
		return lib
	}

	var testaddr []common.Address
	for i := 1; i <= 5; i++ {
		testaddr = append(testaddr, common.StringToAddress(fmt.Sprintf("%d", i*100+i)))
	}

	for _, addr := range testaddr {
		replaceWitnessExec(addr)
	}

	_, err := GetActivWitnessLib(statedb)
	require.Nil(t, err)

}

func TestGetReplaseWitnessTraget(t *testing.T) {

	contanctAddr := common.GenerateContractAddress(common.StringToAddress("addr02"), 0x5050, 100)

	caces := []struct {
		sender common.Address
		input  []byte
		want   interface{}
	}{
		{sender: common.StringToAddress("addr01"), input: nil, want: common.StringToAddress("addr01")},
		{sender: common.StringToAddress("addr02"), input: CreateCallInputData(FuncReplaceWitnessID, contanctAddr), want: contanctAddr},

		{sender: common.StringToAddress("addr03"), input: []byte{1, 2, 3, 4, 5}, want: errors.New("")},
		{sender: common.StringToAddress("addr03"), input: append([]byte{1, 2, 3, 4}, contanctAddr.Bytes()...), want: errors.New("")},
		{sender: common.StringToAddress("addr03"), input: CreateCallInputData(FuncReplaceWitnessID, common.StringToAddress("addr04")), want: errors.New("")},
	}

	for i, c := range caces {
		traget, err := GetReplaseWitnessTraget(c.sender, c.input)
		switch c.want.(type) {
		case error:
			require.Error(t, err, "should be got error at %d", i)
		default:
			require.NoError(t, err)
			require.Equal(t, c.want, traget, "should be ok at %d", i)
		}
	}
}

// 测试场景：测试付费更换见证人
func TestHandleApplyWitnessWay1(t *testing.T) {

	ctx, _ := newTestContext(t, params.SCAccount, params.TestnetChainConfig)

	//先准备三组用户见证人
	var us []List
	for i := 0; i <= params.ConfigParamsUCWitness*3; i++ {
		addr := common.StringToAddress(fmt.Sprintf("user_witness_%d", i))
		ctx.State().AddBalance(addr, new(big.Int).SetUint64(params.ConfigParamsCouncilMargin))

		us = append(us, List{addr, []byte{1}})
	}
	err := SetupUserWitness(ctx.State(), us)
	require.NoError(t, err)

	contract := Contract{caller: common.StringToAddress("caller"), gaspool: big.NewInt(1000000000), value: big.NewInt(0)}

	//开始
	t.Run("invalid", func(t *testing.T) {
		caces := []common.Address{
			common.Address{},
			contract.caller,
			params.SCAccount,
		}
		for _, v := range caces {
			err := HandleApplyWitnessWay2(ctx, &contract, v)
			require.Error(t, err)
		}
	})

	t.Run("newAddress", func(t *testing.T) {
		t.Log(GetWitnessGroupCount(ctx.state))

		chain := common.StringToAddress("chainA")
		err := HandleApplyWitnessWay2(ctx, &contract, chain)
		require.NoError(t, err)

		//应有11个见证人
		list, err := GetChainWitness(ctx.State(), chain)
		require.NoError(t, err)
		require.Len(t, list, params.ConfigParamsUCWitness)

		// 测试：如果再次申请，则会失败，因为已有见证人
		t.Run("failed when have", func(t *testing.T) {
			err := HandleApplyWitnessWay2(ctx, &contract, chain)
			require.Error(t, err)
			t.Log(err)
		})
		//测试：一部分见证人注销，最终因为只有8个见证人时，可以成功申请
		t.Run("ok when not enough", func(t *testing.T) {
			//注销4人，剩余11-4 =7 时，均不能正常事情
			remove := 1 + params.ConfigParamsUCWitness - int(params.ConfigParamsUCMinVotings)
			t.Log(remove)
			for _, w := range list {
				err = removeWitness(ctx, w, false, false)

				nowList, err := GetChainWitness(ctx.State(), chain)
				require.NoError(t, err)

				if len(nowList) < int(params.ConfigParamsUCMinVotings) {
					break
				}
				//依旧失败
				err = HandleApplyWitnessWay2(ctx, &contract, chain)
				require.Error(t, err)
			}
			//此时再申请，则因为人数不足而成功申请
			err = HandleApplyWitnessWay2(ctx, &contract, chain)
			require.NoError(t, err)
			s, _ := GetChainStatus(ctx.State(), chain)
			//链状态为正在更换见证人中
			require.Equal(t, ChainStatusWitnessReplaceUnderway, s)
		})
	})
}

func TestRangeActiveWitness(t *testing.T) {
	ctx, sysWitness := newTestContext(t, params.SCAccount, params.TestnetChainConfig)

	//先准备三组用户见证人
	var us []List
	for i := 0; i <= params.ConfigParamsUCWitness*3; i++ {
		addr := common.StringToAddress(fmt.Sprintf("user_witness_%d", i))
		ctx.State().AddBalance(addr, new(big.Int).SetUint64(params.ConfigParamsCouncilMargin))
		us = append(us, List{addr, []byte{1}})
	}
	err := SetupUserWitness(ctx.State(), us)
	require.NoError(t, err)

	//获取所有
	allActiveWitness := make(map[common.Address]bool)
	RangeActiveWitness(ctx.State(), func(witness common.Address) bool {
		require.False(t, allActiveWitness[witness])
		allActiveWitness[witness] = true
		return true
	})
	//总数应该等于所有人
	require.Len(t, allActiveWitness, len(sysWitness)+len(us))

	//一直注销，但每次均可以正确
	for w := range allActiveWitness {
		contract := Contract{caller: w, gaspool: big.NewInt(1000000000), value: big.NewInt(0)}

		_, err := CancelWitness(ctx, &contract)
		require.NoError(t, err)

		delete(allActiveWitness, w)

		gots := make(map[common.Address]bool)
		RangeActiveWitness(ctx.State(), func(witness common.Address) bool {
			require.False(t, gots[witness])
			gots[witness] = true
			return true
		})
		require.Len(t, gots, len(allActiveWitness))
		//人也必须正确
		for w2 := range gots {
			require.Contains(t, allActiveWitness, w2)
		}

	}

}
