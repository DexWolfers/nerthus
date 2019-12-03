package ntsdb

import (
	"reflect"
	"sync"
	"time"
	"unsafe"

	"gitee.com/nerthus/nerthus/log"

	"github.com/allegro/bigcache"
)

type InMemoryDatabase struct {
	cache  *bigcache.BigCache
	db     Database
	logger log.Logger

	putProcess sync.WaitGroup
	putc       chan string
	quite      chan struct{}
	onStore    func(key, value []byte) //测试使用
}

func NewInMemoryDatabase(db Database) (*InMemoryDatabase, error) {
	logger := log.New("module", "bigcache")

	option := bigcache.DefaultConfig(time.Hour * 24 * 365)
	option.CleanWindow = 0
	option.Shards = 1 << 12
	option.Verbose = false
	option.HardMaxCacheSize = 5 * 1024 //5Gb

	option.OnRemoveWithReason = func(key string, entry []byte, reason bigcache.RemoveReason) {
		if reason == bigcache.Deleted {
			return
		}
		//缓存数据如果非正常删除，必须立即落地
		db.Put(stringToBytes(key), entry)
	}

	cache, err := bigcache.NewBigCache(option)
	if err != nil {
		return nil, err
	}

	cacheDB := &InMemoryDatabase{
		cache:  cache,
		db:     db,
		logger: logger,
		putc:   make(chan string, 50000000),
		quite:  make(chan struct{}),
	}
	go cacheDB.store()
	return cacheDB, nil

}

// Path returns the path to the Database directory.
func (db *InMemoryDatabase) Path() string {
	return ""
}

// Put puts the given key / value to the queue
func (db *InMemoryDatabase) Put(key []byte, value []byte) error {
	k := bytesToString(key)
	err := db.cache.Set(k, value)
	if err != nil {
		return err
	}
	db.putc <- k
	return nil
}

// Get returns the given key if it's present.
func (db *InMemoryDatabase) Get(key []byte) ([]byte, error) {
	dat, err := db.cache.Get(bytesToString(key))
	if err == nil {
		return dat, nil
	}
	if err != bigcache.ErrEntryNotFound {
		return nil, err
	}
	return db.db.Get(key)
}

// Delete deletes the key from the queue and Database
func (db *InMemoryDatabase) Delete(key []byte) error {
	err := db.cache.Delete(bytesToString(key))
	if err != nil && err != bigcache.ErrEntryNotFound {
		return err
	}
	return db.db.Delete(key)
}

func (db *InMemoryDatabase) Iterator(prefix []byte, call func(key, value []byte) bool) error {
	if iter, ok := db.db.(Finder); ok {
		return iter.Iterator(prefix, call)
	}
	return nil
}

func (db *InMemoryDatabase) Close() {
	close(db.quite)
	db.putProcess.Wait()

	db.cache.Close()
	db.cache.Reset()
	db.db.Close()
	db.logger.Info("in-memory database closed")
}

func (db *InMemoryDatabase) NewBatch() Batch {
	return &cacheBatch{db: db, values: make(map[string][]byte)}
}

func (db *InMemoryDatabase) storeItem(k string) {
	v, err := db.cache.Get(k)
	if err != nil {
		return
	}
	if err := db.db.Put(stringToBytes(k), v); err != nil {
		db.logger.Error("failed store key", "key", k, "value", v)
	}
	db.cache.Delete(k)
	if db.onStore != nil {
		db.onStore(stringToBytes(k), v)
	}
}

func (db *InMemoryDatabase) store() {
	db.putProcess.Add(1)
	defer db.putProcess.Done()
	defer func() {
		if len(db.putc) == 0 {
			return
		}
		db.logger.Warn("store data to disk before exit", "items", len(db.putc))
		//全部落地
		for {
			if len(db.putc) == 0 {
				break
			}
			select {
			case k := <-db.putc:
				db.storeItem(k)
			default:
			}
		}

		//落地前需要存储数据
		for iter := db.cache.Iterator(); iter.SetNext(); {
			v, err := iter.Value()
			if err != nil {
				continue
			}
			db.db.Put(stringToBytes(v.Key()), v.Value())
		}
		db.logger.Info("store data to disk done")
	}()

	// 这里考虑保持一致的Put顺序因此串行按顺序落地数据
	for {
		select {
		case <-db.quite:
			return
		case k := <-db.putc:
			db.storeItem(k)
		}
	}
}

type cacheBatch struct {
	values map[string][]byte
	db     *InMemoryDatabase
}

func (b *cacheBatch) Put(key, value []byte) error {
	v := make([]byte, len(value))
	copy(v, value)
	b.values[bytesToString(key)] = v
	return nil
}
func (b *cacheBatch) Delete(key []byte) error {
	return b.db.Delete(key)
}

func (b *cacheBatch) Write() error {
	for k, v := range b.values {
		err := b.db.Put(stringToBytes(k), v)
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *cacheBatch) DB() Database {
	return b.db
}
func (b *cacheBatch) Discard() {
	for k := range b.values {
		delete(b.values, k)
	}
}

func bytesToString(b []byte) string {
	bh := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	sh := reflect.StringHeader{bh.Data, bh.Len}
	return *(*string)(unsafe.Pointer(&sh))
}

func stringToBytes(s string) []byte {
	sh := (*reflect.StringHeader)(unsafe.Pointer(&s))
	bh := reflect.SliceHeader{sh.Data, sh.Len, 0}
	return *(*[]byte)(unsafe.Pointer(&bh))
}
