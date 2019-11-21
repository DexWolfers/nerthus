package statistics

import (
	"testing"

	"gitee.com/nerthus/nerthus/common/testutil"

	"math/big"

	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/ntsdb"
	"github.com/stretchr/testify/require"
)

func TestStatistic_AfterNewUnit(t *testing.T) {

	header := types.Header{
		MC:     common.StringToAddress("chain addr"),
		Number: 123,
	}
	txs := types.TxExecs{
		randTxExec(t),
		randTxExec(t),
		randTxExec(t),
		randTxExec(t),
	}

	unit := types.NewUnit(&header, txs)

	run(t, func(t *testing.T, file string, db ntsdb.RD) {

		ser, _ := New(nil, db)
		err := ser.afterNewUnit(unit)
		require.NoError(t, err)

		//查询数据，必须存在
		query := "select 1 from txs where addr=? and tx=?"

		var result int
		for _, tx := range txs {
			// from
			from, _ := tx.Tx.Sender()
			r := db.QueryRow(query, from.Hex(), tx.TxHash.Hex())
			require.NoError(t, r.Scan(&result))

			//to
			r = db.QueryRow(query, tx.Tx.To().Hex(), tx.TxHash.Hex())
			require.NoError(t, r.Scan(&result))

			//other
			for _, c := range tx.Result.Coinflows {
				r = db.QueryRow(query, c.To.Hex(), tx.TxHash.Hex())
				require.NoError(t, r.Scan(&result))
			}
		}
	})
}

func TestStatistic_GetTxs(t *testing.T) {
	header := types.Header{
		MC:     common.StringToAddress("chain addr"),
		Number: 123,
	}
	var txs types.TxExecs
	for i := 0; i < 4; i++ {
		txs = append(txs, randTxExec(t))
		time.Sleep(1 * time.Second) //为了叉开交易时间，存储时间为秒级
	}
	unit := types.NewUnit(&header, txs)

	run(t, func(t *testing.T, file string, db ntsdb.RD) {

		ser, err := New(nil, db)
		require.NoError(t, err)
		err = ser.afterNewUnit(unit)
		require.NoError(t, err)

		//不区分账户
		t.Run("all", func(t *testing.T) {
			sum, txHashs, err := ser.GetTxs(types.TransactionTypeAll, 0, 100)
			require.NoError(t, err)
			require.Equal(t, sum, len(txs))
			require.Len(t, txHashs, len(txs))
		})

		//按账户查
		t.Run("by addr", func(t *testing.T) {
			sum, txHashs, err := ser.GetTxs(types.TransactionTypeAll, 0, 100, txs[0].Tx.To(), txs[1].Tx.To())
			require.NoError(t, err)
			require.Equal(t, sum, 2)
			require.Len(t, txHashs, 2)
			require.Equal(t, txs[1].TxHash.Hex(), txHashs[0].Hex())
			require.Equal(t, txs[0].TxHash.Hex(), txHashs[1].Hex())
		})

		//直接分页
		t.Run("by page", func(t *testing.T) {
			//倒序数据
			for i := 0; i < len(txs); i++ {
				sum, txHashs, err := ser.GetTxs(types.TransactionTypeAll, uint64(i), 1)
				require.NoError(t, err)
				require.Equal(t, sum, len(txs))
				require.Len(t, txHashs, 1)
				require.Equal(t, txs[len(txs)-1-i].TxHash.Hex(), txHashs[0].Hex(), "at page %d", i)
			}
		})
		t.Run("by type", func(t *testing.T) {
			sum, txHashs, err := ser.GetTxs(types.TransactionTypeTransfer, 1, 2)
			require.NoError(t, err)
			require.Equal(t, sum, len(txs))
			require.Len(t, txHashs, 2)
		})

	})
}

func TestQueryTx(t *testing.T) {
	header := types.Header{
		MC:     common.StringToAddress("chain addr"),
		Number: 123,
	}
	txs := types.TxExecs{
		randTxExec(t),
		randTxExec(t),
		randTxExec(t),
		randTxExec(t),
	}

	unit := types.NewUnit(&header, txs)

	run(t, func(t *testing.T, file string, db ntsdb.RD) {
		ser, err := New(nil, db)
		require.NoError(t, err)
		err = ser.afterNewUnit(unit)
		require.NoError(t, err)

		for _, tx := range txs {
			user, _ := tx.Tx.Sender()
			list, err := ser.QueryTxs(user, tx.Tx.Time2())
			require.NoError(t, err)
			require.Contains(t, list, tx.TxHash)
		}
	})
}

func randTxExec(t *testing.T) *types.TransactionExec {

	k1, err := crypto.GenerateKey()
	require.NoError(t, err)
	k2, err := crypto.GenerateKey()
	require.NoError(t, err)

	a1 := crypto.ForceParsePubKeyToAddress(k1.PublicKey)
	a2 := crypto.ForceParsePubKeyToAddress(k2.PublicKey)

	tx := types.NewTransaction(a1, a2, big.NewInt(100), 100, 100, 0, nil)
	types.SignTx(tx, types.NewSigner(big.NewInt(1)), k1)

	exec := types.TransactionExec{
		Tx:     tx,
		TxHash: tx.Hash(),
		Result: &types.TxResult{
			GasUsed: 100,
		},
	}
	//add result
	exec.Result.Coinflows = append(exec.Result.Coinflows, types.Coinflow{
		To:     a1,
		Amount: big.NewInt(100),
	})
	exec.Result.Coinflows = append(exec.Result.Coinflows, types.Coinflow{
		To:     a2,
		Amount: big.NewInt(200),
	})
	exec.Result.Coinflows = append(exec.Result.Coinflows, types.Coinflow{
		To:     common.StringToAddress("other"),
		Amount: big.NewInt(300),
	})
	return &exec
}

func TestStatistic_MoreTx(t *testing.T) {

	testutil.SetupTestConfig()

	k1, err := crypto.GenerateKey()
	require.NoError(t, err)
	k2, err := crypto.GenerateKey()
	require.NoError(t, err)
	a1 := crypto.ForceParsePubKeyToAddress(k1.PublicKey)
	a2 := crypto.ForceParsePubKeyToAddress(k2.PublicKey)

	run(t, func(t *testing.T, file string, db ntsdb.RD) {

		ser, _ := New(nil, db)
		ser.Start()
		defer ser.Stop()

		//发送5000*200 交易
		count := 5000 * 200

		for i := 0; i < count; i++ {
			tx := types.NewTransaction(a1, a2, big.NewInt(100), 100, 100, 0, big.NewInt(int64(i)).Bytes())

			ser.AfterNewTx(false, tx)
		}

		timeout := time.NewTimer(10 * time.Second)
		tk := time.NewTicker(1 * time.Second)

		defer func() {
			sum, _, err := ser.GetTxs(types.TransactionTypeAll, 0, 1)
			require.NoError(t, err)
			require.Equal(t, count, sum)
		}()
		for {
			select {
			case <-tk.C:
				if len(ser.jobc) == 0 {
					t.Log("done")
					return
				}
			case <-timeout.C:
				t.Fatal("time out")
				return
			}
		}

	})

}
