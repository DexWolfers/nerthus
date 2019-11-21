package state

import (
	"errors"
	"math/big"

	"gitee.com/nerthus/nerthus/common"
)

var (
	fragmentPrefix     = []byte("fg_")
	fragmentPrefixLast = []byte("fgl")

	ErrStateNotFind = errors.New("state not find")
)

// SetBigData 大数据存储，存储时将按拆分成小数据块存储
func (self *StateDB) SetBigData(addr common.Address, key []byte, value []byte) error {
	// 使用小key是为了防止key过长，而导致分块存储的key被截断
	//if len(key) >= common.HashLength/2 {
	//	return fmt.Errorf("key length must less than %d", common.HashLength/2)
	//}
	// todo 删掉原来的数据
	// 计算长度
	length := int64(len(value))
	size := int64(common.HashLength)
	// 计算需要分隔的块数
	number := int64(length / size)
	last := int64(length % size)
	if last > 0 {
		number++
	}
	// 分隔存储
	for i := int64(0); i < number; i++ {
		hash := common.SafeHash256(key, big.NewInt(i).Bytes())
		if last > 0 && i == number-1 {
			// 补块
			self.SetState(addr, hash, common.BytesToHash(value[i*size:(i*size)+last]))
		} else {
			self.SetState(addr, hash, common.BytesToHash(value[i*size:(i+1)*size]))
		}
	}
	// 存储数量
	self.SetState(addr, common.SafeHash256(key), common.BytesToHash(big.NewInt(number).Bytes()))
	// 存储尾部长度
	hash := common.SafeHash256(key, fragmentPrefixLast)
	self.SetState(addr, hash, common.BytesToHash(big.NewInt(last).Bytes()))
	return nil
}

// GetBigData 获取大数据
func (self *StateDB) GetBigData(addr common.Address, key []byte) ([]byte, error) {
	var result []byte
	data := self.GetState(addr, common.SafeHash256(key))
	if data.Empty() {
		return nil, ErrStateNotFind
	}
	number := data.Big().Int64()
	// 取出尾部长度
	data = self.GetState(addr, common.SafeHash256(key, fragmentPrefixLast))
	var last int64
	// 如果尾部不为空再取值
	if !data.Empty() {
		last = data.Big().Int64()
	}
	for i := int64(0); i < number; i++ {
		hash := common.SafeHash256(key, big.NewInt(i).Bytes())
		b := self.GetState(addr, hash)
		// 尾部长度不同要取出对应位置
		if last != 0 && i == number-1 {
			result = append(result, b[int64(common.HashLength)-last:]...)
		} else {
			result = append(result, b[:]...)
		}
	}
	return result, nil
}

// GetBigDataFragmentCount 获取给定Key的值所分片存储的数量
func (self *StateDB) GetBigDataFragmentCount(account common.Address, key []byte) int64 {
	data := self.GetState(account, common.BytesToHash(key))
	if data.Empty() {
		return 0
	}
	return data.Big().Int64()
}
