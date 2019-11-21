package core

import (
	"math/big"
	"testing"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
)

//
// Tests that transactions can be added to strict lists and list contents and
// nonce boundaries are correctly maintained.

func TestTxList(t *testing.T) {
	var txList = txList{txs: make(map[common.Hash]*types.Transaction)}
	var txHashs []*types.Transaction
	// test Empty
	if !txList.Empty() {
		t.Fatalf("expect false, got true")
	}

	// test Add
	for i := 0; i < 1024; i++ {
		addr := common.BigToAddress(big.NewInt(int64(i)))
		tx := types.NewTransaction(addr, addr, big.NewInt(0), 0, 0, 0, nil)
		if txList.Add(tx) {
			t.Fatalf("expect false, got true")
		}
		if !txList.Add(tx) {
			t.Fatalf("expect true, got false")
		}
		txHashs = append(txHashs, tx)
	}

	// test delete
	if !txList.Remove(txHashs[0]) {
		t.Fatalf("expect true, got false")
	}
	if txList.Remove(txHashs[0]) {
		t.Fatalf("expect false, got false")
	}

	// test filter
	var number int
	filterFn := func(tx *types.Transaction) bool {
		number++
		return number > 10
	}
	gotNumber := len(txList.Filter(filterFn))
	if gotNumber != 1023-10 {
		t.Fatalf("expect 1013, got %v", gotNumber)
	}

	// test Flatten
	gotTx := txList.Flatten()
	if len(gotTx) != 1023 {
		t.Fatalf("expect 1022, got %v", len(gotTx))
	}

	for _, tx := range txHashs {
		txList.Remove(tx)
	}

	if !txList.Empty() {
		t.Fatalf("expect true, got false")
	}
}
