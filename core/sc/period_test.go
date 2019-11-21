package sc

import (
	"testing"

	"gitee.com/nerthus/nerthus/common/math"

	"github.com/stretchr/testify/require"
)

func TestPeriodInfo_AddCount(t *testing.T) {
	//插入大量周期
	db := newTestState()

	info := PeriodInfo{db}

	info.AddCouncilCount(10, 2)
	got := info.CouncilCount(10)
	require.Equal(t, uint64(2), got) //应该等于叠加的人数

	info.AddCouncilCount(10, 5)
	got = info.CouncilCount(10)
	require.Equal(t, uint64(2+5), got) //应该等于叠加的人数

	info.AddCouncilCount(10, -4)
	got = info.CouncilCount(10)
	require.Equal(t, uint64(2+5-4), got) //应该等于叠加的人数

	//见证人
	info.AddWitnessCount(10, true, 1)
	got = info.WitnessCount(true, 10)
	require.Equal(t, uint64(1), got) //应该等于叠加的人数

	info.AddWitnessCount(10, true, 5)
	got = info.WitnessCount(true, 10)
	require.Equal(t, uint64(1+5), got) //应该等于叠加的人数

	info.AddWitnessCount(10, true, -4)
	got = info.WitnessCount(true, 10)
	require.Equal(t, uint64(1+5-4), got) //应该等于叠加的人数

	info.AddWitnessCount(10, true, -6)
	got = info.WitnessCount(true, 10)
	require.Equal(t, uint64(0), got) //应该等于叠加的人数
}

func TestPeriodInfo(t *testing.T) {
	//插入大量周期
	db := newTestState()

	info := PeriodInfo{db}

	count := uint64(50 * 10 * 10)
	for i := uint64(1); i <= count; i++ {
		before := info.WitnessCount(false, i)
		info.AddWitnessCount(i, false, int(i))
		after := info.WitnessCount(false, i)
		require.Equal(t, before+i, after)
	}
}

func TestPeriodInfo_UpdateWitnessCount(t *testing.T) {
	db := newTestState()

	info := PeriodInfo{db}

	//无数据时获取0
	for i := uint64(1); i < 100; i++ {
		got := info.WitnessCount(true, i)
		require.Zero(t, got, "should be zero")
	}

	//第五为10
	info.AddWitnessCount(5, true, 10)
	for i := uint64(1); i < 100; i++ {
		//前四个周期应该为0，后续所有周期为10
		got := info.WitnessCount(true, i)

		if i < 5 {
			require.Zero(t, got, "should be zero, at %d", i)
		} else {
			require.Equal(t, uint64(10), got, "should be 5,at %d", i)
		}
	}

	//再更新第6周期为11
	t.Log(info.WitnessCount(true, 6))

	info.AddWitnessCount(6, true, 100000000000)
	//插入干扰数据
	info.updateCouncilCount(6, 12)
	info.updateCouncilCount(1, 2)
	info.updateCouncilCount(5, 9)

	for i := uint64(1); i < 100; i++ {

		got := info.WitnessCount(true, i)

		//前四个周期应该为0
		if i < 5 {
			require.Zero(t, got, "should be zero, at %d", i)
		} else if i == 5 { //第五周期为10
			require.Equal(t, uint64(10), got, "should be 5,at %d", i)
		} else {
			n5 := info.WitnessCount(true, 5)
			require.Equal(t, n5+uint64(100000000000), got, "at %d", i)
		}
	}
}

func TestPeriodInfo_UpdateCouncilCount(t *testing.T) {
	db := newTestState()

	info := PeriodInfo{db}

	//无数据时获取0
	for i := uint64(1); i < 100; i++ {
		got := info.CouncilCount(i)
		require.Zero(t, got, "should be zero")
	}

	//第五为10
	info.updateCouncilCount(5, 10)
	for i := uint64(1); i < 100; i++ {
		//前四个周期应该为0，后续所有周期为10
		got := info.CouncilCount(i)

		if i < 5 {
			require.Zero(t, got, "should be zero, at %d", i)
		} else {
			require.Equal(t, uint64(10), got, "should be 5,at %d", i)
		}
	}

	//再更新第6周期为11
	info.updateCouncilCount(6, math.MaxUint64)
	//插入干扰数据
	info.AddWitnessCount(6, true, 12)
	info.AddWitnessCount(1, true, 2)
	info.AddWitnessCount(5, true, 9)

	for i := uint64(1); i < 100; i++ {

		got := info.CouncilCount(i)

		//前四个周期应该为0
		if i < 5 {
			require.Zero(t, got, "should be zero, at %d", i)
		} else if i == 5 { //第五周期为10
			require.Equal(t, uint64(10), got, "should be 5,at %d", i)
		} else {
			require.Equal(t, math.MaxUint64, got, "at %d", i)
		}
	}
}
