package common

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"gitee.com/nerthus/nerthus/rlp"

	"github.com/stretchr/testify/require"
)

func TestGenerateContractAddress(t *testing.T) {
	userAddr, err := NewAddress(Hash160([]byte("usera")))
	require.NoError(t, err)

	for i := uint64(0); i < 12; i++ {
		contractAddr := GenerateContractAddress(userAddr, 1, i)
		t.Log(contractAddr.String(), contractAddr.Bytes())
		require.True(t, contractAddr.IsContract())
	}
}

func TestDecodeAddress(t *testing.T) {
	userAddr, err := NewAddress(Hash160([]byte("usera")))
	require.NoError(t, err)

	userAddr2, _ := NewAddress(Hash160([]byte("usera")))
	require.Equal(t, userAddr, userAddr2)

	got, err := DecodeAddress(userAddr.String())
	require.NoError(t, err)
	require.Equal(t, userAddr.String(), got.String())
	require.Equal(t, userAddr.PubKeyHash(), got.PubKeyHash())
	require.Equal(t, userAddr.IsContract(), got.IsContract())
	t.Log(userAddr.String())

}

func TestAddress_MarshalJSON2(t *testing.T) {
	type Data struct {
		Address Address
	}
	data := Data{Address: ForceDecodeAddress("nts1vk3e9s2jz7wrlxe27wp5etq7reawgghyz56jr6")}

	b, err := json.Marshal(data)
	require.NoError(t, err)

	var got Data
	err = json.Unmarshal(b, &got)
	require.NoError(t, err)
	require.Equal(t, data.Address.String(), got.Address.String())
}

func TestAddress_MarshalJSON(t *testing.T) {

	type Data struct {
		Address Address
	}

	caces := []struct {
		addrStr string
		failed  bool
	}{
		{"nts1vk3e9s2jz7wrlxe27wp5etq7reawgghyz56jr6", false},
		{"nts1fak4z0y4nf7nqscw45e6wq8feazm7vpes4f5hm", false},
		{"nts1h2gnkd39qyyun6nehasqgh30plw7q7rf7af40h", false},
		{"nts1tyecyrjhj3wra2sr48tcqnsm72ec92pyd4d460", false},
		{"nts1ahmzemdxqvv4lcdl9gl4qs9k2ladhgqrcfta0q", false},
		{"nts1l35g7jykqvjye5xpnjwmkn686a4359ldxe00q8", false},
		{"nts17snnwr2qvylxp02eg07junxlxye64e8c94lchs", false},
		{"nts1kj5330l40d33txllcapvp64p8jarqvv8gph8hd", false},
		{"nts1fxqn3cj9u7q024pt9hmkp278eruspgydfhm5vh", false},
		{"nts13ueqyj827qpclt2xh0lzn4kqszfdpcsds66xdt", false},
		{"nts1vk3e9s2jz7wrlxe27wp5etq7reawgghyz56jr6", false},

		{"ntt1vk3e9s2jz7wrlxe27wp5etq7reawgghyz56jr6", true},

		{emptyAddrJsonDecode, false}, //empty
	}
	for _, c := range caces {

		jsonOutput := fmt.Sprintf("%q", c.addrStr)
		var addr Address

		err := json.Unmarshal([]byte(jsonOutput), &addr)
		if c.failed {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			got, err := json.Marshal(addr)
			require.NoError(t, err)
			require.Equal(t, jsonOutput, string(got))
		}
	}
}

func TestAddress_Empty(t *testing.T) {
	var addr Address
	require.True(t, addr.Empty())

	require.Equal(t, emptyAddrJsonDecode, addr.String())

}
func TestEmptyAddressStr(t *testing.T) {

	addr, err := DecodeAddress("nts1vk3e9s2jz7wrlxe27wp5etq7reawgghyz56jr6")
	require.NoError(t, err)
	require.Equal(t, len(addr.String()), len(emptyAddrJsonDecode))
}
func TestAdressFormat(t *testing.T) {
	addr := ForceDecodeAddress("nts1vk3e9s2jz7wrlxe27wp5etq7reawgghyz56jr6")

	require.Equal(t, addr.String(), fmt.Sprintf("%x", addr))
}

func TestAddressEqual(t *testing.T) {
	addr1 := ForceDecodeAddress("nts1vk3e9s2jz7wrlxe27wp5etq7reawgghyz56jr6")
	addr2 := ForceDecodeAddress("nts1vk3e9s2jz7wrlxe27wp5etq7reawgghyz56jr6")
	require.True(t, addr1.Equal(addr2))
	require.True(t, addr1 == addr2)

	t.Log(addr1.String())
	require.True(t, addr1 == addr2)

	addr3 := ForceDecodeAddress("nts1ahmzemdxqvv4lcdl9gl4qs9k2ladhgqrcfta0q")
	require.False(t, addr1.Equal(addr3))
	require.False(t, addr1 == addr3)

	//update
	addr3.Set(addr2)
	require.True(t, addr1 == addr3)
}

func BenchmarkAddress_String(b *testing.B) {
	userAddr, err := NewAddress(Hash160([]byte("usera")))
	require.NoError(b, err)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = userAddr.String()
	}
}
func TestForceDecodeAddress(t *testing.T) {

	addr1 := ForceDecodeAddress("nts1zupk5lmc84r2dh738a9g3zscavannjy32kr2h7")

	pub := Hex2Bytes("037db227d7094ce215c3a0f57e1bcc732551fe351f94249471934567e0f5dc1bf7")
	addr2, err := NewAddress(pub)
	require.NoError(t, err)

	addr3 := BytesToAddress(addr1.Bytes())

	require.Equal(t, addr1, addr2)
	require.Equal(t, addr2, addr3)
}

func TestRefelctAddress(t *testing.T) {

	addr := BytesToAddress([]byte{1})
	t.Log(addr.String())

	value := reflect.ValueOf(addr)
	for _, rv := range value.MethodByName("Bytes").Call(nil) {
		t.Log(rv.Bytes())
		t.Log(LeftPadBytes(rv.Bytes(), 32))
	}
}

func TestAddress_DecodeRLP(t *testing.T) {
	addr1 := ForceDecodeAddress("nts1vk3e9s2jz7wrlxe27wp5etq7reawgghyz56jr6")

	t.Run("pointer", func(t *testing.T) {
		b, err := rlp.EncodeToBytes(&addr1)
		require.NoError(t, err)

		var got Address
		err = rlp.DecodeBytes(b, &got)
		require.NoError(t, err)
		require.Equal(t, addr1.String(), got.String())
		t.Log(got.String())

	})

	t.Run("value filed", func(t *testing.T) {
		type data struct {
			Chain Address
		}

		d := data{Chain: addr1}

		b, err := rlp.EncodeToBytes(d)
		require.NoError(t, err)

		var got data
		err = rlp.DecodeBytes(b, &got)
		require.NoError(t, err)
		require.Equal(t, addr1.String(), got.Chain.String())

	})

}

func TestAddress_String(t *testing.T) {
	addr1 := ForceDecodeAddress("nts1vk3e9s2jz7wrlxe27wp5etq7reawgghyz56jr6")

	str := addr1.String()

	addr2 := addr1

	require.Equal(t, str, addr2.String())

	t.Run("cache", func(t *testing.T) {
		//getAdd := func(a Address) Address {
		//	return a
		//}
		//addr1 := ForceDecodeAddress("nts1vk3e9s2jz7wrlxe27wp5etq7reawgghyz56jr6")
		//require.NotNil(t, addr1.str.Load())
		//
		//a2 := getAdd(addr1)
		//require.NotNil(t, a2.str.Load())
	})
}
