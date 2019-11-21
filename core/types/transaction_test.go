package types

import (
	"crypto/ecdsa"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/rlp"

	"fmt"

	"github.com/stretchr/testify/assert"
)

// The values in those tests are from the Transaction Tests
// at github.com/ethereum/tests.
var (
//signer = NewSigner(big.NewInt(18))
//
//emptyTx = NewTransaction(
//	0,
//	common.HexToAddress("095e7baea6a6c7c4c2dfeb977efac326af552d87"),
//	big.NewInt(0), big.NewInt(0), big.NewInt(0),
//	nil,
//)
//
//rightvrsTx = NewTransaction(
//	3,
//	common.HexToAddress("b94f5374fce5edbc8e2a8697c15331677e6ebf0b"),
//	big.NewInt(10),
//	big.NewInt(2000),
//	big.NewInt(1),
//	common.FromHex("5544"),
//)
)

func TestCoinflow_MarshalJSON(t *testing.T) {

	flow := Coinflow{
		To:     common.StringToAddress("a"),
		Amount: big.NewInt(10000),
	}

	info, err := json.MarshalIndent(flow, "", "\t")
	require.NoError(t, err)
	require.Contains(t, string(info), `"Amount": "0x2710"`)
}

func TestTransactionSigHash(t *testing.T) {
	//if emptyTx.SigHash(signer) != common.HexToHash("0f6c8bb8d87580cbd0a301fa5897a653f8947b565e2f3682ea1955f7bef0621d") {
	//	t.Errorf("empty transaction hash mismatch, got %x", emptyTx.Hash())
	//}
	//if rightvrsTx.SigHash(signer) != common.HexToHash("45adc2e3107b7a8a950ff73dce7a071ab21038b1412958bd7e1cc4266a1f1994") {
	//	t.Errorf("RightVRS transaction hash mismatch, got %x", rightvrsTx.Hash())
	//}

	//newTx := NewTransaction(0,)
	priKey, err := crypto.GenerateKey()
	if err != nil {
		t.Errorf("generate private key err %v", err)
	}
	address := crypto.ForceParsePubKeyToAddress(priKey.PublicKey)
	//fmt.Println("address=", address.String())

	tx := NewTransaction(address, address, big.NewInt(0), 0, 0, 0, nil)

	signer := NewSigner(big.NewInt(18))
	err = SignBySignHelper(tx, signer, priKey)
	if err != nil {
		t.Errorf("sign err %v", err)
	}
	address1, err := tx.Sender()
	if err != nil {
		t.Fatal(err)
	}
	//fmt.Println("address1=", address1.String())
	if address == address1 {
		t.Log("ok")
	}
}

func TestTxJson(t *testing.T) {
	priKey, _ := crypto.HexToECDSA("86e38b31d6de78971874ced73d96852168e807d3d1fa90744672413bb596dc46")
	address := crypto.ForceParsePubKeyToAddress(priKey.PublicKey) //nts1wrk344fy4hncw2w92cn7yszspav3xj2sn253tx
	tx := NewTransaction(address, address, big.NewInt(1), 2, 3, 0, nil)
	signer := NewSigner(big.NewInt(18))
	err := SignBySignHelper(tx, signer, priKey)
	if err != nil {
		t.Fatal(err)
	}
	data, err := tx.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var mapIns map[string]interface{}
	err = json.Unmarshal(data, &mapIns)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := mapIns["from"]; !ok {
		t.Fatalf("should be contains from item in json,but not")
	} else {
		got, err := common.DecodeAddress(fmt.Sprintf("%v", got))
		require.NoError(t, err)
		require.Equal(t, address, got)
	}

	var got Transaction
	err = json.Unmarshal(data, &got)
	if err != nil {
		t.Fatal(err)
	}
	if got.Hash() != tx.Hash() {
		t.Fatalf("tx hash mismatch,expect:%s,got：%s", tx.Hash(), got.Hash())
	}
	sender, err := got.Sender()
	if err != nil {
		t.Fatal(err)
	}
	if sender != address {
		t.Fatalf("sender mismatch,expect %s,got %s", address.Hex(), sender.Hex())
	}

}

func TestTransactionEncode(t *testing.T) {
	//address := common.StringToAddress("addr")
	private, addr := defaultTestKey()
	tx := NewTransaction(addr, addr, big.NewInt(12), 321, 3213, 0, nil)
	signer := NewSigner(big.NewInt(123))
	_, err := SignTx(tx, signer, private)
	require.Nil(t, err)
	txb, err := rlp.EncodeToBytes(tx)
	require.Nil(t, err)
	//t.Log(tx)
	var tx2 Transaction
	err = rlp.DecodeBytes(txb, &tx2)
	require.Nil(t, err)
	from, err := tx2.Sender()
	require.Nil(t, err)
	require.Equal(t, addr, from)
}

func defaultTestKey() (*ecdsa.PrivateKey, common.Address) {
	key, _ := crypto.HexToECDSA("45a915e4d060149eb4365960e6a7a45f334393093061116b197e3240065ff2d8")
	addr := crypto.ForceParsePubKeyToAddress(key.PublicKey)
	return key, addr
}

func TestTransactionSortBy(t *testing.T) {
	priKey, _ := crypto.GenerateKey()
	address := crypto.ForceParsePubKeyToAddress(priKey.PublicKey)

	var txlist Transactions
	for i := 0; i < 10; i++ {
		tx := NewTransaction(address, address, big.NewInt(int64(i)), uint64(i+1), uint64(i+2), 0, nil)
		txlist = append(txlist, tx)
		t.Log(tx.Time())
	}

	TransactionSortBy(txlist, func(p, q *Transaction) bool {
		if p.Time() > q.Time() {
			return true
		}
		return false
	})

	t.Log("排序")
	for i := 0; i < 10; i++ {
		t.Log(txlist[i].Time())
	}
}

func TestTransactionExec_Hash(t *testing.T) {
	priKey, addr := defaultTestKey()
	//address := crypto.PubkeyToAddress(priKey.PublicKey)
	tx := NewTransaction(addr, addr, big.NewInt(1000), 200, 1, 0, nil)
	signer := NewSigner(big.NewInt(99))
	_, err := SignTx(tx, signer, priKey)
	if err != nil {
		t.Fatal(err)
	}
	txexec := TransactionExec{
		Action: ActionTransferPayment,
		Tx:     tx,
	}
	txexec2 := TransactionExec{
		Action: ActionTransferPayment,
		Tx:     tx,
	}
	// 判断两者Hash必须正确
	if h1, h2 := txexec.Hash(), txexec2.Hash(); h1 != h2 {
		t.Fatalf("mismatch TransactionExec hash %s != %s", h1.Hex(), h2.Hex())
	}
}

func TestTransactionExec_Code(t *testing.T) {
	priKey, addr := defaultTestKey()
	//address := crypto.PubkeyToAddress(priKey.PublicKey)
	tx := NewTransaction(addr, addr, big.NewInt(1000), 200, 1, 0, nil)
	signer := NewSigner(big.NewInt(99))
	_, err := SignTx(tx, signer, priKey)
	if err != nil {
		t.Fatal(err)
	}
	result := TxResult{
		Failed:      true,
		GasUsed:     100,
		ReceiptRoot: EmptyRootHash,
	}

	t.Run("check hash", func(t *testing.T) {
		txexec := TransactionExec{
			Action: ActionTransferReceipt,
			Tx:     tx,
		}

		txexec2 := TransactionExec{
			Action: ActionTransferReceipt,
			Tx:     tx,
		}

		// 判断两者Hash必须正确
		assert.Equal(t, txexec2.Hash().Hex(), txexec.Hash().Hex(), "the same info must have the same hash")

		result.GasUsed = 10000
		txexec3 := TransactionExec{
			Action: ActionTransferPayment,
			Tx:     tx,
		}
		assert.NotEqual(t, txexec3.Hash().Hex(), txexec.Hash().Hex(), "the different info must have the different hash")
	})

	t.Run("check rlp", func(t *testing.T) {
		txexec := TransactionExec{
			Action: ActionTransferReceipt,
			Tx:     tx,
		}

		data, err := rlp.EncodeToBytes(&txexec)
		if err != nil {
			t.Fatalf("failed to encode TransactionExec to rlp:%v", err)
		}

		var tx2 TransactionExec
		err = rlp.DecodeBytes(data, &tx2)
		if err != nil {
			t.Fatalf("failed to edcode rlp bytes to  TransactionExec :%v", err)
		}
		assert.Equal(t, txexec.Hash().Hex(), tx2.Hash().Hex())
	})

	t.Run("check json", func(t *testing.T) {
		txexec := TransactionExec{
			Action: ActionTransferReceipt,
			Tx:     tx,
		}

		data, err := json.Marshal(txexec)
		assert.NoError(t, err, "failed to json marshal TransactionExec")

		var tx2 TransactionExec
		err = json.Unmarshal(data, &tx2)
		assert.NoError(t, err, "failed to json unmarshal bytes to TransactionExec")
		assert.Equal(t, txexec.Hash().Hex(), tx2.Hash().Hex())
	})

}

func TestRLPTransactionExec(t *testing.T) {
	addr := common.StringToAddress("addr")
	tx := NewTransaction(addr, addr, big.NewInt(100), 1000, 199, 0, []byte("abcdefg"))
	txs := []*TransactionExec{
		{Action: ActionContractReceipt, TxHash: common.StringToHash("test")},
		{Action: ActionContractReceipt, TxHash: common.StringToHash("test"), Tx: tx},
		{Action: ActionContractReceipt, TxHash: common.StringToHash("test")},
		{Action: ActionContractReceipt, TxHash: common.StringToHash("test"), Tx: tx},
	}
	data, err := rlp.EncodeToBytes(txs)
	if err != nil {
		t.Fatal(err)
	}

	var got []*TransactionExec
	err = rlp.DecodeBytes(data, &got)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < len(txs); i++ {
		if h1, h2 := txs[i].Hash(), got[i].Hash(); h1 != h2 {
			t.Errorf("%d: mismatch transactionExec hash before %x,after %x", i, h1, h2)
		}
	}
}

func TestTransactionExec_DecodeRLP(t *testing.T) {

	checkInfo := func(t *testing.T, a, b *TransactionExec) {

		require.Equal(t, a.Action, b.Action)
		require.Equal(t, a.TxHash, b.TxHash)
		require.Equal(t, a.PreStep, a.PreStep)

		if a.Tx == nil {
			require.Nil(t, b.Tx)
		} else {
			require.Equal(t, a.Tx.Hash(), b.Tx.Hash())
		}
	}

	t.Run("txnil", func(t *testing.T) {
		info := &TransactionExec{
			Action: ActionContractReceipt,
			TxHash: common.StringToHash("test"),
			PreStep: UnitID{
				ChainID: common.StringToAddress("parent"),
				Height:  1000,
				Hash:    common.StringToHash("preHash"),
			},
		}
		info.Tx = nil
		data, err := rlp.EncodeToBytes(info)
		require.Nil(t, err)
		var got TransactionExec
		err = rlp.DecodeBytes(data, &got)
		require.NoError(t, err)
		checkInfo(t, info, &got)
	})

	t.Run("txNotNil", func(t *testing.T) {
		info := &TransactionExec{
			Action: ActionContractReceipt,
			TxHash: common.StringToHash("test"),
			PreStep: UnitID{
				ChainID: common.StringToAddress("parent"),
				Height:  1000,
				Hash:    common.StringToHash("preHash"),
			},
			Tx: NewTransaction(
				common.StringToAddress("from"),
				common.StringToAddress("to"), big.NewInt(100), 1000, 199, 0, []byte("abcdefg")),
		}
		data, err := rlp.EncodeToBytes(info)
		require.NoError(t, err)

		var got TransactionExec
		err = rlp.DecodeBytes(data, &got)
		require.NoError(t, err)
		checkInfo(t, info, &got)

	})

}

func TestTxSender(t *testing.T) {
	private, addr := defaultTestKey()
	tx := NewTransaction(addr, addr, big.NewInt(12), 321, 3213, 0, nil)
	signer := NewSigner(big.NewInt(123))
	_, err := SignTx(tx, signer, private)
	require.Nil(t, err)

	sender, _ := Sender(signer, tx)
	t.Log(sender.Hex(), sender.Text())
	t.Log(addr.Hex(), addr.Text())
}

func TestReceiptForStorage_DecodeRLP(t *testing.T) {
	rec := ReceiptForStorage{
		TxHash:            common.StringToHash("tx1"),
		Action:            ActionSCIdleContractDeal,
		Failed:            true,
		CumulativeGasUsed: big.NewInt(10000),
		Bloom:             BytesToBloom([]byte("bloom")),
		Coinflows: []Coinflow{
			{common.StringToAddress("to1"), big.NewInt(123)},
			{common.StringToAddress("to2"), big.NewInt(12345)},
		},
		Logs:            []*Log{},
		ContractAddress: common.StringToAddress("new contract address"),
		GasRemaining:    1234567,
		GasUsed:         123456789,
		Output:          []byte("output"),
		PreStep: UnitID{
			ChainID: common.StringToAddress("uint2"),
			Height:  123,
			Hash:    common.StringToHash("unit2hash"),
		},
	}

	data, err := rlp.EncodeToBytes(&rec)
	require.NoError(t, err)

	var got ReceiptForStorage
	err = rlp.DecodeBytes(data, &got)
	require.NoError(t, err)
	require.Equal(t, rec, got)
}

func TestReceipt_EncodeRLP(t *testing.T) {
	r := Receipt{
		TxHash:            common.StringToHash("tx1"),
		Action:            ActionSCIdleContractDeal,
		Failed:            true,
		CumulativeGasUsed: big.NewInt(10000),
		Coinflows: []Coinflow{
			{common.StringToAddress("to1"), big.NewInt(123)},
			{common.StringToAddress("to2"), big.NewInt(12345)},
		},
		Logs:         []*Log{},
		GasRemaining: 1234567,
		GasUsed:      123456789,
		PreStep: UnitID{
			ChainID: common.StringToAddress("uint2"),
			Height:  123,
			Hash:    common.StringToHash("unit2hash"),
		},
		Output:          []byte("abcdefgoutput"),
		ContractAddress: common.StringToAddress("newcontractaddress"),
	}

	b, err := rlp.EncodeToBytes(&r)
	require.NoError(t, err)

	var got Receipt
	err = rlp.DecodeBytes(b, &got)
	require.NoError(t, err)
	require.Equal(t, r, got)

	// 相同回执的哈希值必须一致
	hash1 := common.SafeHash256(&r)
	hash2 := common.SafeHash256(&got)
	require.Equal(t, hash1, hash2)
}

func TestTransaction_IsFuture(t *testing.T) {
	_, addr := defaultTestKey()
	tx := NewTransaction(addr, addr, big.NewInt(12), 321, 3213, 0, nil)

	require.False(t, tx.IsFuture(time.Now()))
	require.False(t, tx.IsFuture(time.Now().Add(time.Minute)))
	require.False(t, tx.IsFuture(tx.Time2().Add(-2*time.Minute)))
	require.True(t, tx.IsFuture(tx.Time2().Add(-2*time.Minute-1)))
}

//now                  2000000	       718 ns/op	     680 B/op	      16 allocs/op
//commit:3b2337b7d     2000000	       842 ns/op	     760 B/op	      17 allocs/op
func BenchmarkTransactionExec_EncodeRLP(b *testing.B) {
	info := &TransactionExec{
		Action: ActionContractReceipt,
		TxHash: common.StringToHash("test"),
		PreStep: UnitID{
			ChainID: common.StringToAddress("parent"),
			Height:  1000,
			Hash:    common.StringToHash("preHash"),
		},
		Tx: NewTransaction(
			common.StringToAddress("from"),
			common.StringToAddress("to"), big.NewInt(100), 1000, 299, 0, []byte("abcdefg")),
	}

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rlp.EncodeToBytes(info)
		}
	})
}

//Now
//BenchmarkTransactionExec_DecodeRLP/for-4         	  300000	      4791 ns/op	    1320 B/op	      21 allocs/op
//BenchmarkTransactionExec_DecodeRLP/parallel-4    	  500000	      2754 ns/op	    1320 B/op	      21 allocs/op
//
//commit:3b2337b7d
// BenchmarkTransactionExec_DecodeRLP/for-4         	  300000	      4926 ns/op	    1312 B/op	      23 allocs/op
// BenchmarkTransactionExec_DecodeRLP/parallel-4    	  500000	      2965 ns/op	    1313 B/op	      23 allocs/op
func BenchmarkTransactionExec_DecodeRLP(b *testing.B) {
	info := &TransactionExec{
		Action: ActionContractReceipt,
		TxHash: common.StringToHash("test"),
		PreStep: UnitID{
			ChainID: common.StringToAddress("parent"),
			Height:  1000,
			Hash:    common.StringToHash("preHash"),
		},
		Tx: NewTransaction(
			common.StringToAddress("from"),
			common.StringToAddress("to"), big.NewInt(100), 1000, 299, 0, []byte("abcdefg")),
	}

	data, err := rlp.EncodeToBytes(info)
	require.NoError(b, err)

	b.Run("for", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			got := new(TransactionExec)
			err = rlp.DecodeBytes(data, got)
			require.NoError(b, err)
			got = nil
		}
	})
	b.Run("parallel", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				got := new(TransactionExec)
				err = rlp.DecodeBytes(data, got)
				require.NoError(b, err)
				got = nil
			}
		})
	})

}
