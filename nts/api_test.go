package nts

import (
	"encoding/json"
	"fmt"
	"math/big"
	"testing"

	"gitee.com/nerthus/nerthus/accounts/abi"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/hexutil"
	"github.com/stretchr/testify/require"
)

func TestBlockJSON(t *testing.T) {
	// var empty = "0x0"
	var empty = "0x0000000000000000000000000000000000000000"
	var fromAddr = common.BigToAddress(big.NewInt(15))
	var block Block
	{
		block.From = fromAddr
	}

	// Test Json decode
	{
		b, err := json.Marshal(block)
		if err != nil {
			t.Fatalf("expect:%v, got:%v", nil, err)
		}
		// t.Logf("%s", b)
		var obj map[string]interface{}
		err = json.Unmarshal(b, &obj)
		if err != nil {
			t.Fatalf("expect:%v, got:%v", nil, err)
		}

		if fmt.Sprintf("%v", obj["from"]) != fromAddr.Hex() {
			t.Errorf("expect:%v, got:%v", fromAddr.Hex(), fmt.Sprintf("%v", obj["from"]))
		}

		// t.Logf("to:%v", obj["to"])
		if fmt.Sprintf("%v", obj["to"]) != empty && fmt.Sprintf("%v", obj["to"]) != "0x0" {
			t.Errorf("expect:%v, got:%v", empty, fmt.Sprintf("%v", obj["to"]))
		}
	}

	// Test Encode
	{
		var newBlock Block
		b := []byte(`{"from":"0x000000000000000000000000000000000000000F","to":"0x0"}`)
		err := json.Unmarshal(b, &newBlock)
		if err != nil {
			t.Fatalf("expect:%v, got:%v", nil, err)
		}
		if block.From.Hex() != newBlock.From.Hex() {
			t.Errorf("expect:%v, got:%v", block.From.Hex(), newBlock.From.Hex())
		}
	}
}

func TestPublicNerthusAPI_DecodeABI(t *testing.T) {
	var args abi.Arguments
	arg := new(abi.Argument)
	err := arg.UnmarshalJSON([]byte(`{"name":"fff","type":"uint256"}`))
	require.Nil(t, err)
	args = append(args, *arg)
	err = arg.UnmarshalJSON([]byte(`{"name":"abc","type":"string"}`))
	require.Nil(t, err)
	args = append(args, *arg)
	t.Log(args)
	api := NewPublicNerthusAPI(nil)
	b, err := hexutil.Decode("0x0000000000000000000000000000000000000000000000000000000000000c8f000000000000000000000000000000000000000000000000000000000000004000000000000000000000000000000000000000000000000000000000000000046675636b00000000000000000000000000000000000000000000000000000000")
	require.Nil(t, err)
	data, err := api.DecodeABI(args, b)
	require.Nil(t, err)
	t.Log(data)
}

func TestPublicNerthusAPI_DecodeABI2(t *testing.T) {
	var args abi.Arguments
	arg := new(abi.Argument)
	err := arg.UnmarshalJSON([]byte(`{"name": "code", "type": "bytes32"}`))
	require.Nil(t, err)
	args = append(args, *arg)
	t.Log(args)
	api := NewPublicNerthusAPI(nil)
	b, err := hexutil.Decode("0x3cb6b6f1608060405234801561001057600080fd5b506040516020806100f2833981016040525160005560bf806100336000396000f30060806040526004361060485763ffffffff7c010000000000000000000000000000000000000000000000000000000060003504166360fe47b18114604d5780636d4ce63c146064575b600080fd5b348015605857600080fd5b5060626004356088565b005b348015606f57600080fd5b506076608d565b60408051918252519081900360200190f35b600055565b600054905600a165627a7a7230582019d363ab3c114369b30017b284874eb127b0d333bd2ec3f5e1819e50292dfa130029000000000000000000000000000000000000000000000000000000000000000c")
	require.Nil(t, err)
	data, err := api.DecodeABI(args, b)
	require.Nil(t, err)
	t.Log(data)

}

func TestPublicNerthusAPI_DecodeABI3(t *testing.T) {
	abiInfo := `[{"name":"","type":"uint256"},{"name":"","type":"bytes32"},{"name":"","type":"uint256[]"},{"name":"4","type":"bytes32"},{"name":"5","type":"address[]"},{"name":"6","type":"address"}]`
	var args abi.Arguments
	err := args.UnmarshalJSON([]byte(abiInfo))
	require.Nil(t, err)
	api := NewPublicNerthusAPI(nil)
	b, err := hexutil.Decode("0x0000000000000000000000000000000000000000000000000000000000000001737472696e67000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000c0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001e0000000000000000000000000000000000000000000000000000000000000012300000000000000000000000000000000000000000000000000000000000000080000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000700000000000000000000000000000000000000000000000000000000000000060000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000030000000000000000000000000000000000000000000000000000000000000123000000000000000000000000000692a70d2e424a56d2c6c27aa97d1a863958770000000000000000000000000000ca35b7d915458ef540ade6068dfe2f44e8fa")
	require.Nil(t, err)
	for _, v := range args {
		t.Log(v.Name, v.Type.String())
	}
	data, err := api.DecodeABI(args, b)
	require.Nil(t, err)

	keys := []string{"r1", "r2", "r3", "4", "5"}
	for _, k := range keys {
		_, ok := data[k]
		require.True(t, ok, "should be contains item %s", k)
	}

	_ = data
	//b, err = json.MarshalIndent(data, "", "\t")
	//require.Nil(t, err)
	//t.Log(string(b))
}

func TestPublicNerthusAPI_DecodeABI4(t *testing.T) {
	abiInfo := `[{"name":"","type":"string"},{"name":"","type":"uint256"}]`
	var args abi.Arguments
	err := args.UnmarshalJSON([]byte(abiInfo))
	require.Nil(t, err)
	api := NewPublicNerthusAPI(nil)
	b, err := hexutil.Decode("0x0000000000000000000000000000000000000000000000000000000000000040000000000000000000000000000000000000000000000000000000000000007b00000000000000000000000000000000000000000000000000000000000000020102000000000000000000000000000000000000000000000000000000000000")
	require.Nil(t, err)
	for _, v := range args {
		t.Log(v.Type.Kind.String(), v.Name)
	}
	data, err := api.DecodeABI(args, b)
	require.Nil(t, err)
	require.Equal(t, hexutil.Bytes{1, 2}, data["r1"])
	require.Equal(t, big.NewInt(123), data["r2"])
}

func TestPublicNerthusAPI_DecodeABI5(t *testing.T) {
	abiInfo := `[{"name": "hash", "type": "bytes32"},{"name": "opinion", "type": "bool"}]`
	var args abi.Arguments
	err := args.UnmarshalJSON([]byte(abiInfo))
	require.Nil(t, err)
	api := NewPublicNerthusAPI(nil)
	b, err := hexutil.Decode("0x6625f65e4f121a589a3b909152e6df58be61705e64e4630ba331d9d786485fb0b67a07a50000000000000000000000000000000000000000000000000000000000000000")
	require.Nil(t, err)
	for _, v := range args {
		t.Log(v.Type.Kind.String(), v.Name)
	}
	t.Log(len(b))
	data, err := api.DecodeABI(args, b)
	require.Nil(t, err)
	t.Log(data)
}
