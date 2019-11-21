package core

import (
	"math/big"

	"gitee.com/nerthus/nerthus/consensus/bft"
)

// 处理自己打包的提案单元
func (c *core) handleRequest(request *bft.Request) error {
	propsoal := request.Proposal.Unit
	logger := c.logger.New("chain", c.chainAddress, "cState", c.current.String(c.state, c.waitingForRoundChange))

	if err := c.checkRequestMsg(propsoal); err != nil {
		if err == errInvalidMessage {
			logger.Warn("Invalid request")
			return err
		}
		logger.Warn("Unexpected request", "err", err, "number", c.current.sequence, "proposal number", propsoal.Number(), "hash", propsoal.Hash())
		return err
	}

	logger.Trace("HandleRequest", "number", propsoal.Number(), "hash", propsoal.Hash())

	// 如果当前状态为StateAcceptRequest
	c.current.pendingRequest = request
	if c.state == StateAcceptRequest {
		c.sendPreprepare(propsoal)
	}
	return nil
}

// check request state
// return errInvalidMessage if the message is invalid
// return errFutureMessage if the sequence of proposal is larger than current sequence
// return errOldMessage if the sequence of proposal is smaller than current sequence
// 检查打包单元是否合法，必须是当前的高度的消息
func (c *core) checkRequestMsg(request bft.Proposal) error {
	if request == nil {
		return errInvalidMessage
	}
	if c := c.current.sequence.Cmp(new(big.Int).SetUint64(request.Number())); c > 0 {
		return errOldMessage
	} else if c < 0 {
		return errFutureMessage
	} else {
		return nil
	}
}
