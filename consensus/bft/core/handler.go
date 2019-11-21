package core

import (
	"fmt"
	"math/big"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/objectpool"
	"gitee.com/nerthus/nerthus/common/tracetime"
	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/consensus/protocol"
	"gitee.com/nerthus/nerthus/core/types"
)

func (c *core) Start() {
	//初始化
	c.logger = c.logger.New("node", c.address.ShortString(), "chain", c.chainAddress)
	c.startNewRound(big.NewInt(0), nil)
	c.logger.Debug("start bft on chain", "success", c.current != nil)
	// 刷新一次链状态
	if c.current != nil {
		c.refreshChainStatus()
	} else {
		// 初始化失败，立即释放
		c.disabledChain()
	}
}
func (c *core) Stop() {
	c.logger.Debug("stop chain engine", "state", c.current.String(c.state, c.waitingForRoundChange))
	c.stopTimer()
	c.chainAddress = common.Address{}
	// 如果单元没有出，回滚单元内的交易
	if c.current != nil && c.current.pendingRequest != nil {
		u := c.current.pendingRequest.Proposal.Unit.(*types.Unit)
		c.backend.Rollback(u)
	}
}
func (c *core) Key() objectpool.ObjectKey {
	return c.chainAddress
}
func (c *core) Reset(key objectpool.ObjectKey, args ...interface{}) {
}

// 是否活动状态
func (c *core) IsActive() bool {
	c.activateLock.Lock()
	defer c.activateLock.Unlock()
	return c.activated
}

// 插入优先消息
func (c *core) PriorHandle(msg []interface{}) bool {
	select {
	case c.priorChan <- msg:
		return true
	default:
		return false
	}
}

// 事件处理 主入口
func (c *core) Handle(args []interface{}) {
	if c.chainAddress.Empty() {
		// 已被停止，不响应所有消息
		return
	}
	if len(args) == 0 {
		return
	}
	// 是否有优先处理消息
	select {
	case ev := <-c.priorChan:
		c.Handle(ev)
	default:
	}
	code := args[0].(bft.BftEventCode)
	tr := tracetime.New("chain", c.chainAddress, "more", code).SetMin(0)
	if c.current != nil {
		tr.AddArgs("round", c.current.round, "number", c.current.sequence)
	} else {
		c.logger.Error("new round init didn't success,ignore event")
		return
	}
	logger := c.logger.New("number", c.current.sequence, "round", c.current.round, "event", code, "state", c.state, "size2", c.current.Prepares.Size(), "size3", c.current.Commits.Size())
	c.locker.Lock()
	var err error
	defer func() {
		if err != nil {
			if err == errFutureMessage || err == errOldMessage || err == errFutureUnitMessage || err == errInconsistentSubject || err == bft.ErrExistVoteMsg {
				logger.Debug("handle event err", "error", err, "args", fmt.Sprintf("%v", args))
			} else {
				logger.Error("handle event err", "error", err, "args", fmt.Sprintf("%v", args))
			}
			tr.AddArgs("status", err)
		}
		tr.Stop()
		c.locker.Unlock()
	}()
	if code != bft.NewTx {
		logger.Debug("bft handle event", "args", args)
	}
	switch code {
	case bft.NewNumber:
		number, hash := args[1].(uint64), args[2].(common.Hash)
		if c.current != nil && number < c.current.sequence.Uint64() {
			logger.Debug("new unit number less than current sequence")
			return
		}
		tr.Tag()
		tr.AddArgs("uhash", hash)
		err = c.handleNewRound(tr)
		tr.Tag()
		if err == nil {
			// 新单元落地，刷新链状态
			c.refreshChainStatus()
		}
	case bft.NewProposal:
		request := args[1].(*bft.Request)
		tr.AddArgs("uhash", request.Proposal.Hash())
		if err = c.handleRequest(request); err != nil {
			tr.AddArgs("status", err)
		}
	case bft.NewMessage:
		msg := args[1].(*protocol.Message)
		tr.AddArgs("code", msg.Code)
		tr.Tag()
		err = c.handleMsg(msg, tr)
		tr.Tag()
		tr.AddArgs("size2", c.current.Prepares.Size(), "size3", c.current.Commits.Size())
	case bft.NewChangeRound:
		rv := args[1].(*RoundView)
		if rv.Sequence.Cmp(c.current.sequence) < 0 || rv.Round.Cmp(c.current.round) < 0 || c.roundchangeResetTime.After(rv.DeadLine) {
			err = errOldMessage
			return
		}
		c.waitingTimes++
		// 长时间不能转动，检查
		if c.waitingForRoundChange && c.waitingTimes%11 == 0 {
			// 0轮长时间不能转动 提醒其他节点检查链待处理交易
			if c.current.round.Sign() == 0 {
				//长时间没有收到提案，尝试重新广播交易
				if c.state == StateAcceptRequest {
					if n := c.backend.ReBroadTx(c.chainAddress); n <= 0 {
						existTx := c.existTx(c.chainAddress)
						logger.Debug("round 0 rebroad tx failed", "existTx", existTx)
						if !existTx {
							c.disabledChain()
							return
						}
					}
					logger.Debug("round 0 wait timeout ,rebroad tx")
				}
				c.sendCheckPendingTx() //提醒其他节点检查待处理交易
				logger.Debug("round 0 wait timeout,send check pending tx")
			}
		} else if c.waitingForRoundChange && c.waitingTimes%29 == 0 {
			logger.Debug("round catch up timeout,send max round")
			// 处理round卡死现象,各自选择最高round
			if max := c.roundChangeSet.MaxRound(1); max != nil {
				c.sendRoundChange(max)
				return
			}
		} else if c.waitingForRoundChange && c.waitingTimes%47 == 0 {
			//强行刷新见证列表
			logger.Debug("refresh validator list before", "list", c.valSet.String())
			if err = c.refreshValidtors(nil); err == nil {
				c.roundChangeSet = newRoundChangeSet(c.valSet, nil)
			}
			logger.Debug("refresh validator list after", "list", c.valSet.String(), "err", err)
		}
		c.sendNextRoundChange()
	case bft.NewTx:
		// 新交易信号激活
		if !c.IsActive() {
			logger.Debug("bft handle event", "code", "newTx")
			c.refreshChainStatus()
		}
	}
}
func (c *core) SetTimer(timer Timer) {
	c.timer = timer
}

// 获取共识状态（用于debug）
func (c *core) CurrentState() (RunState, bft.ConsensusInfo) {
	var info bft.ConsensusInfo
	info.Wait = c.waitingForRoundChange
	info.ChainAddr = c.chainAddress
	info.Node = c.backend.Address()
	info.State.ConsensusState = fmt.Sprintf("%v", c.state)
	if c.current == nil {
		return RunStateNormal, info
	}
	if c.current.Preprepare != nil {
		info.State.Preprepare = c.current.Preprepare.Proposal.Hash()
	}
	info.State.Prepares = c.current.Prepares.Size()
	info.State.Commits = c.current.Commits.Size()

	if c.current.polcRound != nil {
		info.State.LockedHash = c.current.polcRound.lockedHash
	}

	view := c.currentView()
	if c.current.pendingRequest != nil {
		info.State.Req = c.current.pendingRequest.Proposal.Hash()
	}

	info.View = view

	return RunStateNormal, info
}
func (c *core) RollbackPackedTx(newUnit common.Hash) {
	// 打包后出单元失败，交易回滚
	if c.current != nil && c.current.pendingRequest != nil && newUnit != c.current.pendingRequest.Proposal.Hash() {
		u := c.current.pendingRequest.Proposal.Unit.(*types.Unit)
		c.backend.Rollback(u)
	}
}

// 当前轮的提案
func (c *core) CurrentProposal() *bft.MiningResult {
	if c.current != nil && c.current.Preprepare != nil {
		return c.current.Preprepare.Mining
	}
	return nil
}

// 共识消息处理
func (c *core) handleMsg(msg *protocol.Message, trace ...*tracetime.TraceTime) (err error) {
	var tr *tracetime.TraceTime
	if len(trace) == 0 {
		tr = tracetime.New()
		defer tr.Stop()
	} else {
		tr = trace[0]
	}
	var src bft.Validator
	if msg.IsMe {
		_, src = c.valSet.GetByAddress(c.address)
		if src == nil {
			return bft.ErrUnauthorizedAddress
		}
	} else {
		if src, err = c.verifyMsg(msg); err != nil {
			// 校验消息失败，需要断开连接
			c.backend.Disconnect(msg, err)
			return
		}
	}
	err = c.handleCheckedMsg(msg, src, tr)
	return
}

// 校验之后的共识消息处理
func (c *core) handleCheckedMsg(msg *protocol.Message, src bft.Validator, trace ...*tracetime.TraceTime) error {
	var tr *tracetime.TraceTime
	if len(trace) == 0 {
		tr = tracetime.New()
		defer tr.Stop()
	} else {
		tr = trace[0]
	}
	var err error
	defer func() {
		if err == errFutureMessage || err == errFutureUnitMessage {
			c.storeBacklog(msg)
		}
	}()

	tr.Tag("checked")
	switch msg.Code {
	case protocol.MsgPreprepare:
		err = c.handlePreprepare(msg, src)
	case protocol.MsgPrepare:
		err = c.handlePrepare(msg, src, tr)
	case protocol.MsgCommit:
		err = c.handleCommit(msg, src, tr)
	case protocol.MsgRoundChange:
		err = c.handleRoundChange(msg, src)
	case protocol.MsgCheckPendingTx:
		err = c.handleCheckTx(msg, src)
	default:
		c.logger.Error("Invalid message", "msg", msg)
		err = errInvalidMessage
	}
	return err
}

// 校验消息合法性
func (c *core) verifyMsg(msg *protocol.Message, trace ...*tracetime.TraceTime) (validator bft.Validator, err error) {
	var tr *tracetime.TraceTime
	if len(trace) == 0 {
		tr = tracetime.New()
		defer tr.Stop()
	} else {
		tr = trace[0]
	}
	logger := c.logger
	tr.Tag("verifyMsg")
	if c.current == nil {
		return nil, errWaitNewRound
	}
	if _, err := c.validateFn(msg); err != nil {
		logger.Error("Failed to decode message from signBytes", "from", msg.Address, "err", err)
		return nil, err
	}
	tr.Tag()
	// Only accept message if the address is valid
	_, src := c.valSet.GetByAddress(msg.Address)
	if src == nil {
		logger.Error("Invalid address in message")
		return nil, bft.ErrUnauthorizedAddress
	}
	return src, nil
}

// 设置超时
// t 单位为秒
func (c *core) setTimeout(t uint64) {
	rv := &RoundView{
		Chain:    c.chainAddress,
		Round:    c.current.round,
		Sequence: c.current.sequence,
		DeadLine: time.Now().Add(time.Duration(t) * time.Second),
		SendTime: time.Now(),
	}
	c.timer.SetTimeout(rv)
}

// 设置超时截止时间
func (c *core) setTimeoutDeadline(deadline time.Time) {
	rv := &RoundView{
		Chain:    c.chainAddress,
		Round:    c.current.round,
		Sequence: c.current.sequence,
		DeadLine: deadline,
		SendTime: time.Now(),
	}
	c.timer.SetTimeout(rv)
}

func (c *core) refreshChainStatus() {
	c.activated = false
	if c.existTx(c.chainAddress) {
		c.logger.Debug("exist pending tx")
		c.activateChain()
	} else {
		c.logger.Debug("no pending tx")
		c.disabledChain()
	}
}

// 激活链
func (c *core) activateChain() {
	c.activateLock.Lock()
	if c.activated {
		c.activateLock.Unlock()
		return
	}
	c.activated = true
	c.setChainStatus(c.chainAddress, true)
	c.activateLock.Unlock()

	parent, _ := c.getTailHeader()
	//需要计算单元间隔 *链的第一个高度特殊计算
	workWait := c.backend.WaitGenerateUnitTime(parent.Number, parent.Timestamp)
	timeout := time.Now().Add(workWait).Add(time.Second * time.Duration(c.config.RequestTimeout))
	c.setTimeoutDeadline(timeout)

	// 如果是将军，需要立即打包
	isPro := c.isProposer()
	c.logger.Debug("activate round timer", "number", c.current.sequence, "proposer", isPro, "timestamp", time.Unix(0, int64(parent.Timestamp)), "delay", timeout.Sub(time.Now()))

	if isPro {
		go func() {
			if err := c.generateProposal(); err != nil {
				c.logger.Debug("proposer pack pending tx failed", "number", c.current.sequence, "err", err)
				c.disabledChain()
			}
		}()
	}
}

// 释放链
func (c *core) disabledChain() {
	c.activateLock.Lock()
	defer c.activateLock.Unlock()
	c.activated = false
	c.stopTimer()
	c.setChainStatus(c.chainAddress, false)
}

//// 调试跟踪msg流向日志
func (c *core) addDebugLog(args ...interface{}) {
	return
	c.addDebugLog(args...)
}
func (c *core) writeDebugLog() {
	return
	c.logger.Info("bft debug", c.debugLogArgs...)
	c.debugLogArgs = make([]interface{}, 0)
}
