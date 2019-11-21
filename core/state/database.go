package state

import (
	"fmt"

	"gitee.com/nerthus/nerthus/common/sync/cmap"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/trie"

	"github.com/hashicorp/golang-lru"
)

// Trie cache generation limit after which to evic trie nodes from memory.
var MaxTrieCacheGen = uint16(120)

const (
	// Number of past tries to keep. This value is chosen such that
	// reasonable chain reorg depths will hit an existing trie.
	maxPastTries = 2000

	// Number of codehash->size associations to keep.
	codeSizeCacheSize = 100000
)

// Database wraps access to tries and contract code.
type Database interface {
	// Accessing tries:
	// OpenTrie opens the main account trie.
	// OpenStorageTrie opens the storage trie of an account.
	OpenTrie(prefix []byte, root common.Hash) (Trie, error)
	OpenStorageTrie(prefix []byte, addrHash, root common.Hash) (Trie, error)
	// Accessing contract code:
	ContractCode(prefix []byte, addrHash, codeHash common.Hash) ([]byte, error)
	ContractCodeSize(prefix []byte, addrHash, codeHash common.Hash) (int, error)
	// CopyTrie returns an independent copy of the given trie.
	CopyTrie(Trie) Trie
	Database() ntsdb.Database
}

// Trie is a Ethereum Merkle Trie.
type Trie interface {
	TryGet(key []byte) ([]byte, error)
	TryUpdate(key, value []byte) error
	TryDelete(key []byte) error
	CommitTo(db trie.Database, number uint64) (common.Hash, error)
	Hash() common.Hash
	NodeIterator(startKey []byte) trie.NodeIterator
	GetKey([]byte) []byte // TODO(fjl): remove this when SecureTrie is removed
	VerifyProof(key []byte) (value []byte, err error)
	Clear()
	Trie() *trie.Trie

	DB() *DBTable
}

// NewDatabase creates a backing store for state. The returned database is safe for
// concurrent use and retains cached trie nodes in memory.
func NewDatabase(db ntsdb.Database) Database {
	csc, _ := lru.New(codeSizeCacheSize)
	return &cachingDB{db: db, codeSizeCache: csc, pastTries: cmap.NewWith(64)}
}

type cachingDB struct {
	db            ntsdb.Database
	pastTries     cmap.ConcurrentMap //[]*trie.SecureTrie
	codeSizeCache *lru.Cache
}

func (db *cachingDB) OpenTrie(prefix []byte, root common.Hash) (Trie, error) {
	dbtable := newTable(db.Database(), prefix)
	if t, ok := db.pastTries.Get(root.Hex()); ok {
		return cachedTrie{t.(*trie.SecureTrie).Copy(), db, dbtable}, nil
	}

	tr, err := trie.NewSecure(root, dbtable, MaxTrieCacheGen)
	if err != nil {
		return nil, err
	}
	return cachedTrie{tr, db, dbtable}, nil
}

func (db *cachingDB) pushTrie(t *trie.SecureTrie) {
	key := t.Hash().Hex()

	db.pastTries.Set(t.Hash().Hex(), t)

	//随机删除
	if db.pastTries.Count() > maxPastTries {
		var deleteKey string
		db.pastTries.IterCb(func(k string, v interface{}) bool {
			if k != key {
				deleteKey = key
				return true
			}
			return false
		})
		db.pastTries.Remove(deleteKey)
	}
}

func (db *cachingDB) OpenStorageTrie(prefix []byte, addrHash, root common.Hash) (Trie, error) {
	var (
		t   *trie.SecureTrie
		err error
	)
	dbtable := db.Table(prefix)
	t, err = trie.NewSecure(root, dbtable, 0)
	if err != nil {
		return nil, err
	}
	return cachedTrie{t, db, dbtable}, nil
}

func (db *cachingDB) Table(prefix []byte) *DBTable {
	return newTable(db.Database(), prefix)
}

func (db *cachingDB) CopyTrie(t Trie) Trie {
	switch t := t.(type) {
	case cachedTrie:
		return cachedTrie{t.SecureTrie.Copy(), db, t.dbTable}
	default:
		panic(fmt.Errorf("unknown trie type %T", t))
	}
}

func (db *cachingDB) ContractCode(prefix []byte, addrHash, codeHash common.Hash) ([]byte, error) {
	code, err := db.Table(prefix).Get(codeHash[:])
	if err == nil {
		db.codeSizeCache.Add(codeHash, len(code))
	}
	return code, err
}

func (db *cachingDB) ContractCodeSize(prefix []byte, addrHash, codeHash common.Hash) (int, error) {
	if cached, ok := db.codeSizeCache.Get(codeHash); ok {
		return cached.(int), nil
	}
	code, err := db.ContractCode(prefix, addrHash, codeHash)
	return len(code), err
}
func (db *cachingDB) Database() ntsdb.Database {
	return db.db
}

// cachedTrie inserts its trie into a cachingDB on commit.
type cachedTrie struct {
	*trie.SecureTrie
	db      *cachingDB
	dbTable *DBTable
}

func (m cachedTrie) CommitTo(dbw trie.Database, number uint64) (common.Hash, error) {
	predb := newTable(dbw, m.dbTable.table)
	predb.SetValuePrefix(number)
	root, err := m.SecureTrie.CommitTo(predb)
	if err == nil {
		m.db.pushTrie(m.SecureTrie)
	}
	return root, err
}

func (m cachedTrie) DB() *DBTable {
	return m.dbTable
}
