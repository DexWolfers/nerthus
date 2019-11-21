package core

import (
	"errors"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/params"
	"math/big"

	"gitee.com/nerthus/nerthus/common/tracetime"
	"gitee.com/nerthus/nerthus/consensus"

	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/consensus/protocol"
	"gitee.com/nerthus/nerthus/core/types"
)

// 广播提案
func (c *core) sendPreprepare(request bft.Proposal) {
	logger := c.logger.New("state", c.state, "isProposer", c.isProposer())
	logger.Debug("send preprepare message")
	tr := tracetime.New("uhash", request.Hash(), "chain", c.chainAddress, "number", c.current.sequence).SetMin(0)
	defer tr.Stop()
	// pendingRequest收到来自内部出单元消息，判断自己是否该轮次的提案者，是则构建一份提案并广播msgPreprepare消息到网络
	// If I'm the proposer and I have the same sequence with the proposal
	if c.current.Sequence().Cmp(new(big.Int).SetUint64(request.Number())) == 0 && c.isProposer() {
		tr.Tag()
		curView := c.currentView()
		preprepare, err := Encode(&bft.Preprepare{
			View:     curView,
			Proposal: request,
		})
		if err != nil {
			logger.Error("Failed to encode", "view", curView)
			return
		}
		tr.AddArgs("status", "ok")
		tr.Tag()
		c.addDebugLog("sendPreprepare", request.Hash())
		c.broadcast(&protocol.Message{
			Code: protocol.MsgPreprepare,
			Msg:  preprepare,
		})
	}
}

// 广播空的提案，即放弃本轮提案机会
func (c *core) sendGiveUpPreprepare() {
	if c.isProposer() && c.current.pendingRequest == nil {
		preprepare, err := Encode(&bft.Preprepare{
			View:     c.currentView(),
			Proposal: nil,
		})
		if err != nil {
			c.logger.Error("Failed to encode", "view", c.currentView())
			return
		}
		c.addDebugLog("sendPreprepare", "nil")
		c.broadcast(&protocol.Message{
			Code: protocol.MsgPreprepare,
			Msg:  preprepare,
		})
		//tracetime.Info("more", "give up propose", "chain", c.chainAddress, "number", c.current.sequence, "round", c.current.round)
		c.logger.Debug("give up propose", "number", c.current.sequence, "round", c.current.round)
	}

}

// 处理提案
func (c *core) handlePreprepare(msg *protocol.Message, src bft.Validator, trace ...*tracetime.TraceTime) error {
	var tr *tracetime.TraceTime
	if len(trace) == 0 {
		tr = tracetime.New("createat", msg.TraceTime, "P2PReceivedTime", msg.P2PTime).SetMin(0)
		defer tr.Stop()
	} else {
		tr = trace[0]
	}
	tr.Tag("preprepare")
	// Decode PRE-PREPARE
	var preprepare *bft.Preprepare
	err := msg.Decode(&preprepare)
	if err != nil {
		return errFailedDecodePreprepare
	}
	// Ensure we have the same view with the PRE-PREPARE message
	// If it is old message, see if we need to broadcast COMMIT
	if err := c.checkMessage(protocol.MsgPreprepare, preprepare.View); err != nil {
		tr.Tag()
		if preprepare.Proposal == nil {
			return err
		}
		return err
	}
	tr.Tag()
	// Check if the message comes from current proposer
	if !c.valSet.IsProposer(src.Address()) {
		c.logger.Warn("Ignore preprepare messages from non-proposer", "from", src.Address(), "proposer", c.valSet.GetProposer(), "vals", c.valSet)
		return errNotFromProposer
	}
	// 本轮将军放弃提案，并且本地有工作要做，round change
	if preprepare.Proposal == nil {
		tr.AddArgs("status", "give up")
		c.sendNextRoundChange()
		return nil
	}
	c.addDebugLog("uhash", preprepare.Proposal.Hash())
	tr.AddArgs("uhash", preprepare.Proposal.Hash(), "from", src.Address(), "number", preprepare.Proposal.Number(), "round", c.current.round, "chain", c.chainAddress)
	tr.Tag()
	proposalHash := preprepare.Proposal.Hash()
	if c.current.pendingRequest == nil || proposalHash != c.current.pendingRequest.Proposal.Unit.Hash() {
		// Verify the proposal we received
		if result, err := c.verifyProposal(preprepare.Proposal); err != nil {
			// if it's a future block, we will handle it again after the duration

			if !consensus.IsBadUnitErr(err) {
				err = errFutureMessage
			} else {
				c.logger.Error("Failed to verify proposal", "err", err)
				c.sendNextRoundChange()
			}
			tr.Tag()
			tr.AddArgs("status", err)
			return err
		} else {
			preprepare.Mining = result
		}
	} else {
		u := c.current.pendingRequest.Proposal
		preprepare.Mining = u
	}
	tr.Tag()
	tr.AddArgs("txs", len(preprepare.Proposal.(*types.Unit).Transactions()))

	tr.Tag()
	// Here is about to accept the PRE-PREPARE
	if c.state == StateAcceptRequest {
		c.noteDiffHash(src.Address(), proposalHash)
		tr.Tag()
		if c.current.IsHashLocked() {
			// Send ROUND CHANGE if the locked proposal and the received proposal are different
			if proposalHash == c.current.GetPolcRound().LockHash() {
				// Broadcast COMMIT and enters Prepared state directly
				tr.Tag()
				c.acceptPreprepare(preprepare)
				c.setState(StatePrepared)
				c.sendCommit()
			} else {
				// Send round change
				tr.Tag()
				c.logger.Debug("Lock different hash,round change", "local-lock", c.current.GetPolcRound().LockHash(), "receive", proposalHash)
				c.sendNextRoundChange()
			}
		} else {
			// Either
			//   1. the locked proposal and the received proposal match
			//   2. we have no locked proposal
			tr.Tag()
			c.acceptPreprepare(preprepare)
			c.setState(StatePreprepared)
			tr.Tag()
			c.sendPrepare()
			tr.AddArgs("status", "ok")
		}
	}

	return nil
}

// 提案
func (c *core) acceptPreprepare(preprepare *bft.Preprepare) {
	c.current.SetPreprepare(preprepare)
	c.stopTimer()                           //清理旧的round change消息
	c.setTimeout(c.config.ConsensusTimeout) // 收到正确的提案，更新轮次timer
}

func (c *core) verifyProposal(proposal bft.Proposal) (*bft.MiningResult, error) {
	if result, err := c.backend.Verify(proposal); err != nil {
		return nil, err
	} else {
		// 校验sc number本节点是否是见证人
		// 见证人必须出现在用户见证人列表中
		unit := proposal.(*types.Unit)
		chainCurrentWitness := c.backend.WitnessLib(unit.MC(), unit.SCHash())
		if len(chainCurrentWitness) == 0 {
			return nil, errFutureMessage
		}
		// 校验引用SC高度，当前节点是否见证人
		isMyChain := false
		for _, wit := range chainCurrentWitness {
			if wit == c.backend.Address() {
				isMyChain = true
				break
			}
		}
		if !isMyChain {
			c.logger.Warn("not group witness", "chainWitness", common.AddressList(chainCurrentWitness).String(), "cur", c.backend.Address(), "uhash", unit.Hash(), "number", unit.Number(), "scnumber", unit.SCNumber())
			return nil, errors.New("I'm not witness at that sc number ")
		}
		return result, nil
	}
}

// lock不同的提案hash时，记录
func (c *core) noteDiffHash(address common.Address, hash common.Hash) {
	if c.lockHashes != nil {
		c.lockHashes[address] = hash
		return
	}
	if c.current.IsHashLocked() {
		if lhash := c.current.GetPolcRound().LockHash(); lhash != hash {
			c.lockHashes = make(map[common.Address]common.Hash)
			// 记录自己
			c.lockHashes[c.backend.Address()] = lhash
			c.lockHashes[address] = hash
		}
	}
}
func (c *core) checkDeadLock() {
	if c.lockHashes == nil {
		return
	}
	voteSum := int64(params.ConfigParamsUCWitness)
	if c.chainAddress == params.SCAccount {
		voteSum = params.ConfigParamsSCWitness
	}
	// 每2个轮回只处理一次
	if c.current.Round().Int64()%(voteSum*2) != 0 {
		return
	}
	if len(c.lockHashes) > c.valSet.TwoThirdsMajority() {
		// 解锁投票最少的hash
		hashSeq := make(map[common.Hash]int)
		for _, v := range c.lockHashes {
			hashSeq[v] = hashSeq[v] + 1
		}
		mineSum := hashSeq[c.current.polcRound.LockHash()]
		for _, v := range hashSeq {
			// 排除只有1票的提案
			if v > 1 && v < mineSum {
				return
			}
		}
		c.logger.Info("bft recover from dead lock", "hash", c.current.polcRound.LockHash(), "votes", mineSum, "total", len(c.lockHashes), "hashes", len(hashSeq))
		// 如果自己锁定在最少投票的hash上，解锁
		c.current.UnlockHash()
		c.lockHashes = nil

	}
}
