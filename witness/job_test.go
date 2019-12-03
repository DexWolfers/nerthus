package witness

import (
	"fmt"
	"math/big"
	rand2 "math/rand"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common/bytefmt"

	"github.com/stretchr/testify/require"

	"gitee.com/nerthus/nerthus/core/types"

	"gitee.com/nerthus/nerthus/common"
)

func PrintMemUsage(key string) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Print(key)
	// For info on each, see: https://golang.org/pkg/runtime/#MemStats
	fmt.Printf("\tAlloc = %v", bytefmt.ByteSize(m.Alloc))
	fmt.Printf("\t\tTotalAlloc = %v", bytefmt.ByteSize(m.TotalAlloc))
	fmt.Printf("\tSys = %v", bytefmt.ByteSize(m.Sys))
	fmt.Printf("\tNumGC = %v\n", m.NumGC)
}

func TestChainJobCenter_Memory(t *testing.T) {
	t.Skip("just for get memory")

	tx := types.NewTransaction(common.StringToAddress("from"), common.StringToAddress("to"), big.NewInt(1), 1, 1, 100, nil)
	_ = tx.Hash()

	jq := NewMiningJobCenter(nil, 0, nil)
	PrintMemUsage("before")

	chains := 5000

	for i := 0; i < 10000000; i++ {
		jq.Push(common.StringToAddress(strconv.Itoa(i%chains)), tx, types.ActionTransferPayment, types.UnitID{})
		if i > 0 && i%10000 == 0 {
			PrintMemUsage(strconv.Itoa(i))
		}
	}
	jq = nil
	PrintMemUsage("pushed")
	runtime.GC()
	PrintMemUsage("afterGC")
}

func BenchmarkJobQueue_Put(b *testing.B) {
	tx := types.NewTransaction(common.StringToAddress("from"), common.StringToAddress("to"), big.NewInt(1), 1, 1, 100, nil)
	_ = tx.Hash()

	run := func(n int, needPop bool, b *testing.B) {
		jq := NewMiningJobCenter(nil, 0, nil)

		chains := make([]common.Address, n)
		for i := 0; i < len(chains); i++ {
			chains[i] = common.StringToAddress(strconv.Itoa(i))
		}

		if needPop {
			for _, c := range chains {
				go func(c common.Address) {
					q := jq.LoadOrStore(c)
					for {
						v := q.Pop()
						if v == nil {
							time.Sleep(time.Second)
						}
					}
				}(c)
			}
		}

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var index int
			for pb.Next() {
				jq.Push(chains[index%len(chains)], tx, types.ActionTransferPayment, types.UnitID{})
				index++
			}
		})
	}

	b.Run("onlyPut", func(b *testing.B) {
		for i := 1; i < 10; i++ {
			count := i * 500
			b.Run(strconv.Itoa(count), func(b *testing.B) {
				run(count, false, b)
			})
		}
	})

	b.Run("withPop", func(b *testing.B) {
		for i := 1; i < 10; i++ {
			count := i * 500
			b.Run(strconv.Itoa(count), func(b *testing.B) {
				run(count, true, b)
			})
		}
	})

}

func TestChainJobCenter_Len(t *testing.T) {
	tx := types.NewTransaction(common.StringToAddress("from"), common.StringToAddress("to"), big.NewInt(1), 1, 1, 100, nil)
	_ = tx.Hash()
	chain := common.StringToAddress("chainA")

	jq := NewMiningJobCenter(nil, 0, nil)
	require.Zero(t, jq.Txs())

	jq.Push(chain, tx, types.ActionTransferPayment, types.UnitID{})

	require.Equal(t, int64(1), jq.Txs())
	require.Equal(t, int64(1), jq.ChainTxs(chain), "the chain queue size should be 1")

	// 取数据
	msg := jq.Load(chain).Pop()
	require.NotNil(t, msg)
	require.Equal(t, chain.Hex(), msg.MC.Hex())

	// 取完数据后，便是空
	require.Zero(t, int64(0), jq.ChainTxs(chain))
	require.Zero(t, jq.Txs())
}

func TestChainJobQueue_Notifyc(t *testing.T) {

	chain := common.StringToAddress("chainA")

	jq := NewMiningJobCenter(nil, 0, nil)

	count := 100
	for i := 0; i < count; i++ {
		tx := types.NewTransaction(common.StringToAddress("from"),
			common.StringToAddress("to"),
			big.NewInt(int64(i)), 1, 1, 100, nil)
		err := jq.Push(chain, tx, types.ActionTransferPayment, types.UnitID{})
		require.NoError(t, err)
	}

	require.Equal(t, int64(count), jq.LoadOrStore(chain).Len())
	var wg sync.WaitGroup
	wg.Add(count)

	// 能够根据信号获取全部
	go func() {
		q := jq.LoadOrStore(chain)
		for {
			if q.Pop() != nil {
				wg.Done()
			} else {
				break
			}
		}
	}()
	wg.Wait()
}

func TestChainJobCenter_Stop(t *testing.T) {
	jq := NewMiningJobCenter(nil, 0, nil)
	jq.Stop()
}

func TestMiningJobCenter_LoadOrStore(t *testing.T) {
	tx := types.NewTransaction(common.StringToAddress("from"), common.StringToAddress("to"), big.NewInt(1), 1, 1, 100, nil)
	_ = tx.Hash()

	jq := NewMiningJobCenter(nil, 0, nil)

	chain := common.ForceDecodeAddress("nts10mexyv9s44xlcs0canycqmxgzpjeawrjynmdql")
	jq.Push(chain, tx, types.ActionTransferPayment, types.UnitID{})

	chainQ := jq.LoadOrStore(chain)
	require.Equal(t, int64(1), chainQ.Len())

	v := chainQ.Pop()
	require.NotNil(t, v)
	require.Equal(t, tx.Hash().Hex(), v.TXHash.Hex())

}

func TestMiningJobCenter_ClearChainForce(t *testing.T) {
	tx := types.NewTransaction(common.StringToAddress("from"), common.StringToAddress("to"), big.NewInt(1), 1, 1, 100, nil)
	_ = tx.Hash()

	jq := NewMiningJobCenter(nil, 0, nil)

	chain := common.ForceDecodeAddress("nts10mexyv9s44xlcs0canycqmxgzpjeawrjynmdql")
	jq.Push(chain, tx, types.ActionTransferPayment, types.UnitID{})

	jq.ClearChainForce(chain)

	require.Nil(t, jq.Load(chain), "should be cleared")
}

func TestMiningJobCenter_ChainsPer(t *testing.T) {
	jq := NewMiningJobCenter(nil, 0, nil)

	txs := make(map[common.Address]*types.Transaction)

	//每条链一笔交易
	for i := 0; i < 10; i++ {
		price := rand2.Intn(100)

		tx := types.NewTransaction(
			common.StringToAddress(fmt.Sprintf("from_%d", i)),
			common.StringToAddress("to"), big.NewInt(1), 1, uint64(price), 100, nil)

		txs[tx.SenderAddr()] = tx
		jq.Push(tx.SenderAddr(), tx, types.ActionTransferPayment, types.UnitID{})
	}
	require.Equal(t, len(txs), int(jq.Txs())) //应该有 10 笔交易

	//获取的顺序应该是从价高到价低
	var gotOrder []common.Address
	jq.RangeChain(func(chain common.Address) bool {
		gotOrder = append(gotOrder, chain)
		t.Log(chain, txs[chain].GasPrice())
		return true
	})
	//检查顺序
	require.Len(t, gotOrder, len(txs))
	var preTx *types.Transaction
	for _, c := range gotOrder {
		tx := txs[c]

		if preTx == nil {
			preTx = tx
			continue
		}
		require.True(t, preTx.GasPrice() >= tx.GasPrice(), "should be pre tx %d > curr tx %d", preTx.GasPrice(), tx.GasPrice())

		if preTx.GasPrice() == tx.GasPrice() {
			require.True(t, preTx.Time() <= tx.Time())
		}

		preTx = tx
	}
}

//BenchmarkMiningJobCenter_Push-4   	  200000	      6358 ns/op
func BenchmarkMiningJobCenter_Push(b *testing.B) {
	jq := NewMiningJobCenter(nil, 0, nil)

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			price := rand2.Intn(100)

			tx := types.NewTransaction(
				common.StringToAddress(fmt.Sprintf("from_%d", rand2.Intn(2000))),
				common.StringToAddress("to"), big.NewInt(1), 1, uint64(price), 100, nil)

			jq.Push(tx.SenderAddr(), tx, types.ActionTransferPayment, types.UnitID{})
		}
	})
}
