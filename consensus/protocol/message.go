package protocol

import (
	"errors"
	"fmt"
	"gitee.com/nerthus/nerthus/nts/protocol"
	"sync/atomic"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/rlp"
)

// 共识消息事件
type MessageEvent struct {
	Peer    common.Address
	Chain   common.Address
	Message *Message

	FromMe          bool //消息是否由自身创建
	P2PReceivedTime time.Time
	PushTime        time.Time //p2p接受后发送到队列时间
}

//go:generate enumer -type=ConsensusType -json -transform=snake
// 共识消息类型
type ConsensusType uint8

const (
	_ ConsensusType = iota
	MsgPreprepare
	MsgPrepare
	MsgCommit
	MsgRoundChange
	MsgCheckPendingTx
)

// 共识消息
type Message struct {
	Code          ConsensusType // 业务代号
	Msg           []byte
	Address       common.Address     // Sign 来之于Address的签名
	Extra         []byte             `rlp:"nil"`
	TraceTime     uint64             `rlp:"nil"`
	Sign          *types.SignContent `rlp:"nil"` // message的签名
	CommittedSeal *types.WitenssVote `rlp:"nil"`

	P2PTime    time.Time    `rlp:"-"`
	IsMe       bool         `rlp:"-"`
	mid        string       `rlp:"-"`
	sender     atomic.Value // error or address
	recovering int32
	From       protocol.Peer `rlp:"-"`
}

func (m *Message) ID() string {
	return m.mid
}

// Impl sign interface
func (m *Message) GetSignStruct() []interface{} {
	var packet = []interface{}{
		m.Code,
		m.Msg,
		m.Address,
		m.Extra,
	}
	if m.CommittedSeal == nil {
		packet = append(packet, types.NewSignContent())
	} else {
		packet = append(packet, m.CommittedSeal)
	}

	return packet
}

func (m *Message) GetSignStructBytes() []byte {
	buf, err := rlp.EncodeToBytes(m.GetSignStruct())
	if err != nil {
		PanicSanity(err)
	}
	return buf
}

// convert Message into prove tree bytes
func (m *Message) PacketProveTreeBytes() []byte {
	var packet = []interface{}{
		m.Code,
		m.Msg,
		m.Address,
		m.Extra,
		m.Sign,
		m.CommittedSeal,
	}
	buf, err := rlp.EncodeToBytes(packet)
	if err != nil {
		PanicSanity(err)
	}
	return buf
}

func (m *Message) GetRSVBySignatureHash() []byte {
	return m.Sign.Get()
}

func (m *Message) Hash() common.Hash {
	return common.SafeHash256(m.GetSignStruct())
}

func (m *Message) SetSign(sign []byte) {
	m.Sign.Set(sign)
}

func (m *Message) Copy() *Message {
	msg := Message{
		Code:      m.Code,
		Msg:       m.Msg,
		Address:   m.Address,
		Extra:     m.Extra,
		TraceTime: m.TraceTime,
		IsMe:      m.IsMe,
		Sign:      &types.SignContent{},
	}
	msg.Sign.Set(m.Sign.Get())
	if m.CommittedSeal != nil {
		m.CommittedSeal = &types.WitenssVote{Extra: m.CommittedSeal.Extra}
		m.CommittedSeal.Sign.Set(msg.CommittedSeal.Sign.Get())
	}
	return &msg
}

// ==============================================
//
// define the functions that needs to be provided for core.
func (m *Message) FromPayload(b []byte, validateFn func(m *Message) (common.Address, error)) error {
	// Decode Message
	var (
		sender common.Address
		err    error
	)
	if err = rlp.DecodeBytes(b, m); err != nil {
		return err
	}
	// TODO: mid 无用
	m.mid = "empty"

	// Validate Message (on a Message without Signature)
	if validateFn != nil {
		if sender, err = validateFn(m); err != nil {
			return err
		}
		if sender != m.Address {
			return errors.New("unauthorized address")
		}
	}
	// Still return the Message even the err is not nil
	return err
}

func (m *Message) Sender(sign types.Signer) (common.Address, error) {
	//优先从缓存中获取
	if v := m.sender.Load(); v != nil {
		switch v := v.(type) {
		case error:
			return common.Address{}, v
		case common.Address:
			return v, nil
		}
	}
	if !atomic.CompareAndSwapInt32(&m.recovering, 0, 1) {
		//说明此时其他位置已在进行提取签名者信息
		//等待结果
		<-time.After(2 * time.Second)
		//再次获取
		return m.Sender(sign)
	}
	//独占提取
	sender, err := types.Sender(sign, m)
	if err != nil {
		m.sender.Store(err)
		return common.Address{}, err
	}
	if sender != m.Address {
		err = errors.New("unauthorized address")
		m.sender.Store(err)
		return common.Address{}, err
	}

	m.sender.Store(sender)
	return sender, nil
}

func (m *Message) Payload() ([]byte, error) {
	return rlp.EncodeToBytes(m)
}

func (m *Message) Expired(ttl time.Duration) bool {
	//过期校验
	return time.Unix(0, int64(m.TraceTime)).Add(ttl).Before(time.Now())
}

func (m *Message) PayloadNoSig() ([]byte, error) {
	return rlp.EncodeToBytes(&Message{
		Code:          m.Code,
		Msg:           m.Msg,
		Address:       m.Address,
		Extra:         m.Extra,
		TraceTime:     m.TraceTime,
		IsMe:          m.IsMe,
		Sign:          nil,
		CommittedSeal: m.CommittedSeal,
	})
}

func (m *Message) Decode(val interface{}) error {
	return rlp.DecodeBytes(m.Msg, val)
}

func (m *Message) Trace() string {
	return fmt.Sprintf("{mid:%v, code:%v, from:%v, trace:%v, elasp:%v, isMe:%v, extra_size:%v}", m.ID(), m.Code, m.Address.ShortString(),
		m.TraceTime, time.Now().Sub(time.Unix(0, int64(m.TraceTime))), m.IsMe, len(m.Extra))
}

func (m *Message) String() string {
	return fmt.Sprintf("{mid:%v, code:%v, from:%v, isMe:%v, trace:%v}", m.ID(), m.Code, m.Address.ShortString(), m.IsMe, m.TraceTime)
}

func PanicSanity(v interface{}) {
	panic(fmt.Sprintf("Panicked on a Sanity Check: %v", v))
}
