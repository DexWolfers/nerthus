package core

import (
	"crypto/ecdsa"
	"math/big"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/ntsdb"
)

var testLogger = log.Root().New("module", "test")

type testSystemBackend struct {
	id  uint64
	sys *testSystem

	engine Engine
	peers  bft.ValidatorSet
	events *event.TypeMux

	committedMsgs []testCommittedMsgs
	sentMsgs      [][]byte // store the message when Send is called by core

	address common.Address
	db      ntsdb.Database
}

type testCommittedMsgs struct {
	commitProposal bft.Proposal
	committedSeals []types.WitenssVote
}

// ==============================================
//
// define the functions that needs to be provided for Istanbul.

func (self *testSystemBackend) Address() common.Address {
	return self.address
}

// Peers returns all connected peers
func (self *testSystemBackend) Validators(proposal bft.Proposal) (bft.ValidatorSet, error) {
	return self.peers, nil
}

func (self *testSystemBackend) EventMux() *event.TypeMux {
	return self.events
}

func (self *testSystemBackend) Send(message []byte, target common.Address) error {
	testLogger.Info("enqueuing a message...", "address", self.Address())
	self.sentMsgs = append(self.sentMsgs, message)
	self.sys.queuedMessage <- bft.MessageEvent{
		Payload: message,
	}
	return nil
}

func (self *testSystemBackend) Broadcast(valSet bft.ValidatorSet, message []byte) error {
	testLogger.Info("enqueuing a message...", "address", self.Address())
	self.sentMsgs = append(self.sentMsgs, message)
	self.sys.queuedMessage <- bft.MessageEvent{
		Payload: message,
	}
	return nil
}

func (self *testSystemBackend) Gossip(valSet bft.ValidatorSet, message []byte) error {
	testLogger.Warn("not sign any data")
	return nil
}

func (self *testSystemBackend) Commit(proposal bft.Proposal, vote []types.WitenssVote) error {
	testLogger.Info("commit message", "address", self.Address())
	self.committedMsgs = append(self.committedMsgs, testCommittedMsgs{
		commitProposal: proposal,
		committedSeals: vote,
	})

	// fake new head events
	go self.events.Post(bft.FinalCommittedEvent{Err: make(chan error, 1)})
	return nil
}

func (self *testSystemBackend) Verify(proposal bft.Proposal) (time.Duration, error) {
	return 0, nil
}

func (self *testSystemBackend) Sign(sigHelper types.SignHelper) (types.SignHelper, error) {
	testLogger.Warn("not sign any data")
	return sigHelper, nil
}
func (self *testSystemBackend) SignBytes(b []byte) (types.SignHelper, error) {
	testLogger.Warn("not sign any data")
	return types.NewSignContent(), nil
}

func (self *testSystemBackend) CheckSignature([]byte, common.Address, []byte) error {
	return nil
}

func (self *testSystemBackend) CheckValidatorSignature(data []byte, sig []byte) (common.Address, error) {
	addr := common.Address{}
	addr.SetBytes(sig)
	return addr, nil
}

func (self *testSystemBackend) Hash(b interface{}) common.Hash {
	return common.StringToHash("Test")
}

func (self *testSystemBackend) NewRequest(request bft.Proposal) {
	go self.events.Post(bft.RequestEvent{
		Proposal: request,
	})
}

func (self *testSystemBackend) HasBadProposal(hash common.Hash) bool {
	return false
}

func (self *testSystemBackend) LastProposal() (bft.Proposal, common.Address) {
	l := len(self.committedMsgs)
	if l > 0 {
		return self.committedMsgs[l-1].commitProposal, common.Address{}
	}

	return makeUnit(common.EmptyAddress, common.Hash{}, common.Hash{}, 0), common.Address{}
}

// Only block height 5 will return true
func (self *testSystemBackend) HasPropsal(mcaddr common.Address, hash common.Hash, number *big.Int) bool {
	return number.Cmp(big.NewInt(5)) == 0
}

func (self *testSystemBackend) GetProposer(mcaddr common.Address, number uint64) common.Address {
	return common.Address{}
}

func (self *testSystemBackend) GetParentProposer(childProposal bft.Proposal) (common.Address, error) {
	return common.Address{}, nil
}

func (self *testSystemBackend) ParentValidators(proposal bft.Proposal) bft.ValidatorSet {
	return self.peers
}
func (self *testSystemBackend) FetchAncestor(unit *types.Unit) {
	return
}
func (self *testSystemBackend) TxPool(tx *types.Transaction) {
	return
}

// ==============================================
//
// define the struct that need to be provided for integration tests.

type testSystem struct {
	backends []*testSystemBackend

	queuedMessage chan bft.MessageEvent
	quit          chan struct{}
}

func newTestSystem(n uint64) *testSystem {
	//testLogger.SetHandler(elog.StdoutHandler)
	return &testSystem{
		backends: make([]*testSystemBackend, n),

		queuedMessage: make(chan bft.MessageEvent),
		quit:          make(chan struct{}),
	}
}

func generateValidators(n int) []common.Address {
	vals := make([]common.Address, 0)
	for i := 0; i < n; i++ {
		privateKey, _ := crypto.GenerateKey()
		vals = append(vals, crypto.ForceParsePubKeyToAddress(privateKey.PublicKey))
	}
	return vals
}

func newTestValidatorSet(n int) bft.ValidatorSet {
	// 适配 TODO
	addr := generateValidators(n)
	return bft.NewSet(0, newTestZeroProposal().Hash(), addr)
}

//FIXME: int64 is needed for N and F
func NewTestSystemWithBackend(n, f uint64) *testSystem {
	//testLogger.SetHandler(log.StdoutHandler)
	//
	////addrs := generateValidators(int(n))
	sys := newTestSystem(n)
	//config := bft.DefaultConfig
	//vset := newTestValidatorSet(int(n))
	//
	//for i := uint64(0); i < n; i++ {
	//	// TODO 适配
	//	//vset := bft.NewSet(0, common.Hash{}, addrs)
	//	backend := sys.NewBackend(i)
	//	backend.peers = vset
	//	backend.address = vset.GetByIndex(i).Address()
	//
	//	core := New(backend, &config).(*core)
	//	core.state = StateAcceptRequest
	//	core.current = newRoundState(&bft.View{
	//		Round:    big.NewInt(0),
	//		Sequence: big.NewInt(1),
	//	}, vset, common.EmptyAddress, common.Hash{}, nil, nil, func(hash common.Hash) bool {
	//		return false
	//	})
	//	core.valSet = vset
	//	core.logger = testLogger
	//	core.validateFn = func(m *protocol.Message) (common.Address, error) {
	//		return backend.CheckValidatorSignature(m.Msg, m.Address.Bytes())
	//	}
	//
	//	backend.engine = core
	//}
	return sys
}

// listen will consume messages from queue and deliver a message to core
func (t *testSystem) listen() {
	for {
		select {
		case <-t.quit:
			return
		case queuedMessage := <-t.queuedMessage:
			testLogger.Info("consuming a queue message...")
			for _, backend := range t.backends {
				go backend.EventMux().Post(queuedMessage)
			}
		}
	}
}

// Run will start system components based on given flag, and returns a closer
// function that caller can control lifecycle
//
// Given a true for core if you want to initialize core engine.
func (t *testSystem) Run(core bool) func() {
	for _, b := range t.backends {
		if b != nil && core {
			if b.engine == nil {
				continue
			}
			b.engine.Start() // start Istanbul core
		}
	}

	go t.listen()
	closer := func() { t.stop(core) }
	return closer
}

func (t *testSystem) stop(core bool) {
	close(t.quit)

	for _, b := range t.backends {
		if b != nil && core {
			b.engine.Stop()
		}
	}
}

func (t *testSystem) NewBackend(id uint64) *testSystemBackend {
	// assume always success
	ethDB, _ := ntsdb.NewMemDatabase()
	backend := &testSystemBackend{
		id:     id,
		sys:    t,
		events: new(event.TypeMux),
		db:     ethDB,
	}

	t.backends[id] = backend
	return backend
}

// ==============================================
//
// helper functions.

func getPublicKeyAddress(privateKey *ecdsa.PrivateKey) common.Address {
	return crypto.ForceParsePubKeyToAddress(privateKey.PublicKey)
}
