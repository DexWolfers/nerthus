package util

import (
	"io/ioutil"
	"math"
	"math/big"
	"math/rand"
	"os"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/core/rawdb"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/ntsdb"
	"github.com/stretchr/testify/require"
)

func newDB(t *testing.T) (db *ntsdb.LDBDatabase, close func()) {
	name, err := ioutil.TempDir("", "ntsttest")
	require.NoError(t, err)
	db, err = ntsdb.NewLDBDatabase(name, 16, 16)
	require.NoError(t, err)

	return db, func() {
		db.Close()
		os.Remove(name)
	}
}

func createUnit(t *testing.T) *types.Unit {

	header := types.Header{
		Proposer:    common.StringToAddress("proposer"),
		MC:          common.ForceDecodeAddress("nts1x67wl68e9rhyxfkr4nu6jqmpvd2fufju5lk76c"),
		Number:      math.MaxUint64,
		ParentHash:  common.StringToHash("test"),
		SCHash:      common.StringToHash("test1"),
		Timestamp:   uint64(time.Now().UTC().UnixNano()),
		Bloom:       types.BytesToBloom([]byte("bloom...")),
		GasLimit:    rand.Uint64(),
		GasUsed:     rand.Uint64(),
		StateRoot:   common.StringToHash("StateRoot"),
		ReceiptRoot: common.StringToHash("ReceiptRoot"),
	}

	txs := make(types.TxExecs, 10)
	for i := 0; i < len(txs); i++ {
		txs[i] = &types.TransactionExec{}
		tx := types.NewTransaction(common.StringToAddress("from"),
			common.StringToAddress("from"), big.NewInt(int64(i)), rand.Uint64(), rand.Uint64(), 10, nil)
		txs[i].Tx = tx
		txs[i].TxHash = tx.Hash()
		txs[i].Action = types.ActionTransferPayment
	}

	unit := types.NewUnit(&header, txs)
	return unit

}
func TestEncodeMulUnit(t *testing.T) {
	db, close := newDB(t)
	defer close()

	var units []*types.Unit
	var list []common.Hash
	// create
	for i := 0; i < 10; i++ {
		unit := createUnit(t)

		require.NotPanics(t, func() {
			rawdb.WriteHeader(db, unit.Header())
			rawdb.WriteUnitBody(db, unit.MC(), unit.Hash(), unit.Number(), unit.Body())
		})

		units = append(units, unit)
		list = append(list, unit.Hash())
	}

	b := GetUnitRawRLP(db, list, 10000000)

	got, err := DecodeUnitRawRLP(b)
	require.NoError(t, err)
	require.Len(t, got, len(units))
	for i, v := range units {
		require.Equal(t, v.Hash(), got[i].Hash())
	}

	t.Run("encodeone", func(t *testing.T) {
		for _, v := range units {
			b := EncodeUnit(db, v.ID())
			got, err := DecodeUnitRawRLP(b)
			require.NoError(t, err)
			require.Len(t, got, 1)
			require.Equal(t, v.Hash(), got[0].Hash())
		}
	})
}
