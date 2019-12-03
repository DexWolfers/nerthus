// 业务消息协议管理
package protocol

import (
	"errors"
	"time"

	"gitee.com/nerthus/nerthus/p2p"
)

const (
	MaxMsgSize = 10 * 1024 * 1024 // Maximum cap on the size of a protocol message

	// TTL型消息延迟所能容忍的时间范围
	SynchAllowance = 12 * 2 // seconds
)

var ErrorOldMsg = errors.New("very old witness message")

// 检查消息是否合法
func CheckMsg(msg p2p.Msg) (err error) {
	if msg.Size > MaxMsgSize {
		return Error(ErrMsgTooLarge, "%v > %v", msg.Size, MaxMsgSize)
	}
	if msg.TTL == 0 {
		return nil
	}
	// 允许进行广播的消息有：
	switch uint8(msg.Code) {
	case WitnessStatusMsg, CouncilMsg:
	default:
		return Error(ErrInvalidMsgCode, "does not allow send TTL msg (%d)", msg.Code)
	}

	now := uint32(time.Now().Unix())
	if msg.Expiry < now {
		//too old
		if msg.Expiry+SynchAllowance < now {
			return ErrorOldMsg
		} else {
			return nil // drop envelope without error
		}
	}
	// TODO(ysqi): Pow check
	return nil
}
