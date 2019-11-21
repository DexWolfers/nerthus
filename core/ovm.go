// Author: @ysqi

package core

import (
	"errors"
	"math/big"

	"sync"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/params"

	_ "gitee.com/nerthus/nerthus/core/vm/evm"
)

// CanTransfer 检查指定账户下余额是否足够转移amount，不考虑燃油费.
func CanTransfer(db vm.StateDB, addr common.Address, amount *big.Int) bool {
	return db.GetBalance(addr).Cmp(amount) >= 0
}

// Transfer 转账，在db中扣减sender余额，增加recipient余额。
func Transfer(db vm.StateDB, sender, recipient common.Address, amount *big.Int) {
	db.SubBalance(sender, amount)
	db.AddBalance(recipient, amount)
}

// FreeGas 获取可免费试用的Gas量，当前主要用于在Pow特殊交易时抵扣Gas
// TODO: 可以简化降低data的复制，结合IsFreeTx使用
func FreeGas(recipient common.Address, unitNumber uint64, data []byte) uint64 {
	// 只有系统链上的特殊交易才允许豁免交易费
	if recipient != params.SCAccount {
		return 0
	}
	if unitNumber == 0 {
		return math.MaxUint64
	}
	if len(data) < 4 {
		return 0
	}
	//当前系统链上的两种交易才需要免费
	switch common.Bytes2Hex(data[:4]) {
	case sc.FuncIdMemberHeartbeat: //理事会心跳
		return 5000000
	case sc.FuncIdSystemHeartbeat:
		return 5000000
	case sc.FuncReplaceWitnessID, sc.FuncReplaceWitnessExecuteID, sc.FuncApplyWitnessID:
		return 5000000
	case sc.FuncIdRemoveInvalid: //踢出无效超时的见证人/理事
		return 5000000
	case sc.FuncIdAllSystemWitnessReplace: // 替换全部系统见证人
		return 5000000
	case sc.FuncReportDishonest:
		return 5000000
	}
	return 0
}

// 判断交易是否是免费交易，免费交易则不需要检查 GasLimit
func IsFreeTx(tx *types.Transaction) bool {
	return FreeGas(tx.To(), 1, tx.Data4()) > 0
}

// MessageIsToInnerAccount 检查消息是否是发送至内部账户
func MessageIsToInnerAccount(to common.Address) bool {
	return to == params.SCAccount
}

type innerContractHandler = func(vm.CallContext, vm.Contract, []byte, *big.Int) ([]byte, error)

var (
	innerContracts = make(map[common.Address]innerContractHandler)
	contractMu     sync.Mutex
)

// 注册内置合约执行方法
func ContractRegister(contract common.Address, handler innerContractHandler) {
	contractMu.Lock()
	defer contractMu.Unlock()
	_, ok := innerContracts[contract]
	if ok {
		panic("register contract function handle does exist")
	}
	innerContracts[contract] = handler
}

// MessageIsToInnerAccount 处理内部合约
func HandleInnerMessage(vm vm.CallContext, contract vm.Contract, input []byte, value *big.Int) (ret []byte, err error) {
	if contract.Address() == params.SCAccount {
		return sc.ContractHandler(vm, contract, input, value)
	}
	return nil, errors.New("undefined internal contract address")
}

// NewEVMContext 创建一个新虚拟机的上下文
func NewEVMContext(msg Message, header *types.Header, chainContext vm.ChainContext, tx *types.Transaction) vm.Context {
	return vm.Context{
		CanTransfer:        CanTransfer,
		Transfer:           Transfer,
		IsToInnerAccount:   MessageIsToInnerAccount,
		HandleInnerMessage: HandleInnerMessage,
		FreeGas:            FreeGas,
		Chain:              chainContext,
		GetHash:            GetHashFn(header, chainContext),
		Origin:             msg.From(),
		UnitMC:             header.MC,
		UnitParent:         header.ParentHash,
		UnitSCHash:         header.SCHash,
		SCNumber:           header.SCNumber,
		UnitNumber:         new(big.Int).SetUint64(header.Number),
		Time:               new(big.Int).SetUint64(header.Timestamp),
		GasLimit:           msg.Gas(),
		GasPrice:           msg.GasPrice(),
		Coinbase:           header.Proposer,
		Tx:                 tx,
	}
}

// GetHashFn returns a GetHashFunc which retrieves header hashes by number
func GetHashFn(ref *types.Header, chain vm.ChainContext) func(n uint64) common.Hash {
	var cache map[uint64]common.Hash

	return func(n uint64) common.Hash {
		if n == 0 {
			return chain.GenesisHash()
		}
		// If there's no hash cache yet, make one
		if cache == nil {
			cache = map[uint64]common.Hash{
				ref.Number - 1: ref.ParentHash,
			}
		}
		// Try to fulfill the request from the cache
		if hash, ok := cache[n]; ok {
			return hash
		}
		// Not cached, iterate the blocks and cache the hashes
		for header, number := chain.GetHeader(ref.ParentHash),
			ref.Number-1; header != nil && header.Number == number; header, number = chain.GetHeader(header.ParentHash), header.Number-1 {
			cache[number-1] = header.ParentHash
			if n == number-1 {
				return header.ParentHash
			}
		}
		return common.Hash{}
	}
}
