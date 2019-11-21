package core

import (
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
)

// Validator is an interface which defines the standard for unit validation. It
// is only responsible for validating unit contents, as the header validation is
// done by the specific consensus engines.
//
type Validator interface {
	// ValidateBody validates the given unit's content.
	ValidateBody(unit *types.Unit) error

	// ValidateState validates the given statedb and optionally the receipts and
	// gas used.
	ValidateState(unit, parent *types.Unit, state *state.StateDB, receipts types.Receipts, usedGas uint64) error

	//可以设置新的交易发送者校验器
	SetTxSenderValidator(func(tx *types.Transaction) error)
	ValidateTxSender(tx *types.Transaction) error
}

// Processor is an interface for processing blocks using a given initial state.
//
// Process takes the unit to be processed and the statedb upon which the
// initial state is based. It should return the receipts generated, amount
// of gas used in the process and return an error if any of the internal rules
// failed.
type Processor interface {
	Process(unit *types.Unit, statedb *state.StateDB, cfg vm.Config) (types.Receipts, []*types.Log, uint64, error)
}
