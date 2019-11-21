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
	"fmt"
)

type StorageSize uint64

func (s StorageSize) String() string {
	if s > 1000000 {
		return fmt.Sprintf("%dM", s/1000000)
	} else if s > 1000 {
		return fmt.Sprintf("%dKB", s/1000)
	} else {
		return fmt.Sprintf("%dB", s)
	}
}

func (s StorageSize) Empty() bool {
	return s == 0
}

func (s StorageSize) Int64() int64 {
	return int64(s)
}

func (c *StorageSize) Write(b []byte) (int, error) {
	l := len(b)
	*c += StorageSize(l)
	return l, nil
}
