// witness contract db util
// Author: @zwj
package core

import (
	"errors"
	"fmt"
	"math/big"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/params"
)

type Witness struct {
	Address    common.Address `json:"address"`
	ApplyHash  common.Hash    `json:"apply"`
	CancelHash common.Hash    `json:"cancel"`
}

type WitnessLib struct {
	Capacity uint64 `json:"capacity"`
	Size     uint64 `json:"size"`
}

var (
	ErrNotFoundWitness = errors.New("not found the witness")
)

// ExistInWitnessLibs 检查申请人是否已存在
func (self *DAG) ExistInWitnessLibs(target common.Address) (bool, error) {
	state, err := self.dagchain.GetChainTailState(params.SCAccount)
	if err != nil {
		return false, err
	}

	_, err = sc.GetWitnessInfo(state, target)
	if err == sc.ErrNoWintessInfo {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// makeKey 拼接data
func makeKey(data ...interface{}) []byte {
	length := len(data)
	var out []byte
	for i := 0; i < length; i++ {
		switch val := data[i].(type) {
		case int:
			out = append(out, common.Uint64ToBytes(uint64(val))...)
		case int64:
			out = append(out, common.Uint64ToBytes(uint64(val))...)
		case uint64:
			out = append(out, common.Uint64ToBytes(uint64(val))...)
		case string:
			out = append(out, []byte(val)...)
		case []byte:
			out = append(out, val...)
		case *big.Int:
			out = append(out, val.Bytes()...)
		case common.Address:
			out = append(out, val.Bytes()...)
		case common.Hash:
			out = append(out, val.Bytes()...)
		}
	}
	return out
}

// GetWitnesses 根据用户和见证人获取见证人列表
func (self *DAG) GetWitnesses(account common.Address, scUnitHash common.Hash) (common.AddressList, error) {

	gpIndex, err := self.dagchain.GetChainWitnessGroup(account, scUnitHash)
	if err != nil {
		return nil, err
	}
	//判断此是否存在此高度的见证人清单
	value, ok := self.witnessLibCache.Get(scUnitHash)
	if ok {
		return value.(WitnessGroups)[gpIndex], nil
	}
	//需要加锁，等待更新完毕
	db, err := self.dagchain.GetUnitState(scUnitHash)
	if err != nil {
		self.witnessLibUpdateLock.Unlock()
		return nil, fmt.Errorf("failed to get unit state,%v", err)
	}
	lib, err := loadWitnessGroup(db)
	if err != nil {
		self.witnessLibUpdateLock.Unlock()
		return nil, err
	}
	self.witnessLibCache.Add(scUnitHash, lib)

	return lib[gpIndex], nil
}

// GetLastWitnesses 获取最新的用户见证人
func (self *DAG) GetLastWitnesses(address common.Address) ([]common.Address, error) {
	hash := GetChainTailHash(self.db, params.SCAccount)
	return self.GetWitnesses(address, hash)
}

type WitnessGroups = map[uint64][]common.Address

//加载所有组见证人，不包括系统链的备选见证人
func loadWitnessGroup(db sc.StateDB) (WitnessGroups, error) {
	lib := make(WitnessGroups)
	for gp := uint64(0); gp <= sc.GetWitnessGroupCount(db); gp++ {
		if gp == sc.DefSysGroupIndex {
			_, list, err := sc.GetSystemMainWitness(db)
			if err != nil {
				return nil, err
			}
			lib[gp] = list
		} else {
			lib[gp] = sc.GetWitnessListAt(db, gp)
		}
	}
	return lib, nil
}
