package wconn

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/nts/protocol"
)

const MainMessageCode = protocol.WitnessStatusMsg

//go:generate enumer -type=SubCode -json -transform=snake  -trimprefix=Code

type SubCode uint8

const (
	CodeShake            SubCode = iota + 11 //见证节点间互连的握手信息
	CodeStatus                               //节点启动所广播自己的节点连接信息
	CodeFetchWitnessConn                     //请求拉取指定见证人的连接信息，因为每个节点依旧存储
)

type Message struct {
	Code SubCode
	Data []byte
}

func (m *Message) GetSignStruct() []interface{} {
	return []interface{}{m.Code, m.Data}
}

type ShakeMsg struct {
	WitnessGroup uint64         //所在见证组
	Connected    uint64         //已连接的见证人
	Mine         *WitnessNode   //本人见证人地址
	KownNodes    []*EncryptData //已知的同组见证人节点信息
}

type EncryptData struct {
	Receiver common.Address
	Data     []byte // data = NodeInfo
}

type WitnessConnectMsg struct {
	WitnessGroup uint64
	Version      uint64         //消息版本
	Data         []*EncryptData //用于解码节点信息的加密的 key。分别为每个见证人加密一份
	Sign         *types.SignContent
}

func (node *WitnessConnectMsg) GetRSVBySignatureHash() []byte {
	if node.Sign == nil {
		node.Sign = new(types.SignContent)
	}
	return node.Sign.GetRSVBySignatureHash()
}

func (node *WitnessConnectMsg) SetSign(s []byte) {
	if node.Sign == nil {
		node.Sign = new(types.SignContent)
	}
	node.Sign.SetSign(s)
}

func (node *WitnessConnectMsg) GetSignStruct() []interface{} {
	return []interface{}{
		node.Version,
		node.WitnessGroup,
		node.Data,
	}
}

type FetchWitnessConnInfoMsg struct {
	Witness common.Address
}
