package txpool

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/ntsdb"
	"github.com/stretchr/testify/require"
)

func newDB(t testing.TB) (db *ntsdb.LDBDatabase, close func()) {
	name, err := ioutil.TempDir("", "ntsttest")
	require.NoError(t, err)
	db, err = ntsdb.NewLDBDatabase(name, 16, 16)
	require.NoError(t, err)

	return db, func() {
		db.Close()
		os.Remove(name)
	}
}

func TestKey(t *testing.T) {
	t.Log(keyLen)

	msg := ExecMsg{
		Chain: common.StringToAddress("chain1"), TxHash: common.StringToHash("tx1"), Action: types.ActionContractFreeze,
	}

	key := encodeKey(msg)
	got := decodeKey(key)
	require.Equal(t, msg, got)
	t.Log(len(key))
}

func TestTODOPool_Push(t *testing.T) {
	db, close := newDB(t)
	defer close()

	pool := NewTODOPool(db, false)
	pool.Start()
	defer pool.Stop()

	msgs := []ExecMsg{
		{Chain: common.StringToAddress("chain1"), TxHash: common.StringToHash("tx1"), Action: types.ActionTransferPayment},
		{Chain: common.StringToAddress("chain2"), TxHash: common.StringToHash("tx1"), Action: types.ActionTransferReceipt},
		{Chain: common.StringToAddress("chain2"), TxHash: common.StringToHash("tx2"), Action: types.ActionContractFreeze},
		{Chain: common.StringToAddress("chain3"), TxHash: common.StringToHash("tx2"), Action: types.ActionContractDeal},
	}
	byChain := make(map[common.Address][]ExecMsg)

	var wg sync.WaitGroup
	wg.Add(len(msgs))
	onWriteTest = func(count int) {
		for i := 0; i < count; i++ {
			wg.Done()
		}
	}

	for _, m := range msgs {
		pool.Push(m.Chain, m.TxHash, nil, m.Action)
		byChain[m.Chain] = append(byChain[m.Chain], m)
	}

	//等待写入完毕
	wg.Wait()

	//写入杂草
	db.Put([]byte("info"), []byte{1, 23})

	t.Run("rangeByChain", func(t *testing.T) {
		for k, list := range byChain {
			var got []ExecMsg
			pool.RangeChain(k, func(msg ExecMsg) bool {
				got = append(got, msg)
				require.Contains(t, list, msg)

				return true
			})
			require.Len(t, got, len(list))

		}
	})
	t.Run("rangeAll", func(t *testing.T) {

		var count int
		pool.Range(func(msg ExecMsg) bool {
			count++
			require.Contains(t, msgs, msg)
			return true
		})
		require.Equal(t, len(msgs), count)
	})

	t.Run("del", func(t *testing.T) {
		wg.Add(len(msgs))
		onWriteTest = func(count int) {
			for i := 0; i < count; i++ {
				wg.Done()
			}
		}
		for _, m := range msgs {
			pool.Del(m.Chain, m.TxHash, m.Action)
		}
		wg.Wait()

		var count int
		pool.Range(func(msg ExecMsg) bool {
			count++
			return true
		})
		require.Zero(t, count)
	})
	t.Run("delInRange", func(t *testing.T) {
		onWriteTest = nil

		err := pool.Range(func(msg ExecMsg) bool {
			return true
		})
		require.NoError(t, err)
	})
}

//BenchmarkTODOPool_PushAndDel-4   	  100000	     15146 ns/op
func BenchmarkTODOPool_PushAndDel(b *testing.B) {
	db, close := newDB(b)
	defer close()

	pool := NewTODOPool(db, false)
	pool.Start()
	defer pool.Stop()

	var wg sync.WaitGroup
	onWriteTest = func(count int) {
		for i := 0; i < count; i++ {
			wg.Done()
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		chain := common.StringToAddress("chain_" + strconv.Itoa(i%10000))
		tx := common.StringToHash("tx_" + strconv.Itoa(i%100000))
		wg.Add(2)
		pool.Push(chain, tx, nil, types.ActionContractDeal)
		pool.Del(chain, tx, types.ActionContractDeal)
	}
	wg.Wait()
}

func TestBatchPushAndDelete(t *testing.T) {
	db, close := newDB(t)
	defer close()

	pool := NewTODOPool(db, false)
	pool.Start()
	defer pool.Stop()

	var stop bool
	time.AfterFunc(time.Minute*20, func() {
		stop = true
	})

	go func() {
		fmt.Println(http.ListenAndServe("127.0.0.1:8081", nil))
	}()

	var wg sync.WaitGroup
	var add int
	for !stop {
		chain := common.StringToAddress("chain_" + strconv.FormatUint(rand.Uint64(), 10))
		tx := common.StringToHash("tx_" + strconv.FormatUint(rand.Uint64(), 10))

		wg.Add(2)
		pool.Push(chain, tx, nil, types.ActionContractDeal)
		pool.Del(chain, tx, types.ActionContractDeal)
		add++
		if add%1000 == 0 {
			fmt.Printf("%d\n", add)
		}
	}
	fmt.Println("stoped: wait done")
	wg.Wait()
}
