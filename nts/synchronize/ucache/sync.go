package ucache

import (
	"time"

	"gitee.com/nerthus/nerthus/p2p/discover"

	"gitee.com/nerthus/nerthus/log"
)

const (
	defSyncCheckInterval           = 10 * time.Second
	defSyncCacheMissingInterval    = 15 * time.Second
	defSyncCacheMissingMaxInterval = 1 * time.Minute
)

func (r *UnitCache) autoSyncMissing() {
	r.exitw.Add(1)
	log.Info("started auto sync miss unit")
	defer func() {
		log.Info("auto sync miss unit stopped")
		r.exitw.Done()
	}()

	timer := time.NewTicker(defSyncCacheMissingInterval)
	defer timer.Stop()

	for {
		select {
		case <-r.quit:
			return
		case <-r.notifyFetchc:
			r.processCache()
			r.fetchByMissingUnit()
		case <-timer.C:
			// 将间隔式按照缺失单元
			r.fetchByMissingUnit()
		}
	}
}

//当本地存在一些无法落地单元时，需要向网络询问以便不足
func (r *UnitCache) fetchByMissingUnit() (sends int) {
	if r.core.PeerStatusSet().Empty() {
		return
	}

	// peer have data, try other chain
	cachedChains, err := r.GetChains()
	if err != nil {
		log.Error("failed to get cached units chain", "err", err)
		return
	}
	log.Trace("try fetch data by cached units", "chains", len(cachedChains))

	for _, c := range cachedChains {
		select {
		case <-r.quit:
			return
		default:
		}

		// 取该链中已第一个缺失高度
		unitID, err := r.GetChainFirstUnit(c, 0)
		if err != nil {
			log.Warn("failed to get cached unit", "err", err)
			continue
		}
		if unitID.IsEmpty() {
			continue
		}
		log.Trace("begin fetch one chain data for save cached unit", "uid", unitID)
		// check missing
		unit, err := r.Get(unitID.Hash)
		if err != nil {
			log.Warn("failed to get cached unit",
				"chain", unitID.ChainID, "number", unitID.Height, "uhash", unitID.Hash,
				"err", err)
			continue
		}
		if unit == nil {
			r.Del(unitID.Hash) //delete cache
			continue
		}
		if tail := r.cwr.GetChainTailNumber(unit.MC()); tail >= unit.Number() {
			//可以以高度删除一次该链单元
			list, err := r.QueryChainUnitsByMaxNumber(unit.MC(), tail)
			if err != nil {
				log.Error("query failed", "err", err)
			} else {
				r.BatchDel(list)
			}
			continue
		}
		// 查查该单元所缺失的父单元（祖先）
		r.FetchAncestor(discover.NodeID{}, unit)

	}
	return
}
