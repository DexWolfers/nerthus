package sc

import (
	"fmt"
	"math/big"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"
)

const newProposalGas = params.TxGasContractCreation * 3 //创建新提案最低开销

type call = func(vms vm.CallContext, contract vm.Contract, input []byte) ([]byte, error)

var callFuncs = make(map[string]call)

func init() {
	callFuncs[FuncIdSystemHeartbeat] = func(vms vm.CallContext, contract vm.Contract, input []byte) (ret []byte, err error) {
		return SystemHeartbeat(vms, contract)
	}
	callFuncs[FuncIdMemberHeartbeat] = func(vms vm.CallContext, contract vm.Contract, input []byte) (ret []byte, err error) {
		return MemberHeartbeat(vms, contract)
	}
	callFuncs[FuncIdSetMemberJoinApply] = func(vms vm.CallContext, contract vm.Contract, input []byte) (ret []byte, err error) {
		// 固定消耗
		if !contract.UseGas(newProposalGas) {
			return nil, vm.ErrOutOfGas
		}
		return SetMemberJoinApply(vms, contract)
	}
	callFuncs[FuncIdCounciExit] = func(vms vm.CallContext, contract vm.Contract, input []byte) ([]byte, error) {
		return nil, handleCouncilExit(vms, contract)
	}
	callFuncs[FuncIdSetMemberRemoveApply] = func(vms vm.CallContext, contract vm.Contract, input []byte) ([]byte, error) {
		// 固定消耗
		if !contract.UseGas(newProposalGas) {
			return nil, vm.ErrOutOfGas
		}
		var p common.Address
		err := abiObj.Methods["SetMemberRemoveApply"].Inputs.Unpack(&p, input[4:])
		if err != nil {
			return nil, err
		}
		return SetMemberRemoveApply(vms, contract, p)
	}
	callFuncs[FuncIdSetConfigApply] = func(vms vm.CallContext, contract vm.Contract, input []byte) ([]byte, error) {
		// 固定消耗
		if !contract.UseGas(newProposalGas) {
			return nil, vm.ErrOutOfGas
		}
		type P struct {
			//Key   common.Hash
			Key   string
			Value uint64
		}
		var p P
		err := abiObj.Methods["SetConfigApply"].Inputs.Unpack(&p, input[4:])
		if err != nil {
			return nil, err
		}
		return SetConfigApply(vms, contract, p.Key, p.Value)
	}

	callFuncs[FuncIdSetConfigApplyVote] = func(vms vm.CallContext, contract vm.Contract, input []byte) ([]byte, error) {
		type P struct {
			Hash    common.Hash
			Opinion bool
		}
		var p P
		err := abiObj.Methods["SetConfigApplyVote"].Inputs.Unpack(&p, input[4:])
		if err != nil {
			return nil, err
		}
		return SetConfigApplyVote(vms, contract, p.Hash, p.Opinion)
	}
	callFuncs[FuncIdSetConfigVote] = func(vms vm.CallContext, contract vm.Contract, input []byte) ([]byte, error) {
		type P struct {
			Hash    common.Hash
			Opinion bool
		}
		var p P
		err := abiObj.Methods["SetConfigVote"].Inputs.Unpack(&p, input[4:])
		if err != nil {
			return nil, err
		}
		return handlePublicVote(vms, contract, p.Hash, p.Opinion)
	}
	callFuncs[FuncIdSetConfigFinalize] = func(vms vm.CallContext, contract vm.Contract, input []byte) ([]byte, error) {
		var p common.Hash
		err := abiObj.Methods["SetConfigFinalize"].Inputs.Unpack(&p, input[4:])
		if err != nil {
			return nil, err
		}
		return SetConfigFinalize(vms, contract, p)
	}
	callFuncs[FuncIdSetSystemWitness] = func(vms vm.CallContext, contract vm.Contract, input []byte) ([]byte, error) {
		return SetSystemWitness(vms, contract)
	}
	callFuncs[FuncIdSetUserWitness] = func(vms vm.CallContext, contract vm.Contract, input []byte) ([]byte, error) {
		return SetUserWitness(vms, contract)
	}
	callFuncs[FuncIdSetSystemWitnessApply] = func(vms vm.CallContext, contract vm.Contract, input []byte) ([]byte, error) {
		var p common.Hash
		err := abiObj.Methods["SetSystemWitnessApply"].Inputs.Unpack(&p, input[4:])
		if err != nil {
			return nil, err
		}
		return SetSystemWitnessApply(vms, contract, p)
	}
	callFuncs[FuncIdSetSystemWitnessAddMargin] = func(vms vm.CallContext, contract vm.Contract, input []byte) ([]byte, error) {
		var p common.Hash
		err := abiObj.Methods["SetSystemWitnessAddMargin"].Inputs.Unpack(&p, input[4:])
		if err != nil {
			return nil, err
		}
		return SetSystemWitnessAddMargin(vms, contract, p)
	}
	callFuncs[FuncIdAllSystemWitnessReplace] = func(vms vm.CallContext, contract vm.Contract, input []byte) ([]byte, error) {
		return AllSystemWitnessReplace(vms, contract)
	}
	callFuncs[FuncCancelWitnessID] = func(vms vm.CallContext, contract vm.Contract, input []byte) ([]byte, error) {
		return CancelWitness(vms, contract)
	}
	callFuncs[FuncIdRemoveInvalid] = func(vms vm.CallContext, contract vm.Contract, input []byte) ([]byte, error) {
		list, err := GetContractInput(input)
		if err != nil {
			return nil, err
		}
		return nil, handleRemoveTimeout(vms, contract, list.([]common.Address))

	}
	callFuncs[FuncReplaceWitnessID] = func(vms vm.CallContext, contract vm.Contract, input []byte) ([]byte, error) {
		target, err := GetReplaseWitnessTraget(contract.Caller(), input)
		if err != nil {
			return nil, err
		}
		groupIndex, err := replaceWitness(vms, contract, target, false)
		if err != nil {
			return nil, err
		}
		return abiObj.Methods["ReplaceWitness"].Outputs.Pack(groupIndex)
	}
	callFuncs[FuncApplyWitnessID] = func(vms vm.CallContext, contract vm.Contract, input []byte) ([]byte, error) {
		groupIndex, err := ApplyWitness(vms, contract, input)
		if err != nil {
			return nil, err
		}
		return abiObj.Methods["ApplyWitness"].Outputs.Pack(groupIndex)
	}
	callFuncs[FuncIDApplyWitnessWay2] = func(vms vm.CallContext, contract vm.Contract, input []byte) ([]byte, error) {
		var chain common.Address
		if err := abiObj.Methods["applyWitnessWay2"].Inputs.Unpack(&chain, input[4:]); err != nil {
			return nil, fmt.Errorf("invalid input length,can not convert to address,%v", err)
		}
		return nil, HandleApplyWitnessWay2(vms, contract, chain)

	}
	callFuncs[FuncReplaceWitnessExecuteID] = func(vms vm.CallContext, contract vm.Contract, input []byte) ([]byte, error) {
		return nil, ReplaceWitnessExecute(vms, contract, input)
	}
	callFuncs[FuncReportDishonest] = func(vms vm.CallContext, contract vm.Contract, input []byte) ([]byte, error) {
		return nil, SystemReportDishonest(vms, contract, input)
	}
	callFuncs[FuncSettlementID] = func(vms vm.CallContext, contract vm.Contract, input []byte) ([]byte, error) {
		if b, err := GetAdditional(vms, contract); err != nil {
			return b, err
		}
		b, err := Settlement(vms, contract)
		if err != nil && err != ErrNoWintessInfo {
			return b, err
		}
		return b, nil
	}
	callFuncs[FuncCreateContract] = func(vms vm.CallContext, contract vm.Contract, input []byte) ([]byte, error) {
		// 合约创建, 生成见证人，传入的数据需包含新的合约账号
		newContractAddr, groupIndex, err := CreateContract(vms, contract, input[4:])
		if err != nil {
			return nil, err
		}
		return abiObj.Methods["CreateContract"].Outputs.Pack(newContractAddr, groupIndex)
	}

}

// ContractHandler 合约执行处理
func ContractHandler(vms vm.CallContext, contract vm.Contract, input []byte, value *big.Int) ([]byte, error) {
	var funcId string
	ret, err := func() ([]byte, error) {
		if !contract.UseGas(params.ExpByteGas) {
			return nil, vm.ErrOutOfGas
		}

		if len(input) < 4 {
			return nil, fmt.Errorf("invalid input length")
		}
		if !contract.UseGas(params.CreateDataGas) {
			return nil, vm.ErrOutOfGas
		}
		funcId = common.Bytes2Hex(input[:4])
		call, ok := callFuncs[funcId]
		if !ok {
			return nil, fmt.Errorf("undefined contract function:%s", funcId)
		}
		return call(vms, contract, input)
	}()

	log.Debug("handle system contract method",
		"method", funcId,
		"chain", vms.Ctx().UnitMC,
		"number", vms.Ctx().UnitNumber,
		"parent", vms.Ctx().UnitParent,
		"txHash", vms.Ctx().Tx.Hash(),
		"caller", contract.Caller(),
		"err", err)

	if err != nil {
		if err != vm.ErrOutOfGas && !vms.VMConfig().EnableSimulation {
			//需要修改，是的失败情况时能正常收取已用Gas
			err = vm.ErrExecutionReverted
		}
	}
	return ret, err
}
