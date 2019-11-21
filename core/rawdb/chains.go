package rawdb

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/config"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/rlp"
	"github.com/pkg/errors"
	"github.com/syndtr/goleveldb/leveldb/util"
)

/*
需要全局维护所有的链地址，以便快速获取链信息。

实现的初衷：在查找两个节点见链同步差异时，为了快速超出差异部分，需要借助链地址维护。

提供两个功能：
1. 有序获取所有链地址
2. 获取链地址位置

因此在数据存储时，做如下设计：
--------------------------------------
|    Key         |         Value     |
--------------------------------------
| c_i_index        |  chain address   |
--------------------------------------
| c_v_address      |   ChainStatus     |
--------------------------------------

1. 在检测到有新地址出现时，WriteNewChain(chain,system number)
2. 在链出现第一个单元时，  SetChainActive(chain)

*/

var (
	allChainsPrefix   = []byte("ac")
	allChainsCountKey = []byte("ac_c")
	emptyChainFlag    = byte(1)
)

// 链状态
type ChainStatus struct {
	Active      bool //如果为 TRUE，则说明该链已经有出现一个单元
	ApplyNumber uint64
	Index       uint64
}

func GetChainCount(db ntsdb.Getter) uint64 {
	v := Get(db, allChainsCountKey)
	if len(v) == 0 {
		return 0
	}
	return common.BytesToUInt64(v)
}
func chainAddressKeyPrefix() []byte {
	return makeKey(allChainsPrefix, "_i")
}
func chainAddressKey(index uint64) []byte {
	return makeKey(chainAddressKeyPrefix(), index)
}
func chainStatusKey(chain common.Address) []byte {
	return makeKey(allChainsPrefix, "_s", chain)
}

// 获取链状态，如果不存在则返还 false
func GetChainStatus(db ntsdb.PutGetter, chain common.Address) (ChainStatus, bool) {
	b := Get(db, chainStatusKey(chain))
	if len(b) == 0 {
		if config.IgnoreAcctCheck {
			return ChainStatus{
				Active:      false,
				ApplyNumber: 0,
				Index:       1,
			}, true
		}
		return ChainStatus{}, false
	}
	info := new(ChainStatus)
	err := rlp.DecodeBytes(b, info)
	if err != nil {
		gameover(err, "failed to decode")
	}
	return *info, true
}

func RangeChains(db ntsdb.Database, f func(chain common.Address, status ChainStatus) bool) {
	// 从 Start 到固定数量的单元哈希
	iter := mustLDB(db).NewIterator(util.BytesPrefix(chainAddressKeyPrefix()), nil)
	defer iter.Release()
	for iter.Next() {
		chain := common.BytesToAddress(iter.Value())

		status, ok := GetChainStatus(db, chain)
		if !ok {
			gameover(errors.Errorf("missing chain status %s", chain), "")
		}
		if !f(chain, status) {
			break
		}
	}

}
