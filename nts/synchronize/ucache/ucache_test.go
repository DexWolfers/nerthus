package ucache

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"gitee.com/nerthus/nerthus/common"

	"gitee.com/nerthus/nerthus/ntsdb"
	"github.com/stretchr/testify/require"
)

func TestUnitCache_QueryChainNextFetchNumber(t *testing.T) {
	dbf, err := ioutil.TempFile("", "nerthustest")
	require.NoError(t, err)
	dbf.Close()
	db, err := ntsdb.NewRD(dbf.Name())
	require.NoError(t, err)
	defer os.Remove(dbf.Name())
	defer db.Close()

	cache, err := NewCache(nil, nil, db, nil)
	require.NoError(t, err)

	chain := common.StringToAddress("chain")
	ck := func(t *testing.T, number, want uint64) {
		number, err := cache.queryChainNextFetchNumber(chain, number)
		require.NoError(t, err)
		require.Equal(t, int(want), int(number))
	}
	// 1: 结果为空格，则说明无缓存，需要从此高度开始拉取
	// 2. 有结果，但是第一个缓存不是高度 10， 而是 [15,16,20] ，此时：继续从 10 高度拉取
	// 3. 有结果，第一个是 10，但是：
	//		3.1  [10,12, 13,14] ，需要从 11 开始拉取
	//      3.2  [10,11,12,15,16]，需要从 13 开始拉取
	//      3.3  [10,11,12,13,14,15] 需要从 16 开始拉取

	list := []uint64{15, 16, 17, 18, 20, 21, 25, 26, 27, 28, 29}
	for _, number := range list {
		err := cache.put(cacheRecord{
			Chain:  chain.String(),
			Height: number,
			Uhash:  fmt.Sprintf("unit_%d", number),
		})
		require.NoError(t, err)
	}

	ck(t, 100, 100) //case1
	ck(t, 10, 10)   //case2
	ck(t, 18, 19)   //case3
	ck(t, 16, 19)   //case3
	ck(t, 20, 22)   //case3
	ck(t, 25, 30)   //case3

}
