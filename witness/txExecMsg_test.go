package witness

import (
	"math/big"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common/sort/sortslice"

	"github.com/stretchr/testify/require"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
)

func TestMsqSort(t *testing.T) {

	t.Run("actionLevel", func(t *testing.T) {

		from := common.StringToAddress("from")
		tx := types.NewTransaction(from,
			common.StringToAddress("to"),
			big.NewInt(1222), 100, 1, 0, nil)

		list := sortslice.NewList(100)

		for action, _ := range actionPriority {
			list.Add(newTask(types.TxAction(action), from, tx))
		}

		//高级别必须排在最前面
		var pre int
		list.SortRange(func(item sortslice.Item) bool {
			msg := item.(*TxExecMsg)
			if pre == 0 {
				pre = actionPriority[msg.Action]
				return true
			}

			cur := actionPriority[msg.Action]
			require.True(t, pre >= cur, "current Priority %d should be less than pre Priority %d", pre)
			return true
		})

	})

}

func TestMsgCompare(t *testing.T) {
	msg1 := &TxExecMsg{}
	msg2 := &TxExecMsg{}

	t.Run("default is equal", func(t *testing.T) {
		c := msg1.Compare(msg2)
		require.Equal(t, 0, c)
	})

	t.Run("action level", func(t *testing.T) {

		// 两两比较，级别越高优先级越高
		for action, level := range actionPriority {
			msg1.Action = types.TxAction(action)
			for action, level2 := range actionPriority {
				msg2.Action = types.TxAction(action)
				c := msg1.Compare(msg2)

				var want int
				if level > level2 {
					want = 1
				} else if level < level2 {
					want = -1
				} else {
					want = 0
				}
				require.Equal(t, want, c)
			}
		}
	})

	t.Run("same sender by tx none", func(t *testing.T) {
		createTxs := func(n int, to common.Address) []*types.Transaction {
			txs := make([]*types.Transaction, n)
			for i := range txs {
				tx := types.NewTransaction(to, to, big.NewInt(1222), 100, 1, 0, nil)
				txs[i] = tx
				time.Sleep(2)
			}
			return txs
		}
		// 同一用户的交易，按Time处理
		txs := createTxs(3, common.StringToAddress("to"))
		addr := common.StringToAddress("test address")
		var msgs []*TxExecMsg
		for _, t := range txs {
			msgs = append(msgs, newTask(types.ActionContractFreeze, addr, t))
		}
		// 两两比较，最早的交易Time最小，排在前面
		for i := 0; i < len(msgs); i++ {
			for j := 0; j < len(msgs); j++ {
				m1, m2 := msgs[i], msgs[j]
				var want int
				if i < j {
					want = 1
				} else if i > j {
					want = -1
				} else {
					want = 0
				}
				c := m1.Compare(m2)
				require.Equal(t, want, c)
			}
		}
	})

}

func TestGetMsgID(t *testing.T) {

	b1 := GetMsgID(common.StringToHash("tx1"), types.ActionTransferPayment)
	b2 := GetMsgID(common.StringToHash("tx1"), types.ActionTransferReceipt)
	b3 := GetMsgID(common.StringToHash("tx2"), types.ActionTransferReceipt)
	require.NotEqual(t, b1, b2)
	require.NotEqual(t, b1, b3)

}

//goos: darwin
//goarch: amd64
//pkg: gitee.com/nerthus/nerthus/witness
//now:  BenchmarkForamtID-4   	  300000000	             5.01 ns/op	           0 B/op	        0 allocs/op
//BenchmarkForamtID-4             200000000              75.3 ns/op            80 B/op          1 allocs/op
//BenchmarkForamtID_old-4         30000000               536 ns/op             304 B/op         9 allocs/op
func BenchmarkForamtID(b *testing.B) {
	a := types.ActionPowTxContractDeal
	tx := common.HexToHash("0x4c617374556e6974nts16p0x9arx4z5af244ykn49wth277r4fydvd78n7")
	b.ReportAllocs()
	b.ResetTimer()
	var size int
	for i := 0; i < b.N; i++ {
		b := GetMsgID(tx, a)
		size += len(b)
	}
}
