// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"container/list"
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"sync"
	"testing"

	"gitee.com/nerthus/nerthus/core/rawdb"

	"gitee.com/nerthus/nerthus/common/testutil"

	"gitee.com/nerthus/nerthus/core/sc"

	"gitee.com/nerthus/nerthus/common/batch"

	"github.com/spf13/viper"

	"gitee.com/nerthus/nerthus/log"

	"os"

	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
	witvote "gitee.com/nerthus/nerthus/consensus/bft/backend"
	"gitee.com/nerthus/nerthus/consensus/ethash"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//// testTxPoolConfig is a transaction pool configuration without stateful disk
//// sideeffects used during testing.
var testTxPoolConfig TxPoolConfig

func init() {
	testTxPoolConfig = DefaultTxPoolConfig
}

type testBlockChain struct {
	statedb       *state.StateDB
	gasLimit      *big.Int
	chainHeadFeed *event.Feed
	rmTxFeed      *event.Feed
}

func (bc *testBlockChain) GetChainTailState(mcaddr common.Address) (*state.StateDB, error) {
	return bc.statedb, nil
}

func (bc *testBlockChain) GasLimit() *big.Int {
	return new(big.Int).Set(bc.gasLimit)
}

func (bc *testBlockChain) SubscribeChainHeadEvent(ch chan<- types.ChainHeadEvent) event.Subscription {
	return bc.chainHeadFeed.Subscribe(ch)
}

func transaction(gaslimit *big.Int, key *ecdsa.PrivateKey) *types.Transaction {
	return pricedTransaction(gaslimit, big.NewInt(1), key)
}

func pricedTransaction(gaslimit, gasprice *big.Int, key *ecdsa.PrivateKey) *types.Transaction {
	tx, _ := types.SignTx(
		types.NewTransaction(
			crypto.ForceParsePubKeyToAddress(key.PublicKey),
			common.Address{}, big.NewInt(100), gaslimit.Uint64(), gasprice.Uint64(), 0, []byte("Hello Word")),
		testSinger, key)
	return tx
}

func getAddressKeyHex(address common.Address) string {
	accinfos := params.GetDevAccounts()
	for _, ai := range accinfos {
		if common.ForceDecodeAddress(ai.Address) == address {
			return ai.KeyHex
		}
	}
	return ""
}

func setupTxPool(t testing.TB, cfg TxPoolConfig) (*TxPool, *ecdsa.PrivateKey, common.Address) {
	db, _ := ntsdb.NewMemDatabase()
	testutil.SetupTestConfig()

	g, err := LoadGenesisConfig()
	require.NoError(t, err)
	config, _, err := SetupGenesisUnit(db, g)
	if err != nil {
		t.Fatalf("set genesis error:%v", err)
	}

	chain, err := NewDagChain(db, config, witvote.NewFaker(), vm.Config{})
	if err != nil {
		t.Fatalf("fail to new dagchain:%v", err)
	}
	addr := common.ForceDecodeAddress("nts1x67wl68e9rhyxfkr4nu6jqmpvd2fufju5lk76c")
	key := getAddressKeyHex(addr)
	if key == "" {
		t.Fatal("missing account private key")
	}
	privateKey, err := crypto.HexToECDSA(key)
	if err != nil {
		t.Fatalf("expect nil, got %v", err)
	}
	pool := NewTxPool(cfg, params.TestChainConfig, chain)
	addr2 := crypto.ForceParsePubKeyToAddress(privateKey.PublicKey)
	require.Equal(t, addr, addr2)

	return pool, privateKey, addr
}

//
//// validateTxPoolInternals checks various consistency invariants within the pool.
//func validateTxPoolInternals(pool *TxPool) error {
//	pool.mu.RLock()
//	defer pool.mu.RUnlock()
//
//	// Ensure the total transaction set is consistent with pending + queued
//	pending, queued := pool.stats()
//	if total := len(pool.all); total != pending+queued {
//		return fmt.Errorf("total transaction count %d != %d pending + %d queued", total, pending, queued)
//	}
//	if priced := pool.priced.items.Len() - pool.priced.stales; priced != pending+queued {
//		return fmt.Errorf("total priced transaction count %d != %d pending + %d queued", priced, pending, queued)
//	}
//	// Ensure the next nonce to assign is the correct one
//	for addr, txs := range pool.pending {
//		// Find the last transaction
//		var last uint64
//		for nonce, _ := range txs.txs.items {
//			if last < nonce {
//				last = nonce
//			}
//		}
//		if nonce := pool.pendingState.GetNonce(addr); nonce != last+1 {
//			return fmt.Errorf("pending nonce mismatch: have %v, want %v", nonce, last+1)
//		}
//	}
//	return nil
//}
//
var testSinger = types.NewSigner(params.TestChainConfig.ChainId)

func deriveSender(tx *types.Transaction) (common.Address, error) {
	return types.Sender(testSinger, tx)
}

type testChain struct {
	*testBlockChain
	address common.Address
	trigger *bool
}

// testChain.State() is used multiple times to reset the pending state.
// when simulate is true it will create a state that indicates
// that tx0 and tx1 are included in the chain.
func (c *testChain) GetChainTailState(mcaddr common.Address) (*state.StateDB, error) {
	// delay "state change" by one. The tx pool fetches the
	// state multiple times and by delaying it a bit we simulate
	// a state change between those fetches.
	stdb := c.statedb
	if *c.trigger {
		db, _ := ntsdb.NewMemDatabase()
		c.statedb, _ = state.New([]byte{1, 2, 3}, common.Hash{}, state.NewDatabase(db))
		// simulate that the new head unit included tx0 and tx1
		c.statedb.SetNonce(c.address, 2)
		fmt.Println(c.statedb.GetNonce(c.address))
		c.statedb.SetBalance(c.address, new(big.Int).SetUint64(params.Nts))
		*c.trigger = false
	}
	return stdb, nil
}

func TestTxPool(t *testing.T) {
	cfg := testTxPoolConfig
	cfg.NoDiskTx = true
	var (
		txPool, privateKey, fromAddr = setupTxPool(t, cfg)
		txs                          []*types.Transaction
		toAddr                       = common.BigToAddress(big.NewInt(9000))
		amount                       = int64(100)
		gasLimit                     = int64(900000)
		gasPrice                     = int64(10)
		count                        = 100
	)
	defer txPool.Stop()

	// test add
	{
		txs = batchTx(toAddr, amount, gasLimit, gasPrice, nil, testSinger, privateKey, txPool, count, t)
		//if len(txPool.pending) != 1 {
		//	t.Fatalf("expect 1, got %v", len(txPool.pending))
		//}
		pending, err := txPool.Pending()
		if err != nil {
			t.Fatalf("expect nil, got %v", err)
		}
		if len(pending) != count {
			t.Fatalf("expect %d, got %v", count, len(pending))
		}

		txs := txPool.PendingByAcct(fromAddr)
		if len(txs) != count {
			t.Fatalf("expect %d, got %v", count, len(txs))
		}
	}

	// test get
	{
		if txPool.Get(txs[0].Hash()) == nil {
			t.Fatalf("expect not nil, got nil")
		}
	}

	// test delete
	{
		txPool.Remove(txs[0].Hash())
		if txPool.Get(txs[0].Hash()) != nil {
			t.Fatalf("expect nil, got %v", txPool.Get(txs[0].Hash()))
		}

		var txList []common.Hash
		for _, v := range txs {
			txList = append(txList, v.Hash())
			txPool.Remove(v.Hash())
		}

		pending, err := txPool.Pending()
		if err != nil {
			t.Fatalf("expect nil, got %v", err)
		}
		if len(pending) != 0 {
			t.Fatalf("expect 0, got %v", len(pending))
		}
	}

	// test
	{
		var number = 1024
		for i := 0; i < number; i++ {
			tx, err := types.SignTx(
				types.NewTransaction(fromAddr, toAddr, big.NewInt(amount), uint64(gasLimit), uint64(gasPrice), 0, []byte("Hello Word")),
				testSinger, privateKey)
			if err != nil {
				t.Fatalf("expect nil, got %v", err)
			}
			if err = txPool.AddLocal(tx); err != nil {
				t.Fatalf("expect nil, got %v", err)
			}
			pending, err := txPool.Pending()
			if err != nil {
				t.Fatalf("expect nil, got %v", err)
			}
			var ok bool
			for _, v := range pending {
				if v.Hash() == tx.Hash() {
					ok = true
				}
			}
			require.True(t, ok)
			fromTxs := txPool.PendingByAcct(fromAddr)
			if len(fromTxs) != i+1 {
				t.Fatalf("expect %v, got %v", i+1, len(fromTxs))
			}
		}
	}
}

//func TestTxPoolChannel(t *testing.T) {
//	var (
//		txPool, privateKey, fromAddr = setupTxPool(t)
//		txs                          []*types.Transaction
//		toAddr                       = common.BigToAddress(big.NewInt(9000))
//		count                        = 100
//	)
//	defer txPool.Stop()
//	txs = batchTx(toAddr, 1, 90000, 1, nil, testSinger, privateKey, txPool, count, t)
//	if got := len(txs); got != count {
//		t.Fatalf("expect %v, got %v", count, got)
//	}
//
//	if got := len(txPool.AccountPending(fromAddr)); got != count {
//		t.Fatalf("expect %v, got %v", count, got)
//	}
//	// remove transaction by event notify
//	txPool.dagChain.PostChainEvents([]interface{}{types.RemovedTransactionEvent{txs[:10]}}, nil)
//	time.Sleep(time.Millisecond * 30)
//	if got := len(txPool.AccountPending(fromAddr)); got != 90 {
//		t.Fatalf("expect %v, got %v", 90, got)
//	}
//	// remove all
//	txPool.dagChain.PostChainEvents([]interface{}{types.RemovedTransactionEvent{Txs: txs}}, nil)
//	time.Sleep(time.Millisecond * 30)
//	if got := len(txPool.AccountPending(fromAddr)); got != 0 {
//		t.Fatalf("expect %v, got %v", 0, got)
//	}
//}

func batchTx(toAddr common.Address, amount, gasLimit, gasPrice int64,
	data []byte, singer types.Signer, pkey *ecdsa.PrivateKey,
	txPool *TxPool, count int, t *testing.T) types.Transactions {

	var txs types.Transactions

	for i := 0; i < count; i++ {
		tx, err := types.SignTx(
			types.NewTransaction(
				crypto.ForceParsePubKeyToAddress(pkey.PublicKey),
				toAddr, big.NewInt(amount), uint64(gasLimit), uint64(gasPrice), uint64(i+1)*20000, data),
			singer, pkey)
		if err != nil {
			t.Fatalf("expect nil, got %v", err)
		}
		if i%2 == 0 {
			err = txPool.AddLocal(tx)
		} else if i%3 == 0 {
			err = txPool.addTx(tx, false)
		} else if i%5 == 0 {
			err = txPool.AddLocals([]*types.Transaction{tx})
		} else {
			err = txPool.AddRemotes([]*types.Transaction{tx})
		}
		if err != nil {
			t.Fatalf("i %v, expect nil, got %v", i, err)
		}
		// test double addr
		err = txPool.AddLocal(tx)
		if err != ErrExistsTxInTxPool && err != ErrTxChecking {
			t.Fatalf("expect %v, got %v", ErrExistsTxInTxPool, err)
		}
		txs = append(txs, tx)
	}
	return txs
}

func TestTxPool_AddRemote(t *testing.T) {
	cfg := testTxPoolConfig
	cfg.NoDiskTx = true
	txPool, privateKey, fromAddr := setupTxPool(t, cfg)

	_ = fromAddr
	to := common.ForceDecodeAddress("nts1x67wl68e9rhyxfkr4nu6jqmpvd2fufju5lk76c")

	t.Run("no sign", func(t *testing.T) {
		tx := types.NewTransaction(fromAddr, to, big.NewInt(1), 1, 1, 0, nil)
		err := txPool.addTx(tx, false)
		require.Error(t, err, types.ErrInvalidSig.Error())
	})
	t.Run("invalid chainID", func(t *testing.T) {
		tx := types.NewTransaction(fromAddr, to, big.NewInt(1), 1, 1, 0, nil)
		fakeSigner := types.NewSigner(big.NewInt(12345678))
		types.SignTx(tx, fakeSigner, privateKey)
		err := txPool.addTx(tx, false)
		if assert.NotNil(t, err) {
			assert.Contains(t, err.Error(), types.ErrInvalidChainId.Error())
		}
	})
	t.Run("invalid to", func(t *testing.T) {
		var emptyTo common.Address
		tx := types.NewTransaction(fromAddr, emptyTo, big.NewInt(1), 1, 1, 0, nil)
		types.SignTx(tx, txPool.signer, privateKey)
		err := txPool.addTx(tx, false)
		if assert.NotNil(t, err) {
			assert.Contains(t, err.Error(), ErrEmptyToAddress.Error())
		}
	})

	t.Run("invalid sender", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			tx := types.NewTransaction(fromAddr, to, big.NewInt(1), params.TxGas, 1, 0, nil)
			types.SignTx(tx, txPool.signer, privateKey)
			err := txPool.addTx(tx, false)
			require.NoError(t, err)
		})
		t.Run("failed", func(t *testing.T) {
			from := common.ForceDecodeAddress("nts1r3ga242u272jg7krmmz3348ak924xjhrlqsxny")
			tx := types.NewTransaction(from, to, big.NewInt(1), params.TxGas, 1, 0, nil)
			types.SignTx(tx, txPool.signer, privateKey)
			err := txPool.addTx(tx, false)
			require.Error(t, err)
			require.Contains(t, err.Error(), "the tx sender is not equal signer")
		})

	})

	t.Run("Balance", func(t *testing.T) {
		t.Run("zero", func(t *testing.T) {
			key1, _ := crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
			tx := types.NewTransaction(crypto.ForceParsePubKeyToAddress(key1.PublicKey), to, big.NewInt(1), params.MinGasLimit, 1, 0, nil)
			types.SignTx(tx, txPool.signer, key1)

			err := txPool.addTx(tx, false)
			if assert.NotNil(t, err) {
				assert.Contains(t, err.Error(), "insufficient funds")
			}
		})
		t.Run("insufficient value", func(t *testing.T) {
			tx := types.NewTransaction(fromAddr, to, math.MaxBig256, params.MinGasLimit, 1, 0, nil)

			types.SignTx(tx, txPool.signer, privateKey)

			err := txPool.addTx(tx, false)
			if assert.NotNil(t, err) {
				assert.Contains(t, err.Error(), "insufficient funds")
			}
		})
		t.Run("insufficient gas", func(t *testing.T) {
			tx := types.NewTransaction(fromAddr, to, big.NewInt(1), params.MinGasLimit, math.MaxUint32, 0, nil)

			types.SignTx(tx, txPool.signer, privateKey)

			err := txPool.addTx(tx, false)
			if assert.NotNil(t, err) {
				assert.Contains(t, err.Error(), "intrinsic gas too low")
			}
		})
	})
	t.Run("GasLimit", func(t *testing.T) {
		minGas := params.TxGas

		t.Run("Below the min gas", func(t *testing.T) {
			tx := types.NewTransaction(fromAddr, to, big.NewInt(1), minGas-1, 1, 0, nil)

			types.SignTx(tx, txPool.signer, privateKey)
			err := txPool.addTx(tx, false)
			if assert.NotNil(t, err) {
				assert.Contains(t, err.Error(), "intrinsic gas too low")
			}
		})
		t.Run("equal the min gas", func(t *testing.T) {
			tx := types.NewTransaction(fromAddr, to, big.NewInt(1), minGas, 1, 0, nil)

			types.SignTx(tx, txPool.signer, privateKey)
			err := txPool.addTx(tx, false)
			assert.NoError(t, err)
		})
	})
	t.Run("repeat tx", func(t *testing.T) {

		minGas := params.TxGas
		t.Run("repeat in tx pool", func(t *testing.T) {
			tx := types.NewTransaction(fromAddr, to, big.NewInt(1), minGas, 1, 0, nil)
			types.SignTx(tx, txPool.signer, privateKey)
			err := txPool.addTx(tx, false)
			if assert.NoError(t, err) {
				// 重复添加相同交易则会失败
				err = txPool.addTx(tx, false)
				if assert.NotNil(t, err) {
					assert.Contains(t, err.Error(), ErrExistsTxInTxPool.Error())
				}
			}
		})

		//t.Run("repeat in chain", func(t *testing.T) {
		//
		//	t.Run("payment tx", func(t *testing.T) {
		//
		//		tx := types.NewTransaction(to, big.NewInt(1), new(big.Int).SetUint64(params.MinGasLimit), big.NewInt(1), nil, 0)
		//		types.SignTx(tx, txPool.signer, privateKey)
		//
		//		// 准备与构造新的 state Header
		//		insertActionInNewHeader(txPool, tx)
		//
		//		//重新放置
		//		err := txPool.addTx(tx,false)
		//		if assert.NotNil(t, err) {
		//			assert.Contains(t, err.Error(), "repeat transaction in chain")
		//		}
		//	})
		//
		//	t.Run("pow tx", func(t *testing.T) {
		//
		//		changeWitnessAddr := params.SCAccount
		//		abiFn := sc.CreateCallInputData(sc.FuncReplaceWitnessID, fromAddr)
		//		tx := types.NewTransaction(changeWitnessAddr, big.NewInt(0), new(big.Int).SetUint64(params.MinGasLimit*2), big.NewInt(1), abiFn, 0)
		//		tx.SetSeed(new(big.Int).SetUint64(<-ethash.ProofWork(tx, 1,
		//			new(big.Int).SetUint64(txPool.chainconfig.Get(params.ConfigParamsMinFreePoW)),
		//			make(chan struct{}))))
		//		types.SignTx(tx, txPool.signer, privateKey)
		//
		//		// 准备与构造新的 state Header
		//		insertActionInNewHeader(txPool, tx)
		//
		//		//重新放置
		//		err := txPool.addTx(tx,false)
		//		if assert.NotNil(t, err) {
		//			assert.Contains(t, err.Error(), "repeat transaction in chain")
		//		}
		//	})
		//
		//})

	})

	t.Run("transaction has expired", func(t *testing.T) {
		t.Run("invalid set", func(t *testing.T) {
			var timeout uint64 = 1 //1s
			tx := types.NewTransaction(fromAddr, to, big.NewInt(1), params.TxGas*2, 1, timeout, nil)
			types.SignTx(tx, txPool.signer, privateKey)
			err := txPool.addTx(tx, false)
			require.EqualError(t, err, ErrInvalidTimeout.Error())
		})
		t.Run("timeout", func(t *testing.T) {
			var timeout uint64 = types.DefTxMinTimeout
			tx := types.NewTransaction(fromAddr, to, big.NewInt(1), params.TxGas*2, 1, timeout, nil)
			types.SignTx(tx, txPool.signer, privateKey)
			_, err := VerifyTxBaseData(fromAddr, tx, time.Now().Add(time.Second*time.Duration(timeout)), nil, nil, nil)
			require.EqualError(t, err, ErrTxExpired.Error())
		})

	})
	t.Run("check free tx", func(t *testing.T) {
		tx := types.NewTransaction(fromAddr, common.StringToAddress("test"),
			big.NewInt(0), 0, 0, 0, nil)

		tx.SetSeed(<-ethash.ProofWork(tx, 1,
			new(big.Int).SetUint64(txPool.chainconfig.Get(params.ConfigParamsMinFreePoW)),
			make(chan struct{})))
		types.SignTx(tx, txPool.signer, privateKey)
		err := txPool.addTx(tx, false)

		require.NoError(t, err, "should be add successful")
		t.Run("invalid pow seed", func(t *testing.T) {
			tx.SetSeed(123456789)
			types.SignTx(tx, txPool.signer, privateKey)
			err := txPool.addTx(tx, false)
			require.Error(t, err, ErrInvalidSeed.Error())
		})

		//feeTxHash := tx.Hash()
		feeTx := tx

		t.Run("new free tx", func(t *testing.T) {
			tx := types.NewTransaction(fromAddr, common.StringToAddress("test2"),
				big.NewInt(0), params.MinGasLimit*2, 0, 0, nil)
			tx.SetSeed(<-ethash.ProofWork(tx, 1,
				new(big.Int).SetUint64(txPool.chainconfig.Get(params.ConfigParamsMinFreePoW)),
				make(chan struct{})))
			types.SignTx(tx, txPool.signer, privateKey)
			err := txPool.addTx(tx, false)
			require.Error(t, err, "should add failed")

			//移除交易后可以继续添加
			t.Run("afterRemove", func(t *testing.T) {
				txPool.Remove(feeTx.Hash())
				err := txPool.addTx(tx, false)
				require.NoError(t, err)
			})
		})

	})
}
func insertActionInNewHeader(txPool *TxPool, tx *types.Transaction) {
	mc, action, _ := txPool.dagChain.dag.GetTxFirstAction(tx)
	state, _ := txPool.dagChain.GetChainTailState(mc)

	SetTxActionStateInChain(state, mc, tx.Hash(), action)
	root, _ := state.CommitTo(txPool.dagChain.chainDB) //提交到DB
	//修改header
	header := txPool.dagChain.GetChainTailHead(mc)
	header.StateRoot = root
	header.Number++
	header.MC = mc

	batch := txPool.dagChain.chainDB.NewBatch()
	defer batch.Discard()

	rawdb.WriteHeader(batch, header)
	WriteStableHash(txPool.dagChain.chainDB, header.Hash(), header.MC, header.Number)
	WriteChainTailHash(txPool.dagChain.chainDB, header.Hash(), header.MC)
	batch.Write()
}

func TestTxPool_Stored(t *testing.T) {
	txf := os.TempDir()

	cfg := DefaultTxPoolConfig
	cfg.NoDiskTx = false
	cfg.DiskTxPath = txf
	txpool, key, addr := setupTxPool(t, cfg)
	defer func() {
		txpool.Stop()
		os.Remove(txf)
	}()

	tx, err := types.SignTx(types.NewTransaction(addr, addr, big.NewInt(100), 0x9999, 0x1, 0, nil),
		testSinger, key)
	if err != nil {
		t.Fatalf("expect nil, got %v", err)
	}
	err = txpool.AddLocal(tx)
	require.NoError(t, err)

	txHash := tx.Hash()
	got := txpool.GetTxWithDisk(txHash)
	require.NotNil(t, got)
	require.Equal(t, txHash, got.Hash())

	t.Run("load from cached", func(t *testing.T) {
		txpool.Stop()
		txpool2, _, _ := setupTxPool(t, cfg)
		defer txpool2.Stop()
		time.Sleep(2 * time.Second)

		got := txpool2.GetTxWithDisk(txHash)
		require.NotNil(t, got)
		require.Equal(t, txHash, got.Hash())
	})
}

func BenchmarkAddTx(b *testing.B) {
	//关闭日志
	logh := log.Root().GetHandler()
	if gh, ok := logh.(*log.GlogHandler); ok {
		gh.Vmodule("")
		gh.Verbosity(log.LvlCrit)
	}

	cfg := DefaultTxPoolConfig
	cfg.NoDiskTx = true
	txPool, privateKey, addr := setupTxPool(b, cfg)

	ch := make(chan *types.Transaction, b.N)
	to := common.StringToAddress("touser")
	for i := 0; i < b.N; i++ {
		tx, err := types.SignTx(
			types.NewTransaction(addr, to, big.NewInt(1), params.TxGas*2, 1, 0, new(big.Int).SetInt64(int64(i)).Bytes()),
			testSinger, privateKey)

		require.NoError(b, err)
		ch <- tx
	}

	var wg sync.WaitGroup
	wg.Add(b.N)
	b.ResetTimer()
	for i := 0; i < 25; i++ {
		go func() {
			for tx := range ch {
				err := txPool.addTx(tx, false)
				if err != nil {
					b.Log(err)
				}
				wg.Done()
			}
		}()
	}
	wg.Wait()
}

func TestAddRemoveTx(t *testing.T) {
	//关闭日志
	//logh := log.Root().GetHandler()
	//if gh, ok := logh.(*log.GlogHandler); ok {
	//	gh.Vmodule("")
	//	gh.Verbosity(log.LvlCrit)
	//}

	cfg := DefaultTxPoolConfig
	cfg.NoDiskTx = true
	txPool, privateKey, addr := setupTxPool(t, cfg)

	count := 10
	ch := make(chan *types.Transaction, count)
	to := common.StringToAddress("touser")
	for i := 0; i < count; i++ {
		tx, err := types.SignTx(
			types.NewTransaction(addr, to, big.NewInt(1), params.TxGas*2, 1, 0, new(big.Int).SetInt64(int64(i)).Bytes()),
			testSinger, privateKey)

		require.NoError(t, err)
		ch <- tx
	}

	txaddc := make(chan types.TxPreEvent, count)
	sub := txPool.SubscribeTxPreEvent(txaddc)
	defer sub.Unsubscribe()

	var wg sync.WaitGroup
	wg.Add(count)
	for i := 0; i < 25; i++ {
		go func() {
			for tx := range ch {
				err := txPool.AddRemotes([]*types.Transaction{tx})
				if err != nil {
					t.Log(err)
				}
				wg.Done()
			}
		}()
	}
	wg.Wait()

	var added int
	//等待所有接收
	for {
		select {
		case <-txaddc:
			added++
			if added == count {
				time.Sleep(3 * time.Second)
				return
			}
		case <-time.After(5 * time.Second):
			t.Fatal("time out")
			return
		}
	}

}

func BenchmarkTxPool_AddRemote(b *testing.B) {
	log.Root().SetHandler(log.FuncHandler(func(r *log.Record) error {
		return nil
	}))

	cfg := DefaultTxPoolConfig
	cfg.NoDiskTx = true
	txPool, _, _ := setupTxPool(b, cfg)

	chains, count := 500, b.N/500
	if count == 0 {
		count = 1
	}
	var wg sync.WaitGroup
	wg.Add(chains * count)

	txPool.onAddDone = func() {
		wg.Done()
	}
	txPool.SetMiner(nil, func(from, to common.Address) bool {
		return true
	})

	txc, done, err := createTxs(context.TODO(), chains, count)
	require.NoError(b, err)

	<-done
	b.ResetTimer()

	for i := 0; i < 10; i++ {
		go func() {
			for tx := range txc {
				txPool.addTx(tx, false)
			}
		}()
	}
	wg.Wait()

}

type AccountInfo struct {
	Address common.Address
	PKey    *ecdsa.PrivateKey
}

func initAccountKeys(count int) []*AccountInfo {
	list := make([]*AccountInfo, count)
	for i := 0; i < len(list); i++ {
		k, _ := crypto.GenerateKey()
		list[i] = &AccountInfo{
			Address: crypto.ForceParsePubKeyToAddress(k.PublicKey),
			PKey:    k,
		}
	}
	return list
}

func createTxs(ctx context.Context, froms int, txs int) (txcr <-chan *types.Transaction, done chan struct{}, err error) {
	accountInfos := initAccountKeys(froms)

	// 发送至发送给自己
	sumTxs := froms * txs

	// 取可用用户
	fromAccounts := list.New()
	for _, account := range accountInfos {

		if fromAccounts.Len() < froms {
			fromAccounts.PushBack(account)
		}
		if fromAccounts.Len() > froms {
			break
		}
	}

	txc := make(chan *types.Transaction, sumTxs)

	batch := batch.NewBatchExec(fromAccounts.Len())
	batch.Start()
	//计算出每个账户所需要发送的交易量，进行批量创建
	singleTxs := sumTxs / fromAccounts.Len()
	if singleTxs == 0 {
		singleTxs = 1
	}
	var sum int
	createDone := make(chan interface{}, sumTxs/3)

	signer := types.NewSigner(big.NewInt(viper.GetInt64("chain.chainID")))

	var wg sync.WaitGroup
	wg.Add(sumTxs)
	// 循环遍历素所有发送方，并发各创建N个笔交易
	for fromEle := fromAccounts.Front(); fromEle != nil; fromEle = fromEle.Next() {
		sends := singleTxs
		if sends+sum > sumTxs { //严格控制总发送量
			sends = sumTxs - sum
		}
		if sends == 0 { //不需要继续发送
			break
		}
		sum += sends //计加发送量
		from := fromEle.Value.(*AccountInfo)
		log.Info("create tx by account", "from", from.Address, "count", sends)
		batch.Push(func(gp interface{}, args ...interface{}) {
			from, pkey, sends := args[0].(common.Address), args[1].(*ecdsa.PrivateKey), args[2].(int)
			for i := 0; i < sends; i++ {
				select {
				case <-ctx.Done():
					return
				default:
				}
				//创建交易，为了防止交易重复，data加入些标识:i+1
				// 只发给自己
				tx := types.NewTransaction(from, from,
					big.NewInt(1), params.TxGas*2, 1, 0,
					big.NewInt(int64(i)).Bytes())

				_, err = types.SignTx(tx, signer, pkey)
				if err != nil {
					createDone <- err
					return
				}

				txc <- tx
				wg.Done()
			}

		}, from.Address, from.Address, from.PKey, sends)
	}

	done = make(chan struct{})
	//关闭batch
	go func() {
		wg.Wait()
		close(done)
		batch.Stop()
	}()
	return txc, done, nil
}

// 解决：用户更换见证人后系统见证人进行一些处理时无法成功校验问题
func TestFixDeug_AddFreeTxFailed(t *testing.T) {
	cfg := testTxPoolConfig
	cfg.NoDiskTx = true
	txPool, privateKey, fromAddr := setupTxPool(t, cfg)

	data, err := sc.MakeReplaceWitnessExecuteInput(nil)
	if err != nil {
		require.NoError(t, err)
	}

	tx := types.NewTransaction(fromAddr, params.SCAccount, big.NewInt(0), 0, 0, types.DefTxTimeout, data)

	types.SignBySignHelper(tx, txPool.signer, privateKey)

	err = txPool.addTx(tx, true)
	require.NoError(t, err)
}
