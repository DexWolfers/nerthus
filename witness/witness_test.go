package witness

import (
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/rlp"
	"math/big"
	"testing"
)

func TestStruct(t *testing.T) {
	info := &MsgStartWitness{
		Type:  MsgWitnessTypeWitnessStart,
		Data:  []byte{1, 2},
		Index: 1,
	}
	pk, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal()
	}
	siginer := types.NewSigner(big.NewInt(1))
	if err := types.SignBySignHelper(info, siginer, pk); err != nil {
		t.Error(err)
	}
	data, err := rlp.EncodeToBytes(info)
	if err != nil {
		t.Fail()
	}
	deInfo := new(MsgStartWitness)
	err = rlp.DecodeBytes(data, deInfo)
	if err != nil {
		t.Fatal(err)
	}

	t.Log(deInfo)
}
