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

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/big"
	"math/rand"
	"reflect"

	"gitee.com/nerthus/nerthus/common/hexutil"
	"gitee.com/nerthus/nerthus/rlp"
)

const (
	HashLength    = 32
	AddressLength = 20
)

const (
	emptyAddrJsonDecode = "nts100000000000000000000000000000000000000"
)

var (
	hashT = reflect.TypeOf(Hash{})
)

// Hash represents the 32 byte Keccak256 hash of arbitrary data.
type Hash [HashLength]byte

func BytesToHash(b []byte) Hash {
	var h Hash
	h.SetBytes(b)
	return h
}

func StringToHash(s string) Hash   { return BytesToHash([]byte(s)) }
func BigToHash(b *big.Int) Hash    { return BytesToHash(b.Bytes()) }
func HexToHash(s string) Hash      { return BytesToHash(FromHex(s)) }
func U64ToBig(u64 uint64) *big.Int { return new(big.Int).SetUint64(u64) }

// Empty 是否是空
func (h Hash) Empty() bool { return h == Hash{} }

// Get the string representation of the underlying hash
func (h Hash) Str() string      { return string(h[:]) }
func (h Hash) Bytes() []byte    { return h[:] }
func (h Hash) Big() *big.Int    { return new(big.Int).SetBytes(h[:]) }
func (h Hash) Hex() string      { return hexutil.Encode(h[:]) }
func (h Hash) Address() Address { return BytesToAddress(h.Bytes()) }

// TerminalString implements log.TerminalStringer, formatting a string for console
// output during logging.
func (h Hash) TerminalString() string {
	return fmt.Sprintf("0x%x…%x", h[:3], h[29:])
}

// String implements the stringer interface and is used also by the logger when
// doing full logging into a file.
func (h Hash) String() string {
	return h.Hex()
}

// Format implements fmt.Formatter, forcing the byte slice to be formatted as is,
// without going through the stringer interface used for logging.
func (h Hash) Format(s fmt.State, c rune) {
	fmt.Fprintf(s, "%"+string(c), h[:])
}

// UnmarshalText parses a hash in hex syntax.
func (h *Hash) UnmarshalText(input []byte) error {
	return hexutil.UnmarshalFixedText("Hash", input, h[:])
}

// UnmarshalJSON parses a hash in hex syntax.
func (h *Hash) UnmarshalJSON(input []byte) error {
	return hexutil.UnmarshalFixedJSON(hashT, input, h[:])
}

// MarshalText returns the hex representation of h.
func (h Hash) MarshalText() ([]byte, error) {
	return hexutil.Bytes(h[:]).MarshalText()
}

// Sets the hash to the value of b. If b is larger than len(h), 'b' will be cropped (from the left).
func (h *Hash) SetBytes(b []byte) {
	if len(b) > len(h) {
		b = b[len(b)-HashLength:]
	}

	copy(h[HashLength-len(b):], b)
}

// Set string `s` to h. If s is larger than len(h) s will be cropped (from left) to fit.
func (h *Hash) SetString(s string) { h.SetBytes([]byte(s)) }

// Sets h to other
func (h *Hash) Set(other Hash) {
	for i, v := range other {
		h[i] = v
	}
}

// Generate implements testing/quick.Generator.
func (h Hash) Generate(rand *rand.Rand, size int) reflect.Value {
	m := rand.Intn(len(h))
	for i := len(h) - 1; i > m; i-- {
		h[i] = byte(rand.Uint32())
	}
	return reflect.ValueOf(h)
}

func EmptyHash(h Hash) bool {
	return h == Hash{}
}

// UnprefixedHash allows marshaling a Hash without 0x prefix.
type UnprefixedHash Hash

// UnmarshalText decodes the hash from hex. The 0x prefix is optional.
func (h *UnprefixedHash) UnmarshalText(input []byte) error {
	return hexutil.UnmarshalFixedUnprefixedText("UnprefixedHash", input, h[:])
}

// MarshalText encodes the hash as hex.
func (h UnprefixedHash) MarshalText() ([]byte, error) {
	return []byte(hex.EncodeToString(h[:])), nil
}

var EmptyAddress Address

type AddressList []Address

func (l AddressList) String() string {
	b := bytes.NewBufferString("[")
	for i, item := range l {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(item.String())
	}
	b.WriteString("]")
	return b.String()
}

func (l AddressList) Len() int {
	return len(l)
}
func (l AddressList) GetRlp(i int) []byte {
	enc, _ := rlp.EncodeToBytes(l[i])
	return enc
}

func (l AddressList) Less(i, j int) bool {
	return l[i].Big().Cmp(l[j].Big()) < 0
}
func (l AddressList) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

func (l AddressList) Have(address Address) bool {
	for _, v := range l {
		if v == address {
			return true
		}
	}
	return false
}

func (l AddressList) HaveIdx(address Address) int {
	for i, v := range l {
		if v == address {
			return i
		}
	}
	return -1
}

type HashList []Hash

func (h HashList) Have(hash Hash) bool {
	for _, v := range h {
		if v == hash {
			return true
		}
	}
	return false
}
