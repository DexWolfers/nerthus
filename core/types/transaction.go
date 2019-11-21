// @Author: yin
package types

import (
	"bytes"
	"container/heap"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"gitee.com/nerthus/nerthus/params"

	"gitee.com/nerthus/nerthus/common/math"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/hexutil"
	"gitee.com/nerthus/nerthus/rlp"
)

func (tx *Transaction) Cost() *big.Int {
	return new(big.Int).Add(tx.Amount(), math.Mul(tx.GasPrice(), tx.Gas()))
}
func (tx *Transaction) GasCost() (uint64, bool) {
	return math.SafeMul(tx.GasPrice(), tx.Gas())
}

//go:generate enumer -type=TransactionStatus -json -transform=snake -trimprefix=TransactionStatus
type TransactionStatus uint8

const (
	TransactionStatusUnderway TransactionStatus = iota + 1
	TransactionStatusPass
	TransactionStatusFailed
	TransactionStatusNot
	TransactionStatusExpired
)

//go:generate enumer -type=TransactionType -json -transform=snake -trimprefix=TransactionType
type TransactionType uint8

const (
	TransactionTypeTransfer                  TransactionType = iota + 1 // 转账
	TransactionTypeContractDeploy                                       // 部署合约
	TransactionTypeContractRun                                          //执行合约
	TransactionTypeProposalCouncilApply                                 // 理事申请提案
	TransactionTypeProposalCouncilRemove                                // 理事注销提案
	TransactionTypeProposalSetConfig                                    // 修改共识配置提案
	TransactionTypeProposalSysCampaign                                  // 系统见证人竞选提案
	TransactionTypeProposalUcCampaign                                   // 用户见证人竞选提案
	TransactionTypeProposalCouncilVote                                  // 理事投票
	TransactionTypeProposalPublicVote                                   // 公众投票
	TransactionTypeProposalCampaignJoin                                 // 加入竞选
	TransactionTypeProposalCampaignAddMargin                            // 追加保证金
	TransactionTypeProposalFinalize                                     // 提案定案
	TransactionTypeWitnessApply                                         // 申请见证人
	TransactionTypeWitnessReplace                                       // 更换见证人
	TransactionTypeWitnessCancel                                        // 注销见证人
	TransactionTypeSettlement                                           // 领取见证费
	TransactionTypeAll                                                  // 全部
)

//go:generate gencodec -type txdata -field-override txdataMarshaling -out tx_json.go

var emptyTxHash = (&Transaction{}).Hash()

type Transaction struct {
	data txdata
	// caches
	hash common.Hash
	size common.StorageSize
	from common.Address
}

// txdata 交易信息
//
// 交易信息中去除了nonce （外部无需为何一个增长的 nonce ，特别是在并发环境下），
// 可引发的问题是：秒内并发，引起交易hash碰撞。故将时间调整为纳秒级。
//
// 注意：go 纳秒表示的是1678 到 2262年
//
type txdata struct {
	Time      uint64         `json:"time"     gencodec:"required"`
	Price     uint64         `json:"gasPrice" gencodec:"required"`
	GasLimit  uint64         `json:"gas"      gencodec:"required"`
	Sender    common.Address `json:"sender"   gencodec:"required"`
	Recipient common.Address `json:"to"       gencodec:"required"`
	Amount    *big.Int       `json:"value"    gencodec:"required"`
	Payload   []byte         `json:"input"    gencodec:"required"`
	Seed      uint64         `json:"seed"`
	Timeout   uint64         `json:"timeout"` // 过期时间, 多少秒过期
	// Signature values
	Sign *SignContent `json:"sign"    gencodec:"required"`

	// This is only used when marshaling to JSON.
	Hash *common.Hash   `json:"hash" rlp:"-"`
	From common.Address `json:"from" rlp:"-"`
}

type txdataMarshaling struct {
	Time     hexutil.Uint64
	Price    hexutil.Uint64
	GasLimit hexutil.Uint64
	Amount   *hexutil.Big
	Payload  hexutil.Bytes
	Seed     hexutil.Uint64
	Timeout  hexutil.Uint64

	Sign *SignContent
}

func CopyTransaction(t *Transaction) *Transaction {
	if t == nil {
		return nil
	}
	cp := Transaction{
		data: txdata{
			Time:      t.Time(),
			Price:     t.data.Price,
			GasLimit:  t.data.GasLimit,
			Sender:    t.data.Sender,
			Recipient: t.data.Recipient,
			Amount:    common.BigCopy(t.data.Amount),
			Payload:   common.CopyBytes(t.data.Payload),
			Seed:      t.data.Seed,
			Timeout:   t.Timeout(),
			Sign: &SignContent{
				t.data.Sign.Copy(),
			},
		},
		hash: t.hash,
		size: t.size,
		from: t.from,
	}
	return &cp
}

const (
	oneMinute = 60
	oneHour   = 60 * oneMinute
	// 交易默认过期有效时长 1小时(单位秒)
	DefTxTimeout uint64 = oneHour
)

var (
	DefTxMinTimeout uint64 = uint64(params.OneRoundTime() / time.Second) //最短时间 20 分钟
	DefTxMaxTimeout uint64 = 7 * 24 * oneHour                            //最长设置7天
)

// NewTransaction 新建交易
func NewTransaction(from, to common.Address, amount *big.Int, gasLimit, gasPrice, timeout uint64, data []byte) *Transaction {
	if timeout == 0 {
		timeout = DefTxTimeout
	}
	return newTransaction(from, to, amount, gasLimit, gasPrice, timeout, 0, data)
}

// newTransaction 新建交易
func newTransaction(from, to common.Address, amount *big.Int, gasLimit, gasPrice, timeout, seed uint64, data []byte) *Transaction {
	if len(data) > 0 {
		data = common.CopyBytes(data)
	}
	t := uint64(time.Now().UnixNano())
	if t == 0 {
		// 几乎不会发生
		panic("newTransaction:time.Now().UnixNano() equal zero")
	}

	d := txdata{
		Time:      uint64(time.Now().UnixNano()), //记录为毫秒，适用于并发
		Sender:    from,
		Recipient: to,
		Payload:   data,
		Amount:    new(big.Int),
		GasLimit:  gasLimit,
		Price:     gasPrice,
		Seed:      seed,
		Timeout:   timeout,
		Sign:      NewSignContent(),
	}
	if amount != nil {
		d.Amount.Set(amount)
	}
	return &Transaction{data: d}
}

// SetNonce 设置Nonce
// 此方法仅用于测试！！！！！
func (tx *Transaction) SetNonce(nonce uint64) {
	tx.data.Time = nonce
}
func (tx *Transaction) SetTimeStamp(timestamp uint64) {
	tx.data.Time = timestamp
}

// ChainId returns which chain id this transaction was signed for (if at all)
func (tx *Transaction) ChainId() *big.Int {
	return deriveChainId(tx.data.Sign.V())
}

func (tx *Transaction) SetSeed(seed uint64) {
	tx.data.Seed = seed
}

func (tx *Transaction) HashNoSeed() common.Hash {
	// skip seed
	dataInputVec := []interface{}{
		tx.data.Time,
		tx.data.Price,
		tx.data.GasLimit,
		tx.data.Sender,
		tx.data.Recipient,
		tx.data.Amount,
		tx.data.Payload,
		tx.data.Timeout,
	}
	return common.SafeHash256(dataInputVec)
}

// GetSignStruct 获取加密的数据
func (tx *Transaction) GetSignStruct() []interface{} {
	if tx.data.Payload == nil {
		tx.data.Payload = []byte{}
	}
	return []interface{}{
		tx.data.Time,
		tx.data.Price,
		tx.data.GasLimit,
		tx.data.Sender,
		tx.data.Recipient,
		tx.data.Amount,
		tx.data.Payload,
		tx.data.Seed,
		tx.data.Timeout,
	}
}
func (tx *Transaction) GetRSVBySignatureHash() []byte {
	return tx.data.Sign.Get()
}

// SetSign 设置交易密钥
func (tx *Transaction) SetSign(sig []byte) {
	tx.data.Sign.Set(sig)
}

// WithSignature returns a new transaction with the given signature.
// This signature needs to be formatted as described in the yellow paper (v+27).
func (tx *Transaction) WithSignature(signer Signer, sig []byte) (*Transaction, error) {
	err := signer.WithSignature(tx, sig)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

// DecodeRLP implements rlp.Encoder
func (tx *Transaction) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, &tx.data)
}

// DecodeRLP implements rlp.Decoder
func (tx *Transaction) DecodeRLP(s *rlp.Stream) error {
	_, size, _ := s.Kind()
	err := s.Decode(&tx.data)
	if err == nil {
		tx.size = common.StorageSize(rlp.ListSize(size))
	}
	return err
}

func (tx *Transaction) MarshalJSON() ([]byte, error) {
	hash := tx.Hash()
	data := tx.data
	data.Hash = &hash
	data.From, _ = tx.Sender()
	return data.MarshalJSON()
}

// UnmarshalJSON decodes the web3 RPC transaction format.
func (tx *Transaction) UnmarshalJSON(input []byte) error {
	var dec txdata
	if err := dec.UnmarshalJSON(input); err != nil {
		return err
	}

	err := deriveSigner(dec.Sign.V()).ValidateSignatureValues(&Transaction{data: dec})
	if err != nil {
		return err
	}
	*tx = Transaction{data: dec}
	return nil
}

func (tx *Transaction) Data() []byte { return common.CopyBytes(tx.data.Payload) }

// 获取交易数据的前4个长度
func (tx *Transaction) Data4() []byte {
	v := make([]byte, 4)
	copy(v, tx.data.Payload)
	return v
}

func (tx *Transaction) Gas() uint64                { return tx.data.GasLimit }
func (tx *Transaction) GasPrice() uint64           { return tx.data.Price }
func (tx *Transaction) Amount() *big.Int           { return new(big.Int).Set(tx.data.Amount) }
func (tx *Transaction) Time() uint64               { return tx.data.Time }
func (tx *Transaction) Timeout() uint64            { return tx.data.Timeout }
func (tx *Transaction) SenderAddr() common.Address { return tx.data.Sender }
func (tx *Transaction) ExpirationTime() time.Time {
	if tx.data.Timeout > 0 {
		return time.Unix(0, int64(tx.data.Time)).Add(time.Second * time.Duration(tx.data.Timeout))
	}
	return time.Time{}
}

// Expired 判断交易是相对时间vs否已过期
func (tx *Transaction) Expired(vs time.Time) bool {
	if tx.Seed() > 0 {
		if tx.Time()+uint64(time.Hour.Nanoseconds()) < uint64(vs.UnixNano()) {
			return true
		}
	}
	t := tx.ExpirationTime()
	if t.IsZero() {
		return false
	}
	if vs.After(t) {
		return true
	}
	return false
}

func (tx *Transaction) Nonce() uint64 { return tx.data.Time }

func (tx *Transaction) Seed() uint64 {
	return tx.data.Seed
}
func (tx *Transaction) Time2() time.Time {
	return time.Unix(0, int64(tx.data.Time))
}

//是否是未来时间的交易
func (tx *Transaction) IsFuture(now time.Time) bool {
	return tx.Time2().Sub(now) > time.Minute*2
}

// To returns the recipient address of the transaction.
// It returns nil if the transaction is a contract creation.
func (tx *Transaction) To() common.Address {
	//if tx.data.Recipient == nil {
	//	return nil
	//} else {
	//	to := *tx.data.Recipient
	//	return &to
	//}
	return tx.data.Recipient
}

// Hash hashes the RLP encoding of tx.
// It uniquely identifies the transaction.
func (tx *Transaction) Hash() common.Hash {
	if tx == nil {
		return EmptyRootHash
	}
	if tx.hash.Empty() {
		tx.hash = common.SafeHash256(tx.GetSignStruct())
	}
	return tx.hash
}

// SigHash returns the hash to be signed by the sender.
// It does not uniquely identify the transaction.
func (tx *Transaction) SigHash(signer Signer) common.Hash {
	return signer.Hash(tx)
}

func (tx *Transaction) Size() common.StorageSize {
	if tx.size.Empty() {
		c := common.StorageSize(0)
		rlp.Encode(&c, &tx.data)
		tx.size = c
	}
	return tx.size
}

// Sender 解密交易
func (tx *Transaction) Sender() (common.Address, error) {
	return Sender(deriveSigner(tx.data.Sign.V()), tx)
}

func SameTxCheck(oldTx, newTx *Transaction) bool {
	if oldTx.Hash() != newTx.Hash() {
		return false
	}
	//相同交易有相同的签名则视为一致
	if bytes.Equal(oldTx.GetRSVBySignatureHash(), newTx.GetRSVBySignatureHash()) {
		newTx.from = oldTx.from
		return true
	}
	return false
}

// AsMessage 将交易作为一个消息返回。
//
// AsMessage 需要signer获取签名者
func (tx *Transaction) AsMessage(s Signer, gasLimit uint64) (*Message, error) {
	addr, err := Sender(s, tx)
	if err != nil {
		return nil, err
	}
	return NewMessage(tx, gasLimit, addr, tx.To(), false), nil
}

//
//// Cost returns amount + gasprice * gaslimit.
//func (tx *Transaction) Cost() *big.Int {
//	total := new(big.Int).Mul(tx.data.Price, tx.data.GasLimit)
//	total.Add(total, tx.data.Amount)
//	return total
//}

func (tx *Transaction) RawSignatureValues() []byte {
	return tx.data.Sign.Get()
}

func (tx *Transaction) String() string {
	var from, to string
	if tx.data.Sign.V() != nil {
		signer := deriveSigner(tx.data.Sign.V())
		if f, err := Sender(signer, tx); err != nil { // derive but don't cache
			from = "[invalid sender: invalid sig]"
		} else {
			from = f.String()
		}
	} else {
		from = "[invalid sender: nil V field]"
	}

	//if tx.data.Recipient == nil {
	//	to = "[contract creation]"
	//} else {
	//	to = fmt.Sprintf("%x", tx.data.Recipient[:])
	//}
	to = fmt.Sprintf("%x", tx.data.Recipient.String())
	enc, _ := rlp.EncodeToBytes(&tx.data)
	return fmt.Sprintf(`
	TX(%x)
	Contract: %v
	From:     %s
	To:       %s
	Time:    %v
	Timeout:    %d
	GasPrice: %d
	GasLimit  %d
	Seed	  %d
	Value:    %#x
	Data:     0x%x
	Sign:     %v
	Hex:      %x
`,
		tx.Hash(),
		//tx.data.Recipient == nil,
		false,
		from,
		to,
		tx.data.Time,
		tx.data.Timeout,
		tx.data.Price,
		tx.data.GasLimit,
		tx.data.Seed,
		tx.data.Amount,
		tx.data.Payload,
		common.Bytes2Hex(tx.data.Sign.Get()),
		enc,
	)
}

//go:generate gencodec -type TxResult -field-override txresultMarshaling -out gen_txresult_json.go

// TxResult 交易执行结果
// Author: @ysqi
type TxResult struct {
	Failed             bool           `json:"failed"            gencodec:"required"`
	GasUsed            uint64         `json:"gas_used"          gencodec:"required"`
	GasRemaining       uint64         `json:"gas_remaining" gencodec:"required"` // remaining gas
	CallContractOutput []byte         `json:"call_contract_output"`
	ReceiptRoot        common.Hash    `json:"receipt_root"     gencodec:"required"`
	NewContractAcct    common.Address `json:"new_contract_acct"`
	Coinflows          Coinflows      `json:"coinflows" ` // 账务输出 @zwj

	//hash 缓存
	hash common.Hash
}

type txresultMarshaling struct {
	CallContractOutput hexutil.Bytes
	GasUsed            hexutil.Uint64
	GasRemaining       hexutil.Uint64

	Hash common.Hash `json:"hash"` // adds call to Hash() in MarshalJSON
}

func (t *TxResult) Hash() common.Hash {
	if t == nil {
		return common.Hash{}
	}
	if t.hash.Empty() {
		if t.CallContractOutput == nil {
			t.CallContractOutput = []byte{}
		}
		v := common.SafeHash256([]interface{}{
			t.Failed,
			t.CallContractOutput, t.GasUsed, t.GasRemaining,
			t.ReceiptRoot,
			t.NewContractAcct,
		})
		t.hash = v
	}
	return t.hash
}

//go:generate gencodec -type Coinflow -field-override coinflowMarshaling -out gen_coinflow_json.go

// Accounting 交易中记账记录
type Coinflow struct {
	To     common.Address
	Amount *big.Int
}

var _ = coinflowMarshaling{}

type coinflowMarshaling struct {
	Amount *hexutil.Big
}

// 记账簿
type Coinflows []Coinflow

// Len is part of sort.Interface.
func (s Coinflows) Len() int {
	return len(s)
}

// Swap is part of sort.Interface.
func (s Coinflows) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Less is part of sort.Interface.
func (s Coinflows) Less(i, j int) bool {
	if s[i].To == s[j].To {
		return s[i].Amount.Cmp(s[j].Amount) > 0
	}
	return s[i].To.Big().Cmp(s[j].To.Big()) > 0
}

func (s Coinflows) String() string {
	var b, _ = json.Marshal(s)
	return string(b)
}

func CopyCoinflows(flows Coinflows) Coinflows {
	newflows := make(Coinflows, len(flows))
	copy(newflows[:], flows)

	return newflows
}

//go:generate gencodec -type TransactionExec -field-override transactionExecMarshaling -out gen_transactionexec_json.go

// TransactionExec 交易执行信息，执行将包含交易消息和交易执行结果。
// 在单元中存储该信息
type TransactionExec struct {
	Action  TxAction     `json:"action"          gencodec:"required"`
	PreStep UnitID       `json:"prestep"          gencodec:"required"`
	TxHash  common.Hash  `json:"tx_hash"         gencodec:"required"`
	Tx      *Transaction `json:"tx" rlp:"-"`

	hash atomic.Value
}

type transactionExecMarshaling struct {
	Hash common.Hash `json:"hash"` // adds call to Hash() in MarshalJSON
}

// RealTxHash 获取本次交易操作所对应的交易hash
func (tx *TransactionExec) RealTxHash() common.Hash {
	if tx.TxHash.Empty() {
		return tx.Tx.Hash()
	}
	return tx.TxHash
}

type transactionExecRlp struct {
	Action TxAction
	TxHash common.Hash
	HasTx  bool
	Source UnitID
}

var execRlpPool = sync.Pool{
	New: func() interface{} {
		return &transactionExecRlp{}
	},
}

// DecodeRLP implements rlp.Encoder
func (tx *TransactionExec) EncodeRLP(w io.Writer) error {
	info := execRlpPool.Get().(*transactionExecRlp)
	info.Action = tx.Action
	info.Source = tx.PreStep
	info.TxHash = tx.TxHash
	info.HasTx = tx.Tx != nil

	if err := rlp.Encode(w, info); err != nil {
		return err
	}
	execRlpPool.Put(info)

	if tx.Tx != nil {
		if err := rlp.Encode(w, tx.Tx); err != nil {
			return err
		}
	}
	return nil
}

// DecodeRLP implements rlp.Decoder
func (tx *TransactionExec) DecodeRLP(s *rlp.Stream) error {
	info := execRlpPool.Get().(*transactionExecRlp)
	if err := s.Decode(info); err != nil {
		return err
	}
	tx.Tx = nil
	if info.HasTx {
		tx.Tx = new(Transaction)
		if err := s.Decode(tx.Tx); err != nil {
			return err
		}
	}
	tx.TxHash = info.TxHash
	tx.Action = info.Action
	tx.PreStep = info.Source
	info = nil
	return nil
}

func (tx *TransactionExec) Hash() common.Hash {
	if hash := tx.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	v := common.SafeHash256(tx)
	tx.hash.Store(v)
	return v
}

func (tx *TransactionExec) Copy() *TransactionExec {
	var cp TransactionExec
	cp.Action = tx.Action
	cp.PreStep.Hash = tx.PreStep.Hash
	cp.PreStep.ChainID = tx.PreStep.ChainID
	cp.PreStep.Height = tx.PreStep.Height
	cp.TxHash = tx.TxHash
	cp.Tx = CopyTransaction(tx.Tx)
	return &cp
}

func CopyTxResult(r *TxResult) *TxResult {
	if r == nil {
		return nil
	}
	return &TxResult{
		Failed:             r.Failed,
		CallContractOutput: common.CopyBytes(r.CallContractOutput),
		GasUsed:            r.GasUsed,
		GasRemaining:       r.GasRemaining,
		ReceiptRoot:        r.ReceiptRoot,
		NewContractAcct:    r.NewContractAcct,
		// 新增记账数据
		// Author: @zwj
		Coinflows: CopyCoinflows(r.Coinflows),
	}
}

// NewTxResult 创建交易结果信息
func NewTxResult(receipt *Receipt) *TxResult {
	if receipt == nil {
		panic(fmt.Sprintf("NewTxResult: receipt is nil"))
	}

	result := TxResult{
		CallContractOutput: common.CopyBytes(receipt.Output),
		ReceiptRoot:        DeriveSha(Receipts{receipt}),
		Failed:             receipt.Failed,
		GasUsed:            receipt.GasUsed,
		GasRemaining:       receipt.GasRemaining,
		NewContractAcct:    receipt.ContractAddress,
		// 新增记账数据
		// Author: @zwj
		Coinflows: CopyCoinflows(receipt.Coinflows),
	}

	return &result
}

// Transaction slice type for basic sorting.
type Transactions []*Transaction

// Len returns the length of s
func (s Transactions) Len() int { return len(s) }

// Swap swaps the i'th and the j'th element in s
func (s Transactions) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// GetRlp implements Rlpable and returns the i'th element of s in rlp
func (s Transactions) GetRlp(i int) []byte {
	enc, _ := rlp.EncodeToBytes(s[i])
	return enc
}

// Returns a new set t which is the difference between a to b
func TxDifference(a, b Transactions) (keep Transactions) {
	keep = make(Transactions, 0, len(a))

	remove := make(map[common.Hash]struct{})
	for _, tx := range b {
		remove[tx.Hash()] = struct{}{}
	}

	for _, tx := range a {
		if _, ok := remove[tx.Hash()]; !ok {
			keep = append(keep, tx)
		}
	}

	return keep
}

// TxByNonce implements the sort interface to allow sorting a list of transactions
// by their nonces. This is usually only useful for sorting transactions from a
// single account, otherwise a nonce comparison doesn't make much sense.
type TxByNonce Transactions

func (s TxByNonce) Len() int           { return len(s) }
func (s TxByNonce) Less(i, j int) bool { return s[i].data.Time < s[j].data.Time }
func (s TxByNonce) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func (s *TxByNonce) Push(x interface{}) {
	*s = append(*s, x.(*Transaction))
}

func (s *TxByNonce) Pop() interface{} {
	old := *s
	n := len(old)
	x := old[n-1]
	*s = old[0 : n-1]
	return x
}

// TxByPrice implements both the sort and the heap interface, making it useful
// for all at once sorting as well as individually adding and removing elements.
type TxByPrice Transactions

func (s TxByPrice) Len() int           { return len(s) }
func (s TxByPrice) Less(i, j int) bool { return s[i].data.Price > s[j].data.Price }
func (s TxByPrice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func (s *TxByPrice) Push(x interface{}) {
	*s = append(*s, x.(*Transaction))
}

func (s *TxByPrice) Pop() interface{} {
	old := *s
	n := len(old)
	x := old[n-1]
	*s = old[0 : n-1]
	return x
}

// TransactionsByPriceAndNonce represents a set of transactions that can return
// transactions in a profit-maximising sorted order, while supporting removing
// entire batches of transactions for non-executable accounts.
type TransactionsByPriceAndNonce struct {
	txs   map[common.Address]TxByNonce // Per account nonce-sorted list of transactions
	heads TxByPrice                    // Next transaction for each unique account (price heap)
}

// NewTransactionsByPriceAndNonce creates a transaction set that can retrieve
// price sorted transactions in a nonce-honouring way.
//
// Note, the input map is reowned so the caller should not interact any more with
// if after providng it to the constructor.
func NewTransactionsByPriceAndNonce(txs map[common.Address]Transactions) *TransactionsByPriceAndNonce {
	// Initialize a price based heap with the head transactions
	heads := make(TxByPrice, 0, len(txs))
	txsByNonce := make(map[common.Address]TxByNonce, len(txs))

	for acc, accTxs := range txs {
		heads = append(heads, accTxs[0])
		acctxs := TxByNonce(accTxs[1:])
		heap.Init(&acctxs)
		txsByNonce[acc] = acctxs

	}
	heap.Init(&heads)

	// Assemble and return the transaction set
	return &TransactionsByPriceAndNonce{
		txs:   txsByNonce,
		heads: heads,
	}
}

// Peek returns the next transaction by price.
func (t *TransactionsByPriceAndNonce) Peek() *Transaction {
	if len(t.heads) == 0 {
		return nil
	}
	return t.heads[0]
}

// Shift replaces the current best head with the next one from the same account.
func (t *TransactionsByPriceAndNonce) Shift() {
	signer := deriveSigner(t.heads[0].data.Sign.V())
	// derive signer but don't cache.
	acc, _ := Sender(signer, t.heads[0]) // we only sort valid txs so this cannot fail
	if txs, ok := t.txs[acc]; ok && len(txs) > 0 {
		t.heads[0], t.txs[acc] = txs[0], txs[1:]
		heap.Fix(&t.heads, 0)
	} else {
		heap.Pop(&t.heads)
	}
}

// Pop removes the best transaction, *not* replacing it with the next one from
// the same account. This should be used when a transaction cannot be executed
// and hence all subsequent ones should be discarded from the same account.
func (t *TransactionsByPriceAndNonce) Pop() {
	heap.Pop(&t.heads)
}

// Message is a fully derived transaction and implements core.Message
//
// NOTE: In a future PR this will be removed.
type Message struct {
	tx                *Transaction
	gasLimit          uint64
	from              common.Address
	to                common.Address
	disableCheckNonce bool // just for test
}

func NewMessage(tx *Transaction, gasLimit uint64, from common.Address, to common.Address, checkNonce bool) *Message {
	return &Message{
		tx:                tx,
		gasLimit:          gasLimit,
		from:              from,
		to:                to,
		disableCheckNonce: !checkNonce,
	}
}
func (m *Message) ID() common.Hash      { return m.tx.Hash() }
func (m *Message) From() common.Address { return m.from }
func (m *Message) To() common.Address   { return m.to }
func (m *Message) GasPrice() uint64     { return m.tx.GasPrice() }
func (m *Message) Value() *big.Int      { return m.tx.data.Amount }
func (m *Message) Gas() uint64          { return m.gasLimit }
func (m *Message) GasSub(useGas uint64) { m.gasLimit = math.ForceSafeSub(m.gasLimit, useGas) }
func (m *Message) Seed() uint64         { return m.tx.Seed() }
func (m *Message) Time() uint64         { return m.tx.Time() }
func (m *Message) Nonce() uint64        { return m.tx.Nonce() } // 不需要外部计数，使用有序时间戳作为增长标记
func (m *Message) Data() []byte         { return m.tx.data.Payload }
func (m *Message) CheckNonce() bool     { return !m.disableCheckNonce }

// VMMagic 从交易信息中确认此交易的VM标识符
// 魔数（标识符）是虚拟机的标识，
// 		如果是创建合约，则从合约代码前两位中提取，
// 		如果是发送到合约，则从合约地址中判断该合约虚拟机创建。
func (m *Message) VMMagic() uint16 {
	return 0x6060 //强制使用 EVM
}

// 交易列表排序
type SortBy = func(p, q *Transaction) bool

type TransactionSort struct {
	trans  Transactions // 交易列表
	sortBy SortBy       // 排序方法
}

func (ts TransactionSort) Len() int {
	return ts.trans.Len()
}

func (ts TransactionSort) Swap(i, j int) {
	ts.trans[i], ts.trans[j] = ts.trans[j], ts.trans[i]
}

func (ts TransactionSort) Less(i, j int) bool {
	return ts.sortBy(ts.trans[i], ts.trans[j])
}

// 排序函数接口
func TransactionSortBy(txList Transactions, sortBy SortBy) {
	sort.Sort(TransactionSort{txList, sortBy})
}

// TxExecs 交易执行数据集合
type TxExecs []*TransactionExec

// Len returns the length of s
func (s TxExecs) Len() int { return len(s) }

// GetRlp implements Rlpable and returns the i'th element of s in rlp
func (s TxExecs) GetRlp(i int) []byte {
	enc, _ := rlp.EncodeToBytes(s[i])
	return enc
}

func (s TxExecs) MRoot() common.Hash {
	list := s.TxHashItems()
	if len(list) == 0 {
		return EmptyRootHash //保持了Header处理一致
	}
	return DeriveSha(HashList(list))
}

func (s TxExecs) TxHashItems() []common.Hash {
	items := make([]common.Hash, s.Len())
	for i, t := range s {
		// 不考虑两者均为空情况
		if t.TxHash.Empty() == false {
			items[i] = t.TxHash
		} else if t.Tx != nil {
			items[i] = t.Tx.Hash()
		}
	}
	return items
}

// 判断是否包含指定交易
func (s TxExecs) Contains(txHash common.Hash) bool {
	for _, t := range s {
		if t.RealTxHash() == txHash {
			return true
		}
	}
	return false
}

type HashList []common.Hash

// Len returns the length of s
func (s HashList) Len() int { return len(s) }

// GetRlp implements Rlpable and returns the i'th element of s in rlp
func (s HashList) GetRlp(i int) []byte {
	enc, _ := rlp.EncodeToBytes(s[i])
	return enc
}

func (s HashList) String() string {
	b := bytes.NewBufferString("[")
	for i, item := range s {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(item.String())
	}
	b.WriteString("]")
	return b.String()
}

type TxResults []*TxResult

// Len returns the length of s
func (s TxResults) Len() int { return len(s) }

// GetRlp implements Rlpable and returns the i'th element of s in rlp
func (s TxResults) GetRlp(i int) []byte {
	enc, _ := rlp.EncodeToBytes(s[i])
	return enc
}
