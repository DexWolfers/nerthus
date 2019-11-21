package sc

import (
	"errors"
	"fmt"
	"math/big"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"
)

// 处理理事主动注销
func handleCouncilExit(evm vm.CallContext, contract vm.Contract) error {
	stateDB := evm.State()
	// 获取目标
	council := contract.Caller()

	// 消耗
	if !contract.UseGas(params.SstoreLoadGas * 5) {
		return vm.ErrOutOfGas
	}

	info, err := GetCouncilInfo(stateDB, council)
	if err != nil {
		return err
	}
	//理事非正常，不得申请
	if info.Status != CouncilStatusValid {
		return errors.New("you have quit, no repetition is allowed")
	}

	//短时间内不允许主动退出
	canExitNumber := info.ApplyHeight + params.CounciExitNumber
	if evm.Ctx().UnitNumber.Uint64() < canExitNumber {
		return fmt.Errorf("you can exit when the system chain number is %d,but not now", canExitNumber)
	}

	// 返还保证金，但是费用需要自提
	evm.Ctx().Transfer(stateDB, params.SCAccount, council, new(big.Int).SetUint64(info.Margin))
	return RemoveCouncil(stateDB, council, evm.Ctx().UnitNumber.Uint64())
}

// SetMemberJoinApply 申请成为理事会成员提案
func SetMemberJoinApply(evm vm.CallContext, contract vm.Contract) ([]byte, error) {
	stateDB := evm.State()
	// 必须有金额
	if !contract.Value().IsUint64() {
		return nil, errors.New("margin overflow")
	}
	margin := contract.Value().Uint64()
	if margin < params.ConfigParamsCouncilMargin {
		return nil, vm.ErrNotEnoughMargin
	}
	// 获取目标
	target := contract.Caller()
	// 要求target不能是理事会成员
	if err := CheckMemberType(stateDB, target); err != nil {
		return nil, err
	}
	//已无人数要求，但是依旧读取，因为上限是无限大
	if GetCouncilCount(stateDB) >= ForceReadConfig(stateDB, params.ConfigParamsCouncilCap) {
		return nil, ErrNotExceededCouncilCeiling
	}
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return nil, vm.ErrOutOfGas
	}
	// 允许重复申请见证人
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return nil, vm.ErrOutOfGas
	}

	number, now := evm.Ctx().UnitNumber.Uint64(), evm.Ctx().Time.Uint64()
	// 写入提议
	proposal := Proposal{
		Key:            getProposalId(stateDB),
		Type:           ProposalTypeCouncilAdd,
		Status:         ProposalStatusInVoting, //不需要理事投票，直接进入投票阶段
		Applicant:      target,
		Margin:         contract.Value().Uint64(),
		TimeApply:      now,
		TimeVote:       now,
		NumberVote:     number,
		NumberApply:    number,
		NumberFinalize: number + params.ConfigParamsTimeVoting, //设定投票接收时间，结束后可执行定案
	}
	return applyProposal(contract, stateDB, &proposal)
}

// SetMemberRemoveApply 申请移除理事会成员提案
func SetMemberRemoveApply(evm vm.CallContext, contract vm.Contract, target common.Address) ([]byte, error) {
	stateDB := evm.State()
	// 不能转账
	if value := contract.Value(); value != nil && value.Sign() != 0 {
		return nil, vm.ErrNonPayable
	}
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas * 2) {
		return nil, vm.ErrOutOfGas
	}
	// 取出理事会成员
	if s, ok := GetCouncilStatus(stateDB, target); !ok {
		return nil, errors.New("the target is not council member")
	} else if s != CouncilStatusValid {
		return nil, errors.New("the target is not active council member")
	}
	call := contract.Caller()
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return nil, vm.ErrOutOfGas
	}
	wantRemovedMember, err := GetCouncilInfo(stateDB, target)
	if err != nil {
		return nil, err
	}
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return nil, vm.ErrOutOfGas
	}
	number, now := evm.Ctx().UnitNumber.Uint64(), evm.Ctx().Time.Uint64()
	// 写入提议
	proposal := Proposal{
		Key:            getProposalId(stateDB),
		Type:           ProposalTypeCouncilFire,
		Status:         ProposalStatusInVoting,
		Applicant:      call,
		Margin:         wantRemovedMember.Margin,
		FireAddress:    target,
		TimeApply:      now,
		TimeVote:       now,
		NumberVote:     number,
		NumberApply:    number,
		NumberFinalize: number + params.ConfigParamsTimeVoting, //设定投票接收时间，结束后可执行定案
	}
	return applyProposal(contract, stateDB, &proposal)
}

// SetConfigApply 申请修改共识配置提案: 所有满足条件人都可以申请该提案
func SetConfigApply(evm vm.CallContext, contract vm.Contract, cfgKey string, cfgValue uint64) ([]byte, error) {
	stateDB := evm.State()
	// 不能转账
	if value := contract.Value(); value != nil && value.Sign() != 0 {
		return nil, vm.ErrNonPayable
	}
	// 检查配置项是否支持
	if !evm.ChainConfig().Proposal[cfgKey] {
		return nil, vm.ErrNotExist
	}
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas * 2) {
		return nil, vm.ErrOutOfGas
	}
	// 当前修改的配置值不能跟当前值一样
	if curr, err := ReadConfig(stateDB, cfgKey); err != nil {
		return nil, err
	} else if curr == cfgValue {
		return nil, errors.New("current config value with you setting value is same")
	}
	switch cfgKey {
	case params.ConfigParamsLowestGasPrice:
		// gas单价不能大于gas上限
		if cfgValue > evm.ChainConfig().Get(params.ConfigParamsUcGasLimit) {
			return nil, errors.New("gasPrice is not greater than gasLimit")
		}
	case params.ConfigParamsUCWitnessCap:
		// 用户见证人数上限不能小于当前生效的见证人数
		sysActive := GetWitnessCountAt(evm.State(), DefSysGroupIndex)
		if currentSum := math.ForceSafeSub(GetActiveWitnessCount(stateDB), sysActive); cfgValue < currentSum {
			return nil, fmt.Errorf("user witness ceiling can't less than current active witness sum(%d)", currentSum)
		} else if cfgValue%params.ConfigParamsUCWitness != 0 {
			return nil, fmt.Errorf("user witness ceiling must be a multiple of %d", params.ConfigParamsUCWitness)
		}
	case params.ConfigParamsCouncilCap:
		if councilCount := GetCouncilCount(stateDB); cfgValue < councilCount {
			return nil, fmt.Errorf("council ceiling can't less than current active council sum(%d)", councilCount)
		}
	case params.ConfigParamsUcGasLimit:
		if gasLimit := evm.ChainConfig().Get(params.ConfigParamsUcGasLimit); cfgValue < gasLimit {
			return nil, fmt.Errorf("getLimit can't less than current gasLimit(%d)", gasLimit)
		}
	}
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return nil, vm.ErrOutOfGas
	}
	// 检查提案是否可以申请
	if err := ProposalIsCreate(stateDB, ProposalTypeConfigChange, evm.Ctx().UnitNumber.Uint64()); err != nil {
		return nil, err
	}
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return nil, vm.ErrOutOfGas
	}
	// 写入提议
	proposal := Proposal{
		Key:         getProposalId(stateDB),
		Type:        ProposalTypeConfigChange,
		Status:      ProposalStatusInApproval,
		ConfigKey:   cfgKey,
		ConfigValue: cfgValue,
		Applicant:   contract.Caller(),
		TimeApply:   evm.Ctx().Time.Uint64(),
		NumberApply: evm.Ctx().UnitNumber.Uint64(),
		NumberVote:  evm.Ctx().UnitNumber.Uint64() + params.ConfigParamsTimeApprove,
	}
	return applyProposal(contract, stateDB, &proposal)
}

//判断提案是否需要理事审批
func needApproval(typ ProposalType) bool {
	switch typ {
	case ProposalTypeCouncilAdd, ProposalTypeCouncilFire:
		return false
	default:
		return true
	}
}

// applyProposal 申请提案
func applyProposal(contract vm.Contract, stateDB vm.StateDB, proposal *Proposal) ([]byte, error) {
	if needApproval(proposal.Type) {
		var foundErr error
		var count int
		RangeCouncil(stateDB, func(addr common.Address) bool {
			if !contract.UseGas(params.SstoreLoadGas + params.SstoreSetGas) {
				foundErr = vm.ErrOutOfGas
				return false
			}
			count++
			writeCouncilVoteInfo(stateDB, proposal.Key,
				&CouncilVote{addr, VotingResultUnknown, 0}, uint64(count))
			return true
		})
		if foundErr != nil {
			return nil, foundErr
		}
		if count == 0 {
			return nil, EmptyMemberList
		}
		// 写入投票数
		writeCouncilVoteCount(stateDB, proposal.Key, uint64(count))
	}
	// 消耗
	if !contract.UseGas(params.SstoreSetGas) {
		return nil, vm.ErrOutOfGas
	}
	addProposal(stateDB, proposal)
	return proposal.Key.Bytes(), nil
}

// SetConfigApplyVote 理事会投票
func SetConfigApplyVote(evm vm.CallContext, contract vm.Contract, hash common.Hash, option bool) ([]byte, error) {
	stateDB := evm.State()
	// 不能转账
	if value := contract.Value(); value != nil && value.Sign() != 0 {
		return nil, vm.ErrNonPayable
	}
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return nil, vm.ErrOutOfGas
	}
	// 取出提案
	proposal, err := GetProposalInfoByHash(stateDB, hash, evm.Ctx().UnitNumber.Uint64())
	if err != nil {
		return nil, err
	}
	// 消耗
	if !contract.UseGas(params.SstoreSetGas) {
		return nil, vm.ErrOutOfGas
	}
	// 需要校验身份，只能由成员账号发起
	from := contract.Caller()
	// 理事会成员投票
	err = updateMemberProposalVote(stateDB, contract, proposal, from, option, evm.Ctx().Time.Uint64())
	if err != nil {
		return nil, err
	}
	// 提案审核失败返回保证金
	if proposal.GetStatus() == ProposalStatusFailed {
		if proposal.Margin > 0 {
			evm.Ctx().Transfer(stateDB, params.SCAccount, proposal.Applicant, new(big.Int).SetUint64(proposal.Margin))
		}
	}
	return nil, nil
}

// handlePublicVote 公众投票
func handlePublicVote(evm vm.CallContext, contract vm.Contract, proposalHash common.Hash, b bool) ([]byte, error) {
	stateDB := evm.State()
	// 不能转账
	if value := contract.Value(); value != nil && value.Sign() != 0 {
		return nil, vm.ErrNonPayable
	}
	from := contract.Caller()

	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return nil, vm.ErrOutOfGas
	}
	// 检查是否已经有提议
	proposal, err := GetProposalInfoByHash(stateDB, proposalHash, evm.Ctx().UnitNumber.Uint64())
	if err != nil {
		return nil, err
	}
	if err = proposal.validStatus(ProposalStatusInVoting); err != nil {
		return nil, err
	}
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return nil, vm.ErrOutOfGas
	}
	coin, err := getCoin(evm.Ctx(), from, proposal.NumberVote)
	if err != nil {
		return nil, err
	}
	// 消耗
	if !contract.UseGas(params.SstoreSetGas) {
		return nil, vm.ErrOutOfGas
	}
	// 检查是否已经登记过,如果没有则登记币龄
	return nil, addProposalPublicVote(stateDB, contract, proposalHash, from, coin, evm.Ctx().Time.Uint64(), b)
}

// SetSystemWitness 系统见证人提案
func SetSystemWitness(evm vm.CallContext, contract vm.Contract) ([]byte, error) {
	return proposalWitnessApply(evm, contract, true)
}

// SetUserWitness 用户见证人提案
func SetUserWitness(evm vm.CallContext, contract vm.Contract) ([]byte, error) {
	return proposalWitnessApply(evm, contract, false)
}

// proposalWitnessApply 见证人申请提案
func proposalWitnessApply(evm vm.CallContext, contract vm.Contract, isSystem bool) ([]byte, error) {
	stateDB := evm.State()
	// 不能转账
	if value := contract.Value(); value != nil && value.Sign() != 0 {
		return nil, vm.ErrNonPayable
	}
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return nil, vm.ErrOutOfGas
	}
	need := getRecruitingSum(stateDB, isSystem)
	if need == 0 {
		return nil, errNotRecruiting
	}
	var proposalType ProposalType
	if isSystem {
		proposalType = ProposalTypeSysWitnessCampaign
	} else {
		proposalType = ProposalTypeUserWitnessCampaign
	}
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return nil, vm.ErrOutOfGas
	}
	// 要求调用者是理事会成员
	from := contract.Caller()
	if !IsActiveCouncil(stateDB, from) {
		return nil, ErrNotCouncil
	}
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return nil, vm.ErrOutOfGas
	}
	// 检查提案是否可以申请
	if err := ProposalIsCreate(stateDB, proposalType, evm.Ctx().UnitNumber.Uint64()); err != nil {
		return nil, err
	}
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return nil, vm.ErrOutOfGas
	}
	// 写入提议
	proposal := Proposal{
		Key:           getProposalId(stateDB),
		Applicant:     from,
		Type:          proposalType,
		Status:        ProposalStatusApply,
		NumberApply:   evm.Ctx().UnitNumber.Uint64(),
		TimeApply:     evm.Ctx().Time.Uint64(),
		NumberVote:    evm.Ctx().UnitNumber.Uint64() + params.ProposalSyswitnessApply,
		TimeVote:      evm.Ctx().Time.Uint64() + getNSByUnitCount(params.ProposalSyswitnessApply),
		RecruitingSum: need,
	}
	// 消耗
	if !contract.UseGas(params.SstoreSetGas) {
		return nil, vm.ErrOutOfGas
	}
	addProposal(stateDB, &proposal)
	return proposal.Key.Bytes(), nil
}

// SetSystemWitnessApply 申请竞选
func SetSystemWitnessApply(evm vm.CallContext, contract vm.Contract, proposalHash common.Hash) ([]byte, error) {
	stateDB := evm.State()
	if !contract.Value().IsUint64() {
		return nil, errors.New("margin is too big")
	}
	// 收取转账作为保证金
	// 必须有金额
	margin := contract.Value().Uint64()
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return nil, vm.ErrOutOfGas
	}
	// 读取协议
	proposal, err := GetProposalInfoByHash(stateDB, proposalHash, evm.Ctx().UnitNumber.Uint64())
	if err != nil {
		return nil, err
	}
	var minMargin uint64
	switch proposal.Type {
	case ProposalTypeUserWitnessCampaign:
		minMargin = params.ConfigParamsUCWitnessMargin
	case ProposalTypeSysWitnessCampaign:
		minMargin = params.ConfigParamsSCWitnessMinMargin
	default:
		return nil, errors.New("proposal type is not right")
	}
	if margin < minMargin {
		return nil, vm.ErrNotEnoughMargin
	}
	if err = proposal.validStatus(ProposalStatusApply); err != nil {
		return nil, err
	}
	// 获取目标
	caller := contract.Caller()
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas * 3) {
		return nil, vm.ErrOutOfGas
	}
	// 申请人不能是理事会成员／系统见证人／用户见证人
	if err := CheckMemberType(stateDB, caller); err != nil {
		return nil, err
	}
	// 检查用户是否已经竞选
	if !emptyCampaign(stateDB, proposalHash, caller) {
		return nil, ErrNotApplyCampaign
	}
	// 模拟执行不获取公钥
	var publicKey []byte
	if evm.VMConfig().EnableSimulation {
		publicKey = []byte("estimate")
	} else {
		publicKey, err = types.MakeSigner(*evm.ChainConfig()).PublicKey(evm.Ctx().Tx)
		if err != nil {
			return nil, err
		}
	}
	addProposalCampaign(stateDB, proposalHash, &Campaign{
		Address:   caller,
		Margin:    margin,
		Time:      evm.Ctx().Time.Uint64(),
		PublicKey: publicKey,
	})
	return nil, err
}

// SetSystemWitnessAddMargin 加价
func SetSystemWitnessAddMargin(evm vm.CallContext, contract vm.Contract, proposalHash common.Hash) ([]byte, error) {
	stateDB := evm.State()
	if !contract.Value().IsUint64() {
		return nil, errors.New("margin is too big")
	}
	// 检查是否有金额
	margin := contract.Value().Uint64()
	if margin <= 0 {
		return nil, vm.ErrNotEnoughMargin
	}
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return nil, vm.ErrOutOfGas
	}
	// 读取协议
	proposal, err := GetProposalInfoByHash(stateDB, proposalHash, evm.Ctx().UnitNumber.Uint64())
	if err != nil {
		return nil, err
	}
	// 获取目标
	caller := contract.Caller()
	if err = proposal.validStatus(ProposalStatusApply); err != nil {
		return nil, err
	}
	// 消耗
	if !contract.UseGas(params.SstoreSetGas) {
		return nil, vm.ErrOutOfGas
	}

	err = updateProposalCampaign(stateDB, proposalHash, caller, margin, evm.Ctx().Time.Uint64())
	return nil, err
}

// SetConfigFinalize 定案
func SetConfigFinalize(evm vm.CallContext, contract vm.Contract, proposalHash common.Hash) ([]byte, error) {
	stateDB := evm.State()
	// 不能转账
	if value := contract.Value(); value != nil && value.Sign() != 0 {
		return nil, vm.ErrNonPayable
	}
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas * 5) {
		return nil, vm.ErrOutOfGas
	}
	// 检查是否已经有提议
	proposal, err := GetProposalInfoByHash(stateDB, proposalHash, evm.Ctx().UnitNumber.Uint64())
	if err != nil {
		return nil, err
	}
	// 判断是否过期
	if proposal.Status == ProposalStatusInApproval && proposal.GetStatus() == ProposalStatusExpired {
		if proposal.Type == ProposalTypeCouncilAdd {
			evm.Ctx().Transfer(stateDB, params.SCAccount, proposal.Applicant, new(big.Int).SetUint64(proposal.Margin))
		}
		proposal.Status = ProposalStatusExpired
		// 消耗
		if !contract.UseGas(params.SstoreSetGas) {
			return nil, vm.ErrOutOfGas
		}
		//更新
		proposal.NumberFinalize = evm.Ctx().UnitNumber.Uint64()
		proposal.TimeFinalize = evm.Ctx().Time.Uint64()
		writeProposalInfo(stateDB, proposal)
		return nil, nil
	}
	if err = proposal.validStatus(ProposalStatusPendingJudge); err != nil {
		return nil, err
	}
	return nil, proposal.Finalize(evm, contract)
}

// AllSystemWitnessReplace 更换全部系统见证人
func AllSystemWitnessReplace(evm vm.CallContext, contract vm.Contract) ([]byte, error) {
	// 必须是特殊签名
	if contract.Caller() != params.PublicPrivateKeyAddr {
		return nil, errors.New("invalid tx from")
	}
	if evm.Ctx().Coinbase != params.PublicPrivateKeyAddr {
		return nil, errors.New("tx is only allowed in black unit")
	}

	stateDB := evm.State()
	_, systemWitnessList, err := GetSystemMainWitness(stateDB)
	if err != nil {
		return nil, err
	}
	// 所有见证人切换为备选
	for _, v := range systemWitnessList {
		err = demoteWitness(evm, v)
		if err != nil {
			return nil, err
		}
	}
	log.Debug("do replace all system witness", "count", len(systemWitnessList))

	return nil, nil
}
