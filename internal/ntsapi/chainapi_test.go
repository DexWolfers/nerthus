package ntsapi

import (
	"context"
	"io/ioutil"
	"math/big"
	"testing"

	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/consensus/bft/backend"

	"gitee.com/nerthus/nerthus/accounts/keystore"
	"gitee.com/nerthus/nerthus/consensus/ethash"

	"gitee.com/nerthus/nerthus/accounts"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/hexutil"
	"gitee.com/nerthus/nerthus/common/testutil"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"
	"github.com/stretchr/testify/require"
)

type API struct {
	chainConfig *params.ChainConfig
	dagChain    *core.DagChain
	dag         *core.DAG
}

func (a *API) ProtocolVersion() int {
	return 0
}
func (a *API) ChainDb() ntsdb.Database {
	return nil
}
func (a *API) EventMux() *event.TypeMux {
	return nil
}
func (a *API) AccountManager() *accounts.Manager {
	return nil
}

// Genesis returns Genesis unit
func (a *API) Genesis() *core.Genesis {
	return nil
}

func (a *API) SendTransaction(signedPayment *types.Transaction) error {
	return nil
}

func (a *API) ChainConfig() *params.ChainConfig {
	return a.chainConfig
}

func (a *API) SuggestPrice(ctx context.Context) (*big.Int, error) {
	return nil, nil
}

func (a *API) GetPoolNonce(ctx context.Context, addr common.Address) (uint64, error) {
	return 0, nil
}
func (a *API) DagChain() *core.DagChain {
	return a.dagChain
}
func (a *API) DagReader() *core.DAG {
	return a.dag
}

func (a *API) StartWitness(mainAccount common.Address, password string) error {
	return nil
}
func (a *API) StopWitness() error {
	return nil
}

func (a *API) StartMember(ac common.Address, password string) error {
	return nil
}
func (a *API) StopMember() error {
	return nil
}

// SendRemoteTransactions for stress
func (a *API) SendRemoteTransactions(txs []*types.Transaction) error {
	return nil
}

func (a *API) GetWitnessGroupList() []core.AddrNode {
	return nil
}
func (a *API) TxPool() *core.TxPool {
	return core.NewTxPool(core.DefaultTxPoolConfig, a.chainConfig, a.dagChain)
}

func TestPublicChainAPI_EstimateGas_transfer(t *testing.T) {
	db, err := ntsdb.NewMemDatabase()
	require.Nil(t, err)
	testutil.SetupTestConfig()
	g, err := core.LoadGenesisConfig()
	g.MustCommit(db)
	engine := backend.New(&bft.DefaultConfig, nil, nil, nil, nil)
	dagChain, err := core.NewDagChain(db, params.TestChainConfig, engine, vm.Config{})
	require.Nil(t, err)
	dag, err := core.NewDag(db, nil, dagChain)
	require.Nil(t, err)
	backend := &API{chainConfig: params.TestChainConfig, dagChain: dagChain, dag: dag}
	api := NewPublicChainAPI(backend)
	ctx := context.Background()
	to := common.StringToAddress("to")
	from := common.StringToAddress("from")
	t.Log("from", from.String())
	args := SendTxArgs{
		From:     from,
		To:       to,
		Gas:      (*hexutil.Big)(big.NewInt(1000000)),
		GasPrice: (*hexutil.Big)(big.NewInt(1)),
		Amount:   (*hexutil.Big)(big.NewInt(100)),
		Timeout:  (*hexutil.Big)(big.NewInt(0)),
	}
	gas, err := api.EstimateGas(ctx, args)
	require.Nil(t, err)
	t.Log((uint64)(gas))
}

func TestPublicChainAPI_EstimateGas_executeContract(t *testing.T) {
	db, err := ntsdb.NewMemDatabase()
	require.Nil(t, err)
	testutil.SetupTestConfig()
	g, err := core.LoadGenesisConfig()
	g.MustCommit(db)
	engine := backend.New(&bft.DefaultConfig, nil, nil, nil, nil)
	dagChain, err := core.NewDagChain(db, params.TestChainConfig, engine, vm.Config{})
	require.Nil(t, err)
	dag, err := core.NewDag(db, nil, dagChain)
	require.Nil(t, err)
	backend := &API{chainConfig: params.TestChainConfig, dagChain: dagChain, dag: dag}
	api := NewPublicChainAPI(backend)
	ctx := context.Background()
	addr := common.StringToAddress("from")
	data := sc.CreateCallInputData(sc.FuncApplyWitnessID, addr)
	from := common.StringToAddress("from")
	t.Log("from", from.String())
	args := SendTxArgs{
		From:     from,
		To:       params.SCAccount,
		Gas:      (*hexutil.Big)(big.NewInt(0)),
		GasPrice: (*hexutil.Big)(big.NewInt(1)),
		Amount:   (*hexutil.Big)(big.NewInt(0)),
		Data:     (hexutil.Bytes)(data),
		Timeout:  (*hexutil.Big)(big.NewInt(0)),
	}
	gas, err := api.EstimateGas(ctx, args)
	require.Nil(t, err)
	t.Log((uint64)(gas))
}

func TestPublicChainAPI_EstimateGas_deployContract(t *testing.T) {
	db, err := ntsdb.NewMemDatabase()
	require.Nil(t, err)
	testutil.SetupTestConfig()
	g, err := core.LoadGenesisConfig()
	g.MustCommit(db)
	engine := backend.New(&bft.DefaultConfig, nil, nil, nil, nil)
	dagChain, err := core.NewDagChain(db, params.TestChainConfig, engine, vm.Config{})
	require.Nil(t, err)
	dag, err := core.NewDag(db, nil, dagChain)
	require.Nil(t, err)
	backend := &API{chainConfig: params.TestChainConfig, dagChain: dagChain, dag: dag}
	api := NewPublicChainAPI(backend)
	ctx := context.Background()
	var tSmartContractCode = "0x6080604052336000806101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff160217905550610710806100536000396000f30060806040526004361061008e576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff1680630da229be146100935780632f1aa84b146100be57806377ca18a61461012157806387db03b7146101415780638f84aa091461016e578063d0679d34146101c5578063d294cb0f14610205578063fa3bd6c51461025c575b600080fd5b34801561009f57600080fd5b506100a8610289565b6040518082815260200191505060405180910390f35b61011f6004803603810190808035906020019082018035906020019080806020026020016040519081016040528093929190818152602001838360200280828437820191505050505050919291929080359060200190929190505050610293565b005b61013f600480360381019080803590602001909291905050506103cf565b005b34801561014d57600080fd5b5061016c600480360381019080803590602001909291905050506104d9565b005b34801561017a57600080fd5b5061018361053d565b604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200191505060405180910390f35b610203600480360381019080803573ffffffffffffffffffffffffffffffffffffffff16906020019092919080359060200190929190505050610566565b005b34801561021157600080fd5b50610246600480360381019080803573ffffffffffffffffffffffffffffffffffffffff16906020019092919050505061065f565b6040518082815260200191505060405180910390f35b34801561026857600080fd5b5061028760048036038101908080359060200190929190505050610680565b005b6000600154905090565b60008060008451840292508234101515156102ad57600080fd5b600091505b84518210156103c85784828151811015156102c957fe5b9060200190602002015190508073ffffffffffffffffffffffffffffffffffffffff166108fc859081150290604051600060405180830381858888f1935050505015801561031b573d6000803e3d6000fd5b507f091151c6f9d8fda9318c8ce50640a16ca1379e117d817054f49a68dd3f8518f0338286604051808473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020018373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001828152602001935050505060405180910390a181806001019250506102b2565b5050505050565b3373ffffffffffffffffffffffffffffffffffffffff166108fc829081150290604051600060405180830381858888f19350505050158015610415573d6000803e3d6000fd5b507f091151c6f9d8fda9318c8ce50640a16ca1379e117d817054f49a68dd3f8518f06000809054906101000a900473ffffffffffffffffffffffffffffffffffffffff163383604051808473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020018373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001828152602001935050505060405180910390a150565b60006001549050816001600082825401925050819055507f91acf26ac019f933454b8d5ebaceece943fe706a4c5f64064f865edde02e5766816001548460405180848152602001838152602001828152602001935050505060405180910390a15050565b60008060009054906101000a900473ffffffffffffffffffffffffffffffffffffffff16905090565b80341015151561057557600080fd5b8173ffffffffffffffffffffffffffffffffffffffff166108fc829081150290604051600060405180830381858888f193505050501580156105bb573d6000803e3d6000fd5b507f091151c6f9d8fda9318c8ce50640a16ca1379e117d817054f49a68dd3f8518f0338383604051808473ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff1681526020018373ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff168152602001828152602001935050505060405180910390a15050565b60008173ffffffffffffffffffffffffffffffffffffffff16319050919050565b60006001549050816001600082825403925050819055507f91acf26ac019f933454b8d5ebaceece943fe706a4c5f64064f865edde02e5766816001548460405180848152602001838152602001828152602001935050505060405180910390a150505600a165627a7a72305820179216eaf4262238cac6d82f218f160513f3c03ddbbac3ef6679f3c2b754ace90029"
	createContractData := sc.CreateCallInputData(sc.FuncCreateContract, common.FromHex(tSmartContractCode))
	from := common.StringToAddress("from")
	t.Log("from", from.String())
	args := SendTxArgs{
		From:     from,
		To:       params.SCAccount,
		Gas:      (*hexutil.Big)(big.NewInt(1000000)),
		GasPrice: (*hexutil.Big)(big.NewInt(1)),
		Amount:   (*hexutil.Big)(new(big.Int).SetUint64(params.ConfigParamsSCWitnessMinMargin)),
		Data:     (hexutil.Bytes)(createContractData),
		Timeout:  (*hexutil.Big)(big.NewInt(0)),
	}
	gas, err := api.EstimateGas(ctx, args)
	require.Nil(t, err)
	t.Log((uint64)(gas))
}

//pragma solidity ^0.4.1;
//contract Factory {
//	constructor () public {
//	new constr();
//	}
//}
//contract constr {
//}
func TestPublicChainAPI_EstimateGas_deployContract_newContract(t *testing.T) {
	db, err := ntsdb.NewMemDatabase()
	require.Nil(t, err)
	testutil.SetupTestConfig()
	g, err := core.LoadGenesisConfig()
	g.MustCommit(db)
	engine := backend.New(&bft.DefaultConfig, nil, nil, nil, nil)
	dagChain, err := core.NewDagChain(db, params.TestChainConfig, engine, vm.Config{})
	require.Nil(t, err)
	dag, err := core.NewDag(db, nil, dagChain)
	require.Nil(t, err)
	backend := &API{chainConfig: params.TestChainConfig, dagChain: dagChain, dag: dag}
	api := NewPublicChainAPI(backend)
	ctx := context.Background()
	var tSmartContractCode = "0x6080604052348015600f57600080fd5b5060166037565b604051809103906000f0801580156031573d6000803e3d6000fd5b50506046565b60405160528061008983390190565b6035806100546000396000f3006080604052600080fd00a165627a7a723058201bc38e7f71aadaf1f8cc1353b9bdfa73aa85f6a9cd7bb330984e585bc81ba54300296080604052348015600f57600080fd5b50603580601d6000396000f3006080604052600080fd00a165627a7a723058208f4ff9009c285766375f22c9ddac85ddbed726a0ce838d37c1ff08212d71b3ec0029"
	createContractData := sc.CreateCallInputData(sc.FuncCreateContract, common.FromHex(tSmartContractCode))
	from := common.StringToAddress("from")
	t.Log("from", from.String())
	args := SendTxArgs{
		From:     from,
		To:       params.SCAccount,
		Gas:      (*hexutil.Big)(big.NewInt(1000000)),
		GasPrice: (*hexutil.Big)(big.NewInt(1)),
		Amount:   (*hexutil.Big)(new(big.Int).SetUint64(0)),
		Data:     (hexutil.Bytes)(createContractData),
		Timeout:  (*hexutil.Big)(big.NewInt(0)),
	}
	_, err = api.EstimateGas(ctx, args)
	require.EqualError(t, err, vm.ErrNotStrideChain.Error())
}

func TestPowLimitTimeConsuming(t *testing.T) {
	chain := common.StringToAddress("chain")
	inputData := sc.CreateCallInputData(sc.FuncReplaceWitnessID, chain)
	tx := types.NewTransaction(chain, params.SCAccount, big.NewInt(0), 0, 0, types.DefTxTimeout, inputData)
	seedCh := ethash.ProofWork(tx, 1, big.NewInt(31), make(chan struct{}))
	seed := <-seedCh
	t.Log(seed)
}

func TestPrivateAccountAPI_ImportRawKey(t *testing.T) {
	dir, err := ioutil.TempDir("", "nerthustest")
	require.Nil(t, err)

	backend := keystore.NewKeyStore(dir, keystore.StandardScryptN, keystore.StandardScryptP)
	manager := accounts.NewManager(backend)
	api := PrivateAccountAPI{am: manager}
	addr, err := api.ImportRawKey("d748bcbbae887c9189c4a7b863f5e97521dfb168e9038e9ce3698bb8b8b056e7", "123123")
	require.Nil(t, err)
	t.Log(addr.Hex())
}
