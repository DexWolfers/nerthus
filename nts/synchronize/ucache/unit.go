package ucache

import (
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/consensus"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"
)

func (c *UnitCache) ProcessCache() {
	c.notifyFetch()
}
func (c *UnitCache) notifyFetch() {
	select {
	case c.notifyFetchc <- struct{}{}:
	default:
	}
}

// 处理缓存中的单元
// 一些单元只是当下无法落地，所以在缓存中。在一些单元可以在合理实际进行一次尝试处理。
func (c *UnitCache) processCache() (successCount, failedCount, skip int, err error) {
	bt := time.Now()
	log.Debug("begin process cache", "begin.time", bt)
	defer func() {
		log.Debug("process cache end", "begin.time", bt, "cost", common.PrettyDuration(time.Now().Sub(bt)))
	}()
	list, err := c.GetUnitHashByTime(0)

	var unit *types.Unit
	var canDelete []common.Hash
	failedHashs := make(map[common.Hash]struct{})

	//按时间依次处理单元
	for _, uHash := range list {
		unit, err = c.Get(uHash)
		if err != nil {
			return
		}
		if unit == nil {
			canDelete = append(canDelete, uHash)
			continue
		}
		if _, ok := failedHashs[unit.ParentHash()]; ok {
			failedCount++
			failedHashs[uHash] = struct{}{}
			continue
		}
		//忽略陈旧数据
		if tail := c.cwr.GetChainTailNumber(unit.MC()); tail >= unit.Number() {
			canDelete = append(canDelete, uHash)
			log.Trace("ignore too old unit", "stable.height", tail, "unit.height", unit.Number())
			continue
		}
		//try insert
		_, err := c.cwr.InsertChain(types.Blocks{unit})

		switch err {
		case nil:
			log.Trace("insert cached unit successful",
				"chain", unit.MC(), "number", unit.Number(), "uhash", unit.Hash())

			delete(failedHashs, uHash)           //clear flag when inserted successfully
			canDelete = append(canDelete, uHash) //, will auto delete by cache when inserted successfully
			successCount++
		case consensus.ErrKnownBlock:
			skip++
			canDelete = append(canDelete, uHash)
		case consensus.ErrUnknownAncestor:
			skip++
			failedHashs[uHash] = struct{}{}
		default:
			failedCount++
			failedHashs[uHash] = struct{}{}
			canDelete = append(canDelete, uHash) //delete unit when insert failed
			log.Trace("try insert cached unit failed",
				"chain", unit.MC(), "number", unit.Number(), "uhash", unit.Hash(), "err", err)
		}
	}
	if skip > 0 { // check expired
		list, err := c.GetUnitHashByTime(time.Now().Add(-1 * 10 * time.Minute).Unix())
		if err != nil {
			canDelete = append(canDelete, list...)
		}
	}

	if len(canDelete) > 0 {
		//delete
		err := c.BatchDel(canDelete)
		if err != nil {
			log.Error("failed to delete cache", "err", err)
		}
	}

	return
}

func missingWhich(unit *types.Unit, exist func(common.Hash) bool) []types.UnitID {

	// TODO: 需要优化代码
	//    1. 需要检查高度是否一致，如果单元存在，但高度不一致，需要删除缓存
	//    2. 外部也许只需要一个单元，因此不需要遍历所有，可以择机停止遍历
	var list []types.UnitID
	//父单元
	if !exist(unit.ParentHash()) {
		list = append(list, types.UnitID{
			ChainID: unit.MC(),
			Height:  unit.Number() - 1,
			Hash:    unit.ParentHash(),
		})
	}
	if unit.SCHash() != unit.ParentHash() {
		if !exist(unit.SCHash()) {
			list = append(list, types.UnitID{
				ChainID: params.SCAccount,
				Height:  unit.SCNumber(), //暂时对高度位置
				Hash:    unit.SCHash(),
			})
		}
	}
	for _, tx := range unit.Transactions() {
		if tx.PreStep.IsEmpty() {
			continue
		}
		if !exist(tx.PreStep.Hash) {
			list = append(list, tx.PreStep)
		}
	}
	return list
}
