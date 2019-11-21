package core

import (
	"math/big"
	"testing"

	witvote "gitee.com/nerthus/nerthus/consensus/bft/backend"
	"gitee.com/nerthus/nerthus/ntsdb"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/rlp"
	"github.com/stretchr/testify/require"
)

var (
	workFeePrefixx     = []byte("wf_")
	workFeeBeginSuffix = []byte("_b")
	workFeeSumSuffix   = []byte("_s")
)
var dsc *ChainDataStatic

func newChainDsc() *ChainDataStatic {
	if dsc == nil {
		db, _ := ntsdb.NewMemDatabase()
		gspec, _ := LoadGenesisConfig()
		gspec.MustCommit(db)

		dagchain, _ := NewDagChain(db, gspec.Config, witvote.NewFaker(), vm.Config{})
		dsc = NewChainDataStatic(dagchain)
	}
	return dsc
}
func TestWriteWitnessFee(t *testing.T) {
	dc := newChainDsc()
	account := struct {
		privateKey string
		address    common.Address
	}{
		"01b177aa637bc701d0358da7022e492c12c50b44d225879f48ed6950eb1a956e",
		common.ForceDecodeAddress("nts10mexyv9s44xlcs0canycqmxgzpjeawrjynmdql"),
	}
	var (
		txCount    int64  = 3
		txGasPrice uint64 = 1
		txGasUsed  int64  = 100
		unitCount  int64  = 10
	)
	head := &types.Header{
		SCNumber: 1,
	}
	for j := 0; j < int(unitCount); j++ {
		txs := make([]*types.TransactionExec, 0)

		var receipts types.Receipts
		//构建交易
		for i := 0; i < int(txCount); i++ {
			tx := types.NewTransaction(account.address, account.address, big.NewInt(1), 1000, txGasPrice, 1000, nil)
			execTx := &types.TransactionExec{
				Action: types.ActionTransferPayment,
				TxHash: tx.Hash(),
				Tx:     tx,
			}
			txs = append(txs, execTx)

			r := types.NewReceipt(execTx.Action, common.Hash{}, false, big.NewInt(txGasUsed))
			r.GasUsed = uint64(txGasUsed)
			r.TxHash = tx.Hash()
			receipts = append(receipts, r)
		}
		unit := types.NewUnit(head, txs)
		//将军签名
		privKey, err := crypto.HexToECDSA(account.privateKey)
		require.NoError(t, err)
		types.SignBySignHelper(unit, types.NewSigner(big.NewInt(1280)), privKey)
		dc.unitStabled(types.ChainEvent{
			UnitHash: unit.Hash(),
			Unit:     unit,
			Receipts: receipts,
		})
	}

	//判断存储值
	fee := struct {
		Total    big.Int
		SumUnits big.Int
	}{}
	period := sc.GetSettlePeriod(head.SCNumber)
	// 计数从1开始
	begin := new(big.Int).SetUint64(period)
	key := sc.MakeKey(workFeePrefixx, account.address, begin, workFeeSumSuffix)

	data, _ := dc.dagchain.dag.db.Get(key)
	require.NotZero(t, len(data))

	err := rlp.DecodeBytes(data, &fee)
	require.NoError(t, err)

	//金额核对
	totalAmount := txCount * int64(txGasPrice) * txGasUsed * unitCount
	require.Equal(t, totalAmount, fee.Total.Int64())

	//单元个数核对
	require.Equal(t, unitCount, fee.SumUnits.Int64())
}
func TestGetWitnessFee(t *testing.T) {
	dc := newChainDsc()
	account := common.ForceDecodeAddress("nts10mexyv9s44xlcs0canycqmxgzpjeawrjynmdql")

	TestWriteWitnessFee(t)

	amount, next, err := sc.GetWithdrawableWitnessFee(nil, dc.dagchain.dag.db, account, 1)
	require.NoError(t, err)

	t.Logf("next period %d", next)
	require.Equal(t, big.NewInt(3000), amount)

}
