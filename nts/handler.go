package nts

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"gitee.com/nerthus/nerthus/accounts"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/chancluster"
	"gitee.com/nerthus/nerthus/common/mclock"
	"gitee.com/nerthus/nerthus/common/safemap"
	"gitee.com/nerthus/nerthus/common/tracetime"
	"gitee.com/nerthus/nerthus/common/utils"
	"gitee.com/nerthus/nerthus/consensus"
	cprotocol "gitee.com/nerthus/nerthus/consensus/protocol"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/clean"
	"gitee.com/nerthus/nerthus/core/config"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/council/arbitration"
	"gitee.com/nerthus/nerthus/crypto/sha3"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/nts/pman"
	"gitee.com/nerthus/nerthus/nts/protocol"
	"gitee.com/nerthus/nerthus/nts/synchronize"
	"gitee.com/nerthus/nerthus/nts/txpipe"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/p2p"
	"gitee.com/nerthus/nerthus/p2p/discover"
	"gitee.com/nerthus/nerthus/params"

	"github.com/hashicorp/golang-lru"
)

const DisableBroadcastTx = false //是否禁用广播交易

// errIncompatibleConfig is returned if the requested protocols and configs are
// not compatible (low protocol version restrictions and high requirements).
var errIncompatibleConfig = errors.New("incompatible configuration")

const (
	defMessageTTL = 12 //seconds
)

type ProtocolConfig struct {
	NetworkId         uint64
	MaxPeers          int
	MaxMessageSize    uint32
	SubPipeListenAddr string

	SyncMode protocol.SyncMode
	FastSync uint32 // Flag whether fast sync is enabled (gets disabled if we already have blocks)

	AcceptTxs uint32 // Flag whether we're considered synchronised (enables transaction processing)

	GenesisHash common.Hash
}

type ProtocolManager struct {
	config *ProtocolConfig
	P2PSrv *p2p.Server
	engine consensus.Engine

	ntsAPI      *NtsApiBackend
	txpool      txPool
	dagchain    *core.DagChain
	chaindb     ntsdb.Database
	chainconfig *params.ChainConfig

	realTimeSync *synchronize.SyncMain

	stateCleaner *clean.StateDBCleaner

	SubProtocols []p2p.Protocol

	eventMux      *event.TypeMux
	messageFilter *sync.Map
	srvSync       chan *types.SyncDagMsg
	syncLimit     safemap.SafeDoubleKeyMap
	lockSync      chan struct{}
	quit          chan struct{}

	// wait group is used for graceful shutdowns during downloading
	// and processing
	wg            sync.WaitGroup
	running       int32 //是否是运行中
	onlyUseInTest func(p *pman.Peer, message p2p.Msg, result error)
	newPeerCh     chan *pman.Peer
	peers         *pman.PeerSet
	minedBlockSub *event.TypeMuxSubscription

	knownTx     *lru.Cache
	chanCluster *chancluster.ChanCluster
	subPipe     *txpipe.Pipe
	txSync      *txpipe.TxSync
}

func NewProtocolManager(ser *p2p.Server, cfg *ProtocolConfig, config *params.ChainConfig, mux *event.TypeMux, txpool txpipe.TxPool,
	dagchain *core.DagChain, chaindb ntsdb.Database, ntsAPI *NtsApiBackend) (*ProtocolManager, error) {
	// Create the protocol manager with the base fields
	manager := &ProtocolManager{
		eventMux:      mux,
		txpool:        txpool,
		dagchain:      dagchain,
		chaindb:       chaindb,
		chainconfig:   config,
		config:        cfg,
		ntsAPI:        ntsAPI,
		messageFilter: new(sync.Map),
		peers:         pman.NewPeerSet(),
		newPeerCh:     make(chan *pman.Peer),
		quit:          make(chan struct{}),
		srvSync:       make(chan *types.SyncDagMsg, cfg.MaxPeers*10),
		syncLimit:     safemap.NewBuckets(0, 0),
		P2PSrv:        ser,
	}

	// 开启子通道
	if !DisableBroadcastTx {
		manager.subPipe = txpipe.New(txpipe.Config{
			ListenAddr: cfg.SubPipeListenAddr, PrivateKey: ser.PrivateKey,
			NAT:      ser.NAT,
			MaxPeers: ser.MaxPeers,
		}, manager.peers, txpool)
	}
	// Initiate a sub-protocol for every implemented version we can handle
	manager.SubProtocols = make([]p2p.Protocol, 0, len(protocol.ProtocolVersions))
	for _, version := range protocol.ProtocolVersions {
		// Compatible; initialise the sub-protocol
		manager.SubProtocols = append(manager.SubProtocols, p2p.Protocol{
			Name:    ProtocolName,
			Version: version.ID,
			Length:  version.MaxCode,
			Run: func(p *p2p.Peer, rw p2p.MsgReadWriter) error {
				manager.wg.Add(1)
				defer manager.wg.Done()

				peer, err := manager.initPeer(version.ID, p, rw)
				if err != nil {
					return err
				}
				return manager.handle(peer)
			},
			NodeInfo: func() interface{} {
				return manager.NodeInfo()
			},
			PeerInfo: func(id discover.NodeID) interface{} {
				if p := manager.peers.Peer(id); p != nil {
					return p.Info()
				}
				return nil
			},
		})
	}
	if len(manager.SubProtocols) == 0 {
		return nil, errIncompatibleConfig
	}
	if ntsAPI != nil {
		var err error
		manager.realTimeSync, err = synchronize.New(manager, manager, ntsAPI.ChainDb(), ntsAPI.GetRD(), dagchain)
		if err != nil {
			return nil, err
		}
		// 是否开启自动瘦身
		manager.stateCleaner = clean.NewCleaner(ntsAPI.ChainDb(), ntsAPI.DagChain())
	}

	manager.regMsgHandle()

	return manager, nil
}

func (ph *ProtocolManager) Protocols() []p2p.Protocol {
	return ph.SubProtocols
}

// Start 启动协议通讯处理，如果启动失败则返回错误信息
// 注意：不得重复启动
func (pm *ProtocolManager) Start() error {
	if !atomic.CompareAndSwapInt32(&pm.running, 0, 1) {
		return errors.New("is running")
	}

	if pm.txSync != nil {
		pm.txSync.Start()
	}
	// broadcast mined blocks
	pm.minedBlockSub = pm.eventMux.SubscribeWithSize(types.NewWitnessedUnitEvent{}, 5000)
	go pm.minedBroadcastLoop()

	if pm.subPipe != nil {
		if err := pm.subPipe.Run(); err != nil {
			return fmt.Errorf("failed to start tx pipe,%v", err)
		}
	}
	// 接收待广播事件
	go pm.broadcastLoop()

	// 开启p2p消息分发管道
	pm.chanCluster.Start()

	if pm.realTimeSync != nil {
		pm.realTimeSync.Start()
	}

	if pm.stateCleaner != nil {
		pm.stateCleaner.Start()
	}

	atomic.StoreInt32(&pm.running, 1)
	return nil
}

// Stop 启动协议通讯处理，如果启动失败则返回错误信息
func (ph *ProtocolManager) Stop() error {
	defer log.Info("Protocol handler stopped")
	if !atomic.CompareAndSwapInt32(&ph.running, 1, 0) {
		return errors.New("is not running")
	}

	close(ph.quit)
	if ph.txSync != nil {
		ph.txSync.Stop()
	}
	ph.peers.Close()
	if ph.subPipe != nil {
		ph.subPipe.Close()
	}
	//ph.voteSync.Stop()
	if ph.realTimeSync != nil {
		ph.realTimeSync.Stop()
	}
	if ph.stateCleaner != nil {
		ph.stateCleaner.Stop()
	}

	if ph.minedBlockSub != nil {
		ph.minedBlockSub.Unsubscribe()
	}

	close(ph.srvSync)
	ph.chanCluster.Stop()

	ph.wg.Wait()
	return nil
}

// BroadcastTxs will propagate a batch of transactions to all peers which are not known to
// already have the given transaction.
func (pm *ProtocolManager) BroadcastTxs(tx *types.Transaction) {
	peers := pm.peers.PeersWithoutTx(tx.Hash())
	log.Trace("Broadcast transaction", "hash", tx.Hash(), "recipients", len(peers))
	for _, peer := range peers {
		peer.AsyncSendTransactions([]*types.Transaction{tx})
	}
}
func (pm *ProtocolManager) DagChain() *core.DagChain {
	return pm.dagchain
}

// Mined broadcast loop
func (pm *ProtocolManager) minedBroadcastLoop() {
	// automatically stops if unsubscribe
	for obj := range pm.minedBlockSub.Chan() {
		if ev, ok := obj.Data.(types.NewWitnessedUnitEvent); ok {
			pm.BroadcastUnit(ev.Unit)
		}
	}
}

// BroadcastBlock will either propagate a block to a subset of it's peers, or
// will only announce it's availability (depending what's requested).
func (pm *ProtocolManager) BroadcastUnit(unit *types.Unit) {
	now := time.Now()
	hash := unit.Hash()
	peers := pm.peers.PeersWithoutBlock(hash)

	for _, peer := range peers {
		peer.AsyncSendNewBlock(unit)
	}
	log.Trace("Propagated unit", "hash", hash, "recipients", len(peers), "duration", common.PrettyDuration(time.Since(now)))
}

// broadcastLoop 处理待广播事件
func (ph *ProtocolManager) broadcastLoop() {
	ph.wg.Add(1)
	defer func() {
		ph.wg.Done()
	}()

	sub := ph.eventMux.Subscribe(
		&types.SendDirectEvent{},
		&types.SendBroadcastEvent{},
		types.NewBroadcastEvent{},
		types.SendDirectPeersEvent{})

	handler := func(obj *event.TypeMuxEvent) error {
		log.Trace("handler need broadcast subscribe event", "type", fmt.Sprintf("%T", obj.Data), "eventCreatedTime", obj.Time.Unix(), "fromNow", common.PrettyDuration(time.Now().Sub(obj.Time)))
		switch ev := obj.Data.(type) {
		default:
			panic(fmt.Errorf("unknown event:%T", obj.Data))
		case *types.SendDirectEvent:
			if ev.PeerID.IsEmpty() {
				for _, p := range ph.peers.Peers() {
					if err := p.SendAnyData(ev.Code, ev.Data); err != nil {
						return err
					}
				}
			} else {
				if p := ph.peers.Peer(ev.PeerID); p != nil {
					return p.SendAnyData(ev.Code, ev.Data)
				}
			}
		case *types.SendBroadcastEvent:
			if err := ph.peers.BroadcastWithTTL(ev.Code, defMessageTTL, ev.Data); err != nil {
				log.Error("send ttl failed", "err", err)
			}
		case types.NewBroadcastEvent: // 新的广播协议事件
			if ev.Peer.IsEmpty() {
				for _, p := range ph.peers.Peers() {
					if err := p.SendAnyData(protocol.NewBroadcastMsg, ev.Msg); err != nil {
						return err
					}
				}
			} else {
				if p := ph.peers.Peer(ev.Peer); p != nil {
					return p.SendAnyData(protocol.NewBroadcastMsg, ev.Msg)
				}
			}
		case types.SendDirectPeersEvent:
			var sends int
			for _, pid := range ev.Peers {
				if p := ph.peers.Peer(pid); p == nil {
					log.Trace("missing peer", "peer", pid, "code", protocol.CodeString(ev.Code))
				} else {
					if err := p.SendAnyData(ev.Code, ev.Msg); err != nil {
						return err
					}
					sends++
				}
			}
			bftMsg, ok := ev.Msg.(cprotocol.BFTMsg)
			if ok {
				log.Trace("broadcast bft message done", "mid", utils.New64a(bftMsg.Payload.([]byte)))
				log.Trace("broadcast consensus message", "peers", sends, "chainAddress", bftMsg.ChainAddress, "msgHash", bftMsg.Hash())
			}
		}
		return nil
	}

	defer sub.Unsubscribe()
	readC := sub.Chan()
	for {
		select {
		case <-ph.quit:
			return
		case obj, ok := <-readC:
			if !ok {
				return
			}
			if err := handler(obj); err != nil {
			}
		}
	}
}

// 初始化 Peer 对象，在初始化前将检查协议等信息
func (pm *ProtocolManager) initPeer(version uint, p *p2p.Peer, rw p2p.MsgReadWriter) (*pman.Peer, error) {
	select {
	case <-pm.quit:
		return nil, errors.New("stopped")
	default:
	}
	if pm.peers.Peer(p.ID()) != nil {
		return nil, pman.RrrAlreadyRegistered
	}

	var (
		genesis     = pm.config.GenesisHash
		headID      types.UnitID
		subPipeNode *discover.Node
	)
	if genesis.Empty() && pm.dagchain != nil {
		genesis = pm.dagchain.Genesis().Hash()
	}
	if pm.dagchain != nil {
		headID = pm.dagchain.GetChainTailHead(params.SCAccount).ID()
	}
	if pm.subPipe != nil {
		subPipeNode = pm.subPipe.Node()
	}

	if rw, ok := rw.(*pman.MeteredMsgReadWriter); ok {
		rw.Init(int(version))
	}

	// 握手
	peerStatus, err := pman.Handshake(rw, uint32(version), pm.config.NetworkId, headID, genesis, subPipeNode)
	if err != nil {
		return nil, fmt.Errorf("handshake failed: %v", err)
	}

	//只有在握手成功后，才创建实例
	var txPipeConns *pman.PeerSet
	if pm.subPipe != nil {
		txPipeConns = pm.subPipe.PeerSet()
	}
	peer := pman.NewPeerHandler(int(version), p, rw, pman.DisableTx, txPipeConns)
	peer.Set(pman.CfgSubPipeNode, peerStatus.SubPipeNode)
	return peer, nil
}

// handle is the callback invoked to manage the life cycle of an eth peer. When
// this function terminates, the peer is disconnected.
func (pm *ProtocolManager) handle(p *pman.Peer) error {

	p.Log().Debug("peer connected", "name", p.Name())

	// Register the peer locally
	if err := pm.peers.Register(p); err != nil {
		p.Log().Debug("peer registration failed", "err", err)
		return err
	}

	defer pm.removePeer(p.ID())

	// Propagate existing transactions. new transactions appearing
	// after this will be sent via broadcasts.
	if pm.txSync != nil {
		pm.txSync.Sync(p, time.Time{})
	}

	// main loop. handle incoming messages.
	for {

		// Read the next message from the remote peer, and ensure it's fully consumed
		msg, err := p.ReadMsg()
		if err != nil {
			p.Log().Debug("message handling failed", "err", err)
			msg.Discard() //如果不能处理消息，则需要释放消息内容
			return err
		}
		if DisableBroadcastTx && uint8(msg.Code) == protocol.TxMsg {
			msg.Discard()
			continue
		}
		p.Log().Trace("received message", "code", protocol.CodeString(uint8(msg.Code)))
		err = protocol.CheckMsg(msg)
		if err != nil {
			msg.Discard() //如果不能处理消息，则需要释放消息内容
			p.Log().Debug("message handling failed", "err", err)
			switch err {
			case protocol.ErrorOldMsg:
				continue
			default:
				return err
			}
		}
		// 消息分发 异步
		err = pm.handleMsg(p, &msg)
		if err != nil {
			p.Log().Error("p2p handle msg error", "code", msg.Code, "err", err)
		}
	}
}

// handleMsg is invoked whenever an inbound message is received from a remote
// peer. The remote connection is torn down upon returning any error.
func (pm *ProtocolManager) handleMsg(p *pman.Peer, msg *p2p.Msg) error {
	return pm.chanCluster.HandleWithContext(p.HandleContext(), uint8(msg.Code), p, msg, mclock.Now())
}

type msgHandlef func(tr *tracetime.TraceTime, p *pman.Peer, msg *p2p.Msg) (hash common.Hash, err error)

// 注入msgcode处理，通过包装使用。
// 该注入时需要设定所需的并发处理容量，以及可以进行耗时跟踪
func (pm *ProtocolManager) regMsgHandle() {
	// 注册消息处理管道  管道缓冲长度1024*1024
	pm.chanCluster = chancluster.NewChanCluster()

	fs := pm.msgHandles()

	bufPool := sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(nil)
		},
	}
	//并发注入所有处理函数
	for code, _ := range fs {
		g, cache := getMsgSetting(code)

		//不直接注入，而是进行封装，方便处理公共部分
		pm.chanCluster.Register(code, g, cache, func(args []interface{}) error {
			var (
				peer, msg    = args[0].(*pman.Peer), args[1].(*p2p.Msg)
				playloadBuf  *bytes.Buffer
				playloadHash common.Hash
			)
			if msg.TTL > 0 { //如果大于零，说明是需要广播的消息。此时需要复制一份消息
				playloadBuf = bufPool.Get().(*bytes.Buffer)
				defer func() {
					playloadBuf.Reset()
					bufPool.Put(playloadBuf)
				}()

				_, err := io.CopyN(playloadBuf, msg.Payload, int64(msg.Size))
				if err != nil {
					return err
				}
				//如果是TTL消息，则在处理前判断该消息是否已经处理过，如果是则跳过
				playloadHash, err = messagePayloadHash(playloadBuf.Bytes())
				if err != nil {
					return fmt.Errorf("failed reward message,%v", err)
				}
				//标记已处理
				if peer.MarkTTLMsg(playloadHash) {
					peer.Log().Trace("skip known TTL msg", "hash", playloadHash)
					return nil
				}
				//重新放置
				msg.Payload = bytes.NewBuffer(playloadBuf.Bytes())
			}

			dataHash, err := fs[uint8(msg.Code)](tracetime.New(), peer, msg)

			msg.Discard() //如果不能处理消息，则需要释放消息内容

			if err != nil && params.RunMode == "dev" {
				log.Error("handle message failed", "code", protocol.CodeString(uint8(msg.Code)), "createTime", msg.Expiry-msg.TTL,
					"receivedAt", msg.ReceivedAt, "msg.size", msg.Size, "err", err)
			} else {
				peer.Log().Trace("received message and process",
					"code", protocol.CodeString(uint8(msg.Code)), "createTime", msg.Expiry-msg.TTL,
					"receivedAt", msg.ReceivedAt, "msg.size", msg.Size,
					"data.hash", dataHash,
					"ttl", msg.TTL,
					"err", err)
			}

			if pm.onlyUseInTest != nil { // testcase中使用
				pm.onlyUseInTest(peer, *msg, err)
			}

			//出现不应该的错误，断开连接
			if err != nil {
				if err, ok := err.(*protocol.ErrMsg); ok {
					peer.Log().Debug("need close peer", "reason", err)
					//断开连接
					pm.removePeer(peer.ID())
				}
				return err
			}
			if playloadBuf != nil && playloadBuf.Len() > 0 && msg.TTL > 0 {
				//如果未过期，则广播
				now := uint32(time.Now().Unix() + 1)
				if msg.Expiry < now {
					peer.Log().Trace("message expired", "code", msg.Code)
				} else {
					peers := pm.peers.PeersWithoutPayloadHash(playloadHash)
					for _, to := range peers {
						if to.ID() != peer.ID() { //转发给其他人
							cpMsg := protocol.TTLMsg{
								Msg: p2p.Msg{
									Code:    msg.Code,
									Size:    msg.Size,
									TTL:     msg.TTL,
									Expiry:  msg.Expiry,
									Payload: bytes.NewReader(playloadBuf.Bytes()),
								},
								PayloadHash: playloadHash,
							}
							to.Log().Trace("reward TTL message",
								"code", protocol.CodeString(uint8(msg.Code)),
								"ttl", cpMsg.TTL, "expiry", cpMsg.Expiry)
							to.SendTTLMsg(cpMsg)
						}
					}
				}
			}
			return nil
		}, nil)
	}
}

func (pm *ProtocolManager) msgHandles() map[uint8]msgHandlef {
	return map[uint8]msgHandlef{
		protocol.RealTimeSyncMsg: func(tr *tracetime.TraceTime, p *pman.Peer, msg *p2p.Msg) (hash common.Hash, err error) {
			if pm.realTimeSync == nil {
				return hash, nil
			}
			err = pm.realTimeSync.HandleMessage(p.ID(), msg)
			if err != nil {
				return hash, protocol.Error(protocol.ErrInvalidPayload, "%s:%v", msg.String(), err)
			}
			return hash, nil
		},
		protocol.BftMsg: func(tr *tracetime.TraceTime, p *pman.Peer, msg *p2p.Msg) (hash common.Hash, err error) {
			var bftMsg cprotocol.BFTMsg
			if err = msg.Decode(&bftMsg); err != nil {
				return hash, protocol.Error(protocol.ErrDecode, "%s: %v", msg.String(), err)
			}
			if bftMsg.ChainAddress.Empty() {
				return hash, protocol.Error(protocol.ErrInvalidPayload, "")
			}
			//tr.AddArgs("cost2", time.Since(time.Unix(0, int64(bftMsg.Create))))
			if _, ok := bftMsg.Payload.([]byte); !ok {
				return hash, protocol.Error(protocol.ErrInvalidPayload, "invalid bft playload")
			}
			if pm.eventMux != nil {
				submsg := new(cprotocol.Message)
				submsg.P2PTime = time.Now()
				if err := submsg.FromPayload(bftMsg.Payload.([]byte), nil); err != nil {
					return hash, protocol.Error(protocol.ErrDecode, "bftSubMessage: %v", err)
				}
				submsg.From = p
				p.Log().Trace("get bft message", "mid", submsg.ID(), "receivedAt", msg.ReceivedAt)
				pm.ntsAPI.HandleConsensusEvent(cprotocol.MessageEvent{
					Chain:           bftMsg.ChainAddress,
					Message:         submsg,
					P2PReceivedTime: msg.ReceivedAt,
					PushTime:        time.Now(),
				})
				tr.AddArgs("mid", submsg.ID(), "code", submsg.Code, "chain", submsg.Address)
				return common.HexToHash(submsg.ID()), nil
			}
			return
		},
		protocol.ChooseLastUnitMsg: // 见证人高度投票
		func(tr *tracetime.TraceTime, p *pman.Peer, msg *p2p.Msg) (hash common.Hash, err error) {
			if pm.ntsAPI == nil || !pm.ntsAPI.nts.IsWitnessing() {
				return hash, protocol.Error(protocol.ErrInvalidMsgCode, "no mining")
			}
			err = pm.ntsAPI.nts.witness.HandleMsg(p.ID(), msg)
			if err != nil {
				return hash, protocol.Error(protocol.ErrInvalidPayload, "%s: %v", msg.String(), err)
			}
			return hash, nil
		},
		protocol.NewUnitMsg: func(tr *tracetime.TraceTime, p *pman.Peer, msg *p2p.Msg) (hash common.Hash, err error) {
			if pm.dagchain == nil {
				return hash, errors.New("dagchain is nil,ignore")
			}
			// 落地单元
			// 落地后将数据填入事件队列，分发给需要处理的业务
			// 处理单元
			unit := new(types.Unit)
			if err = msg.Decode(unit); err != nil {
				return hash, protocol.Error(protocol.ErrDecode, "%s: %v", msg.String(), err)
			}

			unit.ReceivedAt = msg.ReceivedAt
			unit.ReceivedFrom = p.ID()
			if err := pm.dagchain.InsertUnit(unit); err != nil {
				log.Trace("failed to insert unit", "chain", unit.MC(), "number", unit.Number(), "err", err)
				//校验单元的合法性，如果非法则断开连接
				if consensus.IsBadUnitErr(err) {
					return unit.Hash(), protocol.Error(protocol.ErrInvalidPayload, "%s: %v", msg.String(), err)
				}
				return common.Hash{}, nil
			}
			tr.AddArgs("uhash", unit.Hash(), "chain", unit.MC(), "number", unit.Number())
			p.MarkBlock(unit.Hash())
			return unit.Hash(), err
		},
		protocol.WitnessStatusMsg: func(tr *tracetime.TraceTime, p *pman.Peer, msg *p2p.Msg) (hash common.Hash, err error) {
			//见证人状态信息是全网广播，本地不能处理时直接忽略
			if err := pm.ntsAPI.nts.witnessNodesControl.Handle(p.ID(), msg); err != nil {
				return hash, protocol.Error(protocol.ErrInvalidPayload, "%s: %v", msg.String(), err)
			}
			return hash, nil
		},
		protocol.BadWitnessProof: func(tr *tracetime.TraceTime, p *pman.Peer, msg *p2p.Msg) (hash common.Hash, err error) {
			var info []types.AffectedUnit
			err = msg.Decode(&info)
			if err != nil {
				return hash, protocol.Error(protocol.ErrDecode, "%s: %v", msg.String(), err)
			}
			err = pm.dagchain.ReportBadUnit(info)
			if err != nil {
				return hash, protocol.Error(protocol.ErrInvalidPayload, "%s: %v", msg.String(), err)
			}
			return hash, err
		},
		protocol.CouncilMsg: func(tr *tracetime.TraceTime, p *pman.Peer, msg *p2p.Msg) (hash common.Hash, err error) {
			var info arbitration.CouncilMsgEvent
			err = msg.Decode(&info)
			if err != nil {
				return hash, protocol.Error(protocol.ErrDecode, "%s: %v", msg.String(), err)
			}
			if pm.ntsAPI == nil || pm.ntsAPI.nts.Council() == nil {
				sender, err := types.Sender(types.NewSigner(pm.chainconfig.ChainId), info.Message)
				if err != nil {
					return hash, protocol.Error(protocol.ErrInvalidPayload, "%s: %v", msg.String(), err)
				}
				if sender != info.Message.From {
					return hash, protocol.Error(protocol.ErrInvalidPayload, "%s: %v", msg.String(), types.ErrInvalidSig)
				}
				// 验证发送方是不是有效理事
				statedb, err := pm.dagchain.GetChainTailState(params.SCAccount)
				if err != nil {
					return hash, nil
				}
				cils, err := sc.GetCouncilList(statedb)
				if err != nil {
					return hash, nil
				}
				if !common.AddressList(cils).Have(sender) {
					return hash, protocol.Error(protocol.ErrInvalidPayload, "%s: %v", msg.String(), errors.New("invalid council address"))
				}
				return hash, nil
			}
			if err := pm.ntsAPI.nts.Council().HandleArbitratMsg(info); err != nil {
				return hash, protocol.Error(protocol.ErrInvalidPayload, "%s: %v", msg.String(), err)
			}
			return
		},
	}
}

func (pm *ProtocolManager) removePeer(id discover.NodeID) {
	// Short circuit if the peer was already removed
	peer := pm.peers.Peer(id)
	if peer == nil {
		return
	}
	log.Debug("Removing peer", "peer", id)

	// Unregister the peer from the downloader and Ethereum peer set
	//pm.downloader.UnregisterPeer(id)
	if err := pm.peers.Unregister(id); err != nil {
		log.Error("SyncPeer removal failed", "peer", id, "err", err)
	}
	// Hard disconnect at the networking layer
	if peer != nil {
		peer.Peer.Disconnect(p2p.DiscUselessPeer)
	}
}

// NodeInfo represents a short summary of the Ethereum sub-protocol metadata
// known about the host peer.
type NodeInfo struct {
	Network      uint64            `json:"network"`      // Ethereum network ID
	Genesis      common.Hash       `json:"genesis"`      // SHA3 hash of the host's genesis block
	ChainConfig  map[string]uint64 `json:"chain_config"` // Chain configuration for the fork rules
	SysChainHead types.UnitID      `json:"sys_chain_head"`
	SubPipe      string            `json:"subpipe"`
}

// NodeInfo retrieves some protocol metadata about the running host node.
func (pm *ProtocolManager) NodeInfo() *NodeInfo {
	currentBlock := pm.dagchain.GetChainTailHead(params.SCAccount)
	info := NodeInfo{
		Network:      pm.config.NetworkId,
		Genesis:      pm.dagchain.GenesisHash(),
		ChainConfig:  pm.dagchain.Config().GetConfigs(),
		SysChainHead: currentBlock.ID(),
	}
	if pm.subPipe != nil {
		info.SubPipe = pm.subPipe.Node().String()
	}
	return &info
}

func messagePayloadHash(payload []byte) (h common.Hash, err error) {
	hw := sha3.NewKeccak256()
	_, err = hw.Write(payload)
	if err != nil {
		return h, err
	}
	hw.Sum(h[:0])
	sha3.PutKeccak256(hw)
	return h, nil
}

// 本地所有账户

func (pm *ProtocolManager) Accounts() []common.Address {
	if pm.ntsAPI == nil {
		return nil
	}
	addresses := make([]common.Address, 0) // return [] instead of nil if empty
	for _, wallet := range pm.ntsAPI.AccountManager().Wallets() {
		for _, account := range wallet.Accounts() {
			addresses = append(addresses, account.Address)
		}
	}
	return addresses
}

func (pm *ProtocolManager) SubscribeAccount(c chan<- accounts.WalletEvent) event.Subscription {
	return pm.ntsAPI.AccountManager().Subscribe(c)
}

func (pm *ProtocolManager) IsJoinWitnessGroup(address common.Address) bool {
	return pm.ntsAPI.IsJoinWitnessGroup(address)
}

// 订阅节点
func (pm *ProtocolManager) SubscribePeerEvent(ch chan protocol.PeerEvent) event.Subscription {
	return pm.peers.SubscribePeerEvent(ch)
}

func (pm *ProtocolManager) DisconnectPeer(peerID []byte) {
	id, err := discover.BytesID(peerID)
	if err != nil {
		return
	}
	pm.removePeer(id)
}

func (pm *ProtocolManager) RemoveWhitelist(peerID []byte) {
	id, err := discover.BytesID(peerID)
	if err != nil {
		return
	}
	pm.P2PSrv.RemoveWhitelist(id)
}
func (pm *ProtocolManager) SetWhitelist(peerID []byte) {
	id, err := discover.BytesID(peerID)
	if err != nil {
		return
	}
	pm.P2PSrv.SetWhitelist(id)
}

func (pm *ProtocolManager) CurrentMiner() common.Address {
	if pm.ntsAPI == nil {
		return common.Address{}
	}
	return pm.ntsAPI.GetMainAccount()
}

// GetWitnessWorkChains 返回指定见证人下所有用户链
func (pm *ProtocolManager) GetWitnessChains(witness common.Address) ([]common.Address, error) {
	if pm.dagchain == nil {
		return nil, errors.New("dagchain is nil,ignore")
	}
	return pm.dagchain.GetWitnessInfoChainList(witness)
}

func (pm *ProtocolManager) AddPeer(node *discover.Node) error {
	pm.P2PSrv.AddPeer(node)
	return nil
}
func (pm *ProtocolManager) PeerSelf() *discover.Node {
	return pm.P2PSrv.Self()
}
func (pm *ProtocolManager) PeerCount() int {
	return pm.peers.Len()
}
func (pm *ProtocolManager) Peers() []protocol.Peer {
	s := pm.peers.Peers()
	list := make([]protocol.Peer, len(s))
	for i, v := range s {
		list[i] = v
	}
	return list
}
func (pm *ProtocolManager) Peer(id discover.NodeID) protocol.Peer {
	p := pm.peers.Peer(id)
	if p == nil {
		var p2 protocol.Peer
		return p2
	}
	return p
}
func (pm *ProtocolManager) JoinWhitelist(id discover.NodeID) {
	pm.P2PSrv.SetWhitelist(id)
}

func (pm *ProtocolManager) ClearWhitelist(id discover.NodeID) {
	pm.removePeer(id)
	pm.P2PSrv.RemoveWhitelist(id)
}

// BroadcastMsg 直接广播消息给所有已连接的节点
func (pm *ProtocolManager) BroadcastMsg(code uint8, data interface{}) error {
	for _, p := range pm.peers.Peers() {
		if err := p.SendAnyData(code, data); err != nil {
			return err
		}
	}
	return nil
}

// SendMsg 直接发送消息给指定节点
func (pm *ProtocolManager) SendMsg(peer discover.NodeID, code uint8, data interface{}) error {
	p := pm.peers.Peer(peer)
	if p == nil {
		return errors.New("not found peer")
	}
	return p.SendAnyData(code, data)
}

var defaultMsgSetting = map[uint8]struct {
	parallel, cacheSize int
}{
	0:               {1, 1000}, //缺省选项
	protocol.BftMsg: {5, 1024000},
}

func getMsgSetting(code uint8) (parallel, cacheSize int) {
	//设置允许的并发量，根据code配置。默认为1
	getP := func(code uint8) int {
		v := config.MustInt(fmt.Sprintf("runtime.p2p_msg_parallel_%s", protocol.CodeString(code)), 0)
		if v != 0 { //说明有设置值
			return v
		}
		//否则使用默认值
		if c, ok := defaultMsgSetting[code]; ok {
			return c.parallel
		}
		return defaultMsgSetting[0].parallel
	}
	getSize := func(code uint8) int {
		v := config.MustInt(fmt.Sprintf("runtime.p2p_msg_cache_%s", protocol.CodeString(code)), 0)
		if v != 0 { //说明有设置值
			return v
		}
		//否则使用默认值
		if c, ok := defaultMsgSetting[code]; ok {
			return c.cacheSize
		}
		return defaultMsgSetting[0].cacheSize
	}
	return getP(code), getSize(code)
}
