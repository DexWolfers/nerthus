package core

import (
	"errors"
	"expvar"
	"fmt"
	"gitee.com/nerthus/nerthus/core/rawdb"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"gitee.com/nerthus/nerthus/ntsdb"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/chancluster"
	"gitee.com/nerthus/nerthus/common/mclock"
	"gitee.com/nerthus/nerthus/common/set"
	"gitee.com/nerthus/nerthus/common/sync/cmap"
	"gitee.com/nerthus/nerthus/common/sync/lfreequeue"
	"gitee.com/nerthus/nerthus/common/tracetime"
	configpkg "gitee.com/nerthus/nerthus/core/config"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/txpool"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/metrics"
	"gitee.com/nerthus/nerthus/params"
)

var txPushStatus = expvar.NewMap("txpool")

var (
	// ErrNonceTooLow 在交易 nonce 低于本地账号链中上一笔交易nonce 时返回。
	// 交易中none自动为毫秒时间戳。
	ErrNonceTooLow = errors.New("nonce too low")

	// ErrUnderpriced 在交易的 gas 价格低于交易池配置的最低值时返回
	ErrUnderpriced = errors.New("transaction underpriced")

	// ErrReplaceUnderpriced 在尝试用在不需 gas price bump 下替换交易失败时返回
	ErrReplaceUnderpriced = errors.New("replacement transaction underpriced")

	// ErrIntrinsicGas 在一笔交易比启动调用所需的 gas 还要少时返回
	ErrIntrinsicGas = errors.New("intrinsic gas too low")

	// ErrNegativeValue 一个显而易见的错误：交易是是一个负金额交易
	ErrNegativeValue = errors.New("negative value")

	ErrGasPriceTooLow = errors.New("gas price too low")

	ErrTxExpired = errors.New("transaction has expired")

	// ErrOversizedData 在交易输入数据大小超过有意义的限制是返回。
	// 这不是共识错误是的交易无效，而是阻断 DoS(洪水攻击）。
	ErrOversizedData = errors.New("oversized data")

	//// ErrEmptyContract 部署合约时，合约的字节码不能为空
	//ErrEmptyContract = errors.New("empty deploy contract data")

	// ErrEmptyToAddress 接收者不可为nil
	ErrEmptyToAddress = errors.New("empty receiver address")

	// ErrExistsTxInTxPool 交易已存在交易池内存中
	ErrExistsTxInTxPool = errors.New("transaction has existed in tx pool")

	ErrTxChecking = errors.New("transaction has checking")

	// ErrInvalidSeed 验证不通过的Pow Seed 值
	ErrInvalidSeed    = errors.New("invalid pow seed")
	ErrInvalidTimeout = errors.New("invalid time out set")

	ErrTxCompleted = errors.New("tx process has completed")

	ErrInvalidGas = errors.New("oversized gas limit")

	// ErrExistInChain 交易在链中存在
	ErrExistInChain = errors.New("tx has existed in chain")

	ErrInvalidSender         = errors.New("invalid tx sender")
	ErrDisableSender         = errors.New("disable tx sender")
	ErrAccountMissingWitness = errors.New("tx's sender or receiver does not have witness,then disable push tx")
)

var (
	evictionInterval        = time.Minute     // evictable 交易检查间隔
	statsReportInterval     = 2 * time.Second // 交易池状态报告间隔
	defTxCheckingCacheLimit = 10000
	clearInvalidTxInterval  = 2 * time.Minute //定期清理过期交易间隔
)

// 属于非法交易，正常情况下不应该存在
type ErrInvalidTxData struct {
	Err error
}

func (e *ErrInvalidTxData) Error() string {
	return e.Err.Error()
}
func newInvalidTx(err error) *ErrInvalidTxData {
	return &ErrInvalidTxData{err}
}

var (
	// 交易池中pending交易各项指标
	pendingDiscardCounter   = metrics.NewRegisteredCounter("txpool/pending/discard", nil)   // 待处理队列中丢弃量
	pendingReplaceCounter   = metrics.NewRegisteredCounter("txpool/pending/replace", nil)   //待处理队列中替换量
	pendingRateLimitCounter = metrics.NewRegisteredCounter("txpool/pending/ratelimit", nil) // 待处理队列中超限制丢放弃量
	pendingNofundsCounter   = metrics.NewRegisteredCounter("txpool/pending/nofunds", nil)   // 待处理队列中余额不足放弃量

	// 交易池中 queued 交易各项指标
	queuedDiscardCounter   = metrics.NewRegisteredCounter("txpool/queued/discard", nil)   // 已挂起队列中丢弃量
	queuedReplaceCounter   = metrics.NewRegisteredCounter("txpool/queued/replace", nil)   //已挂起队列中替换量
	queuedRateLimitCounter = metrics.NewRegisteredCounter("txpool/queued/ratelimit", nil) // 已挂起队列中超限制丢放弃量
	queuedNofundsCounter   = metrics.NewRegisteredCounter("txpool/queued/nofunds", nil)   //  已挂起队列中余额不足放弃量

	// 一般交易指标
	invalidTxCounter     = metrics.NewRegisteredCounter("txpool/invalid", nil)     //交易计数
	underpricedTxCounter = metrics.NewRegisteredCounter("txpool/underpriced", nil) //低估交易

)

// TxPoolConfig 交易池配置
type TxPoolConfig struct {
	Disabled       bool
	NoDiskTx       bool          // 是否开启交易落地
	DisableTODO    bool          //是否启用记录  记录
	DiskTxPath     string        // 交易落地路径
	DiskTxInterval time.Duration // 交易落地频率
	DiskTxTimeout  time.Duration // 交易落地失效时间

	PriceLimit uint64 // 进入交易池的交易最低 gas 价格
	PriceBump  uint64 // 用于替换已存在交易的最小 gas price 波动百分比

	Lifetime time.Duration // 不可执行交易的最大排队时间

	PendingTxLimit     int
	ParallelChainLimit int // 最大可并行链数量限制
}

// DefaultTxPoolConfig 交易池默认配置
var DefaultTxPoolConfig = TxPoolConfig{
	NoDiskTx:       false, //默认开启交易落地
	DiskTxPath:     "txs",
	DiskTxTimeout:  time.Hour,
	DiskTxInterval: 20 * time.Second,
	DisableTODO:    false,

	Lifetime:           3 * time.Hour,
	PendingTxLimit:     30000,
	ParallelChainLimit: 2000,
}

// sanitize 检查用户配置并变更不合理或不可行的配置项
func (config *TxPoolConfig) sanitize() TxPoolConfig {
	conf := *config
	if conf.DiskTxTimeout < time.Second {
		log.Error("Sanitize invalid txpool disk-tx-timeout", "timeout", config.DiskTxTimeout)
	}
	if conf.DiskTxInterval < time.Second {
		log.Error("Sanitize invalid txpool disk-tx-interval", "interval", config.DiskTxInterval)
	}

	conf.PendingTxLimit = configpkg.MustInt("runtime.txpool_pending", config.PendingTxLimit)
	conf.ParallelChainLimit = configpkg.MustInt("runtime.witness_at_work_chains", config.ParallelChainLimit)
	return conf
}

type MinerWorkCheck func(from, to common.Address) bool

// TODO optimize gc, lock, pending(by special address filter)
type TxPool struct {
	config      TxPoolConfig
	chainconfig *params.ChainConfig
	dagChain    *DagChain
	gasPrice    uint64
	txFeed      event.Feed
	scope       event.SubscriptionScope
	signer      types.Signer
	mu          sync.RWMutex

	pendingCount       int64
	locals             *accountSet
	diskTxStore        txpool.TxDiskStore
	diskPath           string
	diskTxInterval     int
	diskTxTimeout      int
	all                cmap.ConcurrentMap
	checking           *set.StringSet //正在检查中的交易
	zeroFeePending     cmap.ConcurrentMap
	otherUserAddTxPool *chancluster.ChanCluster
	myUserAddTxPool    *chancluster.ChanCluster
	miner              MinerWorkCheck
	minerWork          Miner
	addedTxQueue       *lfreequeue.Queue
	todoPool           *txpool.TODOPool
	txdb               ntsdb.Database

	quit chan struct{}
	wg   sync.WaitGroup

	onAddDone func()
}

func NewTxPool(config TxPoolConfig, chainconfig *params.ChainConfig, dagChain *DagChain) *TxPool {
	var err error
	config = (&config).sanitize()
	pool := &TxPool{
		config:         config,
		chainconfig:    chainconfig,
		dagChain:       dagChain,
		signer:         types.NewSigner(chainconfig.ChainId),
		checking:       set.NewStrSet(32, defTxCheckingCacheLimit),
		gasPrice:       config.PriceLimit,
		zeroFeePending: cmap.New(),
		quit:           make(chan struct{}),
		all:            cmap.NewWith(1 << 12),
		addedTxQueue:   lfreequeue.NewQueue(),
	}
	if config.Disabled {
		return pool
	}
	pool.locals = newAccountSet(pool.signer)
	pool.reset(nil)

	pool.txdb, err = ntsdb.NewLDBDatabase(config.DiskTxPath, 32, 32)
	if err != nil {
		panic(err)
	}

	pool.todoPool = txpool.NewTODOPool(pool.txdb, pool.config.DisableTODO)

	// TODO reload tx from disk cache
	if !config.NoDiskTx {
		pool.diskTxStore, err = txpool.NewTxDiskStore(config.DiskTxInterval,
			config.DiskTxTimeout, pool.txdb, func(tx *types.Transaction) error {
				if tx.Expired(time.Now()) {
					return ErrTxExpired
				}
				//只添加有效交易
				actions, err := pool.dagChain.dag.GetTxNextAction(tx.Hash(), tx)
				if err != nil {
					return err
				}
				if len(actions) == 0 {
					return nil
				}
				return pool.AddLocal(tx)
			})
		if err != nil {
			panic(fmt.Sprintf("init disk tx store fail, path:%v, err:%v", config.DiskTxPath, err))
		}
	}

	//pool.chainEventSub = pool.dagChain.SubscribeChainEvent(pool.chainEventc)
	pool.otherUserAddTxPool = chancluster.NewChanCluster()
	pool.otherUserAddTxPool.Register(struct{}{}, 3, 1024, pool.pushTx, nil)
	pool.otherUserAddTxPool.Start()

	pool.myUserAddTxPool = chancluster.NewChanCluster()
	pool.myUserAddTxPool.SetUpperLimit(int64(pool.config.ParallelChainLimit))
	pool.myUserAddTxPool.Start()

	txPushStatus.Set("pending", expvar.Func(func() interface{} {
		return pool.pendingCount
	}))

	pool.todoPool.Start()
	go pool.loopPush()
	go pool.listenReceivedNewUnit()
	go pool.AutoClearInvalidTx()
	return pool
}

func (pool *TxPool) listenReceivedNewUnit() {
	pool.wg.Add(1)
	defer pool.wg.Done()

	// 订阅新单元
	newUnitCh := make(chan types.ChainEvent, 128)
	sub := pool.dagChain.SubscribeChainEvent(newUnitCh)
	defer sub.Unsubscribe()
	// 并发处理2个任务
	clt := chancluster.NewChanCluster()
	clt.Register(1, 5, 1024, func(arg []interface{}) error {
		ev := arg[0].(types.ChainEvent)
		for _, v := range ev.Unit.Transactions() {
			pool.Remove(v.TxHash)
		}
		if pool.minerWork != nil {
			err := pool.minerWork.ProcessIrreversibleEvent(ev)
			if err != nil {
				log.Debug("failed: receive new unit", "err", err)
			}
		} else {
			pool.processNewUnit(ev)
		}
		return nil
	}, nil)
	//clt.Register(2, 5, 1024, func(arg []interface{}) error {
	//	unit := arg[0].(*types.Unit)
	//	tr := arg[1].(*tracetime.TraceTime)
	//	now := time.Now()
	//	tr.Tag().AddArgs("cost1", now.Sub(arg[2].(time.Time)))
	//	pool.minerWork.ProcessNewUnit(unit, tr)
	//	tr.Tag().AddArgs("cost2", time.Now().Sub(now))
	//	tr.Stop()
	//	return nil
	//}, nil)
	clt.Start()
	defer clt.Stop()
	for {
		select {
		case <-pool.quit:
			return
		case ev, ok := <-newUnitCh:
			if !ok {
				return
			}
			tr := tracetime.New("chain", ev.Unit.MC(), "number", ev.Unit.Number()).SetMin(0)
			tr.AddArgs("size2", clt.Len(1), "size3", clt.Len(2))
			clt.Handle(1, ev)
			tr.Tag()
			if pool.minerWork != nil {
				//clt.Handle(2, ev.Unit, tr, time.Now())
				clt.HandleOrRegister(ev.Unit.MC(), 1, 32, func(arg []interface{}) error {
					if pool.minerWork == nil {
						return nil
					}
					unit := arg[0].(*types.Unit)
					tr := arg[1].(*tracetime.TraceTime)
					now := time.Now()
					tr.Tag().AddArgs("cost1", now.Sub(arg[2].(time.Time)))
					pool.minerWork.ProcessNewUnit(unit, tr)
					tr.Tag().AddArgs("cost2", time.Now().Sub(now))
					tr.Stop()
					return nil
				}, nil, ev.Unit, tr, time.Now())
			}
		}

	}

}

func (pool *TxPool) addPendingCount(n int64) {
	atomic.AddInt64(&pool.pendingCount, n)
}

func (pool *TxPool) processNewUnit(ev types.ChainEvent) {
	if pool.config.DisableTODO {
		return
	}
	//清理已完成
	chain := ev.Unit.MC()
	for i, tx := range ev.Unit.Transactions() {
		pool.todoPool.Del(chain, tx.TxHash, tx.Action)

		otx := tx.Tx
		if otx == nil {
			otx = pool.dagChain.GetTransaction(tx.TxHash)
		}
		if otx == nil {
			continue
		}

		//检查下一个 Action
		GetTxNextActions(chain, tx.Action, tx.TxHash, ev.Receipts[i], otx, func(chain common.Address, txhash common.Hash, tx *types.Transaction, action types.TxAction) {
			pool.todoPool.Push(chain, txhash, tx, action)
		})
	}
}

func (pool *TxPool) isFull() bool {
	return pool.config.PendingTxLimit > 0 && atomic.LoadInt64(&pool.pendingCount) >= int64(pool.config.PendingTxLimit)
}
func (pool *TxPool) waitAdd() bool {
	//不需要保证实时处理，故直接采用间隔检查
	tk := time.NewTicker(time.Second * 3)
	defer tk.Stop()

	for {
		select {
		case <-pool.quit:
			return false
		case <-tk.C:
			if pool.isFull() {
				continue
			}
			return true
		}
	}
}

type Miner interface {
	ProcessNewTx(tx *types.Transaction, tr *tracetime.TraceTime)
	ProcessNewUnit(unit *types.Unit, tr *tracetime.TraceTime)
	ProcessIrreversibleEvent(ev types.ChainEvent) error
}

func (pool *TxPool) SetMiner(minerWork Miner, miner MinerWorkCheck) {
	// TODO 会出现并发问题,待优化
	pool.miner = miner
	pool.minerWork = minerWork
}

func (pool *TxPool) Stop() {
	close(pool.quit)
	pool.scope.Close()
	pool.otherUserAddTxPool.Stop()
	pool.myUserAddTxPool.Stop()

	if pool.todoPool != nil {
		pool.todoPool.Stop()
	}

	// close tx persists disk
	if !pool.config.NoDiskTx {
		pool.diskTxStore.Close()
	}
	pool.txdb.Close()

	pool.wg.Wait()
	log.Info("Transaction pool stopped")
}

func (pool *TxPool) TODOPool() *txpool.TODOPool {
	return pool.todoPool
}
func (pool *TxPool) SubscribeTxPreEvent(ch chan<- types.TxPreEvent) event.Subscription {
	return pool.scope.Track(pool.txFeed.Subscribe(ch))
}

// TODO filter the special address
func (pool *TxPool) Pending() (types.Transactions, error) {
	txs := make(types.Transactions, 0, pool.all.Count())
	pool.all.IterCb(func(key string, v interface{}) bool {
		txs = append(txs, v.(*types.Transaction))
		return true
	})
	return txs, nil
}
func (pool *TxPool) PendingCount() int {
	return pool.all.Count()
}

// PendingByAcct 某账户下当前所有交易
func (pool *TxPool) PendingByAcct(acct common.Address) (txs types.Transactions) {
	pool.all.IterCb(func(key string, v interface{}) bool {
		if v.(*types.Transaction).SenderAddr() == acct {
			txs = append(txs, v.(*types.Transaction))
		}
		return true
	})
	return
}

// AddLocal 添加本地交易
func (pool *TxPool) AddLocal(tx *types.Transaction) error {
	err := pool.addTx(tx, true)
	pool.afterAddTx(tx, err == nil, nil)
	return err
}

// AddLocals 添加本地交易列表
func (pool *TxPool) AddLocals(txs []*types.Transaction) error {
	success, err := pool.addTxs(txs, true)
	for _, v := range success {
		pool.afterAddTx(v, true, nil)
	}
	return err
}
func (pool *TxPool) ValidPendingTx(tx *types.Transaction) error {
	if nexts, err := pool.dagChain.dag.GetTxNextAction(tx.Hash(), tx); err != nil {
		return err
	} else if len(nexts) == 0 {
		return ErrTxCompleted
	}
	return nil
}

// pushTx 交易推送到交易池中处理
func (pool *TxPool) pushTx(args []interface{}) error {

	tx := args[0].(*types.Transaction)
	local := args[1].(bool)
	createT := args[2].(mclock.AbsTime)

	tr := tracetime.New("cost1", mclock.Now()-createT, "chain", tx.SenderAddr()).SetMin(time.Second / 2)
	err := pool.addTx(tx, local, tr)
	log.Debug("txPool push tx", "err", err)
	pool.afterAddTx(tx, err == nil, tr)
	tr.Stop()
	// 错误回调处理
	if err == ErrExistsTxInTxPool || err == ErrTxChecking || err == ErrExistInChain || err == ErrTxCompleted {
		return nil
	}
	if err != nil && len(args) > 3 && args[3] != nil {
		errFunc, ok := args[3].(func(error))
		if ok {
			errFunc(err)
		}
	}
	return err
}
func (pool *TxPool) afterAddTx(tx *types.Transaction, success bool, tr *tracetime.TraceTime) {
	if success {
		if pool.minerWork != nil {
			pool.minerWork.ProcessNewTx(tx, tr)
		}
		txPushStatus.Add("success", 1)
	} else {
		txPushStatus.Add("failed", 1)
	}
	if pool.onAddDone != nil {
		pool.onAddDone()
	}
}

// AddRemotes 收到新交易
func (pool *TxPool) AddRemotes(txs []*types.Transaction) error {
	for _, tx := range txs {
		pool.AddRemote(tx)
	}
	return nil
}

// 处理其他节点交易同步过来的交易，需要验证错误信息
func (pool *TxPool) AddRemotesTxpipe(txs []*types.Transaction) error {
	for _, tx := range txs {
		err := pool.addRemote(tx, true, false, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

func (pool *TxPool) IsFull() bool {
	return pool.isFull()
}

func (pool *TxPool) setChecking(txhash common.Hash) error {
	added := pool.checking.Add(txhash.Hex())
	if added {
		return nil
	}
	return ErrTxChecking
}
func (pool *TxPool) addRemote(tx *types.Transaction, sync bool, needWait bool, errFunc func(error)) error {
	// 签入，如果存在则跳过
	if err := pool.setChecking(tx.Hash()); err != nil {
		return nil
	}
	//检查Pending上限
	if pool.isFull() {
		if !needWait { // 如果不需要阻塞等待，丢弃
			return nil
		}
		if !pool.waitAdd() {
			return errors.New("pool is full")
		}
	}
	if sync {
		err := pool.addTx(tx, false)
		if err != nil {
			log.Debug("add tx failed", "tx", tx.Hash(), "err", err)
		}
		pool.afterAddTx(tx, err == nil, nil)
		return err
	} else {
		//根据来源不同到不同队列中处理
		if pool.miner != nil && pool.miner(tx.SenderAddr(), tx.To()) {
			txPushStatus.Add("mytxs", 1)

			pool.myUserAddTxPool.HandleOrRegister(tx.SenderAddr(), 1, 1024, pool.pushTx, nil, tx, false, mclock.Now(), errFunc)
		} else {
			txPushStatus.Add("others", 1)
			pool.otherUserAddTxPool.Handle(struct{}{}, tx, false, mclock.Now(), errFunc)
		}
		return nil
	}
}

// AddRemote 添加远程交易
func (pool *TxPool) AddRemote(tx *types.Transaction) error {
	return pool.addRemote(tx, false, true, nil)

}
func (pool *TxPool) AddRemoteSync(tx *types.Transaction) error {
	return pool.addRemote(tx, true, true, nil)
}

func (pool *TxPool) Get(hash common.Hash) *types.Transaction {
	v, ok := pool.all.Get(hash.String())
	if !ok {
		return nil
	}
	return v.(*types.Transaction)
}
func (pool *TxPool) ValidWithCache(tx *types.Transaction) error {
	if tx == nil {
		return errors.New("tx is nil")
	}

	txcache := pool.Get(tx.Hash())
	if txcache == nil || tx.SenderAddr() != txcache.SenderAddr() {
		_, err := types.Sender(pool.signer, tx)
		return err
	}
	if types.SameTxCheck(txcache, tx) {
		return nil
	}
	return types.ErrInvalidSig
}
func (pool *TxPool) Remove(hash common.Hash) {
	result, ok := pool.all.Get(hash.String())
	if !ok {
		return
	}
	pool.addPendingCount(-1)
	pool.all.Remove(hash.String())
	tx := result.(*types.Transaction)
	log.Trace("remove tx", "from", tx.SenderAddr(), "tx", hash)
	if IsFreeTx(tx) {
		pool.DonePendingFreeTx(tx)
	}
	//TODO(ysqi): 需要从Disk中移除已完成的交易
	return
}

// TODO
func (pool *TxPool) reset(accounts []common.Address) {}

func (pool *TxPool) addTxs(txs []*types.Transaction, local bool) ([]*types.Transaction, error) {
	var successTxs []*types.Transaction
	for _, tx := range txs {
		err := pool.addTx(tx, local)
		if err != nil {
			return successTxs, err
		}
		successTxs = append(successTxs, tx)
	}
	return successTxs, nil
}

func (pool *TxPool) addTx(tx *types.Transaction, local bool, trs ...*tracetime.TraceTime) (err error) {
	var tr *tracetime.TraceTime
	if len(trs) > 0 {
		tr = trs[0]
	} else {
		tr = tracetime.New("tx", tx.Hash(), "createat", tx.Time2().Unix())
		defer tr.Stop()
	}

	hash := tx.Hash()
	hashStr := hash.String()

	tr.Tag().AddArgs("from", tx.SenderAddr(), "tx", hash)

	log.Debug("adding transaction", "tx", hash)

	defer pool.checking.Delete(hashStr) //退出时移除

	if pool.all.Has(hashStr) {
		return ErrExistsTxInTxPool
	}

	if local { //remote的交易已在外部检查
		//检查中则退出
		if err := pool.setChecking(tx.Hash()); err != nil {
			return err
		}
	}

	tr.Tag("check")

	// 解签名者耗时长，故在过滤完已存在的交易后再处理
	from, err := types.Sender(pool.signer, tx)
	if err != nil {
		return err
	}
	tr.Tag("sender")
	// 先进行对交易的基本数据合法新进行交易
	if err := pool.ValidateTx(from, local, tx, pool.dagChain.GetChainTailInfo(from).Balance(), false); err != nil {
		return err
	}
	tr.Tag("validate")
	pool.all.Set(hashStr, tx)
	pool.addPendingCount(1)

	tr.Tag()
	if local {
		pool.locals.add(from)
	}

	tr.Tag()
	//推送Tx
	pool.addedTxQueue.Enqueue(types.TxPreEvent{IsLocal: local, Tx: tx})
	tr.Tag()

	pool.persistsTxDisk(local, from, tx)
	if local {
		// 本地交易写入db
		rawdb.WriteTransaction(pool.dagChain.chainDB, tx)
	}
	tr.AddArgs("status", "ok", "from", from)

	log.Debug("Pooled new future transaction", "local", local,
		"hash", hash, "from", from, "to", tx.To())
	tr.Tag()
	return nil
}

func (pool *TxPool) minGasPrice() uint64 {
	return pool.chainconfig.Get(params.ConfigParamsLowestGasPrice)
}
func (pool *TxPool) powDiff() uint64 {
	return pool.chainconfig.Get(params.ConfigParamsMinFreePoW)
}
func (pool *TxPool) gasLimit() uint64 {
	return pool.chainconfig.Get(params.ConfigParamsUcGasLimit)
}

func (pool *TxPool) ValidateTx(sender common.Address, local bool, tx *types.Transaction, fromBalance *big.Int, mock bool) error {
	//不允许特殊地址发送交易
	if sender == params.PublicPrivateKeyAddr {
		return newInvalidTx(ErrDisableSender)
	}
	getGasLimit := pool.gasLimit
	// 系统链交易gas limit保留10%
	if tx.To() == params.SCAccount {
		getGasLimit = func() uint64 {
			return uint64(float64(pool.gasLimit()) * 0.9)
		}
	}
	isFreeTx, err := VerifyTxBaseData(sender, tx, time.Now(), pool.minGasPrice, pool.powDiff, getGasLimit)
	if err != nil {
		return err
	}

	local = local || pool.locals.contains(tx.SenderAddr())
	if !local && pool.gasPrice > tx.GasPrice() && !isFreeTx {
		return newInvalidTx(ErrUnderpriced)
	}

	var pass bool
	if isFreeTx { //如果有免费交易则进行标记
		if !pool.AddPendingFreeTx(tx) {
			return errors.New("exist free tx from account")
		}
		defer func() {
			//如果尚未成功添加则删除记录，防止交易尚未成功到交易池，而已产生标记
			if !pass {
				pool.DonePendingFreeTx(tx)
			}
		}()
	} else { //如果是非免费交易则，需要检查余额是否足额
		if fromBalance.Cmp(tx.Cost()) < 0 {
			return fmt.Errorf("insufficient funds for gas * price + value, balance %d < %d * %d + %d, %d",
				fromBalance, tx.Gas(), tx.GasPrice(), tx.Amount(), tx.Cost())
		}
		//检查Gas是否满足基本要求
		intrGas := IntrinsicGas(tx.Data(), sc.IsCallCreateContractTx(tx.To(), tx.Data()))
		if tx.Gas() < intrGas {
			return newInvalidTx(ErrIntrinsicGas)
		}
	}

	if err := pool.checkAccountWitness(tx); err != nil {
		return err
	}

	if !mock {
		//检查交易是否已处理完毕
		//TODO(ysqi): 此检查对TPS有影响
		if nexts, err := pool.dagChain.dag.GetTxNextAction(tx.Hash(), tx); err != nil {
			return err
		} else if len(nexts) == 0 {
			return ErrTxCompleted
		} else if !pool.config.DisableTODO {
			for _, v := range nexts {
				pool.todoPool.Push(v.MC, tx.Hash(), tx, v.Action)
			}
		}
	}
	pass = true
	return nil
}

// 为了防止用户发送时，任何一方见证人不存在情况下下引起的交易攻击。
// 但这将带有性能损坏，TODO:dagChain 优化对系统链 StataDB 的缓存方案
func (pool *TxPool) checkAccountWitness(tx *types.Transaction) error {
	// sender 不需要检查，因为前面已经校验余额是否足够。
	//既然有余额，则说明 sender 必然已经分配见证组。因此只需校验 To 是否存在见证人
	//过滤掉系统链和转账给自己的情况
	if tx.To() != params.SCAccount && tx.To() != tx.SenderAddr() {
		if _, err := pool.dagChain.GetChainWitnessGroup(tx.To()); err != nil {
			if err == sc.ErrChainMissingWitness {
				return ErrAccountMissingWitness
			}
			return err
		}
	}
	return nil
}

// persistsTxDisk 交易落地磁盘
func (pool *TxPool) persistsTxDisk(local bool, from common.Address, tx *types.Transaction) {
	// 禁用
	if pool.config.NoDiskTx || !pool.locals.contains(from) {
		return
	}
	if err := pool.diskTxStore.Put(local, tx); err != nil {
		log.Error("TxPool.persistsTxDisk", "err", err)
	} else {
		log.Trace("saved tx", "txhash", tx.Hash())
	}
}

// GetTxWithDisk 获取本地交易数据，优先从交易池中，如果没有则从本地磁盘中拉取数据。
func (pool *TxPool) GetTxWithDisk(txHash common.Hash) *types.Transaction {
	tx := pool.Get(txHash)
	if tx != nil {
		return tx
	}
	if pool.diskTxStore != nil {
		tx, err := pool.diskTxStore.Get(txHash)
		if err != nil {
			return nil
		}
		return tx
	}
	return nil
}

// 因为实现了并发发送事件，并发时将引起堵塞。
// 因此独立将信号无限制存放到队列中，再将其独立外发。
// 好处：
//	 1. 不堵塞交易池处理
// 坏处：
//   1. 占用一些额外资源
func (pool *TxPool) loopPush() {
	pool.wg.Add(1)
	defer pool.wg.Done()

	//向外部发送信号
	for {
		select {
		case <-pool.quit:
			//释放资源
			pool.addedTxQueue.Clear()
			return
		case tx, ok := <-pool.addedTxQueue.DequeueChan():
			if ok {
				pool.txFeed.Send(tx)
			}
		}
	}
}

// 定期清理过期交易
func (pool *TxPool) AutoClearInvalidTx() {
	pool.wg.Add(1)
	defer pool.wg.Done()

	tick := time.NewTicker(clearInvalidTxInterval)
	for {
		select {
		case <-pool.quit:
			return
		case <-tick.C:
			var needClear []common.Hash
			now := time.Now()
			pool.all.IterCb(func(key string, v interface{}) bool {
				tx := v.(*types.Transaction)
				if tx.Expired(now) {
					needClear = append(needClear, tx.Hash())
				} else if err := pool.ValidPendingTx(tx); err != nil {
					needClear = append(needClear, tx.Hash())
				}
				return true
			})
			for _, txHash := range needClear {
				pool.Remove(txHash)
				log.Trace("tx is expired,remove it", "hash", txHash)
			}
		}
	}
}
func (pool *TxPool) AddPendingFreeTx(tx *types.Transaction) bool {
	key := common.Bytes2Hex(tx.Data4()) + tx.SenderAddr().Hex()
	if pool.zeroFeePending.Has(key) {
		return false
	}
	pool.zeroFeePending.Set(key, struct{}{})
	log.Trace("add free tx flag", "sender", tx.SenderAddr(), "tx", tx.Hash(), "key", key)
	return true
}
func (pool *TxPool) DonePendingFreeTx(tx *types.Transaction) {
	key := common.Bytes2Hex(tx.Data4()) + tx.SenderAddr().Hex()
	pool.zeroFeePending.Remove(key)
	log.Trace("delete free tx flag", "sender", tx.SenderAddr(), "tx", tx.Hash(), "key", key)
}

// 是否存在某种类型的免费
func (pool *TxPool) ExistFeeTx(prefix []byte, addr common.Address) bool {
	return pool.zeroFeePending.Has(common.Bytes2Hex(prefix) + addr.Hex())
}

type accountSet struct {
	//accounts map[common.Address]struct{}
	accounts sync.Map
	signer   types.Signer
}

// newAccountSet 用相关的签名器创建一个新的地址集，签名器用于从交易中获取交易账户地址。
func newAccountSet(signer types.Signer) *accountSet {
	return &accountSet{
		accounts: sync.Map{},
		signer:   signer,
	}
}

// contains 检查给定地址是否存在于集合中
func (as *accountSet) contains(addr common.Address) bool {
	_, exist := as.accounts.Load(addr)
	return exist
}

// containsTx 检查给定交易的交易发送者是否存在集合中。如果无法导出交易发送者则返回false
func (as *accountSet) containsTx(tx *types.Transaction) bool {
	if addr, err := types.Sender(as.signer, tx); err == nil {
		return as.contains(addr)
	}
	return false
}

// add 添加一个新地址到队列中
func (as *accountSet) add(addr common.Address) {
	as.accounts.Store(addr, struct{}{})
}

// TODO use in futures
type PendingHandler = func(addr common.Address, txPool *TxPool) bool

func pendingFilter(mainAccount common.Address) func(common.Address, *TxPool) bool {
	return func(addr common.Address, txPool *TxPool) bool {
		return false
	}
}
