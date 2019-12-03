package util

import (
	"bytes"

	"github.com/pkg/errors"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/rawdb"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/rlp"
)

func DyncGetUnitRawRLPByIds(db ntsdb.Database, sizeLimit int, next func() types.UnitID) []byte {
	buf := bytes.NewBuffer(nil)
	var size int

	var sizeInfo []uint64

	for {
		uid := next()
		if uid.IsEmpty() {
			break
		}
		b := rawdb.GetRawUnit(db, uid.ChainID, uid.Hash, uid.Height)
		if sizeLimit > 0 {
			size += len(b)
			//保证至少可以同步一个单元
			if len(sizeInfo) > 0 && size > sizeLimit {
				break
			}
		}
		buf.Write(b)
		sizeInfo = append(sizeInfo, uint64(len(b)))
	}
	//无数据则返还空
	if buf.Len() == 0 {
		return nil
	}

	info, err := rlp.EncodeToBytes(sizeInfo)
	if err != nil {
		panic(err)
	}
	buf2 := bytes.NewBuffer(info)
	buf.WriteTo(buf2)
	return buf2.Bytes()
}

func DyncGetUnitRawRLP(db ntsdb.Database, sizeLimit int, next func() common.Hash) []byte {
	return DyncGetUnitRawRLPByIds(db, sizeLimit, func() types.UnitID {
		h := next()
		if h.Empty() {
			return types.UnitID{}
		}
		chain, number := core.GetUnitNumber(db, h)
		if chain.Empty() {
			return types.UnitID{}
		}
		return types.UnitID{
			ChainID: chain,
			Height:  number,
			Hash:    h,
		}
	})
}

// 获取多个单元的 RLP 拼接信息
func GetUnitRawRLP(db ntsdb.Database, uhashs []common.Hash, sizeLimit int) []byte {
	var index int
	return DyncGetUnitRawRLP(db, sizeLimit, func() common.Hash {
		if index < len(uhashs) {
			h := uhashs[index]
			index++
			return h
		} else {
			return common.Hash{}
		}
	})
}

func EncodeUnit(db ntsdb.Database, uid types.UnitID) []byte {
	b := rawdb.GetRawUnit(db, uid.ChainID, uid.Hash, uid.Height)
	if len(b) == 0 {
		return nil
	}
	buf := bytes.NewBuffer(nil)
	rlp.Encode(buf, []uint64{uint64(len(b))})
	buf.Write(b)
	return buf.Bytes()
}

func DecodeUnitRawRLP(b []byte) ([]*types.Unit, error) {
	buf := bytes.NewBuffer(b)

	stream := rlp.NewStream(buf, 0)
	var list []uint64
	err := stream.Decode(&list)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode []uint64")
	}
	units := make([]*types.Unit, 0, len(list))

	for _, v := range list {
		u, err := rawdb.DecodeRawUnit(buf.Next(int(v)))
		if err != nil {
			return nil, err
		}
		units = append(units, u)
	}
	return units, nil
}
