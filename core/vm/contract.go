package vm

import (
	"math/big"

	"gitee.com/nerthus/nerthus/common"
)

// ContractRef is a reference to the contract's backing object
type ContractRef interface {
	Address() common.Address
}

// AccountRef implements ContractRef.
//
// Account references are used during EVM initialisation and
// it's primary use is to fetch addresses. Removing this object
// proves difficult because of the cached jump destinations which
// are fetched from the parent contract (i.e. the caller), which
// is a ContractRef.
type AccountRef common.Address

// Address casts AccountRef to a Address
func (ar AccountRef) Address() common.Address { return (common.Address)(ar) }

// Contract represents an ethereum contract in the state database. It contains
// the the contract code, calling arguments. Contract implements ContractRef
type Contract interface {
	// Caller returns the caller of the contract.
	//
	// Caller will recursively call caller when the contract is a delegate
	// call, including that of caller's caller.
	Caller() common.Address

	// UseGas attempts the use gas and subtracts it and returns true on success
	UseGas(gas uint64) (ok bool)

	// Address returns the contracts address
	Address() common.Address
	//Value returns the contracts value (sent to it from it's caller)
	Value() *big.Int
}
