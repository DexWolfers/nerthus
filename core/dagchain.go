package core

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/common/mclock"
	"gitee.com/nerthus/nerthus/common/set"
	"gitee.com/nerthus/nerthus/common/sync/cmap"
	"gitee.com/nerthus/nerthus/common/sync/lfreequeue"
	"gitee.com/nerthus/nerthus/common/threadpool"
	"gitee.com/nerthus/nerthus/common/tracetime"
	"gitee.com/nerthus/nerthus/consensus"
	"gitee.com/nerthus/nerthus/consensus/arbitration"
	"gitee.com/nerthus/nerthus/consensus/bft"
	cfg "gitee.com/nerthus/nerthus/core/config"
	"gitee.com/nerthus/nerthus/core/rawdb"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/statistics"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"

	"github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
)

var (
	ErrNoGenesis       = errors.New("genesis not found in chain")
	ErrExistStableUnit = consensus.ErrKnownBlock
	ErrInInArbitration = consensus.ErrInInArbitration
)

const (
	bodyCacheLimit     = 2048 //单元Body缓存
	blockCacheLimit    = 2048 //单元缓存
	unitTxCacheLimit   = 2048 //热交易缓存
	headCacheLimit     = 2048 //单元头缓存
	badUnitLimit       = 5
	maxTimeFutureUnits = 30 // 秒
)

type DagchainOption func(dag *DagChain)

func SetBftEngine(backEngine consensus.Engine) DagchainOption {
	return func(dag *DagChain) {
		dag.bftEngine = backEngine
	}
}

// DagChain represents the canonical chain given a database with a genesis
// unit. The Unit Chain manages chain imports, reverts, chain reorganisations.
//
// Importing blocks in to the unit chain happens according to the set of rules
// defined by the two stage Validator. Processing of blocks is done using the
// Processor which processes the included transaction. The validation of the state
// is done in the second part of the Validator. Failing results in aborting of
// the import.
//
// The DagChain also helps in returning blocks from **any** chain included
// in the database as well as blocks that represents the canonical chain. It's
// important to note that GetUnit can return any unit and does not need to be
// included in the canonical one where as GetUnitByNumber always represents the
// canonical chain.
type DagChain struct {
	config *params.ChainConfig // chain & network configuration

	chainDB                         ntsdb.Database
	sta                             *statistics.Statistic
	dataSta                         *ChainDataStatic
	dag                             *DAG
	rmTxFeed                        event.Feed
	rmLogsFeed                      event.Feed
	chainFeed                       event.Feed
	unitStatusFeed                  event.Feed
	chainSideFeed                   event.Feed
	chainBranchChangedFeed          event.Feed
	chainHeadFeed                   event.Feed
	chainVoteFeed                   event.Feed
	reportUnitFeed                  event.Feed // 举报作恶
	reportUnitResultFeed            event.Feed // 举报结果
	mcUnitsProcessStatusChangedFeed event.Feed // 某链被停止处理
	witnessReplaceFeed              event.Feed // 更换见证人
	witnessReplaceVoteFeed          event.Feed // 更换见证人投票
	logsFeed                        event.Feed
	scope                           event.SubscriptionScope
	genesisUnit                     *types.Unit
	settleCalc                      *sc.SettleCalc

	mu      sync.RWMutex       // global mutex for locking chain operations
	chainRW cmap.ConcurrentMap // blockchain insertion lock
	procmu  sync.RWMutex       // unit processor lock

	checkpoint int // checkpoint counts towards the new checkpoint

	stateCache           state.Database     // State database to reuse between imports (contains state cache)
	bodyCache            *lru.Cache         // Cache for the most recent unit bodies
	blockCache           *lru.Cache         // Cache for the most recent entire blocks
	badUnitHashs         *set.StringSet     // Bad unit hash cache, key=unit hash,value =struct{}{}
	unitTxCache          *lru.Cache         //缓存已稳定单元中的交易
	unitHeadCache        *lru.Cache         //缓存已稳定单元的头
	unitInserting        cmap.ConcurrentMap //正在进行
	chainTailCache       *lru.Cache         //缓存链信息
	graphSync            RealTimeSync
	goodUnitq            *lfreequeue.Queue  //合法单元待落地队列
	goodStateCache       *lru.Cache         //缓存state
	pendingUnits         cmap.ConcurrentMap //合法单元待落地数据暂存
	writtenInUnitLimit   int64
	enableAsyncWriteBody bool
	configCache          cmap.ConcurrentMap

	quit    chan struct{} // blockchain quit channel
	running int32         // running must be called atomically
	// procInterrupt must be atomically called
	procInterrupt int32          // interrupt signaler for unit processing
	wg            sync.WaitGroup // chain processing wait group for shutting down
	threadPool    threadpool.ThreadPool

	bftEngine consensus.Engine
	processor Processor // unit processor interface
	validator Validator // unit and state validator interface
	vmConfig  vm.Config

	testPopUnit        func(v interface{})
	testDisableVote    bool //单元测试时是否禁用投票统计
	verifyWithProposal func(u *types.Unit) (*bft.MiningResult, bool)
}

func newCache(size int) *lru.Cache {
	c, err := lru.New(size)
	if err != nil {
		panic(err)
	}
	return c
}

// NewDagChain returns a fully initialised unit chain using information
// available in the database. It initialises the default Ethereum Validator and
// Processor.
func NewDagChain(chainDB ntsdb.Database, config *params.ChainConfig, engine consensus.Engine, vmConfig vm.Config, opts ...DagchainOption) (*DagChain, error) {
	dc := &DagChain{
		config:               config,
		vmConfig:             vmConfig,
		chainDB:              chainDB,
		stateCache:           state.NewDatabase(chainDB),
		quit:                 make(chan struct{}),
		bodyCache:            newCache(bodyCacheLimit),
		blockCache:           newCache(blockCacheLimit),
		badUnitHashs:         set.NewStrSet(64, badUnitLimit),
		unitTxCache:          newCache(unitTxCacheLimit),
		unitHeadCache:        newCache(headCacheLimit),
		unitInserting:        cmap.NewWith(128),
		chainRW:              cmap.NewWith(1024),
		chainTailCache:       newCache(1024),
		goodStateCache:       newCache(blockCacheLimit),
		goodUnitq:            lfreequeue.NewQueue(),
		pendingUnits:         cmap.NewWith(64),
		configCache:          cmap.NewWith(64),
		writtenInUnitLimit:   int64(cfg.MustInt("runtime.db_written_in_units", 10000)),
		enableAsyncWriteBody: cfg.MustBool("runtime.memory_database_enable", false),
	}
	dc.SetValidator(NewUnitValidator(config, dc, engine))
	dc.SetProcessor(NewStateProcessor(config, dc, engine))
	if dag, err := NewDag(chainDB, nil, dc); err != nil {
		return nil, err
	} else {
		dc.SetDag(dag)
	}

	dc.graphSync = NewRealtimeSyncV2(dc)
	dc.genesisUnit = dc.GetUnitByNumber(params.SCAccount, 0)
	if dc.genesisUnit == nil {
		return nil, ErrNoGenesis
	}

	dc.dataSta = NewChainDataStatic(dc)
	dc.settleCalc = sc.NewSettleSer(chainDB, dc)

	for _, opt := range opts {
		opt(dc)
	}

	//必须首先完成数据完整性检查
	dc.checkData()

	if err := dc.settleCalc.Start(); err != nil {
		return nil, err
	}

	go dc.dataSta.Start()
	if dc.enableAsyncWriteBody {
		//异步是才开启
		dc.threadPool = threadpool.NewBaseTreadPoll(1024, 64)
		<-dc.threadPool.Start()

		go dc.goodUnitLookup()
	}
	return dc, nil
}

func (dc *DagChain) getProcInterrupt() bool {
	return atomic.LoadInt32(&dc.procInterrupt) == 1
}

func (dc *DagChain) Config() *params.ChainConfig {
	return dc.config
}
func (dc *DagChain) Engine() consensus.Engine {
	return dc.bftEngine
}

func (dc *DagChain) DagReader() interface{} {
	return dc.dag
}
func (dc *DagChain) SetDag(dag *DAG) {
	dc.dag = dag
}
func (dc *DagChain) SetSta(sta *statistics.Statistic) {
	dc.sta = sta
}

func (dc *DagChain) SetOptions(opts ...DagchainOption) {
	for _, opt := range opts {
		opt(dc)
	}
}

// SetHead rewinds the local chain to a new head. In the case of headers, everything
// above the new head will be deleted and the new one set. In the case of blocks
// though, the head may be further rewound if unit bodies are missing (non-archive
// nodes after a fast sync).
func (dc *DagChain) SetHead(head uint64) error {
	log.Warn("Rewinding blockchain", "target", head)

	dc.mu.Lock()
	defer dc.mu.Unlock()

	// Clear out any stale content from the caches
	dc.bodyCache.Purge()
	dc.blockCache.Purge()
	//dc.futureUnits.Purge()

	return nil
}
func (dc *DagChain) SetProposalVerify(f func(*types.Unit) (*bft.MiningResult, bool)) {
	dc.verifyWithProposal = f
}

// SetProcessor sets the processor required for making state modifications.
func (dc *DagChain) SetProcessor(processor Processor) {
	dc.procmu.Lock()
	defer dc.procmu.Unlock()
	dc.processor = processor
}

// SetValidator sets the validator which is used to validate incoming blocks.
func (dc *DagChain) SetValidator(validator Validator) {
	dc.procmu.Lock()
	defer dc.procmu.Unlock()
	dc.validator = validator
}

// Validator returns the current validator.
func (dc *DagChain) Validator() Validator {
	//dc.procmu.RLock()
	//defer dc.procmu.RUnlock()
	return dc.validator
}

// Processor returns the current processor.
func (dc *DagChain) Processor() Processor {
	//dc.procmu.RLock()
	//defer dc.procmu.RUnlock()
	return dc.processor
}

// GetChainTailState 返回账户链的当前状态
func (dc *DagChain) GetChainTailState(chain common.Address) (*state.StateDB, error) {
	return dc.GetChainTailInfo(chain).State(dc.stateCache), nil
}

// StateAt 返回账户链特定点的状态
func (dc *DagChain) StateAt(chain common.Address, root common.Hash) (*state.StateDB, error) {
	// state缓存
	if v, ok := dc.goodStateCache.Get(root.Hex()); ok {
		return v.(*state.StateDB).Copy(), nil
	}
	return state.New(chain.Bytes(), root, dc.stateCache)
}

func (dc *DagChain) ReadConfigValue(scHeader *types.Header, key common.Hash) (value common.Hash, err error) {
	ck := scHeader.StateRoot.Hex() + "_" + key.Hex()
	v, _ := dc.configCache.LoadOrStore(ck, func() interface{} {
		var sdb *state.StateDB
		sdb, err = dc.GetStateDB(scHeader.MC, scHeader.StateRoot)
		if err != nil {
			return nil
		}
		return sdb.GetState(params.SCAccount, key)
	})
	if v == nil {
		dc.configCache.Remove(ck)
		return
	}
	return v.(common.Hash), err
}

// GetStateDB 获取账户链stateDB
func (dc *DagChain) GetStateDB(chain common.Address, root common.Hash) (*state.StateDB, error) {
	//return state.New(root, dc.stateCache)
	if v, ok := dc.goodStateCache.Get(root.Hex()); ok {
		return v.(*state.StateDB).Copy(), nil
	}
	if ste, err := state.New(chain.Bytes(), root, dc.stateCache); err != nil {
		return nil, err
	} else {
		//cache it,copy is not necessary
		dc.goodStateCache.Add(root.Hex(), ste)
		return ste, nil
	}
}

// GetUnitState 返回单元stateDB
func (dc *DagChain) GetUnitState(unitHash common.Hash) (*state.StateDB, error) {
	defer tracetime.New().Stop()
	header := dc.GetHeaderByHash(unitHash)
	if header == nil {
		return nil, ErrUnitNotFind
	}
	return dc.StateAt(header.MC, header.StateRoot)
}
func (dc *DagChain) GetStateByNumber(chain common.Address, number uint64) (*state.StateDB, error) {
	uhash := GetStableHash(dc.chainDB, chain, number)
	if uhash.Empty() {
		return nil, errors.New("missing unit hash")
	}
	return dc.GetUnitState(uhash)
}

// GetTransaction 获取交易
func (dc *DagChain) GetTransaction(txHash common.Hash) *types.Transaction {
	defer tracetime.New().Stop()
	if tx, ok := dc.unitTxCache.Get(txHash); ok {
		return tx.(*types.Transaction)
	}
	tx := GetTransaction(dc.chainDB, txHash)
	if tx != nil {
		dc.unitTxCache.Add(txHash, tx)
	}
	return tx
}

// QueryOriginTx 根据交易执行结构获取原始交易
func (dc *DagChain) QueryOriginTx(txexec *types.TransactionExec) (*types.Transaction, error) {
	defer tracetime.New().Stop()
	if txexec.Tx != nil {
		return txexec.Tx, nil
	}
	tx := dc.GetTransaction(txexec.TxHash)
	if tx == nil {
		return nil, fmt.Errorf("missing tx %s", txexec.TxHash.String())
	}
	return tx, nil
}

// Genesis retrieves the chain's genesis unit.
func (dc *DagChain) Genesis() *types.Unit {
	return dc.genesisUnit
}

// GenesisHash 获取创世单元Hash
func (dc *DagChain) GenesisHash() common.Hash {
	if g := dc.Genesis(); g == nil {
		return common.Hash{}
	}
	return dc.Genesis().Hash()
}

func (dc *DagChain) GetSyncGraph(nodeName common.Hash) string {
	return dc.graphSync.SyncGraph(nodeName)
}

func (dc *DagChain) GetSyncZeroNodes() []common.Hash {
	return dc.graphSync.ZeroNodes()
}

// GetChainTailHead 获取链的尾部单元头
// TODO(ysqi),需要同时返回UnitHash,以减少重复哈希计算
func (dc *DagChain) GetChainTailHead(chain common.Address) *types.Header {
	return dc.GetChainTailInfo(chain).TailHead()
}

// GetChainTailNumber 获取链的尾部单元高度
func (dc *DagChain) GetChainTailNumber(chain common.Address) uint64 {
	return dc.GetChainTailInfo(chain).TailHead().Number
}

func (dc *DagChain) updateChainTailInfo(chain common.Address,
	tailState *state.StateDB, u *types.Unit) *ChainCache {
	var info *ChainCache
get:
	if v, ok := dc.chainTailCache.Get(chain.Text()); ok {
		info = v.(*ChainCache)
		info.UpTail(tailState, u.Hash(), u)
	} else {
		info = NewChainCacheWithState(chain, tailState, u)
		info.SwitchCacheUnit(dc.enableAsyncWriteBody || chain == params.SCAccount, dc)
		if exist, _ := dc.chainTailCache.ContainsOrAdd(chain.Text(), info); exist {
			goto get
		}
	}
	return info
}

// GetChainTailInfo获取链尾部信息
func (dc *DagChain) GetChainTailInfo(chain common.Address) *ChainCache {
	f := func() *ChainCache {
		var (
			hash common.Hash
			tail *types.Unit
		)
		hash = GetChainTailHash(dc.chainDB, chain)
		if hash.Empty() {
			hash = dc.GenesisHash()
			tail = dc.genesisUnit
		} else {
			tail = dc.GetUnitByHash(hash)
			if tail == nil {
				log.Crit("can not get unit by hash", "chain", chain, "uhash", hash)
			}
		}
		cache, err := NewChainCache(chain, dc.stateCache, hash, tail)
		if err != nil { //如果无State，则错误
			log.Crit("can not load chain current state db", "chain", chain, "number", tail.Number(), "err", err)
		}
		cache.SwitchCacheUnit(dc.enableAsyncWriteBody || chain == params.SCAccount, dc)
		return cache
	}
	var info *ChainCache
get:
	if v, ok := dc.chainTailCache.Get(chain.Text()); ok {
		info = v.(*ChainCache)
	} else {
		info = f()
		if exist, _ := dc.chainTailCache.ContainsOrAdd(chain.Text(), info); exist {
			goto get
		}
	}
	return info
}

// GetChainTailHash 获取链的尾部单元哈希
func (dc *DagChain) GetChainTailHash(chain common.Address) common.Hash {
	return dc.GetChainTailInfo(chain).TailHash()
}

// GetChainTailUnit 获取链尾部单元
func (dc *DagChain) GetChainTailUnit(chain common.Address) *types.Unit {
	return dc.GetChainTailInfo(chain).TailUnit()
}

// GetUnitNumber 根据单元hash获取链和单元位置
func (dc *DagChain) GetUnitNumber(hash common.Hash) (common.Address, uint64) {
	defer tracetime.New().Stop()
	//先从头缓存中获取
	if head := dc.getHeadFromCache(hash); head != nil {
		return head.MC, head.Number
	}
	return GetUnitNumber(dc.chainDB, hash)
}

// GetHeader 根据单元hash获取单元头
func (dc *DagChain) GetHeader(hash common.Hash) *types.Header {
	return dc.GetHeaderByHash(hash)
}

// getHeadFromCache 根据单元hash获取单元头
func (dc *DagChain) getHeadFromCache(uhash common.Hash) *types.Header {
	//先从头缓存中获取
	if head, ok := dc.unitHeadCache.Get(uhash); ok {
		return types.CopyHeader(head.(*types.Header))
	}
	//从单元缓存中获取
	if unit, ok := dc.blockCache.Get(uhash); ok {
		return unit.(*types.Unit).Header()
	}
	if uhash == dc.GenesisHash() {
		return dc.genesisUnit.Header()
	}
	return nil
}

// getUnitFromCache 根据单元hash获取单元
func (dc *DagChain) getUnitFromCache(uhash common.Hash) *types.Unit {
	//先从单元缓存中获取
	if unit, ok := dc.blockCache.Get(uhash); ok {
		return unit.(*types.Unit)
	}
	if unit := dc.getPendingUnit(uhash); unit != nil {
		return unit
	}
	if uhash == dc.GenesisHash() {
		return dc.genesisUnit
	}
	return nil
}

// GetHeaderByHash 通过单元hash获取单元头
func (dc *DagChain) GetHeaderByHash(hash common.Hash) *types.Header {
	if head := dc.getHeadFromCache(hash); head != nil {
		return head
	}
	header := GetHeaderByHash(dc.chainDB, hash)
	if header != nil {
		dc.unitHeadCache.Add(hash, types.CopyHeader(header))
	}
	return header
}
func (dc *DagChain) ExistUnit(uhash common.Hash) bool {
	if uhash == dc.GenesisHash() {
		return true
	}
	//先从头缓存中获取
	if dc.unitHeadCache.Contains(uhash) {
		return true
	}
	//从单元缓存中获取
	if dc.blockCache.Contains(uhash) {
		return true
	}
	return ExistUnitNumber(dc.chainDB, uhash)
}

// GetUnitByHash 根据单元Hash获取单元
func (dc *DagChain) GetUnitByHash(hash common.Hash) *types.Unit {
	defer tracetime.New().Stop()
	if unit := dc.getUnitFromCache(hash); unit != nil {
		return unit
	}
	u := dc.GetUnit(hash)
	return u
}

// GetHeaderByNumber 根据单元位置获取单元头
func (dc *DagChain) GetHeaderByNumber(mcaddr common.Address, number uint64) *types.Header {
	_, h := dc.GetHeaderAndHash(mcaddr, number)
	return h
}

// GetHeaderByNumber 根据单元位置获取单元头和单元哈希
func (dc *DagChain) GetHeaderAndHash(mcaddr common.Address, number uint64) (common.Hash, *types.Header) {
	defer tracetime.New().Stop()

	hash := GetStableHash(dc.chainDB, mcaddr, number)
	if hash.Empty() {
		return hash, nil
	}
	return hash, dc.GetHeader(hash)
}

// GetBody retrieves a unit body (transactions and uncles) from the database by
// hash, caching it if found.
func (dc *DagChain) GetBody(chain common.Address, number uint64, hash common.Hash) *types.Body {
	defer tracetime.New().Stop()

	// Short circuit if the body's already in the cache, retrieve otherwise
	if cached, ok := dc.bodyCache.Get(hash); ok {
		body := cached.(*types.Body)
		return body
	}
	if u := dc.getUnitFromCache(hash); u != nil {
		return u.Body()
	}
	body := GetBody(dc.chainDB, chain, number, hash)
	if body == nil {
		return nil
	}
	// Cache the found body for next time and return
	dc.bodyCache.Add(hash, body)
	return body
}

// HasUnitAndState 检查单元是否存在且状态在数据库中完整存在
func (dc *DagChain) HasUnitAndState(hash common.Hash) bool {
	defer tracetime.New().Stop()
	unit := dc.GetUnitByHash(hash)
	if unit == nil {
		return false
	}
	// 确保能获得正确的状态
	_, err := dc.stateCache.OpenTrie(unit.MC().Bytes(), unit.Root())
	return err == nil
}

// GetUnit retrieves a unit from the database by hash and number,
// caching it if found.
func (dc *DagChain) GetUnit(hash common.Hash) *types.Unit {
	defer tracetime.New().Stop()
	if hash.Empty() {
		return nil
	}
	if u := dc.getUnitFromCache(hash); u != nil {
		return u
	}

	unit := GetUnit(dc.chainDB, hash)
	if unit == nil {
		return nil
	}
	// Cache the found unit for next time and return
	dc.cacheUnit(unit)
	return unit
}

// GetUnitByNumber retrieves a unit from the database by number, caching it
// (associated with its hash) if found.
func (dc *DagChain) GetUnitByNumber(mcaddr common.Address, number uint64) *types.Unit {
	defer tracetime.New().Stop()
	if number == 0 {
		mcaddr = params.SCAccount
	}
	hash := GetStableHash(dc.chainDB, mcaddr, number)
	if hash == (common.Hash{}) {
		return nil
	}
	return dc.GetUnit(hash)
}

// CheckWitnessExist 检查申请人是否已存在
func (self *DagChain) ExistInWitnessLibs(target common.Address) (bool, error) {
	defer tracetime.New().Stop()
	return self.dag.ExistInWitnessLibs(target)
}

// Stop stops the blockchain service. If any imports are currently in progress
// it will abort them using the procInterrupt.
func (dc *DagChain) Stop() {
	if !atomic.CompareAndSwapInt32(&dc.running, 0, 1) {
		return
	}
	// Unsubscribe all subscriptions registered from blockchain
	dc.dataSta.Stop()
	dc.scope.Close()
	dc.settleCalc.Stop()
	close(dc.quit)
	atomic.StoreInt32(&dc.procInterrupt, 1)
	dc.wg.Wait()
	log.Info("Unit Chain manager stopped")
}

// VerifyUnit 校验单元
func (dc *DagChain) VerifyUnit(unit *types.Unit, verifyHeader func(*types.Unit) error) (receipts types.Receipts, statedb *state.StateDB, logs []*types.Log, err error) {
	tr := tracetime.New("uhash", unit.Hash(), "chain", unit.MC(), "number", unit.Number())
	defer tr.Stop()
	if dc.dag.ChainIsInInArbitration(unit.MC()) {
		err = fmt.Errorf("chain is in arbitration, chain is %s", unit.MC())
		return
	}
	tr.Tag()
	logger := log.New("Do", "verify unit")
	// 单元验证耗时计数
	//bstart := time.Now()
	logger.Debug("begin verify unit", "mc", unit.MC(), "number", unit.Number(), "hash", unit.Hash())

	// 如果已终止链处理，则也终止单元处理
	if atomic.LoadInt32(&dc.procInterrupt) == 1 {
		err = errors.New("premature abort during blocks processing")
		return
	}
	tr.Tag()

	// 如果以及是bad 则忽略处理
	if err = dc.BadUnitCheck(unit); err != nil {
		return
	}
	tr.Tag()

	// 检查该链是否存在过仲裁，如果在此高度有仲裁结果则需要检查是否相匹配，如果不匹配则忽略
	if good := dc.dag.GetArbitrationResult(unit.MC(), unit.Number()); !good.Empty() {
		if good != unit.Hash() {
			err = fmt.Errorf("this chain %s has been arbitrated and only one legal unit %s", unit.MC().Hex(), good.Hex())
			return
		}
	}
	tr.Tag() //12%,15%

	if err = verifyHeader(unit); err != nil {
		return
	}
	tr.Tag() //43%,25%,
	validator := dc.Validator()

	if err = validator.ValidateBody(unit); err != nil {
		return
	}
	tr.Tag() //11%,
	parent := dc.GetHeaderByHash(unit.ParentHash())

	statedb, err = dc.StateAt(parent.MC, parent.StateRoot)
	// 链的第一个单元需要迁移数据
	if parent.Number == 0 && unit.MC() != params.SCAccount {
		statedb, err = statedb.NewChainState(unit.MC())
	}
	tr.Tag()
	if err != nil {
		return
	}
	tr.Tag()
	// 处理器根据父单元点继续处理本单元
	var usedGas uint64
	receipts, logs, usedGas, err = dc.processor.Process(unit, statedb, dc.vmConfig)
	if err != nil {
		return
	}
	tr.Tag() //33%,55%
	// 验证状态
	err = validator.ValidateState(unit, nil, statedb, receipts, usedGas)
	if err != nil {
		return
	} //11
	return
}

// BadUnitCheck 校验是否是坏单元
func (dc *DagChain) BadUnitCheck(unit *types.Unit) error {
	// 如果以及是bad 则忽略处理
	if dc.badUnitHashs.Has(unit.Hash().Hex()) {
		return fmt.Errorf("the unit is bad unit, uhash:%v", unit.Hash().Hex())
	}
	if dc.badUnitHashs.Has(unit.ParentHash().Hex()) { // 如果其父单元属于bad,则所有子单元也将属于bad
		dc.badUnitHashs.Add(unit.Hash().Hex())
		return fmt.Errorf("the unit's parent is bad unit, uhash: %v phash:%v", unit.Hash().Hex(), unit.ParentHash().Hex())
	}
	if dc.dag.IsBadUnit(unit.Hash()) {
		return errors.New("is a known bad unit")
	}
	if dc.chainSleeping(unit.MC(), unit.Number(), unit.SCNumber(), unit.Hash()) {
		return errors.New("chain is sleeping")
	}

	return nil
}

// 判断链是否进入休眠状态
// 休眠包括：链正在更换见证人中、链正在仲裁中
func (dc *DagChain) chainSleeping(chain common.Address, number, sysNumber uint64, uhash common.Hash) bool {
	db, err := dc.GetChainTailState(params.SCAccount)
	if err != nil {
		log.Error("can not get chain tail state", "chain", params.SCAccount, "err", err)
		return true
	}
	//当  单元依赖的系统链高度 >= 链开始申请见证人的系统链高度 ,则说明暂时不能接收此单元
	chainStatus, statusNumber := sc.GetChainStatus(db, chain)
	log.Trace("chainStatus", "chain", chain, "status", chainStatus, "statusNumber", statusNumber, "sysNumber", sysNumber)
	if chainStatus == sc.ChainStatusWitnessReplaceUnderway && sysNumber >= statusNumber {
		return true
	}
	//检查是否特殊指定单元
	if sh := sc.GetChainSpecialUnit(db, chain, number); !sh.Empty() && uhash != sh {
		log.Trace("chain exist special unit", "chain", chain, "number", number, "uhash", sh)
		return true
	}

	//如果单元被 Stopped 则暂时不允许写入
	if err := arbitration.CheckChainIsStopped(dc.chainDB, chain, number); err != nil {
		return err != nil
	}
	return false
}

func (dc *DagChain) cacheSign(chain types.Blocks) (int, error) {
	var (
		bTime      = time.Now()
		wg         sync.WaitGroup
		signResult = make(chan error, 1)
		stopped    bool
	)

	asyncNoBlockChFn := func(err error) {
		stopped = true
		select {
		case signResult <- err:
		default:
		}
	}

	// current cache sign
	for _, unit := range chain {
		// 解析区块的签名
		wg.Add(1)
		err := dc.threadPool.PushTask(func(ctx context.Context, args []interface{}) {
			defer wg.Done()
			if stopped {
				return
			}
			unit := args[0].(*types.Unit)
			if _, err := unit.Sender(); err != nil {
				asyncNoBlockChFn(err)
			}
		}, []interface{}{unit})
		if err != nil {
			return 0, err
		}

		if stopped {
			break
		}

		// 解析投票的签名
		wg.Add(1)
		err = dc.threadPool.PushTask(func(ctx context.Context, args []interface{}) {
			defer wg.Done()
			if stopped {
				return
			}
			if _, err := args[0].(*types.Unit).Witnesses(); err != nil {
				signResult <- err
			}
		}, []interface{}{unit})
		if err != nil {
			return 0, err
		}

		if stopped {
			break
		}

		// 解析交易的签名
		for _, tx := range unit.Transactions() {
			if stopped {
				break
			}
			if tx.Tx == nil {
				continue
			}
			wg.Add(1)
			err = dc.threadPool.PushTask(func(ctx context.Context, args []interface{}) {
				defer wg.Done()
				if stopped {
					return
				}
				tx := args[0].(*types.Transaction)
				if err := dc.validator.ValidateTxSender(tx); err != nil {
					asyncNoBlockChFn(err)
				}
			}, []interface{}{tx.Tx})
			if err != nil {
				return 0, err
			}
		}

		if stopped {
			break
		}
	}

	wg.Wait()

	select {
	case err := <-signResult:
		if err != nil {
			return 0, err
		}
	default:
	}

	log.Trace("Parse unit all sign", "elasp", common.PrettyDuration(time.Now().Sub(bTime)))
	return 0, nil
}

// InsertChain 将尝试把给定的单元集合写入合法链中，否则将创建一个Fork版。
// 如果发生错误则返回错误信息与其处理出错的单元序号。错误描述可能是core包内所定义的各种验证错误信息。
func (dc *DagChain) InsertChain(chain types.Blocks) (int, error) {
	if len(chain) == 0 {
		return 0, nil
	}
	for i, u := range chain {
		if err := dc.InsertUnit(u); err != nil {
			if err == ErrExistStableUnit || err == consensus.ErrInsertingBlock {
				continue
			}
			return i, err
		}
	}
	return 0, nil
}

func (dc *DagChain) chainLocker(chain common.Address) *sync.RWMutex {
	r := dc.chainRW.Upsert(chain.Text(), nil, func(exist bool, valueInMap interface{}, newValue interface{}) interface{} {
		if exist {
			return valueInMap
		} else {
			return &sync.RWMutex{}
		}
	})
	return r.(*sync.RWMutex)
}

// InsertUnit 落地单元
func (dc *DagChain) InsertUnit(unit *types.Unit) error {
	id := time.Now().Nanosecond()
	uhash := unit.Hash()
	tr := tracetime.New("id", id, "uhash", uhash,
		"utime", unit.Timestamp(), "number", unit.Number(), "chain", unit.MC(),
		"proposer", unit.Proposer(), "txs", len(unit.Transactions()),
		"cost2", time.Now().Sub(time.Unix(0, int64(unit.Timestamp())))) //cost2表示单元从创建到开始落地的时间

	if len(unit.WitnessVotes()) > 0 {
		tr.AddArgs("round", binary.LittleEndian.Uint64(unit.WitnessVotes()[0].Extra[1:]))
	}
	defer tr.Stop()

	logger := log.New("action", "insertUnit", "id", id, "chain", unit.MC(), "uhash", uhash)

	beginT := mclock.Now()

	//如果最正在写入中，则退出
	if !dc.unitInserting.SetIfAbsent(uhash.Hex(), struct{}{}) {
		return consensus.ErrInsertingBlock
	}
	tr.Tag()

	defer dc.unitInserting.Remove(uhash.Hex())

	//开始校验时则直接判断单元是否存在，减少不必要的重复处理
	if dc.ExistUnit(uhash) {
		return consensus.ErrKnownBlock
	}

	tr.Tag()

	if dc.bftEngine == nil {
		return errors.New("engine is nil")
	}

	// 如果已终止链处理，则也终止单元处理
	if atomic.LoadInt32(&dc.procInterrupt) == 1 {
		return errors.New("premature abort during blocks processing")
	}
	// 如果已经被共识校验通过，则不用校验
	tr.Tag()
	if dc.verifyWithProposal != nil {
		if uresult, ok := dc.verifyWithProposal(unit); ok {
			tr.Tag()
			tr.AddArgs("more", "cached")
			return dc.WriteRightUnit(time.Now(), uresult.StateDB, unit, uresult.Receipts, uresult.Logs)
		}
	}
	// 基本检查完毕，开始完整检查
	dc.wg.Add(1)
	defer dc.wg.Done()

	var err error
	if dc.threadPool != nil {
		_, err = dc.cacheSign(types.Blocks{unit})
		if err != nil {
			return err
		}
	}

	// 遍历检查，保证其有序
	// 仲裁时不允许写入
	if dc.dag.ChainIsInInArbitration(unit.MC()) {
		return ErrInInArbitration
	}

	tr.Tag()

	// 排队处理，为提高并发与速度，在检查头后立即进入区块内容检查
	// 遍历单元集验证单元并依次写入
	// 单元验证耗时计数
	logger.Debug("begin process unit", "mc", unit.MC(), "number", unit.Number())

	// 如果以及是bad 则忽略处理
	if err := dc.BadUnitCheck(unit); err != nil {
		return err
	}

	// 检查该链是否存在过仲裁，如果在此高度有仲裁结果则需要检查是否相匹配，如果不匹配则忽略
	if good := dc.dag.GetArbitrationResult(unit.MC(), unit.Number()); !good.Empty() {
		if good != uhash {
			return fmt.Errorf("this chain %s has been arbitrated and only one legal unit %s", unit.MC().Hex(), good.Hex())
		}
	}
	tr.Tag()

	//按链进行锁定,只对校验进行锁，因为数据保存是有锁。
	// 因此锁针对的是：对链数据操作与读取
	// 校验时使用读锁，这样在存储单元时不会出现校验。可以防止双花单元被写入，也无法校验通过。
	locker := dc.chainLocker(unit.MC())
	tr.Tag()
	locker.RLock()
	tr.Tag()
	err = dc.bftEngine.VerifyHeader(dc, unit, true)
	if err == nil {
		// FIXME io
		err = dc.Validator().ValidateBody(unit)
	}
	locker.RUnlock()
	tr.Tag()
	switch {
	case err == consensus.ErrKnownBlock:
		log.Trace("unit is known", "mc", unit.MC(), "number", unit.Number(), "hash", uhash)
		return err
	case err == consensus.ErrFutureBlock:
		// 允许可超过的最大时间（秒）。如果超过限制，则丢弃处理，否则先暂存
		//max := big.NewInt(time.Now().Unix() + maxTimeFutureUnits)
		max := time.Now().Add(maxTimeFutureUnits * time.Second).UnixNano()
		if unit.Timestamp() > uint64(max) {
			return fmt.Errorf("future unit: %v > %v", unit.Timestamp(), max)
		}
		if dc.BadUnitCheck(unit) == nil {
			dc.graphSync.SyncUnit(types.RealtimeSyncEvent{Unit: unit, SyncType: types.RealtimeSyncFuture})
		}

		return err
	case err == consensus.ErrUnknownAncestor:
		log.Trace("missing ancestor", "chain", unit.MC(), "number", unit.Number(), "uhash", uhash)
		//防止堵塞，使用异步执行，但需要防止频率问题。
		//此问题在后期采用多链并发存储时解决
		dc.graphSync.SyncUnit(types.RealtimeSyncEvent{Unit: unit, SyncType: types.RealtimeSyncFuture})
		return err
	case err == consensus.ErrAlreadyExistStableUnit:
		dc.badUnitHashs.Add(uhash.Hex())
		dc.graphSync.SyncUnit(types.RealtimeSyncEvent{Unit: unit, SyncType: types.RealtimeSyncBad})
	case err != nil:
		dc.reportUnit(unit, nil, err)
		return err
	}
	tr.Tag()

	// 根据父单元状态创建新的Statedb，如果出现错误则报告
	// Parent的合法性已通过校验，直接获取即可
	parent := dc.GetHeaderByHash(unit.ParentHash())
	if parent == nil {
		log.Error("check: missing parent", "chain", unit.MC(),
			"uhash", uhash, "parent.number", unit.Number()-1, "parent.hash", unit.ParentHash())
		//防止堵塞，使用异步执行，但需要防止频率问题。
		//此问题在后期采用多链并发存储时解决
		dc.graphSync.SyncUnit(types.RealtimeSyncEvent{Unit: unit, SyncType: types.RealtimeSyncFuture})
		return consensus.ErrUnknownAncestor
	}
	tr.Tag()
	// state缓存
	receipts, db, logs, err := dc.VerifyUnit(unit, func(unit *types.Unit) error {
		return dc.Engine().VerifyHeader(dc, unit, false)
	})
	if err != nil {
		if err == consensus.ErrUnknownAncestor {
			dc.graphSync.SyncUnit(types.RealtimeSyncEvent{Unit: unit, SyncType: types.RealtimeSyncFuture})
		} else {
			dc.reportUnit(unit, receipts, err)
		}
		return err
	}

	if err = dc.writeRightUnit(db, unit, receipts, logs); err != nil {
		return err
	}
	tr.Tag()
	tr.AddArgs("status", "ok", "cost3", (mclock.Now() - beginT)) //cost3表示单元从创建到开始落地的时间
	stats := insertStats{
		startTime: beginT,
	}
	stats.report(unit)
	tr.Tag()

	return nil
}

// WriteRightUnit 直接将单元作为正确的数据保存到链中
func (dc *DagChain) WriteRightUnit(bstart time.Time, state *state.StateDB, unit *Unit, receipts types.Receipts, logs []*types.Log) error {

	st := insertStats{startTime: mclock.Now()}
	err := dc.writeRightUnit(state, unit, receipts, logs)
	if err == nil {
		st.report(unit)
	}
	return err
}

// insertStats tracks and reports on unit insertion.
type insertStats struct {
	startTime mclock.AbsTime
}

// report prints statistics if some number of blocks have been processed
// or more than a few seconds have passed since the last message.
func (st *insertStats) report(unit *types.Unit) {
	// Fetch the timings for the batch
	var (
		now     = mclock.Now()
		elapsed = time.Duration(now) - time.Duration(st.startTime)
		utime   = time.Unix(0, int64(unit.Timestamp()))
		votes   = unit.WitnessVotes()
		round   = binary.LittleEndian.Uint64(votes[0].Extra[1:])
	)
	log.Info("Imported new chain segment",
		"chain", unit.MC(), "number", unit.Number(), "uhash", unit.Hash(),
		"txs", len(unit.Transactions()),
		"elapsed", common.PrettyDuration(elapsed),
		"mgasps", float64(unit.GasUsed())*1000/float64(elapsed),
		"utime", utime.Format("2006-01-02 15:04:05"),
		"duration", common.PrettyDuration(time.Now().Sub(utime)),
		"round", round,
		"size", unit.Size(),
	)
}

// PostChainEvent 发送事件
func (dc *DagChain) PostChainEvent(events ...interface{}) {
	for _, event := range events {
		switch ev := event.(type) {
		default:
			panic(fmt.Sprintf("can not hand event, event %T", event))
		case types.ChainEvent:
			dc.chainFeed.Send(ev)
		case types.ChainVoteEvent:
			dc.chainVoteFeed.Send(ev)
		case types.ChainHeadEvent:
			dc.chainHeadFeed.Send(ev)
		case types.FoundBadWitnessEvent:
			dc.reportUnitFeed.Send(ev)

		//case types.ReportUnitResultEvent:
		//	dc.reportUnitResultFeed.Send(ev)

		case types.ChainMCProcessStatusChangedEvent:
			dc.mcUnitsProcessStatusChangedFeed.Send(ev)
		case types.RealtimeSyncEvent:
			dc.unitStatusFeed.Send(ev)
		}
	}
}

// PostChainEvents iterates over the events generated by a chain insertion and
// posts them into the event feed.
// TODO: Should not expose PostChainEvents. The chain events should be posted in WriteUnit.
func (dc *DagChain) PostChainEvents(events []interface{}, logs []*types.Log) {
	// post event logs for further processing
	if logs != nil {
		dc.logsFeed.Send(logs)
	}
	dc.PostChainEvent(events...)
}

// HasBlockAndState 检查单元和相关的状态是否完全在数据库中存在。
func (dc *DagChain) HasBlockAndState(hash common.Hash) bool {
	defer tracetime.New().Stop()
	// 获取，已检查完成存在
	header := dc.GetHeaderByHash(hash)
	if header == nil {
		return false
	}
	// 确保对应的状态已完全存在
	_, err := dc.stateCache.OpenTrie(header.MC.Bytes(), header.StateRoot)
	return err == nil
}

// BadUnits returns a list of the last 'bad blocks' that the client has seen on the network
func (dc *DagChain) BadUnits() ([]common.Hash, error) {
	defer tracetime.New().Stop()
	headers := make([]common.Hash, 0, dc.badUnitHashs.Len())
	for _, hash := range dc.badUnitHashs.List() {
		headers = append(headers, common.HexToHash(hash))
	}
	return headers, nil
}

// removeBadUnitFromFutures 循环从Futuer中将其坏单元移除，并记录到badUnits中
func (dc *DagChain) removeBadUnitFromFutures(badUnitHash common.Hash) {
	//从Future中移除，避免重复处理
	dc.badUnitHashs.Add(badUnitHash.Hex())
	dc.graphSync.SyncUnit(types.RealtimeSyncEvent{UnitHash: badUnitHash, SyncType: types.RealtimeSyncBad})
}

// addBadUnit adds a bad unit to the bad-unit LRU cache
func (dc *DagChain) addBadUnit(unit *types.Unit, reason error) {
	dc.removeBadUnitFromFutures(unit.Hash())
	sender, err2 := unit.Sender()
	if err2 != nil {
		log.Crit("failed to get sender but should not be happen", "err", err2)
	}
	dc.foundDishonest(sender, reason)
}

// 报告作恶
func (dc *DagChain) ReportBadUnit(rep []types.AffectedUnit) error {
	if len(rep) == 0 {
		return nil
	}
	//如果是作恶，则存储作恶证据
	err := arbitration.OnFoundBad(dc, dc.chainDB, dc.dag.eventMux, rep, func(db *state.StateDB, address common.Address) error {
		if status, isW := sc.GetWitnessStatus(db, address); !isW {
			return errors.New("this voter is not witness")
		} else if status != sc.WitnessNormal {
			return errors.New("this voter is not normal witness")
		}
		return nil
	})
	if err != nil {
		log.Error("failed to report bad unit", "chain", rep[0].Header.MC, "number", rep[0].Header.Number, "err", err)
	}

	return err
}

// reportUnit logs a bad unit error.
func (dc *DagChain) reportUnit(unit *types.Unit, receipts types.Receipts, err error) {
	badUnit := unit
	switch err := err.(type) {
	case *consensus.ErrWitnessesDishonest:
		rep := []types.AffectedUnit{
			{Header: *types.CopyHeader(&err.Good), Votes: err.GoodVotes},
			{Header: *types.CopyHeader(&err.Bad), Votes: err.BadVotes},
		}
		//如果是作恶，则存储作恶证据
		err2 := dc.ReportBadUnit(rep)
		if err2 != nil {
			log.Error("bad unit process failed", "err", err)
		}

		//如果作恶中认定为坏单元的为此新单元则跳过
		if err.Bad.Hash() == unit.Hash() {
			break
		}
		badUnit = dc.GetUnit(err.Bad.Hash())
		if badUnit == nil {
			return //不应该出现
		}
	}
	dc.addBadUnit(badUnit, err)
	dc.printBadUnit(badUnit, err)
}

func (dc *DagChain) printBadUnit(unit *types.Unit, err error) {
	data, _ := json.MarshalIndent(unit, "", "\t")

	log.Warn("found bad unit", "unitInfo", fmt.Sprintf(`
########## BAD UNIT #########
Chain: %d
Error: %v
UnitHash: %s
Unit:
	%s
##############################
`, dc.config.ChainId, err, unit.Hash().Hex(), string(data)))
}

// SubscribeRemovedLogsEvent registers a subscription of RemovedLogsEvent.
func (dc *DagChain) SubscribeRemovedLogsEvent(ch chan<- types.RemovedLogsEvent) event.Subscription {
	return dc.scope.Track(dc.rmLogsFeed.Subscribe(ch))
}

// SubscribeChainEvent registers a subscription of ChainEvent.
func (dc *DagChain) SubscribeChainEvent(ch chan<- types.ChainEvent) event.Subscription {
	return dc.scope.Track(dc.chainFeed.Subscribe(ch))
}
func (dc *DagChain) SubscribeUnitStatusEvent(ch chan<- types.RealtimeSyncEvent) event.Subscription {
	return dc.scope.Track(dc.unitStatusFeed.Subscribe(ch))
}

// SubscribeChainHeadEvent registers a subscription of ChainHeadEvent.
func (dc *DagChain) SubscribeChainHeadEvent(ch chan<- types.ChainHeadEvent) event.Subscription {
	return dc.scope.Track(dc.chainHeadFeed.Subscribe(ch))
}

// SubscribeLogsEvent registers a subscription of []*types.Log.
func (dc *DagChain) SubscribeLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return dc.scope.Track(dc.logsFeed.Subscribe(ch))
}

func (dc *DagChain) SubscribeReportUnitEvent(ch chan<- types.FoundBadWitnessEvent) event.Subscription {
	return dc.scope.Track(dc.reportUnitFeed.Subscribe(ch))
}

//func (dc *DagChain) SubscribeReportUnitResultEvent(ch chan<- types.ReportUnitResultEvent) event.Subscription {
//	return dc.scope.Track(dc.reportUnitResultFeed.Subscribe(ch))
//}

// SubscribeChainProcessStatusCHangedEvent 订阅某链处理状态(处理中、已停止)改变事件
func (dc *DagChain) SubscribeChainProcessStatusCHangedEvent(ch chan<- types.ChainMCProcessStatusChangedEvent) event.Subscription {
	return dc.scope.Track(dc.mcUnitsProcessStatusChangedFeed.Subscribe(ch))
}

// Get statecache
// Author: @zwj
func (dc *DagChain) StateCache() state.Database {
	return dc.stateCache
}

// Get vmConfig
// Author: @zwj
func (dc *DagChain) VMConfig() vm.Config {
	return dc.vmConfig
}

// GetWitnessInfo 获取见证人详情
func (dc *DagChain) GetWitnessInfo(address common.Address) (*sc.WitnessInfo, error) {
	defer tracetime.New().Stop()
	db, err := dc.GetChainTailState(params.SCAccount)
	if err != nil {
		return nil, err
	}
	info, err := sc.GetWitnessInfo(db, address)
	if err != nil {
		return nil, err
	}
	return info, nil
}

// GetWitnessStatus 获取见证人状态
// isApply: 是否申请过见证人 isWitness: 当前是否是见证人 isSys: 是否是系统链见证人 isMain: 是否是系统链主见证人
func (dc *DagChain) GetWitnessStatus(address common.Address) (isApply bool, isWitness bool, isSys bool, isMain bool, group uint64, err error) {
	defer tracetime.New().Stop()
	stateDB, err := dc.GetChainTailState(params.SCAccount)
	if err != nil {
		return
	}
	info, err := sc.GetWitnessInfo(stateDB, address)
	if err != nil {
		return
	}
	group = info.GroupIndex
	isApply = true
	if info.Status != sc.WitnessNormal {
		return
	}
	isWitness = true
	if info.GroupIndex != sc.DefSysGroupIndex {
		return
	}
	isSys = true
	_, list, err := sc.GetSystemMainWitness(stateDB)
	if err != nil {
		return
	}
	if list.Have(address) {
		isMain = true
	}
	return
}

// GetWitnessAtGroup 根据见证组ID获取见证组列表
func (dc *DagChain) GetWitnessAtGroup(index uint64) (common.AddressList, error) {
	defer tracetime.New().Stop()
	stateDB, err := dc.GetChainTailState(params.SCAccount)
	if err != nil {
		return nil, err
	}
	return sc.GetWitnessListAt(stateDB, index), nil
}

// GetMainSystemWitnessList 获取系统链主见证人
func (dc *DagChain) GetMainSystemWitnessList() (common.AddressList, error) {
	defer tracetime.New().Stop()
	stateDB, err := dc.GetChainTailState(params.SCAccount)
	if err != nil {
		return nil, err
	}
	_, list, err := sc.GetSystemMainWitness(stateDB)
	return list, err
}

// GetWitnessListByIndex 根据索引获取见证人列表
func (dc *DagChain) GetWitnessListByIndex(index uint64) (common.AddressList, error) {
	defer tracetime.New().Stop()
	if index == sc.DefSysGroupIndex {
		return dc.GetMainSystemWitnessList()
	}
	return dc.GetWitnessAtGroup(index)
}

// GetWitnessGroupIndex 获取当前见证人的见证组索引
func (dc *DagChain) GetWitnessGroupIndex(address common.Address) (uint64, error) {
	defer tracetime.New().Stop()
	stateDB, err := dc.GetChainTailState(params.SCAccount)
	if err != nil {
		return 0, err
	}
	info, err := sc.GetWitnessInfo(stateDB, address)
	if err != nil {
		return 0, err
	}
	return info.GroupIndex, nil
}

// GetWitnessInfoChainList 获取给定见证下当前所有链地址
func (dc *DagChain) GetWitnessInfoChainList(address common.Address) ([]common.Address, error) {
	defer tracetime.New().Stop()
	return dc.dag.GetWitnessCurrentUsers(address, 0, math.MaxUint64)
}

func (dc *DagChain) GetBrothers(addr common.Address, number uint64) (common.Hash, error) {
	defer tracetime.New().Stop()
	b := dc.GetStableHash(addr, number)
	if b.Empty() {
		return b, errors.New("brother not find")
	}
	return b, nil
}

// 获取所有投票信息的发起者账号
func (dc *DagChain) GetVoteMsgAddresses(hash common.Hash) []common.Address {
	defer tracetime.New().Stop()
	list, _ := dc.dag.GetVoteMsgAddresses(hash)
	return list
}

// GetChainWitnessLib 获取用户的见证组列表
func (self *DagChain) GetChainWitnessLib(addr common.Address, scUnitHash ...common.Hash) (lib []common.Address, err error) {
	defer tracetime.New().Stop()
	var sc common.Hash
	if len(scUnitHash) == 0 {
		sc = self.GetChainTailHash(params.SCAccount)
	} else {
		sc = scUnitHash[0]
	}
	return self.dag.GetWitnesses(addr, sc)
}

// GetChainWitnessLib 获取链的见证组
func (self *DagChain) GetChainWitnessGroup(chain common.Address, scUnitHash ...common.Hash) (index uint64, err error) {
	if chain == params.SCAccount {
		return sc.DefSysGroupIndex, nil
	}
	if len(scUnitHash) == 0 {
		db, err := self.GetChainTailState(params.SCAccount)
		if err != nil {
			return 0, err
		}
		return sc.GetGroupIndexFromUser(db, chain)
	}

	header := self.GetHeaderByHash(scUnitHash[0])
	if header == nil {
		return 0, types.ErrNotFoundHeader
	}
	if header.Number > 0 && header.MC != params.SCAccount {
		return 0, errors.New("invalid system chain hash")
	}
	db, err := self.GetStateDB(header.MC, header.StateRoot)
	if err != nil {
		return 0,
			fmt.Errorf("failed to load unit state,%s:%s", header.Hash(), err)
	}
	return sc.GetGroupIndexFromUser(db, chain)
}

// GetUnitWitnessLib 获取单元的见证人列表
func (self *DagChain) GetUnitWitnessLib(header *types.Header) (lib []common.Address, err error) {
	defer tracetime.New().Stop()
	var scHash common.Hash
	// 系统链无SCHash
	if header.MC == params.SCAccount {
		scHash = header.ParentHash
	} else {
		scHash = header.SCHash
	}
	return self.GetChainWitnessLib(header.MC, scHash)
}

// 获取见证人列表Hash值
func GetWitenssLibRoot(lib []common.Address) common.Hash {
	return types.DeriveSha(common.AddressList(lib))
}

// 获取见证人数，最低票数
//func (self *DagChain) GetVoteNum(mc common.Address, scHash common.Hash) (max, min int) {
//	// 获取目标状态
//	stateDB, err := self.StateAt(scHash)
//	if err != nil {
//		return
//	}
//
//	if mc == params.SCAccount {
//		imax, err := scapi.ReadConfig(stateDB, scapi.ConfigSystemWitnessNumber)
//		if err != nil {
//			return
//		}
//		imin, err := scapi.ReadConfig(stateDB, scapi.ConfigSystemWitnessNumberMin)
//		if err != nil {
//			return
//		}
//		return int(imax), int(imin)
//	} else {
//		imax, err := scapi.ReadConfig(stateDB, scapi.ConfigUserWitnessNumber)
//		if err != nil {
//			return
//		}
//		imin, err := scapi.ReadConfig(stateDB, scapi.ConfigUserWitnessNumberMin)
//		if err != nil {
//			return
//		}
//		return int(imax), int(imin)
//	}
//}
//
//// 统计有效投票
//// 统计时并不是统计见证人的投票数，而是统计
//func (self *DagChain) statisticalVoting(t interface{}) (err error) {
//	var (
//		needAddWitness []common.Address
//		needAddNeed    bool //标记是否需要记录应该参与见证的数量
//		header         *types.Header
//	)
//
//	switch v := t.(type) {
//	default:
//		return nil
//	case *types.Header:
//		hash := v.Hash()
//		needAddWitness, err = self.dag.GetVoteMsgAddresses(hash)
//		if err != nil {
//			return
//		}
//		header = v
//		//因为是不可逆单元，此时还需要统计此单元应该投票的见证人
//		needAddNeed = true
//
//	case *types.VoteMsg:
//		voter, err := v.Sender()
//		if err != nil {
//			return err
//		}
//
//		header = self.GetHeaderByHash(v.UnitHash)
//		if header == nil {
//			return fmt.Errorf("can not found unit header by hash %v", v.UnitHash.Hex())
//		}
//		needAddWitness = append(needAddWitness, voter)
//	}
//	if len(needAddWitness) == 0 {
//		return nil
//	}
//
//	//获取周期
//	scHeader := self.GetHeaderByHash(header.SCHash)
//	if scHeader == nil {
//		return fmt.Errorf("can not found unit header by hash %v", header.SCHash)
//	}
//	statedb, err := self.StateAt(scHeader.MC,scHeader.StateRoot)
//	if err != nil {
//		return
//	}
//	period, err := sc.GetSettlePeriod( scHeader.Number)
//	if err != nil {
//		return
//	}
//	// TODO 待优化,可以直接取单元的交易数
//	body := GetBody(self.chainDB, header.Hash())
//	txs := uint64(len(body.Txs))
//
//	// 先登记，该见证人在此周期内需要处理的交易数
//
//	var chainWitness []common.Address
//	if needAddNeed {
//		chainWitness, err = sc.GetChainWitness(statedb, header.MC)
//		if err != nil {
//			return
//		}
//	}
//	// 登记此批见证人已处理的交易数
//	self.dag.UpdateWitnessPeriodReport(period, txs, chainWitness, needAddWitness, header.Proposer)
//	return nil
//}

// GetStableHash 获取给定高度稳定单元哈希，如果不存在则为空哈希
func (dc *DagChain) GetStableHash(chain common.Address, number uint64) common.Hash {
	return GetStableHash(dc.chainDB, chain, number)
}

func (dc *DagChain) WriteLashUnitHash(mcaddr common.Address, hash common.Hash) error {
	defer tracetime.New().Stop()
	WriteChainTailHash(dc.chainDB, hash, mcaddr)
	return nil
}
func (dc *DagChain) ClearUnit(uhash common.Hash, delAll bool) error {
	defer tracetime.New().Stop()
	return dc.clearUnit(uhash, delAll, true)
}

// 获取理事心跳
func (dc *DagChain) GetCouncilHeartbeat() (sc.ListHeartbeat, error) {
	defer tracetime.New().Stop()
	stateDB, err := dc.GetChainTailState(params.SCAccount)
	if err != nil {
		return sc.ListHeartbeat{}, err
	}
	return sc.ReadCouncilHeartbeat(stateDB)
}

// 获取周期
func (dc *DagChain) GetPeriod() uint64 {
	lastUnit := dc.GetChainTailHead(params.SCAccount)
	return sc.GetSettlePeriod(lastUnit.Number)
}

// 获取链状态
func (dc *DagChain) GetChainStatus(address common.Address) (sc.ChainStatus, error) {
	defer tracetime.New().Stop()
	stateDB, err := dc.GetChainTailState(params.SCAccount)
	if err != nil {
		return 0, err
	}
	status, _ := sc.GetChainStatus(stateDB, address)
	if status == sc.ChainStatusNormal {
		witnessList, err := sc.GetChainWitness(stateDB, address)
		if err != nil {
			return sc.ChainStatusNormal, err
		}
		if uint64(witnessList.Len()) < params.ConfigParamsUCMinVotings {
			return sc.ChainStatusInsufficient, nil
		}
		return sc.ChainStatusNormal, nil
	}
	return status, nil
}
func (dc *DagChain) cacheUnit(unit *types.Unit) {
	if dc.enableAsyncWriteBody {
		dc.blockCache.Add(unit.Hash(), unit)
	}
}

// 获取时间线上的最后一个单元哈希
func (dc *DagChain) GetLastUintHash() (common.Hash, uint64) {
	//TODO:有性能优化空间，而不需要每次都错数据库读取
	//做法：原子记录最后时间，每次新单元落地完成时原子更新
	return rawdb.LastUnitHash(dc.chainDB)
}
func (dc *DagChain) GetUnitsByTimeLine(start uint64, count int) []common.Hash {
	return rawdb.UnitsByTimeLine(dc.chainDB, start, count)
}
