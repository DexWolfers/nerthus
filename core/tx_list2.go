package core

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
)

func newTxList() *txList {
	return &txList{
		txs: make(map[common.Hash]*types.Transaction),
	}
}

// it is thread unsafe
// TODO optimize object recycle use
type txList struct {
	txs map[common.Hash]*types.Transaction
}

// Remove remove special transaction, if found return true, not found return false
func (tList *txList) Remove(tx *types.Transaction) bool {
	if _, ok := tList.txs[tx.Hash()]; ok {
		delete(tList.txs, tx.Hash())
		return true
	} else {
		return false
	}
}

func (tList *txList) Empty() bool {
	return len(tList.txs) == 0
}

func (tList *txList) Flatten() types.Transactions {
	txs := make(types.Transactions, 0, len(tList.txs))
	for _, tx := range tList.txs {
		txs = append(txs, types.CopyTransaction(tx))
	}
	return txs
}
func (tList *txList) List() types.Transactions {
	txs := make(types.Transactions, 0, len(tList.txs))
	for _, tx := range tList.txs {
		txs = append(txs, tx)
	}
	return txs
}

// Add a transaction, return true if exists the transaction, otherwise return false
func (tList *txList) Add(tx *types.Transaction) (ok bool) {
	_, ok = tList.txs[tx.Hash()]
	if !ok {
		tList.txs[tx.Hash()] = tx
	}
	return
}

func (tList *txList) Filter(filterFn func(tx *types.Transaction) bool) types.Transactions {
	var txs []*types.Transaction
	for _, tx := range tList.txs {
		if filterFn(tx) {
			txs = append(txs, tx)
		}
	}
	return txs
}

func (tList *txList) Len() int {
	return len(tList.txs)
}
