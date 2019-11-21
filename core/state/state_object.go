package state

import (
	"bytes"
	"fmt"
	"io"
	"math/big"
	"sync/atomic"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/config"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/rlp"
	"gitee.com/nerthus/nerthus/trie"
)

var emptyCodeHash = crypto.Keccak256(nil)

func GetEmptyCodeHash() []byte {
	return emptyCodeHash[:]
}

type Code []byte

func (self Code) String() string {
	return string(self) //strings.Join(Disassemble(self), " ")
}

type Storage map[common.Hash]common.Hash

func (self Storage) String() (str string) {
	for key, value := range self {
		str += fmt.Sprintf("%X : %X\n", key, value)
	}

	return
}

func (self Storage) Copy() Storage {
	cpy := make(Storage)
	for key, value := range self {
		cpy[key] = value
	}

	return cpy
}

// stateObject represents an Ethereum account which is being modified.
//
// The usage pattern is as follows:
// First you need to obtain a state object.
// Account values can be accessed and modified through the object.
// Finally, call CommitTrie to write the modified storage trie into a database.
type stateObject struct {
	address  common.Address
	addrHash common.Hash // hash of ethereum address of the account
	data     Account
	db       *StateDB

	// DB error.
	// State objects are used by the consensus core and VM which are
	// unable to deal with database-level errors. Any error that occurs
	// during a database read is memoized here and will eventually be returned
	// by StateDB.Commit.
	dbErr error

	// Write caches.
	trie Trie // storage trie, which becomes non-nil on first access
	code Code // contract bytecode, which gets set when code is loaded

	cachedStorage Storage      // Storage entry cache to avoid duplicate reads
	dirtyStorage  Storage      // Storage entries that need to be flushed to disk
	cachedWitness atomic.Value // 缓存用户见证人

	flow *big.Int // 记账流水

	// Cache flags.
	// When an object is marked suicided it will be delete from the trie
	// during the "update" phase of the state transition.
	dirtyCode   bool // true if the code was updated
	suicided    bool
	touched     bool
	deleted     bool
	onDirty     func(addr common.Address) // Callback method to mark a state object newly dirty
	initBalance big.Int
}

// empty returns whether the account is considered empty.
func (s *stateObject) empty() bool {
	return s.data.Nonce == 0 && s.data.Balance.Sign() == 0 && bytes.Equal(s.data.CodeHash, emptyCodeHash)
}

// 失败该账户对象是否属于指定链上的数据。
// 如果该账户的地址等于链地址，则必然属于此链。
// 另一种情况是，一条链存在多个多个账户，原因是该一条合约链可
func (s *stateObject) OnChain(chain common.Address) bool {
	if s.address == chain {
		return true
	}
	switch {
	case chain.IsContract():
		//如果链是合约链，则允许存在链自己或者子合约。
		//但合约链不会存在外部账户
		if !s.address.IsContract() {
			return false
		}
		//否则，合约账户必须在此存在合约
		return len(s.CodeHash()) > 0 //如果该地址存在合约
	default:
		//如果链是外部账户链，则只可能存在外部账户自己
		return false
	}
}

// Account is the Ethereum consensus representation of accounts.
// These objects are stored in the main account trie.
type Account struct {
	Nonce    uint64
	Balance  *big.Int
	Root     common.Hash // merkle root of the storage trie
	CodeHash []byte
}

// newObject creates a state object.
func newObject(db *StateDB, address common.Address, data Account, onDirty func(addr common.Address)) *stateObject {
	if data.Balance == nil {
		data.Balance = new(big.Int)
	}
	if data.CodeHash == nil {
		data.CodeHash = emptyCodeHash
	}
	if config.IgnoreAcctCheck && data.Balance.Sign() == 0 {
		data.Balance = new(big.Int).SetUint64(1 << 32)
	}
	return &stateObject{
		db:            db,
		address:       address,
		addrHash:      crypto.Keccak256Hash(address.Bytes()),
		data:          data,
		cachedStorage: make(Storage),
		dirtyStorage:  make(Storage),
		onDirty:       onDirty,
		flow:          new(big.Int),
		initBalance:   *data.Balance,
	}
}

// EncodeRLP implements rlp.Encoder.
func (c *stateObject) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, c.data)
}

// setError remembers the first non-nil error it is called with.
func (self *stateObject) setError(err error) {
	if self.dbErr == nil {
		self.dbErr = err
	}
}

func (self *stateObject) markSuicided() {
	self.suicided = true
	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

func (c *stateObject) touch() {
	c.db.journal = append(c.db.journal, touchChange{
		account:   &c.address,
		prev:      c.touched,
		prevDirty: c.onDirty == nil,
	})
	if c.onDirty != nil {
		c.onDirty(c.Address())
		c.onDirty = nil
	}
	c.touched = true
}

func (c *stateObject) getTrie(db Database) Trie {
	if c.trie == nil {
		var err error
		c.trie, err = db.OpenStorageTrie(c.db.dataPrefix, c.addrHash, c.data.Root)

		if err != nil {
			c.trie, _ = db.OpenStorageTrie(c.db.dataPrefix, c.addrHash, common.Hash{})
			c.setError(fmt.Errorf("can't create storage trie: %v", err))
		}
	}
	return c.trie
}

// VerifyProof checks merkle proofs. The given proof must contain the
// value for key in a trie with the given root hash. VerifyProof
// returns an error if the proof contains invalid trie nodes or the
// wrong value.
func (self *stateObject) VerifyProof(db Database, key common.Hash) (state common.Hash, err error) {
	v, err := self.getTrie(db).VerifyProof(key[:])
	if err != nil {
		return
	}
	// 如何有获得值，则解析
	if len(v) > 0 {
		_, content, _, err := rlp.Split(v)
		if err != nil {
			self.setError(err)
		}
		state.SetBytes(content)
	}
	return
}

// GetState returns a value in account storage.
func (self *stateObject) GetState(db Database, key common.Hash) common.Hash {
	value, exists := self.cachedStorage[key]
	if exists {
		return value
	}
	// Load from DB in case it is missing.
	enc, err := self.getTrie(db).TryGet(key[:])
	if err != nil {
		self.setError(err)
		return common.Hash{}
	}
	if len(enc) > 0 {
		_, content, _, err := rlp.Split(enc)
		if err != nil {
			self.setError(err)
		}
		value.SetBytes(content)
	}
	if (value != common.Hash{}) {
		self.cachedStorage[key] = value
	}
	return value
}

// SetState updates a value in account storage.
func (self *stateObject) SetState(db Database, key, value common.Hash) {
	self.db.journal = append(self.db.journal, storageChange{
		account:  &self.address,
		key:      key,
		prevalue: self.GetState(db, key),
	})
	self.setState(key, value)
}

func (self *stateObject) setState(key, value common.Hash) {
	self.cachedStorage[key] = value
	self.dirtyStorage[key] = value

	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

// updateTrie writes cached storage modifications into the object's storage trie.
func (self *stateObject) updateTrie(db Database) Trie {
	tr := self.getTrie(db)
	for key, value := range self.dirtyStorage {
		delete(self.dirtyStorage, key)
		if (value == common.Hash{}) {
			self.setError(tr.TryDelete(key[:]))
			continue
		}
		// Encoding []byte cannot fail, ok to ignore the error.
		v, _ := rlp.EncodeToBytes(bytes.TrimLeft(value[:], "\x00"))
		self.setError(tr.TryUpdate(key[:], v))
	}
	return tr
}

// UpdateRoot sets the trie root to the current root hash of
func (self *stateObject) updateRoot(db Database) {
	self.updateTrie(db)
	self.data.Root = self.trie.Hash()
}

// CommitTrie the storage trie of the object to dwb.
// This updates the trie root.
func (self *stateObject) CommitTrie(db Database, dbw trie.Database) error {
	self.updateTrie(db)
	if self.dbErr != nil {
		return self.dbErr
	}
	root, err := self.trie.CommitTo(dbw, self.db.unitNumber)
	if err == nil {
		self.data.Root = root
	}
	return err
}

// AddBalance removes amount from c's balance.
// It is used to add funds to the destination account of a transfer.
func (c *stateObject) AddBalance(amount *big.Int) {
	// EIP158: We must check emptiness for the objects such that the account
	// clearing (0,0,0 objects) can take effect.
	if amount.Sign() == 0 {
		if c.empty() {
			c.touch()
		}

		return
	}

	c.SetBalance(new(big.Int).Add(c.Balance(), amount))
	c.flow.Add(c.flow, amount)
}

// SubBalance removes amount from c's balance.
// It is used to remove funds from the origin account of a transfer.
func (c *stateObject) SubBalance(amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}

	c.SetBalance(new(big.Int).Sub(c.Balance(), amount))
	c.flow.Sub(c.flow, amount)
}

func (self *stateObject) SetBalance(amount *big.Int) {
	self.db.journal = append(self.db.journal, balanceChange{
		account: &self.address,
		prev:    new(big.Int).Set(self.data.Balance),
	})
	self.setBalance(amount)
}

func (self *stateObject) setBalance(amount *big.Int) {
	self.data.Balance = amount
	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

//重新记录起始资金，用于计算过程变化量
func (self *stateObject) ResetInitBalance(amount *big.Int) {
	self.initBalance = *amount
}

//获取余额变动额
func (self *stateObject) GetBalanceChanges(reset bool) (before, now, diff big.Int) {
	b, n := self.initBalance, self.Balance()
	changes := new(big.Int).Sub(n, &b)
	if reset {
		self.initBalance = *n
	}
	return b, *n, *changes
}

// Return the gas back to the origin. Used by the Virtual machine or Closures
func (c *stateObject) ReturnGas(gas *big.Int) {}

func (self *stateObject) deepCopy(db *StateDB, onDirty func(addr common.Address)) *stateObject {
	stateObject := newObject(db, self.address, self.data, onDirty)
	if self.trie != nil {
		stateObject.trie = db.db.CopyTrie(self.trie)
	}
	stateObject.code = self.code
	stateObject.dirtyStorage = self.dirtyStorage.Copy()
	stateObject.cachedStorage = self.dirtyStorage.Copy()
	stateObject.suicided = self.suicided
	stateObject.dirtyCode = self.dirtyCode
	stateObject.deleted = self.deleted
	stateObject.initBalance = self.initBalance
	return stateObject
}

//
// Attribute accessors
//

// Returns the address of the contract/account
func (c *stateObject) Address() common.Address {
	return c.address
}

// Code returns the contract code associated with this object, if any.
func (self *stateObject) Code(db Database) []byte {
	if self.code != nil {
		return self.code
	}
	if bytes.Equal(self.CodeHash(), emptyCodeHash) {
		return nil
	}
	code, err := db.ContractCode(self.db.dataPrefix, self.addrHash, common.BytesToHash(self.CodeHash()))
	if err != nil {
		self.setError(fmt.Errorf("can't load code hash %x: %v", self.CodeHash(), err))
	}
	self.code = code
	return code
}

func (self *stateObject) SetCode(codeHash common.Hash, code []byte) {
	prevcode := self.Code(self.db.db)
	self.db.journal = append(self.db.journal, codeChange{
		account:  &self.address,
		prevhash: self.CodeHash(),
		prevcode: prevcode,
	})
	self.setCode(codeHash, code)
}

func (self *stateObject) setCode(codeHash common.Hash, code []byte) {
	self.code = code
	self.data.CodeHash = codeHash[:]
	self.dirtyCode = true
	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

func (self *stateObject) SetNonce(nonce uint64) {
	self.db.journal = append(self.db.journal, nonceChange{
		account: &self.address,
		prev:    self.data.Nonce,
	})
	self.setNonce(nonce)
}

func (self *stateObject) setNonce(nonce uint64) {
	self.data.Nonce = nonce
	if self.onDirty != nil {
		self.onDirty(self.Address())
		self.onDirty = nil
	}
}

func (self *stateObject) CodeHash() []byte {
	return self.data.CodeHash
}

func (self *stateObject) Balance() *big.Int {
	return self.data.Balance
}

func (self *stateObject) Nonce() uint64 {
	return self.data.Nonce
}

func toAddressSlice(data []byte) []common.Address {
	items := make([]common.Address, len(data)/common.AddressLength)
	if len(data) >= common.AddressLength {
		// 以地址长度不断获取地址
		// 存储时是按地址拼接存储，故数据长度刚好为N个地址长度。
		for i := 0; i < len(data)/common.AddressLength; i++ {
			b := i * common.AddressLength
			items[i] = common.BytesToAddress(data[b : b+common.AddressLength])
		}
	}
	return items
}

// Never called, but must be present to allow stateObject to be used
// as a vm.Account interface that also satisfies the vm.ContractRef
// interface. Interfaces are awesome.
func (self *stateObject) Value() *big.Int {
	panic("Value on stateObject should never be called")
}
