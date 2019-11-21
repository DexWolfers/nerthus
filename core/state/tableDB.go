package state

import (
	"encoding/binary"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/trie"
)

const valuePrefixLength = 4 // value前缀长度
type DBTable struct {
	orgin    trie.Database
	table    []byte
	valuePre []byte // value前缀
}

func newTable(db trie.Database, table []byte) *DBTable {

	dbtb := &DBTable{
		orgin: db,
		table: table,
	}
	return dbtb
}

func (db *DBTable) SetValuePrefix(number uint64) {
	if valuePrefixLength > 0 {
		vpre := make([]byte, valuePrefixLength)
		binary.BigEndian.PutUint32(vpre, uint32(number))
		db.valuePre = vpre
	}
}
func (db *DBTable) key(key []byte) []byte {
	if len(db.table) == 0 {
		return key
	}
	return append(db.table, key...)
}

func (db *DBTable) Get(key []byte) (value []byte, err error) {
	value, err = db.orgin.Get(db.key(key))
	if err == nil && len(value) >= valuePrefixLength {
		value = value[valuePrefixLength:]
	}
	return value, err
}
func (db *DBTable) GetPrefixValue(key []byte) (number uint64, value []byte, err error) {
	value, err = db.orgin.Get(db.key(key))
	if err != nil || valuePrefixLength == 0 {
		return
	}
	preBytes := value[:valuePrefixLength]
	number = uint64(binary.BigEndian.Uint32(preBytes))
	value = value[valuePrefixLength:]
	return
}

func (db *DBTable) Put(key, value []byte) error {
	if valuePrefixLength > 0 {
		value = append(db.valuePre, value...)
	}
	err := db.orgin.Put(db.key(key), value)
	return err
}

func (db *DBTable) NewBatch() ntsdb.Batch {
	return &DBTableBatch{
		db:       db.orgin.NewBatch(),
		prefix:   db.table,
		valuePre: db.valuePre,
		db2:      db,
	}
}

func (db *DBTable) Delete(key []byte) error {
	return db.orgin.Delete(db.key(key))
}

type DBTableBatch struct {
	db       ntsdb.Batch
	prefix   []byte
	valuePre []byte
	db2      *DBTable
}

func (db *DBTableBatch) key(key []byte) []byte {
	if len(db.prefix) == 0 {
		return key
	}
	return append(db.prefix, key...)
}

func (batch *DBTableBatch) DB() ntsdb.Database {
	return batch.db.DB()
}

func (batch *DBTableBatch) Put(key []byte, value []byte) error {
	if valuePrefixLength > 0 {
		value = append(batch.valuePre, value...)
	}
	return batch.db.Put(batch.key(key), value)
}

func (batch *DBTableBatch) Delete(key []byte) error {
	return batch.db.Delete(batch.key(key))
}

func (batch *DBTableBatch) Write() error {
	return batch.db.Write()
}

func (batch *DBTableBatch) Discard() {
	batch.db.Discard()
}
