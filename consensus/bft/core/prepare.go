package core

import (
	"gitee.com/nerthus/nerthus/common/tracetime"

	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/consensus/protocol"
)

// 广播prepare选票
func (c *core) sendPrepare() {
	c.debugLogArgs = append(c.debugLogArgs, "do", "sendPrepare")
	logger := c.logger.New("state", c.state)
	defer logger.Debug("core.send a Prepare message", "dist", c.current.Subject().Digest)

	sub := c.current.Subject()
	if sub == nil {
		logger.Error("subject packet fail")
		return
	}

	encodedSubject, err := Encode(sub)
	if err != nil {
		logger.Error("Failed to encode", "subject", sub)
		return
	}
	c.broadcast(&protocol.Message{
		Code: protocol.MsgPrepare,
		Msg:  encodedSubject,
	})
}

// 处理prepare选票
func (c *core) handlePrepare(msg *protocol.Message, src bft.Validator, trace ...*tracetime.TraceTime) error {
	var tr *tracetime.TraceTime
	if len(trace) == 0 {
		tr = tracetime.New()
		defer tr.Stop()
	} else {
		tr = trace[0]
	}
	tr.Tag("prepare")
	// Decode PREPARE message
	var prepare *bft.Subject
	err := msg.Decode(&prepare)
	if err != nil {
		return errFailedDecodePrepare
	}
	tr.Tag()
	if err := c.checkMessage(protocol.MsgPrepare, prepare.View); err != nil {
		return err
	}
	tr.Tag()
	// If it is locked, it can only process on the locked block.
	// Passing verifyPrepare and checkMessage implies it is processing on the locked block since it was verified in the Preprepared state.
	if err := c.verifyPrepare(prepare, src); err != nil {
		return err
	}
	tr.Tag()
	if err = c.acceptPrepare(msg, src); err != nil {
		return err
	}
	tr.Tag()
	// Change to Prepared state if we've received enough PREPARE messages or it is locked
	// and we are in earlier state before Prepared state.
	if ((c.current.IsHashLocked() && prepare.Digest == c.current.GetPolcRound().LockHash()) || c.current.GetPrepareOrCommitSize() > c.valSet.TwoThirdsMajority()) &&
		c.state.Cmp(StatePrepared) < 0 {
		c.logger.Debug("majority prepare votes,try lock", "hash", prepare.Digest, "cState", c.current.String(c.state, c.waitingForRoundChange))
		c.current.LockHash(c.currentView(), c.current.GetPrepareOrCommitBytes()) //可能出现节点locked状态不一致
		c.setState(StatePrepared)
		tr.AddArgs("uhash", c.current.Proposal().Hash())
		tr.Tag()
		c.sendCommit()
	}

	return nil
}

// 校验prepare选票
func (c *core) verifyPrepare(prepare *bft.Subject, src bft.Validator) error {

	sub := c.current.Subject()
	//if !reflect.DeepEqual(prepare, sub) {
	if !prepare.Cmp(sub) {
		c.logger.Trace("Inconsistent subjects between PREPARE and proposal", "expected", sub, "got", prepare)
		return errInconsistentSubject
	}

	return nil
}

// 接收prepare选票
func (c *core) acceptPrepare(msg *protocol.Message, src bft.Validator) error {
	logger := c.logger.New("from", src, "state", c.state)
	defer func() {
		logger.Debug("receive a prepare vote", "size", c.current.Prepares.Size())
	}()
	// Add the PREPARE message to current round state
	if err := c.current.Prepares.Add(msg); err != nil {
		logger.Error("Failed to add PREPARE message to round state", "msg", msg, "err", err)
		return err
	}

	return nil
}
