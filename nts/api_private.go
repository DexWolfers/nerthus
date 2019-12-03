package nts

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/rawdb"
	"github.com/pkg/errors"
)

type PrivateNerthusAPI struct {
	n *Nerthus
}

func NewPrivateNerthusAPI(e *Nerthus) *PrivateNerthusAPI {
	return &PrivateNerthusAPI{e}
}

// 获取从起始时间戳开始的 count 个单元哈希，并返回最后一个哈希的时间戳
func (api *PrivateNerthusAPI) GetUnitHashs(startTimestamp uint64, limit int) (interface{}, error) {
	if limit == 0 {
		return nil, errors.New("invalid limit")
	}
	var result = struct {
		EndTimestamp uint64        `json:"endTimestamp"`
		Units        []common.Hash `json:"units"`
	}{}
	result.Units = rawdb.UnitsByTimeLine(api.n.ChainDb(), startTimestamp, limit)
	if len(result.Units) > 0 {
		result.EndTimestamp = api.n.DagChain().GetHeader(result.Units[len(result.Units)-1]).Timestamp
	}
	return result, nil
}
