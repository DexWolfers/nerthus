package txpipe

import (
	"math/big"
	"sort"
	"sync"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common"

	"github.com/stretchr/testify/require"

	"gitee.com/nerthus/nerthus/crypto"

	"gitee.com/nerthus/nerthus/nts/pman"

	"gitee.com/nerthus/nerthus/nts/protocol"

	"gitee.com/nerthus/nerthus/p2p"
	"gitee.com/nerthus/nerthus/p2p/discover"

	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/event"
)

type codeFlag int

const (
	disableTx codeFlag = 1 << iota
	enableTx
)

// testTxPool is a fake, helper transaction pool for testing purposes
type testTxPool struct {
	txFeed event.Feed
	pool   []*types.Transaction // Collection of all transactions

	lock sync.RWMutex // Protects the transaction pool
}

func (p *testTxPool) AddRemotesTxpipe(txs []*types.Transaction) error {
	panic("implement me")
}

func (p *testTxPool) IsFull() bool {
	panic("implement me")
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
	return nil
}

func TestConnect(t *testing.T) {

	ma := pman.NewPeerSet()
	mb := pman.NewPeerSet()

	ka, _ := crypto.GenerateKey()
	kb, _ := crypto.GenerateKey()
	pa, pb := p2p.MsgPipe()

	a := New(Config{ListenAddr: ":0", PrivateKey: ka, MaxPeers: 1}, ma, &testTxPool{})
	b := New(Config{ListenAddr: ":0", PrivateKey: kb, MaxPeers: 1}, mb, &testTxPool{})

	a.Run()
	b.Run()

	maina := pman.NewPeerHandler(0, p2p.NewPeer(discover.PubkeyID(&ka.PublicKey), "a", nil), pa, pman.DisableTx, nil)
	mainb := pman.NewPeerHandler(0, p2p.NewPeer(discover.PubkeyID(&kb.PublicKey), "a", nil), pb, pman.DisableTx, nil)

	maina.Set(pman.CfgSubPipeNode, a.Node().String())
	mainb.Set(pman.CfgSubPipeNode, b.Node().String())

	peerEventc := make(chan protocol.PeerEvent, 1)
	a.peerSet.SubscribePeerEvent(peerEventc)

	mb.Register(maina)
	ma.Register(mainb)
	defer a.Close()
	defer b.Close()

	select {
	case ev := <-peerEventc:
		require.Equal(t, ev.PeerID.String(), b.Node().ID.String())
	case <-time.After(time.Second * 2):
		require.Fail(t, "timeout")
	}

}
