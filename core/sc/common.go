package sc

import (
	"encoding/binary"
	"math/big"

	"gitee.com/nerthus/nerthus/common"
)

// encodeUint64 uint64转化为字节数组
func encodeUint64(value uint64) []byte {
	enc := make([]byte, 8)
	binary.BigEndian.PutUint64(enc, value)
	return enc
}

// decodeUint64 字节数组转化为uint64
func decodeUint64(value []byte) uint64 {
	return binary.BigEndian.Uint64(value[len(value)-8:])
}

// MakeKey 拼接成字节数组
func MakeKey(data ...interface{}) []byte {
	length := len(data)
	var out []byte
	for i := 0; i < length; i++ {
		switch val := data[i].(type) {
		case int:
			out = append(out, encodeUint64(uint64(val))...)
		case int64:
			out = append(out, encodeUint64(uint64(val))...)
		case uint64:
			out = append(out, encodeUint64(uint64(val))...)
		case string:
			out = append(out, []byte(val)...)
		case []byte:
			out = append(out, val...)
		case *big.Int:
			out = append(out, val.Bytes()...)
		case common.Address:
			out = append(out, val.Bytes()...)
		case common.Hash:
			out = append(out, val.Bytes()...)
		}
	}
	return out
}
