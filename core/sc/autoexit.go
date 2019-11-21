package sc

import (
	"fmt"
	"math/big"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"

	"github.com/pkg/errors"
)

// 根据角色直接获取数据查看是否已超时
func systemWitnessIsTimeout(db StateDB, role types.AccountRole, witness common.Address, number uint64) bool {
	lastRound := QueryHeartbeatLastRound(db, role, witness)
	return isTimeoutHeart(role, params.CalcRound(number), lastRound)
}

func councilIsTimeout(db StateDB, council common.Address, number uint64) (bool, error) {
	//是理事，可以检查
	lastRound := QueryHeartbeatLastRound(db, types.AccCouncil, council)
	return isTimeoutHeart(types.AccCouncil, params.CalcRound(number), lastRound), nil
}

func witnessIsTimeout(db StateDB, witness common.Address, number uint64) (types.AccountRole, bool, error) {
	isSys, err := IsSystemWitness(db, witness)
	if err != nil {
		return -1, false, err
	}

	if !isSys {
		//用户见证人
		lastRound := QueryHeartbeatLastRound(db, types.AccCouncil, witness)
		return types.AccUserWitness, isTimeoutHeart(types.AccUserWitness, params.CalcRound(number), lastRound), nil
	}

	//系统见证人
	{
		_, mainList, err := GetSystemMainWitness(db)
		if err != nil {
			return -1, false, err
		}
		//确认身份
		role := types.AccSecondSystemWitness
		if mainList.Have(witness) {
			role = types.AccSecondSystemWitness
		}
		return role, systemWitnessIsTimeout(db, role, witness, number), nil
	}
}

var errInvalidStatus = errors.New("invalid status")

// 判断理事/见证人是否已超时
func IsTimeout(db StateDB, who common.Address, number uint64) (role types.AccountRole, timeout bool, err error) {
	status, isWitness := GetWitnessStatus(db, who)
	if isWitness {
		if status != WitnessNormal {
			return -1, false, errInvalidStatus
		}
		return witnessIsTimeout(db, who, number)
	}

	s2, isCouncil := GetCouncilStatus(db, who)
	if isCouncil {
		if s2 != CouncilStatusValid {
			return types.AccCouncil, false, errInvalidStatus
		}
		timeout, err = councilIsTimeout(db, who, number)
		return types.AccCouncil, timeout, err
	}

	return types.AccNormal, false, errors.New("not a witness/council")
}

// 获取超时的备选见证人
func GetTimeoutSecondSystemWitness(db StateDB, number uint64) ([]common.Address, error) {
	allSystemWitness, err := ReadSystemWitness(db)
	if err != nil {
		return nil, err
	}
	if len(allSystemWitness) <= params.ConfigParamsSCWitness {
		return nil, nil
	}
	var list []common.Address
	for _, w := range allSystemWitness[params.ConfigParamsSCWitness:] {
		if systemWitnessIsTimeout(db, types.AccSecondSystemWitness, w, number) {
			list = append(list, w)
		}
	}
	return list, nil
}

// 获取超时的备选系统见证人
func GetTimeoutMainSystemWitness(db StateDB, number uint64) ([]common.Address, error) {
	_, mainWitness, err := GetSystemMainWitness(db)
	if err != nil {
		return nil, err
	}
	var list []common.Address
	for _, w := range mainWitness {
		if systemWitnessIsTimeout(db, types.AccMainSystemWitness, w, number) {
			list = append(list, w)
		}
	}
	return list, nil
}

// 根据类型判断是否超时
func isTimeoutHeart(rol types.AccountRole, currRound uint64, lastHeartRound uint64) bool {
	var max uint64
	switch rol {
	case types.AccMainSystemWitness:
		max = params.MaxMainSCWitnessHeartbeatRound
	case types.AccSecondSystemWitness:
		max = params.MaxHeartbeatRound
	case types.AccCouncil:
		max = params.MaxHeartbeatRound
	case types.AccUserWitness:
		max = params.MaxHeartbeatRound
	default:
		panic(fmt.Errorf("invalid typ %d", rol))
	}
	//如果最后一个心跳轮次，已经超过最大允许范围则需要踢出
	return math.ForceSafeSub(currRound, lastHeartRound) > max
}

// check address heartbeat timeout
// return true:timeout
func CheckHeartTimeout(db StateDB, role types.AccountRole, addr common.Address, number uint64) bool {
	lastRound := QueryHeartbeatLastRound(db, role, addr)
	return isTimeoutHeart(role, params.CalcRound(number), lastRound)
}

// 获取心跳发送超过最大限定的理事成员，该部分成员可以删除
func GetTimeoutCouncil(db StateDB, number uint64) []common.Address {
	query, err := ReadCouncilHeartbeat(db)
	if err != nil {
		return nil
	}
	currRound := params.CalcRound(number)
	var invalid []common.Address
	//遍历所有理事
	RangeCouncil(db, func(addr common.Address) bool {
		if isTimeoutHeart(types.AccCouncil, currRound, query.QueryLastTurn(addr)) {
			invalid = append(invalid, addr)
		}
		return true //continue
	})
	return invalid
}

// 获取失效用户见证人
func GetInvalidUserWitness(db StateDB, number uint64) ([]common.Address, error) {
	query, err := ReadUserWitnessHeartbeat(db)
	if err != nil {
		return nil, err
	}
	currRound := params.CalcRound(number)
	list, err := GetActivWitnessLib(db)
	if err != nil {
		return nil, err
	}
	var invalid []common.Address
	for _, wit := range list {
		if wit.GroupIndex == DefSysGroupIndex {
			continue
		}
		if isTimeoutHeart(types.AccUserWitness, currRound, query.QueryLastTurn(wit.Address)) {
			invalid = append(invalid, wit.Address)
		}
	}
	return invalid, nil
}

// 移除超时理事
func removeTimeoutCouncil(evm vm.CallContext, list ...common.Address) error {
	//依次移除理事
	//因为是被动移除，干活不积极，将扣除一部分保证金
	for _, c := range list {
		refund := calcPercentMoney(GetCounciMargin(evm.State(), c), params.CounciTimeoutPunish)
		//转给理事
		evm.Ctx().Transfer(evm.State(), params.SCAccount, c, new(big.Int).SetUint64(refund))
		//踢出
		err := RemoveCouncil(evm.State(), c, evm.Ctx().UnitNumber.Uint64())
		if err != nil {
			return err
		}
	}
	return nil
}

// 处理移除心跳已超时不正常的见证人/理事
// 默认配置为如果见证人/理事三天无心跳，则剔除出局
func handleRemoveTimeout(evm vm.CallContext, contract vm.Contract, addresses []common.Address) error {

	//遍历所有需要移除的见证人
	// 1. 如果是非见证人/理事，则交易失败
	// 2. 如果是无效的见证人/理事，则忽略
	// 3. 如果是有效见证人/理事，但是心跳正常，则交易失败

	//只有当前单元提案则才有权限执行，不需要再校验是否是系统见证人
	if evm.Ctx().Coinbase != contract.Caller() {
		return errors.New("invalid caller:you are not unit creator")
	}
	if len(addresses) == 0 {
		return errors.New("empty list")
	}
	if len(addresses) > 100 {
		return errors.Errorf("exceeding the upper processing limit,the upper limit is 100,the actual is %d", len(addresses))
	}

	var userWitnessIsLess bool
	var removed []common.Address
	number := evm.Ctx().UnitNumber.Uint64()
	for _, addr := range addresses {
		role, yes, err := IsTimeout(evm.State(), addr, number)
		if err != nil {
			if err == errInvalidStatus { //如果是不是正常状态，则允许忽略
				continue
			}
			return err
		}
		if !yes {
			return fmt.Errorf("can not remove %q when he's hearbeat is not timeout", addr)
		}
		switch role {
		default:
			return errors.Errorf("can not work for role %s", role)
		case types.AccMainSystemWitness:
			return fmt.Errorf("can not remove the main system witness %q", addr)
		case types.AccSecondSystemWitness:
			if err := removeWitness(evm, addr, false, true); err != nil {
				return errors.Wrapf(err, "remove second system witness %s failed", addr)
			}
		case types.AccUserWitness:
			//如果用户见证人人数已 <=8 则不允许继续移除
			if userWitnessIsLess {
				continue
			}
			if err := removeWitness(evm, addr, false, true); err != nil {
				return errors.Wrapf(err, "remove user witness %s failed", addr)
			}
			//检查是否用户链见证人人数已不足
			allWitness := GetActiveWitnessCount(evm.State())
			systemWitness := GetWitnessCountAt(evm.State(), DefSysGroupIndex)
			userWitnessIsLess = math.ForceSafeSub(allWitness, systemWitness) <= params.ConfigParamsUCMinVotings

		case types.AccCouncil:
			if err := removeTimeoutCouncil(evm, addr); err != nil {
				return errors.Wrapf(err, "remove council %s failed", addr)
			}
		}
		removed = append(removed, addr)
	}

	if len(removed) == 0 {
		return errors.New("no one removed")
	}
	log.Debug("removed some timeout witness/council", "removed", removed)
	return nil
}

// 检查主系统见证人是否有超期，如果是则转为备选见证人
func checkMainSystemStatus(evm vm.CallContext, contract vm.Contract) error {
	list, err := GetTimeoutMainSystemWitness(evm.State(), evm.Ctx().UnitNumber.Uint64())
	if err != nil {
		return err
	}
	//转为备选
	for _, v := range list {
		if err := demoteWitness(evm, v); err != nil {
			return err
		}
	}
	return nil
}
