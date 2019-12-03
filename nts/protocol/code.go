package protocol

import (
	"fmt"
)

//所支持的协议版本信息
var ProtocolVersions = []Version{
	{
		ID: 23, MaxCode: uint64(CouncilMsg) + 1,
	}}

type Version struct {
	ID      uint
	MaxCode uint64
}

const (
	StatusMsg             = 0x00
	TxMsg           uint8 = 0x01 //交易消息
	NewUnitMsg      uint8 = 0x02 //新单元消息
	NewBroadcastMsg uint8 = 0x03
	RealTimeSyncMsg uint8 = 0x04 //同步消息
	BadWitnessProof uint8 = 0x06 //作恶见证人证据消息

	WitnessStatusMsg  uint8 = 0x11 //见证人状态消息
	BftMsg            uint8 = 0x12 // BFT共识消息
	ChooseLastUnitMsg uint8 = 0x13 // 系统见证人选择
	CouncilMsg        uint8 = 0x14 // 理事仲裁消息
)

var codeStr = [CouncilMsg + 1]string{
	TxMsg:             "tx",
	NewUnitMsg:        "unit",
	NewBroadcastMsg:   "witness",
	RealTimeSyncMsg:   "sync",
	WitnessStatusMsg:  "witness_status",
	BftMsg:            "bft",
	ChooseLastUnitMsg: "witness_replace_vote",
	BadWitnessProof:   "bad_witness_proof",
	CouncilMsg:        "council",
}

func CodeString(code uint8) string {
	str := codeStr[code]
	if str == "" {
		return fmt.Sprintf("unkowncode(%d)", code)
	}
	return str
}
