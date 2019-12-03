// Author:@kulics
package witness

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/consensus"

	"gitee.com/nerthus/nerthus/core/types"
)

type Status uint8

const (
	stopped Status = iota
	running
)

type IWitnessFlow interface {
	Start() error
	Stop() error
	IsRunning() bool
	AddUnit(unit *types.Unit)
	Engine() consensus.Engine
}

type Schedule interface {
	ChainWitness() common.AddressList
	WitnessOnlines() int
}
