package core

import (
	"crypto/ecdsa"
	"encoding/json"
	"testing"
	"time"

	"math/big"

	"gitee.com/nerthus/nerthus/common/tracetime"
	"gitee.com/nerthus/nerthus/log"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/ntsdb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadAndSaveChildrenRelation(t *testing.T) {
	db, err := ntsdb.NewMemDatabase()
	if err != nil {
		t.Fatalf("fail to new membase: %v", err)
	}

	check := func(parents, childrens []common.Hash) {
		for _, p := range parents {
			var got []common.Hash
			WalkChildren(db, p, func(children common.Hash) bool {
				got = append(got, children)
				return false
			})
			//个数应该是新单元数量
			assert.Len(t, got, len(childrens), "the children count should be equal new unit count")
			for _, u := range childrens {
				assert.Contains(t, got, u, "the children should be contain the new unit")
			}
		}
		// 新单元则无子单元
		for _, u := range childrens {
			WalkChildren(db, u, func(children common.Hash) bool {
				assert.Fail(t, "the new unit 's children should be empty")
				return true
			})
		}
	}
	t.Run("one", func(t *testing.T) {
		parents := []common.Hash{
			common.StringToHash("p1"),
			common.StringToHash("sc1"),
		}
		unit := types.NewUnit(&types.Header{
			ParentHash: parents[0],
			SCHash:     parents[1],
		}, nil)
		err := WriteChildrens(db, unit)
		assert.NoError(t, err)

		check(parents, []common.Hash{unit.Hash()})
	})

	t.Run("more", func(t *testing.T) {
		parents := []common.Hash{
			common.StringToHash("2p1"),
			common.StringToHash("2sc1"),
			common.StringToHash("2p2"),
		}
		var unitHashs []common.Hash
		//新建多个单元，则父单元的子单元存在多个
		for i := 0; i < 3; i++ {
			unit := types.NewUnit(&types.Header{
				ParentHash: parents[0],
				SCHash:     parents[1],
				Timestamp:  uint64(i),
			}, types.TxExecs{
				{},
				{PreStep: types.UnitID{common.StringToAddress("oo1"), 1, parents[0]}},
				{PreStep: types.UnitID{common.StringToAddress("oo2"), 2, parents[2]}},
			})
			err := WriteChildrens(db, unit)
			assert.NoError(t, err)
			unitHashs = append(unitHashs, unit.Hash())
			t.Log(unit.Hash().Hex())
		}

		// 校验获取是否正确
		check(parents, unitHashs)
	})
}

func defaultTestKey() (*ecdsa.PrivateKey, common.Address) {
	key, _ := crypto.HexToECDSA("45a915e4d060149eb4365960e6a7a45f334393093061116b197e3240065ff2d8")
	addr := crypto.ForceParsePubKeyToAddress(key.PublicKey)
	return key, addr
}

func TestUnitStorageDefault(t *testing.T) {
	db, _ := ntsdb.NewMemDatabase()
	addr := common.StringToAddress("addr1")
	tx := types.NewTransaction(addr, addr, big.NewInt(1), 1000, 3, 0, nil)
	txs := []*types.TransactionExec{
		{Action: types.ActionTransferPayment,
			Tx:     tx,
			TxHash: tx.Hash(),
		},
	}
	unit := types.NewBlockWithHeader(&types.Header{
		MC:         addr,
		Number:     1,
		ParentHash: types.EmptyParentHash,
		SCHash:     types.EmptyParentHash,
	}).WithBody(txs, []types.WitenssVote{}, types.SignContent{})
	if err := WriteUnit(db, unit, nil); err != nil {
		t.Fatalf("Failed to write unit into database: %v", err)
	}
	if entry := GetUnit(db, unit.Hash()); entry == nil {
		t.Fatalf("Stored unit not found")
	} else if entry.Hash() != unit.Hash() {
		t.Fatalf("Retrieved unit mismatch: have %v, want %v", entry, unit)
	}
}

func TestBlockStorage(t *testing.T) {
	db, _ := ntsdb.NewMemDatabase()

	addr := common.StringToAddress("addr1")
	tx := types.NewTransaction(addr, addr, big.NewInt(1), 1000, 3, 0, nil)
	txs := []*types.TransactionExec{
		{Action: types.ActionTransferPayment,
			Tx:     tx,
			TxHash: tx.Hash(),
		},
	}

	unit := types.NewBlockWithHeader(&types.Header{
		MC:         addr,
		Number:     1,
		ParentHash: types.EmptyParentHash,
		SCHash:     types.EmptyParentHash,
		TxsRoot:    types.EmptyRootHash,
	}).WithBody(txs, nil, types.SignContent{})
	getUnit := func() *types.Unit {
		return GetUnit(db, unit.Hash())
	}
	getHeader := func() *types.Header {
		return GetHeader(db, unit.Hash())
	}
	getBody := func() *types.Body {
		return GetBody(db, unit.MC(), unit.Number(), unit.Hash())
	}

	if entry := getUnit(); entry != nil {
		t.Fatalf("Non existent unit returned: %v", entry)
	}
	if entry := getHeader(); entry != nil {
		t.Fatalf("Non existent header returned: %v", entry)
	}
	if entry := getBody(); entry != nil {
		t.Fatalf("Non existent body returned: %v", entry)
	}
	if err := WriteUnit(db, unit, nil); err != nil {
		t.Fatalf("Failed to write unit into database: %v", err)
	}

	if entry := getUnit(); entry == nil {
		t.Fatalf("Stored unit not found")
	} else if entry.Hash() != unit.Hash() {
		t.Fatalf("Retrieved unit mismatch: have %v, want %v", entry, unit)
	}
	if entry := getHeader(); entry == nil {
		t.Fatalf("Stored header not found")
	} else if entry.Hash() != unit.Header().Hash() {
		t.Fatalf("Retrieved header mismatch: have %v, want %v", entry, unit.Header())
	}
	if entry := getBody(); entry == nil {
		t.Fatalf("Stored body not found")
	} else if types.DeriveSha(types.TxExecs(entry.Txs)) != types.DeriveSha(unit.Transactions()) {
		t.Fatalf("Retrieved body mismatch: have %v, want %v", entry, unit.Body())
	}

	got := GetTransaction(db, tx.Hash())
	require.NotNil(t, got)
	require.Equal(t, tx.Hash(), got.Hash())

	// 删除后，应该DB Key 为空
	//DeleteUnit(db, unit.MC(), unit.Hash(), unit.Number())
	//if count := len(db.Keys()); count > 0 {
	//	t.Fatalf("Not cleaned, db keys count %d", count)
	//}
}

func TestTraceDB(t *testing.T) {
	log.Root().SetHandler(log.StdoutHandler)

	defer tracetime.New().Stop()
	time.Sleep(time.Second * 2)
	defer tracetime.New().Stop()
}

func TestDelAddrNodeList(t *testing.T) {
	p := common.StringToHash("32")
	b := append(append(unitChildrenPrefix, p.Bytes()...), unitChildrenCountSuffix...)
	t.Log(len(b), cap(b))
}

func TestGetUnitReceipt(t *testing.T) {
	db, _ := ntsdb.NewMemDatabase()

	var (
		uhash           = common.StringToHash("uint1")
		chain           = common.StringToAddress("chainA")
		number   uint64 = 10
		receipts types.Receipts
	)

	got := GetUnitReceipts(db, chain, uhash, number)
	require.Empty(t, got)

	receipts = append(receipts, types.NewReceipt(types.ActionPowTxContractDeal, common.StringToHash("root"), false, big.NewInt(101)))
	receipts = append(receipts, types.NewReceipt(types.ActionContractReceipt, common.StringToHash("root2"), false, big.NewInt(102)))
	WriteUnitReceipts(db, uhash, chain, number, receipts)

	//读取
	got = GetUnitReceipts(db, chain, uhash, number)
	require.Len(t, got, 2)
	require.Equal(t, receipts[0].Action, got[0].Action)
	require.Equal(t, receipts[1].Action, got[1].Action)

	require.Equal(t, receipts[0].Failed, got[0].Failed)
	require.Equal(t, receipts[1].Failed, got[1].Failed)

	require.Equal(t, receipts[0].CumulativeGasUsed, got[0].CumulativeGasUsed)
	require.Equal(t, receipts[1].CumulativeGasUsed, got[1].CumulativeGasUsed)

}

func TestWriteUnitReceipts(t *testing.T) {

	var rec types.Receipt

	info := `
{
            "root": "0x2cf3c6d4a7757fcf2c4c9bec5cffabf16a9a51e0d70e9a2d4abd4b64c1706517",
            "failed": true,
            "cumulative_gasUsed": "0xadc57",
            "logs_bloom": "0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
            "logs": [],
            "action": "contract_deal",
            "pre_step": {
                "chain_id": "nts1wlddd64hgk9r9nn6xf7wzzqyd2uvm22de3s5nm",
                "height": 5,
                "hash": "0x78352fa8c2a2849b18bac03f78f498c50bb46e97d3d7b9378b92e61ee730bda8"
            },
            "tx_hash": "0xa243ff5a8bbc17014a15085f3268065b4177e6bd169973bd48479d65908beb1b",
            "contract_address": "",
            "gas_remaining": "0x0",
            "gas_used": "0xadc57",
            "coinflows": null,
            "output": "0x"
}
`
	err := json.Unmarshal([]byte(info), &rec)
	require.NoError(t, err)
	require.True(t, rec.Failed)

	//存储
	db := ntsdb.NewMemDB()

	uhash := common.HexToHash("0xa05ec05236accb2a4a01380fa807862d6e804e6735fc8ead5e6f047d3ddd5a8f")
	chain := common.ForceDecodeAddress("nts1wlddd64hgk9r9nn6xf7wzzqyd2uvm22de3s5nm")
	var number uint64 = 488

	WriteUnitReceipts(db, uhash, chain, number, types.Receipts{&rec})

	//再获取
	recs := GetUnitReceipts(db, chain, uhash, number)
	require.Len(t, recs, 1)
	require.True(t, recs[0].Failed)
}
