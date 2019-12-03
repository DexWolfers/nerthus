package witness

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/witness/service/rwit"
)

// 用于替换用户见证人指定最后稳定位置的 Backend backed
type replaceWitnessBackend struct {
	*WitnessSchedule
}

func (b *replaceWitnessBackend) WitnessSet() rwit.WitnessSet {
	return b.witnessSet
}

func (b *replaceWitnessBackend) CurrWitness() common.Address {
	return b.curWitness
}

func (b *replaceWitnessBackend) IsWitness(witness common.Address) bool {
	if witness == b.curWitness {
		return true
	}
	return b.ChainWitness().Have(witness)
}

func (b *replaceWitnessBackend) Sign(info types.SignHelper) error {
	return signThat(b.nts, info)
}
