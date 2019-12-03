// Copyright 2016 The go-ethereum Authors
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

package filters

import (
	"context"
	"math/big"
	"testing"
	"time"

	"errors"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/bloombits"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/rpc"
)

type testBackend struct {
	mux        *event.TypeMux
	db         ntsdb.Database
	sections   uint64
	txFeed     *event.Feed
	rmLogsFeed *event.Feed
	logsFeed   *event.Feed
	chainFeed  *event.Feed
}

func (b *testBackend) ChainDb() ntsdb.Database {
	return b.db
}

func (b *testBackend) EventMux() *event.TypeMux {
	return b.mux
}

func (b *testBackend) HeaderByNumber(ctx context.Context, chain common.Address, blockNr rpc.BlockNumber) (*types.Header, error) {
	//todo 无法处理 需账户地址
	//var hash common.Hash
	//var num uint64
	//if blockNr == rpc.LatestBlockNumber {
	//	hash = core.GetHeadBlockHash(b.db)
	//	num = core.GetBlockNumber(b.db, hash)
	//} else {
	//	num = uint64(blockNr)
	//	hash = core.GetCanonicalHash(b.db, num)
	//}
	//return core.GetHeader(b.db, hash, num), nil

	return nil, errors.New("need acc address")
}

func (b *testBackend) GetReceipts(ctx context.Context, unitHash common.Hash) (types.Receipts, error) {
	mcaddr, number := core.GetUnitNumber(b.db, unitHash)
	return core.GetUnitReceipts(b.db, mcaddr, unitHash, number), nil
}

func (b *testBackend) SubscribeTxPreEvent(ch chan<- types.TxPreEvent) event.Subscription {
	return b.txFeed.Subscribe(ch)
}

func (b *testBackend) SubscribeRemovedLogsEvent(ch chan<- types.RemovedLogsEvent) event.Subscription {
	return b.rmLogsFeed.Subscribe(ch)
}

func (b *testBackend) SubscribeLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return b.logsFeed.Subscribe(ch)
}

func (b *testBackend) SubscribeChainEvent(ch chan<- types.ChainEvent) event.Subscription {
	return b.chainFeed.Subscribe(ch)
}

func (b *testBackend) BloomStatus() (uint64, uint64) {
	return params.BloomBitsBlocks, b.sections
}

func (b *testBackend) ServiceFilter(ctx context.Context, session *bloombits.MatcherSession) {
	requests := make(chan chan *bloombits.Retrieval)

	go session.Multiplex(16, 0, requests)
	go func() {
		for {
			// Wait for a service request or a shutdown
			select {
			case <-ctx.Done():
				return

			case request := <-requests:
				task := <-request

				task.Bitsets = make([][]byte, len(task.Sections))
				//todo 暂时无法提供 获取单元的hash 需知 账户地址
				//for i, section := range task.Sections {
				//	if rand.Int()%4 != 0 { // Handle occasional missing deliveries
				//		head := core.GetCanonicalHash(b.db, (section+1)*params.BloomBitsBlocks-1)
				//		task.Bitsets[i], _ = core.GetBloomBits(b.db, task.Bit, section, head)
				//	}
				//}
				request <- task
			}
		}
	}()
}

// TestBlockSubscription tests if a block subscription returns block hashes for posted chain events.
// It creates multiple subscriptions:
// - one at the start and should receive all posted chain events and a second (blockHashes)
// - one that is created after a cutoff moment and uninstalled after a second cutoff moment (blockHashes[cutoff1:cutoff2])
// - one that is created after the second cutoff moment (blockHashes[cutoff2:])
func TestBlockSubscription(t *testing.T) {
	//t.Parallel()
	//
	//var (
	//	mux         = new(event.TypeMux)
	//	db, _       = ntsdb.NewMemDatabase()
	//	txFeed      = new(event.Feed)
	//	rmLogsFeed  = new(event.Feed)
	//	logsFeed    = new(event.Feed)
	//	chainFeed   = new(event.Feed)
	//	backend     = &testBackend{mux, db, 0, txFeed, rmLogsFeed, logsFeed, chainFeed}
	//	api         = NewPublicFilterAPI(backend, false)
	//	genesis     = core.DefaultTestnetGenesisUnit().MustCommit(db)
	//	devAccounts = params.GetDevAccounts()
	//	fromKey, _  = crypto.HexToECDSA(devAccounts[1].KeyHex)
	//	toKey, _    = crypto.HexToECDSA(devAccounts[3].KeyHex)
	//	toAddr      = crypto.PubkeyToAddress(toKey.PublicKey)
	//	//todo 可能有问题
	//	chain = core.GenerateChain(params.TestChainConfig, genesis, db, 10, func(i int, unitGen *core.UnitGen) {
	//		signer := types.NewSigner(params.TestChainConfig.ChainId)
	//		tx, _ := types.SignTx(types.NewTransaction(toAddr, big.NewInt(10000), big.NewInt(0).SetUint64(params.TxGas), nil), signer, fromKey)
	//		unitGen.SetTx(tx)
	//	})
	//	chainEvents = []types.ChainEvent{}
	//)
	//
	//for _, blk := range chain {
	//	chainEvents = append(chainEvents, types.ChainEvent{UnitHash: blk.Hash(), Unit: blk})
	//}
	//
	//chan0 := make(chan *types.Header)
	//sub0 := api.events.SubscribeNewHeads(chan0)
	//chan1 := make(chan *types.Header)
	//sub1 := api.events.SubscribeNewHeads(chan1)
	//
	//go func() { // simulate client
	//	i1, i2 := 0, 0
	//	for i1 != len(chainEvents) || i2 != len(chainEvents) {
	//		select {
	//		case header := <-chan0:
	//			if chainEvents[i1].UnitHash != header.Hash() {
	//				t.Errorf("sub0 received invalid hash on index %d, want %x, got %x", i1, chainEvents[i1].UnitHash, header.Hash())
	//			}
	//			i1++
	//		case header := <-chan1:
	//			if chainEvents[i2].UnitHash != header.Hash() {
	//				t.Errorf("sub1 received invalid hash on index %d, want %x, got %x", i2, chainEvents[i2].UnitHash, header.Hash())
	//			}
	//			i2++
	//		}
	//	}
	//
	//	sub0.Unsubscribe()
	//	sub1.Unsubscribe()
	//}()
	//
	//time.Sleep(1 * time.Second)
	//for _, e := range chainEvents {
	//	chainFeed.Send(e)
	//}
	//
	//<-sub0.Err()
	//<-sub1.Err()
}

// TestPendingTxFilter tests whether pending tx filters retrieve all pending transactions that are posted to the event mux.
func TestPendingTxFilter(t *testing.T) {
	t.Parallel()

	var (
		mux        = new(event.TypeMux)
		db, _      = ntsdb.NewMemDatabase()
		txFeed     = new(event.Feed)
		rmLogsFeed = new(event.Feed)
		logsFeed   = new(event.Feed)
		chainFeed  = new(event.Feed)
		backend    = &testBackend{mux, db, 0, txFeed, rmLogsFeed, logsFeed, chainFeed}
		api        = NewPublicFilterAPI(backend, false)

		transactions = []*types.Transaction{
			//types.NewTransaction(nil, common.HexToAddress("0xb794f5ea0ba39494ce83a213fffba74279579268"), new(big.Int), 0, new(big.Int), nil),
			types.NewTransaction(common.Address{}, params.SCAccount, new(big.Int), 0, 0, 0, nil),
			types.NewTransaction(common.Address{}, params.SCAccount, new(big.Int), 0, 0, 0, nil),
			types.NewTransaction(common.Address{}, params.SCAccount, new(big.Int), 0, 0, 0, nil),
			types.NewTransaction(common.Address{}, params.SCAccount, new(big.Int), 0, 0, 0, nil),
			types.NewTransaction(common.Address{}, params.SCAccount, new(big.Int), 0, 0, 0, nil),
		}

		hashes []common.Hash
	)

	fid0 := api.NewPendingTransactionFilter()

	time.Sleep(1 * time.Second)
	for _, tx := range transactions {
		ev := types.TxPreEvent{Tx: tx}
		txFeed.Send(ev)
	}

	timeout := time.Now().Add(1 * time.Second)
	for {
		results, err := api.GetFilterChanges(fid0)
		if err != nil {
			t.Fatalf("Unable to retrieve logs: %v", err)
		}

		h := results.([]common.Hash)
		hashes = append(hashes, h...)
		if len(hashes) >= len(transactions) {
			break
		}
		// check timeout
		if time.Now().After(timeout) {
			break
		}

		time.Sleep(100 * time.Millisecond)
	}

	if len(hashes) != len(transactions) {
		t.Errorf("invalid number of transactions, want %d transactions(s), got %d", len(transactions), len(hashes))
		return
	}
	for i := range hashes {
		if hashes[i] != transactions[i].Hash() {
			t.Errorf("hashes[%d] invalid, want %x, got %x", i, transactions[i].Hash(), hashes[i])
		}
	}
}
