package types

import (
	"crypto/ecdsa"
	"encoding/binary"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/rlp"
	"github.com/stretchr/testify/require"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/params"
	"github.com/stretchr/testify/assert"
)

type Raw interface {
	Hash() common.Hash
}

type unmarshal func(data []byte) (Raw, error)

func testjson(t *testing.T, raw Raw, unmarshal unmarshal) {
	t.Logf("json marshal %T", raw)
	data, err := json.MarshalIndent(raw, "", "\t")
	if err != nil {
		t.Fatal(err)
	}
	got, err := unmarshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if raw.Hash() != got.Hash() {
		gotData, _ := json.MarshalIndent(got, "", "\t")
		t.Fatalf("unmarshal result mismatch.\n want:\n%s \ngot:\n%s", string(gotData), string(data))
	}
}

func toVoteMsg(data []byte) (Raw, error) {
	var got VoteMsg
	err := json.Unmarshal(data, &got)
	return &got, err
}
func toSign(data []byte) (Raw, error) {
	var got SignContent
	err := json.Unmarshal(data, &got)
	return &got, err
}

func toHeader(data []byte) (Raw, error) {
	var got Header
	err := json.Unmarshal(data, &got)
	return &got, err
}

func TestJson(t *testing.T) {

	//result := TxResult{
	//	Failed:      false,
	//	GasUsed:     math.MaxBig256,
	//	ReceiptRoot: EmptyRootHash,
	//}

	header := Header{
		MC: common.ForceDecodeAddress("nts1x67wl68e9rhyxfkr4nu6jqmpvd2fufju5lk76c"), Number: math.MaxUint64,
		ParentHash: common.StringToHash("test"), SCHash: common.StringToHash("test1"),
		Timestamp:   uint64(time.Now().UTC().UnixNano()),
		Bloom:       BytesToBloom([]byte("bloom...")),
		GasLimit:    999,
		GasUsed:     888,
		StateRoot:   EmptyRootHash,
		ReceiptRoot: EmptyRootHash,
	}

	testcaces := []struct {
		raw       Raw
		unmarshal unmarshal
	}{
		{&SignContent{}, toSign},
		{&SignContent{[]byte{}}, toSign},
		{&SignContent{[]byte{}}, toSign},

		{&VoteMsg{UnitHash: common.StringToHash("header hash"), Sign: randSign(), Extra: createVoteExtra()}, toVoteMsg},
		{&header, toHeader},
	}
	for _, c := range testcaces {
		testjson(t, c.raw, c.unmarshal)
	}
}

func randSign() SignContent {
	s := SignContent{}
	s.Set([]byte{})
	return s
}

func TestHeadString(t *testing.T) {
	header := Header{
		MC: common.ForceDecodeAddress("nts1x67wl68e9rhyxfkr4nu6jqmpvd2fufju5lk76c"), Number: math.MaxUint64,
		ParentHash: common.StringToHash("test"), SCHash: common.StringToHash("test1"),
		Timestamp:   uint64(time.Now().UTC().UnixNano()),
		Bloom:       BytesToBloom([]byte("bloom...")),
		GasLimit:    999,
		GasUsed:     888,
		StateRoot:   EmptyRootHash,
		ReceiptRoot: EmptyRootHash,
	}
	t.Log(header.String())
}

func newTestKey() (*ecdsa.PrivateKey, common.Address) {
	key, _ := crypto.GenerateKey()
	//key, _ := crypto.HexToECDSA("45a915e4d060149eb4365960e6a7a45f334393093061116b197e3240065ff2d8"+ )
	addr := crypto.ForceParsePubKeyToAddress(key.PublicKey)
	return key, addr
}

func newTestUnit(t *testing.T) (u *Unit, from, to common.Address) {
	signer := NewSigner(big.NewInt(1))
	key, from := newTestKey()

	tx := newTransaction(from, to, math.MaxBig63, 50000, 1000000000, 0, 0, []byte("payloaddatainputherefor test"))
	err := SignBySignHelper(tx, signer, key)
	if err != nil {
		t.Fatal(err)
	}
	header := Header{
		MC: common.ForceDecodeAddress("nts1x67wl68e9rhyxfkr4nu6jqmpvd2fufju5lk76c"), Number: math.MaxUint64,
		ParentHash: common.StringToHash("test"), SCHash: common.StringToHash("test1"),
		Timestamp:   uint64(time.Now().UTC().UnixNano()),
		Bloom:       BytesToBloom([]byte("bloom...")),
		GasLimit:    999,
		GasUsed:     888,
		StateRoot:   EmptyRootHash,
		ReceiptRoot: EmptyRootHash,
	}
	txs := []*TransactionExec{
		{Action: ActionTransferPayment, Tx: tx},
	}

	unit := NewUnit(&header, txs)
	var votes []WitenssVote
	for i := 0; i < 5; i++ {
		key, addr := newTestKey()
		msg := NewVoteMsg(header.Hash(), createVoteExtra())
		err := SignBySignHelper(msg, signer, key)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := msg.Sender()
		if got != addr {
			t.Fatalf("expect %x,got %x", addr, got)
		}
		votes = append(votes, msg.ToWitnessVote())
		unit.witenssVotes = append(unit.witenssVotes, msg.ToWitnessVote())
	}

	// 设置 见证人投票消息
	err = SignBySignHelper(unit, signer, key)
	if err != nil {
		t.Fatal(err)
	}
	return unit, from, to
}

func TestUnitJson(t *testing.T) {
	signer := NewSigner(big.NewInt(1))

	unit, from, _ := newTestUnit(t)

	t.Run("check sign", func(t *testing.T) {
		sender, err := unit.Sender()
		if err != nil {
			t.Fatal(err)
		}
		if sender != from {
			t.Fatalf("sender is error,expect %s,unexpected %s", from, sender)
		}
		_, err = unit.Witnesses()
		assert.NoError(t, err, "should be successful for load witnesses")
	})
	t.Run("check unit json", func(t *testing.T) {
		data, err := json.MarshalIndent(unit, "", "\t")
		if err != nil {
			t.Fatal(err)
		}

		//t.Logf(string(data))
		//获取
		var newUnit Unit
		err = json.Unmarshal(data, &newUnit)
		if !assert.NoError(t, err, "failed to unmarshal unit") {
			t.Logf(string(data))
		}
		newUnitData, err := json.MarshalIndent(newUnit, "", "\t")
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != string(newUnitData) {
			t.Fatal("mimatch json result")
		}

		if newUnit.Hash() != unit.Hash() {
			t.Fatalf("expect %s,unexpected %s", unit.Hash().Hex(), newUnit.Hash().Hex())
		}

		sender, err := Sender(signer, &newUnit)
		if err != nil {
			t.Fatal(err)
		}
		if sender != from {
			t.Fatalf("sender is error,expect %s,unexpected %s", from.Hex(), sender.Hex())
		}
	})

}

func createNewUnit(t *testing.T, max bool) common.StorageSize {
	var (
		val    *big.Int
		voters = int(params.ConfigParamsUCWitness)
		data   []byte
	)
	if max {
		val = math.MaxBig256
		data = make([]byte, params.MaxCodeSize)
	} else {
		val = big.NewInt(1)
	}

	signer := NewSigner(big.NewInt(1))
	key, to := defaultTestKey()

	tx := NewTransaction(to, to, val, val.Uint64(), val.Uint64(), 0, data)
	err := SignBySignHelper(tx, signer, key)
	if err != nil {
		t.Fatal(err)
	}

	header := Header{
		MC: to, Number: 1,
		ParentHash: common.StringToHash("test"), SCHash: common.StringToHash("test1"),
		Timestamp:   uint64(time.Now().UTC().UnixNano()),
		Bloom:       BytesToBloom([]byte("bloom...")),
		GasLimit:    999,
		GasUsed:     888,
		StateRoot:   EmptyRootHash,
		ReceiptRoot: EmptyRootHash,
	}

	txs := []*TransactionExec{
		{Action: ActionTransferPayment, Tx: tx},
	}
	var votes []WitenssVote

	for i := 0; i < voters/3*2+1; i++ {
		msg := NewVoteMsg(header.Hash(), createVoteExtra())
		err := SignBySignHelper(msg, signer, key)
		if err != nil {
			t.Fatal(err)
		}
		votes = append(votes, msg.ToWitnessVote())
	}

	unit := NewUnit(&header, txs)
	unit.WithBody(txs, votes, SignContent{})
	err = SignBySignHelper(unit, signer, key)
	if err != nil {
		t.Fatal(err)
	}
	return unit.Size()
}

func TestUnitSize(t *testing.T) {
	t.Logf("unit store size interval is [%v,%v]", createNewUnit(t, false), createNewUnit(t, true))
}

func TestDefLocatin(t *testing.T) {

	for i := 0; i < 100; i++ {
		u, _, _ := newTestUnit(t)

		b, err := rlp.EncodeToBytes(u)
		require.NoError(t, err)

		var got Unit
		err = rlp.DecodeBytes(b, &got)
		require.NoError(t, err)
		require.Equal(t, u.Hash().Hex(), got.Hash().Hex())
	}

}

func createVoteExtra() []byte {
	var buffer = make([]byte, ExtraSealSize)
	buffer[0] = msgCommit
	binary.LittleEndian.PutUint64(buffer[1:ExtraSealSize], 1)
	return buffer
}

func TestCopyHeader(t *testing.T) {

	header := Header{
		MC:         common.StringToAddress("chain"),
		Number:     1,
		ParentHash: common.StringToHash("test"), SCHash: common.StringToHash("test1"),
		Timestamp:   uint64(time.Now().UTC().UnixNano()),
		Bloom:       BytesToBloom([]byte("bloom...")),
		GasLimit:    999,
		GasUsed:     888,
		StateRoot:   EmptyRootHash,
		ReceiptRoot: EmptyRootHash,
	}

	cpy := CopyHeader(&header)

	h1, h2 := header.Hash(), cpy.Hash()

	require.Equal(t, h1, h2)

	//修改
	cpy.Number = 2
	cpy.MC = common.StringToAddress("cpy")
	cpy.Timestamp = 1
	cpy.SCNumber = header.SCNumber + 1
	cpy.SCHash = common.StringToHash("bad")
	cpy.Proposer = common.StringToAddress("pppp")
	cpy.GasUsed = 1
	cpy.GasLimit = 2
	require.Equal(t, h1, header.Hash())

}
