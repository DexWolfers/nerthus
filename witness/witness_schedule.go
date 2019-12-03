// 见证模块
package witness

import (
	"fmt"
	"math/big"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/mclock"
	"gitee.com/nerthus/nerthus/common/tracetime"
	"gitee.com/nerthus/nerthus/consensus"
	"gitee.com/nerthus/nerthus/consensus/arbitration"
	"gitee.com/nerthus/nerthus/consensus/arbitration/monitor"
	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/consensus/bft/backend"
	consensusProtocol "gitee.com/nerthus/nerthus/consensus/protocol"
	"gitee.com/nerthus/nerthus/core/config"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/council"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/witness/service/rwit"
	"gitee.com/nerthus/nerthus/witness/service/wconn"

	"github.com/hashicorp/golang-lru"
	"github.com/spf13/viper"
)

type WitnessSchedule struct {
	miningAssistant *BftMiningAss
	minner          consensus.Miner
	jobCenter       *MiningJobCenter
	nts             Backend // nts接口
	curWitnessGroup uint64
	curWitness      common.Address
	witnessSet      *wconn.WitnessSet   //见证人Peer信息
	arb             *arbitration.Worker //系统见证人仲裁处理

	// 公共信息
	state                int32 // 0 = stopped or default ,1=running ,2 =stopping
	doMutex              sync.Mutex
	runningWG            sync.WaitGroup
	quite                chan struct{}
	logger               log.Logger
	isSys                bool // 当前是否是系统链
	replaceWitnessWorker *rwit.Worker
	//chainList             set.Interface // 此见证人需要参与见证的链地址（用户）
	myChainCache     *lru.Cache
	notMyChainCache  *lru.Cache
	chainLastWitness struct {
		list common.AddressList
		sync.RWMutex
	} // 链最新见证人
	monitorEngine      *monitor.MonitorEngine                 // 链监控
	removePendingList  map[types.AccountRole][]common.Address //待移除的无效见证人、理事列表
	removeMux          sync.Mutex
	witnessReplaceDone chan common.Address //链更换见证人完成事件

	main *Witness
}

// signThat 利用见证人私钥签名
func signThat(backend Backend, sh types.SignHelper) error {
	private, err := backend.GetPrivateKey()
	if err != nil {
		return err
	}
	return types.SignBySignHelper(sh, types.NewSigner(backend.DagChain().Config().ChainId), private)
}

// NewWitnessSchedule new 见证模块
func NewWitnessSchedule(nts Backend, witnessSet *wconn.WitnessSet) *WitnessSchedule {

	ts := WitnessSchedule{
		nts:   nts,
		quite: make(chan struct{}),
		//chainList:  set.New(set.ThreadSafe),
		myChainCache:       newCache(10000),
		notMyChainCache:    newCache(10000),
		witnessSet:         witnessSet,
		removePendingList:  make(map[types.AccountRole][]common.Address),
		witnessReplaceDone: make(chan common.Address, 200),
	}
	return &ts
}
func newCache(size int) *lru.Cache {
	c, err := lru.New(size)
	if err != nil {
		panic(err)
	}
	return c
}

// Start 见证开启
func (t *WitnessSchedule) Start() error {
	t.doMutex.Lock()
	defer t.doMutex.Unlock()
	if state := atomic.LoadInt32(&t.state); state != 0 {
		if state == 1 {
			return fmt.Errorf("task schedule is running,can not restart")
		}
		return fmt.Errorf("task schedule is stopping,can not start,please wait for stopped")
	}

	t.curWitness = t.nts.GetMainAccount()
	if t.curWitness.Empty() {
		return fmt.Errorf("missing witness account")
	}

	t.logger = log.Root().New("module", "witness", "witness", t.curWitness.String())
	if err := t.initOrReset(false); err != nil {
		return err
	}
	cfg := bft.NewConfig(t.isSys, t.nts.DagChain().Config().ChainId)
	t.jobCenter = NewMiningJobCenter(t.nts, 0, func(msg *TxExecMsg) {
		if t.onlineWitnessIsOk() {
			t.minner.SendNewJob(msg.MC)
		} else {
			t.logger.Warn("can not start mint, online witness too less", "online", t.WitnessOnlines())
		}
	})
	t.miningAssistant = newAssistant(t.chainLastWitness.list, t.jobCenter, t, cfg)

	t.minner = t.NewMinner(cfg)
	if err := t.runMinne(); err != nil {
		return err
	}
	// 需要监控系统链
	t.monitorEngine = monitor.NewMonitorEngine(params.SCAccount, t.nts.DagChain(),
		func(c common.Address) { t.stopChain(c) }, t.recoverChain)
	t.monitorEngine.Start()
	// 提案校对方法注入
	t.nts.DagChain().SetProposalVerify(t.minner.VerifyWithProposal)
	t.addJob(t.heartbeatCheck)
	t.addJob(t.processReportUnitCh)
	t.addJob(t.processChainStatus)
	t.addJob(t.listenProcessTxDel)
	t.addJob(t.listenOnlineChange)
	t.addJob(t.drawMoneyCheck)

	atomic.StoreInt32(&t.state, 1) //running
	go t.processOldTask()
	return nil
}

func (t *WitnessSchedule) initOrReset(reset bool) error {
	//断开所有连接
	if t.witnessSet != nil {
		for _, v := range t.witnessSet.AllWitness() {
			t.witnessSet.RemoveWitness(v)
		}
	}

	groupIndex, err := t.nts.DagChain().GetWitnessGroupIndex(t.curWitness)
	if err != nil {
		return fmt.Errorf("failed to get witness group,%v", err)
	}
	t.curWitnessGroup = groupIndex
	t.isSys = groupIndex == sc.DefSysGroupIndex

	if reset {
		t.jobCenter.Reset()
		t.myChainCache.Purge()
		t.notMyChainCache.Purge()

		t.stopMinne()
	}
	if err := t.reloadWitnessList(); err != nil {
		t.chainLastWitness.Lock()
		t.chainLastWitness.list = t.chainLastWitness.list[:0]
		t.chainLastWitness.Unlock()
		return err
	}

	if reset {
		if t.main != nil {
			if err := t.main.conns.Reload(); err != nil {
				t.logger.Crit("failed reset witness conn info", "err", err)
			}
		}
		if err := t.runMinne(); err != nil {
			t.logger.Crit("failed run minne", "err", err)
		}

		go t.processOldTask()
	}
	return nil
}

func (t *WitnessSchedule) reloadWitnessList() error {
	if witnessList, err := t.nts.DagChain().GetWitnessAtGroup(t.curWitnessGroup); err != nil {
		return err
	} else {
		t.chainLastWitness.Lock()
		t.chainLastWitness.list = witnessList
		t.chainLastWitness.Unlock()
	}
	if t.main != nil {
		t.main.conns.Reload()
	}
	return nil
}

// 当前见证人是否是系统主见证人
func (t *WitnessSchedule) IsSCMainWitness() (isMain bool) {
	if !t.isSys {
		return false
	}
	index := t.ChainWitness().HaveIdx(t.curWitness)
	return index >= 0 && index < params.ConfigParamsSCWitness
}

// runMinne 开启共识引擎
func (t *WitnessSchedule) runMinne() error {
	if t.isSys && !t.IsSCMainWitness() {
		return nil
	}
	t.logger.Info("start bft miner")
	if err := t.minner.Start(); err != nil {
		return err
	}
	t.minner.NextChainSelector(func(exist func(nextChain common.Address) bool) common.Address {
		var find common.Address
		//var count int
		t.jobCenter.RangeChain(func(chain common.Address) bool {
			//count++
			if t.jobCenter.IsInBlack(chain) {
				return true //继续取
			}
			if exist(chain) {
				return true //已存在，继续取
			}
			find = chain
			return false //已找到，终止遍历
		})
		//log.Info("select next chain", "chain", find, "chain.count", count)
		return find
	})
	if !t.isSys {
		return nil
	}
	tail := t.nts.DagChain().GetChainTailHead(params.SCAccount)
	t.minner.SendNewUnit(params.SCAccount, tail.Number, tail.Hash())

	//主系统见证人
	t.replaceWitnessWorker = rwit.New(&replaceWitnessBackend{t},
		t.nts.DagChain(), t.nts.ChainDb(),
		types.MakeSigner(*t.nts.DagChain().Config()))
	t.replaceWitnessWorker.Start()

	//系统见证人还需要启动作恶仲裁
	arb, err := arbitration.New(t.nts.DagChain(), t.nts.ChainDb(), t, t.curWitness) //
	if err != nil {
		return err
	}
	t.arb = arb
	go t.addJob(t.removeInvalidWitnessCheck)

	//启动仲裁
	return t.arb.Start()
}
func (t *WitnessSchedule) stopMinne() error {
	if t.replaceWitnessWorker != nil {
		t.replaceWitnessWorker.Stop()
		t.replaceWitnessWorker = nil
	}
	if t.arb != nil {
		t.arb.Stop()
		t.arb = nil
	}
	if t.replaceWitnessWorker != nil {
		t.replaceWitnessWorker.Stop()
		t.replaceWitnessWorker = nil
	}
	if t.minner != nil {
		return t.minner.Stop()
	}
	return nil
}

// NewMinner 初始化共识引擎
func (t *WitnessSchedule) NewMinner(cfg *bft.Config) consensus.Miner {
	pk, err := t.nts.GetPrivateKey()
	if err != nil {
		panic(err)
	}
	return backend.New(cfg, t.nts.EventMux(), t.nts.DagChain(), pk, t.miningAssistant)
}

// 见证关闭
func (t *WitnessSchedule) Stop() error {
	t.doMutex.Lock()
	defer t.doMutex.Unlock()
	if state := atomic.LoadInt32(&t.state); state == 0 {
		return nil
	} else if state == 2 {
		return fmt.Errorf("task schedule is stopping")
	}

	atomic.StoreInt32(&t.state, 0) //stopped
	close(t.quite)

	t.stopMinne()

	t.jobCenter.Stop()
	t.monitorEngine.Stop()
	t.runningWG.Wait()
	return nil
}

// isRunning 是否是在运行中，防止发生数据给已关闭的channel
func (t *WitnessSchedule) isRunning() bool {
	return t.state == 1
}

// 交易打包成交易任务并添加到交易列表中
func (t *WitnessSchedule) addExecMessage(action types.TxAction, orignTx *types.Transaction, preStep types.UnitID,
	mc common.Address, trs ...*tracetime.TraceTime) {
	bt := mclock.Now()
	err := t.jobCenter.Push(mc, orignTx, action, preStep)
	t.logger.Trace("pushed execMessage",
		"chain", mc, "action", action, "txHash", orignTx.Hash(), "cost", mclock.Now()-bt, "err", err)
}

// 新建任务
func (t *WitnessSchedule) addJob(job func()) {
	t.runningWG.Add(1)
	go func() {
		fname := runtime.FuncForPC(reflect.ValueOf(job).Pointer()).Name()
		t.logger.Trace("begin run job", "func", fname)
		defer t.runningWG.Done()
		defer t.logger.Trace("job done", "func", fname)
		job()
	}()
}

// listenProcessTxDel 接收单元删除信号
func (t *WitnessSchedule) listenProcessTxDel() {
	sub := t.nts.EventMux().Subscribe(types.ReplayTxActionEvent{})
	defer sub.Unsubscribe()

	for {
		select {
		case <-t.quite:
			return
		case obj, ok := <-sub.Chan():
			if !ok {
				return
			}
			switch ev := obj.Data.(type) {
			case types.ReplayTxActionEvent: //交易回发到交易池
				err := t.nts.TxPool().AddRemote(ev.Tx)
				if err != nil {
					t.logger.Error("failed replay transaction", "txhash", ev.Tx.Hash(), "err", err)
				}
			}
		}
	}
}

// handleConsensusMsg 收到共识消息事件,发送共识消息
func (t *WitnessSchedule) handleConsensusMsg(ev consensusProtocol.MessageEvent) {
	select {
	case <-t.quite:
		return
	default:
		if t.minner != nil {
			// 黑名单判断
			if t.jobCenter.IsInBlack(ev.Chain) {
				t.logger.Debug("chain at black list,ignore bft msg", "chain", ev.Chain)
				return
			}
			t.minner.OnMessage(ev)
		}
	}
}

// 停止链
func (t *WitnessSchedule) stopChain(chain common.Address) bool {
	t.logger.Debug("stop chain work", "mc", chain)
	if t.jobCenter.PushBlack(chain) {
		t.minner.StopChainForce(chain)
		t.jobCenter.ClearChainForce(chain)
		return true
	}
	return false
}
func (t *WitnessSchedule) recoverChain(chain common.Address) {
	//从黑名单中移除，并启动
	t.jobCenter.RemoveBlack(chain)
	t.minner.SendNewJob(chain)
}

// ProcessNewUnit 处理新单元
func (t *WitnessSchedule) ProcessNewUnit(unit *types.Unit, tr *tracetime.TraceTime) {
	//tr := tracetime.New().AddArgs("chain", unit.MC(), "number", unit.Number(), "txs", len(unit.Transactions())).SetMin(0)
	tr.Tag("process").AddArgs("txs", len(unit.Transactions()))

	log.Debug("unit event", "chain", unit.MC(), "number", unit.Number(), "uhash", unit.Hash())
	if err := t.MyIsUserWitness(unit.MC()); err == nil {
		tr.Tag()
		// 防止交易丢失，尝试回滚共识提案内的交易
		t.minner.RollBackProposal(unit.Hash(), unit.MC())
		// 移除已处理交易记录
		for _, tx := range unit.Transactions() {
			t.jobCenter.Remove(unit.MC(), tx.TxHash, tx.Action)
		}
		tr.Tag()
		t.minner.SendNewUnit(unit.MC(), unit.Number(), unit.Hash())
		tr.Tag()
		if unit.MC() == params.SCAccount {
			t.monitorEngine.RefreshStatus()
		}
	} else if err != types.ErrNonCurUserwitness {
		t.logger.Error("confirm user witness failed", "chain", unit.MC(), "err", err)
	}

	//tr.Stop()
}

// processReportUnitCh 收到举报单元
func (t *WitnessSchedule) processReportUnitCh() {
	reportUnitCh := make(chan types.FoundBadWitnessEvent, 1024)
	sub := t.nts.DagChain().SubscribeReportUnitEvent(reportUnitCh)
	defer sub.Unsubscribe()
	for {
		select {
		case <-sub.Err():
			return
		case <-t.quite:
			return
		case ev, ok := <-reportUnitCh:
			if !ok {
				return
			}
			t.logger.Info("Found bad witness", "witness", ev.Witness.Hex(), "chain", ev.Bad.MC, "number", ev.Bad.Number, "err", ev.Reason)
			mc := ev.Bad.MC
			if mc != params.SCAccount {
				t.stopChain(mc) //停止相关链工作
			}
			// TODO(ysqi): 也许会重复举报，但没关系，可以优化
			curr := t.nts.GetMainAccount()
			if curr == ev.Witness {
				//如果是本人作恶，则直接关闭程序
				t.logger.Error("Discovering my dishonesty as closing the witness service", "cause", ev.Reason)
				go t.nts.StopWitness() //停止见证
				return
			}
		}
	}
}

// processChainStatus 收到关闭/启动链事件
func (t *WitnessSchedule) processChainStatus() {
	ch := make(chan types.ChainMCProcessStatusChangedEvent, 10)
	sub := t.nts.DagChain().SubscribeChainProcessStatusCHangedEvent(ch)
	defer sub.Unsubscribe()
	for {
		select {
		case <-sub.Err():
			return
		case <-t.quite:
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			switch ev.Status {
			default:
				//panic(fmt.Errorf("can not handle status,%v", ev.Status))
			case types.ChainNeedStop: //需要停止链
				if ev.MC == params.SCAccount {
					t.logger.Debug("reported unit is belong to system chain", "mc", ev.MC)
					continue
				}
				log.Info("receive need stop chain witness event", "mc", ev.MC)

				// 暂停相关的MC工作
				//如果是作恶引起的停止，由报告作恶事件触发停止动作
				if reason, ok := ev.Reason.(types.FoundBadWitnessEvent); ok {
					if reason.Bad.MC == ev.MC {
						log.Debug("skip stop chain witness event", "mc", ev.MC)
						continue
					}
				}
				t.stopChain(ev.MC)
			case types.ChainCanStart:
				log.Info("receive can start chain witness event", "mc", ev.MC)
				t.recoverChain(ev.MC)
			}
		}
	}
}

// 更新我的见证人清单
func (t *WitnessSchedule) updateMyUser(user common.Address, isMy bool) {
	if t.isSys { //系统见证人只有一个用户就是系统链
		return
	}
	if isMy {
		t.notMyChainCache.Remove(user)
		t.myChainCache.Add(user, struct{}{})
	} else {
		t.myChainCache.Remove(user)
		t.notMyChainCache.Add(user, struct{}{})
	}
}

// MyIsUserWitness 校验当前节点是否是给定用户的见证人
func (t *WitnessSchedule) MyIsUserWitness(user common.Address) (err error) {
	if t.isSys {
		if params.SCAccount == user {
			return nil
		}
		return types.ErrNonCurUserwitness
	}
	if t.notMyChainCache.Contains(user) {
		return types.ErrNonCurUserwitness
	}
	if t.myChainCache.Contains(user) {
		return nil
	}
	stateDB, err := t.nts.DagChain().GetChainTailState(params.SCAccount)
	if err != nil {
		return err
	}
	index, err := sc.GetGroupIndexFromUser(stateDB, user)
	if err != nil {
		return err
	}
	isMyUser := t.curWitnessGroup == index
	if isMyUser {
		//如果是我的见证人，则还需判断是否处理更换见证人中，如果是则不处理
		chainStatus, _ := sc.GetChainStatus(stateDB, user)
		isMyUser = chainStatus == sc.ChainStatusNormal
	}

	t.logger.Trace("is my chains", "chain", user, "yes", isMyUser,
		"chain.witness_group", index, "me", t.curWitnessGroup)

	t.updateMyUser(user, isMyUser)
	if isMyUser {
		return nil
	} else {
		return types.ErrNonCurUserwitness
	}
}

// processOldTask 处理当前见证人用户的历史交易和查找交易池中的未处理交易
func (t *WitnessSchedule) processOldTask() {
	// 重新取出待处理交易
	txs, err := t.nts.TxPool().Pending()
	if err != nil {
		log.Error("Failed get pending from txpool", "err", err)
	} else {
		log.Trace("Check pending traction", "txSenderCount", len(txs))
		for _, tx := range txs {
			// push work
			t.ProcessNewTx(tx, tracetime.New())
		}
	}

	if t.isSys { //降低遍历数
		t.addOldTx(params.SCAccount)
	} else {
		//遍历所有带待处理
		t.addOldTx(common.Address{})
	}
}

// ChainWitness 获取链的最新见证人
// 最新见证人时在两个时机自动更新：
//		1. 链更换见证人时，清除缓存
//		2. 部署新合约时，实时添加到缓存（如果是本见证人的用户）
func (t *WitnessSchedule) ChainWitness() common.AddressList {
	t.chainLastWitness.RLock()
	defer t.chainLastWitness.RUnlock()
	return t.chainLastWitness.list
}

// WitnessOnlines 获取见证人peer人数
func (t *WitnessSchedule) WitnessOnlines() int {
	return t.witnessSet.Len()
}

// heartbeatCheck 定时任务,检查是否需要发送见证人心跳
func (self *WitnessSchedule) heartbeatCheck() {
	self.logger.Debug("start send heartbeat timer")

	// 设置循环间隔
	// 见证人心跳，只需要保证一个周期内有一次心跳即可：

	var oldTx common.Hash //再新交易时删除旧交易

	sendHeartbeat := func() {
		// 替补系统见证人、用户见证人 发送心跳交易
		// 备选、用户见证人 需要发送心跳
		if self.curWitnessGroup == sc.DefSysGroupIndex {
			if self.ChainWitness().HaveIdx(self.curWitness) < params.ConfigParamsSCWitness {
				return
			}
		}
		if self.SysChainDataIsTooOld() {
			log.Error("can not send heartbeat tx when system chain is too old", "sys.number", self.nts.DagChain().GetChainTailNumber(params.SCAccount))
			return
		}
		//不需要检查，只需要周期性发送交易，防止出现无心跳情况
		// 组装空Tx， 时间默认为 20 分钟
		tx := types.NewTransaction(self.curWitness, params.SCAccount, big.NewInt(0), 0, 0,
			types.DefTxMinTimeout, sc.CreateCallInputData(sc.FuncIdSystemHeartbeat))
		err := signThat(self.nts, tx)
		if err != nil {
			log.Error("Important: failed to send heartbeat tx", "err", err)
			return
		}
		if !oldTx.Empty() {
			self.nts.TxPool().Remove(oldTx)
		}
		oldTx = tx.Hash()

		peers := self.nts.PeerSet().Peers()
		if len(peers) == 0 {
			log.Warn("send heartbeat failed", "tx", tx.Hash(), "err", "no peer connected")
		} else {
			txs := []*types.Transaction{tx}
			var success int
			for _, p := range peers {
				err := p.SendTransactions(txs)
				if err != nil {
					log.Warn("send heartbeat failed", "tx", tx.Hash(), "peer", p.ID(), "err", err)
				} else {
					success++
				}
			}
			log.Info("push heartbeat tx", "tx", tx.Hash().String(), "peers", len(peers), "success", success)
		}
		//不管是否有成功发送，均发送到本地交易池
		go func() {
			if err := self.nts.TxPool().AddLocal(tx); err != nil {
				log.Warn("failed to send heartbeat to txpool", "tx", tx.Hash().String(), "err", err)
			}
		}()
	}

	sendHeartbeat()

	//心跳不需要非常频繁，当前见证节点过多、心跳频繁时系统链所处理的心跳交易非常多。
	//而心跳过于频繁是没有必要的，因为剔除一个见证人的时间是两天。
	tk := time.NewTicker(time.Hour)
	defer tk.Stop()
	for {
		select {
		case <-self.quite:
			return
		case <-tk.C:
			sendHeartbeat()
		}
	}
}
func (self *WitnessSchedule) drawMoneyCheck() {
	configItem := "witness.auto_withdrawal"
	if !config.MustBool(configItem, true) {
		self.logger.Info("disable witness automatic withdrawal", "config", configItem)
		return
	} else {
		self.logger.Info("enable and start witness automatic withdrawal", "config", configItem)
	}
	defer log.Info("stopped witness automatic withdrawal")

	//一个周期执行依次结算
	tk := time.NewTicker(time.Duration(params.ConfigParamsBillingPeriod) *
		time.Duration(params.OneNumberHaveSecondBySys) * time.Second)
	defer tk.Stop()

	for {
		select {
		case <-self.quite:
			return
		case <-tk.C:
			logger := self.logger.New("do", "auto check withdraw money")
			tx, err := council.GenerateSettlementTx(self.nts.DagChain(), self.curWitness)
			if err != nil {
				logger.Warn("generate settlement tx err", "err", err)
				continue
			}
			err = signThat(self.nts, tx)
			if err != nil {
				logger.Error("sign tx err", "err", err)
				continue
			}
			err = self.BroadcastTx(tx)
			if err != nil {
				logger.Error("broadcast tx err", "err", err)
				continue
			}

			logger.Info("auto send tx to withdraw additional fee", "tx", tx.Hash())
		}
	}
}

// removeInvalidWitnessCheck
func (self *WitnessSchedule) removeInvalidWitnessCheck() {
	tk := time.NewTicker(params.OneNumberHaveSecondBySys * time.Second * params.ConfigParamsSCWitness)
	for {
		select {
		case <-self.quite:
			tk.Stop()
			return
		case <-tk.C:
			if self.isSys {
				// 获取心跳不符的 备选、用户见证人、理事
				dag := self.nts.DagChain()
				last := dag.GetChainTailHead(params.SCAccount)
				statedb, err := dag.StateAt(last.MC, last.StateRoot)
				if err != nil {
					self.logger.Error("check witnesses heartbeat failed", "err", err)
					continue
				}
				sys2, err := sc.GetTimeoutSecondSystemWitness(statedb, last.Number)
				if err != nil {
					self.logger.Error("check system heartbeat failed", "err", err)
				}
				user, err := sc.GetInvalidUserWitness(statedb, last.Number)
				if err != nil {
					self.logger.Error("check user heartbeat failed", "err", err)
				}
				coucil := sc.GetTimeoutCouncil(statedb, last.Number)

				// 更新待移除列表
				self.removeMux.Lock()
				self.removePendingList[types.AccSecondSystemWitness] = sys2
				self.removePendingList[types.AccUserWitness] = user
				self.removePendingList[types.AccCouncil] = coucil
				self.removeMux.Unlock()
				self.logger.Debug("check invalid witness/council", "sys secondary", len(sys2), "user", len(user), "council", len(coucil))
			}
		}
	}
}

// newOtherTx 新建其他交易
func (self *WitnessSchedule) newOtherTx(number uint64, db sc.StateDB) ([]*TxExecMsg, error) {
	var txList []*TxExecMsg
	if self.isSys {
		if tx, err := self.generateHeartbeatTx(); err != nil {
			return nil, err
		} else {
			txList = append(txList, tx)
		}
		if tx, err := self.generateRemoveTx(number, db); err != nil {
			return nil, err
		} else {
			if tx != nil {
				txList = append(txList, tx...)
			}
		}

		//更换见证人指定交易
		if self.replaceWitnessWorker != nil {
			txs := self.replaceWitnessWorker.GenerateTxs()
			for _, tx := range txs {
				txList = append(txList, newTask(types.ActionSCIdleContractDeal, params.SCAccount, tx))
				self.logger.Trace("created exec replace witness tx", "txhash", tx.Hash())
			}
		}
	}
	return txList, nil
}

// generateEmpty 生成系统链心跳空交易
func (self *WitnessSchedule) generateHeartbeatTx() (*TxExecMsg, error) {
	// 组装空Tx
	tx := types.NewTransaction(self.curWitness, params.SCAccount, big.NewInt(0), 0,
		0, types.DefTxMinTimeout*5, sc.CreateCallInputData(sc.FuncIdSystemHeartbeat))
	err := signThat(self.nts, tx)
	if err != nil {
		return nil, err
	}
	return newTask(types.ActionSCIdleContractDeal, params.SCAccount, tx), nil
}

// generateNewWit 生成更换系统见证人交易
func (self *WitnessSchedule) generateRemoveTx(number uint64, db sc.StateDB) ([]*TxExecMsg, error) {
	//如果是测试模式，检查是否不启用更换系统见证人，使得测试时不会动态变更系统见证人
	if viper.GetBool("testci.disableAutoChangeSystemWitness") {
		return nil, nil
	}
	// 构建踢人交易，踢出心跳不满足在线条件的备选、用户见证人、理事
	// 定时任务已筛选一批待踢出地址，这里需要核实
	var invalid []common.Address
	self.removeMux.Lock()
	defer self.removeMux.Unlock()
	for k, v := range self.removePendingList {
		if len(v) == 0 {
			continue
		}
		for i := range v {
			if sc.CheckHeartTimeout(db, k, v[i], number) {
				invalid = append(invalid, v[i])
			}
		}
	}
	if len(invalid) == 0 {
		return nil, nil
	}
	buildTx := func(addrs []common.Address) (*TxExecMsg, error) {
		input := sc.CreateCallInputData(sc.FuncIdRemoveInvalid, addrs)
		tx := types.NewTransaction(self.curWitness, params.SCAccount, big.NewInt(0), 0,
			0, types.DefTxTimeout, input)
		err := signThat(self.nts, tx)
		if err != nil {
			return nil, err
		}
		return newTask(types.ActionSCIdleContractDeal, params.SCAccount, tx), nil
	}
	var txs []*TxExecMsg
	// 写入待踢出的地址
	addrLen := 100
	for i := 0; i < len(invalid); i += addrLen {
		var addrs []common.Address
		if i+addrLen >= len(invalid) {
			addrs = invalid[i:]
		} else {
			addrs = invalid[i : addrLen+i]
		}
		if tx, err := buildTx(addrs); err != nil {
			return nil, err
		} else {
			txs = append(txs, tx)
			self.logger.Debug("generate replace witness tx", "addrs", len(addrs), "hash", tx.TXHash)
		}
	}

	return txs, nil
}

// listenOnlineChange  订阅见证列表变更事件
// 见证人满足人数则开始处理任务
func (schedule *WitnessSchedule) listenOnlineChange() {
	c := make(chan types.WitnessConnectStatusEvent, 11)
	sub := schedule.witnessSet.SubChange(c)
	defer sub.Unsubscribe()
	for {
		select {
		case <-schedule.quite:
			return
		case <-c:
			if schedule.onlineWitnessIsOk() {
				for _, chain := range schedule.jobCenter.Chains() {
					//如果存在待处理，则发出信号
					if schedule.jobCenter.ChainTxs(chain) > 0 {
						schedule.minner.SendNewJob(chain)
					}
				}
			}
		}
	}
}

// onlineWitnessIsOk 判断见证人数是否满足投票人数
func (schedule *WitnessSchedule) onlineWitnessIsOk() bool {
	return schedule.witnessSet.Len()+1 >= schedule.needOnlineWitness()
}

// needOnlineWitness 返回需要的见证人投票人数
func (t *WitnessSchedule) needOnlineWitness() int {
	if t.isSys {
		return int(params.ConfigParamsSCMinVotings)
	}
	return int(params.ConfigParamsUCMinVotings)
}

// 将使用见证人签名交易，需要防止乱用
func (t *WitnessSchedule) SignTx(tx *types.Transaction) error {
	if tx.SenderAddr() != t.curWitness || tx.To() != params.SCAccount {
		return fmt.Errorf("disable this kind tx:from(%s)->to(%s)", tx.SenderAddr().Hex(), tx.To().Hex())
	}
	return signThat(t.nts, tx)
}

func (t *WitnessSchedule) BroadcastTx(tx *types.Transaction) error {
	return t.nts.TxPool().AddLocal(tx)
}

func (t *WitnessSchedule) EventMux() *event.TypeMux {
	return t.nts.EventMux()
}

// 系统链单元是否太过成旧，依旧是 5 小时前的数据
func (t *WitnessSchedule) SysChainDataIsTooOld() bool {
	tail := t.nts.DagChain().GetChainTailHead(params.SCAccount)
	if tail.Number == 0 {
		return false
	}
	now := uint64(time.Now().UnixNano())
	return now > tail.Timestamp && now-tail.Timestamp > uint64(5*time.Hour)
}
