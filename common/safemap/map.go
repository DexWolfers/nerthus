package safemap

import (
	"sync"
)

type ItemKey = interface{}
type ItemValue = interface{}
type CompareSwapFn = func(key1, key2 ItemKey, oldValue ItemValue) (ItemValue, bool)

const (
	MaxBucketsSize = 1024
	MaxBucketSize  = 1024
)

func NewSafeMap() SafeMap {
	return &innerSafeMap{RWMutex: new(sync.RWMutex), inner: make(map[interface{}]interface{})}
}

type SafeMap interface {
	Store(key interface{}, value interface{})
	// 如果第一次存储key值时，old == value
	LoadOrStore(key interface{}, value interface{}) (old interface{}, ok bool)
	Get(key interface{}) (value interface{}, ok bool)
	Keys() []interface{}
	Values() []interface{}
	Len() (length int)
	Clear()
	Delete(key interface{})
	Iter(func(key interface{}, value interface{}) bool)
}

type innerSafeMap struct {
	*sync.RWMutex
	inner map[interface{}]interface{}
}

func (safeMap *innerSafeMap) Store(key interface{}, value interface{}) {
	safeMap.Lock()
	safeMap.inner[key] = value
	safeMap.Unlock()
}

func (safeMap *innerSafeMap) Keys() []interface{} {
	safeMap.RLock()
	var keys = make([]interface{}, 0, len(safeMap.inner))
	for key := range safeMap.inner {
		keys = append(keys, key)
	}
	safeMap.RUnlock()
	return keys
}

func (safeMap *innerSafeMap) Values() []interface{} {
	safeMap.RLock()
	var values = make([]interface{}, 0, len(safeMap.inner))
	for _, value := range safeMap.inner {
		values = append(values, value)
	}
	safeMap.RUnlock()
	return values
}

func (safeMap *innerSafeMap) LoadOrStore(key interface{}, value interface{}) (old interface{}, ok bool) {
	safeMap.Lock()
	old, ok = safeMap.inner[key]
	if !ok {
		safeMap.inner[key] = value
		old = value
	}
	safeMap.Unlock()
	return
}

func (safeMap *innerSafeMap) Len() (length int) {
	safeMap.RLock()
	length = len(safeMap.inner)
	safeMap.RUnlock()
	return
}

func (safeMap *innerSafeMap) Get(key interface{}) (value interface{}, ok bool) {
	safeMap.RLock()
	value, ok = safeMap.inner[key]
	safeMap.RUnlock()
	return
}

func (safeMap *innerSafeMap) Delete(key interface{}) {
	safeMap.Lock()
	delete(safeMap.inner, key)
	safeMap.Unlock()
}

func (safeMap *innerSafeMap) Clear() {
	safeMap.Lock()
	safeMap.inner = make(map[interface{}]interface{})
	safeMap.Unlock()
}

func (safeMap *innerSafeMap) Iter(fn func(key, value interface{}) bool) {
	safeMap.Lock()
	for k, v := range safeMap.inner {
		if ok := fn(k, v); !ok {
			break
		}
	}
	safeMap.Unlock()
}

/// TODO 优化锁的颗粒度
/// instance = map[key1]map[key2]value
/// Slice ==> [(instance1,instance2), (instance3, instance4), (instance5)]
type SafeDoubleKeyMap interface {
	Push(key1, key2 ItemKey, value ItemValue)
	CompareSwap(key1, key2 ItemKey, defaultValue ItemValue, fn CompareSwapFn) bool
	Get(key1, key2 ItemKey) (ItemValue, bool)
	Delete(key1, key2 ItemKey) bool
}

func NewBuckets(bucketsSize, bucketSize int) *Buckets {
	if bucketsSize == 0 {
		bucketsSize = MaxBucketsSize
	}
	if bucketSize == 0 {
		bucketSize = MaxBucketSize
	}
	return &Buckets{
		bucketsSize: bucketsSize,
		bucketSize:  bucketSize,
		l:           new(sync.RWMutex),
		buckets:     map[ItemKey]*Bucket{},
	}
}

type Buckets struct {
	bucketsSize int
	bucketSize  int
	l           *sync.RWMutex
	buckets     map[ItemKey]*Bucket
}

func (buckets *Buckets) Push(key1, key2 ItemKey, value ItemValue) {
	buckets.l.Lock()
	bucket, ok := buckets.buckets[key1]
	if !ok {
		bucket = newBucket(buckets.bucketSize)
		buckets.buckets[key1] = bucket
	}
	bucket.push(key2, value)
	buckets.l.Unlock()
}

// 如果不存在value, 则old == defaultValue
func (buckets *Buckets) CompareSwap(key1, key2 ItemKey, defaultValue ItemValue, fn CompareSwapFn) bool {
	var (
		bucket   *Bucket
		oldValue = defaultValue
		newValue ItemValue
		ok       bool
	)

	buckets.l.Lock()
	bucket, ok = buckets.buckets[key1]
	if !ok {
		bucket = newBucket(buckets.bucketSize)
		buckets.buckets[key1] = bucket
		//
	} else {
		oldValue, ok = bucket.get(key2)
		if !ok {
			oldValue = defaultValue
		}
	}

	// TODO Opt
	newValue, ok = fn(key1, key2, oldValue)
	if ok {
		bucket.push(key2, newValue)
	}
	buckets.l.Unlock()
	return ok
}

func (buckets *Buckets) Get(key1, key2 ItemKey) (item ItemValue, ok bool) {
	var (
		bucket *Bucket
	)
	buckets.l.RLock()
	bucket, ok = buckets.buckets[key1]
	if !ok {
		return
	}
	item, ok = bucket.get(key2)
	buckets.l.RUnlock()
	return
}

func (buckets *Buckets) Delete(key1, key2 ItemKey) bool {
	buckets.l.Lock()
	bucket, ok := buckets.buckets[key1]
	if !ok {
		buckets.l.Unlock()
		return ok
	}
	ok = bucket.delete(key2)
	return ok
}

// TODO Opt
type Bucket struct {
	size  int
	items map[ItemKey]ItemValue
	l     *sync.RWMutex
}

func newBucket(size int) *Bucket {
	return &Bucket{
		size:  size,
		items: make(map[ItemKey]ItemValue),
		l:     new(sync.RWMutex),
	}
}

func (bucket *Bucket) push(key ItemKey, value ItemValue) {
	bucket.l.Lock()
	bucket.items[key] = value
	bucket.l.Unlock()
}

func (bucket *Bucket) get(key ItemKey) (value ItemValue, ok bool) {
	bucket.l.RLock()
	value, ok = bucket.items[key]
	bucket.l.RUnlock()
	return
}

func (bucket *Bucket) delete(key ItemKey) bool {
	bucket.l.Lock()
	_, ok := bucket.items[key]
	if !ok {
		bucket.l.Unlock()
		return ok
	}
	delete(bucket.items, key)
	bucket.l.Unlock()
	return ok
}
