package backend

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common/testutil"

	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/state"
	"github.com/stretchr/testify/require"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/consensus"
	istanbul "gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"
)

// in this test, we can set n to 1, and it means we can process Istanbul and commit a
// block by one node. Otherwise, if n is larger than 1, we have to generate
// other fake events to process Istanbul.
func newBlockChain(n int) (*core.DagChain, *backend) {
	testutil.SetupTestConfig()

	genesis, nodeKeys := getGenesisAndKeys(n)
	memDB, _ := ntsdb.NewMemDatabase()
	config := istanbul.DefaultConfig
	// Use the first key as private key
	mux := new(event.TypeMux)
	b, _ := New(&config, mux, nodeKeys[0], nil, memDB).(*backend)
	genesis.MustCommit(memDB)
	blockchain, err := core.NewDagChain(memDB, nil, b, vm.Config{})
	if err != nil {
		panic(err)
	}
	//b.Start(blockchain, blockchain.CurrentBlock, blockchain.HasBadBlock)
	mc := common.Address{}
	b.Start(blockchain, mc, params.SCAccount, nil, nil, nil)
	//snap, err := b.snapshot(blockchain, 0, common.Hash{}, nil)
	//if err != nil {
	//	panic(err)
	//}
	//if snap == nil {
	//	panic("failed to get snapshot")
	//}
	//proposerAddr := snap.ValSet.GetProposer().Address()
	proposerAddr := b.GetProposer(mc, 1)
	// find proposer key
	for _, key := range nodeKeys {
		addr := crypto.ForceParsePubKeyToAddress(key.PublicKey)
		if addr.String() == proposerAddr.String() {
			//b.privateKey = key
			b.address = addr
		}
	}

	return blockchain, b
}

func getGenesisAndKeys(n int) (*core.Genesis, []*ecdsa.PrivateKey) {
	// Setup validators
	var nodeKeys = make([]*ecdsa.PrivateKey, n)
	var addrs = make([]common.Address, n)
	for i := 0; i < n; i++ {
		nodeKeys[i], _ = crypto.GenerateKey()
		addrs[i] = crypto.ForceParsePubKeyToAddress(nodeKeys[i].PublicKey)
	}

	// generate genesis block
	genesis := core.DefaultGenesisUnit()
	genesis.Config = params.TestChainConfig
	// force enable Istanbul engine
	//genesis.Config.Istanbul = &params.IstanbulConfig{}
	//genesis.Config.Ethash = nil
	//genesis.Difficulty = defaultDifficulty
	//genesis.Nonce = emptyNonce.Uint64()
	//genesis.Mixhash = defaultHash

	//appendValidators(genesis, addrs)
	return genesis, nodeKeys
}

func makeHeader(parent *types.Unit, config *istanbul.Config) *types.Header {
	header := &types.Header{
		ParentHash: parent.Hash(),
		Number:     parent.Number() + common.Big1.Uint64(),
		GasLimit:   core.CalcGasLimit(parent),
		GasUsed:    0,
		//Extra:      parent.Extra(),
		Timestamp: parent.Timestamp() + config.BlockPeriod,
		//Difficulty: defaultDifficulty,
	}
	return header
}

func makeBlock(chain *core.DagChain, engine *backend, parent *types.Unit) *types.Unit {
	block := makeBlockWithoutSeal(chain, engine, parent)
	block, _ = engine.Seal(chain, block, nil)
	return block
}
func makeBlockWithoutSealHeader(chain *core.DagChain, engine *backend, header *types.Header) *types.Unit {
	unit := types.NewUnit(header, nil)
	engine.Prepare(chain, unit)
	block, _ := engine.Finalize(chain, header, nil, nil, nil)
	return block
}
func makeBlockWithoutSeal(chain *core.DagChain, engine *backend, parent *types.Unit) *types.Unit {
	header := makeHeader(parent, engine.config)
	unit := types.NewUnit(header, nil)
	engine.Prepare(chain, unit)
	state, _ := chain.StateAt(parent.MC(), parent.Root())
	block, _ := engine.Finalize(chain, header, state, nil, nil)
	return block
}

func TestPrepare(t *testing.T) {
	chain, engine := newBlockChain(1)
	header := makeHeader(chain.Genesis(), engine.config)
	unit := types.NewUnit(header, nil)
	err := engine.Prepare(chain, unit)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
	header.ParentHash = common.StringToHash("1234567890")
	err = engine.Prepare(chain, unit)
	if err != consensus.ErrUnknownAncestor {
		t.Errorf("error mismatch: have %v, want %v", err, consensus.ErrUnknownAncestor)
	}
}

func TestSealStopChannel(t *testing.T) {
	chain, engine := newBlockChain(4)
	block := makeBlockWithoutSeal(chain, engine, chain.Genesis())
	stop := make(chan error, 1)
	eventSub := engine.EventMux().Subscribe(istanbul.RequestEvent{})
	eventLoop := func() {
		ev := <-eventSub.Chan()
		_, ok := ev.Data.(istanbul.RequestEvent)
		if !ok {
			t.Errorf("unexpected event comes: %v", reflect.TypeOf(ev.Data))
		}
		stop <- nil
		eventSub.Unsubscribe()
	}
	go eventLoop()
	finalBlock, err := engine.Seal(chain, block, stop)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
	if finalBlock != nil {
		t.Errorf("block mismatch: have %v, want nil", finalBlock)
	}
}

func TestSealCommittedOtherHash(t *testing.T) {
	chain, engine := newBlockChain(4)
	block := makeBlockWithoutSeal(chain, engine, chain.Genesis())
	otherBlock := makeBlockWithoutSeal(chain, engine, block)
	eventSub := engine.EventMux().Subscribe(istanbul.RequestEvent{})
	eventLoop := func() {
		ev := <-eventSub.Chan()
		_, ok := ev.Data.(istanbul.RequestEvent)
		if !ok {
			t.Errorf("unexpected event comes: %v", reflect.TypeOf(ev.Data))
		}
		engine.Commit(otherBlock, []types.WitenssVote{})
		eventSub.Unsubscribe()
	}
	go eventLoop()
	seal := func() {
		engine.Seal(chain, block, nil)
		t.Error("seal should not be completed")
	}
	go seal()

	const timeoutDura = 2 * time.Second
	timeout := time.NewTimer(timeoutDura)
	<-timeout.C
	// wait 2 seconds to ensure we cannot get any blocks from Istanbul
}

func TestSealCommitted(t *testing.T) {
	chain, engine := newBlockChain(1)
	block := makeBlockWithoutSeal(chain, engine, chain.Genesis())
	//expectedBlock, _ := engine.updateBlock(engine.chain.GetHeader(block.ParentHash(), block.Number()-1), block)

	finalBlock, err := engine.Seal(chain, block, nil)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
	if finalBlock.Hash() != block.Hash() {
		t.Errorf("hash mismatch: have %v, want %v", finalBlock.Hash(), block.Hash())
	}
}

func TestVerifyHeader(t *testing.T) {
	chain, engine := newBlockChain(1)

	// errEmptyCommittedSeals case
	block := makeBlockWithoutSeal(chain, engine, chain.Genesis())
	//block, _ = engine.updateBlock(chain.Genesis().Header(), block)
	err := engine.VerifyHeader(chain, block, false)
	if err != errEmptyCommittedSeals {
		t.Errorf("error mismatch: have %v, want %v", err, errEmptyCommittedSeals)
	}

	// short extra data
	header := block.Header()
	//header.Extra = []byte{}
	err = engine.VerifyHeader(chain, block, false)
	if err != errInvalidExtraDataFormat {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidExtraDataFormat)
	}
	// incorrect extra format
	//header.Extra = []byte("0000000000000000000000000000000012300000000000000000000000000000000000000000000000000000000000000000")
	err = engine.VerifyHeader(chain, block, false)
	if err != errInvalidExtraDataFormat {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidExtraDataFormat)
	}

	// non zero MixDigest
	block = makeBlockWithoutSeal(chain, engine, chain.Genesis())
	header = block.Header()
	//header.MixDigest = common.StringToHash("123456789")
	err = engine.VerifyHeader(chain, block, false)
	if err != errInvalidMixDigest {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidMixDigest)
	}

	// invalid uncles hash
	block = makeBlockWithoutSeal(chain, engine, chain.Genesis())
	header = block.Header()
	//header.UncleHash = common.StringToHash("123456789")
	err = engine.VerifyHeader(chain, block, false)
	if err != errInvalidUncleHash {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidUncleHash)
	}

	// invalid difficulty
	block = makeBlockWithoutSeal(chain, engine, chain.Genesis())
	header = block.Header()
	//header.Difficulty = big.NewInt(2)
	err = engine.VerifyHeader(chain, block, false)
	if err != errInvalidDifficulty {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidDifficulty)
	}

	// invalid timestamp
	block = makeBlockWithoutSeal(chain, engine, chain.Genesis())
	header = block.Header()
	header.Timestamp = chain.Genesis().Timestamp() + engine.config.BlockPeriod - 1
	err = engine.VerifyHeader(chain, block, false)
	if err != errInvalidTimestamp {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidTimestamp)
	}

	// future block
	block = makeBlockWithoutSeal(chain, engine, chain.Genesis())
	header = block.Header()
	header.Timestamp = uint64(time.Now().Unix() + 10)
	err = engine.VerifyHeader(chain, block, false)
	if err != consensus.ErrFutureBlock {
		t.Errorf("error mismatch: have %v, want %v", err, consensus.ErrFutureBlock)
	}

	// invalid nonce
	block = makeBlockWithoutSeal(chain, engine, chain.Genesis())
	header = block.Header()
	//copy(header.Nonce[:], hexutil.MustDecode("0x111111111111"))
	//header.Number = uint64(engine.config.Epoch)
	err = engine.VerifyHeader(chain, block, false)
	if err != errInvalidNonce {
		t.Errorf("error mismatch: have %v, want %v", err, errInvalidNonce)
	}
}

func TestVerifySeal(t *testing.T) {
	chain, engine := newBlockChain(1)
	genesis := chain.Genesis()
	// cannot verify genesis
	err := engine.VerifySeal(chain, genesis)
	if err != errUnknownBlock {
		t.Errorf("error mismatch: have %v, want %v", err, errUnknownBlock)
	}

	block := makeBlock(chain, engine, genesis)
	// change block content
	header := block.Header()
	header.Number = 4
	block1 := types.NewUnit(header, nil)
	err = engine.VerifySeal(chain, block1)
	if err != errUnauthorized {
		t.Errorf("error mismatch: have %v, want %v", err, errUnauthorized)
	}

	// unauthorized users but still can get correct signer address
	//engine.privateKey, _ = crypto.GenerateKey()
	err = engine.VerifySeal(chain, block)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}
}

func TestVerifyHeaders(t *testing.T) {
	chain, engine := newBlockChain(1)
	genesis := chain.Genesis()

	// success case
	headers := []*types.Header{}
	blocks := []*types.Unit{}
	size := 100

	for i := 0; i < size; i++ {
		var b *types.Unit
		if i == 0 {
			b = makeBlockWithoutSeal(chain, engine, genesis)
			//b, _ = engine.updateBlock(genesis.Header(), b)
		} else {
			b = makeBlockWithoutSeal(chain, engine, blocks[i-1])
			//b, _ = engine.updateBlock(blocks[i-1].Header(), b)
		}
		blocks = append(blocks, b)
		headers = append(headers, blocks[i].Header())
	}
	now = func() time.Time {
		return time.Unix(int64(headers[size-1].Timestamp), 0)
	}
	_, results := engine.VerifyHeaders(chain, blocks, nil)
	const timeoutDura = 2 * time.Second
	timeout := time.NewTimer(timeoutDura)
	index := 0
OUT1:
	for {
		select {
		case err := <-results:
			if err != nil {
				if err != errEmptyCommittedSeals && err != errInvalidCommittedSeals {
					t.Errorf("error mismatch: have %v, want errEmptyCommittedSeals|errInvalidCommittedSeals", err)
					break OUT1
				}
			}
			index++
			if index == size {
				break OUT1
			}
		case <-timeout.C:
			break OUT1
		}
	}
	// abort cases
	abort, results := engine.VerifyHeaders(chain, blocks, nil)
	timeout = time.NewTimer(timeoutDura)
	index = 0
OUT2:
	for {
		select {
		case err := <-results:
			if err != nil {
				if err != errEmptyCommittedSeals && err != errInvalidCommittedSeals {
					t.Errorf("error mismatch: have %v, want errEmptyCommittedSeals|errInvalidCommittedSeals", err)
					break OUT2
				}
			}
			index++
			if index == 5 {
				abort <- struct{}{}
			}
			if index >= size {
				t.Errorf("verifyheaders should be aborted")
				break OUT2
			}
		case <-timeout.C:
			break OUT2
		}
	}
	// error header cases
	headers[2].Number = 100
	abort, results = engine.VerifyHeaders(chain, blocks, nil)
	timeout = time.NewTimer(timeoutDura)
	index = 0
	errors := 0
	expectedErrors := 2
OUT3:
	for {
		select {
		case err := <-results:
			if err != nil {
				if err != errEmptyCommittedSeals && err != errInvalidCommittedSeals {
					errors++
				}
			}
			index++
			if index == size {
				if errors != expectedErrors {
					t.Errorf("error mismatch: have %v, want %v", err, expectedErrors)
				}
				break OUT3
			}
		case <-timeout.C:
			break OUT3
		}
	}
}

type ChainReadHelper struct {
	getStableHash   func(chain common.Address, number uint64) common.Hash
	getVoteMsg      func(uhash common.Hash) (types.Votes, error)
	getHeaderByHash func(hash common.Hash) *types.Header
	getBrothers     func(addr common.Address, number uint64) ([]common.Hash, error)
	config          *params.ChainConfig
	stateAt         func(chain common.Address, root common.Hash) (*state.StateDB, error)
}

func (ch *ChainReadHelper) Config() *params.ChainConfig {
	return ch.config
}

func (ch *ChainReadHelper) GetHeader(mcaddr common.Address, hash common.Hash, number uint64) *types.Header {
	return nil
}

func (ch *ChainReadHelper) GetHeaderByNumber(mcaddr common.Address, number uint64) *types.Header {
	return nil
}

func (ch *ChainReadHelper) GetHeaderByHash(hash common.Hash) *types.Header {
	if ch.getHeaderByHash != nil {
		return ch.getHeaderByHash(hash)
	}
	return nil
}

func (ch *ChainReadHelper) GetUnit(mcaddr common.Address, hash common.Hash, number uint64) *types.Unit {
	return nil
}

func (ch *ChainReadHelper) GetChainWitnessLib(address common.Address, scHash ...common.Hash) ([]common.Address, error) {
	return nil, nil
}

func (ch *ChainReadHelper) GetBrothers(addr common.Address, number uint64) ([]common.Hash, error) {
	if ch.getBrothers != nil {
		return ch.getBrothers(addr, number)
	}
	return nil, nil
}

func (ch *ChainReadHelper) GetVoteMsg(uhash common.Hash) (types.Votes, error) {
	if ch.getVoteMsg != nil {
		return ch.getVoteMsg(uhash)
	}
	return nil, nil
}
func (ch *ChainReadHelper) GetVoteMsgAddresses(hash common.Hash) []common.Address {
	return nil
}

func (ch *ChainReadHelper) GetChainTailState(mcaddr common.Address) (*state.StateDB, error) {
	return nil, nil
}
func (ch *ChainReadHelper) StateAt(chain common.Address, root common.Hash) (*state.StateDB, error) {
	if ch.stateAt != nil {
		return ch.stateAt(chain, root)
	}
	return nil, errors.New("not yet implement")
}

func (ch *ChainReadHelper) GetStableHash(chain common.Address, number uint64) common.Hash {
	if ch.getStableHash != nil {
		return ch.getStableHash(chain, number)
	}
	return common.Hash{}
}
func (ch *ChainReadHelper) GenesisHash() common.Hash {
	return common.Hash{}
}
func (ch *ChainReadHelper) GetChainTailHash(mc common.Address) common.Hash {
	return common.Hash{}
}
func (ch *ChainReadHelper) InsertChain(chain types.Blocks) (int, error) {
	return 0, nil
}
func (ch *ChainReadHelper) VerifyUnit(unit *types.Unit, verifyHeader func(*types.Unit) error) error {
	return nil
}

func TestGasLimit(t *testing.T) {
	priKey, _ := crypto.HexToECDSA("86e38b31d6de78971874ced73d96852168e807d3d1fa90744672413bb596dc46")
	proposer := crypto.ForceParsePubKeyToAddress(priKey.PublicKey)
	parentHeader := types.Header{
		MC:         common.StringToAddress("testc1"),
		Number:     100,
		ParentHash: common.StringToHash("parentHash"),
	}
	scHeader := types.Header{
		MC:         params.SCAccount,
		Number:     2,
		ParentHash: common.StringToHash("001"),
	}
	mdb, _ := ntsdb.NewMemDatabase()
	db, _ := state.New([]byte{1, 2, 3}, common.Hash{}, state.NewDatabase(mdb))

	chain := new(ChainReadHelper)
	chain.config = params.MainnetChainConfig

	chain.getHeaderByHash = func(hash common.Hash) *types.Header {
		if hash == scHeader.Hash() {
			return &scHeader
		}
		if hash == parentHeader.Hash() {
			return &parentHeader
		}
		return nil
	}
	chain.stateAt = func(root common.Hash) (*state.StateDB, error) {
		return db, nil
	}

	signThat := func(t *testing.T, sh types.SignHelper) {
		signer := types.NewSigner(chain.config.ChainId)
		err := types.SignBySignHelper(sh, signer, priKey)
		require.NoError(t, err)
	}

	config := istanbul.DefaultConfig
	engine, _ := New(&config, new(event.TypeMux), priKey, nil, mdb).(*backend)

	t.Run("invalid gas used", func(t *testing.T) {
		//测试 used gas 超过上限
		t.Run("over", func(t *testing.T) {
			header := types.Header{
				Proposer:   proposer,
				MC:         common.StringToAddress("testc1"),
				Number:     parentHeader.Number + 1,
				ParentHash: parentHeader.Hash(),
				Timestamp:  parentHeader.Timestamp + 1,
				SCNumber:   scHeader.Number,
				SCHash:     scHeader.Hash(),

				GasLimit: 1000,
				GasUsed:  10001,
			}
			unit := types.NewUnit(&header, nil)
			signThat(t, unit)

			err := engine.VerifyHeader(chain, unit, false)
			require.Error(t, err)
			require.Equal(t, fmt.Errorf("invalid gasUsed: have %d, gasLimit %d", header.GasUsed, header.GasLimit).Error(), err.Error())
		})
		//测试 used gas 低于下限
		t.Run("<min", func(t *testing.T) {
			header := types.Header{
				Proposer:   proposer,
				MC:         common.StringToAddress("testc1"),
				Number:     parentHeader.Number + 1,
				ParentHash: parentHeader.Hash(),
				Timestamp:  parentHeader.Timestamp + 1,
				SCNumber:   scHeader.Number,
				SCHash:     scHeader.Hash(),

				GasLimit: params.MinGasLimit,
				GasUsed:  params.MinGasLimit - 1,
			}
			unit := types.NewUnit(&header, nil)
			signThat(t, unit)

			err := engine.VerifyHeader(chain, unit, false)
			require.Error(t, err)
			require.Equal(t, fmt.Errorf("invalid gasLimit: have %d, want >= %d", header.GasUsed, params.MinGasLimit).Error(), err.Error())
		})
	})
	t.Run("invalid gas limit", func(t *testing.T) {
		//测试GasLimit不能超过最大上限
		t.Run("over max", func(t *testing.T) {
			header := types.Header{
				Proposer:   proposer,
				MC:         common.StringToAddress("testc1"),
				Number:     parentHeader.Number + 1,
				ParentHash: parentHeader.Hash(),
				Timestamp:  parentHeader.Timestamp + 1,
				SCNumber:   scHeader.Number,
				SCHash:     scHeader.Hash(),

				GasLimit: defGasCap + 1,
				GasUsed:  10001,
			}
			unit := types.NewUnit(&header, nil)
			signThat(t, unit)

			err := engine.VerifyHeader(chain, unit, false)
			require.Error(t, err)
			require.Equal(t, fmt.Errorf("invalid gasLimit: have %d, max %d", header.GasLimit, defGasCap).Error(), err.Error())
		})
		//测试GasLimit不能超过共识中的配置
		t.Run("over config", func(t *testing.T) {
			header := types.Header{
				Proposer:   proposer,
				MC:         common.StringToAddress("testc1"),
				Number:     parentHeader.Number + 1,
				ParentHash: parentHeader.Hash(),
				Timestamp:  parentHeader.Timestamp + 1,
				SCNumber:   scHeader.Number,
				SCHash:     scHeader.Hash(),

				GasLimit: params.MinGasLimit * 2,
				GasUsed:  params.MinGasLimit * 2,
			}
			unit := types.NewUnit(&header, nil)
			signThat(t, unit)
			limit := header.GasLimit - 1

			sc.WriteConfig(db, params.ConfigParamsUcGasLimit, header.GasLimit-1)
			err := engine.VerifyHeader(chain, unit, false)
			require.Error(t, err)
			require.Equal(t, fmt.Errorf("invalid gasLimit: have %d, want <= %d", header.GasLimit, limit).Error(), err.Error())
		})
	})
}

func TestBackend_VerifyHeader(t *testing.T) {
	priKey, _ := crypto.HexToECDSA("86e38b31d6de78971874ced73d96852168e807d3d1fa90744672413bb596dc46")
	proposer := crypto.ForceParsePubKeyToAddress(priKey.PublicKey)

	scHeader1 := types.Header{
		MC:         params.SCAccount,
		Number:     1,
		ParentHash: common.StringToHash("001"),
	}
	scHeader2 := types.Header{
		MC:         params.SCAccount,
		Number:     2,
		ParentHash: scHeader1.Hash(),
	}
	parentHeader := types.Header{
		MC:         common.StringToAddress("testc1"),
		Number:     100,
		ParentHash: common.StringToHash("parentHash"),

		SCNumber: scHeader2.Number,
		SCHash:   scHeader2.Hash(),
	}
	mdb, _ := ntsdb.NewMemDatabase()
	db, _ := state.New([]byte{1, 2, 3}, common.Hash{}, state.NewDatabase(mdb))

	chain := new(ChainReadHelper)
	chain.config = params.MainnetChainConfig
	sc.SetupChainConfig(db, chain.config)

	chain.getHeaderByHash = func(hash common.Hash) *types.Header {
		if hash == scHeader1.Hash() {
			return &scHeader1
		}
		if hash == scHeader2.Hash() {
			return &scHeader2
		}
		if hash == parentHeader.Hash() {
			return &parentHeader
		}
		return nil
	}
	chain.stateAt = func(root common.Hash) (*state.StateDB, error) {
		return db, nil
	}

	signThat := func(t *testing.T, sh types.SignHelper) {
		signer := types.NewSigner(chain.config.ChainId)
		err := types.SignBySignHelper(sh, signer, priKey)
		require.NoError(t, err)
	}

	config := istanbul.DefaultConfig
	engine, _ := New(&config, new(event.TypeMux), priKey, nil, mdb).(*backend)

	t.Run("check sc number", func(t *testing.T) {
		// 测试交叉：其引用的系统链高度小于父单元的应用
		t.Run("less than parent.sc", func(t *testing.T) {
			fakeSCHash := scHeader1.Hash()
			fakeSCNumber := scHeader1.Number

			header := types.Header{
				Proposer:   proposer,
				MC:         common.StringToAddress("testc1"),
				Number:     parentHeader.Number + 1,
				ParentHash: parentHeader.Hash(),
				Timestamp:  parentHeader.Timestamp + 1,

				SCNumber: fakeSCNumber,
				SCHash:   fakeSCHash,

				GasLimit: params.MinGasLimit + 1,
				GasUsed:  params.MinGasLimit + 1,
			}
			unit := types.NewUnit(&header, nil)
			signThat(t, unit)

			err := engine.VerifyHeader(chain, unit, false)
			require.Error(t, err)
			require.EqualError(t, err, fmt.Errorf("invalid system number: have %d,want >=%d", header.SCNumber, parentHeader.SCNumber).Error())
		})
	})
	// 测试：和父单元相同，则允许
	t.Run("equal parent.sc", func(t *testing.T) {
		header := types.Header{
			Proposer:   proposer,
			MC:         common.StringToAddress("testc1"),
			Number:     parentHeader.Number + 1,
			ParentHash: parentHeader.Hash(),
			Timestamp:  parentHeader.Timestamp + 1,

			SCNumber: parentHeader.SCNumber,
			SCHash:   parentHeader.SCHash,

			GasLimit: params.MinGasLimit + 1,
			GasUsed:  params.MinGasLimit + 1,
		}
		unit := types.NewUnit(&header, nil)
		signThat(t, unit)

		err := engine.VerifyHeader(chain, unit, false)
		require.NoError(t, err)
	})
}
