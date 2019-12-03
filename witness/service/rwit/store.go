package rwit

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/rlp"
)

// 链信息
type PendingChain struct {
	Chain         common.Address
	SysUnitHash   common.Hash
	SysUnitNumber uint64
}
type pendingStore struct {
	UnitHash   common.Hash
	UnitNumber uint64
}

type PendingChainSet struct {
	db  ntsdb.Database
	set ntsdb.DataSet
}

func newPendingChainSet(db ntsdb.Database) *PendingChainSet {
	return &PendingChainSet{
		db:  db,
		set: ntsdb.NewSet([]byte("_p_c_s_")),
	}
}

// 存储链进行中信息
func (pc *PendingChainSet) Add(chain common.Address, unitID types.UnitID) {
	//如果存在旧数据，则选择性调整
	if old, exist := pc.Get(chain); exist && old.SysUnitNumber >= unitID.Height {
		return
	}

	value, err := rlp.EncodeToBytes(pendingStore{UnitNumber: unitID.Height, UnitHash: unitID.Hash})
	if err != nil {
		panic(err)
	}
	pc.set.AddWithValue(pc.db, chain.Bytes(), value)
}

func (pc *PendingChainSet) Get(chain common.Address) (PendingChain, bool) {
	value := pc.set.GetValue(pc.db, chain.Bytes())
	if len(value) == 0 {
		return PendingChain{}, false
	}
	var info pendingStore
	if err := rlp.DecodeBytes(value, &info); err != nil {
		panic(err)
	}
	return PendingChain{
		Chain:         chain,
		SysUnitHash:   info.UnitHash,
		SysUnitNumber: info.UnitNumber,
	}, true
}

// 删除链进行中标记
func (pc *PendingChainSet) Del(chain common.Address) bool {
	return pc.set.Remove(pc.db, chain.Bytes())
}

func (pc *PendingChainSet) Range(cb func(PendingChain) bool) {
	pc.set.Range(pc.db, func(index uint32, key []byte) bool {
		pending, ok := pc.Get(common.BytesToAddress(key))
		if !ok {
			return false
		}
		return cb(pending)
	})
}
