// 见证人费用结算
package sc

import (
	"errors"
	gmath "math"
	"math/big"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/rlp"
)

// Settlement 领取见证费用
func Settlement(evm vm.CallContext, contract vm.Contract) ([]byte, error) {
	from := contract.Caller()
	// 不能转账
	if value := contract.Value(); value != nil && value.Sign() != 0 {
		return nil, vm.ErrNonPayable
	}
	statedb := evm.State()
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return nil, vm.ErrOutOfGas
	}
	// 判断是否为见证人, 只有为见证人，才有资格结算
	info, err := GetWitnessInfo(statedb, from)
	if err != nil {
		return nil, err
	}
	if !contract.UseGas(params.SstoreLoadGas) {
		return nil, vm.ErrOutOfGas
	}

	switch info.Status {
	case WitnessNormal, WitnessLoggedOut:
	default:
		return nil, errors.New("this account is not active witness")
	}

	// 统计当前阶段可提取的见证费用
	settleAmount, nextPeriod, err := GetWithdrawableWitnessFee(statedb, statedb.Database(), from, evm.Ctx().UnitNumber.Uint64()-1)
	if err != nil {
		return nil, err
	}

	if !contract.UseGas(params.SstoreSetGas) {
		return nil, vm.ErrOutOfGas
	}

	// 将结算的钱转入用户账号中
	statedb.AddBalance(from, settleAmount)
	// 更新下一提取的周期数
	if !contract.UseGas(params.SstoreSetGas) {
		return nil, vm.ErrOutOfGas
	}
	setNextWithdrawPeriod(statedb, from, nextPeriod)

	log.Debug("withdraw witness fee", "address", from, "amount", settleAmount, "next_period", nextPeriod)
	return nil, nil
}

var (
	periodSettlemtPrefix = []byte("period_sett_l_")
	workFeeFromPrefix    = []byte("s_p")
)

// 检查结算提取有效周期
func checkSettlementPeriod(nextPeriod, nowPeriod uint64) uint64 {
	//如果取款已超过指定期限，则只能取期限内的结算
	if math.ForceSafeSub(nowPeriod, nextPeriod) > params.MaxSettlementPeriod {
		return math.ForceSafeSub(nowPeriod, params.MaxSettlementPeriod)
	}
	return nextPeriod
}

func getSettlementPeriodLogKey(period uint64, witness common.Address) []byte {
	return MakeKey(periodSettlemtPrefix, period, witness)
}

func GetWitnessPeriodWorkReport(db ntsdb.Getter, period uint64, witness common.Address) *types.WitnessPeriodWorkInfo {
	key := getSettlementPeriodLogKey(period, witness)
	data, _ := db.Get(key)
	if len(data) == 0 {
		return nil
	}
	info := new(types.WitnessPeriodWorkInfo)
	info.Fee = new(big.Int)

	if len(data) > 0 {
		err := rlp.DecodeBytes(data, info)
		if err != nil {
			panic(err)
		}
	}
	return info
}

// 获取此周期的所有信息
func GetWitnessPeriodWorkReports(db ntsdb.Finder, period uint64) map[common.Address]types.WitnessPeriodWorkInfo {

	reports := make(map[common.Address]types.WitnessPeriodWorkInfo)

	keyPrefix := MakeKey(periodSettlemtPrefix, period)

	db.Iterator(keyPrefix, func(key, value []byte) bool {
		witness := common.BytesToAddress(key[len(keyPrefix):])

		info := new(types.WitnessPeriodWorkInfo)
		info.Fee = new(big.Int)
		err := rlp.DecodeBytes(value, info)
		if err != nil {
			panic(err)
		}
		reports[witness] = *info
		return true
	})
	return reports
}

func updateWitnessWorkReport(db ntsdb.Putter, key []byte, fee *types.WitnessPeriodWorkInfo) {
	data, err := rlp.EncodeToBytes(&fee)
	if err != nil {
		panic(err)
	}
	if err := db.Put(key, data); err != nil {
		log.Crit("storage witness work fee failed", "err", err)
	}
}

// 设置取款开启周期
func setNextWithdrawPeriod(db StateDB, user common.Address, p uint64) {
	key := MakeKey(user, workFeeFromPrefix)
	db.SetState(params.SCAccount, common.BytesToHash(key), common.BytesToHash(common.Uint64ToBytes(p)))
}

// getNextWithdrawPeriod 获取下次取款开始周期
func getNextWithdrawPeriod(from common.Address, statedb StateDB, nowPeriod uint64) uint64 {
	key := MakeKey(from, workFeeFromPrefix)

	fromPeriod := new(big.Int)
	data := statedb.GetState(params.SCAccount, common.BytesToHash(key))
	if data.Empty() {
		fromPeriod.SetInt64(1)
	} else {
		fromPeriod.SetBytes(data.Bytes())
	}

	return checkSettlementPeriod(fromPeriod.Uint64(), nowPeriod)
}

// GetWithdrawableWitnessFee 获取可提取的见证费用，可提取计算的所有尚未提取的费用。
// 返回： 可提取的见证费，下一个提取周期开始id
func GetWithdrawableWitnessFee(statedb StateDB, db ntsdb.Database, addr common.Address, sysNumber uint64) (*big.Int, uint64, error) {
	period := GetSettlePeriod(sysNumber)

	beginPeriod := getNextWithdrawPeriod(addr, statedb, period)

	// 至少冻结一个周期
	period = math.ForceSafeSub(period, withdrawJD)
	if period == 0 || beginPeriod > period {
		return common.Big0, beginPeriod, nil
	}

	//// 取出该账号记入起始周期
	//workBeginPeriod := getBeginWorkPeriod(db, addr)
	//if workBeginPeriod > beginPeriod {
	//	// 判断起始周期是否远远超过传入的周期，如果是， 则将fromPeriod移到起始周期
	//	beginPeriod = workBeginPeriod
	//}

	total := new(big.Int)
	begin := beginPeriod
	// 统计当前所有可结算周期的见证费
	for beginPeriod <= period {
		fee, err := getPeriodWorkFee(addr, new(big.Int).SetUint64(beginPeriod), db)
		if err != nil {
			return nil, 0, err
		}
		total.Add(total, fee)
		log.Trace("GetWithdrawableWitnessFee", "module", "settlement",
			"witness", addr, "period", beginPeriod, "fee", fee)

		beginPeriod += 1
	}
	log.Trace("GetWithdrawableWitnessFee", "module", "settlement",
		"witness", addr, "sum", total, "begin", begin, "end", period)

	return total, beginPeriod, nil
}

// GetFreezingWitnessFee 查询冻结的见证费
func GetFreezingWitnessFee(statedb StateDB, db ntsdb.Database, addr common.Address, sysNumber uint64) (*big.Int, error) {
	period := GetSettlePeriod(sysNumber)

	fromPeriod := getNextWithdrawPeriod(addr, statedb, period)

	freeze := new(big.Int)
	for i := 0; i < 2; i++ {
		p := math.ForceSafeSub(period, uint64(i))
		if p == 0 || p < fromPeriod {
			continue
		}
		amount, err := getPeriodWorkFee(addr, big.NewInt(int64(p)), db)
		if err != nil {
			return nil, err
		}
		freeze = freeze.Add(freeze, amount)
		log.Trace("GetFreezingWitnessFee", "witness", addr, "period", p, "fee", amount)
	}
	log.Trace("GetFreezingWitnessFee", "witness", addr, "sum", freeze, "from", fromPeriod, "currentPeriod", period, "number", sysNumber)

	return freeze, nil
}

// 用于测试的钩子
var getPeriodWorkFeeTest func(addr common.Address, fromPeriod *big.Int) uint64

// getPeriodWorkFee 查询指定周期见证费
func getPeriodWorkFee(addr common.Address, fromPeriod *big.Int, db ntsdb.Database) (*big.Int, error) {
	if getPeriodWorkFeeTest != nil {
		v := getPeriodWorkFeeTest(addr, fromPeriod)
		return new(big.Int).SetUint64(v), nil
	}
	info := GetWitnessPeriodWorkReport(db, fromPeriod.Uint64(), addr)
	if info == nil {
		return big.NewInt(0), nil
	}
	return info.Fee, nil
}

// GetSettlePeriod 获取指定高度所在的统计周期
func GetSettlePeriod(sysNumber uint64) uint64 {
	if sysNumber <= params.ConfigParamsBillingPeriod {
		return 1
	}
	return uint64(gmath.Ceil(float64(sysNumber) / float64(params.ConfigParamsBillingPeriod)))
}

// GetPeriodRange 返回周期period占用的高度范围，闭区间:[startHeight, endHeight]
func GetPeriodRange(period uint64) (uint64, uint64) {

	settlePeriodBig := big.NewInt(0).SetUint64(params.ConfigParamsBillingPeriod)
	base := big.NewInt(0).Mul(settlePeriodBig, big.NewInt(0).SetUint64(period-1))
	startHeight := big.NewInt(0).Add(base, big.NewInt(1))
	endHeight := big.NewInt(0).Mul(settlePeriodBig, big.NewInt(0).SetUint64(period))
	return startHeight.Uint64(), endHeight.Uint64()
}
