package message

import (
	"fmt"
	"sync/atomic"
	"time"

	"gitee.com/nerthus/nerthus/nts/protocol"

	"gitee.com/nerthus/nerthus/p2p/discover"
	"gitee.com/nerthus/nerthus/rlp"
)

type Decoder interface {
	Decode(val interface{}) error
}

const MainCode = protocol.RealTimeSyncMsg
const DefMaxUnitSize = 900 * 1024 //最大0.9M,一次性能发送多少数据

type (
	//节点 ID 别名
	PeerID = discover.NodeID
	// 同步消息子 Code
	Code uint8
	// 同步消息
	Message struct {
		Seq      uint64
		ResSeq   uint64 //应答时所对应的请求消息
		Code     Code
		Playload []byte

		Src PeerID `rlp:"-"`
	}
)

const (
	_                      Code = iota
	CodeFetchUnitsResponse      //请求单元响应消息
	CodeFetchUnitByID           //根据单元 ID 拉取单元
	CodeRealTimeMsg             // 实时同步消息
	CodeFetchUnitByHash         //根据单元哈希拉取单元
)

var globalSeq = uint64(time.Now().Unix())

func seq() uint64 {
	return atomic.AddUint64(&globalSeq, 1)
}

//创建一个同步消息
// resSeq：用于在应答时附带具体是对哪个消息的应答。如果没有则写空
// code: 消息 Code
// data: 具体消息，消息必须能被 RLP
func NewMsg(resSeq uint64, code Code, data interface{}) *Message {
	var b []byte
	if data != nil {
		var err error
		b, err = rlp.EncodeToBytes(&data)
		if err != nil {
			panic(fmt.Sprintf("failed to rlp encode,%v", err))
		}
	}
	return &Message{
		Seq:      seq(),
		ResSeq:   resSeq,
		Code:     code,
		Playload: b,
	}
}

func DecodeMsg(id PeerID, decoder Decoder) (*Message, error) {
	msg := new(Message)
	err := decoder.Decode(msg)
	if err != nil {
		return nil, err
	}
	msg.Src = id
	return msg, nil
}
