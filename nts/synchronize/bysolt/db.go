package bysolt

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/rlp"
	"github.com/pkg/errors"
)

var (
	lastUnitPointKey          = []byte("upo_l")
	uintPointPrefix           = []byte("upo_i_")
	unitPointUnitDetailPrefix = []byte("upo_c_")
	unitPointUnitTODOPrefix   = []byte("upo_todo_") //待更新的高度
)

func getUnitPointKey(number uint64) []byte {
	return append(uintPointPrefix, common.Uint64ToBytes(number)...)
}

func getUnitPointUpdateTodoKey(number uint64) []byte {
	return append(unitPointUnitTODOPrefix, common.Uint64ToBytes(number)...)
}

func writeUPUpdateTODO(db ntsdb.Putter, number uint64) {
	db.Put(getUnitPointUpdateTodoKey(number), []byte{1})
}

// 遍历所有待更新内容
func rangeTodos(db ntsdb.Finder, cb func(number uint64) bool) {
	db.Iterator(unitPointUnitTODOPrefix, func(key, value []byte) bool {
		return cb(common.BytesToUInt64(key[len(unitPointUnitTODOPrefix):]))
	})
}
func upUpdateDone(db ntsdb.Deleter, number uint64) {
	db.Delete(getUnitPointUpdateTodoKey(number))
}

// 获取最后一个更新点Number，Number 从 1 开始
func getLastUnitPointNumber(db ntsdb.Database) uint64 {
	data, _ := db.Get(lastUnitPointKey)
	if len(data) == 0 {
		return 0
	}
	return common.BytesToUInt64(data)
}
func writeLastUnitPointNumber(db ntsdb.Database, last uint64) {
	db.Put(lastUnitPointKey, common.Uint64ToBytes(last))
}

func getUnitPoint(db ntsdb.Database, number uint64) *Point {
	data, _ := db.Get(getUnitPointKey(number))
	if len(data) == 0 {
		return nil
	}
	point := new(Point)
	if err := rlp.DecodeBytes(data, &point); err != nil {
		panic(err)
	}
	return point
}

func writeUnitPoint(db ntsdb.Putter, point Point) error {
	if point.Number == 0 {
		return errors.New("number is zero")
	}
	b, err := rlp.EncodeToBytes(point)
	if err != nil {
		panic(err)
	}
	return db.Put(getUnitPointKey(point.Number), b)
}
