package core

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/tracetime"
	"gitee.com/nerthus/nerthus/consensus"
	"gitee.com/nerthus/nerthus/consensus/arbitration"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"
)

//  recoveryChain 恢复链操作
func (dc *DagChain) recoveryChain(output types.UnitID) (err error) {
	if output.Hash.Empty() {
		return nil
	}
	log.Info("recovery chain", "mc", output.ChainID, "number", output.Height, "uhash", output.Hash)
	// 将 good 移除bad标记，这样可以重新校验处理
	dc.badUnitHashs.Delete(output.Hash.Hex())
	dc.dag.removeBad(output.Hash)

	// 将其登记，方便过滤
	dc.dag.chainArbitrationDone(output.ChainID, output.Height, output.Hash)

	stableHeader := dc.GetHeaderByNumber(output.ChainID, output.Height)

	//
	// 在接收到系统见证人裁定后，需要恢复指定链的合法位置
	//系统见证人在坐实见证人作恶时，同时指明分叉中的首个有效单元和无效单元。这样，节点在收到系统链的指示时，将切换到明确的分支上。

	// 系统见证人已投票选出合法单元，该高度的其他单元一律视为非法
	//从系统链中取出合法位置
	// 在纠正之前，需要停止链服务
	//				⤹￣￣￣￣￣￣④ <---- ⑤
	//    ① <----- ② <--------③
	//               ↖︎---------⑥ <----- ⑦
	// 裁定③单元合法，此时将清理②的其他两个子分支：④ <--- ⑤ 和 ⑥ <---- ⑦
	if stableHeader == nil && stableHeader.Hash() != output.Hash { //本地已存储单元不是合法单元时清理数据
		//将其他同级的非指定的有效单元数据全部清理
		// 立刻进行 bad 处理
		dc.removeBadUnitFromFutures(stableHeader.Hash())
		// 登记 bad unit hash 将永久记录到DB中
		dc.dag.writeBadUnit(stableHeader.Hash())
		// 清理单元，将同时清理相关的所有单元
		if err = dc.clearUnit(stableHeader.Hash(), true, false); err != nil {
			log.Error("failed to clear bad unit", "bad", stableHeader.Hash(), "err", err)
		}
	}
	//恢复链
	err = arbitration.RecoverStartChain(dc, dc.chainDB, output.ChainID, output.Height)
	if err != nil {
		return err
	}

	good := dc.GetHeaderByHash(output.Hash)
	if good == nil { //本地无此单元，通知外部获取，不干预
		//通知获取
		dc.dag.eventMux.Post(types.MissingAncestorsEvent{
			Chain: output.ChainID,
			UHash: output.Hash,
		})
		return nil
	}
	return nil
}

func (dc *DagChain) clearUnit(unitHash common.Hash, delAll bool, needLock ...bool) (err error) {
	txs, err := dc.clearUnitWithTxs(unitHash, delAll, needLock...)
	// 倒序处理，因为删除单元时从高位置向低位置处理，而在重发交易时需要从低位置处理
	txHashs := make(map[common.Hash]struct{}, len(txs))
	for i := len(txs) - 1; i >= 0; i-- {
		tx := txs[i]
		if _, ok := txHashs[tx.TxHash]; ok { //不重复播放
			continue
		}

		originTx := tx.Tx
		if originTx == nil {
			originTx = dc.GetTransaction(tx.TxHash)
		}
		if originTx == nil {
			continue
		}
		// 该原始交易将被重新广播到网络中
		if tx.Tx != nil && dc.dag != nil {
			dc.dag.eventMux.Post(types.TxPreEvent{Tx: originTx})
		}
		dc.dag.eventMux.Post(types.ReplayTxActionEvent{Tx: originTx})
		txHashs[tx.TxHash] = struct{}{}
	}

	return err
}

// clearUnit 清理单元，将自动清除所有子单元（直接或间接引用此单元的）
func (dc *DagChain) clearUnitWithTxs(unitHash common.Hash, delAll bool, needLock ...bool) (removedTxs types.TxExecs, err error) {
	header := dc.GetHeaderByHash(unitHash)
	if header == nil || header.Number == 0 { //不存在 or 创世
		return
	}

	// 优先处理所有子单元
	WalkChildren(dc.chainDB, unitHash, func(children common.Hash) bool {
		txs, err := dc.clearUnitWithTxs(children, delAll, needLock...)
		if err != nil {
			log.Crit("failed to clear unit", "uhash", children, "err", err)
		}
		removedTxs = append(removedTxs, txs...)
		return false
	})

	if len(needLock) > 0 && needLock[0] {
		//需要进行链锁，此时不允许操作链
		locker := dc.chainLocker(header.MC)
		locker.Lock()
		defer locker.Unlock()
	}
	log.Warn("clearing unit", "chain", header.MC, "number", header.Number, "uhash", unitHash, "delStore", delAll)

	defer func() {
		dc.blockCache.Remove(unitHash) //清理缓存
		dc.unitHeadCache.Remove(unitHash)
		dc.unitInserting.Remove(unitHash.Hex())
		dc.chainTailCache.Remove(header.MC.Text())
	}()
	// 因为上面的递归已经保证下面处理的单元已经是末端单元，如果需要移动标记，只需要向 number - 1 移动即可。
	// 即使  number - 1 是需要移除的单元，也会在下一轮时继续重复处理。

	// 删除标记
	if err = DeleteStableHash(dc.chainDB, unitHash, header.MC, header.Number); err != nil {
		return
	}
	if header.Number == 1 { //创世单元的 MC 不一样
		//说明此链已无单元，清理 Tail
		WriteChainTailHash(dc.chainDB, common.Hash{}, header.MC)
	} else {
		// 修正最佳位置，必须是上一个单元处
		WriteChainTailHash(dc.chainDB, header.ParentHash, header.MC)
	}

	unit := dc.GetUnit(unitHash)
	if unit != nil {
		//删除前，更正见证人干活数据
		dc.settleCalc.RollbackWorkFee(dc.chainDB, unit)

		// 将其中的交易提取处理并交给见证服务重新处理
		removedTxs = append(removedTxs, unit.Transactions()...)
		for _, tx := range unit.Transactions() {
			err = delTransactionStatus(dc.chainDB, unitHash, tx.TxHash, tx.Action)
			if err != nil {
				return
			}
		}
	}

	if delAll {
		DeleteUnit(dc.chainDB, header.MC, unitHash, header.Number)
	}
	return
}

//
//// LoadDAG 从已有单元中，从给定的根单元构建一个在指定链上的DAG
//func (dc *DagChain) loadDAGFromDB(mc common.Address, begin common.Hash) graph.Dag {
//	g := make(graph.Dag)
//	// 从最后的稳定位置加载
//	last := dc.GetChainTailHead(mc)
//	g.AddVertex(last, nil)
//	var roots []common.Hash
//	header := dc.GetHeaderByHash(begin)
//	if header == nil {
//		return g
//	}
//	for p := header; p != nil && p.Number >= last.Number; {
//		roots = append(roots, p.Hash())
//		p = dc.GetHeaderByHash(p.ParentHash)
//	}
//
//	var forkBegin uint64
//	// 说明开始位置在历史不可逆中[begin,......, last, .......] ，则在 [begin, last]间只需要保留一条链，不存在分叉
//	if last.Number > header.Number {
//		for p := last; p != nil && p.Number > header.Number; {
//			roots = append(roots, p.Hash())
//			p = dc.GetHeaderByHash(p.ParentHash)
//		}
//		forkBegin = last.Number + 1
//	} else {
//		forkBegin = header.Number + 1
//	}
//	//顶点（所有父单元全部加载完成后，再添加关系）
//	// roots 集合中，Number 从高到底。
//	// 如：[N6,N5,N4,N3,N2]
//	for i := len(roots) - 1; i > 0; i-- { // 必须优先处理低位单元，否则无法正确累计权重
//		if i == len(roots)-1 {
//			g.AddVertex(roots[i], nil) // N2 无父单元
//		} else {
//			g.AddEdge(roots[i], roots[i+1])
//		}
//		//获取投票,投票数量也作为权重一部分
//		v, err := dc.dag.GetVoteMsg(roots[i])
//		if err != nil {
//			log.Crit("failed to load unit voters", "err", err) //不应该发生此错误
//		}
//		g[roots[i]].AddWeight(len(v))
//	}
//
//	//遍历 root 的所有子单元（同一条链上)
//	for bs := dc.dag.GetBrothers(mc, forkBegin); len(bs) > 0; forkBegin++ {
//		for _, b := range bs {
//			//获取单元头
//			h := dc.GetHeaderByHash(b)
//			if h != nil {
//				//添加边
//				g.AddEdge(b, h.ParentHash)
//				//获取投票,投票数量也作为权重一部分
//				v, err := dc.dag.GetVoteMsg(b)
//				if err != nil {
//					log.Crit("failed to load unit voters", "err", err) //不应该发生此错误
//				}
//				g[b].AddWeight(len(v))
//			}
//		}
//	}
//	return g
//}

// foundDishonest 发现作恶
func (dc *DagChain) foundDishonest(badWitness common.Address, reason error) {
	// 如果是作恶行为，则进行更多处理
	err, ok := reason.(*consensus.ErrWitnessesDishonest)
	if !ok {
		log.Debug("found bad unit but will ignore more check", "reason", reason)
		return
	}

	//发现作恶行为，通知外部
	ev := types.FoundBadWitnessEvent{
		Reason:    err.Reason,
		Witness:   badWitness,
		Bad:       err.Bad,
		BadVotes:  err.BadVotes,
		Good:      err.Good,
		GoodVotes: err.GoodVotes,
	}
	log.Debug("post FoundBadWitness event",
		"basWitness", badWitness, "chain", ev.Bad.MC, "number", ev.Bad.Number, "good.uhash", ev.Good.Hash(), "bad.uhash", ev.Bad.Hash())

	//当出现作恶单元时，需要将消息通知给见证人
	dc.PostChainEvents([]interface{}{ev}, nil)

	unitID := err.Bad.ID()
	// 也许此Bad已经存储需要移除
	if err2 := dc.clearUnit(unitID.Hash, true, true); err2 != nil {
		log.Crit("failed to delete saved bad unit",
			"chain", err.Bad.MC, "number", err.Bad.Number, "uhash", unitID.Hash, "err", err)
	} else {
		log.Warn("removed saved bad unit from database",
			"chain", err.Bad.MC, "number", err.Bad.Number, "uhash", unitID.Hash, "reason", reason)
	}
}

func (dc *DagChain) stopChain(uid types.UnitID) {
	//在用户链中，查找影响面
	//分析last位置：
	// 1.  last > bad 说明是对历史高度重复生成，则不会影响现有
	// 2.  last < bad 说明是对新的不稳定单元的作恶，则不会影响到其他链，只需要停止本链的处理
	// 3.  last = bad 这让很有可能不同节点的视图不一样，短期内出现这种情况，需要降低影响面。争取和系统见证人判断的合法性相同
	last := dc.GetChainTailHead(uid.ChainID)
	//检索子单元
	mcs := make(map[common.Address]struct{})
	//处理
	stop := func(mc common.Address) {
		if mc.Empty() {
			return
		}
		if _, ok := mcs[mc]; ok {
			return
		}
		// 不应该停止系统链
		if mc == params.SCAccount {
			return
		}
		mcs[mc] = struct{}{}
		//通知休眠
		dc.PostChainEvents([]interface{}{types.ChainMCProcessStatusChangedEvent{MC: mc, Status: types.ChainNeedStop}}, nil)
	}
	if last.Number < uid.Height { //
		stop(uid.ChainID)
	} else if last.Number == uid.Height { //
		var lookup func(hash common.Hash)
		lookup = func(hash common.Hash) {
			//遍历所有子单元，寻找影响
			WalkChildren(dc.chainDB, hash, func(children common.Hash) bool {
				mc, _ := GetUnitNumber(dc.chainDB, children)
				stop(mc)
				return false
			})
			mc, _ := GetUnitNumber(dc.chainDB, hash)
			stop(mc)
		}
		// 开始遍历
		lookup(last.Hash())
	}
}

// GetArbitrationResult 获取仲裁结果
func (dag *DAG) GetArbitrationResult(mc common.Address, number uint64) common.Hash {
	defer tracetime.New().Stop()
	key := makeKey(arbitrationResultPrefix, mc, number)
	data, _ := dag.db.Get(key)

	return common.BytesToHash(data)
}

// ChainIsInInArbitration 链是否在仲裁中
func (dag *DAG) ChainIsInInArbitration(mc common.Address) bool {
	return dag.arbitrationIngCache.Has(mc.Text())
}

func (dag *DAG) writeChainInInArbitration(mc common.Address, _ uint64) {
	dag.arbitrationIngCache.Add(mc.Text())
	dag.db.Put(getChainArbitrationInKey(mc), []byte{1})
}
func getChainArbitrationInKey(chain common.Address) []byte {
	return makeKey(arbitrationInPrefix, chain)
}

func (dag *DAG) chainArbitrationDone(mc common.Address, number uint64, result common.Hash) {
	key := makeKey(arbitrationResultPrefix, mc, number)
	dag.db.Put(key, result.Bytes()) //记录仲裁结果

	//抹除标记
	dag.arbitrationIngCache.Delete(mc.Text())
	dag.db.Delete(makeKey(arbitrationInPrefix, mc))
}

func (dag *DAG) loadArbitrationChains() {
	dag.db.Iterator(arbitrationInPrefix, func(key, value []byte) bool {
		dag.arbitrationIngCache.Add(common.BytesToAddress(key[len(arbitrationInPrefix):]).Text())
		return true
	})
}
