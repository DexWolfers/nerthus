// Author: @kulics
package types

import (
	"encoding/binary"
	"errors"
	"sync/atomic"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/rlp"
)

var (
	ErrInvalidVoteMaster = errors.New("the vote master is not witness")
	ErrInvalidVoteSender = errors.New("the vote sender is not witness")
	ErrNonCurUserwitness = errors.New("non-current user witness")
	ErrNotFoundHeader    = errors.New("not found header")
	// errInvalidCommitSeal is returned when the commit seal signature is nil or inavlid
	ErrInvalidCommitSeal = errors.New("invalid commit message sign")
	// errDifferentCommitSeal is returned when the commit seal singer is not equal message sender
	ErrDifferentCommitSeal = errors.New("commit seal signature are different")
	// errDifferentRoundCommitSeal is returned when message's round and commit seal's round are different
	ErrDifferentRoundCommitSeal = errors.New("message's round and commit seal's round are different")
)

const (
	ExtraSealSize = 9
	msgCommit     = uint8(3) // 有回环调用
)

// NewVoteMsg 将单元信息和交易执行结果组合成投票消息
// 注意：还尚未进行签名！
// Author: @ysqi
func NewVoteMsg(h common.Hash, extra []byte) *VoteMsg {
	msg := VoteMsg{
		UnitHash: h,
		Extra:    extra,
	}
	return &msg
}

// VoteSign 投票签名
type VoteSign = SignContent

// GetUnitVoter 通过单元见证人投票签名获取投票者列表。 注意：其中已包括单元发布者（议长）
// 如果签名错误，则返回错误信息。
// Author: @ysqi
func GetUnitVoter(unit *Unit) (common.AddressList, error) {
	// 根据单元的签名中的ChainID进行投票者签名解析
	signer := deriveSigner(unit.Sign.V())
	voters := make([]common.Address, 0, len(unit.witenssVotes))
	flag := make(map[common.Address]struct{}, len(unit.witenssVotes))
	round := uint64(0)
	if unit.header == nil {
		return nil, errors.New("missing header")
	}
	UnitHash := unit.header.HashWithoutVote()
	for _, vote := range unit.witenssVotes {
		if len(vote.Extra) != ExtraSealSize || vote.Extra[0] != msgCommit {
			return nil, ErrInvalidCommitSeal
		}
		extraRound := binary.LittleEndian.Uint64(vote.Extra[1:ExtraSealSize])
		if round != 0 && extraRound != round {
			return nil, ErrDifferentRoundCommitSeal
		}

		msg := VoteMsg{
			UnitHash: UnitHash,
			Extra:    vote.Extra,
			Sign:     vote.Sign,
		}
		addr, err := Sender(signer, &msg)
		if err != nil {
			return voters, err
		}
		if _, ok := flag[addr]; ok {
			continue
		}
		flag[addr] = struct{}{}
		// 因为投票信息如果是从DB中提取的则已经包含议长，因此需要去重
		voters = append(voters, addr)
	}
	return voters, nil
}

//go:generate gencodec -type WitenssVote  -out gen_witenssvote_json.go

// WitenssVote 见证人投票记录，将作为Unit的一部分存储和输出
type WitenssVote struct {
	Extra []byte      `json:"extra" gencodec:"required"`
	Sign  SignContent `json:"sign"             gencodec:"required"`
}

func (wv *WitenssVote) Sender(digest common.Hash) (common.Address, error) {
	voteMsg := VoteMsg{
		Extra:    wv.Extra,
		UnitHash: digest,
		Sign:     wv.Sign,
	}
	return Sender(deriveSigner(wv.Sign.V()), &voteMsg)
}
func (wv *WitenssVote) ToVote(uhash common.Hash) VoteMsg {
	return VoteMsg{
		Extra:    wv.Extra,
		UnitHash: uhash,
		Sign:     SignContent{sign: wv.Sign.Copy()},
	}
}

//go:generate gencodec -type VoteMsg   -out vote_json.go
//VoteMsg 见证人投票信息，用于网络广播见证人投票信息
type VoteMsg struct {
	Extra    []byte      `json:"extra"`                           // 纳秒级
	UnitHash common.Hash `json:"unit_hash"   gencodec:"required"` //签名内容Hash 即为header 不包含投票部分的hash
	Sign     SignContent `json:"sign"        gencodec:"required"`

	hash   atomic.Value   //缓存hash
	sender common.Address //缓存签名者
}

func (self *VoteMsg) Hash() common.Hash {
	if hash := self.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	v := common.SafeHash256(self.GetSignStruct())
	self.hash.Store(v)
	return v
}
func (v *VoteMsg) ToWitnessVote() WitenssVote {
	return WitenssVote{
		Extra: v.Extra,
		Sign:  SignContent{sign: v.Sign.Copy()},
	}
}

// Sender 提取地址
func (self *VoteMsg) Sender() (common.Address, error) {
	return Sender(deriveSigner(self.Sign.V()), self)
}

// GetSignStruct 实现签名接口
func (self *VoteMsg) GetSignStruct() []interface{} {
	if len(self.Extra) == 0 {
		return []interface{}{self.UnitHash}
	}
	return []interface{}{self.UnitHash, self.Extra}
}

// 实现签名接口
func (self *VoteMsg) GetRSVBySignatureHash() []byte {
	return self.Sign.Get()
}

// 实现签名接口
func (self *VoteMsg) SetSign(sig []byte) {
	self.Sign.Set(sig)
}

type Votes []*VoteMsg

func (v Votes) Len() int {
	return len(v)
}
func (v Votes) GetRlp(i int) []byte {
	enc, _ := rlp.EncodeToBytes(v[i])
	return enc
}
func (v Votes) Hash() common.Hash {
	items := make([]interface{}, len(v))
	for i, vote := range v {
		items[i] = vote.Hash()
	}
	return common.SafeHash256(items)
}

func (v Votes) Contains(addr common.Address) (bool, error) {
	for _, wl := range v {
		w, err := wl.Sender()
		if err != nil {
			return false, err
		}
		if w == addr {
			return true, nil
		}
	}
	return false, nil
}
