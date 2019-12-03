// 包 nts 实现Nerthus节点通信协议以及节点运行出来
package nts

import (
	"errors"
	"fmt"
	"math/big"
	"sync"

	"gitee.com/nerthus/nerthus/accounts"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/consensus"
	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/consensus/bft/backend"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/statistics"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/council"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/internal/ntsapi"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/node"
	"gitee.com/nerthus/nerthus/nts/filters"
	"gitee.com/nerthus/nerthus/nts/trace"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/p2p"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/rpc"
	"gitee.com/nerthus/nerthus/witness"
	"gitee.com/nerthus/nerthus/witness/service/wconn"
)

type LesServer interface {
	Start(srvr *p2p.Server)
	Stop()
	Protocols() []p2p.Protocol
}

// Nerthus 实现完整节点运行服务
type Nerthus struct {
	config      *Config
	chainConfig *params.ChainConfig
	// Channel for shutting down the service
	shutdownChan  chan bool    // Channel for shutting down the Nerthus
	stopDbUpgrade func() error // stop chain db sequential key upgrade
	// Handlers
	lesServer LesServer
	// DB interfaces
	chainDb ntsdb.Database // Unit chain database
	sta     *statistics.Statistic
	rd      ntsdb.RD

	// dag 链
	txPool          *core.TxPool
	dagchain        *core.DagChain
	ProtocolHandler *ProtocolManager

	eventMux            *event.TypeMux
	bftEngine           consensus.Engine
	accountManager      *accounts.Manager
	witnessNodesControl *wconn.WitnessGroupConnControl

	ApiBackend *NtsApiBackend

	dag *core.DAG // dag service

	witness       *witness.Witness
	gasPrice      *big.Int
	mainAccount   common.Address
	member        *council.Heartbeat
	networkId     uint64
	netRPCService *ntsapi.PublicNetAPI

	lock sync.RWMutex // Protects the variadic fields (e.g. gas price and mainAccount)
}

func (n *Nerthus) AddLesServer(ls LesServer) {
	n.lesServer = ls
}

// New creates a new Nerthus object (including the
// initialisation of the common Nerthus object)
func New(ctx *node.ServiceContext, config *Config) (*Nerthus, error) {
	if !config.SyncMode.IsValid() {
		return nil, fmt.Errorf("invalid sync mode %d", config.SyncMode)
	}

	chainDb, err := CreateDB(ctx, config, "dagdata")
	if err != nil {
		return nil, fmt.Errorf("failed to open database,%v", err)
	}
	rd, err := CreateRD(ctx, config, "statistics.db3")
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite3,%v", err)
	}
	// @ysqi need create a dag service before handler genesis unit
	chainConfig, genesisHash, genesisErr := core.SetupGenesisUnit(chainDb, config.Genesis)
	if _, ok := genesisErr.(*params.ConfigCompatError); genesisErr != nil && !ok {
		return nil, genesisErr
	}
	log.Info("Initialised chain configuration", "genesis", genesisHash, "chainId", chainConfig.ChainId)
	nts := &Nerthus{
		chainDb:        chainDb,
		rd:             rd,
		chainConfig:    chainConfig,
		eventMux:       ctx.EventMux,
		accountManager: ctx.AccountManager,
		//engine:         CreateConsensusEngine(ctx, config, chainConfig, chainDb),
		shutdownChan: make(chan bool),
		networkId:    config.NetworkId,
		gasPrice:     config.GasPrice,
		mainAccount:  config.MainAccount,
		config:       config,
		witness:      nil,
	}

	log.Info("Initialising Nerthus protocol", "network", config.NetworkId)
	vmConfig := vm.Config{EnablePreimageRecording: config.EnablePreimageRecording}
	nts.dagchain, err = core.NewDagChain(chainDb, nts.chainConfig, nts.bftEngine, vmConfig)
	if err != nil {
		return nil, err
	}

	// 设置bftEngine
	{
		nts.bftEngine = CreateBftConsensusEngine(ctx, config, nts.dagchain, chainConfig, chainDb)
		nts.dagchain.SetOptions(core.SetBftEngine(nts.bftEngine))
	}

	sta, err := statistics.New(nts.dagchain, rd, ctx.AccountManager)
	if err != nil {
		return nil, err
	}
	nts.sta = sta
	nts.dagchain.SetSta(sta)

	dagInst, err := core.NewDag(chainDb, ctx.EventMux, nts.dagchain)
	if err != nil {
		return nil, err
	}
	nts.dag = dagInst

	nts.txPool = core.NewTxPool(config.TxPool, nts.chainConfig, nts.dagchain)

	nts.ApiBackend = &NtsApiBackend{nts: nts, ctx: ctx}

	//构建通讯协议处理
	cfg := ProtocolConfig{
		NetworkId:         config.NetworkId,
		MaxPeers:          config.MaxPeers,
		MaxMessageSize:    config.MaxMessageSize,
		SyncMode:          config.SyncMode,
		SubPipeListenAddr: config.SubPipeListenAddr,
	}
	if nts.ProtocolHandler, err = NewProtocolManager(ctx.Server, &cfg, nts.chainConfig, nts.eventMux, nts.txPool,
		nts.dagchain, chainDb, nts.ApiBackend); err != nil {
		return nil, err
	}
	nts.witnessNodesControl = wconn.New(nts.ChainDb(), nts.ApiBackend.PeerSet(), nts.DagChain())

	//允许借助已校验的交易，来减少重复的交易签名校验
	nts.dag.GetDagChain().Validator().SetTxSenderValidator(nts.txPool.ValidWithCache)
	return nts, nil
}

// TODO 和CreateConsensusEngine合并
func CreateBftConsensusEngine(ctx *node.ServiceContext, config *Config, dag *core.DagChain, chainConfig *params.ChainConfig, db ntsdb.Database) consensus.Engine {
	log.Info("BFT engine init successfully")
	bftConfig := bft.DefaultConfig
	bftConfig.ChainId = chainConfig.ChainId
	return backend.New(&bftConfig, ctx.EventMux, dag, nil, nil)
}

func (n *Nerthus) APIs() []rpc.API {

	apis := ntsapi.GetAPIs(n.ApiBackend)
	apis = append(apis, sc.GetApis(n.ApiBackend, n.DagChain())...)
	// Append all the local APIs and return
	return append(apis, []rpc.API{
		{
			Namespace: "nts",
			Version:   "1.0",
			Service:   NewPublicNerthusAPI(n),
			Public:    true,
		},
		{
			Namespace: "nts",
			Version:   "1.0",
			Service:   NewPrivateNerthusAPI(n),
			Public:    false,
		},
		{
			Namespace: "net",
			Version:   "1.0",
			Service:   n.netRPCService,
			Public:    true,
		},
		{
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewDebugNerthusAPI(n),
			Public:    false,
		},
		{
			Namespace: "nts",
			Version:   "1.0",
			Service:   filters.NewPublicFilterAPI(newFilterAPIBackend(n), false),
			Public:    true,
		},
		{
			Namespace: "trace",
			Version:   "1.0",
			Service:   trace.NewPrivateTraceAPI(n.DagChain(), n.chainDb),
			Public:    false,
		},
	}...)
}

func (n *Nerthus) GetNumber() (uint64, error) {
	return n.DagChain().GetChainTailHead(params.SCAccount).Number, nil
}

func (n *Nerthus) MainAccount() (ma common.Address, err error) {
	n.lock.RLock()
	mainAccount := n.mainAccount
	n.lock.RUnlock()

	if mainAccount != (common.Address{}) {
		return mainAccount, nil
	}
	if wallets := n.AccountManager().Wallets(); len(wallets) > 0 {
		if accounts := wallets[0].Accounts(); len(accounts) > 0 {
			return accounts[0].Address, nil
		}
	}
	return common.Address{}, fmt.Errorf("mainAccount address must be explicitly specified")
}

// 启动见证，需要指定主账号和密码
// Author: @kulics
func (n *Nerthus) StartWitness(mainAccount common.Address, password string) error {
	n.lock.Lock()
	defer n.lock.Unlock()
	if n.witness != nil {
		return errors.New("witness service is running,can not repeat start")
	}
	if n.member != nil {
		return errors.New("council service is running,can not start witness")
	}
	// 判断是否见证人，如果不是则返回错误
	// 需要判断用户见证人或者系统见证人
	_, isWitness, isSys, _, _, err := n.DagChain().GetWitnessStatus(mainAccount)
	if err != nil {
		return err
	}
	if !isWitness {
		return sc.ErrNotWitness
	}
	account := accounts.Account{Address: mainAccount}
	wallet, err := n.AccountManager().Find(account)
	if err != nil {
		return err
	}
	privateKey, err := wallet.GetPrivateKeyByAuth(account, password)
	if err != nil {
		return err
	}
	n.witness, err = witness.New(n.ApiBackend, n.EventMux(), mainAccount, privateKey, isSys)
	if err != nil {
		return err
	}
	n.mainAccount = mainAccount
	chanWitnessOut := make(chan struct{})
	err = n.witness.Start(chanWitnessOut, n.witnessNodesControl)
	if err != nil {
		n.witness = nil
		return err
	}
	n.eventMux.Post(types.WitnessWorkEvent{Witness: n.mainAccount, WitnessGroup: n.witness.GroupIndex(), Action: types.WitnessWorkStarted})
	//注入检查是否是当前见证人的工作链
	n.txPool.SetMiner(n.witness.Schedule(), func(from, to common.Address) bool {
		// 在开启见证服务后才需要判断
		if n.IsWitnessing() {
			return n.Witness().NeedWorkAt(from) || (from != to && n.Witness().NeedWorkAt(to))
		}
		return false
	})
	go n.witnessOut(chanWitnessOut)
	return nil
}

func (n *Nerthus) witnessOut(out <-chan struct{}) {
	select {
	case <-out:
		n.StopWitness()
	}
}

// 停止见证
// Author: @kulics
func (n *Nerthus) StopWitness() error {
	n.lock.Lock()
	defer n.lock.Unlock()
	if n.witness == nil {
		return errors.New("did not have witness")
	}
	n.txPool.SetMiner(nil, nil)
	n.witness.Stop()
	n.eventMux.Post(types.WitnessWorkEvent{Witness: n.mainAccount, WitnessGroup: n.witness.GroupIndex(), Action: types.WitnessWorkStoped})
	n.witness = nil
	return nil
}

// 获取系统链见证人
func (n *Nerthus) GetSystemWitnessList() (common.AddressList, error) {
	// 取出当前成员数据
	arr, err := n.getSysWitList()
	if err != nil {
		return nil, err
	}
	return arr, nil
}

// 获取系统见证人
func (n *Nerthus) getSysWitList() (common.AddressList, error) {
	// 取出系统链数据
	stateDB, err := n.dagchain.GetChainTailState(params.SCAccount)
	if err != nil {
		return nil, err
	}
	_, list, err := sc.GetSystemMainWitness(stateDB)
	// 取出当前成员数据
	return list, err
}

// 获取所有系统见证人
func (n *Nerthus) GetAllSysWitness() (common.AddressList, error) {
	stateDB, err := n.DagChain().GetChainTailState(params.SCAccount)
	if err != nil {
		return nil, err
	}
	return sc.ReadSystemWitness(stateDB)
}

// 启动成员业务，需要指定成员账号和密码
// Author: @kulics
func (n *Nerthus) StartMember(ac common.Address, password string) error {
	n.lock.Lock()
	defer n.lock.Unlock()
	if n.member != nil {
		return errors.New("can't start server again")
	}
	if n.witness != nil {
		return errors.New("witness service is running,can not start council service")
	}
	// 判断是否成员，如果不是则返回错误
	// 取出系统链数据
	stateDB, err := n.DagChain().GetChainTailState(params.SCAccount)
	if err != nil {
		return err
	}
	if s, ok := sc.GetCouncilStatus(stateDB, ac); !ok {
		return sc.ErrNotCouncil
	} else if s != sc.CouncilStatusValid {
		return errors.New("not active council")
	}

	account := accounts.Account{Address: ac}
	wallet, err := n.AccountManager().Find(account)
	if err != nil {
		return err
	}
	privateKey, err := wallet.GetPrivateKeyByAuth(account, password)
	if err != nil {
		return err
	}
	n.member = council.New(n.ApiBackend)
	if err = n.member.Start(ac, privateKey); err != nil {
		return err
	}
	n.mainAccount = ac
	return nil
}

// 停止成员业务
// Author: @kulics
func (n *Nerthus) StopMember() error {
	n.lock.Lock()
	defer n.lock.Unlock()
	if n.member == nil {
		return errors.New("did not have member")
	}
	err := n.member.Stop()
	if err != nil {
		return err
	}
	n.member = nil
	return nil
}

func (n *Nerthus) IsWitnessing() bool {
	n.lock.Lock()
	defer n.lock.Unlock()
	return n.witness != nil && n.witness.Mining()
}
func (n *Nerthus) Witness() *witness.Witness {
	return n.witness
}
func (n *Nerthus) Council() *council.Heartbeat { return n.member }

func (n *Nerthus) AccountManager() *accounts.Manager { return n.accountManager }
func (n *Nerthus) EventMux() *event.TypeMux          { return n.eventMux }
func (n *Nerthus) ChainDb() ntsdb.Database           { return n.chainDb }
func (n *Nerthus) DagChain() *core.DagChain          { return n.dagchain }
func (n *Nerthus) Genesis() *core.Genesis            { return n.config.Genesis }
func (n *Nerthus) Dag() *core.DAG                    { return n.dag }

func (n *Nerthus) IsListening() bool  { return true } // Always listening
func (n *Nerthus) NtsVersion() int    { return 1 }    // TODO(ysqi): 暂时标记为1，在实现协议管理器后再返回实际版本
func (n *Nerthus) NetVersion() uint64 { return n.networkId }

// GetMainAccount 获取主账号
func (n *Nerthus) GetMainAccount() common.Address {
	return n.mainAccount
}

// Protocols implements node.Service, returning all the currently configured
// network protocols to start.
func (n *Nerthus) Protocols() []p2p.Protocol {
	if n.ProtocolHandler != nil {
		return n.ProtocolHandler.SubProtocols
	}
	return []p2p.Protocol{}
}

// Start implements node.Service, starting all internal goroutines needed by the
// Nerthus protocol implementation.
func (n *Nerthus) Start(srvr *p2p.Server) error {
	n.netRPCService = ntsapi.NewPublicNetAPI(srvr, n.NetVersion())
	if n.lesServer != nil {
		n.lesServer.Start(srvr)
	}
	if n.sta != nil {
		if err := n.sta.Start(); err != nil {
			return err
		}
		go n.startHandleTx()
	}

	if n.ProtocolHandler != nil {
		if err := n.ProtocolHandler.Start(); err != nil {
			return err
		}
	}
	return nil
}

// Stop implements node.Service, terminating all internal goroutines used by the
// Nerthus protocol.
func (n *Nerthus) Stop() error {
	if n.txPool != nil {
		n.txPool.Stop()
	}

	if n.stopDbUpgrade != nil {
		n.stopDbUpgrade()
	}
	if n.ProtocolHandler != nil {
		n.ProtocolHandler.Stop()
	}
	if n.dagchain != nil {
		n.dagchain.Stop()
	}
	if n.lesServer != nil {
		n.lesServer.Stop()
	}
	if n.witness != nil {
		n.witness.Stop()
	}
	if n.member != nil {
		n.member.Stop()
	}
	if n.eventMux != nil {
		n.eventMux.Stop()
	}
	if n.chainDb != nil {
		n.chainDb.Close()
	}
	if n.shutdownChan != nil {
		close(n.shutdownChan)
	}
	if n.sta != nil {
		n.sta.Stop()
	}
	if n.rd != nil {
		n.rd.Close()
	}
	log.Info("Nerthus stopped")
	return nil
}

func (nts *Nerthus) startHandleTx() {
	if nts.sta == nil {
		return
	}
	if nts.config.TxPool.Disabled {
		return
	}
	ch := make(chan types.TxPreEvent, 1024*10)
	sub := nts.txPool.SubscribeTxPreEvent(ch)
	for {
		select {
		case <-sub.Err():
			return
		case ev := <-ch:
			if err := nts.sta.AfterNewTx(ev.IsLocal, ev.Tx); err != nil {
				log.Error("failed write relation", "txhash", ev.Tx.Hash(), "err", err)
			}
		}
	}
}

// CreateDB 创建DAG数据库
func CreateDB(ctx *node.ServiceContext, config *Config, name string) (ntsdb.Database, error) {
	db, err := ctx.OpenDatabase(name, config.DatabaseCache, config.DatabaseHandles)
	if err != nil {
		return nil, err
	}
	if db, ok := db.(*ntsdb.LDBDatabase); ok {
		db.Meter("nts/db/dagdata/")
	}
	return db, nil
}
func CreateRD(ctx *node.ServiceContext, config *Config, name string) (ntsdb.RD, error) {
	return ctx.OpenRD(name)
}
