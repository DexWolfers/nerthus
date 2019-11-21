package core

// 共识达成清理动作
func (c *core) handleFinalCommitted() error {
	logger := c.logger.New("state", c.state)
	defer func() {
		logger.Debug("after handleFinalCommitted", "cState", c.current.String(c.state, c.waitingForRoundChange))
	}()
	c.stopTimer()
	c.updateWaitForRoundChange(false)
	return nil
}
