package witness

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"

	"gitee.com/nerthus/nerthus/accounts"
	"gitee.com/nerthus/nerthus/accounts/keystore"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/testutil"
	"gitee.com/nerthus/nerthus/consensus"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/statistics"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/node"
	"gitee.com/nerthus/nerthus/nts/protocol"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/p2p"
	"gitee.com/nerthus/nerthus/p2p/discover"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/whisper/whisperv5"
)

type Context struct {
	DB          ntsdb.Database
	Chain       *core.DagChain
	ChainConfig *params.ChainConfig
	EventMux    *event.TypeMux
	Genesis     *core.Genesis
	Engine      consensus.Engine
	Accountm    *accounts.Manager
	TxPool      *core.TxPool
	Dag         *core.DAG

	MainAccount common.Address
}

var (
	chainID = big.NewInt(99)
)
var testSinger = types.NewSigner(chainID)

func newTransaction(key *ecdsa.PrivateKey) *types.Transaction {
	return pricedTransaction(100000, 1, key)
}

func pricedTransaction(gaslimit, gasprice uint64, key *ecdsa.PrivateKey) *types.Transaction {
	to := common.StringToAddress("testaddress")
	tx, _ := types.SignTx(
		types.NewTransaction(crypto.ForceParsePubKeyToAddress(key.PublicKey), to, big.NewInt(100), gaslimit, gasprice, 0, nil),
		testSinger, key)
	return tx
}

//func fakeResult(tx *types.Transaction) *types.TxResult {
//	root := (common.Hash{}).Bytes()
//	r := types.NewReceipt(types.ActionTransferPayment, root, false, tx.Gas())
//	r.GasUsed = tx.Gas()
//	r.TxHash = tx.Hash()
//	result := types.NewTxResult(r, big.NewInt(1000), nil)
//	return result
//}

func NewTestContext(genesis *core.Genesis, keys ...*ecdsa.PrivateKey) (ctx *Context, err error) {
	testutil.SetupTestConfig()

	ctx = new(Context)
	ctx.DB, err = ntsdb.NewMemDatabase()
	if err != nil {
		return nil, err
	}
	if genesis == nil {
		genesis, err = core.LoadGenesisConfig()
		if err != nil {
			return nil, err
		}
	}
	ctx.Genesis = genesis
	ctx.Genesis.Config.ChainId = chainID
	// 尚未在结束时清理临时文件
	dir, err := ioutil.TempDir("", "nerthustest")
	if err != nil {
		return nil, err
	}

	backend := keystore.NewKeyStore(dir, keystore.StandardScryptN, keystore.StandardScryptP)
	ctx.Accountm = accounts.NewManager(backend)

	// 加载测试账户，并存储
	if len(keys) > 0 {
		for _, k := range keys {
			// 存储并解锁
			acc, err := backend.ImportECDSA(k, "foobar")
			if err != nil {
				return nil, err
			}
			//无期限解锁
			backend.TimedUnlock(acc, "foobar", 0)

			// 由外部统一
			//ctx.Genesis.Alloc[acc.Address] = core.GenesisAccount{
			//	Balance: math.MaxBig256,
			//	Nonce:   0,
			//}
		}
	}

	var ghash common.Hash
	ctx.ChainConfig, ghash, err = core.SetupGenesisUnit(ctx.DB, ctx.Genesis)
	log.Debug("test geneis using core.DevGenesisUnit()", "ghash", ghash)
	if err != nil {
		return nil, err
	}
	//ctx.Engine = backend. .New("", 0, 0, "", 1, 1)

	ctx.Chain, err = core.NewDagChain(ctx.DB, ctx.ChainConfig, ctx.Engine, vm.Config{})
	if err != nil {
		return nil, err
	}
	//for addr, v := range ctx.Genesis.Alloc {
	//	state, _ := ctx.Chain.State(addr)
	//}
	ctx.EventMux = &event.TypeMux{}

	txPoolCfg := core.DefaultTxPoolConfig
	txPoolCfg.DiskTxPath = filepath.Join(dir, "txchache")
	os.Mkdir(dir, os.ModePerm)
	ctx.TxPool = core.NewTxPool(txPoolCfg, ctx.ChainConfig, ctx.Chain)
	ctx.Dag, err = core.NewDag(ctx.DB, ctx.EventMux, ctx.Chain)
	if err != nil {
		return
	}
	//ctx.Dag.Start(nil)
	return
}

type TestBackendEx struct {
	ctx *Context
}

func (b *TestBackendEx) AccountManager() *accounts.Manager {
	return b.ctx.Accountm
}
func (b *TestBackendEx) EventMux() *event.TypeMux {
	return b.ctx.EventMux
}

func (b *TestBackendEx) DagChain() *core.DagChain {
	return b.ctx.Chain
}
func (b *TestBackendEx) ChainDb() ntsdb.Database {
	return b.ctx.DB
}
func (b *TestBackendEx) DagReader() *core.DAG {
	return b.ctx.Dag
}
func (b *TestBackendEx) Genesis() *core.Genesis {
	return b.ctx.Genesis
}
func (b *TestBackendEx) GetMainAccount() common.Address {
	return b.ctx.MainAccount
}

func (b *TestBackendEx) TxPool() *core.TxPool {
	return b.ctx.TxPool
}

func (b *TestBackendEx) IsSystemWitness(addr common.Address) (bool, error) {
	return false, nil
}

// general ntsereum API
func (b *TestBackendEx) ProtocolVersion() int {
	return 0
}

// @Author: yin
func (b *TestBackendEx) ValidateTransaction(signedUnit *types.Transaction) error {
	return errors.New("not yet implement")
}
func (b *TestBackendEx) SendTransaction(signedPayment *types.Transaction) error {
	return errors.New("not yet implement")
}

func (b *TestBackendEx) ChainConfig() *params.ChainConfig {
	return b.ctx.ChainConfig
}

func (b *TestBackendEx) SuggestPrice(ctx context.Context) (*big.Int, error) {
	return big.NewInt(0), errors.New("not yet implement")
}

func (b *TestBackendEx) GetPoolNonce(ctx context.Context, addr common.Address) (uint64, error) {
	return 0, errors.New("not yet implement")
}

func (b *TestBackendEx) StartWitness(mainAccount common.Address, password string) error {
	return errors.New("not yet implement")
}
func (b *TestBackendEx) StopWitness() error {
	return errors.New("not yet implement")
}

func (b *TestBackendEx) Unlock(acct common.Address, password string) error {
	// 解锁账号，直到程序退出或停止见证
	ks := b.AccountManager().Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)
	if err := ks.TimedUnlock(accounts.Account{Address: acct}, password, 0); err != nil {
		return err
	}
	return nil
}

func (b *TestBackendEx) GetSystemWitnessList() ([]common.Address, error) {
	panic("not yet implement")
}

func (b *TestBackendEx) ServiceContext() *node.ServiceContext {
	return nil
}

func (b *TestBackendEx) GetPeers() []*whisperv5.Peer {
	return nil
}

func (b *TestBackendEx) GetNode() *discover.Node {
	return nil
}

func (b *TestBackendEx) SubscribePeerEvent(ch chan p2p.PeerEvent) event.Subscription {
	return nil
}

func (b *TestBackendEx) CurrentIsSystemWitness() (bool, error) {
	return false, nil
}
func (b *TestBackendEx) PeerSet() protocol.PeerSet {
	return nil
}
func (b *TestBackendEx) GetSysWitList() (common.AddressList, error) {
	return nil, nil
}
func (b *TestBackendEx) GetPrivateKey() (*ecdsa.PrivateKey, error) {
	return nil, nil
}

func (b *TestBackendEx) Sta() *statistics.Statistic {
	return nil
}
