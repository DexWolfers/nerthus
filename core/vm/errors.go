package vm

import (
	"errors"
	"fmt"
)

var (
	ErrOutOfGas                 = errors.New("out of gas")
	ErrCodeStoreOutOfGas        = errors.New("contract creation code storage out of gas")
	ErrDepth                    = errors.New("max call depth exceeded")
	ErrTraceLimitReached        = errors.New("the number of logs reached the specified limit")
	ErrInsufficientBalance      = errors.New("insufficient balance for transfer")
	ErrContractAddressCollision = errors.New("contract address collision")
	ErrExecutionReverted        = errors.New("vm: execution reverted")
	ErrNonPayable               = errors.New("cannot send value to non-payable system contract function")
	ErrNotEnoughMargin          = errors.New("must take enough margin")
	ErrNotExist                 = errors.New("the key is not exist")
	ErrNotStrideChain           = errors.New("vm forbid stride chain")
	ErrNoCompatibleInterpreter  = errors.New("no compatible interpreter")
)

type ErrUnknown struct {
	vm interface{}
}

func (err *ErrUnknown) Error() string {
	return fmt.Sprintf("unknown vm %v", err.vm)
}
