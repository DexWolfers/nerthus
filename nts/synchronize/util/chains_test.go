package util

import (
	"fmt"
	"testing"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/rawdb"
	"gitee.com/nerthus/nerthus/params"

	"github.com/stretchr/testify/require"
)

func TestRangeChains(t *testing.T) {
	db, close := newDB(t)
	defer close()

	//系统链固定位置
	rawdb.WriteNewChain(db, 0, []common.Address{params.SCAccount})

	var chains []common.Address
	chains = append(chains, params.SCAccount)

	for i := 0; i < 1000; i++ {
		addr := common.StringToAddress(fmt.Sprintf("chain_%d", i))
		rawdb.WriteNewChain(db, uint64(i), []common.Address{addr})

		chains = append(chains, addr)
	}

	t.Run("range", func(t *testing.T) {
		var index int
		RangeChains(db, func(chain common.Address, status rawdb.ChainStatus) bool {
			require.Equal(t, chains[index], chain)
			index++
			return true
		})
		require.Equal(t, len(chains), index)
	})

	t.Run("get", func(t *testing.T) {
		for i, v := range chains {
			index, ok := GetChainIndex(db, v)
			require.True(t, ok)
			require.Equal(t, i, int(index))

			//重复依次检查
			index, ok = GetChainIndex(db, v)
			require.True(t, ok)
			require.Equal(t, i, int(index))
		}

	})

}
