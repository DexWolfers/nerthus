package rawdb

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
)

func TestTransaction(t *testing.T) {
	db, close := newDB(t)
	defer close()

	var txs types.Transactions
	from := common.StringToAddress("from")
	to := common.StringToAddress("to")
	for i := uint64(1); i <= 10; i++ {
		tx := types.NewTransaction(from, to, big.NewInt(int64(i*2)), i*3, i*4, i*100, nil)
		txs = append(txs, tx)
	}

	for _, v := range txs {
		require.NotPanics(t, func() {
			WriteTransaction(db, v)
		})
	}

	t.Run("getTx", func(t *testing.T) {
		for _, tx := range txs {
			got := GetTx(db, tx.Hash())
			require.NotNil(t, got)
			require.Equal(t, tx.Hash(), got.Hash())
		}
	})

	t.Run("decodeTxs", func(t *testing.T) {
		list := make([]common.Hash, len(txs))

		alltxs := make(map[common.Hash]bool)
		for i, tx := range txs {
			list[i] = tx.Hash()
			alltxs[tx.Hash()] = false
		}

		raw := GetTxListRLP(db, list)

		txs, err := DecodeTxsRLP(raw)
		require.NoError(t, err)
		require.Len(t, txs, len(list))

		//检查所有
		for _, tx := range txs {
			require.Contains(t, alltxs, tx.Hash())
			//require.True(t, alltxs[tx.Hash()], "should be contains")
			require.False(t, alltxs[tx.Hash()], "should be not repeat")
			alltxs[tx.Hash()] = true
		}
	})

}
