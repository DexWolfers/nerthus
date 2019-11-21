package ethash

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"testing"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/params"
)

func TestVerifyProofWork1(t *testing.T) {
	hash := common.HexToHash("0xab5a3f2d90253374d5f78bb6bfd15cd05febcae2741296276a0035bfef39a05b")

	target := new(big.Int).Div(params.MaxUint256, new(big.Int).SetInt64(2000))

	base := big.NewInt(0).Sub(params.MaxUint256, big.NewInt(1))
	base = big.NewInt(0).Rsh(base, 20)
	t.Log(base)
	for i := 0; i < 1000; i++ {
		t.Log(i, base.Bit(i))
		if base.Bit(i) == 0 {
			break
		}
	}
	hash = common.BytesToHash(base.Bytes())
	t.Log(hash.Hex())
	t.Log(common.BigToHash(base).Hex())

	hashTxt := make([]byte, common.HashLength+8)
	copy(hashTxt[:common.HashLength], hash.Bytes())

	binary.LittleEndian.PutUint64(hashTxt[common.HashLength:], 143)
	digest := crypto.Keccak256(hashTxt[:])
	fmt.Println(new(big.Int).SetBytes(digest[:]).Cmp(target) <= 0)

	big := common.HexToHash("0x63e9d38eab98102497b11ef160a6a0abf0649ad10db34b3c1424bdae4ae86a96").Big()
	fmt.Println(big)
}

func TestProofWork(t *testing.T) {
	from := common.StringToAddress("0x028f095BB5bECd86D5f13d8E0157AD9F4e15E14c")
	to := common.StringToAddress("0x99e01F28D7d85aFEbFcBa5Ef2dfB45136060Cfcf")
	var tx = types.NewTransaction(from, to, big.NewInt(10), 19, 19, 0, nil)
	seedCh := ProofWork(tx, 1, big.NewInt(20), make(chan struct{}))
	seed, ok := <-seedCh
	if !ok {
		t.Fail()
	}
	t.Log("seed", "tx", tx.HashNoSeed().Hex(), "s", seed)
	tx.SetSeed(seed)

	res := VerifyProofWork(tx, big.NewInt(20))
	if !res {
		t.Fail()
	}
	//var (
	//	addr        = common.StringToAddress("0x0001")
	//	transaction = types.NewTransaction(addr, addr, big.NewInt(10), 19, 19, 0, nil)
	//	abort       = make(chan struct{})
	//	diff        = big.NewInt(32)
	//)
	//// Test ProofWork
	//seedCh := ProofWork(transaction, 0, diff, abort)
	//
	//seed, ok := <-seedCh
	//if !ok {
	//	t.Fatal("expect true, got nil")
	//}
	//_, ok = <-seedCh
	//if ok {
	//	t.Fatal("expect false, got true")
	//}
	//transaction.SetSeed(seed)
	//t.Log(ProofWorkHash(transaction, diff).Hex())
	//if !VerifyProofWork(transaction, diff) {
	//	t.Fatal("expect true, got false")
	//}
	//
	//// Test CheckProofWork
	//{
	//	seed := transaction.Seed().Uint64() - 1
	//	transaction.SetSeed(seed)
	//	if VerifyProofWork(transaction, diff) {
	//		t.Fatal("expect false, got true")
	//	}
	//}
	//
	//// Test ValidUpdateWitnessProofWork
	////{
	////	tx := types.CopyTransaction(transaction)
	////	tx.To().Set(common.HexToAddress("0x00002"))
	////	diff := new(big.Int).SetUint64(1 << 20)
	////	seedCh := ProofWork(tx, 1, diff, abort)
	////	seed, _ := <-seedCh
	////	tx.SetSeed(big.NewInt(int64(seed)))
	////	if !VerifyProofWork(tx, diff) {
	////		t.Fatal("expect false, got true")
	////	}
	////}
	//
	//// Test Timeout
	//{
	//	seedCh := ProofWork(transaction, 0, diff, abort)
	//	abort <- struct{}{}
	//	_, ok := <-seedCh
	//	if ok {
	//		t.Fatal("expect false, got true")
	//	}
	//}
}

func TestIsNoFeedTx(t *testing.T) {
	//var testAddr common.Address
	//var testAbiFn common.Hash
	//var tx *types.Transaction
	// pass
	//for addr, txs := range params.NoFeeTx() {
	//	to := common.HexToAddress(addr)
	//	testAddr = to
	//	for abiFnNum, _ := range txs {
	//		testAbiFn = common.BigToHash(big.NewInt(int64(abiFnNum)))
	//		abiFn := common.BigToHash(big.NewInt(int64(abiFnNum))).Bytes()
	//		tx = types.NewTransactionWithSeed(&testAddr, big.NewInt(0), big.NewInt(0), big.NewInt(0), big.NewInt(0), abiFn)
	//		seed := <-ProofWork(tx, 0, params.DefUpdateWitnessDiff, make(chan struct{}))
	//		tx.Seed().SetUint64(seed)
	//		if !IsNoFeeTx(tx) {
	//			t.Fatalf("expect true, got false")
	//		}
	//	}
	//
	//	//
	//	{
	//		testAbiFn := hexutil.MustDecode(fmt.Sprintf("0x%s", "e22c1fd5"))
	//		tx = types.NewTransactionWithSeed(&testAddr, big.NewInt(0), big.NewInt(0), big.NewInt(0), big.NewInt(0), testAbiFn)
	//		seed := <-ProofWork(tx, 0, params.DefUpdateWitnessDiff, make(chan struct{}))
	//		tx.Seed().SetUint64(seed)
	//		if !IsNoFeeTx(tx) {
	//			t.Fatalf("expect true, got false")
	//		}
	//	}
	//}

	// not exists abiFn
	//{
	//	testAbiFn := common.BigToHash(big.NewInt(testAbiFn.Big().Int64() + 100))
	//	tx := types.NewTransactionWithSeed(&testAddr, big.NewInt(0), big.NewInt(0), big.NewInt(0), big.NewInt(0), testAbiFn.Bytes())
	//	if IsNoFeeTx(tx) {
	//		t.Fatalf("expect false, got true")
	//	}
	//}
	//// not exists addr
	//{
	//	testAddr := common.BigToAddress(big.NewInt(testAddr.Big().Int64() + 100))
	//	tx := types.NewTransactionWithSeed(&testAddr, big.NewInt(0), big.NewInt(0), big.NewInt(0), big.NewInt(0), testAbiFn.Bytes())
	//	if IsNoFeeTx(tx) {
	//		t.Fatalf("expect false, got true")
	//	}
	//}
	//
	//// invalid seed
	//{
	//	testAddr := common.BigToAddress(big.NewInt(testAddr.Big().Int64() + 100))
	//	tx := types.NewTransactionWithSeed(&testAddr, big.NewInt(0), big.NewInt(0), big.NewInt(0), big.NewInt(0), testAbiFn.Bytes())
	//
	//	tx.SetSeed(big.NewInt(9999))
	//	if IsNoFeeTx(tx) {
	//		t.Fatalf("expect false, got true")
	//	}
	//}
}

func BenchmarkProofWork(b *testing.B) {
	var addr = common.StringToAddress("0x0001")
	var abort = make(chan struct{})
	for n := 0; n < b.N; n++ {
		var transaction = types.NewTransaction(addr, addr, big.NewInt(10), 19, 19, 0, nil)
		seedCh := ProofWork(transaction, 1, big.NewInt(1<<20), abort)
		<-seedCh
	}
}
