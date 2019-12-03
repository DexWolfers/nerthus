package ntsdb

import (
	"fmt"

	"github.com/dgraph-io/badger/options"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/log"

	"github.com/dgraph-io/badger"
)

type BadgerDB struct {
	fn string     // filename for reporting
	db *badger.DB // LevelDB instance

	log log.Logger // Contextual logger tracking the Database path
}

// NewBadger returns a badger wrapped object.
func NewBadger(file string) (*BadgerDB, error) {
	logger := log.New("database", file)

	opts := badger.DefaultOptions
	opts.Dir = file
	opts.ValueDir = file
	opts.SyncWrites = false
	opts.TableLoadingMode = options.MemoryMap
	opts.NumMemtables = 12
	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	logger.Info("New badger database")
	return &BadgerDB{
		fn:  file,
		db:  db,
		log: logger,
	}, nil
}

// Path returns the path to the Database directory.
func (db *BadgerDB) Path() string {
	return db.fn
}

// Put puts the given key / value to the queue
func (db *BadgerDB) Put(key []byte, value []byte) error {
	return db.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, value)
	})
}

// Get returns the given key if it's present.
func (db *BadgerDB) Get(key []byte) ([]byte, error) {
	var dat []byte
	// Retrieve the key and increment the miss counter if not found
	err := db.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			err = nilIfNotFound(err)
			return err
		}
		item.Value(func(val []byte) error {
			dat = append([]byte{}, val...)
			return nil
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return dat, nil
}

// Delete deletes the key from the queue and Database
func (db *BadgerDB) Delete(key []byte) error {
	err := db.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
	return nilIfNotFound(err)
}

func (db *BadgerDB) Close() {
	err := db.db.Close()
	if err == nil {
		db.log.Info("Database closed")
	} else {
		db.log.Error("Failed to close Database", "err", err)
	}
}

func (db *BadgerDB) Iterator(prefix []byte, f func(k, v []byte) bool) error {
	if len(prefix) > 0 {
		return db.db.View(func(txn *badger.Txn) error {
			it := txn.NewIterator(badger.DefaultIteratorOptions)
			defer it.Close()
			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				item := it.Item()
				var dat []byte
				err := item.Value(func(val []byte) error {
					dat = append([]byte{}, val...)
					return nil
				})
				if err != nil {
					return err
				}

				if !f(item.Key(), dat) {
					break
				}
			}
			return nil
		})
	}

	return db.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			var dat []byte
			err := item.Value(func(val []byte) error {
				dat = append([]byte{}, val...)
				return nil
			})
			if err != nil {
				return err
			}

			if common.Bytes2Hex(item.Key()) == "01c00f1690f4870fc596b0eb3a390df996028f32def2a1d3685a65265a98d9e7" {
				fmt.Println("get--->", common.Bytes2Hex(dat))
			}
			if !f(item.Key(), dat) {
				break
			}
		}
		return nil
	})
}

func (db *BadgerDB) NewBatch() Batch {
	return &bgBatch{db: db, b: db.db.NewTransaction(true)}
}

type bgBatch struct {
	db Database
	b  *badger.Txn
}

func (b *bgBatch) DB() Database {
	return b.db
}

func (b *bgBatch) Put(key, value []byte) error {
	// Set 需要由外部保证 key,value 不发送变化，直到 Commit 结束
	k, v := make([]byte, len(key)), make([]byte, len(value))
	copy(k, key)
	copy(v, value)
	return b.b.Set(k, v)
}
func (b *bgBatch) Delete(key []byte) error {
	return b.db.Delete(key)
}

func (b *bgBatch) Write() error {
	err := b.b.Commit()
	return err
}

func (b *bgBatch) Discard() {
	b.b.Discard()
}

func nilIfNotFound(err error) error {
	if err == badger.ErrKeyNotFound {
		return nil
	}
	return err
}
