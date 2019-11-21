// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package runtime

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/core/vm/evm"
)

func NewEnv(cfg *Config) *evm.EVM {
	context := vm.Context{
		CanTransfer: core.CanTransfer,
		Transfer:    core.Transfer,
		GetHash:     func(uint64) common.Hash { return common.Hash{} },

		Origin:     cfg.Origin,
		Coinbase:   cfg.Coinbase,
		UnitNumber: cfg.BlockNumber,
		Time:       cfg.Time,
		GasLimit:   cfg.GasLimit,
		GasPrice:   cfg.GasPrice.Uint64(),
	}

	return evm.NewEVM(context, cfg.State, cfg.ChainConfig, cfg.EVMConfig.Config)
}
