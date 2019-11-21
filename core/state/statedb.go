// Package state provides a caching layer atop the Ethereum state trie.
package state

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"sync"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/tracetime"
	"gitee.com/nerthus/nerthus/core/config"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/rlp"
	"gitee.com/nerthus/nerthus/trie"
)

type revision struct {
	id           int
	journalIndex int
}

// StateDBs within the ethereum protocol are used to store anything
// within the merkle trie. StateDBs take care of caching and storing
// nested states. It's the general query interface to retrieve:
// * Contracts
// * Accounts
type StateDB struct {
	dataPrefix []byte //数据存储前缀
	db         Database
	trie       Trie

	// This map holds 'live' objects, which will get modified while processing a state transition.
	stateObjects      map[common.Address]*stateObject
	stateObjectsDirty map[common.Address]struct{}

	// DB error.
	// State objects are used by the consensus core and VM which are
	// unable to deal with database-level errors. Any error that occurs
	// during a database read is memoized here and will eventually be returned
	// by StateDB.Commit.
	dbErr error

	// The refund counter, also used by state transitioning.
	refund uint64

	thash, bhash common.Hash
	txIndex      int
	unitNumber   uint64
	logs         map[common.Hash][]*types.Log
	logSize      uint

	preimages map[common.Hash][]byte

	// Journal of state modifications. This is the backbone of
	// Snapshot and RevertToSnapshot.
	journal        journal
	validRevisions []revision
	nextRevisionId int

	balanceChanges map[common.Address]big.Int

	lock sync.RWMutex
}

// Create a new state from a given trie
func New(prefix []byte, root common.Hash, db Database) (*StateDB, error) {
	tr, err := db.OpenTrie(prefix, root)
	if err != nil {
		return nil, err
	}
	state := &StateDB{
		dataPrefix:        prefix,
		db:                db,
		trie:              tr,
		stateObjects:      make(map[common.Address]*stateObject),
		stateObjectsDirty: make(map[common.Address]struct{}),
		logs:              make(map[common.Hash][]*types.Log),
		preimages:         make(map[common.Hash][]byte),
		balanceChanges:    make(map[common.Address]big.Int),
	}
	return state, nil
}

// setError remembers the first non-nil error it is called with.
func (self *StateDB) setError(err error) {
	if self.dbErr == nil {
		self.dbErr = err
	}
}

func (self *StateDB) Error() error {
	return self.dbErr
}

// Reset clears out all emphemeral state objects from the state db, but keeps
// the underlying state trie to avoid reloading data for the next operations.
func (self *StateDB) Reset(prefix []byte, root common.Hash) error {
	tr, err := self.db.OpenTrie(prefix, root)
	if err != nil {
		return err
	}
	self.trie = tr
	self.stateObjects = make(map[common.Address]*stateObject)
	self.stateObjectsDirty = make(map[common.Address]struct{})
	self.thash = common.Hash{}
	self.bhash = common.Hash{}
	self.txIndex = 0
	self.logs = make(map[common.Hash][]*types.Log)
	self.logSize = 0
	self.preimages = make(map[common.Hash][]byte)
	self.balanceChanges = make(map[common.Address]big.Int)
	self.clearJournalAndRefund()
	return nil
}

func (self *StateDB) AddLog(log *types.Log) {
	self.journal = append(self.journal, addLogChange{txhash: self.thash})

	log.TxHash = self.thash
	log.UnitHash = self.bhash
	log.UnitIndex = self.unitNumber
	log.TxIndex = uint(self.txIndex)
	log.Index = self.logSize
	self.logs[self.thash] = append(self.logs[self.thash], log)
	self.logSize++
}

func (self *StateDB) GetLogs(hash common.Hash) []*types.Log {
	return self.logs[hash]
}

func (self *StateDB) Logs() []*types.Log {
	var logs []*types.Log
	for _, lgs := range self.logs {
		logs = append(logs, lgs...)
	}
	return logs
}

// AddPreimage records a SHA3 preimage seen by the VM.
func (self *StateDB) AddPreimage(hash common.Hash, preimage []byte) {
	if _, ok := self.preimages[hash]; !ok {
		self.journal = append(self.journal, addPreimageChange{hash: hash})
		pi := make([]byte, len(preimage))
		copy(pi, preimage)
		self.preimages[hash] = pi
	}
}

// Preimages returns a list of SHA3 preimages that have been submitted.
func (self *StateDB) Preimages() map[common.Hash][]byte {
	return self.preimages
}

func (self *StateDB) AddRefund(gas uint64) {
	self.journal = append(self.journal, refundChange{prev: self.refund})
	self.refund += gas
}

// SubRefund removes gas from the refund counter.
// This method will panic if the refund counter goes below zero
func (self *StateDB) SubRefund(gas uint64) {
	self.journal = append(self.journal, refundChange{prev: self.refund})
	if gas > self.refund {
		panic("Refund counter below zero")
	}
	self.refund -= gas
}

// Exist reports whe
// Exist reports whether the given account address exists in the state.
// Notably this also returns true for suicided accounts.
func (self *StateDB) Exist(addr common.Address) bool {
	return self.getStateObject(addr) != nil
}

// Empty returns whether the state object is either non-existent
// or empty according to the EIP161 specification (balance = nonce = code = 0)
func (self *StateDB) Empty(addr common.Address) bool {
	so := self.getStateObject(addr)
	return so == nil || so.empty()
}

// Retrieve the balance from the given address or 0 if object not found
func (self *StateDB) GetBalance(addr common.Address) *big.Int {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.Balance()
	}
	if config.IgnoreAcctCheck {
		return big.NewInt(1 << 32)
	}
	return common.Big0
}

func (self *StateDB) GetNonce(addr common.Address) uint64 {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.Nonce()
	}

	return 0
}

func (self *StateDB) GetCode(addr common.Address) []byte {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.Code(self.db)
	}
	return nil
}

func (self *StateDB) GetCodeSize(addr common.Address) int {
	stateObject := self.getStateObject(addr)
	if stateObject == nil {
		return 0
	}
	if stateObject.code != nil {
		return len(stateObject.code)
	}
	size, err := self.db.ContractCodeSize(stateObject.db.dataPrefix, stateObject.addrHash, common.BytesToHash(stateObject.CodeHash()))
	if err != nil {
		self.setError(err)
	}
	return size
}

func (self *StateDB) GetCodeHash(addr common.Address) common.Hash {
	stateObject := self.getStateObject(addr)
	if stateObject == nil {
		return common.Hash{}
	}
	return common.BytesToHash(stateObject.CodeHash())
}

func (self *StateDB) GetState(a common.Address, b common.Hash) common.Hash {
	stateObject := self.getStateObject(a)
	if stateObject != nil {
		return stateObject.GetState(self.db, b)
	}
	return common.Hash{}
}

// StorageTrie returns the storage trie of an account.
// The return value is a copy and is nil for non-existent accounts.
func (self *StateDB) StorageTrie(a common.Address) Trie {
	stateObject := self.getStateObject(a)
	if stateObject == nil {
		return nil
	}
	cpy := stateObject.deepCopy(self, nil)
	return cpy.updateTrie(self.db)
}

func (self *StateDB) HasSuicided(addr common.Address) bool {
	stateObject := self.getStateObject(addr)
	if stateObject != nil {
		return stateObject.suicided
	}
	return false
}

/*
 * SETTERS
 */

// AddBalance adds amount to the account associated with addr
func (self *StateDB) AddBalance(addr common.Address, amount *big.Int) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.AddBalance(amount)
	}
}

// SubBalance subtracts amount from the account associated with addr
func (self *StateDB) SubBalance(addr common.Address, amount *big.Int) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SubBalance(amount)
	}
}

func (self *StateDB) SetBalance(addr common.Address, amount *big.Int) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetBalance(amount)
	}
}

func (self *StateDB) SetNonce(addr common.Address, nonce uint64) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetNonce(nonce)
	}
}

func (self *StateDB) SetCode(addr common.Address, code []byte) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetCode(crypto.Keccak256Hash(code), code)
	}
}

func (self *StateDB) SetState(addr common.Address, key common.Hash, value common.Hash) {
	stateObject := self.GetOrNewStateObject(addr)
	if stateObject != nil {
		stateObject.SetState(self.db, key, value)
	}
}

// Suicide marks the given account as suicided.
// This clears the account balance.
//
// The account's state object is still available until the state is committed,
// getStateObject will return a non-nil account after Suicide.
func (self *StateDB) Suicide(addr common.Address) bool {
	stateObject := self.getStateObject(addr)
	if stateObject == nil {
		return false
	}
	self.journal = append(self.journal, suicideChange{
		account:     &addr,
		prev:        stateObject.suicided,
		prevbalance: new(big.Int).Set(stateObject.Balance()),
	})
	stateObject.markSuicided()
	stateObject.data.Balance = new(big.Int)

	return true
}

//
// Setting, updating & deleting state object methods
//

// updateStateObject writes the given object to the trie.
func (self *StateDB) updateStateObject(stateObject *stateObject) {
	// 更新是需清理流水
	stateObject.flow.SetInt64(0)
	addr := stateObject.Address()
	data, err := rlp.EncodeToBytes(stateObject)
	if err != nil {
		panic(fmt.Errorf("can't encode object at %s: %v", addr, err))
	}
	self.setError(self.trie.TryUpdate(addr.Bytes(), data))
}

// deleteStateObject removes the given object from the state trie.
func (self *StateDB) deleteStateObject(stateObject *stateObject) {
	stateObject.deleted = true
	addr := stateObject.Address()
	self.setError(self.trie.TryDelete(addr.Bytes()))
}

// Retrieve a state object given my the address. Returns nil if not found.
func (self *StateDB) getStateObject(addr common.Address) (stateObject *stateObject) {
	// Prefer 'live' objects.
	if obj := self.stateObjects[addr]; obj != nil {
		if obj.deleted {
			return nil
		}
		return obj
	}
	// Load the object from the database.
	enc, err := self.trie.TryGet(addr.Bytes())
	if len(enc) == 0 {
		self.setError(err)
		return nil
	}

	var data Account
	if err := rlp.DecodeBytes(enc, &data); err != nil {
		log.Error("Failed to decode state object", "addr", addr, "err", err)
		return nil
	}
	// Insert into the live set.
	obj := newObject(self, addr, data, self.MarkStateObjectDirty)
	self.setStateObject(obj)
	return obj
}

func (self *StateDB) setStateObject(object *stateObject) {
	//记录第一次载入账户的余额信息
	if _, ok := self.balanceChanges[object.Address()]; !ok {
		self.balanceChanges[object.Address()] = *object.Balance()
	}
	self.stateObjects[object.Address()] = object
}

// Retrieve a state object or create a new state object if nil
func (self *StateDB) GetOrNewStateObject(addr common.Address) *stateObject {
	stateObject := self.getStateObject(addr)
	if stateObject == nil || stateObject.deleted {
		stateObject, _ = self.createObject(addr)
	}
	return stateObject
}

// MarkStateObjectDirty adds the specified object to the dirty map to avoid costly
// state object cache iteration to find a handful of modified ones.
func (self *StateDB) MarkStateObjectDirty(addr common.Address) {
	self.stateObjectsDirty[addr] = struct{}{}
}

// createObject creates a new state object. If there is an existing account with
// the given address, it is overwritten and returned as the second return value.
func (self *StateDB) createObject(addr common.Address) (newobj, prev *stateObject) {
	prev = self.getStateObject(addr)
	newobj = newObject(self, addr, Account{}, self.MarkStateObjectDirty)
	newobj.setNonce(0) // sets the object to dirty
	if prev == nil {
		self.journal = append(self.journal, createObjectChange{account: &addr})
	} else {
		self.journal = append(self.journal, resetObjectChange{prev: prev})
	}
	self.setStateObject(newobj)
	return newobj, prev
}

// CreateAccount explicitly creates a state object. If a state object with the address
// already exists the balance is carried over to the new account.
//
// CreateAccount is called during the EVM CREATE operation. The situation might arise that
// a contract does the following:
//
//   1. sends funds to sha(account ++ (nonce + 1))
//   2. tx_create(sha(account ++ nonce)) (note that this gets the address of 1)
//
// Carrying over the balance ensures that Ether doesn't disappear.
func (self *StateDB) CreateAccount(addr common.Address) {
	new, prev := self.createObject(addr)
	if prev != nil {
		new.setBalance(prev.data.Balance)
	}
}

// VerifyProof checks merkle proofs. The given proof must contain the
// value for key in a trie with the given root hash. VerifyProof
// returns an error if the proof contains invalid trie nodes or the
// wrong value.
func (db *StateDB) VerifyProof(addr common.Address, key common.Hash) (value common.Hash, err error) {
	so := db.getStateObject(addr)
	if so == nil {
		err = errors.New("missing state object")
		return
	}
	return so.VerifyProof(db.db, key)
}
func (db *StateDB) ForEachStorage(addr common.Address, cb func(key, value common.Hash) bool) {
	so := db.getStateObject(addr)
	if so == nil {
		return
	}

	// When iterating over the storage check the cache first
	for h, value := range so.cachedStorage {
		cb(h, value)
	}

	it := trie.NewIterator(so.getTrie(db.db).NodeIterator(nil))
	for it.Next() {
		// ignore cached values
		key := common.BytesToHash(db.trie.GetKey(it.Key))
		if _, ok := so.cachedStorage[key]; !ok {
			if !cb(key, common.BytesToHash(it.Value)) {
				break
			}
		}
	}
}

// Copy creates a deep, independent copy of the state.
// Snapshots of the copied state cannot be applied to the copy.
func (self *StateDB) Copy() *StateDB {
	self.lock.RLock()
	defer self.lock.RUnlock()

	// Copy all the basic fields, initialize the memory ones
	state := &StateDB{
		dataPrefix:        self.dataPrefix,
		db:                self.db,
		trie:              self.db.CopyTrie(self.trie),
		stateObjects:      make(map[common.Address]*stateObject, len(self.stateObjectsDirty)),
		stateObjectsDirty: make(map[common.Address]struct{}, len(self.stateObjectsDirty)),
		refund:            self.refund,
		logs:              make(map[common.Hash][]*types.Log, len(self.logs)),
		logSize:           self.logSize,
		preimages:         make(map[common.Hash][]byte),
		balanceChanges:    make(map[common.Address]big.Int),
	}
	// Copy the dirty states, logs, and preimages
	for addr := range self.stateObjectsDirty {
		state.stateObjects[addr] = self.stateObjects[addr].deepCopy(state, state.MarkStateObjectDirty)
		state.stateObjectsDirty[addr] = struct{}{}
	}
	for hash, logs := range self.logs {
		state.logs[hash] = make([]*types.Log, len(logs))
		copy(state.logs[hash], logs)
	}
	for hash, preimage := range self.preimages {
		state.preimages[hash] = preimage
	}
	for addr, b := range self.balanceChanges {
		state.balanceChanges[addr] = b
	}
	return state
}

// NewCopyTrie 新建一个StateDB但是只复用Trie.
// 重复利用Trie查询数据
func (sdb *StateDB) NewCopyTrie() *StateDB {
	return &StateDB{
		dataPrefix:        sdb.dataPrefix,
		db:                sdb.db,
		trie:              sdb.db.CopyTrie(sdb.trie),
		stateObjects:      make(map[common.Address]*stateObject),
		stateObjectsDirty: make(map[common.Address]struct{}),
		logs:              make(map[common.Hash][]*types.Log),
		preimages:         make(map[common.Hash][]byte),
		balanceChanges:    make(map[common.Address]big.Int),
	}
}

// Snapshot returns an identifier for the current revision of the state.
func (self *StateDB) Snapshot() int {
	id := self.nextRevisionId
	self.nextRevisionId++
	self.validRevisions = append(self.validRevisions, revision{id, len(self.journal)})
	return id
}

// RevertToSnapshot reverts all state changes made since the given revision.
func (self *StateDB) RevertToSnapshot(revid int) {
	// Find the snapshot in the stack of valid snapshots.
	idx := sort.Search(len(self.validRevisions), func(i int) bool {
		return self.validRevisions[i].id >= revid
	})
	if idx == len(self.validRevisions) || self.validRevisions[idx].id != revid {
		panic(fmt.Errorf("revision id %v cannot be reverted", revid))
	}
	snapshot := self.validRevisions[idx].journalIndex

	// Replay the journal to undo changes.
	for i := len(self.journal) - 1; i >= snapshot; i-- {
		self.journal[i].undo(self)
	}
	self.journal = self.journal[:snapshot]

	// Remove invalidated snapshots from the stack.
	self.validRevisions = self.validRevisions[:idx]
}

// GetRefund returns the current value of the refund counter.
func (self *StateDB) GetRefund() uint64 {
	return self.refund
}

// Finalise finalises the state by removing the self destructed objects
// and clears the journal as well as the refunds.
func (s *StateDB) Finalise(chain common.Address) {

	for addr, stateObject := range s.stateObjects {
		_, isDirty := s.stateObjectsDirty[addr]
		switch {
		case stateObject.suicided || (isDirty && stateObject.empty()):
			// If the object has been removed, don't bother syncing it
			// and just mark it for deletion in the trie.
			s.deleteStateObject(stateObject)
		case isDirty && (chain.Empty() || stateObject.OnChain(chain)):
			// Update the object in the main account trie.
			stateObject.updateRoot(s.db)
			s.updateStateObject(stateObject)
		}
	}
	s.clearJournalAndRefund()
}

type BalanceJourna struct {
	Address common.Address
	Before  big.Int
	Now     big.Int
	Diff    big.Int
}

// IntermediateRoot computes the current root hash of the state trie.
// It is called in between transactions to get the root hash that
// goes into transaction receipts.
func (s *StateDB) IntermediateRoot(chain common.Address) (common.Hash, []BalanceJourna) {

	logs := s.getBalanceChanged()
	s.Finalise(chain)
	return s.trie.Hash(), logs
}
func (s *StateDB) GetBalanceChanged() []BalanceJourna {
	return s.getBalanceChanged()
}
func (s *StateDB) getBalanceChanged() []BalanceJourna {
	//记录变更信息
	var logs []BalanceJourna
	for addr, obj := range s.stateObjects {
		before, now, diff := obj.GetBalanceChanges(true)
		logs = append(logs, BalanceJourna{
			Address: addr,
			Before:  before,
			Now:     now,
			Diff:    diff,
		})
	}
	if len(logs) > 0 {
		//确保顺序正确
		sort.Slice(logs, func(i, j int) bool {
			return bytes.Compare(logs[i].Address.Bytes(), logs[j].Address.Bytes()) < 0
		})
	}
	return logs
}

// Prepare sets the current transaction hash and index and block hash which is
// used when the EVM emits new state logs.
func (self *StateDB) Prepare(thash, bhash common.Hash, ti int, number uint64) {
	self.thash = thash
	self.bhash = bhash
	self.txIndex = ti
	self.unitNumber = number
}

// 返回当前 State 操作所关联的单元交易编号
// 这需要调用 Prepare 才能设置正确关联。
// 警告：不要过于依赖
func (s *StateDB) TxIndex() int {
	return s.txIndex
}

// 返回当前 State 操作所关联的单元交易哈希
// 这需要调用 Prepare 才能设置正确关联。
// 警告：不要过于依赖
func (s *StateDB) TxHash() common.Hash {
	return s.thash
}

// DeleteSuicides flags the suicided objects for deletion so that it
// won't be referenced again when called / queried up on.
//
// DeleteSuicides should not be used for consensus related updates
// under any circumstances.
func (s *StateDB) DeleteSuicides() {
	// Reset refund so that any used-gas calculations can use this method.
	s.clearJournalAndRefund()

	for addr := range s.stateObjectsDirty {
		stateObject := s.stateObjects[addr]

		// If the object has been removed by a suicide
		// flag the object as deleted.
		if stateObject.suicided {
			stateObject.deleted = true
		}
		delete(s.stateObjectsDirty, addr)
	}
}

func (s *StateDB) clearJournalAndRefund() {
	s.journal = nil
	s.validRevisions = s.validRevisions[:0]
	s.refund = 0
}
func (s *StateDB) CommitTo(dbw trie.Database) (root common.Hash, err error) {
	root, err = s.commitTo(dbw, common.Address{})
	return
}

// 添加 DB 到指定链，因为指定链只应该存在本身属于此链的数据。
// 因此在提交时，如果账户数据不属于此链，即使存在余额信息也将被移除
func (s *StateDB) CommitToChain(dbw trie.Database, chain common.Address) (root common.Hash, err error) {
	return s.commitTo(dbw, chain)
}

// CommitTo writes the state to the given database.
func (s *StateDB) commitTo(dbw trie.Database, chain common.Address) (root common.Hash, err error) {
	tr := tracetime.New().SetMin(0)

	s.lock.Lock()
	tr.Tag()
	defer s.clearJournalAndRefund()

	dbtable := newTable(dbw, s.dataPrefix)
	dbtable.SetValuePrefix(s.unitNumber)
	// Commit objects to the trie.
	for addr, stateObject := range s.stateObjects {
		_, isDirty := s.stateObjectsDirty[addr]
		switch {
		case stateObject.suicided || (isDirty && stateObject.empty()):
			// If the object has been removed, don't bother syncing it
			// and just mark it for deletion in the trie.
			s.deleteStateObject(stateObject)
		case isDirty && (chain.Empty() || stateObject.OnChain(chain)):
			// Write any contract code associated with the state object
			if stateObject.code != nil && stateObject.dirtyCode {
				if err := dbtable.Put(stateObject.CodeHash(), stateObject.code); err != nil {
					return common.Hash{}, err
				}
				stateObject.dirtyCode = false
			}
			// Write any storage changes in the state object to its storage trie.
			if err := stateObject.CommitTrie(s.db, dbw); err != nil {
				return common.Hash{}, err
			}
			// Update the object in the main account trie.
			s.updateStateObject(stateObject)
		}
		delete(s.stateObjectsDirty, addr)

		// 清空记账记录
		// Author: @zwj
		stateObject.flow.SetInt64(0)
		tr.Tag()
	}
	s.lock.Unlock()
	tr.Tag()
	// Write trie changes.
	root, err = s.trie.CommitTo(dbw, s.unitNumber)
	//log.Info("finish commit account trie")
	tr.Tag()
	log.Debug("Trie cache stats after commit", "misses", trie.CacheMisses(), "unloads", trie.CacheUnloads())
	tr.Tag()

	return root, err
}

// GetObjects 获取当前操作过的Object地址
func (s *StateDB) GetObjects() []common.Address {
	addrs := make([]common.Address, 0, len(s.stateObjects))
	for k, v := range s.stateObjects {
		if v.deleted || v.empty() {
			continue
		}
		addrs = append(addrs, k)
	}
	return addrs
}

// Database 获取底层KeyValue DB ，
// 槽糕的提供方式
func (s *StateDB) Database() ntsdb.Database {
	if cachingDB, ok := s.db.(*cachingDB); ok {
		return cachingDB.db
	}
	return nil
}

// 清空交易变更流水信息
func (s *StateDB) ClearFlow() {
	for _, obj := range s.stateObjects {
		obj.flow.SetUint64(0)
	}
}

// 获取记账交易记录
// Author: @zwj
func (s *StateDB) GetCoinflows() types.Coinflows {
	var coinflows types.Coinflows
	for _, stateObj := range s.stateObjects {
		if stateObj.flow.Sign() <= 0 {
			continue
		}
		// 记录流水，流水总账进行值复制
		coinflows = append(coinflows, types.Coinflow{To: stateObj.address, Amount: new(big.Int).Set(stateObj.flow)})
	}

	// 必须排序，map 属于无需内容
	sort.Sort(coinflows)
	return coinflows
}

func (s *StateDB) Clear() {
	s.trie = nil
	for k, obj := range s.stateObjects {
		delete(s.stateObjects, k)
		obj.db = nil
		if obj.trie != nil {
			obj.trie.Clear()
		}
		obj.cachedStorage = nil
		obj.dirtyStorage = nil
	}
	s.logs = nil
	s.preimages = nil
	s.validRevisions = nil

}
func (s *StateDB) Root() common.Hash {
	return s.trie.Hash()
}

// 重新构建一个StateDB，拥有原db中chain的全部数据
func (s *StateDB) NewChainState(chain common.Address) (statedb *StateDB, err error) {
	statedb, err = New(chain.Bytes(), common.Hash{}, s.db)
	obj := s.getStateObject(chain)
	if obj == nil {
		return
	}
	tr := obj.getTrie(s.db)
	if tr == nil {
		return
	}
	accData := Account{Nonce: 0, Balance: obj.Balance()}
	newobj := newObject(statedb, chain, accData, statedb.MarkStateObjectDirty)
	statedb.setStateObject(newobj)
	// 迁移所有key
	storageIt := trie.NewIterator(obj.getTrie(s.db).NodeIterator(nil))
	for storageIt.Next() {
		statedb.SetState(chain, common.BytesToHash(s.trie.GetKey(storageIt.Key)), common.BytesToHash(storageIt.Value))
	}
	return
}
