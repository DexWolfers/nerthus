package queue

import (
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"github.com/stretchr/testify/require"
)

type testValue struct {
	mc   common.Address
	hash common.Hash
	tm   time.Time
}

func sortTestValueCmpFn(x LazyQItem, y LazyQItem) bool {
	x1 := x.(testValue)
	y1 := y.(testValue)
	return x1.tm.Before(y1.tm)
}

func sortTestKeyCmpFn(x LazyQItem, y LazyQItem) bool {
	x1 := x.(time.Time)
	y1 := y.(time.Time)
	return x1.Before(y1)
}

func TestLazySyncQueue(t *testing.T) {
	t.Run("sort by key", func(t *testing.T) {
		q := NewLazySyncQueue()
		timeNow := time.Now()
		var indexRuneTests = []struct {
			key   time.Time
			value string
		}{
			{timeNow.Add(time.Second * 5), "a"},
			{timeNow.Add(time.Second * 3), "b"},
			{timeNow.Add(time.Second * 4), "b"},
			{timeNow.Add(time.Second * 1), "b"},
			{timeNow.Add(time.Second * 2), "b"},
		}

		var expect = []struct {
			key   time.Time
			value string
		}{
			{timeNow.Add(time.Second * 1), "b"},
			{timeNow.Add(time.Second * 2), "b"},
			{timeNow.Add(time.Second * 3), "b"},
			{timeNow.Add(time.Second * 4), "b"},
			{timeNow.Add(time.Second * 5), "a"},
		}
		for _, arg := range indexRuneTests {
			q.Put(arg.key, arg.value)
			t.Log(arg.key)
		}

		require.Len(t, q.queueItems(), 5)

		/// 3个超时
		time.Sleep(time.Second * 3)

		i := 0
		q.Iter(sortTestKeyCmpFn, func(key LazyQKey, value LazyQValue) bool {
			k := key.(time.Time)
			item := value.(string)
			if k.Before(time.Now()) {
				require.Equal(t, expect[i].key, key)
				require.Equal(t, expect[i].value, item)
				i++
				return true
			}
			return false
		})
		require.Equal(t, 3, i)
		require.Len(t, q.queueItems(), 2)

		// 全部超时
		time.Sleep(time.Second * 2)
		q.Iter(sortTestKeyCmpFn, func(key LazyQKey, value LazyQValue) bool {
			k := key.(time.Time)
			item := value.(string)
			if k.Before(time.Now()) {
				require.Equal(t, expect[i].key, k)
				require.Equal(t, expect[i].value, item)
				i++
				return true
			}
			return false
		})

		require.Equal(t, 5, i)
		require.Empty(t, q.queueItems())
	})
	t.Run("sort by value", func(t *testing.T) {
		q := NewLazySyncQueue(false)
		timeNow := time.Now()
		var indexRuneTests = []struct {
			key   common.Address
			value testValue
		}{
			{common.StringToAddress("C"), testValue{common.StringToAddress("C"), common.StringToHash("C"), timeNow.Add(time.Second * 3)}},
			{common.StringToAddress("D"), testValue{common.StringToAddress("D"), common.StringToHash("D"), timeNow.Add(time.Second * 4)}},
			{common.StringToAddress("A"), testValue{common.StringToAddress("A"), common.StringToHash("A"), timeNow.Add(time.Second * 1)}},
			{common.StringToAddress("B"), testValue{common.StringToAddress("B"), common.StringToHash("B"), timeNow.Add(time.Second * 2)}},
			{common.StringToAddress("E"), testValue{common.StringToAddress("E"), common.StringToHash("E"), timeNow.Add(time.Second * 5)}},
		}

		var expect = []struct {
			mc    common.Address
			value testValue
		}{
			{common.StringToAddress("A"), testValue{common.StringToAddress("A"), common.StringToHash("A"), timeNow.Add(time.Second * 1)}},
			{common.StringToAddress("B"), testValue{common.StringToAddress("B"), common.StringToHash("B"), timeNow.Add(time.Second * 2)}},
			{common.StringToAddress("C"), testValue{common.StringToAddress("C"), common.StringToHash("C"), timeNow.Add(time.Second * 3)}},
			{common.StringToAddress("D"), testValue{common.StringToAddress("D"), common.StringToHash("D"), timeNow.Add(time.Second * 4)}},
			{common.StringToAddress("E"), testValue{common.StringToAddress("E"), common.StringToHash("E"), timeNow.Add(time.Second * 5)}},
		}

		for _, arg := range indexRuneTests {
			q.Put(arg.key, arg.value)
			t.Log(arg.key.Hex())
		}

		require.Len(t, q.queueItems(), 5)

		/// 3个超时
		time.Sleep(time.Second * 3)

		i := 0
		q.Iter(sortTestValueCmpFn, func(key LazyQKey, value LazyQValue) bool {
			mc := key.(common.Address)
			item := value.(testValue)
			if item.tm.Before(time.Now()) {
				require.Equal(t, expect[i].mc.Hex(), mc.Hex())
				require.Equal(t, expect[i].value.tm, item.tm)
				require.Equal(t, expect[i].value.hash, item.hash)
				i++
				return true
			}
			return false
		})
		require.Equal(t, 3, i)
		require.Len(t, q.queueItems(), 2)

		// 全部超时
		time.Sleep(time.Second * 2)
		q.Iter(sortTestValueCmpFn, func(key LazyQKey, value LazyQValue) bool {
			mc := key.(common.Address)
			item := value.(testValue)
			if item.tm.Before(time.Now()) {
				require.Equal(t, expect[i].mc.Hex(), mc.Hex())
				require.Equal(t, expect[i].value.tm, item.tm)
				require.Equal(t, expect[i].value.hash, item.hash)
				i++
				return true
			}
			return false
		})

		require.Equal(t, 5, i)
		require.Empty(t, q.queueItems())
	})
}
