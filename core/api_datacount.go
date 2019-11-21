// 数据统计API

package core

import (
	"gitee.com/nerthus/nerthus/core/state"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/params"
	"github.com/pkg/errors"
)

// 见证人相关交易数据统计
var (
	wtsInfoPrefix         = []byte("wts")
	wtsPeriodReportSuffix = []byte("_prep") //见证人周期状态数据
)

// GetWitnessReport 获取见证人指定周期的见证数据报表
func (self *DagChain) GetWitnessReport(witness common.Address, period uint64) types.WitnessPeriodWorkInfo {
	info := sc.GetWitnessPeriodWorkReport(self.chainDB, period, witness)
	if info == nil {
		return types.WitnessPeriodWorkInfo{}
	}
	return *info
}
func (self *DagChain) GetWitnessWorkReport(period uint64) map[common.Address]types.WitnessPeriodWorkInfo {
	return sc.GetWitnessPeriodWorkReports(self.chainDB, period)
}

// GetWitnessAllUser 获取见证人当前的链列表
func (self *DAG) GetWitnessCurrentUsers(witness common.Address, pageIndex, pageSize uint64) ([]common.Address, error) {
	// 取出系统链数据
	stateDB, err := self.dagchain.GetChainTailState(params.SCAccount)
	if err != nil {
		return nil, err
	}
	return self.GetWitnessListAt(stateDB, witness, pageIndex, pageSize)
}

func (self *DAG) GetWitnessListAt(stateDB *state.StateDB, witness common.Address, pageIndex, pageSize uint64) ([]common.Address, error) {
	// 否则，继续查看是否是用户见证人
	info, err := sc.GetWitnessInfo(stateDB, witness)
	if err != nil {
		return nil, err
	}
	// 如果已经是非正常账户，则返回错误
	if info.Status != sc.WitnessNormal {
		return nil, errors.New("is not active witness now")
	}
	var list []common.Address
	// 系统见证人分主见证人和备选见证人,备选见证人不需要工作
	if info.GroupIndex == sc.DefSysGroupIndex {
		_, witnessList, err := sc.GetSystemMainWitness(stateDB)
		if err != nil {
			return nil, err
		}
		if !witnessList.Have(witness) {
			return list, nil
		}
	}
	sc.GetChainsAtGroup(stateDB, info.GroupIndex, func(index uint64, chain common.Address) bool {
		if pageIndex > 0 && index < pageIndex*pageSize {
			return true
		}
		list = append(list, chain)
		if uint64(len(list)) >= pageSize {
			return false
		}
		return true
	})
	return list, nil
}
