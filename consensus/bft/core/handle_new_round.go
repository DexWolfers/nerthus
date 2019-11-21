package core

import (
	"gitee.com/nerthus/nerthus/common/tracetime"
	"math/big"
)

func (c *core) handleNewRound(tr *tracetime.TraceTime) error {
	logger := c.logger.New("state", c.current.String(c.state, c.waitingForRoundChange))
	logger.Debug("before new round")
	defer func() {
		logger.Debug("after new round")
	}()
	c.startNewRound(big.NewInt(0), nil, tr)
	return nil
}
