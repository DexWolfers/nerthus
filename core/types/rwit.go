package types

import (
	"time"

	"gitee.com/nerthus/nerthus/common"
)

// 为链的最后一批见证人选择最后合法位置的投票内容
type ChooseVote struct {
	Chain       common.Address
	ApplyNumber uint64      // 申请更换见证人的系统链高度
	ApplyUHash  common.Hash //对应单元信息
	Choose      UnitID      //见证人的选择

	Sign SignContent //签名

	CreateAt time.Time `rlp:"-"` //创建时间
}

func (c *ChooseVote) GetSignStruct() []interface{} {
	return []interface{}{
		c.Chain, c.ApplyNumber, c.ApplyUHash,
		c.Choose,
	}
}

func (c *ChooseVote) GetRSVBySignatureHash() []byte {
	return c.Sign.GetRSVBySignatureHash()
}

func (c *ChooseVote) SetSign(sig []byte) {
	c.Sign.SetSign(sig)
}

func (c *ChooseVote) Hash(signer Signer) common.Hash {
	return signer.Hash(c)
}
