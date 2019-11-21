package core

import (
	"errors"
	"math/big"
	"sync"
	"time"

	"gitee.com/nerthus/nerthus/common/tracetime"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/utils"
	"gitee.com/nerthus/nerthus/consensus"
	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/consensus/protocol"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
)

// New creates an consensus core
func New(backend bft.Backend, config *bft.Config, mc common.Address, packUnit func(mc common.Address) error, existTx func(mc common.Address) bool, setChainStatus func(mc common.Address, status bool), timer Timer) Engine {
	c := &core{
		sign:           types.NewSigner(config.ChainId),
		config:         config,
		address:        backend.Address(),
		chainAddress:   mc,
		state:          StateAcceptRequest,
		logger:         log.New("module", "core"),
		backend:        backend,
		packUnit:       packUnit,
		existTx:        existTx,
		setChainStatus: setChainStatus,
		timer:          timer,
		priorChan:      make(chan []interface{}, 1),
	}
	c.validateFn = c.checkValidatorSignature
	return c
}

type core struct {
	sign         types.Signer
	config       *bft.Config
	address      common.Address
	chainAddress common.Address
	state        State
	logger       log.Logger

	backend bft.Backend

	valSet                bft.ValidatorSet // 当前的validator
	waitingForRoundChange bool             //是否在等待轮次的更替
	waitingTimes          uint
	validateFn            func(m *protocol.Message) (common.Address, error)

	backlogs []*protocol.Message

	current *roundState

	roundChangeSet *roundChangeSet // 轮次变更的消息集合

	debugLogArgs         []interface{}
	timer                Timer
	activated            bool
	activateLock         sync.Mutex
	packUnit             func(mc common.Address) error
	existTx              func(mc common.Address) bool
	setChainStatus       func(mc common.Address, status bool)
	roundchangeResetTime time.Time
	priorChan            chan []interface{}
	locker               sync.Mutex
	lockHashes           map[common.Address]common.Hash // 记录所有锁定的提案hash
}

// 打包消息
func (c *core) finalizeMessage(msg *protocol.Message) error {
	var err error
	// 增加发送者地址
	msg.Address = c.Address()
	msg.IsMe = true
	msg.TraceTime = uint64(time.Now().UnixNano())

	logger := c.logger.New("func", "core.finalizeMessage", "TraceTime", msg.TraceTime)
	defer func(b time.Time) {
		logger.Trace("finalizeMessage finish", "cost", common.PrettyDuration(time.Since(b)))
	}(time.Now())
	logger.Trace("finalizeMessage")
	if msg.Code == protocol.MsgRoundChange {
		msg.Extra = utils.RandomFnv64aBytes()
	}

	// Sign message
	if msg.Sign == nil {
		msg.Sign = types.NewSignContent()
	}
	logger.Trace("begin sign")
	_, err = c.backend.Sign(msg)
	if err != nil {
		return err
	}
	logger.Trace("sign end")
	return nil
}

// 广播消息
func (c *core) broadcast(msg *protocol.Message) {
	tr := tracetime.New("code", msg.Code, "chain", c.chainAddress).SetMin(0)
	defer tr.Stop()

	logger := c.logger.New("state", c.current.String(c.state, c.waitingForRoundChange), "msgType", msg.Code)
	logger.Debug("backend broadcast", "code", msg.Code)
	if msg.IsMe {
		c.logger.Debug("ignore the message come from me", "code", msg.Code)
		return
	}
	// 打包消息，签名
	if err := c.finalizeMessage(msg); err != nil {
		logger.Error("Failed to finalize message", "msg", msg, "err", err)
		return
	}
	tr.AddArgs("mid", msg.ID())
	tr.Tag()
	m := protocol.MessageEvent{
		Peer:    c.address,
		Chain:   c.chainAddress,
		Message: msg,
	}
	if err := c.backend.Broadcast(c.valSet, m); err != nil {
		logger.Warn("Failed to broadcast message", "msg", msg, "err", err)
		tr.Tag()
		return
	}
	tr.Tag()
}

// 返回当前的视图
func (c *core) currentView() *bft.View {
	return &bft.View{
		Sequence: new(big.Int).Set(c.current.Sequence()),
		Round:    new(big.Int).Set(c.current.Round()),
	}
}

// 检查自己是不是提案者
func (c *core) isProposer() bool {
	v := c.valSet
	if v == nil {
		return false
	}
	return v.IsProposer(c.address)
}
func (c *core) isNextProposer() bool {
	v := c.valSet
	if v == nil {
		return false
	}
	return v.IsNextProposer(c.address, c.current.round.Uint64())
}

/// 共识达成，提交
func (c *core) commit() {
	c.addDebugLog("do", "commit")
	c.setState(StateCommitted)
	tr := tracetime.New("chain", c.chainAddress, "number", c.current.sequence, "round", c.current.round).SetMin(0)
	defer tr.Stop()
	proposal := c.current.Preprepare
	tr.AddArgs("uhash", proposal.Proposal.Hash())
	if proposal != nil && proposal.Proposal != nil {
		committedSeals := make([]types.WitenssVote, c.current.Commits.Size())
		for i, v := range c.current.Commits.Values() {
			committedSeals[i] = *v.CommittedSeal
		}
		tr.Tag()
		if err := c.backend.Commit(proposal.Mining, committedSeals); err != nil &&
			err != consensus.ErrKnownBlock &&
			err != consensus.ErrInsertingBlock {
			//c.current.UnlockHash() //Unlock block when insertion fails
			//c.sendNextRoundChange()
			//bft.PanicSanity(err)
			c.logger.Error("faied to commit unit",
				"number", proposal.Proposal.Number(), "uhash", proposal.Proposal.Hash(),
				"chain", c.chainAddress,
				"err", err)
		}
		tr.AddArgs("status", "ok")
		tr.Tag()
		// 共识达成，清理状态
		c.handleFinalCommitted()
	}
}

// 启动新的轮次
// startNewRound starts a new round. if round equals to 0, it means to starts a new sequence
func (c *core) startNewRound(round *big.Int, preMaxChangeBytes [][]byte, trace ...*tracetime.TraceTime) {
	//var tr *tracetime.TraceTime
	//if len(trace) == 0 {
	//	tr = tracetime.New()
	//	defer tr.Stop()
	//} else {
	//	tr = trace[0]
	//}
	tr := tracetime.New().SetMin(0)
	defer tr.Stop()
	tr.Tag("newround")
	c.addDebugLog("do", "startNewRound")
	var roundChange bool
	lastProposal, lastProposer := c.getTailHeader()
	if round.Cmp(big.NewInt(0)) == 0 {
		roundChange = false
		lastProposalNumber := big.NewInt(int64(lastProposal.Number))
		if c.current != nil && lastProposalNumber.Cmp(c.current.sequence) < 0 {
			c.logger.Warn("Start new round failed", "number", lastProposalNumber, "sequence", c.current.sequence)
			return
		}
	} else {
		roundChange = true
		if c.current == nil {
			c.logger.Warn("round change need init first", "number", lastProposal.Number, "round", round)
			c.startNewRound(big.NewInt(0), nil)
		}
		if c.current.round.Cmp(round) >= 0 {
			c.logger.Warn("round change failed", "number", lastProposal.Number, "sequence", c.current.sequence, "current_round", c.current.round, "to_round", round)
			return
		}
	}
	tr.Tag().AddArgs("chain", c.chainAddress, "more", roundChange)
	var newView *bft.View
	if roundChange {
		newView = &bft.View{
			Sequence: new(big.Int).Set(c.current.Sequence()),
			Round:    new(big.Int).Set(round),
		}
	} else {
		newView = &bft.View{
			Sequence: big.NewInt(int64(lastProposal.Number) + 1),
			Round:    big.NewInt(0),
		}
		if err := c.refreshValidtors(lastProposal); err != nil {
			c.logger.Error("can't get the validators", "lastProposalHeight", lastProposal.Number, "lastProposalHash", lastProposal.Hash(), "err", err)
			return
		}
	}
	tr.Tag()
	tr.AddArgs("number", newView.Sequence, "round", newView.Round)
	// Update logger
	logger := c.logger.New("lastProposer", lastProposer.Hex(), "last_number", lastProposal.Number, "new view", newView)
	tr.Tag()
	c.roundChangeSet = newRoundChangeSet(c.valSet, nil)
	// New snapshot for new round
	c.updateRoundState(newView, c.valSet, roundChange, round)
	tr.Tag()
	// Calculate new proposer
	c.valSet.CalcProposer(lastProposer, newView.Round.Uint64())
	tr.Tag()
	tr.AddArgs("proposer", c.valSet.GetProposer().Address())
	// 重置状态，状态机重置为StateAcceptRequest
	c.updateWaitForRoundChange(false)
	c.waitingTimes = 0
	tr.Tag()
	c.setState(StateAcceptRequest)
	tr.Tag()
	if roundChange && c.isProposer() && c.current != nil {
		// If it is locked, propose the old proposal
		// If we have pending request, propose pending request
		if c.current.IsHashLocked() {
			c.sendPreprepare(c.current.Proposal())
		} else if c.current.pendingRequest != nil {
			tr.Tag()
			c.sendPreprepare(c.current.pendingRequest.Proposal.Unit)
		} else {
			tr.Tag()
			// 无提案，立即打包
			err := c.generateProposal()
			if err != nil {
				tr.Tag()
				c.sendGiveUpPreprepare()
				tr.Tag()
				logger.Warn("pack unit err,give up propose", "err", err)
			}
		}
	}

	tr.Tag()
	// 开启新的轮次时钟 ps:新高度需要等待工作信号
	if roundChange {
		c.checkDeadLock() //检查是否死锁
		c.setTimeout(c.config.RequestTimeout)
	} else {
		c.lockHashes = nil // 新高度清理lock记录
		c.stopTimer()
	}
	tr.Tag().AddArgs("status", "ok")
	logger.Debug("New round", "new_round", newView.Round, "new_seq", newView.Sequence,
		"new_proposer", c.valSet.GetProposer(),
		"state", c.current.String(c.state, c.waitingForRoundChange),
		"valSet", c.valSet.List(), "size", c.valSet.Size(), "isProposer", c.isProposer())
}
func (c *core) refreshValidtors(last *types.Header) error {
	if c.chainAddress.Empty() {
		return errors.New("chain is stopping")
	}
	if vals, err := c.backend.Validators(c.chainAddress, last); err != nil {
		return err
	} else if vals.Size() == 0 {
		c.Stop()
		return errors.New("chain validator is empty")
	} else {
		if _, v := vals.GetByAddress(c.backend.Address()); v == nil {
			return errors.New("is not my chain")
		}
		c.valSet = vals
	}
	return nil
}
func (c *core) catchUpRound(view *bft.View) {
	logger := c.logger.New("old_round", c.current.Round(), "old_seq", c.current.Sequence(), "old_proposer", c.valSet.GetProposer())
	c.updateWaitForRoundChange(true)
	c.setTimeout(c.config.RequestTimeout)
	logger.Trace("Catch up round", "new_round", view.Round, "new_seq", view.Sequence, "new_proposer", c.valSet)
}

func (c *core) updateWaitForRoundChange(f bool) {
	c.waitingForRoundChange = f
	//c.traceState.UpdateWaitForRoundChange(c.current.Sequence(), c.current.Round(), f)
}

// updateRoundState updates round state by checking if locking block is necessary
// 使用新轮次是，如果已经锁定某个提案Hash，则继续使用该提案，而且其preprepare也被继承下来
func (c *core) updateRoundState(view *bft.View, validatorSet bft.ValidatorSet, roundChange bool, round *big.Int) {
	// Lock only if both roundChange is true and it is locked
	c.logger.Debug("before update round state", "roundchange", roundChange, "round", round.Uint64(), "oldcState", c.current.String(c.state, c.waitingForRoundChange))
	defer func() {
		c.logger.Debug("after update round state", "roundchange", roundChange, "round", round.Uint64(), "newcState", c.current.String(c.state, c.waitingForRoundChange))
	}()
	if roundChange && c.current != nil {
		if c.current.IsHashLocked() {
			c.current = newRoundState(view, validatorSet, c.chainAddress, c.current.GetPolcRound(), c.current.Preprepare, c.current.pendingRequest)
		} else {
			c.current = newRoundState(view, validatorSet, c.chainAddress, nil, nil, c.current.pendingRequest)
		}
	} else if round.Uint64() == 0 {
		// call from NewHeader
		lastProposal, _ := c.getTailHeader()
		var pRNumber uint64
		var warning bool
		if c.current != nil && c.current.pendingRequest != nil {
			// pRNumber should equal = lastProposal + 1 and equal
			pRNumber = c.current.pendingRequest.Proposal.Unit.Number()
			warning = pRNumber > c.current.sequence.Uint64() || pRNumber > lastProposal.Number
		}
		c.logger.Trace("Pending request, lock hash will be cleared", "warning", warning, "pR_height", pRNumber, "last_height", lastProposal.Number,
			"new_view.seq", view.Sequence.Uint64(), "new_view.round", view.Round.Uint64(), "cState", c.current.String(c.state, c.waitingForRoundChange))
		c.current = newRoundState(view, validatorSet, c.chainAddress, nil, nil, nil)
	}
}

// 更新状态
func (c *core) setState(state State) {
	c.logger.Debug("setstate", "old state", c.state, "new state", state)
	if c.state != state {
		// 如果收到正确提案，尝试激活链
		if c.state == StateAcceptRequest && state == StatePreprepared {
			c.activateChain()
		}
		c.state = state
		c.processBacklog()
	}
}

func (c *core) Address() common.Address {
	return c.address
}

func (c *core) stopTimer() {
	c.timer.StopTimer(c.chainAddress)
	c.roundchangeResetTime = time.Now()
}

// 校验消息签名
func (c *core) checkValidatorSignature(m *protocol.Message) (common.Address, error) {
	// 1. Get signature address
	signer, err := m.Sender(c.sign)
	if err != nil {
		log.Error("Failed to get signer address", "err", err)
		return common.Address{}, err
	}

	// 2. Check validator
	if _, val := c.valSet.GetByAddress(signer); val != nil {
		return val.Address(), nil
	}

	return common.Address{}, bft.ErrUnauthorizedAddress
}
func (c *core) getTailHeader() (*types.Header, common.Address) {
	return c.backend.TailHeader(c.chainAddress)
}

// 打包单元
func (c *core) generateProposal() error {
	return c.packUnit(c.chainAddress)
}
