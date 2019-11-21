// Copyright 2014 The go-ethereum Authors
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

// Package common contains various helper functions.
package common

import (
	"encoding/binary"
	"encoding/hex"

	"gitee.com/nerthus/nerthus/crypto/sha3"
	"gitee.com/nerthus/nerthus/rlp"
)

func ToHex(b []byte) string {
	hex := Bytes2Hex(b)
	// Prefer output of "0x0" instead of "0x"
	if len(hex) == 0 {
		hex = "0"
	}
	return "0x" + hex
}

func FromHex(s string) []byte {
	if len(s) > 1 {
		if s[0:2] == "0x" || s[0:2] == "0X" {
			s = s[2:]
		}
		if len(s)%2 == 1 {
			s = "0" + s
		}
		return Hex2Bytes(s)
	}
	return nil
}

// Copy bytes
//
// Returns an exact copy of the provided bytes
func CopyBytes(b []byte) (copiedBytes []byte) {
	if b == nil {
		return nil
	}
	copiedBytes = make([]byte, len(b))
	copy(copiedBytes, b)

	return
}

func HasHexPrefix(str string) bool {
	l := len(str)
	return l >= 2 && str[0:2] == "0x"
}

func IsHex(str string) bool {
	l := len(str)
	return l >= 4 && l%2 == 0 && str[0:2] == "0x"
}

func Bytes2Hex(d []byte) string {
	return hex.EncodeToString(d)
}

func Hex2Bytes(str string) []byte {
	h, _ := hex.DecodeString(str)

	return h
}

func Hex2BytesFixed(str string, flen int) []byte {
	h, _ := hex.DecodeString(str)
	if len(h) == flen {
		return h
	} else {
		if len(h) > flen {
			return h[len(h)-flen:]
		} else {
			hh := make([]byte, flen)
			copy(hh[flen-len(h):flen], h[:])
			return hh
		}
	}
}

func RightPadBytes(slice []byte, l int) []byte {
	if l <= len(slice) {
		return slice
	}

	padded := make([]byte, l)
	copy(padded, slice)

	return padded
}

func LeftPadBytes(slice []byte, l int) []byte {
	if l <= len(slice) {
		return slice
	}

	padded := make([]byte, l)
	copy(padded[l-len(slice):], slice)

	return padded
}

// Author: @zwj
// int64 转 []byte
func Uint64ToBytes(n uint64) []byte {
	var buf = make([]byte, 8)
	binary.BigEndian.PutUint64(buf, n)
	return buf
}

// []byte 转 int64
func BytesToUInt64(data []byte) uint64 {
	return binary.BigEndian.Uint64(data)
}

// 对内容进行 RLP 编码并获取哈希值，哈希值使用sha3加密
// Author: @ysqi
func SafeHash256(v ...interface{}) (h Hash) {
	// 使用rlp编码计算哈希
	hw := sha3.NewKeccak256()
	err := rlp.Encode(hw, v)
	if err != nil {
		panic(err)
	}
	hw.Sum(h[:0])
	sha3.PutKeccak256(hw)
	return h
}
