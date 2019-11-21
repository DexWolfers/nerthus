package sc

import (
	"errors"

	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"
)

var (
	errNonSystemWitness = errors.New("the caller is not system witness")
	errRepeatHeartbeat  = errors.New("repeat heartbeat")
)

// SystemHeartbeat 系统见证人维持心跳方法
func SystemHeartbeat(evm vm.CallContext, contract vm.Contract) ([]byte, error) {
	stateDB := evm.State()
	// 不能转账
	if value := contract.Value(); value != nil && value.Sign() != 0 {
		return nil, vm.ErrNonPayable
	}
	// 需要校验身份，只能由见证人发起
	from := contract.Caller()
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas * 2) {
		return nil, vm.ErrOutOfGas
	}
	// 解析
	gpIndex, err := GetWitnessGroupIndex(stateDB, from)
	if err != nil {
		return nil, err
	}
	// 是用户见证人还是系统见证人
	if gpIndex != DefSysGroupIndex {
		return nil, handleUserWitnessHeartbeat(evm, contract)
	}
	if !contract.UseGas(params.SstoreLoadGas) {
		return nil, vm.ErrOutOfGas
	}
	wintessIndex, ye := GetWitnessSet(stateDB, gpIndex).Exist(stateDB, from)
	if !ye {
		return nil, errNonSystemWitness
	}
	num, _ := GetVoteNum(params.SCAccount)
	if err != nil {
		return nil, err
	}

	//如果是前11个中的一个，则为主系统见证人（index从0开始，num=11，）
	isManSysWitness := int(wintessIndex) < num

	//处理心跳
	if err := handleWitnessHeartbeat(evm, contract, isManSysWitness); err != nil {
		return nil, err
	}
	if !isManSysWitness {
		return nil, nil
	}

	//主系统见证人只允许出单元的提案者包含心跳
	if proposer := evm.Ctx().Coinbase; from != proposer {
		log.Info("not leader to send heartbeat", "proposer", proposer, "from", from)
		return nil, errors.New("not leader to send heartbeat")
	}
	if evm.State().TxIndex() != 0 {
		return nil, errors.New("main system witness heart must at unit txs[0]")
	}
	//单元的第一个单元的第一笔空交易执行检查
	return nil, checkMainSystemStatus(evm, contract)
}

func handleUserWitnessHeartbeat(evm vm.CallContext, contract vm.Contract) error {
	listHeartbeat, err := ReadUserWitnessHeartbeat(evm.State())
	if err != nil {
		return err
	}
	from := contract.Caller()
	turns := params.CalcRound(evm.Ctx().UnitNumber.Uint64())
	periods := GetSettlePeriod(evm.Ctx().UnitNumber.Uint64())
	// 检查是否已有记录
	if turns <= listHeartbeat.QueryLastTurn(from) {
		return errRepeatHeartbeat
	}
	listHeartbeat = listHeartbeat.AddBackCount(from, turns, evm.Ctx().UnitNumber.Uint64(), periods)
	// 消耗
	if !contract.UseGas(params.SstoreSetGas) {
		return vm.ErrOutOfGas
	}
	log.Debug("handle heartbeat from user witness", "from", from, "last turn", turns, "number", evm.Ctx().UnitNumber)
	// 存储
	return WriteUserWitnessHeartbeat(evm.State(), listHeartbeat)
}

func handleWitnessHeartbeat(evm vm.CallContext, contract vm.Contract, isManSysWitness bool) error {
	listHeartbeat, err := ReadSystemHeartbeat(evm.State())
	if err != nil && err != state.ErrStateNotFind {
		return err
	}
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return vm.ErrOutOfGas
	}

	periods := GetSettlePeriod(evm.Ctx().UnitNumber.Uint64())

	// 消耗
	if !contract.UseGas(params.SloadGas) {
		return vm.ErrOutOfGas
	}
	// 获取当前回合数
	round := params.CalcRound(evm.Ctx().UnitNumber.Uint64())
	//round, index := slot.CalcSlot(num, time.Unix(0, evm.Ctx().Time.Int64()))

	turns := uint64(round)
	from := contract.Caller()
	// 判断是主见证人还是候选见证人，根据不同执行记录
	if isManSysWitness {
		number := evm.Ctx().UnitNumber.Uint64()
		if number <= listHeartbeat.QueryLastNumber(from) {
			return errRepeatHeartbeat
		}
		listHeartbeat = listHeartbeat.AddUnitCount(from, turns, number, periods)

	} else {
		// 检查是否已有记录
		if turns <= listHeartbeat.QueryLastTurn(from) {
			return errRepeatHeartbeat
		}
		listHeartbeat = listHeartbeat.AddBackCount(from, turns, evm.Ctx().UnitNumber.Uint64(), periods)
		log.Debug("handle heartbeat from secondary witness ", "from", from, "last turn", turns, "number", evm.Ctx().UnitNumber)
	}
	// 消耗
	if !contract.UseGas(params.SstoreSetGas) {
		return vm.ErrOutOfGas
	}
	// 存储
	return WriteSystemHeartbeat(evm.State(), listHeartbeat)
}

// MemberHeartbeat 成员心跳处理
func MemberHeartbeat(evm vm.CallContext, contract vm.Contract) ([]byte, error) {
	stateDB := evm.State()
	// 不能转账
	if value := contract.Value(); value != nil && value.Sign() != 0 {
		return nil, vm.ErrNonPayable
	}
	// 需要校验身份，只能由成员账号发起
	from := contract.Caller()
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return nil, vm.ErrOutOfGas
	}
	if !IsActiveCouncil(stateDB, from) {
		return nil, errors.New("the caller is not council member")
	}

	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return nil, vm.ErrOutOfGas
	}
	// 记录心跳
	listHB, err := ReadCouncilHeartbeat(stateDB)
	if err != nil && err != state.ErrStateNotFind {
		return nil, err
	}
	// 消耗
	if !contract.UseGas(params.SloadGas) {
		return nil, vm.ErrOutOfGas
	}
	round := params.CalcRound(evm.Ctx().UnitNumber.Uint64())
	lastNumber := listHB.QueryLastNumber(from)
	// 确保同一高度只有一个心跳记录
	if lastNumber < evm.Ctx().UnitNumber.Uint64() {
		period := GetSettlePeriod(evm.Ctx().UnitNumber.Uint64())

		listHB = listHB.AddUnitCount(from, uint64(round), evm.Ctx().UnitNumber.Uint64(), period)

		// 消耗
		if !contract.UseGas(params.SstoreSetGas) {
			return nil, vm.ErrOutOfGas
		}

		if err = WriteCouncilHeartbeat(stateDB, listHB); err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("repeat hearbeat")
	}
	log.Debug("handle heartbeat from council ", "from", from, "last turn", round, "number", evm.Ctx().UnitNumber)
	return nil, nil
}
