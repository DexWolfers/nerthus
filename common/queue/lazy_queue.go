package queue

import (
	"sync"

	"gitee.com/nerthus/nerthus/common/sort"
)

type LazyQKey = interface{}
type LazyQValue = interface{}
type LazyQItem = interface{}
type LazyQCompareFn = func(LazyQItem, LazyQItem) bool
type LazyIterFn = func(key LazyQKey, value LazyQValue) bool
type LazyParseKey = func(queueItem LazyQValue) (LazyQKey, bool)

var (
	lazyQCompareFnMiddle = func(sortByKey bool, fn LazyQCompareFn) LazyQCompareFn {
		return func(keyOrValue1 LazyQItem, keyOrValue2 LazyQItem) bool {
			if sortByKey {
				return fn(keyOrValue1, keyOrValue2)
			} else {
				value1, value2 := keyOrValue1.(innerQueueItem).LazyQValue, keyOrValue2.(innerQueueItem).LazyQValue
				return fn(value1, value2)
			}
		}
	}
)

type innerQueueItem struct {
	innerKey LazyQKey
	LazyQValue
}

/// default sortBy Key
/// 延时处理队列，队列排序支持按照key，或者value排序，只要实现比较函数LazyQCompareFn即可
/// 如果是按照key排序，则LazyQCompareFn的参数类型为LazyQKey，反之为LazyQValue
type LazySyncQueue struct {
	*sync.RWMutex
	quit       chan struct{}
	sortByKey  bool
	keyQueue   []LazyQKey
	valueQueue []LazyQValue
	hashMap    map[LazyQKey]LazyQValue
}

func NewLazySyncQueue(sortByKey ...bool) *LazySyncQueue {
	if len(sortByKey) == 0 {
		sortByKey = []bool{true}
	}
	return &LazySyncQueue{
		RWMutex:   new(sync.RWMutex),
		quit:      make(chan struct{}),
		hashMap:   map[LazyQKey]LazyQValue{},
		sortByKey: sortByKey[0],
	}
}

/// TODO 暂时没有必要更新入队时间
func (lazyQ *LazySyncQueue) Put(key LazyQValue, value LazyQValue, flag ...bool) {
	if len(flag) == 0 {
		flag = []bool{false}
	}
	lazyQ.Lock()
	_, ok := lazyQ.hashMap[key]
	if !ok {
		lazyQ.hashMap[key] = value
		if lazyQ.sortByKey {
			lazyQ.keyQueue = append(lazyQ.keyQueue, key)
		} else {
			inner := innerQueueItem{innerKey: key, LazyQValue: value}
			lazyQ.valueQueue = append(lazyQ.valueQueue, inner)
		}
		lazyQ.Unlock()
		return
	}

	lazyQ.Unlock()
}

func (lazyQ *LazySyncQueue) Get(key LazyQKey) (LazyQValue, bool) {
	lazyQ.RLock()
	value, ok := lazyQ.hashMap[key]
	lazyQ.RUnlock()
	return value, ok
}

/// Just for test, get the queue item
func (lazyQ *LazySyncQueue) queueItems() []interface{} {
	lazyQ.RLock()
	var iterms []interface{}
	if lazyQ.sortByKey {
		iterms = lazyQ.keyQueue
	} else {
		for _, value := range lazyQ.valueQueue {
			iterms = append(iterms, value)
		}
	}
	lazyQ.RUnlock()
	return iterms
}

/// 迭代器：为了性能考虑，在迭代处理时，会清理掉过期的单元
/// cmpFn: 用于队列排序
/// iterFn: 队列过滤器，如果返回true，则表示继续迭代，false则中断迭代
func (lazyQ *LazySyncQueue) Iter(cmpFn LazyQCompareFn, iterFn LazyIterFn) {
	lazyQ.RLock()
	var (
		sortSlice sort.SortSlice
	)
	if lazyQ.sortByKey && len(lazyQ.keyQueue) == 0 {
		lazyQ.RUnlock()
		return
	}
	if !lazyQ.sortByKey && len(lazyQ.valueQueue) == 0 {
		lazyQ.RUnlock()
		return
	}
	if lazyQ.sortByKey {
		sortSlice.Items = lazyQ.keyQueue
	} else {
		sortSlice.Items = lazyQ.valueQueue
	}
	sort.QuicklySort(sortSlice, lazyQCompareFnMiddle(lazyQ.sortByKey, cmpFn))

	var delIdx = -1
	for i, queueItem := range sortSlice.Items {
		var (
			value LazyQValue
			key   LazyQKey
		)

		if lazyQ.sortByKey {
			key = queueItem
		} else {
			key = queueItem.(innerQueueItem).innerKey
		}
		value = lazyQ.hashMap[key]

		// 满足条件，则继续迭代，直到返回false为止，或者队列为空
		if !iterFn(key, value) {
			break
		}
		delIdx = i
		delete(lazyQ.hashMap, key)
	}
	if delIdx == -1 {
		lazyQ.RUnlock()
		return
	}

	// reset the slice
	if delIdx == len(sortSlice.Items)-1 {
		lazyQ.hashMap = make(map[LazyQKey]LazyQValue)
		sortSlice.Items = sortSlice.Items[:0]
	} else {
		sortSlice.Items = sortSlice.Items[delIdx+1:]
	}
	if lazyQ.sortByKey {
		lazyQ.keyQueue = sortSlice.Items
	} else {
		lazyQ.valueQueue = sortSlice.Items
	}
	lazyQ.RUnlock()
}
