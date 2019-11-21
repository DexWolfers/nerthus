package protocol

import (
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/crypto"
)

// bft consensus
type BFTMsg struct {
	ChainAddress common.Address
	//Type types.SyncMode
	Payload interface{}

	Create time.Time
}

func (bftMsg *BFTMsg) Hash() common.Hash {
	if buf, ok := bftMsg.Payload.([]byte); ok {
		return crypto.Keccak256Hash(buf)
	} else {
		return common.SafeHash256(bftMsg.Payload)
	}
}
