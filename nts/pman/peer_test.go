package pman

import (
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/nts/protocol"
	"gitee.com/nerthus/nerthus/p2p"
	"gitee.com/nerthus/nerthus/p2p/discover"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"github.com/stretchr/testify/require"
)

func TestClearExpiredTx(t *testing.T) {

	t.Run("clear", func(t *testing.T) {

		var txs types.Transactions

		timeout := 3
		var txHash common.Hash
		for i := 0; i < 10; i++ {
			timeoutV := uint64(timeout)
			if i == 5 {
				timeoutV = 1000
			}
			tx := types.NewTransaction(common.StringToAddress("a"),
				common.StringToAddress("b"), nil, 1, 1, timeoutV, nil)
			if i == 5 {
				txHash = tx.Hash()
			}
			txs = append(txs, tx)
		}

		time.Sleep(time.Second * 6 * time.Duration(timeout))

		txs = clearExpiredTx(txs, time.Now())
		require.Len(t, txs, 1)
		require.Equal(t, txHash, txs[0].Hash())
	})

	t.Run("all valid", func(t *testing.T) {
		tx := types.NewTransaction(common.StringToAddress("a"),
			common.StringToAddress("b"), nil, 1, 1, uint64(time.Hour*24/time.Second), nil)

		txs := clearExpiredTx([]*types.Transaction{tx}, time.Now())
		require.Len(t, txs, 1)
	})
}

func TestPeer_AsyncSendMessage(t *testing.T) {

	peer := NewPeerHandler(1, p2p.NewPeer(discover.EmptyNodeID, "test", nil), nil, DisableTx, nil)
	peer.Close(p2p.DiscProtocolError)
	require.NotPanics(t, func() {
		peer.AsyncSendNewBlock(nil)
		peer.AsyncSendMessage(protocol.BftMsg, nil)
		peer.AsyncSendNewBlockHash(nil)
		peer.AsyncSendTransactions(nil)
		peer.AsyncSendTTLMsg(protocol.TTLMsg{})
	})

}
