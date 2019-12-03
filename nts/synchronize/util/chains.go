package util

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/rawdb"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"

	"github.com/hashicorp/golang-lru"
)

// 遍历获取所有链
func RangeChains(db ntsdb.Database, f func(chain common.Address, status rawdb.ChainStatus) bool) {
	rawdb.RangeChains(db, f)
}

var (
	hotChainIndexCache, _ = lru.New(200) //缓存部分链位置信息
)

// 获取链位置索引，如果链不存在则为返回 False
func GetChainIndex(db ntsdb.Database, chain common.Address) (uint64, bool) {
	if chain == params.SCAccount {
		return 0, true
	}
	v, ok := hotChainIndexCache.Get(chain)
	if ok {
		return v.(uint64), true
	}
	status, ok := rawdb.GetChainStatus(db, chain)
	if !ok {
		return 0, false
	}
	hotChainIndexCache.Add(chain, status.Index)
	return status.Index, true
}
