package core

import (
	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/consensus/protocol"
)

// 检查是否存在待处理交易
func (c *core) handleCheckTx(msg *protocol.Message, src bft.Validator) error {
	c.logger.Debug("handle check pending tx msg", "actived", c.IsActive())
	// 只有未激活状态才检查
	if c.IsActive() {
		return nil
	}
	var prepare *bft.Subject
	err := msg.Decode(&prepare)
	if err != nil {
		return errFailedDecodePrepare
	}
	if err := c.checkMessage(protocol.MsgCheckPendingTx, prepare.View); err != nil {
		return err
	}
	// 尝试刷新激活链
	if !c.IsActive() {
		c.refreshChainStatus()
	}
	return nil
}
func (c *core) sendCheckPendingTx() {
	sub := c.current.Subject()
	if sub == nil {
		c.logger.Error("subject packet fail")
		return
	}

	payload, err := Encode(sub)
	if err != nil {
		c.logger.Error("Failed to encode", "subject", sub)
		return
	}
	c.logger.Debug("send check pending tx msg")
	c.broadcast(&protocol.Message{
		Code: protocol.MsgCheckPendingTx,
		Msg:  payload,
	})
}
