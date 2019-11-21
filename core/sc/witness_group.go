package sc

import (
	"errors"
	"fmt"
	"math/big"
	"sort"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"
)

var (
	errNoWitnessGroupUse = errors.New("no new witness group available for you")
)
var (
	PrefixWitnessGroupCount = common.StringToHash("witness_group_count")
	PrefixWitnessGroupInfo  = []byte("witness_group_info")
	ErrNotWitnessGroup      = errors.New("witness group is empty")

	defGroupChainPrefix          = []byte("_chains_")
	defGroupWitenssPrefix        = []byte("_witness_")
	DefSysGroupIndex      uint64 = 0
)

// ApplyWitnessGroupOfSys 系统见证人加入见证组
func ApplyWitnessGroupOfSys(db StateDB, address common.Address) (uint64, error) {
	witnessSet := GetWitnessSet(db, DefSysGroupIndex)
	l := witnessSet.Len(db)
	if l+1 > params.ConfigParamsSCWitnessCap {
		return 0, ErrMaxWitnessLimit
	}
	if l == 0 && GetWitnessGroupCount(db) == 0 { //组不存在则创建组
		WriteWitnessGroupCount(db, 1)
		//存储自己
		GetChainsSet(DefSysGroupIndex).Add(db, params.SCAccount)
	}
	witnessSet.Add(db, address)
	return DefSysGroupIndex, nil
}

// ApplyWitnessGroupOfUser 见证人加入见证组
func ApplyWitnessGroupOfUser(db StateDB, address common.Address) (uint64, error) {
	//需要寻找出所有缺失见证人的组
	// 获取所有缺失见证人的见证组
	var (
		count    = GetWitnessGroupCount(db)
		min, max = params.ConfigParamsUCMinVotings, uint64(params.ConfigParamsUCWitness)
	)

	type ChooseItem struct {
		GroupIndex uint64
		Priority   uint64 //优先级值越大优先选择此组
	}
	var list []ChooseItem

	addToGroup := func(witness common.Address, gp uint64) {
		witnessSet := GetWitnessSet(db, gp)
		witnessSet.Add(db, witness)
	}

	for gpIndex := uint64(1); gpIndex < count; gpIndex++ {
		if gpIndex == DefSysGroupIndex {
			continue
		}
		c := GetWitnessCountAt(db, gpIndex)
		if c == max { //已经足够
			continue
		}
		if c == min-1 { //刚好缺一人时，直接进入该组
			addToGroup(address, gpIndex)
			return gpIndex, nil
		}

		//人数少的组中，缺的人最少的优先级高
		//人数足够的组中，人数越少优先级越高
		// c  p, min=8,max=11
		// 0->0,1->1,2->2,3->3... 7->7
		// 8=>3,9=>2,10=>1
		var p uint64
		if c < min {
			p = c
		} else if c > min {
			p = max - c //
		}

		list = append(list, ChooseItem{
			GroupIndex: gpIndex,
			Priority:   p,
		})
	}

	if len(list) > 0 {
		//选出一个见证人人数最少的组
		sort.SliceStable(list, func(i, j int) bool {
			return list[i].Priority > list[j].Priority
		})
		addToGroup(address, list[0].GroupIndex)
		return list[0].GroupIndex, nil
	}
	if count == 0 {
		count = 1
	}
	//如果无可选组，则需要新开见证组
	gpIndex := count
	WriteWitnessGroupCount(db, count+1)
	addToGroup(address, gpIndex)
	return gpIndex, nil
}

// CancelWitnessFromWitnessGroup 见证人从见证组中注销
func CancelWitnessFromWitnessGroup(db StateDB, address common.Address, groupIndex uint64) error {
	witnessSet := GetWitnessSet(db, groupIndex)
	index, ok := witnessSet.Exist(db, address)
	if !ok {
		return errors.New("not found witness from given group")
	}
	witnessSet.RemoveOrder(db, index)
	return fixWitnessGroup(db, groupIndex)
}

// CancelChainFromWitnessGroup 从见证组中删除链地址
func CancelChainFromWitnessGroup(db StateDB, address common.Address, groupIndex uint64) error {
	chainsSet := GetChainsSet(groupIndex)
	// 当前链列表中是否存在该地址,存在则跳过
	index, ok := chainsSet.Exist(db, address)
	if !ok {
		return errors.New("not found chain address from given group")
	}
	chainsSet.Remove(db, index)
	return nil
}

type choseGroup struct {
	index        uint64 //组
	chainCount   uint64 //已分配的见证人数
	rate         uint64 //投票率
	witnessCount uint64 //该组的见证人数
}

// ApplyChainFromWitnessGroup 链加入见证组 or 链更换见证组
// oldIndex 当前链所在见证组
func ApplyChainFromWitnessGroup(db StateDB, address common.Address, number *big.Int, oldIndex *uint64, chainNumber uint64) (uint64, error) {

	witenssGroup, err := selectNewGroup(oldIndex, number, db, address)
	if err != nil {
		return 0, err
	}
	return applyWitnessWithGroup(db, address, number, oldIndex, witenssGroup, chainNumber)
}
func applyWitnessWithGroup(db StateDB, address common.Address, number *big.Int, oldIndex *uint64, newIndex *uint64, chainNumber uint64) (uint64, error) {

	// 删除原有用户组的链地址
	if oldIndex != nil {
		log.Debug("del chain is witness group old", "old index", *oldIndex, "address", address)
		CancelChainFromWitnessGroup(db, address, *oldIndex)
	}
	chainsSet := GetChainsSet(*newIndex)
	chainsSet.Add(db, address)
	WriteChainInfo(db, address, number.Uint64(), *newIndex, chainNumber)

	//更换见证人成功，添加事件
	var old uint64
	if oldIndex != nil {
		old = *oldIndex
	}
	err := AddUserWitnessChangeEventLog(db, address, old, *newIndex)
	if err != nil {
		return 0, err
	}
	log.Debug("apply chain witness group", "address", address.Hex(), "sc.number", number, "group.index", *newIndex, "group.index.old", oldIndex, "chainNumber", chainNumber)
	return *newIndex, nil
}

func fixWitnessGroup(db StateDB, removedGp uint64) error {
	//系统见证组忽略
	if removedGp == DefSysGroupIndex {
		return nil
	}
	min, max := params.ConfigParamsUCMinVotings, uint64(params.ConfigParamsUCWitness)
	//	首先，检查此移除见证人的组是否人数足够
	//   如果 count >= min ,则不需要继续
	curr := GetWitnessCountAt(db, removedGp)
	if curr == 0 {
		return nil
	}
	if curr >= min {
		need := max - curr
		//如果本组人人数不足，则检查其他组是否有可调配人。
		// 在所有人数不足的组中调配足够的人数填充
		for gpIndex := uint64(1); gpIndex < GetWitnessGroupCount(db); gpIndex++ {
			if gpIndex == removedGp { //忽略自己
				continue
			}
			c := GetWitnessCountAt(db, gpIndex)
			if c >= min { //此组人数高于最低，不能使用
				continue
			}
			//将此组人补充到当前组中
			list := GetWitnessListAt(db, gpIndex)
			for _, w := range list {
				//移动
				if err := removeWtinessGroup(db, w, gpIndex, removedGp); err != nil {
					return err
				}
				need--
				if need == 0 { //人数已足够
					break
				}
			}
			if need == 0 { //人数已足够
				break
			}
		}
		return nil
	}

	// 提取剩余的见证人
	list := GetWitnessListAt(db, removedGp)

	// 获取所有缺失见证人的见证组
	var cache []choseGroup
	for i := uint64(1); i < GetWitnessGroupCount(db); i++ {
		if i == removedGp {
			continue
		}
		c := GetWitnessCountAt(db, i)
		if c != max && c != 0 {
			cache = append(cache, choseGroup{
				index:        i,
				witnessCount: c,
			})
		}
	}
	sort.SliceStable(cache, func(i, j int) bool {
		if cache[i].witnessCount+curr == min {
			return true
		}
		return cache[i].witnessCount < cache[i].witnessCount
	})

	var times int
	for {
		if len(cache) == 0 {
			return nil
		}
		//依次补充人数

		times++
		for i := 0; i < len(cache); i++ {
			if len(list) == 0 {
				return nil
			}
			gp := cache[i]
			//在第一次分配时按最小量分配，后续按一个人分配
			//因为不能一次性全部放到某一个见证组，需要看此见证组至少需要多少人，优先满足最低要求，
			// 这样可以均衡其他见证组。
			var lessNeed uint64
			if min < gp.witnessCount {
				lessNeed = 1
			} else {
				lessNeed = min - gp.witnessCount
			}
			if times > 1 {
				lessNeed = 1
			}
			if lessNeed == 0 {
				continue
			}
			for i := 0; i < int(lessNeed); i++ {
				witness := list[0]
				if err := removeWtinessGroup(db, witness, removedGp, gp.index); err != nil {
					return err
				}
				//已无剩余
				if len(list) == 1 {
					return nil
				}
				//移除已分配
				list = list[1:]
			}
			cache[i].witnessCount += lessNeed
			if cache[i].witnessCount == max {
				if len(cache) == 1 {
					return nil
				}
				//移除分配足额的见证组
				cache[i] = cache[len(cache)-1]
				cache = cache[:len(cache)-1]
			}
		}
	}
}
func removeWtinessGroup(db StateDB, witness common.Address, oldGp, newGp uint64) error {
	if oldGp == newGp {
		return errors.New("not change witness group")
	}
	//从旧组中移除
	oldWitnessSet := GetWitnessSet(db, oldGp)
	index, ok := oldWitnessSet.Exist(db, witness)
	if !ok {
		return fmt.Errorf("bug: can not found info at set,witness %s,old group=%d", witness, oldGp)
	}
	oldWitnessSet.RemoveOrder(db, index)
	//加入新组
	witnessSet := GetWitnessSet(db, newGp)
	witnessSet.Add(db, witness)

	//更新见证人索引
	updateWitnessGroupIndex(db, witness, newGp)

	//生成事件
	return AddWitnessGroupChangedEvent(db, witness, oldGp, newGp)
}

// selectNewGroup 为链选择见证组。
// 选择需要加入特殊检查：同组见证人必须存在 8 人以上有发送一次心跳，否则将分配用户。这样避免尚未准备就绪的见证组被选中
func selectNewGroup(oldIndex *uint64, number *big.Int, db StateDB, address common.Address) (*uint64, error) {
	//防止地址为系统链
	if address == params.SCAccount {
		return nil, errors.New("disable apply witness for system chain")
	}
	// 仲裁中的链不允许选择见证人
	if number := GetChainArbitrationNumber(db, address); number > 0 {
		return nil, errors.New("chain in arbitrationing")
	}

	count := GetWitnessGroupCount(db)
	if count == 0 {
		return nil, errors.New("witness group is empty")
	}

	var (
		witenssGroup *uint64
		canChoseFrom []choseGroup

		isG      = number.Sign() == 0
		minRound = math.ForceSafeSub(params.CalcRound(number.Uint64()), 2)
	)

	for i := uint64(0); i < count; i++ {
		if i == DefSysGroupIndex || (oldIndex != nil && *oldIndex == i) {
			continue
		}
		if isG {
			return &i, nil
		}
		witnessSet := GetWitnessSet(db, i)

		witenssCount := witnessSet.Len(db)
		if witenssCount < params.ConfigParamsUCMinVotings { //见证人数不足
			continue
		}
		chainsSet := GetChainsSet(i)
		chainCount := chainsSet.Len(db)

		//空组,但必须由足够的见证人
		if chainCount == 0 {
			//如果在半小时前（2 个 round 时间）时间内容心跳不正常（尚未就绪）的将不参与分配。
			if !haveGoodHeartbeatOnGroup(db, i, minRound) {
				continue
			}
			return &i, nil
		}

		canChoseFrom = append(canChoseFrom, choseGroup{
			index:        i,
			chainCount:   chainCount,
			witnessCount: witenssCount,
		})
	}

	if len(canChoseFrom) == 0 {
		return nil, errNoWitnessGroupUse
	}
	sortWitnessGroup(canChoseFrom)

	//即使是现有见证组，如果该组在 30 分钟内心跳不正常，也不参与分配。
	for i := len(canChoseFrom) - 1; i >= 0; i-- {
		gp := canChoseFrom[i]
		//需保证该组见证人（心跳正常）。
		if haveGoodHeartbeatOnGroup(db, gp.index, minRound) {
			return &gp.index, nil
		}
	}
	//否则，默认选最后一个
	witenssGroup = &canChoseFrom[len(canChoseFrom)-1].index
	return witenssGroup, nil
}

// 在指定做是否有健康的心跳
func haveGoodHeartbeatOnGroup(db StateDB, gp uint64, minRound uint64) bool {
	if gp == DefSysGroupIndex {
		panic("can not work for system witness")
	}
	if minRound == 0 {
		return true
	}
	var good uint64
	GetWitnessSet(db, gp).Range(db, func(index uint64, value common.Hash) bool {
		//最后一次心跳要符合预期
		lastRound := QueryHeartbeatLastRound(db, types.AccUserWitness, common.BytesToAddress(value.Bytes()))
		if lastRound >= minRound {
			good++
		}
		//以符合最低票，则不再检查
		if good >= params.ConfigParamsUCMinVotings {
			return false
		}
		return true
	})
	return good >= params.ConfigParamsUCMinVotings
}

// sortWitnessGroup 见证组优先级排序
func sortWitnessGroup(canChoseFrom []choseGroup) {
	// 选组策略： less 排序，按优先级从低到高排列
	sort.SliceStable(canChoseFrom, func(i, j int) bool {
		// 有效见证人数少，则优先级低
		if canChoseFrom[i].witnessCount < canChoseFrom[j].witnessCount {
			return true
		}
		// 已有用户多，则优先级低
		if canChoseFrom[i].chainCount > canChoseFrom[j].chainCount {
			return true
		}
		// 工作效率低，则优先级低
		if canChoseFrom[i].rate < canChoseFrom[j].rate {
			return true
		}
		// 旧见证组，优先级低
		if canChoseFrom[i].index < canChoseFrom[j].index {
			return true
		}
		return false
	})
}

// GetWitnessGroupCount 获取总见证组数
func GetWitnessGroupCount(db StateDB) uint64 {
	count := db.GetState(params.SCAccount, PrefixWitnessGroupCount)
	if count.Empty() {
		return 1 //至少有系统见证人组
	}
	return count.Big().Uint64()
}

// WriteWitnessGroupCount 写入总见证组数
func WriteWitnessGroupCount(db StateDB, count uint64) {
	db.SetState(params.SCAccount, PrefixWitnessGroupCount, common.BigToHash(new(big.Int).SetUint64(count)))
}

// GetWitnessListAt 获取给定组下的见证人
func GetWitnessListAt(db StateDB, group uint64) (lib []common.Address) {
	witnessSet := GetWitnessSet(db, group)
	witnessSet.Range(db, func(index uint64, value common.Hash) bool {
		lib = append(lib, common.BytesToAddress(value.Bytes()))
		return true
	})
	return
}

// GetWitnessCountAt 获取见证组总数
func GetWitnessCountAt(db StateDB, group uint64) uint64 {
	witnessSet := GetWitnessSet(db, group)
	return witnessSet.Len(db)
}

// GetChainsAtGroup 根据见证组ID迭代
func GetChainsAtGroup(db StateDB, group uint64, f func(index uint64, chain common.Address) bool) {
	chainsSet := GetChainsSet(group)
	chainsSet.Range(db, func(index uint64, value common.Hash) bool {
		return f(index, common.BytesToAddress(value.Bytes()))
	})
}

// GetWitnessSet 一个见证人组集合
func GetWitnessSet(db StateDB, group uint64) StateSet {
	return StateSet([]interface{}{PrefixWitnessGroupInfo, group, defGroupWitenssPrefix})
}
func GetChainsSet(group uint64) StateSet {
	return StateSet([]interface{}{PrefixWitnessGroupInfo, group, defGroupChainPrefix})
}
