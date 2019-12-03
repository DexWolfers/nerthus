package bytime

import "gitee.com/nerthus/nerthus/nts/synchronize/message"

type Code = message.Code

const (
	CodeFetchUnitsByTime Code = iota + 10 //请求单元
	CodeFetchUnitsResponse
)

// 拉取单元请求消息
type FetchUnitsByTime struct {
	Start, End uint64 //单元起始和结束时间，将包括 Start 上的单元
}

func (req FetchUnitsByTime) Check() error {
	//TODO: 需要检查消息合法性
	return nil
}
