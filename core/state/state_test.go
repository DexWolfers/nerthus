package state

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"testing"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/rlp"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	checker "gopkg.in/check.v1"
)

type StateSuite struct {
	db    *ntsdb.MemDatabase
	state *StateDB
}

var _ = checker.Suite(&StateSuite{})

var toAddr = common.BytesToAddress

func TestDiffState(t *testing.T) {
	db, _ := ntsdb.NewMemDatabase()
	db2 := NewDatabase(db)
	prefix := []byte{1, 2, 3}

	type info struct {
		Root    common.Hash
		Addr    common.Address
		Balance *big.Int
	}

	caces := make([]info, 10)

	for i := 0; i < 10; i++ {
		item := info{
			Addr:    toAddr([]byte{0x01}),
			Balance: big.NewInt(int64(i * 2)),
		}
		if i != 0 {
			item.Root = caces[i-1].Root
		}
		st, _ := New(prefix, item.Root, db2)
		st.SetBalance(item.Addr, item.Balance)

		item.Root, _ = st.CommitTo(db)
		caces[i] = item
	}
	for _, item := range caces {
		st, _ := New(prefix, item.Root, db2)
		if got := st.GetBalance(item.Addr); got.Cmp(item.Balance) != 0 {
			t.Fatalf("expect %d,unexpected %d", item.Balance, got)
		}
	}

}

func (s *StateSuite) TestDump(c *checker.C) {

	// generate a few entries
	obj1 := s.state.GetOrNewStateObject(toAddr([]byte{0x01}))
	obj1.AddBalance(big.NewInt(22))
	obj2 := s.state.GetOrNewStateObject(toAddr([]byte{0x01, 0x02}))
	obj2.SetCode(crypto.Keccak256Hash([]byte{3, 3, 3, 3, 3, 3, 3}), []byte{3, 3, 3, 3, 3, 3, 3})
	obj3 := s.state.GetOrNewStateObject(toAddr([]byte{0x02}))
	obj3.SetBalance(big.NewInt(44))

	// write some of them to the trie
	s.state.updateStateObject(obj1)
	s.state.updateStateObject(obj2)
	_, err := s.state.CommitTo(s.db)
	if err != nil {
		c.Fatal(err)
	}

	// check that dump contains the state objects that are in trie
	got := string(s.state.Dump())
	want := `{
    "root": "71edff0130dd2385947095001c73d9e28d862fc286fca2b922ca6f6f3cddfdd2",
    "accounts": {
        "0000000000000000000000000000000000000001": {
            "balance": "22",
            "nonce": 0,
            "root": "56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
            "codeHash": "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470",
            "code": "",
            "storage": {}
        },
        "0000000000000000000000000000000000000002": {
            "balance": "44",
            "nonce": 0,
            "root": "56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
            "codeHash": "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470",
            "code": "",
            "storage": {}
        },
        "0000000000000000000000000000000000000102": {
            "balance": "0",
            "nonce": 0,
            "root": "56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
            "codeHash": "87874902497a5bb968da31a2998d8f22e949d1ef6214bcdedd8bae24cca4b9e3",
            "code": "03030303030303",
            "storage": {}
        }
    }
}`
	if got != want {
		c.Errorf("dump mismatch:\ngot: %s\nwant: %s\n", got, want)
	}
}

func (s *StateSuite) SetUpTest(c *checker.C) {
	s.db, _ = ntsdb.NewMemDatabase()
	s.state, _ = New([]byte{1, 2, 3}, common.Hash{}, NewDatabase(s.db))
}

func (s *StateSuite) TestNull(c *checker.C) {
	address := common.ForceDecodeAddress("nts1e5r8jrtwhpapx6tjs5rnqu4qm65pmyc25wwuhd")
	s.state.CreateAccount(address)
	//value := common.FromHex("0x823140710bf13990e4500136726d8b55")
	var value common.Hash
	s.state.SetState(address, common.Hash{}, value)
	s.state.CommitTo(s.db)
	value = s.state.GetState(address, common.Hash{})
	if !common.EmptyHash(value) {
		c.Errorf("expected empty hash. got %x", value)
	}
}

func (s *StateSuite) TestSnapshot(c *checker.C) {
	stateobjaddr := toAddr([]byte("aa"))
	var storageaddr common.Hash
	data1 := common.BytesToHash([]byte{42})
	data2 := common.BytesToHash([]byte{43})

	// set initial state object value
	s.state.SetState(stateobjaddr, storageaddr, data1)
	// get snapshot of current state
	snapshot := s.state.Snapshot()

	// set new state object value
	s.state.SetState(stateobjaddr, storageaddr, data2)
	// restore snapshot
	s.state.RevertToSnapshot(snapshot)

	// get state storage value
	res := s.state.GetState(stateobjaddr, storageaddr)

	c.Assert(data1, checker.DeepEquals, res)
}

func (s *StateSuite) TestSnapshotEmpty(c *checker.C) {
	s.state.RevertToSnapshot(s.state.Snapshot())
}

// use testing instead of checker because checker does not support
// printing/logging in tests (-check.vv does not work)
func TestSnapshot2(t *testing.T) {
	db, _ := ntsdb.NewMemDatabase()
	prefix := []byte{1, 2, 3}
	state, _ := New(prefix, common.Hash{}, NewDatabase(db))

	stateobjaddr0 := toAddr([]byte("so0"))
	stateobjaddr1 := toAddr([]byte("so1"))
	var storageaddr common.Hash

	data0 := common.BytesToHash([]byte{17})
	data1 := common.BytesToHash([]byte{18})

	state.SetState(stateobjaddr0, storageaddr, data0)
	state.SetState(stateobjaddr1, storageaddr, data1)
	// db, trie are already non-empty values
	so0 := state.getStateObject(stateobjaddr0)
	so0.SetBalance(big.NewInt(42))
	so0.SetNonce(43)
	so0.SetCode(crypto.Keccak256Hash([]byte{'c', 'a', 'f', 'e'}), []byte{'c', 'a', 'f', 'e'})
	so0.suicided = false
	so0.deleted = false
	state.setStateObject(so0)
	state.SetBalance(stateobjaddr1, big.NewInt(1))
	root, err := state.CommitTo(db)
	if err != nil {
		t.Fatal(err)
	}
	state.Reset(prefix, root)

	so1 := state.getStateObject(stateobjaddr1)
	so1.SetBalance(big.NewInt(52))
	so1.SetNonce(53)
	so1.SetCode(crypto.Keccak256Hash([]byte{'c', 'a', 'f', 'e', '2'}), []byte{'c', 'a', 'f', 'e', '2'})
	so1.suicided = true
	so1.deleted = true
	state.setStateObject(so1)

	so1 = state.getStateObject(stateobjaddr1)
	if so1 != nil {
		t.Fatalf("deleted object not nil when getting")
	}

	snapshot := state.Snapshot()
	state.RevertToSnapshot(snapshot)

	so0Restored := state.getStateObject(stateobjaddr0)
	// Update lazily-loaded values before comparing.
	so0Restored.GetState(state.db, storageaddr)
	so0Restored.Code(state.db)
	// non-deleted is equal (restored)
	compareStateObjects(so0Restored, so0, t)

	// deleted should be nil, both before and after restore of state copy
	so1Restored := state.getStateObject(stateobjaddr1)
	if so1Restored != nil {
		t.Fatalf("deleted object not nil after restoring snapshot: %+v", so1Restored)
	}
}

func compareStateObjects(so0, so1 *stateObject, t *testing.T) {
	if so0.Address() != so1.Address() {
		t.Fatalf("Address mismatch: have %v, want %v", so0.address, so1.address)
	}
	if so0.Balance().Cmp(so1.Balance()) != 0 {
		t.Fatalf("Balance mismatch: have %v, want %v", so0.Balance(), so1.Balance())
	}
	if so0.Nonce() != so1.Nonce() {
		t.Fatalf("Nonce mismatch: have %v, want %v", so0.Nonce(), so1.Nonce())
	}
	if so0.data.Root != so1.data.Root {
		t.Errorf("Root mismatch: have %x, want %x", so0.data.Root[:], so1.data.Root[:])
	}
	if !bytes.Equal(so0.CodeHash(), so1.CodeHash()) {
		t.Fatalf("CodeHash mismatch: have %v, want %v", so0.CodeHash(), so1.CodeHash())
	}
	if !bytes.Equal(so0.code, so1.code) {
		t.Fatalf("Code mismatch: have %v, want %v", so0.code, so1.code)
	}

	if len(so1.cachedStorage) != len(so0.cachedStorage) {
		t.Errorf("Storage size mismatch: have %d, want %d", len(so1.cachedStorage), len(so0.cachedStorage))
	}
	for k, v := range so1.cachedStorage {
		if so0.cachedStorage[k] != v {
			t.Errorf("Storage key %x mismatch: have %v, want %v", k, so0.cachedStorage[k], v)
		}
	}
	for k, v := range so0.cachedStorage {
		if so1.cachedStorage[k] != v {
			t.Errorf("Storage key %x mismatch: have %v, want none.", k, v)
		}
	}

	if so0.suicided != so1.suicided {
		t.Fatalf("suicided mismatch: have %v, want %v", so0.suicided, so1.suicided)
	}
	if so0.deleted != so1.deleted {
		t.Fatalf("Deleted mismatch: have %v, want %v", so0.deleted, so1.deleted)
	}
}

func TestAccounting(t *testing.T) {
	db, _ := ntsdb.NewMemDatabase()
	prefix := []byte{1, 2, 3}
	statedb, _ := New(prefix, common.Hash{}, NewDatabase(db))

	addr := common.BytesToAddress([]byte("1"))
	addr2 := common.BytesToAddress([]byte("2"))

	statedb.AddBalance(addr, big.NewInt(20))
	statedb.AddBalance(addr, big.NewInt(31))
	statedb.AddBalance(addr2, big.NewInt(34))
	statedb.AddBalance(addr, big.NewInt(45))
	statedb.AddBalance(addr2, big.NewInt(15))
	statedb.AddBalance(addr, big.NewInt(40))
	statedb.AddBalance(addr2, big.NewInt(78))

	accounting := statedb.GetCoinflows()
	printAccoutning(accounting)

	root, _ := statedb.CommitTo(db)
	t.Log("Root", root.String())

	statedb.AddBalance(addr, big.NewInt(50))
	statedb.AddBalance(addr2, big.NewInt(28))
	statedb.AddBalance(addr, big.NewInt(19))
	statedb.AddBalance(addr2, big.NewInt(48))

	accounting = statedb.GetCoinflows()
	printAccoutning(accounting)
}

func printAccoutning(accounting types.Coinflows) {
	for _, a := range accounting {
		fmt.Println(a.To.String(), a.Amount)
	}
}

func TestStateVerifyProof(t *testing.T) {
	addr := common.StringToAddress("user1")
	state, kv := randomState(addr, 10)

	obj := state.GetOrNewStateObject(addr)
	for k, v := range kv {
		got, err := obj.VerifyProof(state.db, k)
		if assert.NoError(t, err) {
			assert.Equal(t, v, got)
		}
		t.Log(got.Hex())
		if t.Failed() {
			return
		}
	}
}

func randomState(addr common.Address, n int) (*StateDB, map[common.Hash]common.Hash) {
	db, _ := ntsdb.NewMemDatabase()
	prefix := []byte{1, 2, 3}
	state, _ := New(prefix, common.Hash{}, NewDatabase(db))

	vals := make(map[common.Hash]common.Hash)
	for i := byte(0); i < 100; i++ {
		key1, value1 := common.BytesToHash(common.LeftPadBytes([]byte{i}, 32)), common.BytesToHash([]byte{i})
		key2, value2 := common.BytesToHash(common.LeftPadBytes([]byte{i + 10}, 32)), common.BytesToHash([]byte{i})

		state.SetState(addr, key1, value1)
		state.SetState(addr, key2, value2)

		vals[key1] = value1
		vals[key2] = value2
	}
	for i := 0; i < n; i++ {
		key, value := common.BytesToHash(randBytes(32)), common.BytesToHash(randBytes(20))
		state.SetState(addr, key, value)
		vals[key] = value
	}
	// empty
	for i := 0; i < n; i++ {
		key, value := common.BytesToHash(randBytes(32)), common.Hash{}
		state.SetState(addr, key, value)
		vals[key] = value
	}
	state.GetOrNewStateObject(addr).updateRoot(state.db)
	return state, vals
}

func randBytes(n int) []byte {
	r := make([]byte, n)
	rand.Read(r)
	return r
}

func TestStateDB_BigData(t *testing.T) {
	db, _ := ntsdb.NewMemDatabase()
	prefix := []byte{1, 2, 3}
	state, _ := New(prefix, common.Hash{}, NewDatabase(db))

	data := bytes.NewBufferString("")
	for i := 0; i < 10000; i++ {
		data.WriteString(fmt.Sprintf("%d", i))
	}

	addr, key := common.StringToAddress("addr1"), []byte("ak")
	err := state.SetBigData(addr, key, data.Bytes())
	assert.NoError(t, err)

	got, err := state.GetBigData(addr, key)
	assert.NoError(t, err)
	assert.Equal(t, data.String(), string(got), "the data should be equal stored")
}

func TestBigData(t *testing.T) {

	type info struct {
		Index  uint64
		Values []common.Address
	}

	one := info{
		Index: math.MaxUint64,
		Values: []common.Address{
			common.StringToAddress("1"),
			common.StringToAddress("2"),
			common.StringToAddress("3"),
		},
	}

	data, err := rlp.EncodeToBytes(one)
	if !assert.NoError(t, err) {
		return
	}

	db, _ := ntsdb.NewMemDatabase()
	state, _ := New([]byte{1, 2, 3}, common.Hash{}, NewDatabase(db))

	add0, key := toAddr([]byte("account0")), []byte("mykey")
	err = state.SetBigData(add0, key, data)
	t.Log(data)
	if assert.NoError(t, err) {
		got, err := state.GetBigData(add0, key)
		if assert.NoError(t, err) {
			assert.Equal(t, data, got)

			var two info
			err = rlp.DecodeBytes(got, &two)
			if assert.NoError(t, err) {
				assert.Equal(t, one, two)
			}
		}
	}
}

func TestBigData_DiffLen(t *testing.T) {

	//create value
	cv := func(c int) []byte {
		v := make([]byte, c)
		for i := range v {
			v[i] = 1 + byte(c)
		}
		return v
	}

	// 测试不同长度的值存储
	caces := []struct {
		data []byte
		key  []byte
	}{
		{cv(2), []byte("k1")},
		{cv(30), []byte("k2")},
		{cv(32), []byte("k3")},
		{cv(33), []byte("k4")},
		{cv(32 * 2), []byte("k4")},
		{cv(32*2 + 1), []byte("k5")},
		{cv(32*3 - 1), []byte("k6")},
	}

	db, _ := ntsdb.NewMemDatabase()
	state, _ := New([]byte{1, 2, 3}, common.Hash{}, NewDatabase(db))
	add0 := toAddr([]byte("account0"))
	for _, c := range caces {
		state.SetBigData(add0, c.key, c.data)
		got, err := state.GetBigData(add0, c.key)
		require.NoError(t, err)
		require.Equal(t, c.data, got, "should get data with key %s", string(c.key))
	}
}

// 数据分块测试
func TestBlockData(t *testing.T) {
	t.Log(big.NewInt(100).Bytes())

	//	value := []byte(`1234567812345678123999999999234923532857342985023859034852098320580392850
	//123214jsdfjcxklvjdslvdcdanfewfldsjklcjxlvjefoijo342r89yfvdgyre7gyd8v7ydf8v7ycxh9yr2kh423uyf9yu8ds9uf234fhdsfy
	//oiufdgu34oijfdlksflvdcdanfewfldsjklcjxlvjefoijo342r89yfvdgyre7gyd8v7ydf8v7ycxh9yr2kh423uyf9yu8ds9uf234fhdsfy
	//oiufdgu34oijfdlksf
	//lvdcdanfewfldsjklcjxlvjefoijo342r89yfvdgyre7gyd8v7ydf8v7ycxh9yr2kh423uyf9yu8ds9uf234fhdsfy
	//oiufdgu34oijfdlksf
	//lvdcdanfewfldsjklcjxlvjefoijo342r89yfvdgyre7gyd8v7ydf8v7ycxh9yr2kh423uyf9yu8ds9uf234fhdsfy
	//oiufdgu34oijfdlksflvdcdanfewfldsjklcjxlvjefoijo342r89yfvdgyre7gyd8v7ydf8v7ycxh9yr2kh423uyf9yu8ds9uf234fhdsfy
	//oiufdgu34oijfdlksflvdcdanfewfldsjklcjxlvjefoijo342r89yfvdgyre7gyd8v7ydf8v7ycxh9yr2kh423uyf9yu8ds9uf234fhdsfy
	//oiufdgu34oijfdlksf
	//lvdcdanfewfldsjklcjxlvjefoijo342r89yfvdgyre7gyd8v7ydf8v7ycxh9yr2kh423uyf9yu8ds9uf234fhdsfy
	//oiufdgu34oijfdlksflvdcdanfewfldsjklcjxlvjefoijo342r89yfvdgyre7gyd8v7ydf8v7ycxh9yr2kh423uyf9yu8ds9uf234fhdsfy
	//oiufdgu34oijfdlksflvdcdanfewfldsjklcjxlvjefoijo342r89yfvdgyre7gyd8v7ydf8v7ycxh9yr2kh423uyf9yu8ds9uf234fhdsfy
	//oiufdgu34oijfdlksflvdcdanfewfldsjklcjxlvjefoijo342r89yfvdgyre7gyd8v7ydf8v7ycxh9yr2kh423uyf9yu8ds9uf234fhdsfy
	//oiufdgu34oijfdlksf
	//lvdcdanfewfldsjklcjxlvjefoijo342r89yfvdgyre7gyd8v7ydf8v7ycxh9yr2kh423uyf9yu8ds9uf234fhdsfy
	//oiufdgu34oijfdlksflvdcdanfewfldsjklcjxlvjefoijo342r89yfvdgyre7gyd8v7ydf8v7ycxh9yr2kh423uyf9yu8ds9uf234fhdsfy
	//oiufdgu34oijfdlksflvdcdanfewfldsjklcjxlvjefoijo342r89yfvdgyre7gyd8v7ydf8v7ycxh9yr2kh423uyf9yu8ds9uf234fhdsfy
	//oiufdgu34oijfdlksflvdcdanfewfldsjklcjxlvjefoijo342r89yfvdgyre7gyd8v7ydf8v7ycxh9yr2kh423uyf9yu8ds9uf234fhdsfy
	//oiufdgu34oijfdlksflvdcdanfewfldsjklcjxlvjefoijo342r89yfvdgyre7gyd8v7ydf8v7ycxh9yr2kh423uyf9yu8ds9uf234fhdsfy
	//oiufdgu34oijfdlksflvdcdanfewfldsjklcjxlvjefoijo342r89yfvdgyre7gyd8v7ydf8v7ycxh9yr2kh423uyf9yu8ds9uf234fhdsfy
	//oiufdgu34oijfdlksflvdcdanfewfldsjklcjxlvjefoijo342r89yfvdgyre7gyd8v7ydf8v7ycxh9yr2kh423uyf9yu8ds9uf234fhdsfy
	//oiufdgu34oijfdlksflvdcdanfewfldsjklcjxlvjefoijo342r89yfvdgyre7gyd8v7ydf8v7ycxh9yr2kh423uyf9yu8ds9uf234fhdsfy
	//oiufdgu34oijfdlksflvdcdanfewfldsjklcjxlvjefoijo342r89yfvdgyre7gyd8v7ydf8v7ycxh9yr2kh423uyf9yu8ds9uf234fhdsfy
	//oiufdgu34oijfdlksflvdcdanfewfldsjklcjxlvjefoijo342r89yfvdgyre7gyd8v7ydf8v7ycxh9yr2kh423uyf9yu8ds9uf234fhdsfy
	//oiufdgu34oijfdlksflvdcdanfewfldsjklcjxlvjefoijo342r89yfvdgyre7gyd8v7ydf8v7ycxh9yr2kh423uyf9yu8ds9uf234fhdsfy
	//oiufdgu34oijfdlksflvdcdanfewfldsjklcjxlvjefoijo342r89yfvdgyre7gyd8v7ydf8v7ycxh9yr2kh423uyf9yu8ds9uf234fhdsfy
	//oiufdgu34oijfdlksflvdcdanfewfldsjklcjxlvjefoijo342r89yfvdgyre7gyd8v7ydf8v7ycxh9yr2kh423uyf9yu8ds9uf234fhdsfy
	//oiufdgu34oijfdlksflvdcdanfewfldsjklcjxlvjefoijo342r89yfvdgyre7gyd8v7ydf8v7ycxh9yr2kh423uyf9yu8ds9uf234fhdsfy
	//oiufdgu34oijfdlksf
	//000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000005678
	//`)
	value := []byte{248, 74, 136, 255, 255, 255, 255, 255, 255, 255, 255, 248, 63, 148, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 49, 148, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 50, 148, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 51}

	// 计算长度
	length := int64(len(value))
	size := int64(8)
	// 计算需要分隔的块数
	number := int64(length / size)
	last := int64(length % size)
	if last > 0 {
		number++
	}

	// 分隔存储
	var store []common.Hash
	for i := int64(0); i < number; i++ {
		if last > 0 && i == number-1 {
			t.Log(common.BytesToHash(value[i*size : (i*size)+last]))
			store = append(store, common.BytesToHash(value[i*size:(i*size)+last]))
		} else {
			t.Log(common.BytesToHash(value[i*size : (i+1)*size]))
			store = append(store, common.BytesToHash(value[i*size:(i+1)*size]))
		}
	}
	var result []byte
	for i := 0; i < len(store); i++ {
		b := store[i]
		if last != 0 && i == len(store)-1 {
			result = append(result, b[32-last:]...)
		} else {
			result = append(result, b[32-size:]...)
		}
	}
	//t.Log(string(result))
	t.Log(result)
}

func TestNewAcc(t *testing.T) {
	db, _ := ntsdb.NewMemDatabase()
	state, _ := New([]byte{1, 2, 3}, common.Hash{}, NewDatabase(db))

	addr := common.StringToAddress("a")
	state.SetBalance(addr, big.NewInt(100))

	obj1 := state.GetOrNewStateObject(toAddr([]byte{0x01}))
	obj1.AddBalance(big.NewInt(22))
	obj1.setState(common.StringToHash("key1"), common.StringToHash("value1"))
	obj2 := state.GetOrNewStateObject(toAddr([]byte{0x01, 0x02}))
	obj2.SetCode(crypto.Keccak256Hash([]byte{3, 3, 3, 3, 3, 3, 3}), []byte{3, 3, 3, 3, 3, 3, 3})
	obj2.setState(common.StringToHash("key1"), common.StringToHash("value1"))

	// write some of them to the trie
	state.updateStateObject(obj1)
	state.updateStateObject(obj2)

	root, err := state.CommitTo(db)
	require.NoError(t, err)

	state2, err := New([]byte{1, 2, 3}, root, NewDatabase(db))
	require.NoError(t, err)

	t.Log(string(state2.Dump()))

	state2.SetState(obj2.address, common.StringToHash("key1"), common.Hash{})
	state2.CommitTo(db)
	t.Log(state2.GetBalance(addr))
	t.Log(state2.GetBalance(obj1.address))
	t.Log(state2.GetBalance(obj2.address))
	t.Log(state2.GetState(obj1.address, common.StringToHash("key1")))
	t.Log(state2.GetState(obj2.address, common.StringToHash("key1")))
}

func TestDeleteOutsideChain(t *testing.T) {
	db, _ := ntsdb.NewMemDatabase()
	state, _ := New([]byte{1, 2, 3}, common.Hash{}, NewDatabase(db))

	a, b := common.StringToAddress("a"), common.StringToAddress("b")
	state.SetBalance(a, big.NewInt(1))
	state.SetBalance(b, big.NewInt(2))

	root, _ := state.IntermediateRoot(a)

	root2, err := state.CommitToChain(db, a)
	require.NoError(t, err)
	require.Equal(t, root, root2, "should be equal")
}
func TestSnapshotOnDelete(t *testing.T) {
	db, _ := ntsdb.NewMemDatabase()
	state, _ := New([]byte{1, 2, 3}, common.Hash{}, NewDatabase(db))

	a, b := common.StringToAddress("a"), common.StringToAddress("b")
	state.SetBalance(a, big.NewInt(1))
	state.SetBalance(b, big.NewInt(2))

	root, _ := state.IntermediateRoot(a)

	sid := state.Snapshot()
	state.SetBalance(b, big.NewInt(3))
	state.SetBalance(a, big.NewInt(4))
	state.SetBalance(common.StringToAddress("c"), big.NewInt(5))

	state.RevertToSnapshot(sid)
	root2, _ := state.IntermediateRoot(a)
	require.Equal(t, root.Hex(), root2.Hex(), "should be not change any things after revert")
	require.Equal(t, 2, len(state.GetObjects()), "should be only have chain 1")
	require.Contains(t, state.GetObjects(), a)
	require.Contains(t, state.GetObjects(), b)
}
