// Author:@kulics
package sc

import (
	"math/big"
	"strings"
	"testing"

	"gitee.com/nerthus/nerthus/crypto"
	"github.com/stretchr/testify/require"

	"gitee.com/nerthus/nerthus/accounts/abi"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/params"
)

// 测试abi
func TestAbi(t *testing.T) {
	t.Log(params.SCAccount.String())

	abi, err := abi.JSON(strings.NewReader(AbiCode))
	if err != nil {
		t.Fatal(err)
	}
	input, err := abi.Pack("SetMemberApplyVote", big.NewInt(123), true)
	if err != nil {
		t.Fatal(err)
	}
	realInput := append(common.Hex2Bytes(FuncIdSetConfigApplyVote), input[4:]...)

	t.Log(common.Bytes2Hex(realInput))

	//
	input, err = abi.Pack("Settlement")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(common.Bytes2Hex(append(common.Hex2Bytes(FuncSettlementID), input...)))

	//
	input, err = abi.Pack("ApplyWitness")
	if err != nil {
		t.Fatal(err)
	}

	t.Log(common.Bytes2Hex(input), common.FuncID("Apply"))

	input, err = abi.Pack("Report", common.HexToHash("test hash"))
	if err != nil {
		t.Fatal(err)
	}

	t.Log(common.Bytes2Hex(input))

}

// 打印函数id
func TestFuncID(t *testing.T) {
	abi, err := abi.JSON(strings.NewReader(AbiCode))
	if err != nil {
		t.Fatal(err)
	}

	for name, m := range abi.Methods {
		t.Logf("method %s id: %v", name, common.Bytes2Hex(m.Id()))
	}
}

func TestArray(t *testing.T) {
	obj := LoadSysABI()

	arr := make([]common.Address, 1)

	arr = append(arr, common.Address{})

	data, err := obj.Pack("SystemWitnessReplace", arr)
	if err != nil {
		t.Fatalf("err:%v", err)
	}
	t.Log("0x" + common.Bytes2Hex(data))
}

func TestSySABI(t *testing.T) {
	obj := LoadSysABI()

	addr := common.BytesToAddress([]byte("22222222233333333333"))
	var wits []common.Address
	for i := 1; i < 5; i++ {
		a := common.BytesToAddress(big.NewInt(int64(i * 100000000000000000)).Bytes())
		wits = append(wits, a)
	}

	type P struct {
		Contract common.Address
		Witness  []common.Address
	}

	data, err := obj.Methods["CreateContract"].Outputs.Pack(addr, wits)
	if err != nil {
		t.Fatalf("err:%v", err)
	}

	var out P
	err = obj.Methods["CreateContract"].Outputs.Unpack(&out, data)
	if err != nil {
		t.Fatalf("error 2: %v", err)
	}

	if out.Contract != addr {
		t.Fatalf("contract address not match:%v, %v", out.Contract, addr)
	}

	count := len(wits)
	for i := 0; i < count; i++ {
		if wits[i] != out.Witness[i] {
			t.Fatalf("witness address not match:%v, %v", wits[i], out.Witness[i])
		}
	}

	data, err = obj.Methods["ReplaceWitness"].Outputs.Pack(wits)
	if err != nil {
		t.Log("fail to replace witness output pack")
	}

	var r []common.Address
	err = obj.Methods["ReplaceWitness"].Outputs.Unpack(&r, data)
	if err != nil {
		t.Fatalf("fail to unpack")
	}
	t.Log(r)
}

func TestRemoveInvalidABI(t *testing.T) {

	var list []common.Address
	for i := 0; i < 100; i++ {
		k, err := crypto.GenerateKey()
		require.NoError(t, err)
		list = append(list, crypto.ForceParsePubKeyToAddress(k.PublicKey))
	}

	input := CreateCallInputData(FuncIdRemoveInvalid, list)
	t.Log(len(input))

	get, err := GetContractInput(input)
	require.NoError(t, err)

	inputData := get.([]common.Address)
	require.Equal(t, len(list), len(inputData))
	for _, v := range list {
		require.Contains(t, inputData, v)
	}
}
