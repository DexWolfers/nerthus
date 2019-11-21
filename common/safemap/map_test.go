package safemap

import (
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"github.com/stretchr/testify/require"
)

type itemValue struct {
	tm       time.Time
	tryTimes int
}

func cmpSwapMid(maxTryTimes int) CompareSwapFn {
	return func(key1, key2 ItemKey, oldValue ItemValue) (ItemValue, bool) {
		nextTime := time.Now()
		old := oldValue.(itemValue)
		// 重置时钟
		if maxTryTimes == old.tryTimes {
			old.tryTimes = 1
			old.tm = nextTime
			return old, true
		}

		// 多次同步
		timeout := old.tm.Add(2 * time.Second * time.Duration(old.tryTimes))
		// 触发再次同步
		if nextTime.After(timeout) {
			old.tryTimes++
			old.tm = nextTime
			return old, true
		}
		// 未超时，丢弃
		return old, false
	}
}

func TestBucketsTimeout(t *testing.T) {
	var (
		uHash       = common.StringToHash("A")
		id          = common.StringToAddress("A")
		maxTryTimes = 3
		expect      = []bool{true, false, true, false, false, false, true}
	)
	buckets := NewBuckets(0, 0)
	for i := 0; i < len(expect); i++ {
		newValue := itemValue{
			tm:       time.Now(),
			tryTimes: 0,
		}
		time.Sleep(time.Second)
		ok := buckets.CompareSwap(id, uHash, newValue, cmpSwapMid(maxTryTimes))
		require.Equal(t, expect[i], ok)
	}
}

func TestSafeMap(t *testing.T) {
	safeMap := NewSafeMap()
	for i := 0; i < 100; i++ {
		safeMap.LoadOrStore(i, i)
	}

	for i := 0; i < 100; i++ {
		value, ok := safeMap.Get(i)
		require.True(t, ok)
		require.Equal(t, value, i)
	}
}
