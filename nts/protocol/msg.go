package protocol

import (
	"bytes"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/crypto/sha3"
	"gitee.com/nerthus/nerthus/p2p"
)

type TTLMsg struct {
	p2p.Msg
	PayloadHash common.Hash
}

const ttlSeconds = 50

func NewTTLMsg(code uint8, data []byte) (*TTLMsg, error) {
	hash, err := messagePayloadHash(data)
	if err != nil {
		return nil, err
	}
	msg := TTLMsg{
		Msg: p2p.Msg{
			Code:    uint64(code),
			Size:    uint32(len(data)),
			Expiry:  uint32(time.Now().Add(time.Second * time.Duration(ttlSeconds)).Unix()),
			TTL:     ttlSeconds,
			Payload: bytes.NewReader(data)},
		PayloadHash: hash,
	}
	return &msg, nil
}

func messagePayloadHash(payload []byte) (h common.Hash, err error) {
	hw := sha3.NewKeccak256()
	_, err = hw.Write(payload)
	if err != nil {
		return h, err
	}
	hw.Sum(h[:0])
	sha3.PutKeccak256(hw)
	return h, nil
}
