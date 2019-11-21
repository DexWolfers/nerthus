package sc

import (
	"errors"
	"math"
	"math/big"
	"strconv"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"
)

type additionalType = uint8

const (
	undefinedAdditionalType  additionalType = iota
	systemWitAdditionalType                 // 系统见证人
	councilWitAdditionalType                // 理事会
	foundationAdditionalType                // 基金会
	normalAdditionalType                    // 普通见证人
	interestAdditionalType                  // 额外的利息分成
)
const (
	maxMultipleNormalAdditional = 3    // 用户见证人个人占比上限倍数
	minMultipleNormalAdditional = 3    // 用户见证人个人见证率最低倍数
	minWitnessGroupUnitCount    = 1000 // 筛选见证率的总单元下限
)

var (
	everyPeriodYear = uint64(364 / 7) // 每年占用52个周期
)
var (
	errAdditionalInvalidAccount = errors.New("account is invalid")
)

// GetAdditionalBalance 查询可领取余额
func GetAdditionalBalance(from common.Address, number uint64, state vm.StateDB, dagChain vm.ChainContext) (*big.Int, *big.Int, error) {
	if !canWithdarwAddition(state, from) {
		return nil, nil, errors.New("illegal identity")
	}
	// 获取当前的周期
	period := GetSettlePeriod(number)

	balance, freeze, err := getAdditionalBalance(from, period, number, state, dagChain, false)
	if err != nil {
		return nil, nil, err
	}
	return balance, freeze, err
}

func canWithdarwAddition(db StateDB, user common.Address) bool {
	if user == params.FoundationAccount {
		return true
	}
	//现在/曾经是合法见证人
	{
		status, isW := GetWitnessStatus(db, user)
		if isW {
			switch status {
			case WitnessNormal, WitnessLoggedOut:
				return true
			default:
				return false
			}
		}
	}
	//现在/曾经是理事
	{
		_, isC := GetCouncilStatus(db, user)
		if isC {
			return true
		}
	}
	return false
}

// GetAdditional 领取增发与见证费
func GetAdditional(evm vm.CallContext, contract vm.Contract) ([]byte, error) {
	stateDB := evm.State()
	from := contract.Caller()
	if value := contract.Value(); value != nil && value.Sign() != 0 {
		return nil, vm.ErrNonPayable
	}
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return nil, vm.ErrOutOfGas
	}
	number := evm.Ctx().UnitNumber.Uint64()
	if number <= 1 {
		return nil, errors.New("stop calc")
	}

	//if !contract.UseGas(params.SstoreLoadGas*10) {
	//	return nil, vm.ErrOutOfGas
	//}
	if !canWithdarwAddition(stateDB, from) {
		return nil, errors.New("illegal identity")
	}

	// 获取当前的周期,不能涉及当前单元，领取部分只能是从开始到上一个高度
	periodNumber := number - 1

	period := GetSettlePeriod(periodNumber)

	if period <= 2 {
		return nil, errors.New("additional fee calc must after 2 periods from genesis")
	}

	// 计算费用过程工作量大，一次性收取足够 Gas
	c := GetActiveWitnessCount(stateDB)
	if c == 0 {
		c = 33
	}
	if !contract.UseGas(params.SstoreLoadGas * c * 2) {
		return nil, vm.ErrOutOfGas
	}
	additionalBalance, _, err := getAdditionalBalance(from, period, periodNumber, stateDB, evm.Ctx().Chain, true)
	if err != nil {
		return nil, err
	}
	if additionalBalance.Sign() <= 0 {
		return nil, errors.New("no additional fee")
	}
	log.Debug("withdraw additional fee", "address", from, "amount", additionalBalance)
	return nil, nil
}

// getAdditionalBalance 增发费用
// return 可取余额,冻结余额,err
func getAdditionalBalance(from common.Address, period uint64, number uint64, stateDB vm.StateDB, dagChain vm.ChainContext, withdraw bool) (*big.Int, *big.Int, error) {

	cotr := newDefaultAddController(stateDB, dagChain, number)
	//周期不超过2，只计算冻结
	if period <= withdrawJD {
		balance, err := cotr.getAbortBalance(from, period)
		if err != nil {
			return nil, nil, err
		}
		return big.NewInt(0), nts2dot(big.NewFloat(balance)), nil
	}
	// 计算可领取的周期,2周之前才可领取
	period = period - withdrawJD

	balance, err := cotr.getAbortBalance(from, period)
	if err != nil {
		return nil, nil, err
	}
	// 单位换算成Dot
	drawDot := nts2dot(big.NewFloat(balance))
	if withdraw {
		// 是取现 不查冻结
		err := cotr.withdraw(from, period, drawDot)
		return drawDot, nil, err
	}
	freeze := big.NewFloat(0)
	for i := period + 1; i <= period+withdrawJD; i++ {
		amount, err := cotr.calcPeriodBalance(from, i)
		if err != nil {
			return nil, nil, err
		}
		freeze = freeze.Add(freeze, big.NewFloat(amount))
		log.Trace("getAdditionalBalance", "address", from, "period", i, "freeze", nts2dot(big.NewFloat(amount)), "sum", nts2dot(freeze))
	}
	log.Trace("getAdditionalBalance", "address", from, "curr_period", period, "sum_freeze", nts2dot(freeze), "sum_canWithdraw", drawDot)
	return drawDot, nts2dot(freeze), nil
}

// nts2dot nts转化为dot
func nts2dot(f *big.Float) *big.Int {
	if f.Sign() == 0 {
		return big.NewInt(0)
	}
	nts := new(big.Float).Mul(f, new(big.Float).SetUint64(params.Nts))
	dot := new(big.Int)
	// 单位换算成Dot，省去小数
	nts.Int(dot)
	return dot
}

type calcProofWorkFunc func(address common.Address, period uint64) (float64, error)

// 角色
type additionalRole struct {
	db            StateDB
	role          additionalType
	proofRateFunc func(address common.Address, period uint64) (float64, error)
	//计算此角色的工作量
	calcProofWork calcProofWorkFunc
}

// calcBalance 计算角色的增发金额
func (this *additionalRole) calcBalance(address common.Address, period uint64, amount float64, scnumber uint64) (float64, error) {

	//获取周期内的比例
	roleRate := calcRate(this.db, period, this.role, scnumber)
	// 计算角色所占比例
	roleAmount := amount * roleRate
	//从工作证明取应得比例
	rate, err := this.proofRateFunc(address, period) //
	var userAmount float64
	if rate == 0 || math.IsNaN(rate) { //防止出现 NaN
		userAmount = 0.0
	} else {
		userAmount = roleAmount * rate
	}

	toStr := func(f float64) string {
		return strconv.FormatFloat(f, 'f', 24, 64)
	}

	log.Debug("additional calc fee of role", "address", address,
		"role", this.role,
		"period", period, "period.amount", toStr(amount),
		"role.amount", toStr(roleAmount), "role.rate", toStr(roleRate),
		"proof.rate", toStr(rate), "proof.amount", toStr(userAmount))
	return userAmount, err
}

const (
	foundationRate         float64 = 0.2                                 //基金会占比
	interestRate           float64 = 0.4                                 // 利息占比
	witnessRate            float64 = 1.0 - foundationRate - interestRate // 剩余为见证人与理事会工作量占比
	oneUserMaxInterestRate float64 = 0.5                                 //个人利息收入的最大比例，不超过 50%
)

// 应为提现增发属于对过去工作的奖励，即使当前见证人已注销，也应该可以获取/
// 因此取款占比需要根据周期计算，需要计算在周期结束时，根据当时人数分布来确认分配比例
// 而不能根据此刻的人数分布，因为会这样导致当人数变化时不同人在不同时候提现分配比例不稳定。
// 维护每个周期的两项数据更新：
//   1. 见证人数，更新时机：注销/加入/黑名单
//   2. 理事人数，更新时机：退出/加入
// 具体见：PeriodInfo
// 系统见证人+用户见证人+理事会 合占总比例38%
//见证人占比=  有效见证人人数 / (有效见证人人数+有效理事人数*0.8)
//理事占比=  有效理事人数*0.8 / (有效见证人人数+有效理事人数*0.8)
func calcRate(db StateDB, period uint64, role additionalType, sysNumber uint64) float64 {
	switch role {
	case foundationAdditionalType:
		return foundationRate
	case interestAdditionalType:
		return interestRate
	case systemWitAdditionalType:
		return witnessRate * 0.25
	case normalAdditionalType:
		return witnessRate * 0.7
	case councilWitAdditionalType:
		return witnessRate * 0.05
	default:
		return 0
	}

}

func newDefaultAddController(statedb vm.StateDB, chainContext vm.ChainContext, number uint64) *additionalController {

	co := &additionalController{
		stateDB:      statedb,
		chainContext: chainContext,
		dbContext:    SubItems([]interface{}{prefixAdditionalPeriod}),
		number:       number,
	}
	co.addRole(createSystemWitnessRoule(statedb, number))
	co.addRole(createUserWitnessRule(statedb, chainContext, number))
	co.addRole(createCouncilRole(statedb, chainContext, number))    //理事
	co.addRole(createFoundationRole(statedb, chainContext, number)) //基金会

	co.addRole(createInterestRole(statedb, chainContext, number, func(address common.Address, period uint64) (float64, error) {
		//需要根据人的不同角色采用不同的计算方式
		role, err := co.confirmRole(address)
		if err != nil {
			return 0, err
		}
		return role.calcProofWork(address, period)
	})) //利息分配

	return co
}

func createSystemWitnessRoule(state StateDB, number uint64) *additionalRole {

	// 系统见证人
	sysWitness := additionalRole{
		db:   state,
		role: systemWitAdditionalType,
	}
	sysWitness.calcProofWork = func(address common.Address, period uint64) (float64, error) {
		sys, err := ReadSystemHeartbeat(state)
		if err != nil {
			return 0, err
		}
		addrH := sys.QueryHeartSub(address)
		return float64(addrH.Query(period).UnitCount), nil
	}

	sysWitness.proofRateFunc = func(address common.Address, period uint64) (ratio float64, err error) {

		// 系统见证人心跳总数
		var uCount float64
		if period*params.ConfigParamsBillingPeriod > number {
			uCount = float64(number % params.ConfigParamsBillingPeriod)
		} else {
			uCount = params.ConfigParamsBillingPeriod
		}
		if uCount <= 0 {
			return 0, nil
		}

		work, err := sysWitness.calcProofWork(address, period)
		if err != nil {
			return 0, err
		}
		log.Trace("proofRate",
			"address", address, "period", period, "work", work, "sum", uCount, "rate", work/float64(uCount))
		return work / uCount, nil
	}

	return &sysWitness
}

func createUserWitnessRule(state StateDB, chainContext vm.ChainContext, sysNumber uint64) *additionalRole {
	const prefixPeriodUnits = "preiod_units"

	// 用户见证人
	normalWitness := additionalRole{
		db:   state,
		role: normalAdditionalType,
	}
	normalWitness.calcProofWork = func(address common.Address, period uint64) (float64, error) {
		//自己
		report := chainContext.GetWitnessReport(address, period)
		if report.ShouldVote > minWitnessGroupUnitCount {
			// 见证率低于正常水平1/3将没有奖励
			min := report.ShouldVote / (params.ConfigParamsSCWitness * minMultipleNormalAdditional)
			if report.CreateUnits < min {
				return 0, nil
			}
		}
		return float64(report.CreateUnits), nil
	}
	normalWitness.proofRateFunc = func(address common.Address, period uint64) (float64, error) {
		work, err := normalWitness.calcProofWork(address, period)
		if err != nil {
			return 0, err
		}
		if work == 0.0 {
			return 0, nil
		}
		// 用户见证人心跳总数
		var uCount uint64
		var witCount uint64
		var reports map[common.Address]types.WitnessPeriodWorkInfo
		getWitnessHearts := func() {
			reports = chainContext.GetWitnessWorkReport(period)
			for _, r := range reports {
				if r.IsSys {
					continue
				}
				witCount++
				uCount += r.CreateUnits
			}
		}
		valueKey := common.SafeHash256(prefixPeriodUnits, period)
		v := state.GetState(params.SCAccount, valueKey)
		if !v.Empty() {
			uCount = v.Big().Uint64()
		} else {
			getWitnessHearts()
			//存储计算结果，下次可直接使用。
			if uCount > 0 {
				state.SetState(params.SCAccount, valueKey, common.BigToHash(new(big.Int).SetUint64(uCount)))
			}
		}

		if uCount == 0 {
			return 0, nil
		}
		if reports == nil {
			getWitnessHearts()
		}
		// 单人的占比有上限
		// 最多只能是平均值的3倍
		max := float64(uCount) / float64(witCount) * maxMultipleNormalAdditional
		// 如果自己超出上限，设置为最大值
		if work > max {
			work = max
		}
		var beyond float64
		for _, r := range reports {
			if r.IsSys {
				continue
			}
			if float64(r.CreateUnits) > max {
				// 多余的部分需要分摊给所有人
				beyond += float64(r.CreateUnits) - max
			}
		}
		// 分摊
		work += beyond / float64(witCount)

		log.Trace("proofRate",
			"address", address, "period", period, "work", work, "sum", uCount, "rate", work/float64(uCount))
		return work / float64(uCount), nil
	}
	return &normalWitness
}
func createCouncilRole(state StateDB, _ vm.ChainContext, _ uint64) *additionalRole {
	const prefixPeriodHearts = "preiod_counli_heart_count"

	// 理事会
	council := additionalRole{
		db:   state,
		role: councilWitAdditionalType,
	}
	council.calcProofWork = func(address common.Address, period uint64) (float64, error) {
		counli, err := ReadCouncilHeartbeat(state)
		if err != nil {
			return 0, err
		}
		find := counli.QueryHeartSub(address)
		work := find.Query(period).UnitCount
		return float64(work), nil
	}
	council.proofRateFunc = func(address common.Address, period uint64) (float64, error) {
		work, err := council.calcProofWork(address, period)
		if err != nil {
			return 0, err
		}
		if work == 0.0 {
			return 0, nil
		}
		counliHeartCount := uint64(0)
		valueKey := common.SafeHash256(prefixPeriodHearts, period)
		v := state.GetState(params.SCAccount, valueKey)
		if !v.Empty() {
			counliHeartCount = v.Big().Uint64()
		} else {
			counli, err := ReadCouncilHeartbeat(state)
			if err != nil {
				return 0, err
			}
			counli.Range(func(a common.Address, h Heartbeat) bool {
				pcount := h.Count.Query(period)
				counliHeartCount += pcount.UnitCount
				return true
			})
			if counliHeartCount > 0 {
				state.SetState(params.SCAccount, valueKey, common.BigToHash(new(big.Int).SetUint64(counliHeartCount)))
			}
		}
		//log.Debug("calc work", "me", currHeartCount, "total", counliCount)
		if counliHeartCount == 0 {
			return 0, nil
		}
		log.Trace("proofRate",
			"address", address, "period", period, "work", work, "sum", counliHeartCount, "rate", work/float64(counliHeartCount))
		return work / float64(counliHeartCount), nil
	}
	return &council
}
func createFoundationRole(state StateDB, _ vm.ChainContext, _ uint64) *additionalRole {
	// 基金会 占总比例0.2%
	foundation := additionalRole{
		db:   state,
		role: foundationAdditionalType,
	}
	foundation.calcProofWork = func(address common.Address, period uint64) (float64, error) {
		return 1, nil
	}
	foundation.proofRateFunc = func(address common.Address, period uint64) (float64, error) {
		// 基金会只有一个账户，故占所有比例
		return 1, nil
	}
	return &foundation
}

func createInterestRole(state StateDB, chainContext vm.ChainContext, number uint64, getProofWork calcProofWorkFunc) *additionalRole {
	// 利息分成占总比例60%
	interest := additionalRole{
		db:   state,
		role: interestAdditionalType,
		proofRateFunc: func(address common.Address, period uint64) (float64, error) {
			total := new(big.Float)
			var addrBL *big.Float
			applyPeriodHeight := applyPeriodHeight(number, period)

			_, lastNumber := GetPeriodRange(period)
			if lastNumber > number-1 {
				lastNumber = number - 1
			}
			periodState, err := chainContext.GetStateByNumber(params.SCAccount, lastNumber)
			if err != nil {
				return 0, err
			}

			calcAge := func(witness common.Address, appHeight, margin uint64) (*big.Float, error) {
				if err != nil {
					//不应该发生的错误
					log.Error("failed to load info", "witness", witness, "err", err)
					return nil, err
				}
				h := applyPeriodHeight(appHeight)
				if h <= 0 {
					return big.NewFloat(0), nil
				}
				work, err := getProofWork(witness, period)
				if err != nil {
					return nil, err
				}
				if work == 0.0 || math.IsNaN(work) {
					return big.NewFloat(0), nil

				}
				return new(big.Float).Mul(new(big.Float).SetUint64(margin), new(big.Float).SetUint64(h)), nil
			}

			var oneErr error

			addBL := func(account common.Address, apply, margin uint64) bool {
				witBL, err := calcAge(account, apply, margin)
				log.Trace("calculate coin age", "account", account,
					"apply", apply, "margin", margin,
					"bili", witBL, "err", err)

				if err != nil {
					oneErr = err
					return false
				}
				if address == account {
					addrBL = witBL
					//如果自己的比例是 0，则无需继续计算
					if addrBL == nil || addrBL.Sign() <= 0 {
						return false
					}
				}
				total = total.Add(total, witBL)
				return true
			}
			var (
				cacheKey common.Hash
				saveSize = big.NewFloat(1000)
			)
			cacheKey = common.SafeHash256("periodTotalBi_", period)
			//如果已存在缓存，则只需要获取本人比例则可直接计算
			if totalCache := state.GetState(params.SCAccount, cacheKey); !totalCache.Empty() {
				total = total.SetInt(totalCache.Big())
				//存储时已经放大 1000 倍，此时只需要降低 1000 倍，以便尽可能的保留精度。
				total = total.Quo(total, saveSize)

				if ExistWitness(periodState, address) {
					wit, err := CreateWintessInfoLazy(periodState, address)
					if err != nil {
						return 0, err
					}
					addBL(address, wit.ApplyHeight(), wit.Margin())
				} else {
					couci := CreateCouncilInfoLazy(periodState, address)
					if s, isC := couci.Status(); !isC || s != CouncilStatusValid {
						return 0, nil
					}
					addBL(address, couci.ApplyHeight(), couci.Margin())
				}
				if addrBL == nil || addrBL.Sign() <= 0 {
					return 0, nil
				}
				r, _ := addrBL.Quo(addrBL, total).Float64()
				//有比例上限
				if r > oneUserMaxInterestRate {
					r = oneUserMaxInterestRate
				}
				return r, nil
			}

			// 见证人总币龄
			RangeActiveWitness(periodState, func(witness common.Address) bool {
				wit, err := CreateWintessInfoLazy(periodState, witness)
				if err != nil {
					if err != nil {
						oneErr = err
						return false
					}
					return false
				}
				return addBL(witness, wit.ApplyHeight(), wit.Margin())
			})
			if oneErr != nil {
				return 0, oneErr
			}
			//如果自己的比例是 0，则无需继续计算
			if addrBL == nil || addrBL.Sign() <= 0 {
				return 0, nil
			}

			// 理事会总币龄
			RangeCouncil(periodState, func(addr common.Address) bool {
				couci := CreateCouncilInfoLazy(periodState, addr)
				if s, isC := couci.Status(); !isC || s != CouncilStatusValid {
					return true
				}
				return addBL(addr, couci.ApplyHeight(), couci.Margin())
			})
			if oneErr != nil {
				return 0, oneErr
			}

			//存储一个取整的总量，对第一个取款的人稍有影响。但不需干预
			if !cacheKey.Empty() {
				totalInt1000, _ := new(big.Float).Mul(total, saveSize).Int(nil) //放大 1000 倍
				state.SetState(params.SCAccount, cacheKey, common.BigToHash(totalInt1000))
			}
			if addrBL == nil || addrBL.Sign() <= 0 {
				return 0, nil
			}
			if total.Sign() <= 0 {
				return 0, nil
			}
			r, _ := addrBL.Quo(addrBL, total).Float64()
			//有比例上限
			if r > oneUserMaxInterestRate {
				r = oneUserMaxInterestRate
			}

			log.Trace("calculate work rate", "account", address, "sumAge", total, "bili", addrBL, "rate", r)
			return r, nil
		},
	}

	return &interest
}

// applyPeriodHeight 计算在这个期间质押的高度
func applyPeriodHeight(number uint64, period uint64) func(uint64) uint64 {
	var s_height, e_height uint64
	if number <= params.ConfigParamsBillingPeriod {
		s_height = 0
		e_height = number
	} else if period*params.ConfigParamsBillingPeriod > number {
		s_height = (period-1)*params.ConfigParamsBillingPeriod + 1
		e_height = number
	} else {
		s_height = (period-1)*params.ConfigParamsBillingPeriod + 1
		e_height = period * params.ConfigParamsBillingPeriod
	}
	return func(applyHeight uint64) uint64 {
		if applyHeight > e_height {
			return 0
		}
		if applyHeight >= s_height && applyHeight <= e_height {
			return e_height - applyHeight
		}
		return params.ConfigParamsBillingPeriod
	}
}

type additionalController struct {
	rules        []*additionalRole
	stateDB      vm.StateDB
	chainContext vm.ChainContext
	dbContext    SubItems
	number       uint64

	testOnlyAadditional bool // 仅仅用于协助测试，是否只计算利息收益
}

// withdraw 取现
func (this *additionalController) withdraw(address common.Address, period uint64, amount *big.Int) error {
	//amount, err := this.getAbortBalance(address, period)
	//if err != nil {
	//	return 0, err
	//}
	if amount.Sign() == 0 {
		return nil
	}
	// 记录取现记录
	if err := this.setLastDrawPeriod(address, period); err != nil {
		return err
	}
	// 转账
	this.stateDB.AddBalance(address, amount)
	return nil
}

// getAbortBalance 获取截止周期可取现额度
func (this *additionalController) getAbortBalance(address common.Address, abortPeriod uint64) (float64, error) {
	if abortPeriod <= 0 {
		return 0, nil
	}
	lastP, err := this.getLastDrawPeriod(address)
	if err != nil {
		return 0, err
	}
	lastP = checkSettlementPeriod(lastP, abortPeriod)

	var total float64 = 0
	for i := lastP + 1; i <= abortPeriod; i++ {
		am, err := this.calcPeriodBalance(address, i)
		if err != nil {
			return 0, err
		}
		total += am
		log.Trace("getAbortBalance",
			"address", address, "period", i, "freeze", nts2dot(big.NewFloat(am)), "sum", nts2dot(big.NewFloat(total)))
	}
	return total, nil
}

// calcPeriodBalance 计算指定周期可取现量
func (this *additionalController) calcPeriodBalance(address common.Address, period uint64) (float64, error) {
	mine, _, err := this.calcPeriodBalance2(address, period)
	return mine, err

}

// 计算指定周期内的收益：
// 返回：总计收益和奖励收益，其中他们的差额为利息收益
func (this *additionalController) calcPeriodBalance2(address common.Address, period uint64) (float64, float64, error) {
	// 计算当前周期增发量
	total := additionalEveryPeriod(period)

	role, err := this.confirmRole(address)
	if err != nil {
		return 0, 0, err
	}
	// 计算工作量所得
	mine, err := role.calcBalance(address, period, total, this.number)
	if err != nil {
		return 0, 0, err
	}

	workReward := mine
	// 不是基金会，需要额外计算一次利息
	// 周期内没有干活，不给利息
	if role.role != foundationAdditionalType && mine > 0 {
		calcInterest := func() (float64, error) {
			for _, r := range this.rules {
				if r.role == interestAdditionalType {
					interest, err := r.calcBalance(address, period, total, this.number)
					if err != nil {
						return 0, err
					}
					return interest, nil
				}
			}
			return 0, nil
		}
		interest, err := calcInterest()
		if err != nil {
			return 0, 0, err
		}
		if this.testOnlyAadditional {
			return interest, 0, nil
		}
		mine += interest
	}
	if math.IsNaN(mine) {
		log.Trace("calcPeriodBalance", "address", address, "period", period, "total", 0)
		return 0, 0, err
	}

	log.Trace("calcPeriodBalance", "address", address, "period", period, "total", nts2dot(big.NewFloat(mine)),
		"workReward", nts2dot(big.NewFloat(workReward)), "interest", nts2dot(big.NewFloat(mine-workReward)))
	return mine, workReward, nil

}

// getLastDrawPeriod 获取最后取现周期
func (this *additionalController) getLastDrawPeriod(address common.Address) (uint64, error) {
	hash := this.dbContext.GetSub(this.stateDB, address)
	return hash.Big().Uint64(), nil
}

// setLastDrawPeriod 记录最后取现周期
func (this *additionalController) setLastDrawPeriod(address common.Address, period uint64) error {
	this.dbContext.SaveSub(this.stateDB, address, period)
	//if err != nil {
	//	return err
	//}
	return nil
}

// confirmRole 确认角色
func (this *additionalController) confirmRole(address common.Address) (*additionalRole, error) {
	role, err := accountRole(address, this.stateDB)
	if err != nil {
		return nil, err
	}
	fIndex := -1
	for i, r := range this.rules {
		if r.role == role {
			fIndex = i
			break
		}
	}
	if fIndex < 0 {
		return nil, errAdditionalInvalidAccount
	}
	return this.rules[fIndex], nil
}

// addRole 添加角色到处理器中
func (this *additionalController) addRole(roles ...*additionalRole) {
	this.rules = append(this.rules, roles...)
}

// accountRole 确认账户类型
func accountRole(addr common.Address, stateDB vm.StateDB) (additionalType, error) {
	if addr == params.FoundationAccount {
		return foundationAdditionalType, nil
	}
	// 见证人
	{
		item := SubItems([]interface{}{witnessPrefix, addr})
		status := item.GetSub(stateDB, "status")
		if !status.Empty() {
			gp := item.GetSub(stateDB, "group_index")
			//这里不能区分已拉黑见证人，提现的权限控制已经在外部处理
			if gp.Big().Uint64() == DefSysGroupIndex {
				return systemWitAdditionalType, nil
			} else {
				return normalAdditionalType, nil
			}
		}
	}
	// 理事会
	{
		item := getCouncilInfoItem(addr)
		status := item.GetSub(stateDB, "status")
		if !status.Empty() {
			return councilWitAdditionalType, nil
		}
	}

	return undefinedAdditionalType, errAdditionalInvalidAccount
}

// additionalEveryPeriod 当前周期的增发量, 每年`everyPeriodYear`个周期
func additionalEveryPeriod(period uint64) float64 {
	year := divideCeil(period, everyPeriodYear)
	additionalNumbers := additionalCalcFormula(year)

	return additionalNumbers / float64(everyPeriodYear)
}

// https://gitee.com/Nerthus/developdoc/blob/master/content/basic/增发与奖励.md
// additionalCalcFormula 获取指定第N年的增发量
func additionalCalcFormula(year uint64) float64 {
	tokens := params.TotalTokens()
	ratio := calcIssueRatio(year)
	// tokens * 增发比例
	oneYear := new(big.Float).Mul(new(big.Float).SetInt(tokens), new(big.Float).SetFloat64(ratio))
	// 转换为 NTS
	oneYearNTS := oneYear.Quo(oneYear, new(big.Float).SetUint64(params.Nts))
	v, _ := oneYearNTS.Float64()
	return v
}

const (
	minIssueRatio  = 0.05 / 100 //每年增发比例最小值 0.05%
	firstYearRatio = 2.5 / 100  //首次 2.5%
)

// 计算每年增发比率：
// 每四年减半增发，但最小增发比例为0.069%
func calcIssueRatio(year uint64) float64 {
	switch year {
	case 0:
		panic("invalid year,must be greater than 0")
	default:
		pow := float64(divideCeil(year, 4) - 1)    //每四年一个周期
		ratio := firstYearRatio / math.Pow(2, pow) //比例减半
		//四舍五入，保留四位小数，即 00.0000%
		return math.Max(math.Round(ratio*10000)/10000, minIssueRatio)
	}
}

func divideNumber(x, y uint64) (uint64, uint64) {
	divideN := x / y
	return divideN, x % y
}

func divideCeil(x, y uint64) uint64 {
	xx := float64(x)
	yy := float64(y)
	return uint64(math.Ceil(xx / yy))
}

func divideFloor(x, y uint64) uint64 {
	xx := float64(x)
	yy := float64(y)
	return uint64(math.Floor(xx / yy))
}
