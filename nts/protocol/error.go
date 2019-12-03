package protocol

import "fmt"

// 错误的消息，可以依据此类消息断开连接
type ErrMsg struct {
	code errCode
	err  string
}

type errCode int

const (
	ErrMsgTooLarge = iota
	ErrDecode
	ErrInvalidMsgCode
	ErrProtocolVersionMismatch
	ErrNetworkIdMismatch
	ErrGenesisBlockMismatch
	ErrNoStatusMsg
	ErrExtraStatusMsg
	ErrSuspendedPeer
	ErrInvalidPayload
	ErrDropMessage
)

func (e errCode) String() string {
	return errorToString[int(e)]
}

// XXX change once legacy code is out
var errorToString = map[int]string{
	ErrMsgTooLarge:             "Message too long",
	ErrDecode:                  "Invalid message",
	ErrInvalidMsgCode:          "Invalid message code",
	ErrProtocolVersionMismatch: "Protocol version mismatch",
	ErrNetworkIdMismatch:       "NetworkId mismatch",
	ErrGenesisBlockMismatch:    "Genesis unit mismatch",
	ErrNoStatusMsg:             "No status message",
	ErrExtraStatusMsg:          "Extra status message",
	ErrSuspendedPeer:           "Suspended peer",
	ErrInvalidPayload:          "Invalid payload message",
	ErrDropMessage:             "Drop a message",
}

func (e *ErrMsg) Error() string {
	return fmt.Sprintf("%s - %v", errorToString[int(e.code)], e.err)
}
func Error(code errCode, format string, v ...interface{}) error {
	return &ErrMsg{
		code: code,
		err:  fmt.Sprintf(format, v...),
	}
}
