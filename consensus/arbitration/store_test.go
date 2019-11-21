package arbitration

import (
	"testing"

	"gitee.com/nerthus/nerthus/core/types"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/ntsdb"
	"github.com/stretchr/testify/require"
)

func TestGetAllBadChain(t *testing.T) {
	db := ntsdb.NewMemDB()

	chainA := common.StringToAddress("chainA")
	chainB := common.StringToAddress("chainB")

	storeds := []BadChainFlag{
		{Chain: chainA, Number: 10000},
		{Chain: chainA, Number: 10001},
		{Chain: chainA, Number: 10002},

		{Chain: chainB, Number: 50},
		{Chain: chainB, Number: 10},
		{Chain: chainB, Number: 20},
	}

	//存储
	for _, v := range storeds {
		UpdateBadChainFlag(db, v)
	}

	got := GetAllBadChain(db)
	want := []common.Address{chainA, chainB}
	//因为不确定集合内容顺序，则相互校验
	require.Subset(t, want, got)
	require.Subset(t, got, want)
}

func TestGetBadChainFlag(t *testing.T) {
	db := ntsdb.NewMemDB()

	chain := common.StringToAddress("chainA")
	info := GetBadChainFlag(db, chain)
	require.True(t, info.Chain.Empty())
	require.Zero(t, info.Number)

	//存储

	stored := BadChainFlag{
		Chain:   chain,
		Number:  10000,
		ProofID: common.SafeHash256("id"),
	}

	UpdateBadChainFlag(db, stored)

	// 可以获取
	t.Run("get", func(t *testing.T) {
		got := GetBadChainFlag(db, chain)
		require.Equal(t, stored, got)

	})

	t.Run("update", func(t *testing.T) {
		// 更新的位置是高高度时，失败
		t.Run("ignore", func(t *testing.T) {
			info2 := BadChainFlag{
				Chain:   chain,
				Number:  stored.Number + 1,
				ProofID: common.SafeHash256("id"),
			}
			UpdateBadChainFlag(db, info2)
		})
		// 更新的位置是低高度时，成功
		t.Run("ok", func(t *testing.T) {
			info2 := BadChainFlag{
				Chain:   chain,
				Number:  stored.Number - 1,
				ProofID: common.SafeHash256("id"),
			}
			UpdateBadChainFlag(db, info2)
		})
	})

	t.Run("clear", func(t *testing.T) {
		got := GetBadChainFlag(db, chain)
		//不能清除非对应位置
		ok := clearBadChainFlag(db, chain, got.Number+1)
		require.False(t, ok, "should be can not clear")

		// 可以成功清除
		ok = clearBadChainFlag(db, chain, got.Number)
		require.False(t, ok, "should be cleared")

		got3 := GetBadChainFlags(db, chain)

		for _, v := range got3 {
			if v.Number == got.Number {
				require.Fail(t, "should be cleared")
			}
		}
	})

}

func TestGetStoppedChainFlag(t *testing.T) {
	db := ntsdb.NewMemDB()

	chain := common.StringToAddress("chainA")

	stopped := ChainNumber{
		Chain:  chain,
		Number: 100000,
	}

	info := GetStoppedChainFlag(db, chain)
	require.Zero(t, info, "should be not found")

	ok := writeStoppedChain(db, stopped.Chain, stopped.Number)
	require.True(t, ok, "写入成功")
	// 在存储后可以获取
	t.Run("get", func(t *testing.T) {
		info := GetStoppedChainFlag(db, chain)
		require.Equal(t, stopped.Number, info, "should be not found")
	})
	t.Run("update", func(t *testing.T) {
		ok := writeStoppedChain(db, stopped.Chain, stopped.Number+1)
		require.True(t, ok)

		ok = writeStoppedChain(db, stopped.Chain, stopped.Number)
		require.False(t, ok)

		//可以成功更新
		ok = writeStoppedChain(db, stopped.Chain, stopped.Number-1)
		require.True(t, ok)
		//更新后可正确获取位置，属于最低
		number := GetStoppedChainFlag(db, stopped.Chain)
		require.Equal(t, stopped.Number-1, number)

		t.Run("Delete", func(t *testing.T) {
			//可以获取
			number := GetStoppedChainFlag(db, chain)
			require.NotZero(t, number)

			//存在三个标记，依次删除
			clearStoppedChainFlag(db, chain, stopped.Number+1)
			clearStoppedChainFlag(db, chain, stopped.Number)
			clearStoppedChainFlag(db, chain, stopped.Number-1)

			info2 := GetStoppedChainFlag(db, chain)
			require.Zero(t, info2)
		})
	})

	t.Run("mul", func(t *testing.T) {
		chains := make([]ChainNumber, 10)
		for i := 0; i < len(chains); i++ {
			chains[i] = ChainNumber{Chain: common.BytesToAddress([]byte{byte(i)}), Number: uint64(i)}
		}
		for _, v := range chains {
			info := GetStoppedChainFlag(db, v.Chain)
			require.Zero(t, info, "should be not found")

			ok := writeStoppedChain(db, v.Chain, v.Number)
			require.True(t, ok)
			//更新后可正确获取
			number := GetStoppedChainFlag(db, v.Chain)
			require.Equal(t, v.Number, number)
		}

	})

}

func TestCheckChainIsStopped(t *testing.T) {
	db := ntsdb.NewMemDB()

	chain := common.StringToAddress("chainA")

	stopped := ChainNumber{
		Chain:  chain,
		Number: 100000,
	}
	require.NoError(t, CheckChainIsStopped(db, chain, stopped.Number-1), "尚且不属于停止")
	require.NoError(t, CheckChainIsStopped(db, chain, stopped.Number), "尚且不属于停止")
	require.NoError(t, CheckChainIsStopped(db, chain, stopped.Number+1), "尚且不属于停止")

	writeStoppedChain(db, stopped.Chain, stopped.Number)

	require.NoError(t, CheckChainIsStopped(db, chain, stopped.Number-1), "低高度不属于停止")
	require.Error(t, CheckChainIsStopped(db, chain, stopped.Number), "此高度属于停止")
	require.Error(t, CheckChainIsStopped(db, chain, stopped.Number+1), "高高度属于停止")
}

func TestGetStoppedChainByBad(t *testing.T) {
	db := ntsdb.NewMemDB()

	chain := common.StringToAddress("chainA")
	stopped := ChainNumber{
		Chain:  chain,
		Number: 100000,
	}

	//无数据
	items := GetStoppedChainByBad(db, stopped.Chain, stopped.Number)
	require.Empty(t, items)

	others := map[common.Address]uint64{
		common.StringToAddress("a"): 1,
		common.StringToAddress("b"): 2,
		common.StringToAddress("c"): 3,
		common.StringToAddress("d"): 4,
		common.StringToAddress("e"): 5,
		common.StringToAddress("f"): 6,
		common.StringToAddress("g"): 7,
	}

	writeStoppedByWho(db, stopped.Chain, stopped.Number, others)

	items = GetStoppedChainByBad(db, stopped.Chain, stopped.Number)
	require.Equal(t, len(others), len(items))
	for _, v := range items {
		find, ok := others[v.Chain]
		require.True(t, ok)
		require.Equal(t, find, v.Number)
	}

}

func TestWriteBadWitneeProof(t *testing.T) {
	var r []types.AffectedUnit
	r = append(r, types.AffectedUnit{
		Header: types.Header{MC: common.StringToAddress("testChain"), Number: 100},
		Votes: []types.WitenssVote{
			{Extra: make([]byte, 3), Sign: *types.NewSignContent().Set(make([]byte, 50))},
		},
	})
	r = append(r, types.AffectedUnit{
		Header: types.Header{MC: common.StringToAddress("testChain2"), Number: 2000},
		Votes: []types.WitenssVote{
			{Extra: make([]byte, 2), Sign: *types.NewSignContent().Set(make([]byte, 25))},
		},
	})

	db := ntsdb.NewMemDB()
	id := common.SafeHash256(r)
	exist := ExistProof(db, id)
	require.False(t, exist)

	id = writeBadWitneeProof(db, r)

	exist = ExistProof(db, id)
	require.True(t, exist, "should be exist after write to db")

}

// 测试对 Proof 的排序规则，已使得能优先按时间升序排列，里面每项按签名升序排列
func TestSortProof(t *testing.T) {

	one := []types.AffectedUnit{
		{
			Header: types.Header{MC: common.StringToAddress("testChain"), Number: 100, Timestamp: 2},
			Votes: []types.WitenssVote{
				{Extra: make([]byte, 3), Sign: *types.NewSignContent().Set([]byte{1, 2, 3})},
				{Extra: make([]byte, 3), Sign: *types.NewSignContent().Set([]byte{4, 5, 6})},
			},
		},
		{
			Header: types.Header{MC: common.StringToAddress("testChain"), Number: 100, Timestamp: 1},
			Votes: []types.WitenssVote{
				{Extra: make([]byte, 2), Sign: *types.NewSignContent().Set([]byte{6, 7, 8})},
				{Extra: make([]byte, 2), Sign: *types.NewSignContent().Set([]byte{6, 7, 9})},
			},
		},
		{
			Header: types.Header{MC: common.StringToAddress("testChain"), Number: 100, Timestamp: 3},
			Votes: []types.WitenssVote{
				{Extra: make([]byte, 2), Sign: *types.NewSignContent().Set([]byte{6, 5, 4})},
				{Extra: make([]byte, 2), Sign: *types.NewSignContent().Set([]byte{4, 5, 6})},
			},
		},
	}

	SortProof(one)

	require.Equal(t, uint64(1), one[0].Header.Timestamp)
	require.Equal(t, uint64(2), one[1].Header.Timestamp)
	require.Equal(t, uint64(3), one[2].Header.Timestamp)

	require.Equal(t, []byte{4, 5, 6}, one[2].Votes[0].Sign.Get(), "should be sort by Sign")

}
