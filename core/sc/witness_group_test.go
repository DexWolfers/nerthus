package sc

import (
	"fmt"
	"math/big"
	"math/rand"
	"sort"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/params"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyChainFromWitnessGroup2(t *testing.T) {
	stateDB := newTestState()

	groupCount := 3

	groups := make(map[uint64]int)
	//准备三组见证人
	for i := 0; i < params.ConfigParamsUCWitness*groupCount; i++ {
		witness := common.StringToAddress(fmt.Sprintf("w_%d", i))
		gpIndex, err := ApplyWitnessGroupOfUser(stateDB, witness)
		require.NoError(t, err)
		groups[gpIndex] = groups[gpIndex] + 1
	}
	//检查，必须由三组
	for i := 1; i <= groupCount; i++ {
		list := GetWitnessListAt(stateDB, uint64(i))
		require.Len(t, list, int(params.ConfigParamsUCWitness), "witness group index=%d", i)

	}
	//用户申请更换见证人
	chains := groupCount * 10
	chainsGp := make(map[uint64]int)
	for i := 0; i < chains; i++ {
		gpIndex, err := ApplyChainFromWitnessGroup(stateDB,
			common.StringToAddress(fmt.Sprintf("chain_%d", i)), big.NewInt(1),
			nil, 0)
		require.NoError(t, err)
		chainsGp[gpIndex] = chainsGp[gpIndex] + 1
	}
	//人数需要均匀分配到三组见证人
	for i := 1; i <= groupCount; i++ {
		require.Equal(t, chains/groupCount, chainsGp[uint64(i)], "witness group index=%d", i)
	}

}

func TestSortWitnessGroup(t *testing.T) {

	t.Run("byChainCount", func(t *testing.T) {
		gps := make([]choseGroup, 40)
		var chainCounts sort.IntSlice
		for i := 0; i < len(gps); i++ {
			gps[i].index = uint64(i + 1)
			// 随机设置链数目
			gps[i].chainCount = uint64(rand.Int())
			chainCounts = append(chainCounts, int(gps[i].chainCount))
		}
		sortWitnessGroup(gps)
		//希望的顺序是：链数越多优先级越低，排在最前面
		sort.Sort(chainCounts)
		for i := 0; i < len(gps); i++ {
			require.Equal(t, uint64(chainCounts[len(chainCounts)-i-1]), gps[i].chainCount)
		}
	})

}

func TestTooUsers(t *testing.T) {
	stateDB := newTestState()
	var addrList common.AddressList
	for i := 0; i < 22; i++ {
		addrList = append(addrList, common.StringToAddress(fmt.Sprintf("addr_%d", i)))
	}
	// 写入系统见证人
	assert.Nil(t, applyFunc(stateDB, addrList[0], false))

	//申请两组系统见证人
	for i := 0; i < params.ConfigParamsUCWitness*3; i++ {
		_, err := applyWitness(stateDB, common.StringToAddress(fmt.Sprintf("user_w_%d", i)),
			100000000, 1234, []byte{1, 2, 4}, false)
		require.NoError(t, err)
	}

	b := time.Now()
	for i := 0; i < 8000; i++ {
		b2 := time.Now()
		_, err := ApplyChainFromWitnessGroup(stateDB, addrList[1], new(big.Int).SetUint64(0), nil, 1)
		fmt.Println(i, common.PrettyDuration(time.Since(b2)))
		assert.NoError(t, err)
	}
	fmt.Println(common.PrettyDuration(time.Since(b)))
}

func TestCancelWitnessFromWitnessGroup(t *testing.T) {
	caces := []struct {
		name   string
		init   map[uint64][]string
		remove []string
		want   map[uint64][]string
	}{
		{name: "t1",
			init: map[uint64][]string{
				1: {"W1_1", "W1_2", "W1_3", "W1_4", "W1_5", "W1_6", "W1_7", "W1_8", "W1_9", "W1_10", "W1_11"},
				2: {"W2_1", "W2_2", "W2_3", "W2_4", "W2_5", "W2_6", "W2_7", "W2_8", "W2_9", "W2_10"},
			},
			remove: []string{"W1_1", "W1_11"},
			want: map[uint64][]string{
				1: {"W1_2", "W1_3", "W1_4", "W1_5", "W1_6", "W1_7", "W1_8", "W1_9", "W1_10"},
				2: {"W2_1", "W2_2", "W2_3", "W2_4", "W2_5", "W2_6", "W2_7", "W2_8", "W2_9", "W2_10"},
			},
		},
		{name: "t2",
			init: map[uint64][]string{
				1: {"W1_1", "W1_2", "W1_3", "W1_4", "W1_5", "W1_6", "W1_7", "W1_8", "W1_9", "W1_10", "W1_11"},
				2: {"W2_1", "W2_2", "W2_3", "W2_4", "W2_5", "W2_6", "W2_7", "W2_8", "W2_9", "W2_10"},
			},
			remove: []string{"W1_1", "W1_11"},
			want: map[uint64][]string{
				1: {"W1_2", "W1_3", "W1_4", "W1_5", "W1_6", "W1_7", "W1_8", "W1_9", "W1_10"},
				2: {"W2_1", "W2_2", "W2_3", "W2_4", "W2_5", "W2_6", "W2_7", "W2_8", "W2_9", "W2_10"},
			},
		},
		{name: "t3",
			init: map[uint64][]string{
				1: {"W1_1", "W1_2", "W1_3", "W1_4", "W1_5", "W1_6", "W1_7", "W1_8", "W1_9", "W1_10", "W1_11"},
				2: {"W2_1", "W2_2", "W2_3", "W2_4", "W2_5", "W2_6", "W2_7", "W2_8", "W2_9"},
			},
			remove: []string{"W1_1", "W1_11", "W1_5", "W1_6"},
			want: map[uint64][]string{
				1: {"W1_4", "W1_7", "W1_8", "W1_9", "W1_10"},
				2: {"W2_1", "W2_2", "W2_3", "W2_4", "W2_5", "W2_6", "W2_7", "W2_8", "W2_9", "W1_2", "W1_3"},
			},
		},
		{name: "t4",
			init: map[uint64][]string{
				1: {"W1_1", "W1_2", "W1_3", "W1_4", "W1_5", "W1_6", "W1_7", "W1_8", "W1_9", "W1_10", "W1_11"},
				2: {"W2_1", "W2_2", "W2_3", "W2_4", "W2_5", "W2_6", "W2_7", "W2_8", "W2_9"},
				3: {"W3_1", "W3_2", "W3_3", "W3_4", "W3_5", "W3_6", "W3_7", "W3_8", "W3_9", "W3_10"},
			},
			remove: []string{"W1_1", "W1_11", "W1_5", "W1_6"},
			want: map[uint64][]string{
				1: {"W1_7", "W1_8", "W1_9", "W1_10"},
				2: {"W2_1", "W2_2", "W2_3", "W2_4", "W2_5", "W2_6", "W2_7", "W2_8", "W2_9", "W1_2", "W1_4"},
				3: {"W3_1", "W3_2", "W3_3", "W3_4", "W3_5", "W3_6", "W3_7", "W3_8", "W3_9", "W3_10", "W1_3"},
			},
		},
		//当一组的见证人人数减少时，将从其他组调配
		{name: "t5",
			init: map[uint64][]string{
				1: {"W1_1", "W1_2", "W1_3", "W1_4", "W1_5", "W1_6", "W1_7", "W1_8", "W1_9"},
				2: {"W2_1", "W2_2"},
				3: {"W3_1"},
				4: {"W4_1", "W4_2", "W4_3", "W4_4", "W4_5", "W4_6", "W4_7"},
			},
			remove: []string{"W1_1", "W1_5", "W1_6"},
			want: map[uint64][]string{
				1: {"W1_2", "W1_3", "W1_4", "W1_7", "W1_8", "W1_9", "W2_1", "W2_2", "W3_1", "W4_1", "W4_2"},
				2: {},
				3: {},
				4: {"W4_3", "W4_4", "W4_5", "W4_6", "W4_7"},
			},
		},
	}

	getAddr := func(s string) common.Address {
		return common.StringToAddress(s)
	}
	toStrList := func(list []common.Address) []string {
		l := make([]string, len(list))
		for i, v := range list {
			l[i] = string(v.Bytes())
		}
		return l
	}
	for _, c := range caces {
		t.Run(c.name, func(t *testing.T) {
			db := newTestState()
			//设置见证人组数
			WriteWitnessGroupCount(db, uint64(len(c.init)+1))
			for g, l := range c.init {
				for _, w := range l {
					// 当前见证人列表数
					witnessSet := GetWitnessSet(db, g)
					witnessSet.Add(db, getAddr(w))

					info := &WitnessInfo{
						Status:      WitnessNormal,
						Margin:      100000,
						ApplyHeight: 8,
						Address:     getAddr(w),
						GroupIndex:  g,
						Pubkey:      []byte{1},
					}
					err := WriteWitnessInfo(db, info)
					require.NoError(t, err)
					err = UpdateAcitvieWitenssLib(db, info)
					require.NoError(t, err)

				}
			}
			//移除
			for _, w := range c.remove {
				addr := getAddr(w)
				info, err := GetWitnessInfo(db, addr)
				info.Status = WitnessLoggedOut
				require.NoError(t, err)
				onWitnessExit(db, 10, info)
			}
			//检查结果
			for g, l := range c.want {
				got := GetWitnessListAt(db, g)
				list := toStrList(got)
				require.Len(t, list, len(l))
				//包含
				for _, w := range l {
					require.Contains(t, got, getAddr(w), "should contains %s", w)
				}
			}
		})
	}

}

func TestGetWitnessGroupCount(t *testing.T) {
	db := newTestState()

	ApplyWitnessGroupOfSys(db, common.StringToAddress("sysA"))
	ApplyWitnessGroupOfSys(db, common.StringToAddress("sysB"))

	for i := 0; i < params.ConfigParamsUCWitness*3; i++ {
		ApplyWitnessGroupOfUser(db, common.StringToAddress(fmt.Sprintf("user_w_%d", i)))
	}
	grouptCount := uint64(4)

	got := GetWitnessGroupCount(db)
	require.Equal(t, grouptCount, got)

	list := GetWitnessListAt(db, 0)
	require.Len(t, list, 2)
	require.Contains(t, list, common.StringToAddress("sysA"))
	require.Contains(t, list, common.StringToAddress("sysB"))

	for i := uint64(1); i < grouptCount; i++ {
		list := GetWitnessListAt(db, i)
		require.Len(t, list, 11)
	}

}

func TestGetWitnessGroupCount2(t *testing.T) {
	ctx, _ := newTestContext(t, params.SCAccount, params.MainnetChainConfig)
	db := ctx.state

	for i := 0; i < params.ConfigParamsUCWitness*3; i++ {
		_, err := applyWitness(db, common.StringToAddress(fmt.Sprintf("user_w_%d", i)),
			100000000, 1234, []byte{1, 2, 4}, false)
		require.NoError(t, err)
	}
	grouptCount := uint64(4)

	got := GetWitnessGroupCount(db)
	require.Equal(t, grouptCount, got)

	//即使移除见证人，见证组组数也不应该变化
	contract := Contract{caller: common.StringToAddress("sysA"), gaspool: big.NewInt(1000000000), value: big.NewInt(0)}
	for gp := uint64(1); gp < grouptCount; gp++ {
		list := GetWitnessListAt(db, gp)
		require.Len(t, list, 11)
		for _, w := range list {
			err := writeBadWitness(db, ctx, &contract, w)
			require.NoError(t, err)
		}
		//此时的见证组数不应该变化
		gotGp := GetWitnessGroupCount(db)
		require.Equal(t, grouptCount, gotGp)
	}
}

func TestApplyWitnessGroupOfUser(t *testing.T) {
	ctx, _ := newTestContext(t, params.SCAccount, params.MainnetChainConfig)
	db := ctx.state

	//申请两组系统见证人
	for i := 0; i < params.ConfigParamsUCWitness*2; i++ {
		_, err := applyWitness(db, common.StringToAddress(fmt.Sprintf("user_w_%d", i)),
			100000000, 1234, []byte{1, 2, 4}, false)
		require.NoError(t, err)
	}
	grouptCount := uint64(2 + 1)
	got := GetWitnessGroupCount(db)
	require.Equal(t, grouptCount, got)
	//注销第二组见证人
	groupIndex := uint64(1)
	list := GetWitnessListAt(db, groupIndex)

	for _, w := range list {
		contract := Contract{caller: w, gaspool: big.NewInt(1000000000), value: big.NewInt(0)}

		_, err := CancelWitness(ctx, &contract)
		require.NoError(t, err)
	}
	//成功注销后，第二组人数为0
	list = GetWitnessListAt(db, groupIndex)
	require.Empty(t, list)

	t.Run("addNew", func(t *testing.T) {
		//但是见证组应保持不变
		require.Equal(t, grouptCount, GetWitnessGroupCount(db))

		//当有新见证人加入时，将填充到这个空组中
		for i := 0; i < params.ConfigParamsUCWitness; i++ {

			witness := common.StringToAddress(fmt.Sprintf("newWitness_%d", i))

			_, err := applyWitness(db, witness,
				100000000, 1235, []byte{1, 2, 4}, false)
			require.NoError(t, err)

			//此时,见证组不变
			require.Equal(t, grouptCount, GetWitnessGroupCount(db))
			//通过此组见证人人数存在此新见证人
			list = GetWitnessListAt(db, groupIndex)
			require.Len(t, list, i+1)
			require.Contains(t, list, witness)
		}

		//如果继续加入见证人，则见证人将进入新的一组

		t.Run("more", func(t *testing.T) {
			beforeCount := GetWitnessGroupCount(db)
			t.Log(len(GetWitnessListAt(db, beforeCount-1)))

			witness := common.StringToAddress("user_new_w2")
			_, err := applyWitness(db, witness,
				100000000, 1235, []byte{1, 2, 4}, false)
			require.NoError(t, err)

			info, err := GetWitnessInfo(db, witness)
			require.NoError(t, err)
			t.Log(info.GroupIndex, beforeCount)

			nowCount := GetWitnessGroupCount(db)
			require.Equal(t, beforeCount+1, nowCount)
			//通过此组见证人人数存在此新见证人
			list = GetWitnessListAt(db, nowCount-1) //在最后一组中
			require.Len(t, list, 1)
			require.Equal(t, witness, list[0])
		})
	})
}

func TestHaveGoodHeartbeatOnGroup(t *testing.T) {
	ctx, _ := newTestContext(t, params.SCAccount, params.MainnetChainConfig)
	db := ctx.state
	//申请两组见证人
	for i := 0; i < params.ConfigParamsUCWitness*2; i++ {
		_, err := applyWitness(db, common.StringToAddress(fmt.Sprintf("user_w_%d", i)),
			100000000, 1234, []byte{1, 2, 4}, false)
		require.NoError(t, err)
	}

	gps := GetWitnessGroupCount(db)

	//默认新加入，从开始申请加入时边有记录
	round := params.CalcRound(1234)
	for i := uint64(1); i < gps; i++ {
		ok := haveGoodHeartbeatOnGroup(db, i, round)
		require.True(t, ok)

		ok = haveGoodHeartbeatOnGroup(db, i, round+1)
		require.True(t, ok)

		ok = haveGoodHeartbeatOnGroup(db, i, round-1)
		require.False(t, ok)
	}

}
