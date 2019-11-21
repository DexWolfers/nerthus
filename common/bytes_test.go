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

package common

import (
	"bytes"
	"reflect"
	"testing"
)

func TestCopyBytes(t *testing.T) {
	data1 := []byte{1, 2, 3, 4}
	exp1 := []byte{1, 2, 3, 4}
	res1 := CopyBytes(data1)

	if ok := reflect.DeepEqual(res1, exp1); !ok {
		t.Errorf("want %v got %v ", exp1, res1)
	}
}

func TestIsHex(t *testing.T) {
	data1 := "a9e67e"
	exp1 := false
	res1 := IsHex(data1)

	if ok := reflect.DeepEqual(res1, exp1); !ok {
		t.Errorf("want %v got %v ", exp1, res1)
	}

	data2 := "0xa9e67e00"
	exp2 := true
	res2 := IsHex(data2)
	if ok := reflect.DeepEqual(res2, exp2); !ok {
		t.Errorf("want %v got %v ", exp2, res2)
	}
}

func TestLeftPadBytes(t *testing.T) {
	val1 := []byte{1, 2, 3, 4}
	exp1 := []byte{0, 0, 0, 0, 1, 2, 3, 4}

	res1 := LeftPadBytes(val1, 8)
	res2 := LeftPadBytes(val1, 2)

	if ok := reflect.DeepEqual(res1, exp1); !ok {
		t.Errorf("want %v got %v ", exp1, res1)
	}

	if ok := reflect.DeepEqual(res2, val1); !ok {
		t.Errorf("want %v got %v ", val1, res2)
	}
}

func TestRightPadBytes(t *testing.T) {
	val := []byte{1, 2, 3, 4}
	exp := []byte{1, 2, 3, 4, 0, 0, 0, 0}

	resstd := RightPadBytes(val, 8)
	resshrt := RightPadBytes(val, 2)

	if ok := reflect.DeepEqual(resstd, exp); !ok {
		t.Errorf("want %v got %v ", exp, resstd)
	}

	if ok := reflect.DeepEqual(resshrt, val); !ok {
		t.Errorf("want %v got %v ", val, resshrt)
	}
}

func TestFromHex(t *testing.T) {
	input := "0x01"
	expected := []byte{1}
	result := FromHex(input)
	if !bytes.Equal(expected, result) {
		t.Errorf("Expected % x got % x", expected, result)
	}
}

func TestFromHexOddLength(t *testing.T) {
	input := "0x1"
	expected := []byte{1}
	result := FromHex(input)
	if !bytes.Equal(expected, result) {
		t.Errorf("Expected % x got % x", expected, result)
	}
}

// Author: @zwj
func TestInt64ToBytes(t *testing.T) {
	var a uint64 = 10000
	b := Uint64ToBytes(a)
	c := BytesToUInt64(b)
	if a != c {
		t.Errorf("Expect %d got %d", a, c)
	}
}
