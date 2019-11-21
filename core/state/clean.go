package state

import (
	"bytes"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/rlp"
	"gitee.com/nerthus/nerthus/trie"
)

func NewCleaner(db ntsdb.Database, chain common.Address, from, to uint64, stateDB *StateDB, getTrieRoot func(chain common.Address, number uint64) common.Hash) *Cleaner {
	clner := &Cleaner{
		cacheTries:    make(map[string]*trie.Trie),
		db:            newTable(db, chain.Bytes()),
		chain:         chain,
		from:          from,
		to:            to,
		stateDB:       stateDB,
		getTrieRoot:   getTrieRoot,
		cacheVerified: make(map[common.Hash]struct{}),
	}
	return clner
}

type Cleaner struct {
	cacheTries    map[string]*trie.Trie
	cacheVerified map[common.Hash]struct{}
	getTrieRoot   func(chain common.Address, number uint64) common.Hash
	db            trie.PrefixDatabase
	chain         common.Address
	from          uint64
	to            uint64
	stateDB       *StateDB // 有效的树
	seq           int64
}

func (t *Cleaner) StartClear() int {
	totalRemoved := 0
	for i := t.to - 1; i >= t.from; i-- {
		now := time.Now()
		seq := t.seq
		root := t.getTrieRoot(t.chain, i)
		if root.Empty() {
			log.Debug("skip clear empty root", "number", i)
			continue
		}
		// number前缀变更
		t.db.SetValuePrefix(i)
		removeCount, err := t.clearOne(root, t.to)
		log.Debug("clear state space of one number", "removed", removeCount, "seq", t.seq-seq, "total_seq", t.seq, "chain", t.chain, "number", i, "root", root, "cost", time.Now().Sub(now), "err", err)
		totalRemoved += removeCount
	}
	return totalRemoved
}

func (t *Cleaner) clearOne(root common.Hash, deadline uint64) (int, error) {
	//s, err := New(root, NewDatabase(t.db))
	//if err != nil {
	//	log.Debug("miss state root,skip", "number", t.stateDB.unitNumber, "err", err)
	//	return 0, nil
	//}
	keyCount := 0
	// 获取账户trie下的所有子trie
	getAccTrie := func(key, value []byte) {
		// 待删除的trie
		var data Account
		if err := rlp.DecodeBytes(value, &data); err != nil {
			log.Debug("Failed to decode state object", "key", common.Bytes2Hex(key), "err", err)
			return
		}
		cn := trie.NewCleaner(t.db, data.Root.Bytes(), nil, t.cacheVerified)

		// 缓存取 有效的trie
		currTrie := t.loadOrCacheTrie(key, func() *trie.Trie {
			bts, err := cn.GetByOriginKey(key, t.stateDB.trie.Trie())
			if err != nil {
				log.Debug("failed to get deadline trie", "key", common.Bytes2Hex(key), "err", err)
				return nil
			}
			var currData Account
			if err := rlp.DecodeBytes(bts, &currData); err != nil {
				log.Debug("Failed to decode state object", "key", common.Bytes2Hex(key), "bytes", len(bts), "err", err)
				return nil
			}

			t, err := trie.New(currData.Root, t.db)
			if err != nil {
				log.Error("failed to create current trie", "root", currData.Root, "err", err)
				return nil
			}
			return t
		})
		if currTrie == nil {
			return
		}
		if count, err := t.clearTrie(cn, currTrie, deadline); err != nil {
			log.Error("clear child trie err", "err", err)
		} else {
			keyCount += count
		}
	}
	// 先清理账户trie
	cleaner := trie.NewCleaner(t.db, root.Bytes(), getAccTrie, t.cacheVerified)
	c, err := t.clearTrie(cleaner, t.stateDB.trie.Trie(), deadline)
	if err != nil {
		log.Error("clear account trie err", "err", err)
	}
	keyCount += c
	return keyCount, err
}

func (t *Cleaner) clearTrie(cleaner *trie.TrieCleaner, deadTrie *trie.Trie, number uint64) (int, error) {
	if bytes.Equal(cleaner.Root(), deadTrie.Hash().Bytes()) {
		log.Trace("skip clean one trie", "root", deadTrie.Hash())
		return 0, nil
	}
	count, err := cleaner.Clean(deadTrie, number)
	t.seq += cleaner.Seq()
	//log.Debug("clean one trie finish", "keys", count, "dirty", cleaner.Root(), "curr", deadTrie.Hash(), "seq", cleaner.Seq(), "err", err)
	return count, err
}

func (t *Cleaner) loadOrCacheTrie(key []byte, f func() *trie.Trie) *trie.Trie {
	hex := common.Bytes2Hex(key)
	if v, ok := t.cacheTries[hex]; ok {
		return v
	}
	v := f()
	if v == nil {
		return nil
	}
	t.cacheTries[hex] = v
	return v
}

func (t *Cleaner) initWhiteList(db ntsdb.Database) {
	// 创世数据
	t.cacheVerified = make(map[common.Hash]struct{})
	if t.chain != params.SCAccount {
		return
	}
	u0 := t.getTrieRoot(params.SCAccount, 0)
	cacheAllKey := func(key []byte) {
		t.cacheVerified[common.BytesToHash(key)] = struct{}{}
	}
	RangeTrieKey(db, params.SCAccount, u0.Bytes(), cacheAllKey)
	log.Debug("get genesis trie keys", "count", len(t.cacheVerified))
}
func RangeTrieKey(db trie.Database, chain common.Address, root []byte, f func(key []byte)) {
	predb := newTable(db, chain.Bytes())
	tn := trie.NewCleaner(predb, root, func(key, value []byte) {
		// 获取全部子树
		var data Account
		if err := rlp.DecodeBytes(value, &data); err != nil {
			log.Debug("Failed to decode state object", "key", common.Bytes2Hex(key), "err", err)
			return
		}
		childCln := trie.NewCleaner(predb, data.Root.Bytes(), nil, nil)
		childCln.RangeAllKeys(f)
	}, nil)
	tn.RangeAllKeys(f)
}
func (s *StateDB) GetTrie(addr common.Address) Trie {
	return s.getStateObject(addr).getTrie(s.db)
}
