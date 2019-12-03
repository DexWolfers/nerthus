package rwit

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitee.com/nerthus/nerthus/params"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/ntsdb"
)

func TestPendingChainSet(t *testing.T) {

	db := ntsdb.NewMemDB()
	set := newPendingChainSet(db)

	items := []PendingChain{
		{Chain: common.StringToAddress("c1"), SysUnitNumber: 100, SysUnitHash: common.StringToHash("uint100")},
		{Chain: common.StringToAddress("c2"), SysUnitNumber: 101, SysUnitHash: common.StringToHash("uint101")},
		{Chain: common.StringToAddress("c3"), SysUnitNumber: 102, SysUnitHash: common.StringToHash("uint102")},
		{Chain: common.StringToAddress("c4"), SysUnitNumber: 103, SysUnitHash: common.StringToHash("uint103")},
	}

	for _, v := range items {
		set.Add(v.Chain, types.UnitID{ChainID: params.SCAccount, Height: v.SysUnitNumber, Hash: v.SysUnitHash})
	}

	t.Run("get", func(t *testing.T) {
		for _, v := range items {
			got, ok := set.Get(v.Chain)
			require.True(t, ok)
			require.Equal(t, v, got)
		}
	})
	t.Run("range", func(t *testing.T) {
		var index int
		set.Range(func(info PendingChain) bool {
			require.Equal(t, items[index], info)
			index++
			return true
		})
		//遍历所有
		require.Equal(t, len(items), index)
	})

	t.Run("del", func(t *testing.T) {
		//随机删除
		chains := make(map[common.Address]bool, len(items))
		for _, v := range items {
			chains[v.Chain] = true
		}

		for chain := range chains {
			ok := set.Del(chain)
			require.True(t, ok)

			delete(chains, chain)
			//依旧可以遍历剩余
			var index int
			set.Range(func(got PendingChain) bool {
				require.Contains(t, chains, got.Chain)
				index++
				return true
			})
			require.Equal(t, len(chains), index)
		}

	})

}
