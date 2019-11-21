package evm

import "gitee.com/nerthus/nerthus/core/vm"

var (
	ErrOutOfGas                 = vm.ErrOutOfGas
	ErrCodeStoreOutOfGas        = vm.ErrCodeStoreOutOfGas
	ErrDepth                    = vm.ErrDepth
	ErrTraceLimitReached        = vm.ErrTraceLimitReached
	ErrInsufficientBalance      = vm.ErrInsufficientBalance
	ErrContractAddressCollision = vm.ErrContractAddressCollision
	ErrNoCompatibleInterpreter  = vm.ErrNoCompatibleInterpreter
	ErrExecutionReverted        = vm.ErrExecutionReverted
)
