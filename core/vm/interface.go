package vm

import (
	"math/big"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"
)

type (
	CanTransferFunc func(StateDB, common.Address, *big.Int) bool
	TransferFunc    func(StateDB, common.Address, common.Address, *big.Int)
	// GetHashFunc returns the nth block hash in the blockchain
	// and is used by the BLOCKHASH EVM op code.
	GetHashFunc func(uint64) common.Hash
	// 可免费使用的Gas量
	FreeGasFunc func(recipient common.Address, unitNumber uint64, data []byte) uint64
)

// StateDB 虚拟机运行时状态数据操作
type StateDB interface {
	// CreateAccount 创建账户
	CreateAccount(common.Address)

	// SubBalance 减少账户余额
	SubBalance(common.Address, *big.Int)
	// AddBalance 增加账户余额
	AddBalance(common.Address, *big.Int)
	// GetBalance 获取账户余额
	GetBalance(common.Address) *big.Int

	// GetNonce 获取账户当前Nonce
	GetNonce(common.Address) uint64
	// SetNonce 更新对应账户的Nonce
	SetNonce(common.Address, uint64)

	// GetCodeHash 获取合约账户代码Hash
	GetCodeHash(common.Address) common.Hash
	// GetCode 获取合约账户代码
	GetCode(common.Address) []byte
	// SetCode 设置合约账户代码
	SetCode(common.Address, []byte)
	// GetCodeSize 获取合约账户代码字节数
	GetCodeSize(common.Address) int

	// AddRefund 添加退款费
	AddRefund(uint64)
	// GetRefund 获取退款费
	GetRefund() uint64
	SubRefund(gas uint64)

	// GetState 获取账户下key对应的value
	GetState(common.Address, common.Hash) common.Hash
	// SetState 设置账户下key对应的value
	SetState(common.Address, common.Hash, common.Hash)
	GetBigData(addr common.Address, key []byte) ([]byte, error)
	SetBigData(addr common.Address, key, value []byte) error

	// Suicide 销毁账户并告知是否销毁成功
	Suicide(common.Address) bool
	// HasSuicided 报告指定账户是否已被销毁
	HasSuicided(common.Address) bool
	// VerifyProof 校验对应的Key是否存在，如果存在则返回value否则返回error
	VerifyProof(addr common.Address, key common.Hash) (value common.Hash, err error)

	// Exist reports whether the given account exists in state.
	// Notably this should also return true for suicided accounts.
	// Exist 报告指定账户是否存在.
	// 已销毁账户也是存在的.
	Exist(common.Address) bool
	// Empty returns whether the given account is empty. Empty
	// Empty 报告指定账户是否为空. 空账户的定义是余额,nonce,代码均为0
	Empty(common.Address) bool

	// RevertToSnapshot 恢复State到指定位置
	RevertToSnapshot(int)
	// Snapshot 对State进行快照并返回当前State的标识符
	Snapshot() int

	// AddLog 记录日志
	AddLog(*types.Log)
	// AddPreimage 记录当前VM的 SHA3
	AddPreimage(common.Hash, []byte)

	// ForEachStorage 遍历指定账户下的存储信息
	ForEachStorage(common.Address, func(common.Hash, common.Hash) bool)

	// 获取底层db， 此方法慎用
	Database() ntsdb.Database

	// 返回当前 State 操作所关联的单元交易编号
	// 这需要调用 Prepare 才能设置正确关联。
	// 警告：不要过于依赖
	TxIndex() int

	// 返回当前 State 操作所关联的单元交易哈希
	// 这需要调用 Prepare 才能设置正确关联。
	// 警告：不要过于依赖
	TxHash() common.Hash

	// 获取记账记录
	// Author: @zwj
	GetCoinflows() types.Coinflows
	Clear()
}

// CallContext provides a basic interface for the VM calling conventions. The VM
// depends on this context being implemented for doing subcalls and initialising new VM contracts.
type CallContext interface {
	Ctx() Context
	State() StateDB

	// ChainConfig returns the environment's chain configuration
	ChainConfig() *params.ChainConfig

	VMConfig() Config

	// Cancel cancels any running EVM operation. This may be called concurrently and
	// it's safe to be called multiple times.
	Cancel()

	// Create creates a new contract using code as deployment code.
	Create(caller ContractRef, code []byte, gas uint64, value *big.Int) (ret []byte, contractAddr common.Address, leftOverGas uint64, err error)

	// Call executes the contract associated with the addr with the given input as
	// parameters. It also handles any necessary value transfer required and takes
	// the necessary steps to create accounts and reverses the state in case of an
	// execution error or failed value transfer.
	Call(caller ContractRef, addr common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error)

	// CallCode executes the contract associated with the addr with the given input
	// as parameters. It also handles any necessary value transfer required and takes
	// the necessary steps to create accounts and reverses the state in case of an
	// execution error or failed value transfer.
	//
	// CallCode differs from Call in the sense that it executes the given address'
	// code with the caller as context.
	CallCode(caller ContractRef, addr common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error)

	// DelegateCall executes the contract associated with the addr with the given input
	// as parameters. It reverses the state in case of an execution error.
	//
	// DelegateCall differs from CallCode in the sense that it executes the given address'
	// code with the caller as context and the caller is set to the caller of the caller.
	DelegateCall(caller ContractRef, addr common.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error)

	// StaticCall executes the contract associated with the addr with the given input
	// as parameters while disallowing any modifications to the state during the call.
	// Opcodes that attempt to perform such modifications will result in exceptions
	// instead of performing the modifications.
	StaticCall(caller ContractRef, addr common.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error)
}
