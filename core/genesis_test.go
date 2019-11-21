package core

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/testutil"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func init() {
	testutil.SetupTestConfig()
}

func TestBadgerData(t *testing.T) {
	log.Root().SetHandler(log.FuncHandler(func(r *log.Record) error {
		return nil
	}))

	g := DevGenesisUnit()
	for k := range g.Alloc {
		delete(g.Alloc, k)
	}

	db, _ := ntsdb.NewMemDatabase()
	_, _, err := SetupGenesisUnit(db, g)
	if err != nil {
		t.Fatal(err)
	}

	//创建完成后，使用 badger 重新创建，对比数据
	f, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.Remove(f)

	badgerDB, err := ntsdb.NewBadger(f)
	require.NoError(t, err)
	_, _, err = SetupGenesisUnit(badgerDB, g)
	if err != nil {
		t.Fatal(err)
	}

	//对比数据
	for _, k := range db.Keys() {
		want, err := db.Get(k)
		require.NoError(t, err)
		got, err := badgerDB.Get(k)
		require.NoError(t, err)

		require.Equal(t, common.Bytes2Hex(want), common.Bytes2Hex(got), "key=%s", common.Bytes2Hex(k))
	}

	t.Run("in-memory", func(t *testing.T) {
		dbIns, err := ntsdb.NewMemDatabase()
		require.NoError(t, err)
		db, err := ntsdb.NewInMemoryDatabase(dbIns)
		require.NoError(t, err)
		testSetupGenesisUnit(t, g, db)
	})
}

func TestSetupGenesisUnit_Mem(t *testing.T) {
	log.Root().SetHandler(log.FuncHandler(func(r *log.Record) error {
		return nil
	}))

	g := DevGenesisUnit()
	for k := range g.Alloc {
		delete(g.Alloc, k)
	}

	print := func(pre string, db ntsdb.Database, t *testing.T) {
		err := db.(ntsdb.Finder).Iterator([]byte{}, func(key, value []byte) bool {
			fmt.Printf("%s\t%s\t%s\n", pre, common.Bytes2Hex(key), common.Bytes2Hex(value))

			return true
		})
		require.NoError(t, err)
	}

	t.Run("mem", func(t *testing.T) {
		dbIns, _ := ntsdb.NewMemDatabase()
		testSetupGenesisUnit(t, g, dbIns)

		//d377455719cec4fd6e2e18931f5b7957d3a051a91fe862304eddae99798c6f38
		print("mem", dbIns, t)

	})
	t.Run("badger", func(t *testing.T) {
		f, err := ioutil.TempDir("", "")
		require.NoError(t, err)
		defer os.Remove(f)

		dbIns, err := ntsdb.NewBadger(f)
		require.NoError(t, err)
		testSetupGenesisUnit(t, g, dbIns)

		print("badger", dbIns, t)
	})
	t.Run("level", func(t *testing.T) {
		f, err := ioutil.TempDir("", "")
		require.NoError(t, err)
		defer os.Remove(f)

		dbIns, err := ntsdb.NewLDBDatabase(f, 1, 1)
		require.NoError(t, err)
		testSetupGenesisUnit(t, g, dbIns)
		print("leveldb", dbIns, t)
	})

	t.Run("in-memory", func(t *testing.T) {
		dbIns, err := ntsdb.NewMemDatabase()
		require.NoError(t, err)
		db, err := ntsdb.NewInMemoryDatabase(dbIns)
		require.NoError(t, err)
		testSetupGenesisUnit(t, g, db)
	})
}

func testSetupGenesisUnit(t *testing.T, g *Genesis, dbIns ntsdb.Database) {
	_, hash, err := SetupGenesisUnit(dbIns, g)
	if err != nil {
		t.Fatal(err)
	}

	if g.Number != 0 {
		t.Fatalf("number of genesis unit should be zero")
	}

	// 创世单元
	unit := GetUnit(dbIns, hash)
	if unit == nil {
		t.Fatal("unexpected nil")
	}

	if !g.ParentHash.Empty() {
		t.Fatalf("parent hash of genesis should be empty")
	}

	// 创世Hash
	if unit.Hash() != hash {
		t.Fatalf("unit hash should be equal")
	}
	//0x33804b81d8c1c1dda589c8154a4ba60d19a6ccc84780520201892e522e1f2c31
	t.Log(unit.Root().Hex())
	// 创世余额
	stateDB, err := state.New(unit.Root(), state.NewDatabase(dbIns))
	if err != nil {
		t.Fatal(err)
	}
	// 创世用户余额确认
	for addr, info := range g.Alloc {
		got := stateDB.GetBalance(addr)
		//如果是见证人/理事将需要扣除抵押
		_, err1 := sc.GetWitnessInfo(stateDB, addr)
		_, err2 := sc.GetCouncilInfo(stateDB, addr)
		if err1 == sc.ErrNoWintessInfo && err2 == sc.ErrNotCouncil {
			if got.Cmp(info.Balance) != 0 {
				t.Fatalf("%s: expect balance %d,unexptected %d", addr.Hex(), info.Balance, got)
			}
		} else {
			require.NotEqual(t, info.Balance, got)
		}
	}

	// 创世账户
	if unit.MC() != params.SCAccount {
		t.Fatalf("main account not match systemChainAccount; mainAccount:%v, syschainAccount:%v", params.SCAccount.String(), unit.MC().String())
	}

	_, hash2, err := SetupGenesisUnit(dbIns, g)
	require.NoError(t, err)
	require.Equal(t, hash.Hex(), hash2.Hex())
}

func TestGetLastWitnesses(t *testing.T) {
	testutil.SetupTestConfig()
	g, err := LoadGenesisConfig()
	require.NoError(t, err)

	dbIns, _ := ntsdb.NewMemDatabase()

	_, _, err = SetupGenesisUnit(dbIns, g)
	require.NoError(t, err)

	chain, err := NewDagChain(dbIns, g.Config, nil, vm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	ucNum := params.ConfigParamsUCWitness
	scNum := params.ConfigParamsSCWitness
	var userWitness common.AddressList
	defaultUserList := DefaultUserWitness()[:ucNum]
	for _, v := range defaultUserList {
		userWitness = append(userWitness, v.Address)
	}
	// 创世用户
	for addr := range g.Alloc {
		// 创世用户见证人
		w, err := chain.GetChainWitnessLib(addr)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, len(w), ucNum,
			"the chain witness should be have %d", ucNum)
		//逻辑已变化
		if !t.Failed() {
			assert.Equal(t, common.AddressList(userWitness),
				common.AddressList(w), "the chain witness should be equal default witenss")
		}
	}

	stateDB, err := chain.GetChainTailState(params.SCAccount)
	if err != nil {
		t.Fatal(err)
	}
	lib, err := sc.ReadSystemWitness(stateDB)

	require.NoError(t, err)

	var want common.AddressList
	defaultList := DefaultSystemWitness()
	for _, v := range defaultList {
		want = append(want, v.Address)
	}
	require.Equal(t, want, lib)

	// 创世系统见证人为
	list, err := chain.GetChainWitnessLib(params.SCAccount)
	//for _, v := range list {
	//	t.Log(v.Hex())
	//}
	assert.Equal(t, want[:scNum], common.AddressList(list))

	t.Log(scNum, len(list))

	t.Run("in-memory", func(t *testing.T) {
		dbIns, err := ntsdb.NewMemDatabase()
		require.NoError(t, err)
		db, err := ntsdb.NewInMemoryDatabase(dbIns)
		require.NoError(t, err)
		testSetupGenesisUnit(t, g, db)
	})
}

func TestBatchCreateKeys(t *testing.T) {
	type keyAddr struct {
		Address string `json: address`
		KeyHex  string `json: keyhex`
		Keys    string `json: keys`
	}

	var keys []keyAddr
	var amounts string

	for i := 0; i < 2000; i++ {
		key, _ := crypto.GenerateKey()
		keyHex := common.Bytes2Hex(crypto.FromECDSA(key))
		pubKey := crypto.ForceParsePubKeyToAddress(key.PublicKey).Hex()
		var kk keyAddr
		kk.Address = pubKey
		kk.KeyHex = keyHex
		kk.Keys = ""
		keys = append(keys, kk)

		arr := `- { address: "` + pubKey + `"` + `, balance: ` + `20000000000000}` + "\n"
		amounts += arr
	}
	b, err := yaml.Marshal(keys)
	if err != nil {
		panic(err)
	}

	tmp := os.TempDir()
	ioutil.WriteFile(path.Join(tmp, "kk.yaml"), b, os.ModePerm)

	b, err = yaml.Marshal(amounts)
	if err != nil {
		panic(err)
	}
	ioutil.WriteFile(path.Join(tmp, "amount.yaml"), b, os.ModePerm)

	t.Log(tmp)
	t.Log(string(b))
}

// 生成地址
func TestCreateKeys(t *testing.T) {
	//t.Skip("just run for create account")
	tmpDir, err := ioutil.TempDir("", "")
	defer os.Remove(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	var accoutns []common.Address
	data := bytes.NewBufferString("\n\n")
	for i := 0; i < 10000; i++ {
		key, _ := crypto.GenerateKey()
		keyHex := common.Bytes2Hex(crypto.FromECDSA(key))

		data.WriteString(fmt.Sprintf(`-
	address: "%s" 
	keyHex: "%s"
`, crypto.ForceParsePubKeyToAddress(key.PublicKey).Hex(), keyHex))

		accoutns = append(accoutns, crypto.ForceParsePubKeyToAddress(key.PublicKey))
	}

	data.WriteString("\n\n------------------\n\n")
	for _, a := range accoutns {
		data.WriteString(fmt.Sprintf("- { address: \"%s\", balance: 20000000000000000}\n", a.Hex()))
	}
	fmt.Println(data.String())
}

func TestGetLastUnit(t *testing.T) {
	g := DevGenesisUnit()
	dbIns, _ := ntsdb.NewMemDatabase()
	_, hash, err := SetupGenesisUnit(dbIns, g)
	if err != nil {
		t.Fatal(err)
	}
	chain, err := NewDagChain(dbIns, g.Config, nil, vm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for addr := range g.Alloc {
		unit := chain.GetUnitByNumber(addr, 0)
		if assert.NotNil(t, unit, "every chain units[0] should be genesis") {
			assert.Equal(t, hash.Hex(), unit.Hash().Hex(), "every chain units[0] should be genesis")
		}
	}
	header := GetHeaderByHash(dbIns, hash)
	assert.NotNil(t, header)
}

func TestGetSystemChainWitness(t *testing.T) {
	testutil.SetupTestConfig()
	g, err := LoadGenesisConfig()
	require.NoError(t, err)

	dbIns, _ := ntsdb.NewMemDatabase()

	_, _, err = SetupGenesisUnit(dbIns, g)
	require.NoError(t, err)
	chain, err := NewDagChain(dbIns, g.Config, nil, vm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	lib, err := chain.GetChainWitnessLib(params.SCAccount)
	require.NoError(t, err)
	require.Len(t, lib, params.ConfigParamsSCWitness)
	t.Log(lib)
}
