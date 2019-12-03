package txpipe

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru"

	"gitee.com/nerthus/nerthus/common/chancluster"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/nts/pman"
	"gitee.com/nerthus/nerthus/nts/protocol"
	"gitee.com/nerthus/nerthus/p2p"
	"gitee.com/nerthus/nerthus/p2p/discover"
	"gitee.com/nerthus/nerthus/p2p/nat"
)

const (
	pname = "txpipe"
)

type Config struct {
	PrivateKey *ecdsa.PrivateKey
	ListenAddr string
	NAT        nat.Interface `toml:",omitempty"` // NAT穿墙设置
	MaxPeers   int
}

type Pipe struct {
	mainPipe *pman.PeerSet
	peerSet  *pman.PeerSet

	serv        *p2p.Server
	peerRunw    sync.WaitGroup
	cfg         Config
	peers       sync.Map
	batchHandle *chancluster.ChanCluster
	txpool      TxPool
	txSync      *TxSync
	term        chan struct{}
	running     chan struct{}

	connectHistory *lru.Cache //key=peerid,value=close time

	txSlicePool sync.Pool
}

func New(cfg Config, mainPipe *pman.PeerSet, txpool TxPool) *Pipe {
	p := Pipe{
		cfg:      cfg,
		mainPipe: mainPipe,
		txpool:   txpool,
		peerSet:  pman.NewPeerSet(),
		term:     make(chan struct{}),
		running:  make(chan struct{}),

		txSlicePool: sync.Pool{
			New: func() interface{} {
				return make([]*types.Transaction, 0, 1) //实时交易转发是单条，同步时是批量
			},
		},
	}
	p.connectHistory, _ = lru.New(30)
	p.txSync = NewTxSync(txpool, func(ctx context.Context, txs types.Transactions) {
		p.peerSet.BroadcastTxs(ctx, txs)
	}, p.SendTransaction)
	return &p
}

func (pipe *Pipe) Node() *discover.Node {
	<-pipe.running

	return pipe.serv.Self()
}

func (pipe *Pipe) Run() error {
	servcfg := p2p.Config{
		PrivateKey:      pipe.cfg.PrivateKey,
		ListenAddr:      pipe.cfg.ListenAddr,
		NAT:             pipe.cfg.NAT,
		NoDial:          true,
		NoDiscovery:     false,
		MaxPeers:        pipe.cfg.MaxPeers,
		MaxPendingPeers: pipe.cfg.MaxPeers,
	}
	pipe.serv = &p2p.Server{Config: servcfg}

	//注入Peer处理
	pipe.serv.Protocols = append(pipe.serv.Protocols, p2p.Protocol{
		Name:    pname,
		Version: protocol.ProtocolVersions[0].ID,
		Length:  protocol.ProtocolVersions[0].MaxCode,
		Run: func(p *p2p.Peer, rw p2p.MsgReadWriter) error {
			pipe.peerRunw.Add(1)
			defer pipe.peerRunw.Done()
			return pipe.handle(pman.NewPeerHandler(int(protocol.ProtocolVersions[0].ID), p, rw, pman.OnlyTx, pipe.peerSet))
		},
	})
	err := pipe.serv.Start()
	if err != nil {
		return err
	}
	close(pipe.running)

	go pipe.run()
	return nil
}

func (pipe *Pipe) Close() {
	close(pipe.term)

	pipe.peerSet.Close()
	pipe.peerRunw.Wait()
}

func (pipe *Pipe) run() {
	pipe.txSync.Start()

	pipe.runBatchHandle()

	//订阅主通道
	peerEventc := make(chan protocol.PeerEvent, 25)
	peerSub := pipe.mainPipe.SubscribePeerEvent(peerEventc)

	for {
		select {
		case <-peerSub.Err():
			return
		case ev := <-peerEventc:
			//因为Peer是通过Add Static 加入因此需要主动释放，避免占用连接数
			pipe.serv.RemovePeer(ev.PeerID)
			//主通道关闭，则同时关闭此通道
			pipe.peerSet.Disconnect(ev.PeerID, p2p.DiscQuitting)
			if ev.Connected {
				p := pipe.mainPipe.Peer(ev.PeerID)
				if p != nil {
					pipe.addPeer(p)
				}
			}
		}
	}

}

func (pipe *Pipe) PeerSet() *pman.PeerSet {
	return pipe.peerSet
}

func (pipe *Pipe) SendTransaction(txs types.Transactions) error {
	peers := pipe.peerSet.Peers()
	if len(peers) == 0 {
		return errors.New("no peers")
	}
	for _, peer := range peers {
		if err := peer.SendTransactions(txs); err != nil {
			return err
		}
	}
	return nil
}

func (pipe *Pipe) addPeer(mainPeer *pman.Peer) {
	v := mainPeer.Get(pman.CfgSubPipeNode).(string)
	log.Debug("try connect", "node", v)
	if v != "" {
		n, err := discover.ParseNode(v)

		if err != nil {
			mainPeer.Close(p2p.DiscProtocolError)
		} else {
			pipe.serv.AddPeer(n)
		}
	}
}

func (pipe *Pipe) runBatchHandle() {
	pipe.batchHandle = chancluster.NewChanCluster()
	errFunc := func(p *pman.Peer, err error) {
		//只有在发送非法交易时才需要注销，其他情况下忽略
		if _, ok := err.(*core.ErrInvalidTxData); !ok {
			return
		}
		log.Error("disconnect peer", "peer", p, "err", err)
		p.Close(p2p.DiscProtocolError)
		pipe.peerSet.Unregister(p.ID())
		//坏节点，则全部断开
		pipe.mainPipe.Unregister(p.ID())
	}

	pipe.batchHandle.Register(struct{}{}, 10, 1024, func(args []interface{}) error {
		msg, p := args[0].(*p2p.Msg), args[1].(*pman.Peer)

		//如果交易池已满，则放弃处理此交易
		if pipe.txpool.IsFull() {
			msg.Discard()
			return nil
		}
		defer msg.Discard()

		txs := pipe.txSlicePool.Get().([]*types.Transaction)
		defer func() {
			//交易转发非常频繁，因此在接收到交易时使用临时的 Slice 来解析交易。降低 GC 和 slice 大小分配。
			//释放
			for i := 0; i < len(txs); i++ {
				txs[i] = nil
			}
			pipe.txSlicePool.Put(txs)
		}()
		if err := msg.Decode(&txs); err != nil {
			return protocol.Error(protocol.ErrDecode, "%s: %v", msg.String(), err)
		}
		if len(txs) == 0 {
			return protocol.Error(protocol.ErrInvalidPayload, "transaction list is empty")
		}

		for i, tx := range txs {
			if tx == nil {
				return protocol.Error(protocol.ErrInvalidPayload, "transaction %d is nil", i)
			}
			p.MarkTransaction(tx.Hash())
			p.Log().Trace("received tx", "tx", tx.Hash())
		}
		if err := pipe.txpool.AddRemotesTxpipe(txs); err != nil {
			return protocol.Error(protocol.ErrInvalidPayload, "%v", err)
		}
		return nil
	}, func(err error, args []interface{}) {
		if _, ok := err.(*protocol.ErrMsg); ok {
			p := args[1].(*pman.Peer)
			errFunc(p, err)
		}
	})

	//注册后启动
	pipe.batchHandle.Start()
}

func (pipe *Pipe) handle(peer *pman.Peer) error {
	//判断是否有相同主Peer
	id := peer.ID()
	if pipe.mainPipe.Peer(id) == nil {
		return errors.New("not found peer in main pipe")
	}
	if err := pipe.peerSet.Register(peer); err != nil {
		return err
	}
	var isBadPeer bool
	defer func() {
		pipe.connectHistory.Add(id, time.Now())
		pipe.peerSet.Disconnect(id, p2p.DiscProtocolError)
		if isBadPeer {
			//坏节点，则全部断开
			pipe.mainPipe.Disconnect(id, p2p.DiscProtocolError)
		}
	}()

	log.Debug("connected", "peer", id)
	//连接成功后需要首先推送交易
	//如果距离上次连接过短，则不
	var txBeginTime time.Time
	if before, ok := pipe.connectHistory.Get(id); ok {
		txBeginTime = before.(time.Time).Add(-time.Minute) //距离上次断开
	}
	pipe.txSync.Sync(peer, txBeginTime)

	for {
		msg, err := peer.ReadMsg()
		if err != nil {
			return err
		}
		if err := protocol.CheckMsg(msg); err != nil {
			isBadPeer = true
			return err
		}
		if msg.Code != uint64(protocol.TxMsg) {
			isBadPeer = true
			return protocol.Error(protocol.ErrInvalidMsgCode, "")
		}

		//如果交易池已满，则放弃处理此交易
		if pipe.txpool.IsFull() {
			msg.Discard()
			continue
		}
		err = pipe.batchHandle.Handle(struct{}{}, &msg, peer)
		if err != nil {
			return err
		}
	}
}
