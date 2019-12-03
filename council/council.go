// 理事心跳包
// 用来发送理事心跳交易
package council

import (
	"crypto/ecdsa"
	"errors"
	"math/big"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/consensus/arbitration/monitor"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/config"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/council/arbitration"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/nts/protocol"
	"gitee.com/nerthus/nerthus/params"
)

// 接口，用来使用其它模块功能
type Backend interface {
	EventMux() *event.TypeMux
	DagChain() *core.DagChain
	DagReader() *core.DAG
	GetMainAccount() common.Address
	TxPool() *core.TxPool
	PeerSet() protocol.PeerSet
}

// Heartbeat 理事心跳体
type Heartbeat struct {
	curCouncil    common.Address
	log           log.Logger
	nts           Backend
	quit          chan struct{}
	councils      common.AddressList
	privateKey    *ecdsa.PrivateKey
	monitorEngine *monitor.MonitorEngine
	arbitrat      *arbitration.ArbitrationSchedule
}

func New(nts Backend) *Heartbeat {
	return &Heartbeat{
		log:  log.New("module", "memberHeartbeat"),
		nts:  nts,
		quit: make(chan struct{}),
	}
}

// Start 开启理事
func (h *Heartbeat) Start(addr common.Address, key *ecdsa.PrivateKey) error {
	h.curCouncil = addr
	//保存私钥
	h.privateKey = key
	// 获取成员数据
	h.checkMemberStatus()

	// 需要监控系统链
	h.monitorEngine = monitor.NewMonitorEngine(params.SCAccount, h.nts.DagChain(),
		h.stopChain, h.recoverChain)
	h.monitorEngine.Start()
	// 理事仲裁
	h.arbitrat = arbitration.NewSchedule(h.nts.DagChain(), types.NewSigner(h.nts.DagChain().Config().ChainId), key, h.nts.EventMux())
	h.arbitrat.SetActivatedCallback(h.monitorEngine.SetStopping)

	// 开启成员循环
	go h.listenReplaceCouncilLoop()
	// 开启系统链标记
	go h.taskHeartbeatLoop()

	// 开启自动取增发
	go h.drawMoneyCheck()

	h.log.Info("System Chain member started")
	return nil
}
func (h *Heartbeat) HandleArbitratMsg(msg arbitration.CouncilMsgEvent) error {
	return h.arbitrat.Handle(msg)
}

// Stop 关闭理事
func (h *Heartbeat) Stop() error {
	close(h.quit)
	h.log.Info("System Chain member stopped")
	return nil
}

// listenReplaceCouncilLoop 监听理事加入和注销事件
// 更新理事列表
func (h *Heartbeat) listenReplaceCouncilLoop() {
	subEvents := h.nts.EventMux().Subscribe(types.SystemChainUnitEvent{})
	defer subEvents.Unsubscribe()
	for {
		select {
		case <-h.quit:
			return
		case ev, ok := <-subEvents.Chan():
			if !ok {
				return
			}
			switch ev.Data.(type) {
			case types.SystemChainUnitEvent:
				h.checkMemberStatus()
			}
		}
	}
}

// taskHeartbeatLoop 执行监听循环
// 定时判断是否需要发送理事心跳交易,因为需要通过理事心跳来计算收益
func (h *Heartbeat) taskHeartbeatLoop() {

	var oldTx common.Hash
	fun := func() {
		//不需要检查，只需要周期性发送交易，防止出现无心跳情况
		tx := types.NewTransaction(h.curCouncil, params.SCAccount, big.NewInt(0), 0,
			0, types.DefTxMinTimeout*5, sc.CreateCallInputData(sc.FuncIdMemberHeartbeat))

		err := h.signThat(tx)
		if err != nil {
			log.Error("Important: failed to send heartbeat tx", "err", err)
			return
		}
		if !oldTx.Empty() {
			h.nts.TxPool().Remove(oldTx)
		}
		oldTx = tx.Hash()

		peers := h.nts.PeerSet().Peers()
		if len(peers) == 0 {
			log.Warn("send heartbeat failed", "tx", tx.Hash(), "err", "no peer connected")
		} else {
			txs := []*types.Transaction{tx}
			for _, p := range peers {
				err := p.SendTransactions(txs)
				if err != nil {
					log.Warn("send heartbeat failed", "tx", tx.Hash(), "peer", p.ID(), "err", err)
				}
			}
			log.Info("push heartbeat tx", "tx", tx.Hash(), "peers", len(peers))
		}
		//不管是否有成功发送，均发送到本地交易池
		go h.nts.TxPool().AddLocal(tx)
	}

	fun() //启动时发送

	tick := time.NewTicker(time.Hour)
	defer tick.Stop()

	for {
		select {
		case <-tick.C:
			fun()
		case <-h.quit:
			return
		}
	}
}

// signThat 利用见证人主账号签名
func (h *Heartbeat) signThat(sh types.SignHelper) error {
	return types.SignBySignHelper(sh, types.NewSigner(h.nts.DagChain().Config().ChainId), h.privateKey)
}

// runEmptyTx 发送空交易，维持系统链心跳
func (h *Heartbeat) runEmptyTx() {
	// 组装空Tx
	tx := types.NewTransaction(h.nts.GetMainAccount(), params.SCAccount, big.NewInt(0), 0,
		0, types.DefTxMinTimeout*5, sc.CreateCallInputData(sc.FuncIdMemberHeartbeat))
	err := h.signThat(tx)
	if err != nil {
		h.log.Error("Failed to create empty tx", "err", err)
		return
	}
	h.log.Debug("runEmptyTx", "address", h.nts.GetMainAccount(), "txHash", tx.Hash().Hex())
	// 投送
	err = h.nts.TxPool().AddLocal(tx)
	if err != nil {
		h.log.Error("Failed to send empty tx", "err", err)
	}
}

// getMember 获取最新的理事列表
func (h *Heartbeat) checkMemberStatus() {
	// 取出系统链数据
	stateDB, err := h.nts.DagChain().GetChainTailState(params.SCAccount)
	if err != nil {
		h.log.Error("state get error", "err", err)
		return
	}
	// 取出当前成员数据
	// 解析
	status, isC := sc.GetCouncilStatus(stateDB, h.curCouncil)
	if !isC {
		h.log.Info("is not council member,stop work")
		h.Stop()
	} else if status != sc.CouncilStatusValid {
		h.log.Info("is not council member,stop work", "status", status)
		h.Stop()
	}
}

// checkHeartbeat 判断是否需要发送心跳交易
func (h *Heartbeat) checkHeartbeat() (bool, error) {
	if !h.councils.Have(h.nts.GetMainAccount()) {
		return false, errors.New("the target not council")
	}
	stateDB, err := h.nts.DagChain().GetChainTailState(params.SCAccount)
	if err != nil {
		return false, err
	}
	listHB, err := sc.ReadCouncilHeartbeat(stateDB)
	if err != nil {
		return false, err
	}
	round := params.CalcRound(h.nts.DagChain().GetChainTailHead(params.SCAccount).Number)
	lastTurn := listHB.QueryLastTurn(h.nts.GetMainAccount())
	if math.ForceSafeSub(uint64(round), lastTurn) < params.HeartbeatSendInterval {
		return false, nil
	}
	return true, nil
}

func (self *Heartbeat) drawMoneyCheck() {
	configItem := "council.auto_withdrawal"
	if !config.MustBool(configItem, true) {
		log.Info("disable council automatic withdrawal", "config", configItem)
		return
	} else {
		log.Info("enable and start council automatic withdrawal", "config", configItem)
	}

	defer log.Info("stopped council automatic withdrawal")

	addr := crypto.ForceParsePubKeyToAddress(self.privateKey.PublicKey)

	//TODO: 需要优化，只有在前一笔交易执行完成后，才能发送第二笔。
	// 否则因为一些特殊情况，会导致交易重复发送。
	tk := time.NewTicker(time.Duration(params.ConfigParamsBillingPeriod) *
		time.Duration(params.OneNumberHaveSecondBySys) * time.Second)
	defer tk.Stop()

	for {
		select {
		case <-self.quit:
			return
		case <-tk.C:
			//身份检查
			db, err := self.nts.DagChain().GetChainTailState(params.SCAccount)
			if err != nil {
				log.Error("failed to get chain tail state", "err", err)
			}
			if s, isC := sc.GetCouncilStatus(db, addr); !isC {
				log.Error("you are not council member,stop automatic withdrawal", "account", addr)
				return
			} else if s != sc.CouncilStatusValid {
				log.Error("you are not valid council member,stop automatic withdrawal")
				return
			}

			logger := log.New("node", addr, "do", "auto check withdraw money")
			tx, err := GenerateSettlementTx(self.nts.DagChain(), addr)
			if err != nil {
				logger.Debug("generate settlement tx err", "err", err)
				continue
			}
			err = self.signThat(tx)
			if err != nil {
				logger.Error("sign tx err", "err", err)
			}
			err = self.nts.TxPool().AddLocal(tx)
			if err != nil {
				logger.Error("broadcast tx err", "err", err)
			}
			logger.Info("auto send tx to withdraw additional fee", "tx", tx.Hash())
		}
	}
}
func (h *Heartbeat) stopChain(addresses common.Address) {
	h.arbitrat.Activating()
}
func (h *Heartbeat) recoverChain(addresses common.Address) {
	h.arbitrat.Stop()
}

// 构建提现交易
func GenerateSettlementTx(dag *core.DagChain, addr common.Address) (*types.Transaction, error) {
	tail := dag.GetChainTailHead(params.SCAccount)
	db, err := dag.StateAt(tail.MC, tail.StateRoot)
	if err != nil {
		return nil, err
	}
	gasPrice, err := sc.ReadConfigFromUnit(dag, tail, params.ConfigParamsLowestGasPrice)
	if err != nil {
		return nil, err
	}
	// 发送取现交易
	header := types.CopyHeader(tail)
	header.Number += 1
	header.SCNumber += 1
	tx := types.NewTransaction(addr, params.SCAccount, big.NewInt(0), math.MaxUint64,
		gasPrice, types.DefTxMaxTimeout, sc.CreateCallInputData(sc.FuncSettlementID))
	msg := types.NewMessage(tx, tx.Gas(), addr, tx.To(), false)
	txExec := &types.TransactionExec{Action: types.ActionContractDeal, TxHash: tx.Hash(), Tx: tx}
	receipt, err := core.ProcessTransaction(dag.Config(),
		dag, db, header, new(core.GasPool).AddGas(math.MaxBig256), txExec, big.NewInt(0), vm.Config{EnableSimulation: true}, msg)
	if err != nil {
		return nil, err
	}
	if receipt.Failed {
		return nil, errors.New("simulate exec failed")
	}

	gas := receipt.GasUsed + params.TxGas + 10000

	tx = types.NewTransaction(addr, params.SCAccount, big.NewInt(0), gas,
		gasPrice, types.DefTxTimeout, sc.CreateCallInputData(sc.FuncSettlementID))

	//余额不足时不发送
	if dag.GetBalance(tx.SenderAddr()).Cmp(tx.Cost()) <= 0 {
		return nil, errors.New("insufficient balance")
	}

	return tx, err
}
