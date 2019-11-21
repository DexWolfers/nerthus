package core

import (
	"encoding/binary"
	"gitee.com/nerthus/nerthus/common/tracetime"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/consensus/protocol"
	"gitee.com/nerthus/nerthus/core/types"
)

const ExtraSealSize = 9

// 发送commit消息
func (c *core) sendCommit() {
	c.addDebugLog("do", "sendCommit")
	tr := tracetime.New()
	defer tr.Stop()
	sub := c.current.Subject()
	tr.AddArgs("uhash", c.current.Proposal().Hash(), "number", c.current.Proposal().Number())
	tr.Tag()
	c.broadcastCommit(sub, c.current.Proposal().Hash())
}

// 广播commit消息
func (c *core) broadcastCommit(sub *bft.Subject, seal common.Hash) {
	c.addDebugLog("commitHash", seal)
	logger := c.logger.New("state", c.state)
	logger.Trace("broadcastCommit")
	tr := tracetime.New("uhash", sub.Digest)
	defer tr.Stop()
	encodedSubject, err := Encode(sub)
	if err != nil {
		logger.Error("Failed to encode", "subject", sub)
		return
	}

	var msg = protocol.Message{
		Code: protocol.MsgCommit,
		Msg:  encodedSubject,
	}
	tr.Tag()
	var buffer = make([]byte, ExtraSealSize)
	buffer[0] = byte(msg.Code)
	binary.LittleEndian.PutUint64(buffer[1:ExtraSealSize], sub.View.Round.Uint64())
	tr.Tag()
	vote := types.VoteMsg{Extra: buffer, UnitHash: seal}
	logger.Trace("begin sign vote")
	if _, err := c.backend.Sign(&vote); err != nil {
		logger.Error("Fail to signature when packet commit seal", "err", err)
		return
	}
	tr.Tag()
	msg.CommittedSeal = &types.WitenssVote{Extra: vote.Extra, Sign: vote.Sign}
	logger.Trace("begin broadcast")
	c.broadcast(&msg)
}

// 处理commit类型的消息
func (c *core) handleCommit(msg *protocol.Message, src bft.Validator, trace ...*tracetime.TraceTime) error {
	var tr *tracetime.TraceTime
	if len(trace) == 0 {
		tr = tracetime.New()
		defer tr.Stop()
	} else {
		tr = trace[0]
	}
	tr.Tag("commit")
	// Decode COMMIT message
	var commit *bft.Subject
	err := msg.Decode(&commit)
	if err != nil {
		return errFailedDecodeCommit
	}
	tr.Tag()
	// 检查消息的合法性
	if err := c.checkMessage(protocol.MsgCommit, commit.View); err != nil {
		return err
	}
	tr.Tag()
	sender, err := msg.Sender(c.sign)
	if err != nil {
		return err
	}
	tr.Tag()
	// 检查commit的合法性
	if err := c.verifyCommit(sender, msg.CommittedSeal, commit, src); err != nil {
		return err
	}
	tr.Tag()
	// 接收commit消息
	c.acceptCommit(msg, src)
	tr.Tag()
	// Commit the proposal once we have enough COMMIT messages and we are not in the Committed state.
	//
	// If we already have a proposal, we may have chance to speed up the consensus process
	// by committing the proposal without PREPARE messages.
	if c.current.Commits.Size() > c.valSet.TwoThirdsMajority() && c.state.Cmp(StateCommitted) < 0 {
		c.logger.Debug("commit unit", "signBytes", c.current.Commits.Size(), "cState", c.current.String(c.state, c.waitingForRoundChange))
		// Still need to call LockHash here since state can skip Prepared state and jump directly to the Committed state.
		c.current.LockHash(c.currentView(), c.current.GetPrepareOrCommitBytes())
		tr.Tag()
		c.commit()
	} else if c.state.Cmp(StatePrepared) < 0 && c.current.GetPrepareOrCommitSize() > c.valSet.TwoThirdsMajority() && !c.current.IsHashLocked() {
		c.current.LockHash(c.currentView(), c.current.GetPrepareOrCommitBytes())
		c.setState(StatePrepared)
		c.sendCommit()
		tr.Tag()
	}

	return nil
}

// 校验commit投票
func (c *core) verifyCommit(msgSender common.Address, commitSeal *types.WitenssVote, commit *bft.Subject, src bft.Validator) error {
	logger := c.logger.New("from", src, "state", c.state)
	if commitSeal == nil {
		return types.ErrInvalidCommitSeal
	}
	if len(commitSeal.Extra) != ExtraSealSize || protocol.ConsensusType(commitSeal.Extra[0]) != protocol.MsgCommit {
		return types.ErrInvalidCommitSeal
	}
	round := binary.LittleEndian.Uint64(commitSeal.Extra[1:ExtraSealSize])
	if round != commit.View.Round.Uint64() {
		return types.ErrDifferentRoundCommitSeal
	}
	commitSealSigner, err := commitSeal.Sender(commit.Digest)
	if err != nil {
		return err
	}
	if commitSealSigner != msgSender {
		return types.ErrDifferentCommitSeal
	}

	sub := c.current.Subject()
	if !sub.Cmp(commit) {
		logger.Trace("Inconsistent subjects between commit and proposal", "expected", sub, "got", commit)
		return errInconsistentSubject
	}

	return nil
}

// 接收commit投票
func (c *core) acceptCommit(msg *protocol.Message, src bft.Validator) error {
	logger := c.logger.New("from", src, "state", c.state)

	// Add the COMMIT message to current round state
	if err := c.current.Commits.Add(msg); err != nil {
		logger.Error("Failed to record commit message", "msg", msg, "err", err)
		return err
	}

	return nil
}
