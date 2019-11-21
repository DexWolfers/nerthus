package core

import (
	"gitee.com/nerthus/nerthus/core/rawdb"
	"testing"

	"math/big"

	"gitee.com/nerthus/nerthus/core/vm"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/ntsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDAG_GetTxNextAction(t *testing.T) {
	db, _ := ntsdb.NewMemDatabase()
	gspec, err := LoadGenesisConfig()
	require.NoError(t, err)
	gspec.MustCommit(db)

	dagchain, _ := NewDagChain(db, gspec.Config, nil, vm.Config{})
	defer dagchain.Stop()
	dag := dagchain.dag

	signer := types.NewSigner(gspec.Config.ChainId)
	// unit 中将包含不同场景的交易，更加交易结果获取正确位置

	key, from := defaultTestKey()
	signTx := func(tx *types.Transaction) *types.Transaction {
		types.SignTx(tx, signer, key)
		return tx
	}

	parent := types.Header{MC: from, Number: 1}
	header := types.Header{
		Number:     1,
		MC:         from,
		ParentHash: parent.Hash(),
		SCHash:     parent.Hash(),
	}
	//save parent
	batch := db.NewBatch()
	rawdb.WriteHeader(batch, &parent)
	require.NoError(t, batch.Write())

	to1 := common.StringToAddress("to1")
	t.Run("payment", func(t *testing.T) {
		t.Run("status zero", func(t *testing.T) {
			t.Run("to other", func(t *testing.T) {
				tx := signTx(types.NewTransaction(from, to1, big.NewInt(1), 1, 1, 0, nil))

				txs := make([]*types.TransactionExec, 1)
				txs[0] = &types.TransactionExec{ //转账
					Action: types.ActionTransferPayment,
					Tx:     tx,
					TxHash: tx.Hash(),
				}
				r := types.NewReceipt(txs[0].Action, common.Hash{}, false, big.NewInt(100))
				r.TxHash = tx.Hash()

				unit := types.NewUnit(&header, txs)
				err := WriteUnit(db, unit, types.Receipts{r})
				require.NoError(t, err)
				actions, err := dag.GetTxNextAction(tx.Hash(), tx)
				if assert.NoError(t, err) {
					assert.Len(t, actions, 1, "the next action should be %s", types.ActionTransferReceipt)
					assert.Equal(t, types.ActionTransferReceipt, actions[0].Action, "the next action should be %s", types.ActionTransferReceipt)
					assert.Equal(t, to1, actions[0].MC, "the next action should be on mc %s", to1.Hex())
				}
			})

			t.Run("to self", func(t *testing.T) {
				tx := signTx(types.NewTransaction(from, from, big.NewInt(1), 1, 1, 0, nil))

				txs := make([]*types.TransactionExec, 1)
				txs[0] = &types.TransactionExec{ //转账
					Action: types.ActionTransferPayment,
					Tx:     tx,
					TxHash: tx.Hash(),
				}
				r := types.NewReceipt(txs[0].Action, common.Hash{}, false, big.NewInt(100))
				r.TxHash = tx.Hash()

				unit := types.NewUnit(&header, txs)
				err := WriteUnit(db, unit, types.Receipts{r})
				require.NoError(t, err)
				actions, err := dag.GetTxNextAction(tx.Hash(), tx)
				if assert.NoError(t, err) {
					assert.Len(t, actions, 0, "the action should be end if payment to yourself")
				}
			})

		})
		t.Run("status have one", func(t *testing.T) {
			t.Run("to other", func(t *testing.T) {
				tx := signTx(types.NewTransaction(from, to1, big.NewInt(1), 1, 1, 0, nil))

				txs := make([]*types.TransactionExec, 1)
				txs[0] = &types.TransactionExec{ //转账
					Action: types.ActionTransferReceipt,
					Tx:     tx,
					TxHash: tx.Hash(),
				}
				r := types.NewReceipt(txs[0].Action, common.Hash{}, false, big.NewInt(100))
				r.TxHash = tx.Hash()

				unit := types.NewUnit(&header, txs)
				err := WriteUnit(db, unit, types.Receipts{r})
				require.NoError(t, err)
				actions, err := dag.GetTxNextAction(tx.Hash(), tx)
				if assert.NoError(t, err) {
					assert.Len(t, actions, 0, "the action should be end if payment done")
				}

			})

			t.Run("failed", func(t *testing.T) {
				tx := signTx(types.NewTransaction(from,
					from, big.NewInt(1), 1, 1, 0, nil))

				txs := make([]*types.TransactionExec, 1)
				txs[0] = &types.TransactionExec{
					Action: types.ActionTransferPayment,
					Tx:     tx,
					TxHash: tx.Hash(),
				}
				r := types.NewReceipt(txs[0].Action, common.Hash{}, false, big.NewInt(100))
				r.TxHash = tx.Hash()

				unit := types.NewUnit(&header, txs)
				err := WriteUnit(db, unit, types.Receipts{r})
				require.NoError(t, err)
				actions, err := dag.GetTxNextAction(tx.Hash(), tx)
				if assert.NoError(t, err) {
					assert.Len(t, actions, 0, "the action should be end if payment failed")
				}
			})

		})
	})

	//TODO(ysqi): 测试合约部分
}
