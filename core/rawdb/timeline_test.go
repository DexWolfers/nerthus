package rawdb

import (
	"io/ioutil"
	"math/rand"
	"os"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/ntsdb"

	"github.com/stretchr/testify/require"
)

func newDB(t *testing.T) (db *ntsdb.LDBDatabase, close func()) {
	name, err := ioutil.TempDir("", "ntsttest")
	require.NoError(t, err)
	db, err = ntsdb.NewLDBDatabase(name, 16, 16)
	require.NoError(t, err)

	return db, func() {
		db.Close()
		os.Remove(name)
	}
}

func TestTimeline(t *testing.T) {
	db, close := newDB(t)
	defer close()

	type Item struct {
		Time uint64
		Hash common.Hash
	}

	//无序写入数据后，
	items := make(map[common.Hash]uint64)
	var max, min uint64
	for i := 0; i < 100; i++ {

		utime := uint64(rand.Int63n(1000))
		hash := common.SafeHash256(utime)

		WriteTimeline(db, hash, utime)
		items[hash] = utime

		//混入杂草
		db.Put(timelinePrefix[len(timelinePrefix)/2:], []byte("test"))
		db.Put(hash.Bytes(), []byte("test"))

		if utime > max {
			max = utime
		}
		if utime < min {
			min = utime
		}
	}
	t.Log("max:", max, "min:", min, "items:", len(items))
	//能够有序读取
	t.Run("order", func(t *testing.T) {
		all := UnitsByTimeLine(db, min, len(items))
		require.Len(t, all, len(items))

		var cur uint64
		for _, v := range all {
			if cur == 0 {
				cur = items[v]
				continue
			}
			require.True(t, cur < items[v], "time should be order: pre=%d,curr=%d", cur, items[v])
			cur = items[v]
		}
	})
	t.Run("last", func(t *testing.T) {
		h, utime := LastUnitHash(db)
		require.Equal(t, max, utime)
		require.Equal(t, max, items[h])
	})
	t.Run("byCount", func(t *testing.T) {
		all := UnitsByTimeLine(db, max, len(items))
		require.Len(t, all, 1)
		require.Equal(t, max, items[all[0]])
	})
	// 删除部分后，依旧可以检索所有剩余
	t.Run("DelTimeline", func(t *testing.T) {
		save := 30
		for k, v := range items {
			DelTimeline(db, v, k)
			delete(items, k)

			if len(items) < save {
				break
			}
		}
		all := UnitsByTimeLine(db, min, len(items))
		require.Len(t, all, len(items))

	})
}

func TestRangRangeUnits(t *testing.T) {
	db, close := newDB(t)
	defer close()

	type Item struct {
		Time uint64
		Hash common.Hash
	}

	//无序写入数据后，
	var items []Item

	now := time.Now()
	for i := 0; i < 15; i++ {
		now = now.Add(time.Duration(i))
		utime := uint64(now.UnixNano())
		hash := common.SafeHash256(utime)
		WriteTimeline(db, hash, utime)
		items = append(items, Item{utime, hash})
	}
	t.Run("middle", func(t *testing.T) {
		var gots []Item
		begin, end := items[5].Time, items[13].Time
		// 5,6,7,8,9,10,11,12,13
		RangRangeUnits(db, begin, end, func(utime uint64, uhash common.Hash) bool {
			gots = append(gots, Item{utime, uhash})
			require.True(t, utime <= end)
			require.True(t, utime >= begin)
			return true
		})
		//检查
		require.Len(t, gots, 13-5+1)
		for i, h := range gots {
			require.Equal(t, items[5+i], h)
		}
	})
	t.Run("differentRange", func(t *testing.T) {

		for beginIndex := 0; beginIndex < len(items); beginIndex++ {

			for endIndex := beginIndex; endIndex < len(items); endIndex++ {
				begin, end := items[beginIndex].Time, items[endIndex].Time
				var gots []Item
				RangRangeUnits(db, begin, end, func(utime uint64, uhash common.Hash) bool {
					gots = append(gots, Item{utime, uhash})
					require.True(t, utime <= end)
					require.True(t, utime >= begin)
					return true
				})
				//检查
				require.Len(t, gots, endIndex-beginIndex+1)
				for i, h := range gots {
					require.Equal(t, items[beginIndex+i], h)
				}
			}
		}

	})

}

func TestSameTimeUnitHash(t *testing.T) {
	db, close := newDB(t)
	defer close()

	type Item struct {
		Time uint64
		Hash common.Hash
	}

	//无序写入数据后，
	items := []Item{
		Item{1000, common.SafeHash256(uint64(1000))},
		Item{1000, common.SafeHash256(uint64(10002))},
		Item{1001, common.SafeHash256(uint64(1001))},
		Item{1002, common.SafeHash256(uint64(1002))},
		Item{1002, common.SafeHash256(uint64(10022))},
		Item{1003, common.SafeHash256(uint64(1003))},
	}
	for _, v := range items {
		WriteTimeline(db, v.Hash, v.Time)
	}

	t.Run("get all", func(t *testing.T) {
		var gots []Item
		RangRangeUnits(db, 1000, 1005, func(utime uint64, uhash common.Hash) bool {
			gots = append(gots, Item{utime, uhash})
			return true
		})
		//检查
		require.Len(t, gots, len(items))
		for _, item := range gots {
			require.Contains(t, items, item)
		}
	})

	t.Run("part", func(t *testing.T) {
		var gots []Item
		RangRangeUnits(db, 1000, 1000, func(utime uint64, uhash common.Hash) bool {
			gots = append(gots, Item{utime, uhash})
			return true
		})
		//检查
		require.Len(t, gots, 2)
		for _, item := range gots {
			require.Contains(t, items[:2], item)
		}
	})

}
