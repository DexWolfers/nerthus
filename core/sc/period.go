package sc

import "gitee.com/nerthus/nerthus/common/math"

type PeriodInfo struct {
	db StateDB
}

func safeAdd(a uint64, b int) uint64 {
	if b >= 0 {
		return a + uint64(b)
	}
	return math.ForceSafeSub(a, uint64(-b))
}

// 添加见证人人数统计
func (p PeriodInfo) WitnessCount(isSystem bool, period uint64) uint64 {
	if isSystem {
		return p.getCount("sys_witness", period)
	}
	return p.getCount("user_witness", period)
}

// 增加/减少指定周期下见证人数量
func (p PeriodInfo) AddWitnessCount(period uint64, isSystemWitness bool, n int) {
	if n == 0 {
		return
	}
	var key string
	if isSystemWitness {
		key = "sys_witness"
	} else {
		key = "user_witness"
	}
	p.updateCount(key, period, safeAdd(p.getCount(key, period), n))
}

func (p PeriodInfo) CouncilCount(period uint64) uint64 {
	return p.getCount("council", period)
}

// 增加/减少指定周期下理事人数量
func (p PeriodInfo) AddCouncilCount(period uint64, n int) {
	if n == 0 {
		return
	}
	p.updateCouncilCount(period, safeAdd(p.CouncilCount(period), n))
}

func (p PeriodInfo) updateCouncilCount(period, count uint64) {
	p.updateCount("council", period, count)
}

func (p PeriodInfo) getCount(key string, period uint64) uint64 {
	item := SubItems([]interface{}{[]byte("periodInfo")})
	k := []interface{}{key, period}

	v := item.GetSub(p.db, k)
	//如果不存在，则继续沿用上一个周期的计数，并更新
	if v.Empty() {
		if period <= 1 {
			return 0
		}
		count := p.getCount(key, period-1)
		return count
	}
	return v.Big().Uint64()
}

func (p PeriodInfo) updateCount(key string, period, count uint64) {
	item := SubItems([]interface{}{[]byte("periodInfo")})
	k := []interface{}{key, period}
	item.SaveSub(p.db, k, count)
}
