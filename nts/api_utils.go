package nts

import (
	"errors"
	"sort"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/hexutil"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/types"
)

// 单元信息用于展开单元细节
type Block struct {
	General common.Address      `json:"general"`
	Header  *types.Header       `json:"header"`
	Body    *types.Body         `json:"body"`
	From    common.Address      `json:"from"`
	ListWit []common.Address    `json:"witness_list"`
	Votes   []types.WitenssVote `json:"witness_votes"`
}

func ToBlockStruct(unit *types.Unit) (*Block, error) {
	if unit == nil {
		return nil, errors.New("unit not find")
	}
	var enc Block

	sender, err := unit.Sender()
	if err != nil {
		return nil, err
	}

	witnesses, err := unit.Witnesses()
	if err != nil {
		return nil, err
	}

	enc.Header = unit.Header()
	enc.General = sender
	enc.ListWit = witnesses
	enc.From = unit.MC()
	enc.Body = unit.Body()
	enc.Votes = unit.WitnessVotes()
	return &enc, nil
}

type ClientTransaction struct {
	TxType   int
	TxAction types.TxAction
	Source   common.Hash
	Source2  types.UnitID
	TxHash   common.Hash
	Tx       *types.Transaction
	Result   *types.TxResult

	txTime uint64
}

// 返回给客户端的结果集
type ResultSet struct {
	Total  int
	Result interface{}
}

type sortTransaction struct {
	set []*ClientTransaction
	by  func(p, q *ClientTransaction) bool
}

func (st sortTransaction) Len() int           { return len(st.set) }
func (st sortTransaction) Swap(i, j int)      { st.set[i], st.set[j] = st.set[j], st.set[i] }
func (st sortTransaction) Less(i, j int) bool { return st.by(st.set[i], st.set[j]) }

type sortBlocks struct {
	set []*Block
	by  func(p, q *Block) bool
}

func (sb sortBlocks) Len() int                           { return len(sb.set) }
func (sb sortBlocks) Swap(i, j int)                      { sb.set[i], sb.set[j] = sb.set[j], sb.set[i] }
func (sb sortBlocks) Less(i, j int) bool                 { return sb.by(sb.set[i], sb.set[j]) }
func BlockSortBy(bs []*Block, by func(p, q *Block) bool) { sort.Sort(sortBlocks{bs, by}) }

// RPCTransaction represents a transaction that will serialize to the RPC representation of a transaction
type RPCTransaction struct {
	From     common.Address `json:"from"`
	To       common.Address `json:"to"`
	Gas      hexutil.Uint64 `json:"gas"`
	GasUsed  hexutil.Uint64 `json:"gasUsed"`
	GasPrice hexutil.Uint64 `json:"gasPrice"`
	Hash     common.Hash    `json:"hash"`
	Input    hexutil.Bytes  `json:"input"`
	Nonce    hexutil.Uint64 `json:"nonce"`

	Amount hexutil.Big   `json:"amount"`
	Sign   hexutil.Bytes `json:"sign"`

	Time           uint64                  `json:"time"`
	ExpirationTime uint64                  `json:"expiration_time"`
	Type           types.TransactionType   `json:"type"`
	Status         types.TransactionStatus `json:"status"`
	Seed           hexutil.Uint64          `json:"seed"`
}

// newRPCTransaction returns a transaction that will serialize to the RPC
// representation, with the given location metadata set (if available).
func newRPCTransaction(dc *core.DagChain, tx *types.Transaction) (*RPCTransaction, error) {

	signer := types.NewSigner(tx.ChainId())
	from, _ := types.Sender(signer, tx)
	sign := tx.RawSignatureValues()

	status, err := GetTxStatus(dc, tx.Hash(), tx)
	if err != nil {
		return nil, err
	}

	result := &RPCTransaction{
		From:           from,
		Gas:            hexutil.Uint64(tx.Gas()),
		GasPrice:       hexutil.Uint64(tx.GasPrice()),
		Hash:           tx.Hash(),
		Input:          hexutil.Bytes(tx.Data()),
		Nonce:          hexutil.Uint64(tx.Nonce()),
		To:             tx.To(),
		Amount:         hexutil.Big(*tx.Amount()),
		Sign:           sign,
		Time:           tx.Time(),
		ExpirationTime: uint64(tx.ExpirationTime().UnixNano()),
		Type:           sc.GetTxType(tx),
		Status:         status,
		Seed:           hexutil.Uint64(tx.Seed()),
	}
	return result, nil
}

// 获取指定交易的各执行阶段状态
func GetTxSetpStatus(dag *core.DAG, dc *core.DagChain, txhash common.Hash) ([]TxActionStatusInfo, error) {
	//TODO: 可以从 receipts 下取数据
	list := make([]TxActionStatusInfo, 0)
	err := dag.GetTransactionStatus(txhash, func(info core.TxStatusInfo) (b bool, e error) {
		chain, number := dc.GetUnitNumber(info.UnitHash)
		status := TxActionStatusInfo{
			Chain:    chain,
			Number:   number,
			UnitHash: info.UnitHash,
			Action:   info.Action,
			Failed:   info.Failed,
			Status:   "stabled",
		}
		list = append(list, status)
		return true, nil
	})
	if err == core.ErrMissingTxStatusInfo {
		return list, nil
	}
	return list, err
}

func GetTxStatus(dc *core.DagChain, hash common.Hash, tx *types.Transaction) (types.TransactionStatus, error) {
	dag := dc.DagReader().(*core.DAG)
	next, err := dag.GetTxNextAction(hash, tx)
	if err == core.ErrTxExpired {
		return types.TransactionStatusExpired, nil
	}
	if err != nil {
		return types.TransactionStatusUnderway, err
	}
	// 如果 next 为空，则说明阶段已处理完成
	if len(next) == 0 {
		list, err := GetTxSetpStatus(dag, dc, hash)
		if err != nil {
			return types.TransactionStatusUnderway, err
		}
		//只需要关注最后一个action是否已稳定，如果最后一个已稳定则正常
		var lastAction types.TxAction
		for _, v := range list {
			if lastAction < v.Action {
				lastAction = v.Action
			}
		}
		status := types.TransactionStatusUnderway
		for _, v := range list {
			if v.Action != lastAction {
				continue
			}
			chain, number := dc.GetUnitNumber(v.UnitHash)
			if dc.GetStableHash(chain, number) == v.UnitHash {
				if v.Failed { //失败时标识为失败
					return types.TransactionStatusFailed, nil
				}
				//否则为成功
				status = types.TransactionStatusPass
			}
		}
		//如果最后一个单元已稳定则查找是否过程中出现过失败情况
		if status == types.TransactionStatusPass {
			for _, v := range list {
				if v.Failed {
					return types.TransactionStatusFailed, nil
				}
			}
		}
		return status, nil
	}
	return types.TransactionStatusUnderway, nil
}

type TransactionList struct {
	List  []RPCTransaction
	Count uint64
}

func (t *TransactionList) Add(transaction ...RPCTransaction) {
	t.List = append(t.List, transaction...)
}
