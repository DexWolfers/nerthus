package state

import (
	"encoding/json"
	"fmt"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/rlp"
	"gitee.com/nerthus/nerthus/trie"
)

type DumpAccount struct {
	Balance  string            `json:"balance"`
	Nonce    uint64            `json:"nonce"`
	Root     string            `json:"root"`
	CodeHash string            `json:"codeHash"`
	Code     string            `json:"code"`
	Storage  map[string]string `json:"storage"`
}

type Dump struct {
	Root     string                 `json:"root"`
	Accounts map[string]DumpAccount `json:"accounts"`
}

func (self *StateDB) RawDump() Dump {
	dump := Dump{
		Root:     fmt.Sprintf("%x", self.trie.Hash()),
		Accounts: make(map[string]DumpAccount),
	}

	it := trie.NewIterator(self.trie.NodeIterator(nil))
	for it.Next() {
		addr := self.trie.GetKey(it.Key)
		var data Account
		if err := rlp.DecodeBytes(it.Value, &data); err != nil {
			panic(err)
		}

		obj := newObject(self, common.BytesToAddress(addr), data, nil)
		account := DumpAccount{
			Balance:  data.Balance.String(),
			Nonce:    data.Nonce,
			Root:     common.Bytes2Hex(data.Root[:]),
			CodeHash: common.Bytes2Hex(data.CodeHash),
			Code:     common.Bytes2Hex(obj.Code(self.db)),
			Storage:  make(map[string]string),
		}
		storageIt := trie.NewIterator(obj.getTrie(self.db).NodeIterator(nil))
		for storageIt.Next() {
			account.Storage[common.Bytes2Hex(self.trie.GetKey(storageIt.Key))] = common.Bytes2Hex(storageIt.Value)
		}
		dump.Accounts[common.Bytes2Hex(addr)] = account
	}
	return dump
}

func (self *StateDB) Dump() []byte {
	json, err := json.MarshalIndent(self.RawDump(), "", "    ")
	if err != nil {
		fmt.Println("dump err", err)
	}

	return json
}

func (self *StateDB) RangObjects(f func(address common.Address) bool) {
	it := trie.NewIterator(self.trie.NodeIterator(nil))
	for it.Next() {
		addr := self.trie.GetKey(it.Key)
		if !f(common.BytesToAddress(addr)) {
			break
		}
	}
}

// 用于统计数据
func (self *StateDB) Statistics() (accounts int, keys int, size int) {
	it := trie.NewIterator(self.trie.NodeIterator(nil))
	for it.Next() {
		addr := self.trie.GetKey(it.Key)
		var data Account
		if err := rlp.DecodeBytes(it.Value, &data); err != nil {
			panic(err)
		}
		accounts++
		size += len(it.Value) + len(it.Key)
		keys++

		obj := newObject(self, common.BytesToAddress(addr), data, nil)

		it2 := obj.getTrie(self.db).NodeIterator(nil)
		for it2.Next(true) {
			keys++
			if it2.Leaf() {
				size += len(it2.LeafBlob())
				size += len(it2.LeafKey())
			}
			//key长度
			size += common.AddressLength + common.HashLength
			fmt.Printf("\r %s: %d", obj.Address(), keys)
		}
	}
	return
}
