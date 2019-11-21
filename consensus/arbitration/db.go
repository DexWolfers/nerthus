package arbitration

import "gitee.com/nerthus/nerthus/ntsdb"

type dbHelp struct {
	ntsdb.Batch
	get ntsdb.Getter
}

func (db dbHelp) Get(key []byte) ([]byte, error) {
	return db.get.Get(key)
}

func newBatch(db ntsdb.Database) dbHelp {
	batch := db.NewBatch()
	return dbHelp{
		Batch: batch,
		get:   db,
	}
}
