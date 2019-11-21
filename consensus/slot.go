package consensus

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/types"
)

// IsPackagingUnitLeader 判断是否是将军(出单元者)
func IsPackagingUnitLeader(chain ChainReader, genAddr common.Address, header *types.Header) (bool, error) {
	return header.Proposer == genAddr, nil
}

// getChainWitness 获取指定地址的当前见证人列表
// Author: @ysqi
func getChainWitness(chain ChainReader, mc common.Address, scHash common.Hash) (lib []common.Address, number int, err error) {
	lib, err = chain.GetChainWitnessLib(mc, scHash)
	if err != nil {
		return
	}
	number, _ = sc.GetVoteNum(mc)
	return
}
