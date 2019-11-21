package core

import (
	"bytes"
	"crypto/ecdsa"
	"io/ioutil"
	"math/big"
	"os"
	"testing"

	"gitee.com/nerthus/nerthus/accounts/abi"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/common/testutil"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"

	"fmt"

	witvote "gitee.com/nerthus/nerthus/consensus/bft/backend"
	"gitee.com/nerthus/nerthus/consensus/ethash"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIdleExec(t *testing.T) {
	testutil.SetupTestConfig()
	db, _ := ntsdb.NewMemDatabase()
	gspec, err := LoadGenesisConfig()
	require.NoError(t, err)
	gspec.MustCommit(db)

	signer := types.NewSigner(gspec.Config.ChainId)

	dagchain, _ := NewDagChain(db, gspec.Config, witvote.NewFaker(), vm.Config{})
	defer dagchain.Stop()

	//系统主见证人发送心跳
	var systemWitness common.Address
	list, err := dagchain.GetChainWitnessLib(params.SCAccount)

	ks := params.GetDevAccounts()
	var keyInfo *ecdsa.PrivateKey
	//已成交
	for _, v := range ks {
		if common.AddressList(list).Have(common.ForceDecodeAddress(v.Address)) {
			systemWitness = common.ForceDecodeAddress(v.Address)
			keyInfo, err = crypto.HexToECDSA(v.KeyHex)
			require.NoError(t, err)

		}
	}
	tx := types.NewTransaction(systemWitness, params.SCAccount, big.NewInt(0), 0, 0,
		0, sc.CreateCallInputData(sc.FuncIdSystemHeartbeat))
	_, err = types.SignTx(tx, signer, keyInfo)
	require.NoError(t, err)

	var gp GasPool
	gp.AddGas(new(big.Int).SetUint64(dagchain.Config().Get(params.ConfigParamsUcGasLimit)))

	state, _ := dagchain.GetChainTailState(params.SCAccount)

	header := dagchain.GetHeaderByHash(dagchain.GetChainTailHash(params.SCAccount))
	header.ParentHash = header.Hash()
	header.SCHash = dagchain.GetChainTailHead(params.SCAccount).Hash()
	header.Number += 1
	header.MC = params.SCAccount
	header.Proposer = systemWitness

	txExec := &types.TransactionExec{
		Tx:     tx,
		TxHash: tx.Hash(),
		Action: types.ActionSCIdleContractDeal,
	}

	r, err := ProcessTransaction(gspec.Config, dagchain, state, header, &gp, txExec, big.NewInt(0), vm.Config{}, nil)

	t.Logf("sender:%s,tx=%s", systemWitness.Hex(), tx.Hash().Hex())
	require.NoError(t, err)
	require.False(t, r.Failed, "freeze should be successfully")
	require.NotZero(t, r.GasUsed)   //因为是免费交易，则无 Gas 使用
	require.Zero(t, r.GasRemaining) //剩余0
	require.Equal(t, tx.Gas(), r.GasRemaining)
	require.Empty(t, r.Coinflows) //应该不存在返还
}

// 如果冻结时的Gas过大时将失败，否则成功
func TestMaxGas(t *testing.T) {
	var (
		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		addr1   = crypto.ForceParsePubKeyToAddress(key1.PublicKey) //0x71562b71999873DB5b286dF957af199Ec94617F7

		//tSmartContractCode="0x6060604052336000806101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff1602179055506106bd806100536000396000f30060606040526004361061008e576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff1680630da229be146100935780632f1aa84b146100bc57806377ca18a61461011457806387db03b71461012c5780638f84aa091461014f578063d0679d34146101a4578063d294cb0f146101db578063fa3bd6c514610228575b600080fd5b341561009e57600080fd5b6100a661024b565b6040518082815260200191505060405180910390f35b610112600480803590602001908201803590602001908080602002602001604051908101604052809392919081815260200183836020028082843782019150505050505091908035906020019091905050610255565b005b61012a600480803590602001909190505061038a565b005b341561013757600080fd5b61014d600480803590602001909190505061048d565b005b341561015a57600080fd5b6101626104f1565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b6101d9600480803573ffffffffffffffffffffffffffffffffffffffff1690602001909190803590602001909190505061051a565b005b34156101e657600080fd5b610212600480803573ffffffffffffffffffffffffffffffffffffffff1690602001909190505061060c565b6040518082815260200191505060405180910390f35b341561023357600080fd5b610249600480803590602001909190505061062d565b005b6000600154905090565b600080600084518402925082341015151561026f57600080fd5b600091505b845182101561038357848281518110151561028b57fe5b9060200190602002015190508073ffffffffffffffffffffffffffffffffffffffff166108fc859081150290604051600060405180830381858888f1935050505015156102d757600080fd5b7f091151c6f9d8fda9318c8ce50640a16ca1379e117d817054f49a68dd3f8518f0338286604051808473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020018373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001828152602001935050505060405180910390a18180600101925050610274565b5050505050565b3373ffffffffffffffffffffffffffffffffffffffff166108fc829081150290604051600060405180830381858888f1935050505015156103ca57600080fd5b7f091151c6f9d8fda9318c8ce50640a16ca1379e117d817054f49a68dd3f8518f06000809054906101000a900473ffffffffffffffffffffffffffffffffffffffff163383604051808473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020018373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001828152602001935050505060405180910390a150565b60006001549050816001600082825401925050819055507f91acf26ac019f933454b8d5ebaceece943fe706a4c5f64064f865edde02e5766816001548460405180848152602001838152602001828152602001935050505060405180910390a15050565b60008060009054906101000a900473ffffffffffffffffffffffffffffffffffffffff16905090565b80341015151561052957600080fd5b8173ffffffffffffffffffffffffffffffffffffffff166108fc829081150290604051600060405180830381858888f19350505050151561056957600080fd5b7f091151c6f9d8fda9318c8ce50640a16ca1379e117d817054f49a68dd3f8518f0338383604051808473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020018373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001828152602001935050505060405180910390a15050565b60008173ffffffffffffffffffffffffffffffffffffffff16319050919050565b60006001549050816001600082825403925050819055507f91acf26ac019f933454b8d5ebaceece943fe706a4c5f64064f865edde02e5766816001548460405180848152602001838152602001828152602001935050505060405180910390a150505600a165627a7a723058201b6b051798b445bcce8abc65cb8543f35c5f372a3c397a772d645d1a188af4e90029"
	)

	testutil.SetupTestConfig()
	db, _ := ntsdb.NewMemDatabase()
	gspec, err := LoadGenesisConfig()
	require.NoError(t, err)
	gspec.Alloc[addr1] = GenesisAccount{Balance: math.MaxBig256}

	gspec.MustCommit(db)
	signer := types.NewSigner(gspec.Config.ChainId)

	dagchain, _ := NewDagChain(db, gspec.Config, witvote.NewFaker(), vm.Config{})
	defer dagchain.Stop()

	createContractData := sc.CreateCallInputData(sc.FuncCreateContract, common.FromHex(tSmartContractCode))

	var gp GasPool
	gp.AddGas(new(big.Int).SetUint64(dagchain.Config().Get(params.ConfigParamsUcGasLimit)))

	tx := types.NewTransaction(addr1, params.SCAccount, big.NewInt(9999), gp.Gas(), 1, 0, createContractData)
	types.SignTx(tx, signer, key1)

	// 冻结
	state, _ := dagchain.GetChainTailState(addr1)

	header := dagchain.GetHeaderByHash(dagchain.GetChainTailHash(addr1))
	header.ParentHash = header.Hash()
	header.SCHash = dagchain.GetChainTailHead(params.SCAccount).Hash()
	header.Number += 1
	header.MC = addr1

	txExec := &types.TransactionExec{
		Tx:     tx,
		TxHash: tx.Hash(),
		Action: types.ActionContractFreeze,
	}
	r, err := ProcessTransaction(gspec.Config, dagchain, state, header, &gp, txExec, big.NewInt(0), vm.Config{}, nil)

	require.NoError(t, err)
	require.False(t, r.Failed, "freeze should be successfully")
}

// 如果冻结时的Gas过大时将失败，否则成功
func TestFreszeIssue(t *testing.T) {
	var (
		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		addr1   = crypto.ForceParsePubKeyToAddress(key1.PublicKey) //0x71562b71999873DB5b286dF957af199Ec94617F7
	)

	testutil.SetupTestConfig()
	db, _ := ntsdb.NewMemDatabase()
	gspec, err := LoadGenesisConfig()
	require.NoError(t, err)

	balance := new(big.Int).SetUint64(gspec.Config.Get("councilMargin"))

	gspec.Alloc[addr1] = GenesisAccount{Balance: balance}

	gspec.MustCommit(db)
	signer := types.NewSigner(gspec.Config.ChainId)

	dagchain, _ := NewDagChain(db, gspec.Config, witvote.NewFaker(), vm.Config{})
	defer dagchain.Stop()

	input := sc.CreateCallInputData(sc.FuncIdSetMemberJoinApply)

	//测试冻结失败情况
	t.Run("ErrInsufficientBalance", func(t *testing.T) {
		var gp GasPool
		gp.AddGas(new(big.Int).SetUint64(dagchain.Config().Get(params.ConfigParamsUcGasLimit)))

		tx := types.NewTransaction(addr1,
			params.SCAccount, balance, gp.Gas(), 1, 0, input)
		types.SignTx(tx, signer, key1)

		// 冻结
		state, _ := dagchain.GetChainTailState(addr1)
		state.SetBalance(addr1, balance) //修改余额，是的无法支付全部费用

		header := dagchain.GetHeaderByHash(dagchain.GetChainTailHash(addr1))
		header.ParentHash = header.Hash()
		header.SCHash = dagchain.GetChainTailHead(params.SCAccount).Hash()
		header.Number += 1
		header.MC = addr1

		txExec := &types.TransactionExec{
			Tx:     tx,
			TxHash: tx.Hash(),
			Action: types.ActionContractFreeze,
		}
		_, err := ProcessTransaction(gspec.Config, dagchain, state, header, &gp, txExec, big.NewInt(0), vm.Config{}, nil)
		require.EqualError(t, err, vm.ErrInsufficientBalance.Error())
	})
	//测试 转账金额与手续费刚好足够情况
	t.Run("ok", func(t *testing.T) {
		var gp GasPool
		gp.AddGas(new(big.Int).SetUint64(dagchain.Config().Get(params.ConfigParamsUcGasLimit)))

		tx := types.NewTransaction(addr1,
			params.SCAccount, balance, gp.Gas(), 1, 0, input)

		types.SignTx(tx, signer, key1)

		// 冻结
		state, _ := dagchain.GetChainTailState(addr1)
		state.SetBalance(addr1, tx.Cost()) //设置为刚好足额

		header := dagchain.GetHeaderByHash(dagchain.GetChainTailHash(addr1))
		header.ParentHash = header.Hash()
		header.SCHash = dagchain.GetChainTailHead(params.SCAccount).Hash()
		header.Number += 1
		header.MC = addr1

		txExec := &types.TransactionExec{
			Tx:     tx,
			TxHash: tx.Hash(),
			Action: types.ActionContractFreeze,
		}
		r, err := ProcessTransaction(gspec.Config, dagchain, state, header, &gp, txExec, big.NewInt(0), vm.Config{}, nil)
		require.NoError(t, err)
		require.False(t, r.Failed, "freeze should be successfully")
		//余额也被全部冻结
		b := state.GetBalance(addr1)
		require.Zero(t, b.Int64())
	})
}

func TestStateKeys(t *testing.T) {
	//var (
	//	key1, _ = crypto.GenerateKey()
	//	addr1   = crypto.ForceParsePubKeyToAddress(key1.PublicKey) //0x71562b71999873DB5b286dF957af199Ec94617F7
	//)

	//testutil.SetupTestConfig()

	testutil.EnableLog(log.LvlError, "")
	file, err := ioutil.TempDir("", "test")
	require.NoError(t, err)
	defer os.Remove(file)

	db, err := ntsdb.NewLDBDatabase(file, 0, 0)
	require.NoError(t, err)
	gspec, err := LoadGenesisConfig()
	require.NoError(t, err)

	gspec.MustCommit(db)
	signer := types.NewSigner(gspec.Config.ChainId)

	dagchain, _ := NewDagChain(db, gspec.Config, witvote.NewFaker(), vm.Config{})
	defer dagchain.Stop()

	gp := GasPool(*math.MaxBig256)
	header := dagchain.GetHeaderByHash(dagchain.GetChainTailHash(params.SCAccount))
	header.ParentHash = header.Hash()
	header.SCHash = header.ParentHash
	header.Number += 1
	header.MC = params.SCAccount

	getKeys := func() (int, int) {
		var keys int
		var size int
		db.Iterator(nil, func(key, value []byte) bool {
			keys++
			size += len(key) + len(value)
			return true
		})
		return keys, size
	}

	keys, size := getKeys()

	root := header.StateRoot

	//testutil.EnableLog(log.LvlInfo, "")
	testutil.EnableLog(log.LvlError, "")
	units := 10000
	for i := 0; i < units; i++ {
		statedb, err := dagchain.StateAt(header.MC, root)
		require.NoError(t, err)

		//1: 8kb   46,   7
		//2: 14kb
		//5: 24kb  163
		//10: 38kb  292

		txs := 1
		//if i%2 == 0 {
		//	txs = 1
		//}
		for i := 0; i < txs; i++ {
			key1, _ := crypto.GenerateKey()
			addr1 := crypto.ForceParsePubKeyToAddress(key1.PublicKey)

			tx := types.NewTransaction(addr1,
				params.SCAccount, big.NewInt(0), 0, 0, 0, sc.CreateCallInputData(sc.FuncApplyWitnessID, addr1))

			_, err = types.SignTx(tx, signer, key1)

			txExec := &types.TransactionExec{
				Tx:     tx,
				TxHash: tx.Hash(),
				Action: types.ActionContractDeal,
			}
			log.Info("----->>>>start")
			_, err = ProcessTransaction(gspec.Config, dagchain, statedb, header, &gp, txExec, big.NewInt(1000000000), vm.Config{}, nil)

			//tx := types.NewTransaction(addr1,
			//	common.StringToAddress("to"), big.NewInt(100), 0, 0, 0, nil)
			//
			//_, err = types.SignTx(tx, signer, key1)
			//
			//header.MC = common.StringToAddress("to")
			//txExec := &types.TransactionExec{
			//	Tx:     tx,
			//	TxHash: tx.Hash(),
			//	Action: types.ActionTransferReceipt,
			//}
			//
			//_, err = ProcessTransaction(gspec.Config, dagchain, statedb, header, &gp, txExec, big.NewInt(1000000000), vm.Config{}, nil)
			//
			//require.NoError(t, err)
		}

		root, err = statedb.CommitTo(db)
		require.NoError(t, err)

		keys1, size1 := getKeys()
		t.Log(keys1, size1, "txs:", txs, "addKeys:", keys1-keys, "addSize:", common.StorageSize(size1-size))
		keys, size = keys1, size1
	}
	t.Log(keys, size)
	//
	//require.NoError(t, err)
	//_, keys2, size2 := db2.KeyInfo()
	//
	//oneSize := (size2 - size) / txs
	//t.Log(keys, size)
	//t.Log(keys2, size2)
	//t.Log("txs:", txs, "add Keys:", keys2-keys, "add Size:", common.StorageSize(size2-size), "oneSize:", common.StorageSize(oneSize))
	//t.Log("100万", common.StorageSize(oneSize*1000000), "keys:", ((size2-size)/txs)*1000000)

}

func TestPowTx(t *testing.T) {
	var (
		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		addr1   = crypto.ForceParsePubKeyToAddress(key1.PublicKey) //0x71562b71999873DB5b286dF957af199Ec94617F7
	)

	testutil.SetupTestConfig()

	db, _ := ntsdb.NewMemDatabase()
	gspec, err := LoadGenesisConfig()
	require.NoError(t, err)
	gspec.Alloc[addr1] = GenesisAccount{Balance: math.MaxBig256}

	gspec.MustCommit(db)
	signer := types.NewSigner(gspec.Config.ChainId)

	dagchain, _ := NewDagChain(db, gspec.Config, witvote.NewFaker(), vm.Config{})
	defer dagchain.Stop()

	inputData := sc.CreateCallInputData(sc.FuncReplaceWitnessID, addr1)
	tx := types.NewTransaction(addr1,
		params.SCAccount, big.NewInt(0), 0, 0, 0, inputData)
	seedCh := ethash.ProofWork(tx, 1, big.NewInt(1), make(chan struct{}))
	seed, ok := <-seedCh
	require.True(t, ok)
	tx.SetSeed(seed)
	_, err = types.SignTx(tx, signer, key1)

	gp := GasPool(*math.MaxBig256)
	header := dagchain.GetHeaderByHash(dagchain.GetChainTailHash(addr1))
	header.ParentHash = header.Hash()
	header.SCHash = dagchain.GetChainTailHead(params.SCAccount).Hash()
	header.Number += 1
	header.MC = addr1

	state, _ := dagchain.GetChainTailState(addr1)
	txExec := &types.TransactionExec{
		Tx:     tx,
		TxHash: tx.Hash(),
		Action: types.ActionContractDeal,
	}
	_, err = ProcessTransaction(gspec.Config, dagchain, state, header, &gp, txExec, big.NewInt(0), vm.Config{}, nil)

	require.Error(t, err)
}

//
//// 修复执行系统合约失败时，扣除全部Gas问题
//func TestFixedBug_ExecInserSysContract(t *testing.T) {
//	var (
//		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
//		addr1   = crypto.ForceParsePubKeyToAddress(key1.PublicKey) //0x71562b71999873DB5b286dF957af199Ec94617F7
//	)
//
//	testutil.SetupTestConfig()
//
//	db, _ := ntsdb.NewMemDatabase()
//	gspec, err := LoadGenesisConfig()
//	require.NoError(t, err)
//	gspec.Alloc[addr1] = GenesisAccount{Balance: math.MaxBig256}
//
//	gspec.MustCommit(db)
//	signer := types.NewSigner(gspec.Config.ChainId)
//
//	dagchain, _ := NewDagChain(db, gspec.Config, witvote.NewFaker(), vm.Config{})
//	defer dagchain.Stop()
//
//	data := common.Hex2Bytes(sc.FuncIDApplyWitnessWay2) // error data
//
//	tx := types.NewTransaction(addr1,
//		params.SCAccount, big.NewInt(9999), 0x900000, 1, 0, data)
//	types.SignTx(tx, signer, key1)
//
//	gp := GasPool(*math.MaxBig256)
//	header := dagchain.GetHeaderByHash(dagchain.GetChainTailHash(addr1))
//	header.ParentHash = header.Hash()
//	header.SCHash = dagchain.GetChainTailHead(params.SCAccount).Hash()
//	header.Number += 1
//	header.MC = addr1
//
//	state, _ := dagchain.GetChainTailState(addr1)
//	txExec := &types.TransactionExec{
//		Tx:     tx,
//		TxHash: tx.Hash(),
//		Action: types.ActionContractDeal,
//	}
//	r, err := ProcessTransaction(gspec.Config, dagchain, state, header, &gp, txExec, big.NewInt(0), vm.Config{EnableSimulation: false}, nil)
//
//	require.NoError(t, err)
//	require.True(t, r.Failed)
//	require.NotEqual(t, tx.Gas(), r.GasUsed)
//	require.NotEqual(t, uint64(0), r.GasUsed)
//}

func TestFixedBug_NewProcessorDeal(t *testing.T) {
	var (
		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		addr1   = crypto.ForceParsePubKeyToAddress(key1.PublicKey) //0x71562b71999873DB5b286dF957af199Ec94617F7
	)

	testutil.SetupTestConfig()

	db, _ := ntsdb.NewMemDatabase()
	gspec, err := LoadGenesisConfig()
	require.NoError(t, err)
	gspec.Alloc[addr1] = GenesisAccount{Balance: math.MaxBig256}

	gspec.MustCommit(db)
	signer := types.NewSigner(gspec.Config.ChainId)

	dagchain, _ := NewDagChain(db, gspec.Config, witvote.NewFaker(), vm.Config{})
	defer dagchain.Stop()

	data := common.Hex2Bytes(sc.FuncIdSetUserWitness) // error data

	tx := types.NewTransaction(addr1, params.SCAccount, big.NewInt(0), 0x900000, 1, 100, data)
	types.SignTx(tx, signer, key1)

	gp := GasPool(*math.MaxBig256)
	header := dagchain.GetHeaderByHash(dagchain.GetChainTailHash(params.SCAccount))
	header.ParentHash = header.Hash()
	header.SCHash = dagchain.GetChainTailHead(params.SCAccount).Hash()
	header.Number += 1
	header.MC = addr1

	state, _ := dagchain.GetChainTailState(params.SCAccount)
	txExec := &types.TransactionExec{
		Tx:     tx,
		TxHash: tx.Hash(),
		Action: types.ActionContractDeal,
	}
	r, err := ProcessTransaction(gspec.Config, dagchain, state, header, &gp, txExec, big.NewInt(0), vm.Config{}, nil)

	require.NoError(t, err)
	require.True(t, r.Failed)
	require.NotEqual(t, tx.Gas(), r.GasUsed)
	require.NotEqual(t, uint64(0), r.GasUsed)

}

func TestFixedManyDeployContractOnOneUnit(t *testing.T) {
	txData := common.FromHex("0x09e46464608060405234801561001057600080fd5b506040516109b03803806109b08339810180604052602081101561003357600080fd5b81019080805164010000000081111561004b57600080fd5b8281019050602081018481111561006157600080fd5b815185600182028301116401000000008211171561007e57600080fd5b505092919050505060008151116100fd576040517f08c379a00000000000000000000000000000000000000000000000000000000081526004018080602001828103825260128152602001807f6d7573742073657420796f7572206e616d65000000000000000000000000000081525060200191505060405180910390fd5b336000806101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff160217905550806001600001908051906020019061015692919061015d565b5050610202565b828054600181600116156101000203166002900490600052602060002090601f016020900481019282601f1061019e57805160ff19168380011785556101cc565b828001600101855582156101cc579182015b828111156101cb5782518255916020019190600101906101b0565b5b5090506101d991906101dd565b5090565b6101ff91905b808211156101fb5760008160009055506001016101e3565b5090565b90565b61079f806102116000396000f3fe6080604052600436106100705760003560e01c806386d125a51161004e57806386d125a5146100d95780638da5cb5b146101bc578063a176640f14610213578063cf1724031461032b57610070565b8063228cb733146100755780633129a89d1461007f5780634a8c725f146100aa575b600080fd5b61007d610390565b005b34801561008b57600080fd5b50610094610495565b6040518082815260200191505060405180910390f35b3480156100b657600080fd5b506100bf61049b565b604051808215151515815260200191505060405180910390f35b3480156100e557600080fd5b506101ba600480360360608110156100fc57600080fd5b81019080803560ff1690602001909291908035906020019064010000000081111561012657600080fd5b82018360208201111561013857600080fd5b8035906020019184600183028401116401000000008311171561015a57600080fd5b91908080601f016020809104026020016040519081016040528093929190818152602001838380828437600081840152601f19601f820116905080830192505050505050509192919290803561ffff1690602001909291905050506104ba565b005b3480156101c857600080fd5b506101d1610516565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b34801561021f57600080fd5b5061022861053b565b60405180806020018560ff1660ff168152602001806020018461ffff1661ffff168152602001838103835287818151815260200191508051906020019080838360005b8381101561028657808201518184015260208101905061026b565b50505050905090810190601f1680156102b35780820380516001836020036101000a031916815260200191505b50838103825285818151815260200191508051906020019080838360005b838110156102ec5780820151818401526020810190506102d1565b50505050905090810190601f1680156103195780820380516001836020036101000a031916815260200191505b50965050505050505060405180910390f35b34801561033757600080fd5b5061037a6004803603602081101561034e57600080fd5b81019080803573ffffffffffffffffffffffffffffffffffffffff1690602001909291905050506106a4565b6040518082815260200191505060405180910390f35b60003411610406576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040180806020018281038252600c8152602001807f72657175697265206d6f7265000000000000000000000000000000000000000081525060200191505060405180910390fd5b34600660003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020019081526020016000205401600660003373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001908152602001600020819055506104936106bc565b565b60055481565b600060126001800160009054906101000a900460ff1660ff1611905090565b826001800160006101000a81548160ff021916908360ff16021790555081600160020190805190602001906104f09291906106ce565b5080600160030160006101000a81548161ffff021916908361ffff160217905550505050565b6000809054906101000a900473ffffffffffffffffffffffffffffffffffffffff1681565b6001806000018054600181600116156101000203166002900480601f0160208091040260200160405190810160405280929190818152602001828054600181600116156101000203166002900480156105d55780601f106105aa576101008083540402835291602001916105d5565b820191906000526020600020905b8154815290600101906020018083116105b857829003601f168201915b5050505050908060010160009054906101000a900460ff1690806002018054600181600116156101000203166002900480601f0160208091040260200160405190810160405280929190818152602001828054600181600116156101000203166002900480156106865780601f1061065b57610100808354040283529160200191610686565b820191906000526020600020905b81548152906001019060200180831161066957829003601f168201915b5050505050908060030160009054906101000a900461ffff16905084565b60066020528060005260406000206000915090505481565b34600560008282540192505081905550565b828054600181600116156101000203166002900490600052602060002090601f016020900481019282601f1061070f57805160ff191683800117855561073d565b8280016001018555821561073d579182015b8281111561073c578251825591602001919060010190610721565b5b50905061074a919061074e565b5090565b61077091905b8082111561076c576000816000905550600101610754565b5090565b9056fea165627a7a723058207cd144a9e3653853e6d327d83761c9dfc416325a3e37f461021352d05feeeb7a002900000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000000000000000000008746573744e616d65000000000000000000000000000000000000000000000000")

	var (
		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		addr1   = crypto.ForceParsePubKeyToAddress(key1.PublicKey) //0x71562b71999873DB5b286dF957af199Ec94617F7
	)

	//testutil.SetupTestConfig()
	log.Root().SetHandler(log.FuncHandler(func(r *log.Record) error {
		return nil
	}))
	db, _ := ntsdb.NewMemDatabase()
	gspec, err := LoadGenesisConfig()
	require.NoError(t, err)
	gspec.Alloc[addr1] = GenesisAccount{Balance: math.MaxBig256}

	gspec.MustCommit(db)
	signer := types.NewSigner(gspec.Config.ChainId)

	dagchain, _ := NewDagChain(db, gspec.Config, witvote.NewFaker(), vm.Config{})
	defer dagchain.Stop()

	maxGas := uint64(1239000)
	gp := GasPool(*new(big.Int).SetUint64(maxGas))
	header := dagchain.GetHeaderByHash(dagchain.GetChainTailHash(params.SCAccount))
	header.ParentHash = header.Hash()
	header.SCHash = dagchain.GetChainTailHead(params.SCAccount).Hash()
	header.Number += 1
	header.MC = params.SCAccount

	state, _ := dagchain.GetChainTailState(params.SCAccount)

	var used uint64
	for i := 0; i < 50; i++ {
		tx := types.NewTransaction(addr1,
			params.SCAccount, big.NewInt(0), 0xe77d7, 1, types.DefTxMaxTimeout, txData)
		types.SignTx(tx, signer, key1)
		txExec := &types.TransactionExec{
			Tx:     tx,
			TxHash: tx.Hash(),
			Action: types.ActionContractDeal,
		}

		r, err := ProcessTransaction(gspec.Config, dagchain, state, header, &gp, txExec, big.NewInt(0), vm.Config{EnableSimulation: false}, nil)
		if err != nil {
			t.Logf("%d:%s", i, err)
			if err == ErrGasLimitReached {
				break
			}
		} else {
			used += r.GasUsed
			if r.Failed {
				t.Logf("%d: failed, used:%d,remainng:%d", i, r.GasUsed, gp.Gas())
			}
		}
	}
	require.True(t, maxGas >= used, "used should be less max:  used=%d,max=%d", used, maxGas)
}

func TestTransferFlow(t *testing.T) {
	var (
		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		addr1   = crypto.ForceParsePubKeyToAddress(key1.PublicKey) //0x71562b71999873DB5b286dF957af199Ec94617F7
	)

	testutil.SetupTestConfig()
	db, _ := ntsdb.NewMemDatabase()
	gspec, err := LoadGenesisConfig()
	require.NoError(t, err)

	gspec.Alloc[addr1] = GenesisAccount{Balance: big.NewInt(10000000000)}

	gspec.MustCommit(db)
	signer := types.NewSigner(gspec.Config.ChainId)

	dagchain, _ := NewDagChain(db, gspec.Config, witvote.NewFaker(), vm.Config{})
	defer dagchain.Stop()
	gp := GasPool(*math.MaxBig256)

	// 转账
	payment := func(t *testing.T, tx *types.Transaction, shouldBeFailed bool) (*types.Receipt, *types.Header, *state.StateDB, *types.TransactionExec) {
		types.SignTx(tx, signer, key1)

		from, _ := tx.Sender()
		state, _ := dagchain.GetChainTailState(from)
		balance := state.GetBalance(from).Uint64()

		header := dagchain.GetHeaderByHash(dagchain.GetChainTailHash(from))
		header.ParentHash = header.Hash()
		header.SCHash = dagchain.GetChainTailHead(params.SCAccount).Hash()
		header.Number += 1
		header.MC = from

		txExec := &types.TransactionExec{
			Tx:     tx,
			TxHash: tx.Hash(),
			Action: types.ActionTransferPayment,
		}
		r, err := ProcessTransaction(gspec.Config, dagchain, state, header, &gp, txExec, big.NewInt(0), vm.Config{}, nil)
		if shouldBeFailed {
			require.Error(t, err) //如果失败则直接error
		} else {
			require.False(t, r.Failed) //不应该存在失败情况

			if !r.Failed {
				//交易方花费 = 已用Gas * 单价 + 转账额
				fee := tx.GasPrice() * r.GasUsed

				if tx.To() == from {
					//只需要支付手续费
					assert.Equal(t, balance-fee, state.GetBalance(from).Uint64(), "should be only pay fee")

					assert.Empty(t, r.Coinflows, "should be empty if payment to yourself")
				} else {
					//需要支付手续与转账额
					assert.Equal(t, balance-fee-tx.Amount().Uint64(), state.GetBalance(from).Uint64())

					assert.Len(t, r.Coinflows, 1, "should contain received")
					checkInclude(t, r.Coinflows, tx.To(), tx.Amount())
				}
			}
		}
		return r, header, state, txExec
	}

	receipt := func(t *testing.T, tx *types.Transaction, shouldBeFailed bool) (*types.Receipt, *types.Header, *state.StateDB, *types.TransactionExec) {
		r, header1, statedb, txexec1 := payment(t, tx, false)

		unit := saveResultToDB(t, dagchain, statedb, header1, txexec1, r)

		to := tx.To()
		state, _ := dagchain.GetChainTailState(to)
		balance := state.GetBalance(to).Uint64()
		header := dagchain.GetHeaderByHash(dagchain.GetChainTailHash(to))
		header.ParentHash = header.Hash()
		header.SCHash = dagchain.GetChainTailHead(params.SCAccount).Hash()
		header.Number += 1
		header.MC = to

		txExec := &types.TransactionExec{
			TxHash:  tx.Hash(),
			PreStep: unit.ID(),
			Action:  types.ActionTransferReceipt,
		}
		r, err := ProcessTransaction(gspec.Config, dagchain, state, header, &gp, txExec, big.NewInt(0), vm.Config{}, nil)
		require.NoError(t, err)
		require.Equal(t, shouldBeFailed, r.Failed)

		if !r.Failed {
			//收款方，不涉及gas消费
			assert.Zero(t, r.GasUsed)
			checkRemainingGas(t, dagchain, r)

			assert.Equal(t, balance+tx.Amount().Uint64(), state.GetBalance(to).Uint64())

			assert.Len(t, r.Coinflows, 1)
			checkInclude(t, r.Coinflows, tx.To(), tx.Amount())
		}
		return r, header, state, txExec
	}

	t.Run("payment", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			//转账
			to := common.StringToAddress("addr2")
			tx := types.NewTransaction(addr1, to, big.NewInt(9999), 0x900000, 1, 0, nil)
			payment(t, tx, false)
		})
		t.Run("failed", func(t *testing.T) {
			//转账
			to := common.StringToAddress("addr3")
			// gas 不足
			tx := types.NewTransaction(addr1, to, big.NewInt(2000), 20000, 1, 0, nil)
			payment(t, tx, true)
		})
		t.Run("to self", func(t *testing.T) {
			//转账
			to := addr1
			tx := types.NewTransaction(addr1, to, big.NewInt(2000), 0x900000, 1, 0, nil)
			payment(t, tx, false)
		})
	})
	t.Run("receipt", func(t *testing.T) {
		t.Run("default", func(t *testing.T) {
			//转账
			to := common.StringToAddress("addr3")
			tx := types.NewTransaction(addr1, to, big.NewInt(9999), 0x900000, 1, 0, nil)
			receipt(t, tx, false)
		})
		t.Run("include data", func(t *testing.T) {
			//转账
			to := common.StringToAddress("addr4")
			tx := types.NewTransaction(addr1, to, big.NewInt(9999), 0x900000, 1, 0, []byte("abcdefg"))
			receipt(t, tx, false)
		})
	})
}

func TestContractFlow(t *testing.T) {
	var (
		key1, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		addr1   = crypto.ForceParsePubKeyToAddress(key1.PublicKey) //0x71562b71999873DB5b286dF957af199Ec94617F7

		//tSmartContractCode="0x6060604052336000806101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff1602179055506106bd806100536000396000f30060606040526004361061008e576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff1680630da229be146100935780632f1aa84b146100bc57806377ca18a61461011457806387db03b71461012c5780638f84aa091461014f578063d0679d34146101a4578063d294cb0f146101db578063fa3bd6c514610228575b600080fd5b341561009e57600080fd5b6100a661024b565b6040518082815260200191505060405180910390f35b610112600480803590602001908201803590602001908080602002602001604051908101604052809392919081815260200183836020028082843782019150505050505091908035906020019091905050610255565b005b61012a600480803590602001909190505061038a565b005b341561013757600080fd5b61014d600480803590602001909190505061048d565b005b341561015a57600080fd5b6101626104f1565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b6101d9600480803573ffffffffffffffffffffffffffffffffffffffff1690602001909190803590602001909190505061051a565b005b34156101e657600080fd5b610212600480803573ffffffffffffffffffffffffffffffffffffffff1690602001909190505061060c565b6040518082815260200191505060405180910390f35b341561023357600080fd5b610249600480803590602001909190505061062d565b005b6000600154905090565b600080600084518402925082341015151561026f57600080fd5b600091505b845182101561038357848281518110151561028b57fe5b9060200190602002015190508073ffffffffffffffffffffffffffffffffffffffff166108fc859081150290604051600060405180830381858888f1935050505015156102d757600080fd5b7f091151c6f9d8fda9318c8ce50640a16ca1379e117d817054f49a68dd3f8518f0338286604051808473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020018373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001828152602001935050505060405180910390a18180600101925050610274565b5050505050565b3373ffffffffffffffffffffffffffffffffffffffff166108fc829081150290604051600060405180830381858888f1935050505015156103ca57600080fd5b7f091151c6f9d8fda9318c8ce50640a16ca1379e117d817054f49a68dd3f8518f06000809054906101000a900473ffffffffffffffffffffffffffffffffffffffff163383604051808473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020018373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001828152602001935050505060405180910390a150565b60006001549050816001600082825401925050819055507f91acf26ac019f933454b8d5ebaceece943fe706a4c5f64064f865edde02e5766816001548460405180848152602001838152602001828152602001935050505060405180910390a15050565b60008060009054906101000a900473ffffffffffffffffffffffffffffffffffffffff16905090565b80341015151561052957600080fd5b8173ffffffffffffffffffffffffffffffffffffffff166108fc829081150290604051600060405180830381858888f19350505050151561056957600080fd5b7f091151c6f9d8fda9318c8ce50640a16ca1379e117d817054f49a68dd3f8518f0338383604051808473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020018373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001828152602001935050505060405180910390a15050565b60008173ffffffffffffffffffffffffffffffffffffffff16319050919050565b60006001549050816001600082825403925050819055507f91acf26ac019f933454b8d5ebaceece943fe706a4c5f64064f865edde02e5766816001548460405180848152602001838152602001828152602001935050505060405180910390a150505600a165627a7a723058201b6b051798b445bcce8abc65cb8543f35c5f372a3c397a772d645d1a188af4e90029"
	)

	testutil.SetupTestConfig()
	db, _ := ntsdb.NewMemDatabase()
	gspec, err := LoadGenesisConfig()
	require.NoError(t, err)
	gspec.Alloc[addr1] = GenesisAccount{Balance: new(big.Int).Mul(math.MaxBig256, big.NewInt(2))}

	gspec.MustCommit(db)
	signer := types.NewSigner(gspec.Config.ChainId)

	dagchain, _ := NewDagChain(db, gspec.Config, witvote.NewFaker(), vm.Config{})
	defer dagchain.Stop()
	dagchain.testDisableVote = true

	createContractData := sc.CreateCallInputData(sc.FuncCreateContract, common.FromHex(tSmartContractCode))

	gp := GasPool(*math.MaxBig256)

	// 执行冻结
	freeze := func(tx *types.Transaction, shouldBeFailed bool) (*types.Receipt, *types.Header, *state.StateDB, *types.TransactionExec) {
		types.SignTx(tx, signer, key1)

		// 冻结
		state, _ := dagchain.GetChainTailState(addr1)
		balance := state.GetBalance(addr1)

		header := dagchain.GetHeaderByHash(dagchain.GetChainTailHash(addr1))
		header.ParentHash = header.Hash()
		header.SCHash = dagchain.GetChainTailHead(params.SCAccount).Hash()
		header.Number += 1
		header.MC = addr1

		txExec := &types.TransactionExec{
			Tx:     tx,
			TxHash: tx.Hash(),
			Action: types.ActionContractFreeze,
		}
		r, err := ProcessTransaction(gspec.Config, dagchain, state, header, &gp, txExec, big.NewInt(0), vm.Config{}, nil)

		if shouldBeFailed {
			//
			require.Error(t, err)
		} else {
			require.NotNil(t, r)
			assert.False(t, r.Failed, "freeze should be successfully")

			//检查是否有全款冻结
			freeze := common.U64ToBig(tx.GasPrice() * tx.Gas())
			freeze.Add(freeze, tx.Amount())
			assert.Equal(t, balance.Sub(balance, freeze), state.GetBalance(addr1), "need freeze all gas and value")

			//检查剩余Gas
			assert.Equal(t, new(big.Int).Sub(common.U64ToBig(tx.Gas()), new(big.Int).SetUint64(r.GasUsed)), new(big.Int).SetUint64(r.GasRemaining),
				"gas remaining should be equal tx gas(%d) - used gas(%d)", tx.Gas(), r.GasUsed)

			require.Equal(t, txExec.PreStep, r.PreStep)
		}
		state.IntermediateRoot(common.Address{})

		return r, header, state, txExec
	}

	// 在系统链执行已获得合约地址与见证人
	createContractAddr := func(t *testing.T, tx *types.Transaction, shouldBeFailed bool) (*types.Receipt, *types.Header, *state.StateDB, *types.TransactionExec, common.Address) {
		//先冻结
		r1, header, state, txExec := freeze(tx, false)
		unit := saveResultToDB(t, dagchain, state, header, txExec, r1)

		// 部署之分配地址
		sysheader := dagchain.GetHeaderByHash(dagchain.GetChainTailHash(params.SCAccount))
		sysheader.ParentHash = sysheader.Hash()
		sysheader.SCHash = sysheader.ParentHash
		sysheader.Number += 1
		sysheader.MC = params.SCAccount //关键位置
		txExec2 := &types.TransactionExec{
			TxHash:  tx.Hash(),
			PreStep: unit.ID(),
			Action:  types.ActionContractDeal,
		}
		state2, err := dagchain.StateAt(sysheader.MC, sysheader.StateRoot)
		require.NoError(t, err)

		sysChainBalance := state2.GetBalance(params.SCAccount)

		total := big.NewInt(0)
		r, err := ProcessTransaction(gspec.Config, dagchain, state2, sysheader, &gp, txExec2, total, vm.Config{}, nil)
		require.NoError(t, err)
		//无论成功与否，系统链上合约不发生变化
		require.Equal(t, sysChainBalance.Uint64(), state2.GetBalance(params.SCAccount).Uint64(), "should be not change system chain balance")
		//无论成功与否都不能添加其他非本合约信息
		//objs := state2.GetObjects()
		//require.Len(t, common.AddressList(state2.GetObjects()), 1)
		//require.Contains(t, objs, sysheader.MC, "should contain system chain info")
		require.Equal(t, txExec2.PreStep, r.PreStep)
		require.Equal(t, total.Uint64(), r.GasUsed)

		checkRemainingGas(t, dagchain, r)

		if !shouldBeFailed {
			require.False(t, r.Failed, "should be process successful")
			require.NotEmpty(t, r.Output, "output should not be empty")

			// 能提取新合约地址
			info, err := sc.GetContractOutput(sc.FuncCreateContract, r.Output)
			require.NoError(t, err, "should be get new contract address from output")

			//检查剩余Gas
			assert.Equal(t, r1.GasRemaining-r.GasUsed, r.GasRemaining,
				"gas remaining should be equal tx gas(%d) - used gas(%d)", tx.Gas(), r.GasUsed)

			contractInfo := info.(sc.NewCtWit)
			//t.Logf("got new contract address:%v", contractInfo.Contract.Hex())
			assert.NotEmpty(t, contractInfo.Contract)
			assert.True(t, contractInfo.Contract.IsContract(), "should be a contract type address")
			assert.Len(t, r.Coinflows, 0, "coinflows should be empty")

			return r, sysheader, state, txExec2, contractInfo.Contract
		} else {
			require.True(t, r.Failed, "should be process failed")
			checkFailedReceipt(t, r)
		}

		return r, sysheader, state, txExec2, common.Address{}
	}

	initContract := func(t *testing.T, tx *types.Transaction, shouldBeFailed bool) (*types.Receipt, *types.Header, *state.StateDB, *types.TransactionExec) {
		r1, header, state, txexec, contractAddr := createContractAddr(t, tx, false)

		//必须存储到DB
		unit := saveResultToDB(t, dagchain, state, header, txexec, r1)

		// 部署合约
		txExec2 := &types.TransactionExec{
			TxHash:  tx.Hash(),
			PreStep: unit.ID(),
			Action:  types.ActionContractDeal,
		}
		cheader := dagchain.GetHeaderByHash(dagchain.GetChainTailHash(contractAddr))
		cheader.ParentHash = header.Hash()
		cheader.Number += 1
		cheader.MC = contractAddr
		cheader.SCHash = dagchain.GetChainTailHash(params.SCAccount)

		state2, err := dagchain.StateAt(cheader.MC, cheader.StateRoot)
		require.NoError(t, err)
		r, err := ProcessTransaction(gspec.Config, dagchain, state2, cheader, &gp, txExec2, big.NewInt(0), vm.Config{}, nil)
		require.NoError(t, err)
		require.Equal(t, shouldBeFailed, r.Failed)
		require.Equal(t, txExec2.PreStep, r.PreStep)

		checkRemainingGas(t, dagchain, r)
		if !shouldBeFailed {
			//检查剩余Gas
			assert.Len(t, r.Coinflows, 1, "should be only include contract value")
			//应包括给合约的转账
			checkInclude(t, r.Coinflows, contractAddr, tx.Amount())
			//不涉及转出

			// 逻辑已变，无
			//checkInclude(t, r.Coinflows, addr1, new(big.Int).SetUint64(r.GasRemaining*tx.GasPrice().Uint64()))
		} else {
			checkFailedReceipt(t, r)

			//检查剩余Gas
			assert.Equal(t, r1.GasRemaining-r.GasUsed, r.GasRemaining,
				"gas remaining should be equal tx gas(%d) - used gas(%d)", r1.GasRemaining, r.GasUsed)

			//data,_:= json.MarshalIndent(r,"","\t")
			//t.Log(string(data))

		}
		return r, cheader, state2, txExec2
	}

	refundContract := func(t *testing.T, tx *types.Transaction, shouldBeFailed bool) (*types.Receipt, *types.Header, *state.StateDB, *types.TransactionExec) {
		types.SignTx(tx, signer, key1)

		from, _ := tx.Sender()
		balance := dagchain.GetBalance(from)

		r1, cHeader, cstate, txexec := initContract(t, tx, false)

		unit := saveResultToDB(t, dagchain, cstate, cHeader, txexec, r1)
		t.Logf("initDone,%s.Balance=%d , %d", from.Text(), cstate.GetBalance(from), dagchain.GetBalance(from))

		// 部署合约
		txExec2 := &types.TransactionExec{
			TxHash:  tx.Hash(),
			PreStep: unit.ID(),
			Action:  types.ActionContractRefund,
		}

		header := dagchain.GetHeaderByHash(dagchain.GetChainTailHash(from))
		header.ParentHash = header.Hash()
		header.Number += 1
		header.MC = from
		header.SCHash = dagchain.GetChainTailHash(params.SCAccount)

		state2, err := dagchain.StateAt(header.MC, header.StateRoot)
		require.NoError(t, err)
		toal := big.NewInt(0)
		r, err := ProcessTransaction(gspec.Config, dagchain, state2, header, &gp, txExec2, toal, vm.Config{}, nil)
		require.NoError(t, err)
		assert.Equal(t, shouldBeFailed, r.Failed)
		require.Equal(t, toal.Uint64(), r.GasUsed)
		require.Equal(t, txExec2.PreStep, r.PreStep)

		if !shouldBeFailed {
			assert.Zero(t, r.GasUsed, "used gas should be zero if action is ActionContractRefund") //返回余额，不消耗Gas
			//应该包含剩余返回的流水
			checkInclude(t, r.Coinflows, from, new(big.Int).SetUint64(r.GasRemaining*tx.GasPrice()))

			//检查剩余Gas
			checkRemainingGas(t, dagchain, r)
			//返回给交易方
			//此时交易方余额 = 交易前余额 - value - fee
			usedGas := tx.Gas() - r.GasRemaining

			afterB := state2.GetBalance(from)

			diff := new(big.Int).Sub(balance, afterB) // 开销= 期初余额-余额= 该交易开销
			// 该交易实际开销= 转账金额+手续费
			changed := new(big.Int).Add(tx.Amount(), mul(tx.GasPrice(), usedGas))

			require.Equal(t, changed, diff,
				"amount=%d, initBalance=%d,usedGas=%d * %d, afterBalance=%d",
				tx.Amount(), balance, usedGas, tx.GasPrice(), state2.GetBalance(from))

		} else {
			checkFailedReceipt(t, r)

			//检查剩余Gas
			assert.Equal(t, r1.GasRemaining-r.GasUsed, r.GasRemaining,
				"gas remaining should be equal tx gas(%d) - used gas(%d)", r1.GasRemaining, r.GasUsed)

			//data,_:= json.MarshalIndent(r,"","\t")
			//t.Log(string(data))

		}
		return r, header, state2, txExec2
	}

	callContract := func(t *testing.T, tx *types.Transaction, shouldBeFailed bool) (*types.Receipt, *types.Header, *state.StateDB, *types.TransactionExec) {

		r, fheader, fstate, ftxExec := freeze(tx, false)
		funit := saveResultToDB(t, dagchain, fstate, fheader, ftxExec, r)

		contractAddr := tx.To()

		header := dagchain.GetHeaderByHash(dagchain.GetChainTailHash(contractAddr))

		header.ParentHash = header.Hash()
		header.SCHash = dagchain.GetChainTailHead(params.SCAccount).Hash()
		header.Number += 1
		header.MC = contractAddr

		statedb, _ := dagchain.StateAt(header.MC, header.StateRoot)

		txExec := &types.TransactionExec{
			TxHash:  tx.Hash(),
			PreStep: funit.ID(),
			Action:  types.ActionContractDeal,
		}
		r, err := ProcessTransaction(gspec.Config, dagchain, statedb, header, &gp, txExec, big.NewInt(0), vm.Config{}, nil)
		require.NoError(t, err)
		require.Equal(t, txExec.PreStep, r.PreStep)
		//data, _ := json.MarshalIndent(r, "", "\t")
		//t.Log(string(data))

		return r, header, statedb, txExec
	}

	// 1. 冻结
	t.Run("freeze", func(t *testing.T) {
		t.Run("successful", func(t *testing.T) {
			tx := types.NewTransaction(addr1, params.SCAccount, big.NewInt(9999), 0x900000, 1, 0, createContractData)
			freeze(tx, false)
		})
		t.Run("failed", func(t *testing.T) {
			// 模拟 Gas 不足场景
			tx := types.NewTransaction(addr1, params.SCAccount, big.NewInt(9999), params.TxGas, 1, 0, createContractData)
			freeze(tx, true)
		})
	})

	//2. 获得地址
	t.Run("create addr", func(t *testing.T) {

		t.Run("successful", func(t *testing.T) {
			var sum uint64 = 0x900000
			tx := types.NewTransaction(addr1, params.SCAccount, big.NewInt(9999), sum, 1, 0, createContractData)
			r, _, _, _, _ := createContractAddr(t, tx, false)

			allUsed := sum - r.GasRemaining
			//如果减少一个Gas，将执行失败
			t.Run("-1 gas", func(t *testing.T) {
				//Gas不足
				tx := types.NewTransaction(addr1, params.SCAccount, big.NewInt(9999), allUsed-1, 1, 0, createContractData)
				createContractAddr(t, tx, true)
			})
		})
	})
	// 3. 在合约链上部署
	t.Run("init contact", func(t *testing.T) {
		var sum uint64 = 0x900000

		tx := types.NewTransaction(addr1, params.SCAccount, big.NewInt(9999), sum, 1, 0, createContractData)
		r, cHeader, cstate, txexec := initContract(t, tx, false)
		saveResultToDB(t, dagchain, cstate, cHeader, txexec, r)

		allUsed := sum - r.GasRemaining
		//如果减少一个Gas，将执行失败
		t.Run("-1 gas", func(t *testing.T) {
			//Gas不足
			tx := types.NewTransaction(addr1, params.SCAccount, big.NewInt(9999), allUsed-1, 1, 0, createContractData)
			r, _, _, _, _ := createContractAddr(t, tx, true)
			require.NotZero(t, r.GasUsed) //也将消费Gas
		})
		//Gas不足使得预处理失败
		t.Run("pre check faild", func(t *testing.T) {
			var infoList []TxStatusInfo
			err := dagchain.dag.GetTransactionStatus(tx.Hash(), func(info TxStatusInfo) (b bool, e error) {
				infoList = append(infoList, info)
				return true, nil
			})
			require.Len(t, infoList, 3, "should be have 3 actions(freeze,deal,deal)")
			txexec, err := dagchain.dag.GetTxReceiptAtUnit(infoList[1].UnitHash, r.TxHash)
			require.Nil(t, err)
			require.NotNil(t, txexec)

			action1and3Gas := allUsed - txexec.GasUsed //步骤1和3消费Gas

			t.Run("action1_3", func(t *testing.T) {
				//预处理在第二阶段，如果Gas不足将导致第二阶段执行失败，但会收取Gas。
				// 此结果是Gas耗尽，无剩余
				tx2 := types.NewTransaction(addr1, params.SCAccount, big.NewInt(1234), action1and3Gas-1, 1, 0, createContractData)
				createContractAddr(t, tx2, true)
			})
			t.Run("action1_3_2", func(t *testing.T) {
				// 测试预估足够，但是继续执行步骤二时Gas不足，将导致步骤2失败，收取全部
				// 此结果是Gas耗尽，无剩余
				tx3 := types.NewTransaction(addr1, params.SCAccount, big.NewInt(5678), action1and3Gas+1, 1, 0, createContractData)
				createContractAddr(t, tx3, true)
			})
		})

	})

	//4. 返还多余冻结
	t.Run("refound after create contact", func(t *testing.T) {
		tx := types.NewTransaction(addr1, params.SCAccount, big.NewInt(9999), 0x900000, 1, 0, createContractData)
		refundContract(t, tx, false)
	})

	// 4. 执行合约
	t.Run("call contact", func(t *testing.T) {
		//部署合约
		tx := types.NewTransaction(addr1, params.SCAccount, big.NewInt(9999), 0x900000, 1, 0, createContractData)
		r, cHeader, cstate, txexec := initContract(t, tx, false)

		saveResultToDB(t, dagchain, cstate, cHeader, txexec, r)

		abiInfo, err := abi.JSON(bytes.NewReader(testContractABI()))
		require.NoError(t, err)
		contractAddr := cHeader.MC

		t.Run("func:sends", func(t *testing.T) {
			transferValue := big.NewInt(33)
			recipeAccounts := []common.Address{addr1,
				common.ForceDecodeAddress("nts1x67wl68e9rhyxfkr4nu6jqmpvd2fufju5lk76c"),
				common.ForceDecodeAddress("nts154ffjespv0wdeela09398qc90entwf6z858jyw")}
			data, err := abiInfo.Pack("sends", recipeAccounts, transferValue)
			require.NoError(t, err, "should be create input data")

			fmt.Println(common.Bytes2Hex(data))

			t.Run("successfuly", func(t *testing.T) {

				callTx := types.NewTransaction(addr1, contractAddr,
					big.NewInt(0).SetInt64(transferValue.Int64()*int64(len(recipeAccounts))+2), 120000, 1, 0, data)

				r, _, _, _ := callContract(t, callTx, false)

				require.False(t, r.Failed)
				assert.Empty(t, r.Output)
				//检查剩余Gas
				checkRemainingGas(t, dagchain, r)

				//检查coinflow,应该包含转账的接收方
				//应该有三个收款方 + 合约收款
				require.Len(t, r.Coinflows, len(recipeAccounts)+1, "from %x,tos: %v", addr1, common.AddressList(recipeAccounts))
				for _, a := range recipeAccounts {
					checkInclude(t, r.Coinflows, a, transferValue)
				}
				//还包含合约收款 = 收款额 - 转出（给三个账户）
				checkInclude(t, r.Coinflows, contractAddr, new(big.Int).Sub(callTx.Amount(),
					new(big.Int).SetUint64(transferValue.Uint64()*uint64(len(recipeAccounts)))))

			})
			t.Run("failed", func(t *testing.T) {

				t.Run("out of gas", func(t *testing.T) {
					callTx := types.NewTransaction(addr1, contractAddr,
						big.NewInt(0).SetInt64(transferValue.Int64()*int64(len(recipeAccounts))+2), 30000, 1, 0, data)

					r, _, _, _ := callContract(t, callTx, false)
					//公共检查
					checkFailedReceipt(t, r)
					//检查剩余Gas
					checkRemainingGas(t, dagchain, r)
				})
				t.Run("contact check failed", func(t *testing.T) {
					callTx := types.NewTransaction(addr1, contractAddr,
						big.NewInt(1), 100000, 1, 0, data)

					r, _, _, _ := callContract(t, callTx, false)
					//公共检查
					checkFailedReceipt(t, r)
					//检查剩余Gas
					checkRemainingGas(t, dagchain, r)
				})
			})
		})
	})

}

func checkRemainingGas(t *testing.T, dagchain *DagChain, r *types.Receipt) {
	assert.NotEqual(t, 0, r.GasUsed, "need have used gas")
	//这笔交易之前的执行记录
	// 从中
	var usedGas uint64
	var toalGas uint64

	if tx := dagchain.GetTransaction(r.TxHash); tx != nil {
		toalGas = tx.Gas()
	}

	err := dagchain.dag.GetTransactionStatus(r.TxHash, func(info TxStatusInfo) (b bool, e error) {
		txexec, err := dagchain.dag.GetTxReceiptAtUnit(info.UnitHash, r.TxHash)
		require.NoError(t, err)

		log.Debug("tx status",
			"action", txexec.Action, "usedGas", txexec.GasUsed,
			"txhash", r.TxHash,
			"uhash", info.UnitHash.Hex())

		usedGas += txexec.GasUsed
		return true, nil
	})
	assert.Nil(t, err)
	log.Debug("tx status", "action", r.Action, "usedGas", r.GasUsed, "txhash", r.TxHash, "total gas", toalGas, "used gas", r.GasUsed)

	assert.Equal(t, toalGas-usedGas-r.GasUsed, r.GasRemaining,
		"gas remaining should be equal  gas(%d) - used gas(%d)", toalGas-usedGas, r.GasUsed)

}

// checkFailedReceipt 对失败的回执内容进行检查
func checkFailedReceipt(t *testing.T, r *types.Receipt) {
	assert.Equal(t, r.Failed, true)
	assert.Empty(t, r.Output, "should be empty when exec failed")
	assert.Len(t, r.Coinflows, 0, "should be empty when exec failed")
	assert.NotEqual(t, 0, r.GasUsed, "need have used gas")
}

// saveResultToDB 保存执行信息作为Unit到DB
func saveResultToDB(t *testing.T, dc *DagChain, db *state.StateDB, header *types.Header,
	txexec *types.TransactionExec, receipts ...*types.Receipt) *types.Unit {
	//root, err := db.CommitTo(dc.chainDB)
	//require.NoError(t, err)
	require.NotNil(t, db)
	header.StateRoot, _ = db.IntermediateRoot(common.Address{})
	unit := types.NewUnit(header, []*types.TransactionExec{txexec})

	//err = WriteUnit(dc.chainDB, unit)
	//require.NoError(t, err)

	//手工修改末端位置
	err := dc.writeRightUnit(db, unit, receipts, nil)
	require.NoError(t, err)
	//
	//err = WriteChainTailHash(dc.chainDB, unit.Hash(), header.MC)
	//require.NoError(t, err)
	//
	//WriteStableHash(dc.chainDB, unit.Hash(), header.MC, header.Number)
	db2, err := dc.StateAt(header.MC, header.StateRoot)
	require.NoError(t, err)
	*db = *db2

	return unit
}

func mul(a, b uint64) *big.Int {
	return new(big.Int).Mul(new(big.Int).SetUint64(a), new(big.Int).SetUint64(b))
}

func checkInclude(t *testing.T, coinflows types.Coinflows, addr common.Address, amount *big.Int) {
	for _, v := range coinflows {
		if v.To == addr {
			assert.Equal(t, amount, v.Amount)
			return
		}
	}
	assert.Fail(t, "not found", "should be contains %v", addr.Hex())
}

// 合约源代码保存在 test data 目录中 transfer.sol
var tSmartContractCode = "0x6080604052336000806101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff160217905550610710806100536000396000f30060806040526004361061008e576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff1680630da229be146100935780632f1aa84b146100be57806377ca18a61461012157806387db03b7146101415780638f84aa091461016e578063d0679d34146101c5578063d294cb0f14610205578063fa3bd6c51461025c575b600080fd5b34801561009f57600080fd5b506100a8610289565b6040518082815260200191505060405180910390f35b61011f6004803603810190808035906020019082018035906020019080806020026020016040519081016040528093929190818152602001838360200280828437820191505050505050919291929080359060200190929190505050610293565b005b61013f600480360381019080803590602001909291905050506103cf565b005b34801561014d57600080fd5b5061016c600480360381019080803590602001909291905050506104d9565b005b34801561017a57600080fd5b5061018361053d565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b610203600480360381019080803573ffffffffffffffffffffffffffffffffffffffff16906020019092919080359060200190929190505050610566565b005b34801561021157600080fd5b50610246600480360381019080803573ffffffffffffffffffffffffffffffffffffffff16906020019092919050505061065f565b6040518082815260200191505060405180910390f35b34801561026857600080fd5b5061028760048036038101908080359060200190929190505050610680565b005b6000600154905090565b60008060008451840292508234101515156102ad57600080fd5b600091505b84518210156103c85784828151811015156102c957fe5b9060200190602002015190508073ffffffffffffffffffffffffffffffffffffffff166108fc859081150290604051600060405180830381858888f1935050505015801561031b573d6000803e3d6000fd5b507f091151c6f9d8fda9318c8ce50640a16ca1379e117d817054f49a68dd3f8518f0338286604051808473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020018373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001828152602001935050505060405180910390a181806001019250506102b2565b5050505050565b3373ffffffffffffffffffffffffffffffffffffffff166108fc829081150290604051600060405180830381858888f19350505050158015610415573d6000803e3d6000fd5b507f091151c6f9d8fda9318c8ce50640a16ca1379e117d817054f49a68dd3f8518f06000809054906101000a900473ffffffffffffffffffffffffffffffffffffffff163383604051808473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020018373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001828152602001935050505060405180910390a150565b60006001549050816001600082825401925050819055507f91acf26ac019f933454b8d5ebaceece943fe706a4c5f64064f865edde02e5766816001548460405180848152602001838152602001828152602001935050505060405180910390a15050565b60008060009054906101000a900473ffffffffffffffffffffffffffffffffffffffff16905090565b80341015151561057557600080fd5b8173ffffffffffffffffffffffffffffffffffffffff166108fc829081150290604051600060405180830381858888f193505050501580156105bb573d6000803e3d6000fd5b507f091151c6f9d8fda9318c8ce50640a16ca1379e117d817054f49a68dd3f8518f0338383604051808473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020018373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001828152602001935050505060405180910390a15050565b60008173ffffffffffffffffffffffffffffffffffffffff16319050919050565b60006001549050816001600082825403925050819055507f91acf26ac019f933454b8d5ebaceece943fe706a4c5f64064f865edde02e5766816001548460405180848152602001838152602001828152602001935050505060405180910390a150505600a165627a7a72305820179216eaf4262238cac6d82f218f160513f3c03ddbbac3ef6679f3c2b754ace90029"

func testContractABI() []byte {
	var b = `
[
	{
		"anonymous": false,
		"inputs": [
			{
				"indexed": false,
				"name": "sender",
				"type": "address"
			},
			{
				"indexed": false,
				"name": "beforeBalance",
				"type": "uint256"
			},
			{
				"indexed": false,
				"name": "afterBalance",
				"type": "uint256"
			}
		],
		"name": "balanceEvent",
		"type": "event"
	},
	{
		"constant": false,
		"inputs": [
			{
				"name": "a",
				"type": "int256"
			}
		],
		"name": "add",
		"outputs": [],
		"payable": false,
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"constant": false,
		"inputs": [
			{
				"name": "amount",
				"type": "uint256"
			}
		],
		"name": "contractTransferout",
		"outputs": [],
		"payable": true,
		"stateMutability": "payable",
		"type": "function"
	},
	{
		"constant": false,
		"inputs": [
			{
				"name": "to",
				"type": "address"
			},
			{
				"name": "amount",
				"type": "uint256"
			}
		],
		"name": "send",
		"outputs": [],
		"payable": true,
		"stateMutability": "payable",
		"type": "function"
	},
	{
		"constant": false,
		"inputs": [
			{
				"name": "to",
				"type": "address[]"
			},
			{
				"name": "amount",
				"type": "uint256"
			}
		],
		"name": "sends",
		"outputs": [],
		"payable": true,
		"stateMutability": "payable",
		"type": "function"
	},
	{
		"anonymous": false,
		"inputs": [
			{
				"indexed": false,
				"name": "old",
				"type": "int256"
			},
			{
				"indexed": false,
				"name": "_sum",
				"type": "int256"
			},
			{
				"indexed": false,
				"name": "add",
				"type": "int256"
			}
		],
		"name": "sumEvent",
		"type": "event"
	},
	{
		"constant": false,
		"inputs": [
			{
				"name": "a",
				"type": "int256"
			}
		],
		"name": "sub",
		"outputs": [],
		"payable": false,
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"anonymous": false,
		"inputs": [
			{
				"indexed": false,
				"name": "from",
				"type": "address"
			},
			{
				"indexed": false,
				"name": "to",
				"type": "address"
			},
			{
				"indexed": false,
				"name": "amount",
				"type": "uint256"
			}
		],
		"name": "transationEvent",
		"type": "event"
	},
	{
		"inputs": [],
		"payable": true,
		"stateMutability": "payable",
		"type": "constructor"
	},
	{
		"constant": true,
		"inputs": [
			{
				"name": "account",
				"type": "address"
			}
		],
		"name": "accountBalance",
		"outputs": [
			{
				"name": "",
				"type": "uint256"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [],
		"name": "ownerAddress",
		"outputs": [
			{
				"name": "",
				"type": "address"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [],
		"name": "sumResult",
		"outputs": [
			{
				"name": "",
				"type": "int256"
			}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	}
]
`
	return []byte(b)
}
