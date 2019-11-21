package sc

import (
	"fmt"
	"math/big"
	"testing"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/testutil"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"
	"github.com/stretchr/testify/require"
)

func TestMath(t *testing.T) {
	// test divideCeil
	{
		require.Equal(t, uint64(1), divideCeil(1, 4))
		require.Equal(t, uint64(1), divideCeil(4, 4))

		require.Equal(t, uint64(2), divideCeil(5, 4))
	}

	// test divideFloor
	{
		require.Equal(t, uint64(0), divideFloor(1, 4))
		require.Equal(t, uint64(0), divideFloor(3, 4))

		require.Equal(t, uint64(1), divideFloor(5, 4))
		require.Equal(t, uint64(1), divideFloor(6, 4))

		require.Equal(t, uint64(2), divideFloor(8, 4))
		require.Equal(t, uint64(2), divideFloor(9, 4))
	}

	// test divideNumber
	{
		x, y := divideNumber(1, 4)
		require.Equal(t, uint64(0), x)
		require.Equal(t, uint64(1), y)

		x, y = divideNumber(6, 4)
		require.Equal(t, uint64(1), x)
		require.Equal(t, uint64(2), y)
	}
}

// 测试增发比例和增发额度是否符合预期，这里需要预设总发行量为100亿NTS
func TestIssueRatio(t *testing.T) {

	var everyYear = []struct {
		ratio float64
		issue float64
	}{
		{0.02500, 250000000},
		{0.02500, 250000000},
		{0.02500, 250000000},
		{0.02500, 250000000},
		{0.01250, 125000000},
		{0.01250, 125000000},
		{0.01250, 125000000},
		{0.01250, 125000000},
		{0.00630, 63000000},
		{0.00630, 63000000},
		{0.00630, 63000000},
		{0.00630, 63000000},
		{0.00310, 31000000},
		{0.00310, 31000000},
		{0.00310, 31000000},
		{0.00310, 31000000},
		{0.00160, 16000000},
		{0.00160, 16000000},
		{0.00160, 16000000},
		{0.00160, 16000000},
		{0.00080, 8000000},
		{0.00080, 8000000},
		{0.00080, 8000000},
		{0.00080, 8000000},
		{0.00050, 5000000},
		{0.00050, 5000000},
		{0.00050, 5000000},
		{0.00050, 5000000},
		{0.00050, 5000000},
		{0.00050, 5000000},
		{0.00050, 5000000},
		{0.00050, 5000000},
		{0.00050, 5000000},
		{0.00050, 5000000},
	}

	for year := 1; year <= len(everyYear); year++ {
		data := everyYear[year-1]
		got := calcIssueRatio(uint64(year))
		issue := additionalCalcFormula(uint64(year))

		require.Equal(t, data.ratio, got, "get error at year %d", year)
		require.Equal(t, data.issue, issue, "get error at year %d", year)
	}
}

func TestGetSettlePeriod(t *testing.T) {
	caces := []struct {
		number uint64
		period uint64
	}{
		{0, 1},
		{1, 1},
		{2, 1},
		{params.ConfigParamsBillingPeriod - 1, 1},
		{params.ConfigParamsBillingPeriod, 1},
		{params.ConfigParamsBillingPeriod + 1, 2},
		{params.ConfigParamsBillingPeriod*2 - 1, 2},
		{params.ConfigParamsBillingPeriod * 2, 2},
		{params.ConfigParamsBillingPeriod*2 + 1, 3},
	}
	for _, c := range caces {
		got := GetSettlePeriod(c.number)
		require.Equal(t, c.period, got, "not equal at number %d,one period units=%d", c.number, params.ConfigParamsBillingPeriod)
	}
}

func TestDrawAllAdditionalFee(t *testing.T) {
	testutil.SetupTestLogging()

	t.Run("无工作无奖励，其他工作者瓜分所有", func(t *testing.T) {
		caces := []struct {
			typ     int // 1=sys,2=user,3=理事
			account common.Address
			margin  uint64
		}{
			{1, common.BytesToAddress([]byte{1}), 5},
			{1, common.BytesToAddress([]byte{2}), 10},
			{1, common.BytesToAddress([]byte{3}), 20},

			{2, common.BytesToAddress([]byte{4}), 5},
			{2, common.BytesToAddress([]byte{5}), 5},
			{2, common.BytesToAddress([]byte{6}), 10},
			{2, common.BytesToAddress([]byte{7}), 15},

			{3, common.BytesToAddress([]byte{8}), 10},
			{3, common.BytesToAddress([]byte{9}), 10},
			{3, common.BytesToAddress([]byte{10}), 10},
		}

		state := newTestState()
		chainContext := &testChainContext{nil, 0, nil}

		for _, c := range caces {
			switch c.typ {
			case 1, 2:
				_, err := applyWitness(state, c.account, c.margin, 1, []byte("pk"), c.typ == 1)
				require.NoError(t, err)
			case 3:
				err := AddCouncil(state, &Council{Address: c.account, Status: CouncilStatusValid, Margin: c.margin, ApplyHeight: 1}, 100)
				require.NoError(t, err)

			}
		}

		var (
			number     = uint64(params.ConfigParamsBillingPeriod*2) - 1
			workPeriod = uint64(1)
			cotr       = newDefaultAddController(state, chainContext, number)
		)

		//设置只有少数3个人有工作，他们可以瓜分
		workedAddress := map[common.Address]bool{
			caces[0].account: true,
			caces[5].account: true,
			caces[9].account: true,
		}

		cotr.testOnlyAadditional = true
		for i, rule := range cotr.rules {
			if rule.role != interestAdditionalType {
				cotr.rules[i].calcProofWork = func(address common.Address, period uint64) (float64, error) {
					if workedAddress[address] {
						return 1, nil
					}
					//其他人工作为0
					return 0, nil
				}
				cotr.rules[i].proofRateFunc = func(address common.Address, period uint64) (float64, error) {
					if workedAddress[address] {
						return 0.1, nil
					}
					//其他人工作为0
					return 0, nil
				}
			}
		}

		//一个周期的总费用中一定比例（60%）用于支付利息
		onPeriod := additionalEveryPeriod(workPeriod) * interestRate
		var totalMargin uint64
		//只有干活的人参与分成
		for _, c := range caces {
			if workedAddress[c.account] {
				totalMargin += c.margin
			}
		}

		for _, c := range caces {
			balance, err := cotr.calcPeriodBalance(c.account, workPeriod)
			require.NoError(t, err)
			if !workedAddress[c.account] {
				require.Zero(t, balance)
			} else {
				marginP := float64(c.margin) / float64(totalMargin)
				require.Equal(t, onPeriod*marginP, balance, "margin=%f,total=%f", marginP, onPeriod)
			}
		}
	})

	t.Run("利息直接根据保证金比例分配", func(t *testing.T) {
		caces := []struct {
			typ     int // 1=sys,2=user,3=理事
			account common.Address
			margin  uint64
		}{
			{1, common.BytesToAddress([]byte{1}), 5},
			{1, common.BytesToAddress([]byte{2}), 10},
			{1, common.BytesToAddress([]byte{3}), 20},

			{2, common.BytesToAddress([]byte{4}), 5},
			{2, common.BytesToAddress([]byte{5}), 5},
			{2, common.BytesToAddress([]byte{6}), 10},
			{2, common.BytesToAddress([]byte{7}), 15},

			{3, common.BytesToAddress([]byte{8}), 10},
			{3, common.BytesToAddress([]byte{9}), 10},
			{3, common.BytesToAddress([]byte{10}), 10},
		}

		state := newTestState()
		chainContext := &testChainContext{nil, 0, nil}

		for _, c := range caces {
			switch c.typ {
			case 1, 2:
				_, err := applyWitness(state, c.account, c.margin, 1, []byte("pk"), c.typ == 1)
				require.NoError(t, err)
			case 3:
				err := AddCouncil(state, &Council{Address: c.account, Status: CouncilStatusValid, Margin: c.margin, ApplyHeight: 1}, 100)
				require.NoError(t, err)

			}
		}

		var (
			number     = uint64(params.ConfigParamsBillingPeriod*2) - 1
			workPeriod = uint64(1)
			cotr       = newDefaultAddController(state, chainContext, number)
		)

		//禁用其他，便于测试
		cotr.testOnlyAadditional = true

		var callInterest *additionalRole

		for i, rule := range cotr.rules {
			if rule.role == interestAdditionalType {
				callInterest = rule
			}
			//预设所有人均有工作
			if rule.role != interestAdditionalType {
				cotr.rules[i].calcProofWork = func(address common.Address, period uint64) (float64, error) {
					return 1, nil
				}
			}
		}

		//一个周期的总费用中一定比例（60%）用于支付利息
		onPeriod := additionalEveryPeriod(workPeriod) * interestRate
		var totalMargin uint64
		for _, c := range caces {
			totalMargin += c.margin
		}

		for _, c := range caces {
			marginP := float64(c.margin) / float64(totalMargin)
			balance, err := callInterest.calcBalance(c.account, workPeriod, additionalEveryPeriod(workPeriod), 0)
			require.NoError(t, err)
			require.Equal(t, onPeriod*marginP, balance, "margin=%f,total=%f", marginP, onPeriod)
		}

	})

}

func TestWorkFee(t *testing.T) {
	// 取现系统链高度
	number := uint64(88)
	periodUcount := uint64(100)

	testutil.SetupTestLogging()
	state := newTestState()
	var chainContext vm.ChainContext
	// 准备账户
	witList := make([]testApplyInfo, 0)
	for i := 0; i < 3; i++ {
		for r := 1; r < 4; r++ {
			var t string
			switch r {
			case 1:
				t = "sys"
			case 2:
				t = "user"
			case 3:
				t = "council"
			}
			name := fmt.Sprintf("%s_%d", t, i)
			witList = append(witList, testApplyInfo{
				name: name,
				addr: common.StringToAddress(name),
				role: r, money: 10000, height: uint64(i), hearts: map[uint64]uint64{
					1: 3, 2: 4, 3: 3, 4: 3,
				},
			})
		}
	}
	// 心跳准备
	sys, err := ReadSystemHeartbeat(state)
	require.NoError(t, err)
	counci, err := ReadCouncilHeartbeat(state)
	require.NoError(t, err)
	addHeart := func(addr common.Address, data map[uint64]uint64, beat ListHeartbeat) {
		for k := range data {
			for i := uint64(0); i < data[k]; i++ {
				beat.AddUnitCount(addr, 0, 0, k)
			}
		}
	}
	// 申请见证人、理事
	for _, acc := range witList {
		var err error
		switch acc.role {
		case 1:
			_, err = applyWitness(state, acc.addr, acc.money, acc.height, []byte("pk"), true)
			addHeart(acc.addr, acc.hearts, sys)
		case 2:
			_, err = applyWitness(state, acc.addr, acc.money, acc.height, []byte("pk"), false)
			chainContext = &testChainContext{witList, periodUcount, nil}
		case 3:
			AddCouncil(state, &Council{
				Address: acc.addr, Status: CouncilStatusValid, Margin: acc.money, ApplyHeight: acc.height,
			}, 100)
			addHeart(acc.addr, acc.hearts, counci)
		}
		require.NoError(t, err)
	}
	sys.CommitCache()
	counci.CommitCache()

	calcResult := make(map[common.Address]*big.Int)
	//准备工作结束，计算
	period := GetSettlePeriod(number)
	var sys_count, user_count, council_count float64
	sys_wo := make(map[uint64]uint64)
	coun_wo := make(map[uint64]uint64)
	money_c := make(map[uint64]uint64)
	for _, wit := range witList {
		amount, freeze, err := GetAdditionalBalance(wit.addr, number, state, chainContext)
		require.NoError(t, err)
		//fmt.Printf("name: %s\t amount: %v\t freeze: %v\n", wit.name, amount, freeze)
		//log.Info("code result----", "name", wit.name, "addr", wit.addr, "amount", amount, "freeze", freeze)
		calcResult[wit.addr] = new(big.Int).Add(amount, freeze)
		// calc work
		switch wit.role {
		case 1:
			sys_count++
			for k := range wit.hearts {
				v, _ := sys_wo[k]
				sys_wo[k] = v + wit.hearts[k]
			}
		case 2:
			user_count++
		case 3:
			council_count++
			for k := range wit.hearts {
				v, _ := coun_wo[k]
				coun_wo[k] = v + wit.hearts[k]
			}
		}
		// 押金
		for p := uint64(1); p <= period; p++ {
			applyPeriodHeight := applyPeriodHeight(number, p)
			v, _ := money_c[p]
			money_c[p] = v + applyPeriodHeight(wit.height)*wit.money
		}
	}
	for _, wit := range witList {
		var amount, freeze = new(big.Float), new(big.Float)
		for p := uint64(1); p <= period; p++ {
			var witCount float64
			var w_t, m_t float64
			m_t = float64(wit.hearts[p])
			switch wit.role {
			case 1:
				//w_t = float64(sys_wo[p])
				if p*params.ConfigParamsBillingPeriod > number {
					w_t = float64(number % params.ConfigParamsBillingPeriod)
				} else {
					w_t = params.ConfigParamsBillingPeriod
				}
				witCount = sys_count
			case 2:
				w_t = float64(periodUcount)
				witCount = user_count
			case 3:
				w_t = float64(coun_wo[p])
				witCount = council_count * 0.8
			}
			// 工作量比例
			rate := m_t / w_t
			if w_t <= 0 {
				rate = 0
			}
			//log.Info("test work", "me", m_t, "total", w_t)
			// 人数比例
			roleRate := witCount / (sys_count + user_count + council_count*0.8) * 0.38

			// 本周期增发
			periodAdd := additionalEveryPeriod(p)
			// 工作量所得
			result := periodAdd * roleRate * rate
			// 利息所得
			me_money := wit.money * (applyPeriodHeight(number, p)(wit.height))
			interestRate := float64(me_money) / float64(money_c[p])
			if temp, ok := money_c[p]; !ok || temp <= 0 {
				interestRate = 0
			}
			result2 := periodAdd * interestRate * 0.6
			if p > period-2 {
				freeze = freeze.Add(freeze, big.NewFloat(result+result2))
			} else {
				amount = amount.Add(amount, big.NewFloat(result+result2))
			}
			log.Debug("test of role", "address", wit.addr, "role", wit.role, "role.rate", roleRate, "proof.rate", rate,
				"period", p, "period total", periodAdd, "workFee", result, "interest", result2)
		}
		am, fr := nts2dot(amount), nts2dot(freeze)
		//log.Info("test confirm----", "name", wit.name, "addr", wit.addr, "amount", am, "freeze", fr)
		if calcResult[wit.addr].Cmp(new(big.Int).Add(am, fr)) != 0 {
			t.Error("name", wit.name, "expect", calcResult[wit.addr], "actual", new(big.Int).Add(am, fr))
		}

	}
}
func TestCalcPeriod(t *testing.T) {
	t.Log(GetSettlePeriod(62050))
}

type testApplyInfo struct {
	name   string
	addr   common.Address
	role   int // 1.系统见证人 2.用户见证人 3.理事会
	money  uint64
	height uint64
	hearts map[uint64]uint64
}

type testChainContext struct {
	d        []testApplyInfo
	userUnit uint64

	getWitnessReport func(period uint64) map[common.Address]types.WitnessPeriodWorkInfo
}

func (t *testChainContext) GetStateByNumber(chain common.Address, number uint64) (*state.StateDB, error) {
	panic("implement me")
}

func (t *testChainContext) GetHeader(hash common.Hash) *types.Header   { return nil }
func (t *testChainContext) GetBalance(address common.Address) *big.Int { return nil }
func (t *testChainContext) GetWitnessWorkReport(period uint64) map[common.Address]types.WitnessPeriodWorkInfo {
	if t.getWitnessReport != nil {
		return t.getWitnessReport(period)
	}
	return nil
}
func (t *testChainContext) GetWitnessReport(witness common.Address, period uint64) types.WitnessPeriodWorkInfo {

	return types.WitnessPeriodWorkInfo{}
}
func (t *testChainContext) GetAvailableWorkFee(statedb vm.StateDB, address common.Address, end uint64) (*big.Int, *big.Int, error) {
	return nil, nil, nil
}
func (t *testChainContext) GetChainTailHash(chain common.Address) common.Hash     { return common.Hash{} }
func (t *testChainContext) GenesisHash() common.Hash                              { return common.Hash{} }
func (t *testChainContext) GetHeaderByHash(hash common.Hash) *types.Header        { return nil }
func (t *testChainContext) GetUnitState(hash common.Hash) (*state.StateDB, error) { return nil, nil }
