package message

import (
	"errors"

	"gitee.com/nerthus/nerthus/core/types"
)

// 消息处理接口
type HandleMsg func(msg *Message) error

// 拉取单元响应消息
type FetchUnitsResponse struct {
	UnitsRLP []byte //RLP格式的单元数组
}

type FetchUnitById struct {
	UID types.UnitID //缺失的单元 ID
}

func (f FetchUnitById) Check() error {
	if f.UID.ChainID.Empty() {
		return errors.New("invalid response data,chain is empty")
	}
	if f.UID.Height == 0 {
		return errors.New("invalid reqsponse data,number is zero")
	}
	return nil
}
