package txpool

import (
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"runtime"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/ntsdb"
	"github.com/stretchr/testify/require"

	"sync"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/core/types"
)

func generateTxDiskStore(t *testing.T, interval, timeout time.Duration, countTx int) (TxDiskStore, func(...bool), string) {
	dir, err := ioutil.TempDir("/tmp", "tx_disk_store")
	if err != nil {
		t.Fatalf("expect nil, got %v", err)
	}
	db, err := ntsdb.NewLDBDatabase(dir, 0, 0)
	require.NoError(t, err)

	store, err := NewTxDiskStore(interval, timeout, db, nil)
	if err != nil {
		t.Fatalf("expect nil, got %v", err)
	}

	var txs = make(map[common.Hash]*types.Transaction)
	for i := 0; i < countTx; i++ {
		to := common.StringToAddress(fmt.Sprintf("%v", i))
		tx := types.NewTransaction(to, to, big.NewInt(int64(i)), uint64(i), uint64(i), 0, nil)
		txs[tx.Hash()] = tx
	}
	for _, tx := range txs {

		//t.Logf("tx Push, time: %v, hash %v", tx.Time(), tx.Hash().Hex())
		err := store.Put(true, tx)
		if err != nil {
			t.Fatalf("expect nil, got %v", err)
		}
	}
	return store, func(clear ...bool) {
		store.Close()
		db.Close()
		if len(clear) > 0 && clear[0] {
			if err := os.RemoveAll(dir); err != nil {
				t.Fatalf("expect nil, got %v", err)
			}
		}
	}, dir
}

func TestLeveldbTxLoadFromDisk(t *testing.T) {
	count := 10000
	_, clearFn, dir := generateTxDiskStore(t, 3*time.Second, 1000*time.Second, count)
	clearFn(false)

	var wg sync.WaitGroup
	wg.Add(count)

	db, err := ntsdb.NewLDBDatabase(dir, 0, 0)
	// reload tx data from disk
	store, err := NewTxDiskStore(time.Second*3, time.Second*1000, db, func(tx *types.Transaction) error {
		wg.Done()
		return nil
	})
	if err != nil {
		t.Fatalf("expect nil, got %v", err)
	}
	defer func() {
		store.Close()
		db.Close()
		os.RemoveAll(dir)
	}()
	wg.Wait()
	leveldbDiskStore := store.(*leveldbTxDiskStore)
	max := math.MaxUint64
	if len(*leveldbDiskStore.q) != count {
		t.Fatalf("expect 10000, got %v", len(*leveldbDiskStore.q))
	}

	for {
		tx := leveldbDiskStore.pop()
		if tx == nil {
			break
		}
		if max < tx.Time() {
			t.Fatalf("max(%v) should > tx.time(%v)", max, tx.Time())
		}
		max = tx.Time()
	}

	if len(*leveldbDiskStore.q) != 0 {
		t.Fatalf("expect 0, got %v", len(*leveldbDiskStore.q))
	}
}
func TestLeveldbTxDiskStore_Put(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())
	var (
		interval = 2 * time.Second
		timeout  = 5 * time.Second
		conutTx  = 100
	)
	txDiskStore, clearFn, _ := generateTxDiskStore(t, interval, timeout, conutTx)
	defer clearFn()
	if txDiskStore == nil {
		t.Fatalf("expect not nil, but it is nil")
	}

	i := 1029
	to := common.StringToAddress("9ikkk")
	expectTx := types.NewTransaction(to, to, big.NewInt(int64(i)), uint64(i), uint64(i), 0, nil)
	if err := txDiskStore.Put(true, expectTx); err != nil {
		t.Fatalf("expect nil, got %v", err)
	}

	{
		gotTx, err := txDiskStore.Get(expectTx.Hash())
		if err != nil {
			t.Fatalf("expect nil, got %v", err)
		}
		if gotTx.Hash() != expectTx.Hash() {
			t.Fatalf("expect %v, got %v", expectTx.Hash().Hex(), gotTx.Hash().Hex())
		}
	}
	// not timeout, it should be ok
	{
		time.Sleep(timeout - 2)
		gotTx, err := txDiskStore.Get(expectTx.Hash())
		if err != nil {
			t.Fatalf("expect nil, got %v", err)
		}
		if gotTx.Hash() != expectTx.Hash() {
			t.Fatalf("expect %v, got %v", expectTx.Hash().Hex(), gotTx.Hash().Hex())
		}
	}

	// timeout, it should be Not found
	{
		time.Sleep(timeout)
		_, err := txDiskStore.Get(expectTx.Hash())
		if err != ErrNotFoundTxDisk {
			t.Fatalf("expect %v, got %v", ErrNotFoundTxDisk, err)
		}
	}
}

func TestLeveldbTxDiskStore_Get(t *testing.T) {
	diskStore, clearFn, _ := generateTxDiskStore(t, 10*time.Second, 10*time.Second, 10)
	defer clearFn()
	leveldbDiskStore := diskStore.(*leveldbTxDiskStore)
	max := math.MaxUint64

	for {
		tx := leveldbDiskStore.pop()
		if tx == nil {
			break
		}
		if max < tx.Time() {
			t.Fatalf("max(%v) should > tx.time(%v)", max, tx.Time())
		}
		max = tx.Time()
	}

	if len(*leveldbDiskStore.q) != 0 {
		t.Fatalf("expect 0, got %v", len(*leveldbDiskStore.q))
	}
}
