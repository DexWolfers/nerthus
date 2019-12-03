package ntsdb

import (
	"encoding/binary"
)

var (
	defDataSetPrefix   = []byte("__ds_")
	defSetLengthPrefix = []byte("_length")
	defSetIndexPrefix  = []byte("_i_")
	defSetKeyPrefix    = []byte("_k_")
	defSetValuePrefix  = []byte("_v_")
)

func uint32ToBytes(n uint32) []byte {
	var buf = make([]byte, 4)
	binary.BigEndian.PutUint32(buf, n)
	return buf
}

// []byte 转 int32
func bytesToUInt32(data []byte) uint32 {
	if len(data) == 0 {
		return 0
	}
	return binary.BigEndian.Uint32(data)
}

//一种适合小数据集的有序存储
type DataSet []byte

func NewSet(prefix []byte) DataSet {
	if len(prefix) == 0 {
		panic("prefix is empty")
	}
	return DataSet(append(defDataSetPrefix, prefix...))
}

func (s DataSet) subKey(key ...[]byte) []byte {
	var k []byte
	k = append(k, s...)
	for _, v := range key {
		k = append(k, v...)
	}
	return k
}

func (s DataSet) Len(db Database) uint32 {
	v, _ := db.Get(s.subKey(defSetLengthPrefix))

	return bytesToUInt32(v)
}

// subLen  长度减1
func (s DataSet) subLen(db Database) {
	l := s.Len(db)
	if l <= 0 {
		return
	}
	db.Put(s.subKey(defSetLengthPrefix), uint32ToBytes(l-1))
}

// addLen 长度加1
func (s DataSet) addLen(db Database) uint32 {
	l := s.Len(db)
	db.Put(s.subKey(defSetLengthPrefix), uint32ToBytes(l+1))
	return l
}

func (s DataSet) update(db Database, index uint32, key, value []byte) {
	// key: index ,value: value
	db.Put(s.subKey(defSetIndexPrefix, uint32ToBytes(index)), key)

	// key: value ,value: index+1 , 当 index =0 时将被作为空处理，所以加1记录
	db.Put(s.subKey(defSetKeyPrefix, key), uint32ToBytes(index+1))

	if len(value) > 0 {
		db.Put(s.subKey(defSetValuePrefix, key), value) //存储值
	}
}

func (s DataSet) clear(db Database, index uint32, key []byte) {
	db.Put(s.subKey(defSetIndexPrefix, uint32ToBytes(index)), nil) //delete index
	db.Put(s.subKey(defSetKeyPrefix, key), nil)                    //delete key
	db.Put(s.subKey(defSetValuePrefix, key), nil)                  //delete value
}

func (s DataSet) Add(db Database, key []byte) uint32 {
	return s.AddWithValue(db, key, nil)
}

// Add 添加新项到集合，返回该项所在位置信息
func (s DataSet) AddWithValue(db Database, key, value []byte) uint32 {
	index, ok := s.Exist(db, key)
	if ok {
		return index
	}
	newLen := s.addLen(db)
	s.update(db, newLen, key, value)
	return newLen
}

// Get 根据索引获取hash
func (s DataSet) GetKey(db Database, index uint32) []byte {
	v, err := db.Get(s.subKey(defSetIndexPrefix, uint32ToBytes(index)))
	_ = err
	return v
}

// 根据 Key 获取 Value
func (s DataSet) GetValue(db Database, key []byte) []byte {
	v, err := db.Get(s.subKey(defSetValuePrefix, key))
	_ = err
	return v
}

// Range 遍历所有元素，函数f返回false将终止遍历，否则继续直到所有
func (s DataSet) Range(db Database, f func(index uint32, key []byte) bool) {
	l := s.Len(db)
	maxIndex := l
	for i := uint32(0); i < maxIndex; i++ {
		key := s.GetKey(db, i)
		if len(key) == 0 {
			break
		}
		if !f(i, key) {
			break
		}
	}
}

// Exist 返回值是否存在于集合中，如果存在则返回所在位置
func (s DataSet) Exist(db Database, key []byte) (uint32, bool) {
	v, err := db.Get(s.subKey(defSetKeyPrefix, key))
	_ = err
	if len(v) == 0 {
		return 0, false
	}
	return bytesToUInt32(v) - 1, true
}

// Remove 删除索引对应的数据, 并把最后一个的数据移动到删除的索引上,为了不破坏现有的索引对应的数据
func (s *DataSet) RemoveByIndex(db Database, index uint32) []byte {
	sLen := s.Len(db)
	if sLen == 0 {
		return nil
	}

	removedKey := s.GetKey(db, index)
	if len(removedKey) == 0 {
		return nil
	}
	if sLen-1 == index {
		s.clear(db, index, removedKey)
	} else {
		lastIndex := s.Len(db) - 1
		lastKey := s.GetKey(db, lastIndex)
		lastValue := s.GetValue(db, lastKey)
		s.clear(db, lastIndex, lastKey)
		s.clear(db, index, removedKey)
		s.update(db, index, lastKey, lastValue)
	}
	s.subLen(db)
	return removedKey
}
func (s *DataSet) Remove(db Database, key []byte) bool {
	index, ok := s.Exist(db, key)
	if !ok {
		return false
	}
	s.RemoveByIndex(db, index)
	return true
}
