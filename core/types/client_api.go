package types

// code 0 表示操作成功
// code 非0 表示有错误

// 结果结构体
// Author:@kulics
type ClientApiResult struct {
	Code    string      `json:"code"`
	Message string      `json:"message,omitempty"`
	Payload interface{} `json:"payload,omitempty"`
}

type PageList struct {
	List []interface{} `json:"list"`
	Sum  uint64        `json:"sum"`
}

func NewPageList() *PageList {
	return &PageList{
		List: make([]interface{}, 0),
		Sum:  0,
	}
}

// Add 添加数据
func (pl *PageList) Add(l interface{}) {
	pl.Sum++
	pl.List = append(pl.List, l)
}

func (pl *PageList) Len() uint64 {
	return uint64(len(pl.List))
}

// Page 返回列表 index从1开始,所以需要-1
func (pl *PageList) Page(index, size uint64) *PageList {
	if pl.Sum == 0 || len(pl.List) == 0 {
		return pl
	}
	if index <= 0 {
		index = 1
	}
	index = (index - 1) * size
	end := index + size
	if pl.Sum < index {
		pl.List = pl.List[0:0]
	} else {
		if pl.Sum > end {
			pl.List = pl.List[index:end]
		} else {
			pl.List = pl.List[index:]
		}
	}
	return pl
}
