package core

import (
	"gitee.com/nerthus/nerthus/consensus"
	"time"

	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/consensus/protocol"
)

// checkMessage checks the message state
// return errInvalidMessage if the message is invalid
// return errFutureMessage if the message view is larger than current view
// return errOldMessage if the message view is smaller than current view
func (c *core) checkMessage(msgCode protocol.ConsensusType, view *bft.View) error {
	c.addDebugLog("number", c.current.sequence, "msgView", view.String())
	if view == nil || view.Sequence == nil || view.Round == nil {
		return errInvalidMessage
	}

	// 同一高度，且轮次等于或高于本地轮次
	if msgCode == protocol.MsgRoundChange {
		if view.Sequence.Cmp(c.currentView().Sequence) > 0 {
			return errFutureUnitMessage
		} else if view.Cmp(c.currentView()) < 0 {
			return errOldMessage
		}
		return nil
	}

	//当前高度落后于其他节点 触发拉取动作
	if view.Sequence.Cmp(c.currentView().Sequence) > 0 {
		return errFutureUnitMessage
	}

	if view.Cmp(c.currentView()) > 0 {
		return errFutureMessage
	}
	if view.Cmp(c.currentView()) < 0 {
		return errOldMessage
	}

	// StateAcceptRequest only accepts MsgPreprepare
	// other messages are future messages
	if c.state == StateAcceptRequest {
		if msgCode == protocol.MsgPrepare || msgCode == protocol.MsgCommit {
			return errFutureMessage
		}
		return nil
	}

	if c.state == StateCommitted {
		return errOldMessage
	}

	// For states(StatePreprepared, StatePrepared, StateCommitted),
	// can accept all message types if processing with same view
	return nil
}

// 消息暂存 一般是future消息
func (c *core) storeBacklog(msg *protocol.Message) {
	c.backlogs = append(c.backlogs, msg)
}

// 取暂存消息，只取属于当前轮的消息
func (c *core) processBacklog() {
	if len(c.backlogs) <= 0 {
		return
	}
	var rest []*protocol.Message
	for _, msg := range c.backlogs {
		var view *bft.View
		switch msg.Code {
		case protocol.MsgPreprepare:
			var m *bft.Preprepare
			err := msg.Decode(&m)
			if err == nil {
				view = m.View
			}
			// for protocol.MsgRoundCh, MsgPrepare and protocol.MsgCommit cases
		default:
			var sub *bft.Subject
			err := msg.Decode(&sub)
			if err == nil {
				view = sub.View
			}
		}
		if view == nil {
			c.logger.Debug("invalid backlog message", "msg", msg)
			continue
		}
		if err := c.checkMessage(msg.Code, view); err == nil {
			ev := protocol.MessageEvent{
				Peer:     c.address,
				Chain:    c.chainAddress,
				Message:  msg,
				FromMe:   true,
				PushTime: time.Now(),
			}
			c.backend.(consensus.Miner).OnMessage(ev)
		} else if err == errFutureUnitMessage || err == errFutureMessage {
			rest = append(rest, msg)
		}
	}
	c.backlogs = rest
}
