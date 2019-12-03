// Author: @ysqi

package ntsdb

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitee.com/nerthus/nerthus/common"

	"gitee.com/nerthus/nerthus/core/config"

	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/metrics"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/errors"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type LDBDatabase struct {
	fn string      // filename for reporting
	db *leveldb.DB // LevelDB instance

	quitLock sync.Mutex      // Mutex protecting the quit channel access
	quitChan chan chan error // Quit channel to stop the metrics collection before closing the Database

	log log.Logger // Contextual logger tracking the Database path
}

// NewLDBDatabase returns a LevelDB wrapped object.
func NewLDBDatabase(file string, cache int, handles int) (*LDBDatabase, error) {
	return newLDB(file, cache, handles, false)
}
func NewLDBReadOnly(file string, cache int, handles int) (*LDBDatabase, error) {
	return newLDB(file, cache, handles, true)
}
func newLDB(file string, cache int, handles int, readonly bool) (*LDBDatabase, error) {
	logger := log.New("database", file)

	// Ensure we have some minimal caching and file guarantees
	if cache < 16 {
		cache = 16
	}
	if handles < 16 {
		handles = 16
	}

	var (
		wbuffer    = cache / 4 * opt.MiB
		blockCache = cache / 2 * opt.MiB // Two of these are used internally
		tableSize  = config.MustInt("leveldb.tableSize", 2) * opt.MiB
	)

	logger.Info("Allocated cache and file handles",
		"cache", cache, "handles", handles,
		"leveledb.blockCache", common.StorageSize(blockCache),
		"leveldb.writeBuffer", common.StorageSize(wbuffer),
		"leveldb.tableSize", common.StorageSize(tableSize),
	)

	// Open the db and recover any potential corruptions
	db, err := leveldb.OpenFile(file, &opt.Options{
		OpenFilesCacheCapacity: handles,
		BlockCacheCapacity:     blockCache,
		WriteBuffer:            blockCache,
		Filter:                 filter.NewBloomFilter(10),
		CompactionTableSize:    tableSize,
		CompactionL0Trigger:    128,
		ReadOnly:               readonly,
		WriteL0PauseTrigger:    128,
	})
	if _, corrupted := err.(*errors.ErrCorrupted); corrupted {
		db, err = leveldb.RecoverFile(file, nil)
	}
	// (Re)check for errors and abort if opening of the db failed
	if err != nil {
		return nil, fmt.Errorf("open leveldb %s: %v", file, err)
	}
	return &LDBDatabase{
		fn:  file,
		db:  db,
		log: logger,
	}, nil
}

// Path returns the path to the Database directory.
func (db *LDBDatabase) Path() string {
	return db.fn
}

// Put puts the given key / value to the queue
func (db *LDBDatabase) Put(key []byte, value []byte) error {
	return db.db.Put(key, value, nil)
}

// Get returns the given key if it's present.
func (db *LDBDatabase) Get(key []byte) ([]byte, error) {
	// Retrieve the key and increment the miss counter if not found
	dat, err := db.db.Get(key, nil)
	if err != nil {
		return nil, err
	}
	return dat, nil
}

// Delete deletes the key from the queue and Database
func (db *LDBDatabase) Delete(key []byte) error {
	// Execute the actual operation
	return db.db.Delete(key, nil)
}

func (db *LDBDatabase) NewIterator(r *util.Range, readOpt *opt.ReadOptions) iterator.Iterator {
	return db.db.NewIterator(r, readOpt)
}

func (db *LDBDatabase) Iterator(prefix []byte, call func(key, value []byte) bool) error {
	ter := db.db.NewIterator(util.BytesPrefix(prefix), nil)
	defer ter.Release()
	for ter.Next() {
		if !call(ter.Key(), ter.Value()) {
			break
		}
	}
	return nil
}

func (db *LDBDatabase) Close() {
	// Stop the metrics collection to avoid internal Database races
	db.quitLock.Lock()
	defer db.quitLock.Unlock()

	if db.quitChan != nil {
		errc := make(chan error)
		db.quitChan <- errc
		if err := <-errc; err != nil {
			db.log.Error("Metrics collection failed", "err", err)
		}
	}
	err := db.db.Close()
	if err == nil {
		db.log.Info("Database closed")
	} else {
		db.log.Error("Failed to close Database", "err", err)
	}
}

func (db *LDBDatabase) LDB() *leveldb.DB {
	return db.db
}

// Meter configures the Database metrics collectors and
func (db *LDBDatabase) Meter(prefix string) {
	// Short circuit metering if the metrics system is disabled
	if !metrics.Enabled {
		return
	}
	// Create a quit channel for the periodic collector and run it
	db.quitLock.Lock()
	db.quitChan = make(chan chan error)
	db.quitLock.Unlock()

	go db.meter(3 * time.Second)
}

// meter periodically retrieves internal leveldb counters and reports them to
// the metrics subsystem.
//
// This is how a stats table look like (currently):
//   Compactions
//    Level |   Tables   |    Size(MB)   |    Time(sec)  |    Read(MB)   |   Write(MB)
//   -------+------------+---------------+---------------+---------------+---------------
//      0   |          0 |       0.00000 |       1.27969 |       0.00000 |      12.31098
//      1   |         85 |     109.27913 |      28.09293 |     213.92493 |     214.26294
//      2   |        523 |    1000.37159 |       7.26059 |      66.86342 |      66.77884
//      3   |        570 |    1113.18458 |       0.00000 |       0.00000 |       0.00000
func (db *LDBDatabase) meter(refresh time.Duration) {
	// Create the counters to store current and previous values
	counters := make([][]float64, 2)
	for i := 0; i < 2; i++ {
		counters[i] = make([]float64, 3)
	}
	// Iterate ad infinitum and collect the stats
	for i := 1; ; i++ {
		// Retrieve the Database stats
		stats, err := db.db.GetProperty("leveldb.stats")
		if err != nil {
			db.log.Error("Failed to read Database stats", "err", err)
			return
		}
		// Find the compaction table, skip the header
		lines := strings.Split(stats, "\n")
		for len(lines) > 0 && strings.TrimSpace(lines[0]) != "Compactions" {
			lines = lines[1:]
		}
		if len(lines) <= 3 {
			db.log.Error("Compaction table not found")
			return
		}
		lines = lines[3:]

		// Iterate over all the table rows, and accumulate the entries
		for j := 0; j < len(counters[i%2]); j++ {
			counters[i%2][j] = 0
		}
		for _, line := range lines {
			parts := strings.Split(line, "|")
			if len(parts) != 6 {
				break
			}
			for idx, counter := range parts[3:] {
				value, err := strconv.ParseFloat(strings.TrimSpace(counter), 64)
				if err != nil {
					db.log.Error("Compaction entry parsing failed", "err", err)
					return
				}
				counters[i%2][idx] += value
			}
		}
		// Sleep a bit, then repeat the stats collection
		select {
		case errc := <-db.quitChan:
			// Quit requesting, stop hammering the Database
			errc <- nil
			return

		case <-time.After(refresh):
			// Timeout, gather a new set of stats
		}
	}
}

// TODO: remove this stuff and expose leveldb directly

func (db *LDBDatabase) NewBatch() Batch {
	return &ldbBatch{db: db.db, b: new(leveldb.Batch), ldb: db}
}

type ldbBatch struct {
	db  *leveldb.DB
	b   *leveldb.Batch
	ldb *LDBDatabase
}

func (b *ldbBatch) Put(key, value []byte) error {
	b.b.Put(key, value)
	return nil
}
func (b *ldbBatch) Delete(key []byte) error {
	b.b.Delete(key)
	return nil
}
func (b *ldbBatch) Write() error {
	return b.db.Write(b.b, nil)
}

func (b *ldbBatch) DB() Database {
	return b.ldb
}
func (b *ldbBatch) Discard() {
	b.b.Reset()
}

type table struct {
	db     Database
	prefix []byte
}

// NewTable returns a Database object that prefixes all keys with a given
// string.
func NewTable(db Database, prefix []byte) Database {
	return &table{
		db:     db,
		prefix: prefix,
	}
}
func (dt *table) NewBatch() Batch {
	return &tableBatch{dt.db.NewBatch(), dt.prefix}
}

func (dt *table) Put(key []byte, value []byte) error {
	return dt.db.Put(append(dt.prefix, key...), value)
}

func (dt *table) Get(key []byte) ([]byte, error) {
	return dt.db.Get(append(dt.prefix, key...))
}

func (dt *table) Delete(key []byte) error {
	return dt.db.Delete(append(dt.prefix, key...))
}

func (dt *table) Close() {
	// Do nothing; don't close the underlying DB.
}

func (dt *table) Iterator(prefix []byte, call func(key, value []byte) bool) error {
	return dt.db.Iterator(append(dt.prefix, prefix...), call)
}

type tableBatch struct {
	batch  Batch
	prefix []byte
}

// NewTableBatch returns a Batch object which prefixes all keys with a given string.
func NewTableBatch(db Database, prefix []byte) Batch {
	return &tableBatch{db.NewBatch(), prefix}
}

func (tb *tableBatch) Put(key, value []byte) error {
	return tb.batch.Put(append(tb.prefix, key...), value)
}
func (tb *tableBatch) Delete(key []byte) error {
	return tb.batch.Delete(append(tb.prefix, key...))
}

func (tb *tableBatch) Write() error {
	return tb.batch.Write()
}

func (tb *tableBatch) DB() Database {
	return tb.batch.DB()
}

func (tb *tableBatch) Discard() {
	tb.batch.Discard()
}

func (tb *tableBatch) Iterator(prefix []byte, call func(key, value []byte) bool) error {
	return tb.batch.DB().Iterator(append(tb.prefix, prefix...), call)
}
