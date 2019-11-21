// Author: @kulics
package types

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"sync/atomic"
	"time"
	"unsafe"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/hexutil"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/rlp"
)

var (
	ErrInvalidSig = errors.New("invalid sign v, r, s values")
)

type TxAction uint8

//go:generate enumer -type TxAction -json -transform=snake -trimprefix=Action
// 交易类型
const (
	ActionTransferPayment    TxAction = iota // 转账付款
	ActionTransferReceipt                    // 转账收款
	ActionContractFreeze                     // 部署/执行合约前冻结（简称冻结）
	ActionContractDeal                       // 执行合约(注意系统合约)
	ActionPowTxContractDeal                  // 执行proof work transaction contract, 会跳过第一个单元
	ActionSCIdleContractDeal                 // 系统链空转合约执行, 跳过第一个单元以及地
	ActionContractRefund                     // 部署/执行合约后返还多余冻结部分（简称返还）
	ActionContractReceipt                    // 执行合约后合约中有转账给多个其他账户时的接收者收款
)

var (
	EmptyRootHash   = common.Hash{}
	EmptyParentHash = common.SafeHash256(common.Hash{})
)

// UnitID 单元唯一性ID
type UnitID struct {
	ChainID common.Address `json:"chain_id"` //链地址ID
	Height  uint64         `json:"height"`   //单元高度
	Hash    common.Hash    `json:"hash"`     //单元哈希
}

func (id UnitID) IsEmpty() bool {
	return id.Height == 0 || id.ChainID.Empty() || id.Hash.Empty()
}
func (id UnitID) Equal(b UnitID) bool {
	if id.Height != b.Height {
		return false
	}
	if !id.ChainID.Equal(b.ChainID) {
		return false
	}
	if id.Hash != id.Hash {
		return false
	}
	return true
}

func (id UnitID) String() string {
	return fmt.Sprintf("{chain:%x,height:%d,hash:%x}", id.ChainID, id.Height, id.Hash)
}

//go:generate gencodec -type Header -field-override headerMarshaling -out gen_header_json.go

// Header represents a unit header in the Nerthus unit chain.
// Author: @ysqi
type Header struct {
	MC          common.Address `json:"mc" gencodec:"required"`                //单元所在链地址
	Number      uint64         `json:"number" gencodec:"required"`            //单元所在链中的高度序号
	ParentHash  common.Hash    `json:"parent_hash" gencodec:"required"`       //单元父单元
	SCHash      common.Hash    `json:"sc_hash" gencodec:"required"`           //最新稳定的系统链单元
	SCNumber    uint64         `json:"sc_number" gencodec:"required"`         // 最新稳定的系统链高度
	Proposer    common.Address `json:"proposer" gencodec:"required"`          // 提案者地址
	TxsRoot     common.Hash    `json:"transactions_root" gencodec:"required"` //单元中交易树根值
	ReceiptRoot common.Hash    `json:"receipt_root" gencodec:"required"`      //本单元中所有交易收据树根值
	StateRoot   common.Hash    `json:"state_root" gencodec:"required"`        //状态树根，方便立即从数据库使用该单元状态快照
	Bloom       Bloom          `json:"bloom"`                                 //由日志信息组成的一个Bloom过滤器
	GasLimit    uint64         `json:"gas_limit" gencodec:"required"`
	GasUsed     uint64         `json:"gas_used" gencodec:"required"`
	Timestamp   uint64         `json:"timestamp" gencodec:"required"`
}

// 不优雅的实现，此目的是为了即使不提供整个单元，也可以获取通过头获取签名内容部分
func (h *Header) GetSignStruct() []interface{} {
	return []interface{}{h.Hash()}
}
func (h *Header) GetRSVBySignatureHash() []byte {
	panic("not yet implement")
}
func (h *Header) SetSign([]byte) {
	panic("not yet implement")
}

// field type overrides for gencodec
type headerMarshaling struct {
	Number    hexutil.Uint64
	Timestamp hexutil.Uint64
	GasLimit  hexutil.Uint64
	GasUsed   hexutil.Uint64
	SCNumber  hexutil.Uint64

	Hash common.Hash `json:"hash"` // adds call to Hash() in MarshalJSON
}

func (h *Header) ID() UnitID {
	return UnitID{
		ChainID: h.MC,
		Height:  h.Number,
		Hash:    h.Hash(),
	}
}

func (h *Header) String() string {
	return fmt.Sprintf(`Header(%x):
[
    MC          :%x
    Number      :%d
    ParentHash  :%x
    SCHash      :%x 
    TxsRoot     :%x 
    ReceiptRoot:%x
    StateRoot   :%x
    Bloom       :%x
    GasLimit    :%v		
    GasUsed     :%v
    Timestamp   :%v
]`,
		h.Hash(), h.MC, h.Number, h.ParentHash, h.SCHash, h.TxsRoot,
		h.ReceiptRoot, h.StateRoot, h.Bloom,
		h.GasLimit, h.GasUsed, h.Timestamp,
	)
}

// Hash returns the block hash of the header, which is simply the keccak256 hash of its
// RLP encoding.
func (h *Header) Hash() common.Hash {
	return common.SafeHash256(h)
}

// Size returns the approximate memory used by all internal contents. It is used
// to approximate and limit the memory consumption of various caches.
func (h *Header) Size() common.StorageSize {
	return common.StorageSize(unsafe.Sizeof(*h))
}

// Parents 获取不为空的父单元Hash
// Author: @ysqi
func (h *Header) Parents() []common.Hash {
	v := []common.Hash{}
	if !h.ParentHash.Empty() && h.ParentHash != EmptyParentHash {
		v = append(v, h.ParentHash)
	}

	if !h.SCHash.Empty() && h.SCHash != EmptyParentHash {
		v = append(v, h.SCHash)
	}
	return v
}

func (h *Header) HashWithoutVote() common.Hash {
	return common.SafeHash256([]interface{}{
		h.MC,
		h.Number,
		h.ParentHash,
		h.SCHash,
		h.SCNumber,
		h.Proposer,
		h.TxsRoot,
		h.ReceiptRoot,
		h.StateRoot,
		h.Bloom,
		h.GasLimit,
		h.GasUsed,
		h.Timestamp,
	})
}

func (h *Header) HashWithoutSeal() common.Hash {
	return common.SafeHash256([]interface{}{
		h.MC,
		h.Number,
		h.ParentHash,
		h.SCHash,
		h.TxsRoot,
		h.ReceiptRoot,
		h.StateRoot,
		h.Bloom,
		h.GasLimit,
		h.GasUsed,
		h.Timestamp,
	})
}

// Author: @yin
func (h *Header) IsSystemMC() bool {
	return h.MC == params.SCAccount
}

// NewUnit 创建一个全新的单元，输入信息均被复制，防止值修改影响。
//
// 同时将 交易、凭据、投票数据集分别进行trie处理存储Root到单元头中.
func NewUnit(header *Header, txs TxExecs) *Unit {
	b := &Unit{header: header}

	if len(txs) == 0 {
		b.header.TxsRoot = EmptyRootHash
	} else {
		if b.header.GasUsed == 0 {
			//TODO: 不应该出现 GasUsed 为0情况
			//panic("gas used is zero")
		}

		b.header.TxsRoot = DeriveSha(txs)
		b.transactions = txs
	}
	return b
}

// NewBlockWithHeader 利用给定的header创建新单元，头部内容已复制，外部修改不会相互影响.
func NewBlockWithHeader(header *Header) *Unit {
	return &Unit{header: CopyHeader(header)}
}
func NewUnitWithBody(header *Header, body *Body) *Unit {
	u := Unit{
		header:       header,
		transactions: body.Txs,
		witenssVotes: body.Votes,
		Sign:         body.Sign,
	}
	return &u
}

// CopyHeader 深度复制单元头信息以防止修改引起副作用
func CopyHeader(h *Header) *Header {
	cpy := *h
	return &cpy
}

// Body is a simple (mutable, non-safe) data container for storing and moving
// a unit's data contents (transactions and uncles) together.
type Body struct {
	Txs   TxExecs       `json:"transactions"`
	Sign  SignContent   `json:"sign"`
	Votes []WitenssVote `json:"voting"`
}

// Unit represents an entire unit in the   blockchain.
// Author: @ysqi
type Unit struct {
	header       *Header
	transactions TxExecs
	witenssVotes []WitenssVote

	// caches
	hash       atomic.Value
	voterCache atomic.Value
	size       atomic.Value
	sender     common.Address

	// These fields are used by package eth to track
	// inter-peer unit relay.
	ReceivedAt   time.Time   `json:"-"`
	ReceivedFrom interface{} `json:"-"`

	Sign SignContent `json:"sign"` //签名

}

func (b *Unit) ID() UnitID {
	return UnitID{
		ChainID: b.MC(),
		Height:  b.Number(),
		Hash:    b.Hash(),
	}
}

func (b *Unit) Transactions() TxExecs       { return b.transactions }
func (b *Unit) TxCount() int                { return b.transactions.Len() }
func (b *Unit) MC() common.Address          { return b.header.MC }
func (b *Unit) Number() uint64              { return b.header.Number }
func (b *Unit) WitnessVotes() []WitenssVote { return b.witenssVotes }
func (b *Unit) ParentHash() common.Hash     { return b.header.ParentHash }
func (b *Unit) SCHash() common.Hash         { return b.header.SCHash }
func (b *Unit) SCNumber() uint64            { return b.header.SCNumber }
func (b *Unit) Proposer() common.Address    { return b.header.Proposer }

//判断该单元是否是黑球
func (b *Unit) IsBlackUnit() bool { return b.header.Proposer.Equal(params.PublicPrivateKeyAddr) }

func (b *Unit) Timestamp() uint64    { return b.header.Timestamp }
func (b *Unit) TxsRoot() common.Hash { return b.header.TxsRoot }

func (b *Unit) Bloom() Bloom {
	return b.header.Bloom
}
func (b *Unit) Root() common.Hash {
	return b.header.StateRoot
}

func (b *Unit) ReceiptRoot() common.Hash {
	return b.header.ReceiptRoot
}
func (b *Unit) GasLimit() uint64 {
	return b.header.GasLimit
}
func (b *Unit) GasUsed() uint64 {
	return b.header.GasUsed
}

// Witnesses 获取单元的见证人列表，其中已包括单元发布者（议长）
func (b *Unit) Witnesses() ([]common.Address, error) {
	if cache := b.voterCache.Load(); cache != nil {
		switch v := cache.(type) {
		case common.AddressList:
			return v, nil
		case error:
			return nil, v
		default:
			panic(fmt.Sprintf("unknown type convert %t", cache))
		}
	}

	voters, err := GetUnitVoter(b)
	if err != nil {
		b.voterCache.Store(err)
		return nil, err
	}
	b.voterCache.Store(voters)
	return voters, err
}

// Header 返回单元头部分
func (b *Unit) Header() *Header { return CopyHeader(b.header) }

func (b *Unit) Body() *Body {
	return &Body{
		Sign:  b.Sign,
		Txs:   b.transactions,
		Votes: b.witenssVotes,
	}
}

// "external" unit encoding. used for eth protocol, etc.
type extblock struct {
	Header *Header
	Txs    TxExecs
	Votes  []WitenssVote
	Sign   *SignContent
}

// EncodeRLP 实现 RLP 序列化
func (b *Unit) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, extblock{
		Header: b.header,
		Txs:    b.transactions,
		Votes:  b.witenssVotes,
		Sign:   &b.Sign,
	})
}

// DecodeRLP 实现 RLP 编码
func (b *Unit) DecodeRLP(s *rlp.Stream) error {
	var eb extblock
	_, size, _ := s.Kind()
	if err := s.Decode(&eb); err != nil {
		return err
	}
	b.header, b.transactions, b.witenssVotes, b.Sign = eb.Header, eb.Txs, eb.Votes, *eb.Sign
	b.size.Store(common.StorageSize(rlp.ListSize(size)))
	return nil
}

func (b *Unit) Size() common.StorageSize {
	if size := b.size.Load(); size != nil {
		return size.(common.StorageSize)
	}
	c := common.StorageSize(0)
	err := rlp.Encode(&c, b)
	if err != nil {
		panic(err)
	}
	b.size.Store(c)
	return c
}

// Hash returns the keccak256 hash of b's header.
// The hash is computed on the first call and cached thereafter.
func (b *Unit) Hash() common.Hash {
	if hash := b.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	v := b.header.Hash()
	b.hash.Store(v)
	return v
}

// WithBody 基于给定的交易与投票创建新的单元.
func (b *Unit) WithBody(txs TxExecs, votes []WitenssVote, sign SignContent) *Unit {
	unit := &Unit{
		header:       CopyHeader(b.header),
		transactions: make(TxExecs, len(txs)),
		witenssVotes: votes,
		Sign:         sign,
	}
	copy(unit.transactions, txs)
	return unit
}

func (b *Unit) GetSignStruct() []interface{} {
	// 单元签名时只需要对其Hash进行签名即可
	return []interface{}{b.header.Hash()}
}

func (b *Unit) GetRSVBySignatureHash() []byte {
	return b.Sign.Get()
}

func (b *Unit) SetSign(sig []byte) {
	b.Sign.Set(sig)
}

func (b *Unit) Sender() (common.Address, error) {
	return Sender(deriveSigner(b.Sign.V()), b)
}

type Blocks []*Unit

type BlockBy func(b1, b2 *Unit) bool

func (self BlockBy) Sort(blocks Blocks) {
	bs := blockSorter{
		blocks: blocks,
		by:     self,
	}
	sort.Sort(bs)
}

func (self BlockBy) Stable(blocks Blocks) {
	bs := blockSorter{
		blocks: blocks,
		by:     self,
	}
	sort.Stable(bs)
}

type blockSorter struct {
	blocks Blocks
	by     func(b1, b2 *Unit) bool
}

func (self blockSorter) Len() int { return len(self.blocks) }
func (self blockSorter) Swap(i, j int) {
	self.blocks[i], self.blocks[j] = self.blocks[j], self.blocks[i]
}
func (self blockSorter) Less(i, j int) bool { return self.by(self.blocks[i], self.blocks[j]) }

func Number(b1, b2 *Unit) bool { return b1.header.Number < b2.header.Number }

func UnitTime(b1, b2 *Unit) bool { return b1.Timestamp() < b2.Timestamp() }

func (b *Unit) String() string {
	str := fmt.Sprintf(`Unit(%s#%v): Size: %v {
Hash: %x
%v
Transactions:
%v  
Witness:
%v
}
`, b.MC().ShortString(), b.Number(), b.Size(),
		b.Hash(), b.header, b.transactions, b.witenssVotes)
	return str
}

// AffectedUnit 受影响单元信息
type AffectedUnit struct {
	Header Header        //受影响单元
	Votes  []WitenssVote //该单元的投票
}
