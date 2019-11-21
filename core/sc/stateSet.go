package sc

import (
	"fmt"
	"math/big"
	"reflect"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/params"
)

var (
	defSetLengthPrefix = []byte("_length")
	defSetIndexPrefix  = []byte("_value_")
	defSetValuePrefix  = []byte("_index_")
)

type StateSet []interface{}

func (s StateSet) subKey(key ...interface{}) common.Hash {
	return common.SafeHash256(append(s, key...))
}

func (s StateSet) Len(db StateDB) uint64 {
	return db.GetState(params.SCAccount, s.subKey(defSetLengthPrefix)).Big().Uint64()
}

// subLen  长度减1
func (s StateSet) subLen(db StateDB) {
	l := s.Len(db)
	if l <= 0 {
		return
	}
	l -= 1
	if l == 0 {
		db.SetState(params.SCAccount, s.subKey(defSetLengthPrefix), common.Hash{}) //clear
	} else {
		db.SetState(params.SCAccount, s.subKey(defSetLengthPrefix), uint2Hash(l))
	}
}

// addLen 长度加1
func (s StateSet) addLen(db StateDB) uint64 {
	l := s.Len(db) + 1
	db.SetState(params.SCAccount, s.subKey(defSetLengthPrefix), uint2Hash(l))
	return l
}

func (s StateSet) update(db StateDB, index uint64, value interface{}) {
	v := value2hash(value)
	// key: index ,value: value
	db.SetState(params.SCAccount, s.subKey(defSetIndexPrefix, index), v)
	// key: value ,value: index+1 , 当 index =0 时将被作为空处理，所以加1记录
	db.SetState(params.SCAccount, s.subKey(defSetValuePrefix, v), common.BigToHash(new(big.Int).SetUint64(index+1)))
}

func (s StateSet) clear(db StateDB, index uint64, valueKey common.Hash) {
	db.SetState(params.SCAccount, s.subKey(defSetIndexPrefix, index), common.Hash{})    //delete
	db.SetState(params.SCAccount, s.subKey(defSetValuePrefix, valueKey), common.Hash{}) //delete value
}

// Add 添加新项到集合，返回该项所在位置信息
func (s StateSet) Add(db StateDB, value interface{}) uint64 {
	newLen := s.addLen(db)
	index := newLen - 1
	s.update(db, index, value)
	return index
}

// Get 根据索引获取hash
func (s StateSet) Get(db StateDB, index uint64) common.Hash {
	return db.GetState(params.SCAccount, s.subKey(defSetIndexPrefix, index))
}

// Range 遍历所有元素，函数f返回false将终止遍历，否则继续直到所有
func (s StateSet) Range(db StateDB, f func(index uint64, value common.Hash) bool) {
	l := s.Len(db)
	for i := uint64(0); i < l; i++ {
		if !f(i, s.Get(db, i)) {
			break
		}
	}
}

// Exist 返回值是否存在于集合中，如果存在则返回所在位置
func (s StateSet) Exist(db StateDB, value interface{}) (uint64, bool) {
	v := db.GetState(params.SCAccount, s.subKey(defSetValuePrefix, value2hash(value)))
	if v.Empty() {
		return 0, false
	}
	return v.Big().Uint64() - 1, true
}

// Remove 删除索引对应的数据, 并把最后一个的数据移动到删除的索引上,为了不破坏现有的索引对应的数据
func (s StateSet) Remove(db StateDB, index uint64) common.Hash {
	l := s.Len(db)
	if l == 0 || index >= l { // 为空则不需要继续
		return common.Hash{}
	}
	removedValue := s.Get(db, index)
	if index == l-1 { //如果是移除末尾，则可以直接移除
		s.clear(db, index, removedValue)
		s.subLen(db)
		return removedValue
	}
	//否则是移除列表中间的项，则将末尾项替换移动的位置
	//  | A | B | C | D |
	// 		如果移除B，则最终结果是：| A | D | C |
	last := s.Get(db, l-1)
	s.update(db, index, last)
	s.clear(db, l-1, removedValue)
	s.subLen(db)
	return removedValue
}

// Remove 删除索引对应的数据，并保证集合的顺序
func (s *StateSet) RemoveOrder(db StateDB, index uint64) common.Hash {
	l := s.Len(db)
	if l == 0 || index >= l { // 为空则不需要继续
		return common.Hash{}
	}

	// remove old
	removedValue := s.Get(db, index)
	s.clear(db, index, removedValue)

	if index == l-1 { //如果是移除末尾，则可以直接移除
		s.subLen(db)
		return removedValue
	}
	for i := index + 1; i < l; i++ {
		value := s.Get(db, i)
		if i == l-1 {
			s.clear(db, i, value)
		}
		newIndex := i - 1
		s.update(db, newIndex, value)

	}
	s.subLen(db)
	return removedValue
}

type SubItems []interface{}

// subKey 生成db key
func (s SubItems) subKey(key ...interface{}) common.Hash {
	return common.SafeHash256(append(s, key...))
}

func (s SubItems) GetSub(db StateGetter, key interface{}) common.Hash {
	return db.GetState(params.SCAccount, s.subKey(key))
}
func (s SubItems) SaveSub(db StateDB, key, value interface{}) {
	db.SetState(params.SCAccount, s.subKey(key), value2hash(value))
}
func (s SubItems) SaveBigData(db StateDB, key interface{}, value []byte) {
	db.SetBigData(params.SCAccount, s.subKey(key).Bytes(), value)
}
func (s SubItems) GetBigData(db StateDB, key interface{}) ([]byte, error) {
	v, err := db.GetBigData(params.SCAccount, s.subKey(key).Bytes())
	if err == state.ErrStateNotFind {
		return []byte{}, nil
	}
	return v, err
}
func (s SubItems) DeleteSub(db StateDB, key interface{}) {
	db.SetState(params.SCAccount, s.subKey(key), common.Hash{})
}

func (s SubItems) Join(key ...interface{}) SubItems {
	return SubItems(append(s, key...))
}

// uint2Hash uint64转化为hash
func uint2Hash(u64 uint64) common.Hash {
	return common.BigToHash(new(big.Int).SetUint64(u64))
}
func hash2uint(v common.Hash) uint64 {
	return v.Big().Uint64()
}

// value2hash value转化为hash
func value2hash(value interface{}) common.Hash {
	switch v := value.(type) {
	case uint64:
		return uint2Hash(v)
	case *big.Int:
		return common.BigToHash(v)
	case byte:
		return common.BytesToHash([]byte{v})
	case WitnessStatus:
		return common.BytesToHash([]byte{byte(v)})
	case ChainStatus:
		return common.BytesToHash([]byte{byte(v)})
	case common.Hash:
		return v
	case string:
		return common.StringToHash(v)
	case common.Address:
		return v.Hash()
	case CouncilStatus, VotingResult, ProposalType, ProposalStatus:
		return uint2Hash(reflect.ValueOf(value).Uint())
	default:
		panic(fmt.Errorf("can not support convert value(%T) to Hash", value))
	}
}

func ClearSet(db StateDB, set StateSet) {
	//从尾部开始移除，效率最高
	for last := set.Len(db); last > 0; last = set.Len(db) {
		set.Remove(db, last-1)
	}
}
