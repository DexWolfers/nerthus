package sc

import (
	"math/rand"
	"strconv"
	"testing"

	"gitee.com/nerthus/nerthus/params"

	"gitee.com/nerthus/nerthus/common"
	"github.com/stretchr/testify/require"
)

func TestRemoveCouncil2(t *testing.T) {
	ctx, _ := newTestContext(t, params.SCAccount, params.TestChainConfig)
	db := ctx.state

	//所有理事
	list, err := GetCouncilList(db)
	require.NoError(t, err)
	require.Len(t, list, 10, "should be have 10 councils") //newTestContext创建了十个理事

	delOne := func(t *testing.T, c common.Address, shouldSucc bool) bool {
		beforec := GetCouncilCount(db)

		//分别删除
		err := RemoveCouncil(db, c, 100)
		if !shouldSucc {
			require.Error(t, err)
			return false
		}
		require.NoError(t, err)

		afterc := GetCouncilCount(db)
		require.Equal(t, beforec-1, afterc)
		//检查状态
		info, err := GetCouncilInfo(db, c)
		require.NoError(t, err)
		require.Equal(t, c, info.Address)
		require.Equal(t, CouncilStatusInvalid, info.Status)

		list, err := GetCouncilList(db)
		require.NoError(t, err)
		require.Len(t, list, int(afterc))

		return true
	}

	var cannotDel common.Address

	//随机删除
	for i := 0; i < len(list); i++ {
		index := rand.Intn(len(list))
		c := list[index]
		if !delOne(t, c, len(list) != 1) {
			cannotDel = c
		}
		list = append(list[:index], list[index+1:]...)

	}
	for i, c := range list {
		if !delOne(t, c, len(list)-1 != i) {
			cannotDel = c
		}
	}

	t.Run("addNew", func(t *testing.T) {
		a := common.StringToAddress("c_a")
		info := Council{
			Address:     a,
			Status:      CouncilStatusValid,
			Margin:      10000,
			ApplyHeight: 0}

		err := AddCouncil(db, &info, 10)
		require.NoError(t, err)

		got, err := GetCouncilInfo(db, a)
		require.NoError(t, err)
		require.Equal(t, info, *got)

		t.Run("delOld", func(t *testing.T) {
			delOne(t, cannotDel, true)
			delOne(t, a, false)
		})
	})
	t.Run("check", func(t *testing.T) {
		before := GetCouncilCount(db)

		for i := 0; i < 100; i++ {
			info := Council{
				Address: common.StringToAddress("c_new_" + strconv.Itoa(i)),
				Status:  CouncilStatusValid,
				Margin:  2000,
			}
			err = AddCouncil(db, &info, 0)

			require.Equal(t, before+uint64(i+1), GetCouncilCount(db))
		}
		t.Run("range", func(t *testing.T) {
			count := GetCouncilCount(db)
			list, _ := GetCouncilList(db)
			require.Len(t, list, int(count))
		})
	})
}
