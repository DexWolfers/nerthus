package witness

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"gitee.com/nerthus/nerthus/nts/protocol"
	"gitee.com/nerthus/nerthus/witness/service/wconn"

	"gitee.com/nerthus/nerthus/consensus/arbitration/monitor"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/common/tracetime"
	"gitee.com/nerthus/nerthus/consensus/arbitration"
	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"

	"github.com/pkg/errors"
)

const (
	zeroGasUseMaxSum = 200
)

var (
	errConsensusInterrupt = errors.New("consensus interrupted")
	errNoSuccessTx        = errors.New("no success transaction")
	errRollbackTx         = errors.New("need to rollback")
	errInvalidTx          = errors.New("invalid tx")
)

type MiningResult struct {
	CreateAt time.Time
	Unit     *types.Unit
	Receipts types.Receipts
	Logs     []*types.Log
	StateDB  *state.StateDB
}

type TxPool interface {
	AddRemote(*types.Transaction) error
}

type BftMiningAss struct {
	team       []common.Address
	eventMux   *event.TypeMux
	txPool     *core.TxPool
	center     *MiningJobCenter
	witnessSet *wconn.WitnessSet
	dagChain   *core.DagChain
	schedule   *WitnessSchedule
	config     *bft.Config
}

func newAssistant(team []common.Address, center *MiningJobCenter, schedule *WitnessSchedule, cfg *bft.Config) *BftMiningAss {
	return &BftMiningAss{
		team:       team,
		eventMux:   schedule.nts.EventMux(),
		txPool:     schedule.nts.TxPool(),
		witnessSet: schedule.witnessSet,
		schedule:   schedule,
		center:     center,
		dagChain:   schedule.nts.DagChain(),
		config:     cfg,
	}
}

// 重新广播这条链的交易
func (ass *BftMiningAss) ReBroadTx(chain common.Address) int {
	if chain == params.SCAccount {
		return 1
	}
	if !ass.schedule.onlineWitnessIsOk() {
		log.Warn("online witness is not enough,cancel rebroad")
		return 0
	}
	que := ass.center.Load(chain)
	if que == nil || que.Len() == 0 {
		return 0
	}

	//不需要广播所有交易，只需要广播一个单元的交易量即可
	max := 300
	list := make([]*types.Transaction, 0, max)
	que.Range(func(txmsg *TxExecMsg) bool {
		// 只重新广播原始交易
		if txmsg.PreAction.IsEmpty() {
			list = append(list, txmsg.Tx)
			return len(list) < max
		}
		return true
	})

	for _, tx := range list {
		ass.witnessSet.Gossip(protocol.TxMsg, tx)
	}
	if len(list) > 0 {
		return 1
	}
	return 0
}

// TxPool 交易池
func (ass *BftMiningAss) Rollback(unit *types.Unit) {
	// 需要检查单元是否已存在
	if ass.dagChain.GetHeaderByHash(unit.Hash()) != nil {
		return
	}
	for _, tx := range unit.Transactions() {
		orignTx := tx.Tx
		// 需要定位到交易信息
		if orignTx == nil {
			orignTx = ass.txPool.Get(tx.TxHash)
			if orignTx == nil {
				orignTx = ass.dagChain.GetTransaction(tx.TxHash)
			}
		}
		if orignTx == nil {
			continue
		}
		// 忽略系统见证人发送的心跳
		if unit.MC() == params.SCAccount && ass.schedule.curWitness == orignTx.SenderAddr() && sc.CheckContractType(orignTx.Data(), sc.FuncIdSystemHeartbeat) {
			continue
		}
		//推送到处理队列
		ass.center.Push(unit.MC(), orignTx, tx.Action, tx.PreStep)
	}

}

// Broadcaster 见证peer列表
func (ass *BftMiningAss) Broadcaster() bft.Broadcaster {
	return ass.witnessSet
}

// ExistValidTx 是否存在未处理的交易
func (ass *BftMiningAss) ExistValidTx(mc common.Address) bool {
	if ass.schedule.isSys {
		return true
	}

	queue := ass.center.Load(mc)
	if queue == nil || queue.Len() <= 0 {
		return false
	}
	return queue.NeedWork()
}

// Mining 打包单元
func (ass *BftMiningAss) Mining(ctx context.Context, chain common.Address) (result *bft.MiningResult, err error) {
	//defer func() {
	//	if err2 := recover(); err2 != nil {
	//		stack := debug.Stack()
	//		os.Stderr.Write(stack)
	//		//不会因为一些异常问题导致程序退出
	//		log.Error("recover system when mining", "chain", chain, "message", err2, "stack", stack)
	//		err = errors.New("recover system when mining")
	//	}
	//}()
	result, err = ass.generateUnit(ctx, chain)
	if err == errNoSuccessTx {
		return nil, err
	}
	return result, err
}

// VerifyUnit 校验单元
func (ass *BftMiningAss) VerifyUnit(ctx context.Context, unit *types.Unit) (*bft.MiningResult, error) {
	rec, db, logs, err := ass.dagChain.VerifyUnit(unit, func(unit *types.Unit) error {
		return ass.dagChain.Engine().VerifyHeader(ass.dagChain, unit, false)
	})
	if err != nil {
		return nil, err
	}
	return &bft.MiningResult{
		CreateAt: time.Now(),
		Unit:     unit,
		Receipts: rec,
		Logs:     logs,
		StateDB:  db,
	}, nil
}

// Commit 单元提案者提交新单元
func (ass *BftMiningAss) Commit(ctx context.Context, info *bft.MiningResult) error {
	unit := info.Unit.(*types.Unit)
	ass.eventMux.Post(types.NewWitnessedUnitEvent{Unit: unit})

	// 存入chain
	if err := ass.dagChain.WriteRightUnit(time.Now(), info.StateDB, unit, info.Receipts, info.Logs); err != nil {
		if err == core.ErrExistStableUnit {
			return nil
		}
		return err
	}

	log.Info("committed new unit",
		"chain", unit.MC(),
		"number", info.Unit.Number(), "uhash", info.Unit.Hash(),
		"votesSize", len(unit.WitnessVotes()), "miner", unit.Proposer(), "fromMe", unit.Proposer() == ass.schedule.curWitness)

	return nil
}

// generateUnit 生成单元
func (self *BftMiningAss) generateUnit(ctx context.Context,
	chain common.Address) (*bft.MiningResult, error) {

	tr := tracetime.New("chain", chain).SetMin(0)
	defer tr.Stop()

	// 用户链如果在仲裁中，则不允许工作
	if self.center.IsInBlack(chain) {
		// 如果在此高度
		return nil, errors.New("create new unit is not allowed during chain arbitration")
	}

	logger := log.Root().New("chain", chain)

	tr.Tag("prepare")

	tail := self.dagChain.GetChainTailInfo(chain)
	parent, statedb := tail.TailUnit(), tail.State(self.dagChain.StateCache())
	// 链的第一个单元需要从创世迁移state数据
	if parent.Number() == 0 && chain != params.SCAccount {
		var err error
		statedb, err = statedb.NewChainState(chain)
		if err != nil {
			return nil, err
		}
	}
	tr.Tag("beginprocess")

	header, err := self.generateHeader(chain, parent)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create header")
	}
	tr.Tag()
	beforePack := time.Now()
	tr.AddArgs("createat", beforePack, "clen", self.center.ChainTxs(chain))
	txExecs, successMsg, receipts, logs, selfHahs, err := self.packTxs(ctx, statedb, parent, header, tr)
	tr.Tag("processed").AddArgs(
		"number", header.Number,
		"size", self.center.ChainTxs(chain),
		"cost2", time.Now().Sub(beforePack))
	defer func() {
		if err != nil {
			tr.AddArgs("status", err)
			if err == errNoSuccessTx {
				return
			}
			self.rollback(chain, successMsg, selfHahs)
			logger.Error("generate unit error", "number", header.Number, "err", err)
		}
	}()
	if err != nil {
		return nil, err
	}

	newUnit := types.NewUnit(header, txExecs)

	tr.AddArgs("utime", header.Timestamp)
	tr.Tag("fixed")
	// 签名
	if err = signThat(self.schedule.nts, newUnit); err != nil {
		return nil, err
	}
	tr.Tag("signed")
	tr.AddArgs("uhash", newUnit.Hash(), "txs", len(newUnit.Transactions()))

	logger.Debug("generate unit finished", "uhash", newUnit.Hash(), "number", newUnit.Number(), "timestamp", header.Timestamp,
		"txs", len(newUnit.Transactions()),
		"usedGas", newUnit.GasUsed(),
		"gasRemain", newUnit.GasLimit()-newUnit.GasUsed(),
		"pendingTxs", self.schedule.jobCenter.ChainTxs(newUnit.MC()))

	tr.Tag()
	return &bft.MiningResult{
		CreateAt: time.Now(),
		Unit:     newUnit,
		Receipts: receipts,
		Logs:     logs,
		StateDB:  statedb,
	}, nil

}

// rollback 交易回滚
func (self *BftMiningAss) rollback(chain common.Address, msgs []*TxExecMsg, ignoreTx common.HashList) {
	//将执行失败的交易回滚，一般只有一笔
	for _, m := range msgs {
		if ignoreTx.Have(m.TXHash) {
			continue
		}
		self.center.Push(chain, m.Tx, m.Action, m.PreAction)
	}
}

// packTxs 获取新交易并处理,用来打包进单元中
// 回滚交易时需要排除自动生成的交易,例如:心跳,更换系统见证人
func (self *BftMiningAss) packTxs(ctx context.Context, db *state.StateDB, parent *types.Unit,
	header *types.Header, tr *tracetime.TraceTime) (
	result []*types.TransactionExec, successMsg []*TxExecMsg, receipts types.Receipts, logs []*types.Log, selfHash common.HashList, err error) {

	var (
		gp          *core.GasPool
		totalGas    = new(big.Int)
		zeroGasSum  = 0
		index       = 0
		rollbackMsg = make([]*TxExecMsg, 0)
		errMsg      = make([]*TxExecMsg, 0)
		timeover    = false
		timeLimit   = time.Second * 3
	)
	tr.Tag("packTxs")
	// 收到链的第一笔交易，等待单元间隔
	delay := self.WaitGenerateUnitTime(parent.Number(), parent.Timestamp())
	if delay > timeLimit {
		timeLimit = delay
	}
	header.Timestamp = uint64(time.Now().UnixNano()) + uint64(timeLimit.Nanoseconds())
	tr.AddArgs("cost1", timeLimit)
	timeout := time.After(timeLimit)
	log.Debug("begin generate unit", "chain", header.MC, "number", header.Number, "timestamp", header.Timestamp, "wait_time", timeLimit)
	priceLimit := self.dagChain.Config().Get(params.ConfigParamsLowestGasPrice)
	tr.Tag()
	gp = new(core.GasPool).AddGas(new(big.Int).SetUint64(header.GasLimit))

	queue := self.center.Load(header.MC)
	tr.Tag()
	defer func() {
		// 失败交易处理
		for _, v := range errMsg {
			if !selfHash.Have(v.TXHash) {
				// 从交易池中删除失败的交易
				self.txPool.Remove(v.TXHash)
			}
		}
		//将执行失败的交易回滚，一般只有一笔
		self.rollback(header.MC, rollbackMsg, selfHash)
	}()

	processTx := func(txmsg *TxExecMsg) bool {
		tr := tracetime.New()
		var reterr error
		defer func() {
			log.Trace("commit tx into unit", "chain", header.MC, "number", header.Number, "index", index,
				"action", txmsg.Action, "tx", txmsg.TXHash, "err", reterr)
		}()
		receivd := time.Now()

		// 如果剩余的Gas已不足以执行一笔交易的最低要求，则退出
		if gp.Gas() < params.TxGas {
			rollbackMsg = append(rollbackMsg, txmsg)
			reterr = fmt.Errorf("rest gas not enough")
			return false
		}
		tr.Tag()
		if txmsg.Action == types.ActionTransferReceipt ||
			txmsg.Action == types.ActionContractRefund ||
			txmsg.Action == types.ActionSCIdleContractDeal {
			zeroGasSum++
			if zeroGasSum >= zeroGasUseMaxSum {
				rollbackMsg = append(rollbackMsg, txmsg)
				reterr = fmt.Errorf("zeroGasSum %d beyond", zeroGasSum)
				return false
			}
		}
		tr.Tag()
		tx, receipt, err := self.handleTx(db, txmsg, header, priceLimit, index, gp, totalGas, tr)
		if err != nil {
			reterr = err
			if err == core.ErrGasLimitReached {
				rollbackMsg = append(rollbackMsg, txmsg)
				return index == 0 //如果是第一笔交易边出现超过 Limit，则放弃，继续下一笔交易处理
			} else if err == errInvalidTx {
				// 忽略
				return true
			} else if err == errRollbackTx {
				rollbackMsg = append(rollbackMsg, txmsg)
				return true
			}
			errMsg = append(errMsg, txmsg)
			return true
		}
		tr.Tag()
		//成功
		receipts = append(receipts, receipt)
		logs = append(logs, receipt.Logs...)
		successMsg = append(successMsg, txmsg)
		result = append(result, tx)
		index++

		//log.Error("--------->>>>", "gas", receipt.GasUsed)

		tr.AddArgs("chain", header.MC, "number", header.Number, "createat", receivd, "cost1", time.Now().Sub(receivd), "cost2", timeLimit)
		tr.Stop()

		return index <= math.MaxUint16
	}

	txList, e := self.schedule.newOtherTx(parent.Number(), db)
	if err != nil {
		log.Error("generate system tx err", "error", e)
		err = e
		return
	} else {
		for _, v := range txList {
			selfHash = append(selfHash, v.TXHash)
			processTx(v)
		}
	}
	tr.Tag()
	seq := 0

	waitMsg := time.NewTicker(100)
	defer waitMsg.Stop()

loop:
	for {
		if queue == nil {
			break
		}
		seq++
		// 判断是否超时
		select {
		case <-timeout:
			timeover = true
			break loop
		case <-ctx.Done():
			err = errConsensusInterrupt
			// 所有交易回滚
			rollbackMsg = append(rollbackMsg, successMsg...)
			break loop
		default:
			msg := queue.Pop()
			if msg != nil {
				canContinue := processTx(msg)
				if !canContinue {
					break loop
				}
			} else {
				<-waitMsg.C
			}
		}
	}
	tr.Tag().AddArgs("size2", seq)
	if result == nil || len(result) == 0 {
		err = errNoSuccessTx
		return
	}
	// 插入部分结果数据
	header.StateRoot, _ = db.IntermediateRoot(header.MC)
	header.GasUsed = totalGas.Uint64()
	header.ReceiptRoot = types.DeriveSha(receipts)
	header.Bloom = types.CreateBloom(receipts)

	var gas uint64
	for _, v := range receipts {
		gas += v.GasUsed
	}

	tr.Tag()
	// 需要等待时间间隔
	if !timeover {
		tr.Tag()
		<-timeout
	}
	return
}

// 选择一个合理的系统链引用单元，选择时需要保证符合一定的规则。
func (self *BftMiningAss) chooseSC(chain common.Address, parent *types.Unit, nowTimestamp uint64) (types.UnitID, error) {
	//如果是系统链单元，则引用需要和父单元一致
	if chain == params.SCAccount {
		return parent.ID(), nil
	}
	var (
		sc *types.Unit
	)
	//只允许使用两小时内的系统链
	const minSCTime = uint64(2 * time.Hour)

	for {
		//第一次使用末尾单元，后面使用父单元，递归向上找出
		if sc == nil {
			sc = self.dagChain.GetChainTailUnit(params.SCAccount)
			// 使用系统链最后单元引用时，需要判断系统链状态
			if sta := self.schedule.monitorEngine.Status(); sta != monitor.StatusRunning {
				log.Warn("system chain status not normal,reference skip tail", "status", sta, "tail", sc.Number(), "hash", sc.Hash(), "receive time", sc.ReceivedAt)
				continue
			}
		} else {
			sc = self.dagChain.GetUnitByHash(sc.ParentHash())
		}
		if sc == nil {
			return types.UnitID{}, errors.New("not found unit")
		}
		//已经是创世单元
		if sc.Number() == 0 {
			return sc.ID(), nil
		}
		//系统链时间不能晚于当前时间
		if sc.Timestamp() > nowTimestamp {
			log.Trace("bad timestamp", "chain", chain, "scTime", sc.Timestamp(), "utime", nowTimestamp)
			continue
		}
		//太久远的系统链不能使用
		if nowTimestamp > sc.Timestamp() && nowTimestamp-sc.Timestamp() > minSCTime {
			log.Error("stop mining when last system chain unit is too old",
				"tail.utime", time.Unix(0, int64(sc.Timestamp())), "use.utime", time.Unix(0, int64(nowTimestamp)))
			return types.UnitID{}, errors.Errorf("system chain unit is too old,system chain last unit time is %s < %s+2h",
				time.Unix(0, int64(sc.Timestamp())), time.Unix(0, int64(nowTimestamp)))
		}
		// 新的 SC 引用，必须是父单元引用SC 后面
		if parent.SCNumber() > sc.Number() {
			//这里不能继续查找也无意义
			return types.UnitID{}, errors.New("can not choose a right system chain unit")
		}

		//已找到合法单元，可退出
		break
	}
	return sc.ID(), nil
}

// generateHeader 打包单元头
func (self *BftMiningAss) generateHeader(chain common.Address, parent *types.Unit) (*types.Header, error) {
	nowTimestamp := uint64(time.Now().UnixNano())
	if parent.Timestamp() > nowTimestamp {
		return nil, fmt.Errorf("do not allow new unit time earlier than the parent unit, %d > %d", parent.Timestamp(), nowTimestamp)
	}
	//如果该链已经是被终止允许的链则需要停止工作
	if err := arbitration.CheckChainIsStopped(self.schedule.nts.ChainDb(), chain, parent.Number()+1); err != nil {
		return nil, fmt.Errorf("stop it now, %s", err)
	}

	sc, err := self.chooseSC(chain, parent, nowTimestamp)
	if err != nil {
		return nil, err
	}
	// 构建头
	header := &types.Header{
		MC:         chain,
		Number:     parent.Number() + 1,
		ParentHash: parent.Hash(),
		SCHash:     sc.Hash, // 系统链稳定单元
		SCNumber:   sc.Height,
		Proposer:   self.schedule.curWitness,
		Timestamp:  nowTimestamp,
		GasLimit:   self.dagChain.Config().Get(params.ConfigParamsUcGasLimit),
	}
	return header, nil
}

// handleTx 处理交易
func (self *BftMiningAss) handleTx(db *state.StateDB,
	msg *TxExecMsg,
	header *types.Header,
	priceLimit uint64, index int,
	gp *core.GasPool, totalUsedGas *big.Int, tr *tracetime.TraceTime) (
	tx *types.TransactionExec, receipt *types.Receipt, err error) {
	tr.Tag("handleTx")
	if !msg.PreAction.IsEmpty() {
		if h := self.dagChain.GetHeader(msg.PreAction.Hash); h == nil {
			log.Error("not found previous step info", "txhash", msg.TXHash, "prestep", msg.PreAction)
		} else if h.Timestamp >= header.Timestamp {
			log.Debug("future transaction", "txhash", msg.TXHash, "until", h.Timestamp)
			err = errRollbackTx
			return
		}
	}
	tr.Tag()
	tx = msg.ConvertToTxExec()
	tr.Tag()

	// 临时过滤交易
	if header.MC == params.SCAccount && msg.Tx.Gas() >= 8000000 &&
		self.dagChain.Config().ChainId.Cmp(params.TestnetChainConfig.ChainId) == 0 {
		err = errInvalidTx
		return
	}

	//如果原始交易中GasPrice低于要求，则忽略。在进入交易池时有检查，但是此处需要更加单元的实际情况再次检查
	if tx.PreStep.IsEmpty() {
		if msg.Tx != nil && !core.IsFreeTx(msg.Tx) && msg.Tx.GasPrice() < priceLimit {
			var err2 error
			if core.IsFreeTx(msg.Tx) {
				// 免费交易gas必须为0
				if msg.Tx.GasPrice() > 0 || msg.Tx.Gas() > 0 {
					err2 = core.ErrInvalidGas
				}
			} else {
				if msg.Tx.GasPrice() < priceLimit {
					err2 = core.ErrGasPriceTooLow
				}
			}
			if err2 != nil {
				log.Debug("transaction failed, account skipped",
					"action", msg.Action, "hash", msg.TXHash, "err", err2)
				err = errInvalidTx
			}
			return
		}
		unitTime := time.Unix(0, int64(header.Timestamp))
		// 只对交易的第一个阶段进行判断
		if tx.Tx.Expired(unitTime) {
			log.Debug("transaction timeout",
				"tx create time", tx.Tx.Time(),
				"timeout", tx.Tx.Timeout(),
				"txHash", tx.TxHash)
			err = errInvalidTx
			return
		}
		//如果交易时间在单元时间之后则不允许处理
		if tx.Tx.IsFuture(unitTime) {
			return nil, nil, errInvalidTx
		}
	}
	tr.Tag()
	//交易执行前，需要备份快照，如果交易执行产生Error则恢复数据
	revid := db.Snapshot()
	db.Prepare(tx.TxHash, common.Hash{}, index, header.Number)
	tr.Tag()
	// 处理单条交易，错误交易跳过
	receipt, err = core.ProcessTransaction(self.dagChain.Config(), self.dagChain, db,
		header, gp, tx, totalUsedGas, vm.Config{}, nil)
	tr.Tag()

	if err != nil {
		db.RevertToSnapshot(revid)
	}
	switch err {
	case core.ErrGasLimitReached:
		log.Trace("Gas limit exceeded for current unit", "action", tx.Action, "txhash", tx.TxHash, "have", gp)
		//因为单元中Gas上限设置导致该交易执行失败，则需要将此交易回炉
		return nil, nil, err
	case nil:
		// 装填数据
		if !tx.PreStep.IsEmpty() { //如果有记录TxHash则移除Tx
			tx.Tx = nil
		} else {
			tx.Tx = types.CopyTransaction(tx.Tx) //完全复制，使单元不依赖外部任何内容
		}
	}
	tr.Tag()
	return
}

// WaitGenerateUnitTime 计算需要等待的单元打包间隔
func (self *BftMiningAss) WaitGenerateUnitTime(parentNumber uint64, lastUnitStamp uint64) time.Duration {
	cfg := time.Duration(self.config.BlockPeriod) * time.Second
	// 链的首笔交易特殊计算
	if parentNumber == 0 {
		return cfg
	}
	tm := time.Unix(0, int64(lastUnitStamp))
	next := tm.Add(cfg)
	now := time.Now()
	if next.After(now) {
		t := next.Sub(now)
		if t > cfg {
			return cfg
		}
		return t
	}
	return 0
}
