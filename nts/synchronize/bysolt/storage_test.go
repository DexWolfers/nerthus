package bysolt

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/ntsdb"
)

func TestStorage_Start(t *testing.T) {
	db, close := newDB(t)
	defer close()

	first, err := time.Parse("2006-01-02 15:04:05", "2019-02-01 01:00:05")
	require.NoError(t, err)

	store := NewStorage(db, uint64(first.UnixNano()), nil)

	t.Run("start", func(t *testing.T) {
		//启动后将需要开始拉取数据，且从第一个时区开始
		var wg sync.WaitGroup
		wg.Add(1)
		store.getUnits = func(db ntsdb.Database, start, end uint64, cb func(utime uint64, uhash common.Hash) bool) {
			size := store.PointTimeRange(1)
			require.Equal(t, size.Begin.UnixNano(), int64(start))
			require.Equal(t, size.End.UnixNano(), int64(end))
			cb(start, common.StringToHash("unit1"))
			wg.Done()
		}
		store.Start()
		wg.Wait()

		t.Run("stop", func(t *testing.T) {
			require.False(t, store.stopped())
			store.Stop()
			require.True(t, store.stopped())
		})
	})

}

func TestStorage_NeedUpdate(t *testing.T) {
	db, close := newDB(t)
	defer close()

	first, err := time.Parse("2006-01-02 15:04:05", "2019-02-01 01:00:05")
	require.NoError(t, err)

	store := NewStorage(db, uint64(first.UnixNano()), nil)

	require.Panics(t, func() {
		store.addTODO(0)
	})

	for i := 1; i <= 10; i++ {
		store.addTODO(uint64(i))
		store.addTODO(uint64(i))
	}
	//应该有十个
	var count int
	store.todoCache.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	require.Equal(t, count, 10)

	//且在数据库中存在
	count = 0
	rangeTodos(db, func(number uint64) bool {
		count++
		return true
	})
	require.Equal(t, count, 10)

	//完成一个
	t.Run("done", func(t *testing.T) {
		store.todoDone(5)
		count = 0
		rangeTodos(db, func(number uint64) bool {
			require.NotEqual(t, uint64(5), number)
			count++
			return true
		})
		require.Equal(t, count, 9)
	})
}

func TestStorage_update(t *testing.T) {
	db, close := newDB(t)
	defer close()
	first, err := time.Parse("2006-01-02 15:04:05", "2019-02-01 01:00:05")
	require.NoError(t, err)

	store := NewStorage(db, uint64(first.UnixNano()), nil)

	//检查时，应该将处理第一个周期内的单元
	store.getUnits = func(db ntsdb.Database, start, end uint64, cb func(utime uint64, uhash common.Hash) bool) {
		size := store.PointTimeRange(1)
		require.Equal(t, size.Begin.UnixNano(), int64(start))
		require.Equal(t, size.End.UnixNano(), int64(end))

		//只提供一个
		cb(start, common.StringToHash("unit1"))

	}

	point := store.LastPoint()
	require.Nil(t, point)

	store.checkUpdate()

	point2 := store.LastPoint()
	require.False(t, point2.Empty())
	require.Equal(t, uint64(1), point2.Number)
}
