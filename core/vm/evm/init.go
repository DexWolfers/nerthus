package evm

import (
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/params"
)

const (
	Name        = "evm"
	Mgic uint16 = 0x6060
)

type create struct {
	mgic uint16
}

func (c *create) Magic() uint16 {
	return c.mgic
}
func (c *create) New(ctx vm.Context, statedb vm.StateDB, chainConfig *params.ChainConfig, vmConfig vm.Config) (vm.CallContext, error) {
	return NewEVM(ctx, statedb, chainConfig, vmConfig), nil
}

func init() {
	vm.Register(Name, &create{0x6060})
	vm.Register("evm2", &create{0x6080})
}
