package core

import (
	"math/big"
	"testing"

	"gitee.com/nerthus/nerthus/core/types"
	"github.com/stretchr/testify/require"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/state"

	"gitee.com/nerthus/nerthus/ntsdb"
)

func TestChainCache(t *testing.T) {

	db, _ := ntsdb.NewMemDatabase()
	sdb := state.NewDatabase(db)
	stataDB, err := state.New([]byte{1, 2, 3}, common.Hash{}, sdb)
	require.NoError(t, err)

	chain := common.StringToAddress("chainA")
	header := types.Header{
		MC:       chain,
		Number:   1000,
		SCHash:   common.StringToHash("---"),
		SCNumber: 3,
	}

	balance := big.NewInt(100000000)
	stataDB.SetBalance(chain, balance)
	header.StateRoot, err = stataDB.CommitTo(db)
	require.NoError(t, err)

	cache, err := NewChainCache(chain, sdb, header.Hash(), types.NewUnit(&header, nil))
	require.NoError(t, err)

	// 检查基本输出
	t.Run("checkValue", func(t *testing.T) {
		require.Equal(t, balance.Uint64(), cache.Balance())
		require.Equal(t, header.Hash(), cache.TailHash())
		require.Equal(t, header.Number, cache.TailHead().Number)
		require.Equal(t, header.Hash(), cache.TailHead().Hash())
	})
	// 检查外部修改对内部的影响
	t.Run("change", func(t *testing.T) {
		t.Run("uhash", func(t *testing.T) {
			//修改哈希
			uhash := cache.TailHash()
			uhash[0] = 0
			require.Equal(t, header.Hash(), cache.TailHash())
		})
		t.Run("head", func(t *testing.T) {
			h := cache.TailHead()
			h.Number++

			require.NotEqual(t, h.Number, cache.TailHead().Number)
			require.Equal(t, header.Number, cache.TailHead().Number)

		})
	})
}
