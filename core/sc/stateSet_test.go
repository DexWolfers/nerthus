package sc

import (
	"testing"

	"math/big"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/ntsdb"
	"github.com/stretchr/testify/require"
)

func TestStateSet_Add(t *testing.T) {
	db := newTestState()
	set := StateSet([]interface{}{common.StringToAddress("test"), []byte("like_colors")})

	require.Zero(t, set.Len(db))

	values := []common.Hash{
		common.StringToHash("red"),
		common.StringToHash("blue"),
		common.StringToHash("yellow"),
	}
	for i, v := range values {
		index := set.Add(db, v)
		require.Equal(t, uint64(i), index, "should be get right index")

		// get
		got := set.Get(db, index)
		require.Equal(t, v, got, "should be get real value after save")

		v, ok := set.Exist(db, v)
		require.True(t, ok, "should be exist")
		require.Equal(t, index, v, "should be get right index")
	}
}

func TestClearSet(t *testing.T) {
	db := newTestState()
	set := StateSet([]interface{}{common.StringToAddress("test"), []byte("like_colors")})

	require.Zero(t, set.Len(db))

	values := make([]common.Hash, 13)
	for i := 0; i < len(values); i++ {
		values[i] = common.BytesToHash([]byte{byte(i)})
		set.Add(db, values[i])
	}

	require.Equal(t, uint64(13), set.Len(db))
	ClearSet(db, set)
	require.Zero(t, set.Len(db))

	for _, v := range values {
		_, ok := set.Exist(db, v)
		require.False(t, ok)
	}

}

func TestStateSet_Remove(t *testing.T) {
	db := newTestState()
	set := StateSet([]interface{}{common.StringToAddress("test"), []byte("like_colors")})

	require.Zero(t, set.Len(db))

	values := make([]common.Hash, 13)
	for i := 0; i < len(values); i++ {
		values[i] = common.BytesToHash([]byte{byte(i)})
		index := set.Add(db, values[i])
		require.Equal(t, uint64(i), index, "should be get right index")
	}
	//remove first
	set.Remove(db, 0)
	require.Equal(t, uint64(len(values))-1, set.Len(db), "set length should be reduced by 1")
	// first replace with end
	require.Equal(t, values[len(values)-1], set.Get(db, 0), "the first should be equal the last item")

	//remove end
	set.Remove(db, set.Len(db)-1)
	require.Equal(t, uint64(len(values))-1-1, set.Len(db), "set length should be reduced by 1")

	count := set.Len(db)

	set.Range(db, func(index uint64, value common.Hash) bool {
		t.Logf("index:%d,item=%s", index, value.Hex())
		return true
	})
	t.Log("----------->>>before", count)

	for i := count; i > 0; i-- {
		cur := set.Len(db)
		removed := set.RemoveOrder(db, 0) //不断移除中间一个
		require.Equal(t, cur-1, set.Len(db), "set length should be reduced by 1")

		set.Range(db, func(index uint64, value common.Hash) bool {
			t.Logf("index:%d,item=%s", index, value.Hex())

			_, exist := set.Exist(db, value)
			require.True(t, exist, "should be exist, at index %d, vlaue=%s", index, value.Hex())
			return true
		})

		_, ok := set.Exist(db, removed)
		require.False(t, ok, "should be not exist")

		t.Log("----------->>>", i, "afterLen", set.Len(db))
	}
	require.Zero(t, set.Len(db), "set should be empty after deleted all")

	//check clear all
	for i := 0; i < len(values); i++ {
		v := set.Get(db, uint64(i))
		require.Equal(t, common.Hash{}, v, "should be empty at index %d", i)
	}
}

func TestSubItems_SaveSub(t *testing.T) {
	db, _ := ntsdb.NewMemDatabase()
	statedb, _ := state.New([]byte{1, 2, 3}, common.Hash{}, state.NewDatabase(db))

	item := SubItems([]interface{}{common.StringToAddress("testChain").Bytes()})

	attributes := map[string]common.Hash{
		"age":     common.BigToHash(big.NewInt(16)),
		"name":    common.StringToHash("nerthus"),
		"address": common.StringToHash("shenzen"),
	}
	for k, v := range attributes {
		item.SaveSub(statedb, k, v)
	}
	for k, v := range attributes {
		got := item.GetSub(statedb, k)
		require.Equal(t, v, got, "should be equal saved")

		item.DeleteSub(statedb, k)
		got = item.GetSub(statedb, k)
		require.Equal(t, common.Hash{}, got, "should be equal saved")
	}
}

func TestValue2Hash(t *testing.T) {

	cacas := []struct {
		value interface{}
		hash  common.Hash
		panic bool
	}{
		{uint64(100), common.BigToHash(big.NewInt(100)), false},
		{byte(1), common.BigToHash(big.NewInt(1)), false},
		{WitnessInBlackList, common.BigToHash(big.NewInt(int64(WitnessInBlackList))), false},
		{"abc", common.StringToHash("abc"), false},
		{big.NewInt(12345), common.BigToHash(big.NewInt(12345)), false},
		{common.StringToAddress("chain"), common.StringToAddress("chain").Hash(), false},

		{100, common.Hash{}, true},
		{CouncilStatusInvalid, uint2Hash(uint64(CouncilStatusInvalid)), false},
		{VotingResultDisagree, uint2Hash(uint64(VotingResultDisagree)), false},
	}
	for _, c := range cacas {
		if c.panic {
			require.Panics(t, func() {
				value2hash(c.value)
			})
		} else {
			got := value2hash(c.value)
			require.Equal(t, c.hash, got)
		}
	}

}
