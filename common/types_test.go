// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package common

//func TestBytesConversion(t *testing.T) {
//	bytes := []byte{5}
//	hash := BytesToHash(bytes)
//
//	var exp Hash
//	exp[31] = 5
//
//	if hash != exp {
//		t.Errorf("expected %x got %x", exp, hash)
//	}
//}
//
//func TestHashJsonValidation(t *testing.T) {
//	var tests = []struct {
//		Prefix string
//		Size   int
//		Error  string
//	}{
//		{"", 62, "json: cannot unmarshal hex string without 0x prefix into Go value of type common.Hash"},
//		{"0x", 66, "hex string has length 66, want 64 for common.Hash"},
//		{"0x", 63, "json: cannot unmarshal hex string of odd length into Go value of type common.Hash"},
//		{"0x", 0, "hex string has length 0, want 64 for common.Hash"},
//		{"0x", 64, ""},
//		{"0X", 64, ""},
//	}
//	for _, test := range tests {
//		input := `"` + test.Prefix + strings.Repeat("0", test.Size) + `"`
//		var v Hash
//		err := json.Unmarshal([]byte(input), &v)
//		if err == nil {
//			if test.Error != "" {
//				t.Errorf("%s: error mismatch: have nil, want %q", input, test.Error)
//			}
//		} else {
//			if err.Error() != test.Error {
//				t.Errorf("%s: error mismatch: have %q, want %q", input, err, test.Error)
//			}
//		}
//	}
//}

//func TestAddressUnmarshalJSON(t *testing.T) {
//	var tests = []struct {
//		Input     string
//		ShouldErr bool
//		Output    *big.Int
//	}{
//		{"", true, nil},
//		{`""`, true, nil},
//		{`"0x"`, true, nil},
//		{`"0x00"`, true, nil},
//		{`"0xG000000000000000000000000000000000000000"`, true, nil},
//		{`"0x0000000000000000000000000000000000000000"`, false, big.NewInt(0)},
//		{`"0x0000000000000000000000000000000000000010"`, false, big.NewInt(16)},
//	}
//	for i, test := range tests {
//		var v Address
//		err := json.Unmarshal([]byte(test.Input), &v)
//		if err != nil && !test.ShouldErr {
//			t.Errorf("test #%d: unexpected error: %v", i, err)
//		}
//		if err == nil {
//			if test.ShouldErr {
//				t.Errorf("test #%d: expected error, got none", i)
//			}
//			if v.Big().Cmp(test.Output) != 0 {
//				t.Errorf("test #%d: address mismatch: have %v, want %v", i, v.Big(), test.Output)
//			}
//		}
//	}
//}
//
//func TestAddressHexChecksum(t *testing.T) {
//	var tests = []struct {
//		Input  string
//		Output string
//	}{
//		// Test cases from https://github.com/ethereum/EIPs/blob/master/EIPS/eip-55.md#specification
//		{"0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed", "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed"},
//		{"0xfb6916095ca1df60bb79ce92ce3ea74c37c5d359", "0xfB6916095ca1df60bB79Ce92cE3Ea74c37c5d359"},
//		{"0xdbf03b407c01e7cd3cbea99509d93f8dddc8c6fb", "0xdbF03B407c01E7cD3CBea99509d93f8DDDC8C6FB"},
//		{"0xd1220a0cf47c7b9be7a2e6ba89f429762e7b9adb", "0xD1220A0cf47c7B9Be7A2E6BA89F429762e7b9aDb"},
//		// Ensure that non-standard length input values are handled correctly
//		{"0xa", "0x000000000000000000000000000000000000000A"},
//		{"0x0a", "0x000000000000000000000000000000000000000A"},
//		{"0x00a", "0x000000000000000000000000000000000000000A"},
//		{"0x000000000000000000000000000000000000000a", "0x000000000000000000000000000000000000000A"},
//	}
//	for i, test := range tests {
//		output := HexToAddress(test.Input).Hex()
//		if output != test.Output {
//			t.Errorf("test #%d: failed to match when it should (%s != %s)", i, output, test.Output)
//		}
//	}
//}
//
//func BenchmarkAddressHex(b *testing.B) {
//	testAddr := HexToAddress("0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed")
//	for n := 0; n < b.N; n++ {
//		testAddr.Hex()
//	}
//}
//
//func TestByteToAddresses(t *testing.T) {
//	addresses := []Address{
//		HexToAddress("0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed"),
//		HexToAddress("0xfb6916095ca1df60bb79ce92ce3ea74c37c5d359"),
//		HexToAddress("0xdbf03b407c01e7cd3cbea99509d93f8dddc8c6fb"),
//	}
//
//	byteData := AddressesToByte(addresses)
//	if len(byteData) == 0 {
//		t.Fatalf("address to byte fail")
//	}
//
//	outputAddresses, _ := ByteToAddresses(byteData)
//	if len(outputAddresses) != len(addresses) {
//		t.Fatalf("output length not equal origin")
//	}
//
//	for i := 0; i < len(addresses); i++ {
//		if addresses[i] != outputAddresses[i] {
//			t.Fatalf("addresses %v not equal output %v", addresses[i], outputAddresses[i])
//		}
//	}
//
//}
//
//func TestEmptyAddress(t *testing.T) {
//
//	var nilAddress *Address
//
//	address := []struct {
//		address interface{}
//		isEmpty bool
//	}{
//		{Address{}, true},
//		{Address{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, true},
//		{Address{0, 0, 0}, true},
//		{Address{0, 0, 0, 1}, false},
//
//		{StringToAddress(""), true},
//		{StringToAddress("abc"), false},
//		{&Address{}, true},
//		{nilAddress, true},
//	}
//
//	for _, c := range address {
//		var got bool
//		switch address := c.address.(type) {
//		case Address:
//			got = address.Empty()
//		case *Address:
//			got = address.Empty()
//		}
//		if got != c.isEmpty {
//			t.Errorf("expectet (address %+v) is empty returns %v ,unexpected %v", c.address, c.isEmpty, got)
//		}
//	}
//
//}
//
//func TestAddress_ShortString(t *testing.T) {
//	addr := HexToAddress("0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed")
//	want, got := "0x5aaeb6...1beaed", addr.ShortString()
//	if want != got {
//		t.Fatalf("expect address short string is %s,but got %s", want, got)
//	}
//}
//
//func TestAddressesSort(t *testing.T) {
//
//	list := AddressList{
//		StringToAddress("4"),
//		StringToAddress("3"),
//		StringToAddress("2"),
//		StringToAddress("5"),
//		StringToAddress("8"),
//	}
//	sort.Sort(list)
//	assert.Equal(t, StringToAddress("2"), list[0])
//	assert.Equal(t, StringToAddress("3"), list[1])
//	assert.Equal(t, StringToAddress("4"), list[2])
//	assert.Equal(t, StringToAddress("5"), list[3])
//	assert.Equal(t, StringToAddress("8"), list[4])
//}
//
//func TestAddress_Text(t *testing.T) {
//
//	var tests = []struct {
//		add1 string
//		add2 string
//	}{
//		{"0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed", "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed"},
//		{"0xfb6916095ca1df60bb79ce92ce3ea74c37c5d359", "0xfB6916095ca1df60bB79Ce92cE3Ea74c37c5d359"},
//		{"0xdbf03b407c01e7cd3cbea99509d93f8dddc8c6fb", "0xdbF03B407c01E7cD3CBea99509d93f8DDDC8C6FB"},
//		{"0xd1220a0cf47c7b9be7a2e6ba89f429762e7b9adb", "0xD1220A0cf47c7B9Be7A2E6BA89F429762e7b9aDb"},
//		// Ensure that non-standard length input values are handled correctly
//		{"0xa", "0x000000000000000000000000000000000000000A"},
//		{"0x0a", "0x000000000000000000000000000000000000000A"},
//		{"0x00a", "0x000000000000000000000000000000000000000A"},
//		{"0x000000000000000000000000000000000000000a", "0x000000000000000000000000000000000000000A"},
//	}
//
//	for _, c := range tests {
//		a1, a2 := HexToAddress(c.add1), HexToAddress(c.add2)
//		require.Equal(t, a1.Text(), a2.Text())
//	}
//}
//
//func TestAddress_JSON(t *testing.T) {
//	addr := HexToAddress("0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed")
//
//	b, err := json.Marshal(addr)
//	require.NoError(t, err)
//
//	var got Address
//	err = json.Unmarshal(b, &got)
//	require.NoError(t, err)
//	require.Equal(t, addr.Hex(), got.Hex())
//
//}
