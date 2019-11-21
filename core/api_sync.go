// Author: @kulics
package core

import (
	"container/list"
	"errors"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/tracetime"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
)

// WakChildren 获取所有子单元（所有直接引用此单元的子单元）
func (dc *DagChain) WakChildren(parentHash common.Hash, cb func(children common.Hash) bool) {
	defer tracetime.New().Stop()
	WalkChildren(dc.chainDB, parentHash, cb)
}

// GetBehindCountRelations 获取某个地址往后的链count条数据,包括hash的单元
func (self *DAG) GetBehindCountRelations(mcAccount common.Address, hash common.Hash, count int) ([]*types.Unit, error) {
	logger := log.New("module", "Dag", "caller", "GetBehindCountRelations")
	logger.Trace("get behind count relations", "mc", mcAccount.Hex(), "uHash", hash, "count", count)
	var units = make([]*types.Unit, 0, count)
	var parentHash = hash
	// 获取hash 单元
	unit, err := self.GetUnitByHash(parentHash)
	if err != nil {
		return units, err
	}
	// 获取兄弟节点
	brother := self.GetStableHash(mcAccount, unit.Number())
	if brother.Empty() {
		return nil, errors.New("unit not find")
	}
	// 获取孩子单元哈希
	getChildren := func(uHash common.Hash) []common.Hash {
		var result = make([]common.Hash, 0)
		WalkChildren(self.db, uHash, func(children common.Hash) bool {
			result = append(result, children)
			return false
		})
		return result
	}

	queue := list.New()
	queue.PushFront(brother)
	for {
		item := queue.Front()
		if item == nil {
			break
		}
		queue.Remove(item)
		parentHash = item.Value.(common.Hash)
		// 获取父节点
		{
			var unit *types.Unit
			if unit, err = self.GetUnitByHash(parentHash); err != nil {
				return units, err
			}
			units = append(units, unit)
			if len(units) >= count {
				return units, nil
			}
		}
		// 遍历子节点
		for _, child := range getChildren(parentHash) {
			queue.PushFront(child)
		}
	}

	return units, nil
}
