// This file contains some shares testing functionality, common to  multiple
// different files and modules being tested.

package nts

import (
	"fmt"
	"math/big"
	"net"
	"sort"
	"sync"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/consensus/bft/backend"

	"gitee.com/nerthus/nerthus/common/testutil"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/p2p"
	"gitee.com/nerthus/nerthus/p2p/discover"

	"github.com/stretchr/testify/require"
)

var (
	testBankKey, _ = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	testBank       = crypto.ForceParsePubKeyToAddress(testBankKey.PublicKey)
)

// testTxPool is a fake, helper transaction pool for testing purposes
type testTxPool struct {
	txFeed event.Feed
	pool   []*types.Transaction // Collection of all transactions

	lock sync.RWMutex // Protects the transaction pool
}

// AddRemotes appends a batch of transactions to the pool, and notifies any
// listeners if the addition channel is non nil
func (p *testTxPool) AddRemotes(txs []*types.Transaction) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.pool = append(p.pool, txs...)
	for _, v := range txs {
		p.txFeed.Send(types.TxPreEvent{Tx: v, IsLocal: false})
	}

	return nil
}

var signer = types.NewSigner(big.NewInt(1))

// Pending returns all the transactions known to the pool
func (p *testTxPool) Pending() (types.Transactions, error) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	batches := make(types.Transactions, 0, 10)
	for _, tx := range p.pool {
		batches = append(batches, tx)
	}
	sort.Sort(types.TxByNonce(batches))
	return batches, nil
}

func (p *testTxPool) SubscribeTxPreEvent(ch chan<- types.TxPreEvent) event.Subscription {
	return p.txFeed.Subscribe(ch)
}

func (p *testTxPool) Get(hash common.Hash) *types.Transaction {
	for _, v := range p.pool {
		if v.Hash() == hash {
			return v
		}
	}
	return nil

}

func init() {
	testutil.SetupTestConfig()
}

func newTestProtocolManager(server *p2p.Server, t testing.TB, initGenesis ...func(*core.Genesis)) *ProtocolManager {

	var (
		evmux = new(event.TypeMux)
		db, _ = ntsdb.NewMemDatabase()
	)
	gspec, err := core.LoadGenesisConfig()
	if err != nil {
		t.Fatal(err)
	}
	gspec.Alloc[testBank] = core.GenesisAccount{Balance: big.NewInt(1000000)}
	if len(initGenesis) > 0 && initGenesis[0] != nil {
		initGenesis[0](gspec)
	}
	gspec.MustCommit(db)

	engine := backend.New(&bft.DefaultConfig, evmux, nil, nil, nil)
	dagchain, err := core.NewDagChain(db, gspec.Config, engine, vm.Config{})
	dagchain.SetOptions(core.SetBftEngine(engine))
	if err != nil {
		t.Fatal(err)
	}

	core.NewDag(db, evmux, dagchain)
	cfg := ProtocolConfig{
		MaxMessageSize: DefaultConfig.MaxMessageSize,
		MaxPeers:       100,
		NetworkId:      99,
	}

	ph, err := NewProtocolManager(server, &cfg, dagchain.Config(), evmux, &testTxPool{}, dagchain, db, nil)
	if err != nil {
		t.Fatal(err)
	}

	server.Protocols = append(server.Protocols, ph.Protocols()...)
	err = server.Start()
	if err != nil {
		t.Fatalf("failed to start server. %v", err)
	}
	if err = ph.Start(); err != nil {
		t.Fatal(err)
	}

	return ph
}

var port = 30303

//var nodes []*p2p.Server

func newNode(t testing.TB, nodes ...*p2p.Server) *p2p.Server {
	var err error
	ip := net.IPv4(127, 0, 0, 1)
	port++
	addr := fmt.Sprintf(":%d", port) // e.g. ":30303"
	name := common.MakeName("whisper-go", "2.0")
	var peers []*discover.Node
	for _, p := range nodes {
		peer := discover.NewNode(p.Self().ID, ip, p.Self().UDP, p.Self().TCP)
		peers = append(peers, peer)
	}
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	server := &p2p.Server{
		Config: p2p.Config{
			PrivateKey:     key,
			MaxPeers:       999,
			Name:           name,
			ListenAddr:     addr,
			NAT:            nil,
			BootstrapNodes: peers,
			StaticNodes:    peers,
			TrustedNodes:   peers,
		},
	}
	return server
}

func waitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		return false // completed normally
	case <-time.After(timeout):
		return true // timed out
	}
}
