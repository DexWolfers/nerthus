package rawdb

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/syndtr/goleveldb/leveldb/util"

	"gitee.com/nerthus/nerthus/common"
)

func TestSize(t *testing.T) {

	run := func(t *testing.T, n int) {
		before, after := getChainSize(t, n)
		t.Log(common.StorageSize(before), common.StorageSize(after))
	}

	t.Run("一亿地址", func(t *testing.T) {
		run(t, 100000000)
	})

}

func getChainSize(t *testing.T, n int) (int64, int64) {
	db, close := newDB(t)
	fmt.Println("path", db.Path())
	_ = close
	//defer close()

	for i := 0; i < n; i++ {
		addr := common.StringToAddress(fmt.Sprintf("one_address_%d", i))
		WriteNewChain(db, uint64(i), []common.Address{addr})
	}

	f, err := os.Lstat(db.Path())
	require.NoError(t, err)

	before := f.Size()

	err = db.LDB().CompactRange(util.Range{})
	require.NoError(t, err)

	f, err = os.Lstat(db.Path())
	require.NoError(t, err)
	after := f.Size()

	return before, after
}

func TestGetChainStatus(t *testing.T) {
	db, close := newDB(t)
	defer close()

	var chains []common.Address

	for i := 0; i < 11; i++ {
		chains = append(chains, common.StringToAddress(fmt.Sprintf("address_%d", i)))

		WriteNewChain(db, uint64(i*2), []common.Address{chains[i]})
	}

	t.Run("get", func(t *testing.T) {
		for i, v := range chains {
			status, ok := GetChainStatus(db, v)
			require.True(t, ok)

			require.Equal(t, status.ApplyNumber, uint64(i*2))
			require.Equal(t, status.Index, uint64(i))
			require.False(t, status.Active)
		}
	})
	t.Run("active", func(t *testing.T) {
		for i, v := range chains {
			SetChainActive(db, v)

			status, ok := GetChainStatus(db, v)
			require.True(t, ok)
			require.Equal(t, status.ApplyNumber, uint64(i*2))
			require.Equal(t, status.Index, uint64(i))
			require.True(t, status.Active)
		}
	})
	t.Run("range", func(t *testing.T) {

		var index int
		RangeChains(db, func(chain common.Address, status ChainStatus) bool {
			require.Equal(t, chains[index], chain)

			require.Equal(t, status.ApplyNumber, uint64(index*2))
			require.Equal(t, status.Index, uint64(index))
			require.True(t, status.Active)

			index++

			return true
		})
		require.Equal(t, len(chains), index)
	})

}

func TestBatchWriteNewChains(t *testing.T) {
	db, close := newDB(t)
	defer close()

	var chains []common.Address

	for i := 0; i < 11; i++ {
		chains = append(chains, common.StringToAddress(fmt.Sprintf("address_%d", i)))
	}

	WriteNewChain(db, 10, chains)
	require.Equal(t, GetChainCount(db), uint64(len(chains)))

	t.Run("get", func(t *testing.T) {
		for i, v := range chains {
			status, ok := GetChainStatus(db, v)
			require.True(t, ok)

			require.Equal(t, status.ApplyNumber, uint64(10))
			require.Equal(t, status.Index, uint64(i))
			require.False(t, status.Active)
		}
	})

	t.Run("again", func(t *testing.T) {
		var newChains []common.Address
		for i := 0; i < 11; i++ {
			addr := common.StringToAddress(fmt.Sprintf("chain_%d", i))
			chains = append(chains, addr)
			newChains = append(newChains, addr)
		}
		WriteNewChain(db, 10, newChains)
		require.Equal(t, GetChainCount(db), uint64(len(chains)))

		for i, v := range chains {
			status, ok := GetChainStatus(db, v)
			require.True(t, ok)

			require.Equal(t, status.ApplyNumber, uint64(10))
			require.Equal(t, status.Index, uint64(i))
			require.False(t, status.Active)
		}
	})
}
