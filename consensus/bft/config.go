package bft

import (
	"math/big"

	configpkg "gitee.com/nerthus/nerthus/core/config"
	"gitee.com/nerthus/nerthus/params"
)

// 共识配置
type Config struct {
	ChainId            *big.Int `toml:"-"`
	RequestTimeout     uint64   `toml:",omitempty"` // 等待提案的超时时间（秒）
	ConsensusTimeout   uint64   `toml:",omitempty"` // 二阶段投票的等待超时时间（秒）
	BlockPeriod        uint64   `toml:",omitempty"` // 单元之间最小间隔时间
	ParallelChainLimit int      `toml:"-"`          // 并行共识链数量限制
}

var DefaultConfig = Config{
	RequestTimeout:     10,                             //秒
	ConsensusTimeout:   60,                             //秒
	BlockPeriod:        params.OneNumberHaveSecondByUc, //秒
	ParallelChainLimit: 2000,
}

func NewConfig(isSys bool, chainId *big.Int) *Config {
	config := DefaultConfig
	if isSys {
		config.BlockPeriod = params.OneNumberHaveSecondBySys
	}
	config.ChainId = chainId
	config.ParallelChainLimit = configpkg.MustInt("runtime.witness_at_work_chains", config.ParallelChainLimit)
	if configpkg.InStress {
		config.RequestTimeout = 60 * 2
	}
	return &config
}
