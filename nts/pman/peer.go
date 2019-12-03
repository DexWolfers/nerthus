//已连接对等节点管理
package pman

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"sync"
	"time"

	"gitee.com/nerthus/nerthus/core/config"

	"gitee.com/nerthus/nerthus/common/utils"
	bft "gitee.com/nerthus/nerthus/consensus/protocol"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/set"
	"gitee.com/nerthus/nerthus/common/tracetime"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/nts/protocol"
	"gitee.com/nerthus/nerthus/p2p"
	"gitee.com/nerthus/nerthus/p2p/discover"
)

var (
	errNotRegistered     = errors.New("Peer is not registered")
	errClosed            = errors.New("Peer set is closed")
	RrrAlreadyRegistered = errors.New("Peer is already registered")
)

// 节点消息过滤标识
type PeerFlag int

const (
	DisableTx PeerFlag = 1 << iota
	OnlyTx
)

const (
	CfgSubPipeNode = "subPipeNode"
)

const (
	maxKnownTxs    = 32768 // Maximum transactions hashes to keep in the known list (prevent DOS)
	maxKnownBlocks = 3000  // Maximum block hashes to keep in the known list (prevent DOS)

	// maxQueuedTxs is the maximum number of transaction lists to queue up before
	// dropping broadcasts. This is a sensitive number as a transaction list might
	// contain a single transaction, or thousands.
	maxQueuedTxs = 128

	// maxQueuedProps is the maximum number of block propagations to queue up before
	// dropping broadcasts. There's not much point in queueing stale blocks, so a few
	// that might cover uncles should be enough.
	maxQueuedProps = 4

	maxQueuedConsensus = 2000 * 13

	// maxQueuedAnns is the maximum number of block announcements to queue up before
	// dropping broadcasts. Similarly to block propagations, there's no point to queue
	// above some healthy uncle limit, so use that.
	maxQueuedAnns = 4

	// 异步广播消息队列的上限
	maxQueueTTLMsgs = 128

	handshakeTimeout = 5 * time.Second
)

// statusData is the network packet for the status message.
type StatusData struct {
	ProtocolVersion  uint32      //协议版本
	NetworkId        uint64      //网络ID
	GenesisBlock     common.Hash //创世单元hash
	CurrSysChainUnit types.UnitID
	SubPipeNode      string //子通道连接地址
}

// PeerInfo represents a short summary of the Ethereum sub-protocol metadata known
// about a connected Peer.
type PeerInfo struct {
	Version    int                    `json:"version"`    // Ethereum protocol version negotiated
	Difficulty *big.Int               `json:"difficulty"` // Total difficulty of the Peer's blockchain
	Head       types.UnitID           `json:"head"`       // SHA3 hash of the Peer's best owned block
	Status     map[string]interface{} `json:"config"`
}

// propEvent is a block propagation, waiting for its turn in the broadcast queue.
type propEvent struct {
	block *types.Unit
	td    *big.Int
}

type peerType uint8

const (
	normalPeerType peerType = iota
	witnessPeerType
)

// 已连接节点信息
type Peer struct {
	*p2p.Peer
	id            discover.NodeID
	handleContext struct {
		ctx    context.Context
		cancel func()
	}
	status map[string]interface{}
	rw     p2p.MsgReadWriter
	flag   PeerFlag

	version  int         // Protocol version negotiated
	forkDrop *time.Timer // Timed connection dropper if forks aren't validated in time

	sysChainHead types.UnitID //当前该Peer的系统链高度
	lock         sync.RWMutex

	knownTTLMsg     *set.StringSet            // 广播时已知消息
	knownTxs        *set.StringSet            // StringSet of transaction hashes known to be known by this Peer
	knownBlocks     *set.StringSet            // StringSet of block hashes known to be known by this Peer
	queuedTxs       chan []*types.Transaction // Queue of transactions to broadcast to the Peer
	queuedProps     chan *propEvent           // Queue of blocks to broadcast to the Peer
	queuedConsensus chan interface{}          // Queue of consensus to broadcast to the Peer
	queuedAnns      chan *types.Unit          // Queue of blocks to announce to the Peer
	queuedTTLMsgs   chan protocol.TTLMsg
	term            chan struct{} // Termination channel to stop the broadcaster
	t               peerType

	txPipes *PeerSet
}

// 创建一个节点通信管理实例。
// 是对P2P节点的一次封装，已满足业务需求。
func NewPeerHandler(version int, p *p2p.Peer, rw p2p.MsgReadWriter, flag PeerFlag, txPipes *PeerSet) *Peer {

	peer := &Peer{
		Peer:        p,
		rw:          NewMeteredMsgWriter(rw),
		version:     version,
		id:          p.ID(),
		flag:        flag,
		txPipes:     txPipes,
		knownTxs:    set.NewStrSet(32, config.MustInt("runtime.peer_known_txs", maxKnownTxs)/32),
		knownBlocks: set.NewStrSet(32, config.MustInt("runtime.peer_known_units", maxKnownBlocks)/32),
		knownTTLMsg: set.NewStrSet(1, config.MustInt("runtime.peer_known_ttls", maxQueueTTLMsgs)),
		status:      make(map[string]interface{}),
		term:        make(chan struct{}),
	}

	//如果只允许发送交易则只需要创建交易接口
	if peer.opt(OnlyTx) {
		peer.queuedTxs = make(chan []*types.Transaction, config.MustInt("runtime.peer_broadcast_tx_queue", maxQueuedTxs))
	} else {
		peer.queuedProps = make(chan *propEvent, maxQueuedProps)
		peer.queuedConsensus = make(chan interface{}, maxQueuedConsensus)
		peer.queuedAnns = make(chan *types.Unit, maxQueuedAnns)
		peer.queuedTTLMsgs = make(chan protocol.TTLMsg, maxQueueTTLMsgs)
		if !peer.opt(DisableTx) {
			peer.queuedTxs = make(chan []*types.Transaction, maxQueuedTxs)
		}
	}

	// 为节点消息的处理创建一个处理上下文，外部在涉及此节点消息处理时均可以利用，以便及时退出。
	ctx, cancel := context.WithCancel(context.TODO())
	peer.handleContext = struct {
		ctx    context.Context
		cancel func()
	}{ctx: ctx, cancel: cancel}

	go func() {
		select {
		case <-p.WaitClose():
			// 等待底层关闭时，直接关闭
			peer.exit()
		case <-peer.term:
		}
	}()

	return peer
}

func (p *Peer) opt(flag PeerFlag) bool {
	return p.flag&flag == flag
}

// broadcast is a write loop that multiplexes block propagations, announcements
// and transaction broadcasts into the remote Peer. The goal is to have an async
// writer that does not lock up node internals.
func (p *Peer) broadcast() {
	go func() {
		for {
			select {
			case consensus, ok := <-p.queuedConsensus:
				if !ok {
					return
				}
				if err := p2p.Send(p.rw, uint64(protocol.BftMsg), consensus); err != nil {
					return
				}
			case <-p.term:
				return
			}
		}
	}()
	for {
		select {
		case txs, ok := <-p.queuedTxs:
			if !ok {
				return
			}
			if err := p.sendTransactions(txs); err != nil {
				return
			}
		case prop, ok := <-p.queuedProps:
			if !ok {
				return
			}
			if err := p.SendNewBlock(prop.block); err != nil {
				return
			}
			p.Log().Trace("Propagated block", "number", prop.block.Number(), "hash", prop.block.Hash())
		case msg, ok := <-p.queuedTTLMsgs:
			if !ok {
				return
			}
			if err := p.SendTTLMsg(msg); err != nil {
				return
			}
			p.Log().Trace("Broadcast TTLMsg", "code", msg.Code, "hash", msg.PayloadHash)
		case <-p.term:
			return
		}
	}
}

// 获取节点消息处理的Context，利用此Context可以实时退出对Peer消息的处理
func (p *Peer) HandleContext() context.Context {
	return p.handleContext.ctx
}

func (p *Peer) exit() {
	p.lock.Lock()
	defer p.lock.Unlock()
	select {
	case <-p.term:
		return
	default:
		close(p.term)
		p.handleContext.cancel()
	}

	//清理所有未释放内容
	if p.knownBlocks != nil {
		p.knownBlocks.Reset()
	}
	if p.knownTTLMsg != nil {
		p.knownTTLMsg.Reset()
	}
	if p.knownTxs != nil {
		p.knownTxs.Reset()
	}
	if p.queuedAnns != nil {
		for len(p.queuedAnns) > 0 {
			<-p.queuedAnns
		}
		close(p.queuedAnns)
	}
	if p.queuedConsensus != nil {
		for len(p.queuedConsensus) > 0 {
			<-p.queuedConsensus
		}
		close(p.queuedConsensus)
	}
	if p.queuedProps != nil {
		for len(p.queuedProps) > 0 {
			<-p.queuedProps
		}
		close(p.queuedProps)
	}
	if p.queuedTTLMsgs != nil {
		for len(p.queuedTTLMsgs) > 0 {
			<-p.queuedTTLMsgs
		}
		close(p.queuedTTLMsgs)
	}
	if p.queuedTxs != nil {
		for len(p.queuedTxs) > 0 {
			<-p.queuedTxs
		}
		close(p.queuedTxs)
	}

}

func (p *Peer) Close(reason p2p.DiscReason) {
	p.exit()
	p.Disconnect(reason)
}

// Info gathers and returns a collection of metadata known about a Peer.
func (p *Peer) Info() *PeerInfo {
	return &PeerInfo{
		Version: p.version,
		Head:    p.SysChainHead(),
		Status:  p.status,
	}
}

func (p *Peer) Get(key string) interface{} {
	switch key {
	case "KnownTxs":
		return p.knownTxs.Len()
	case "KnownBlocks":
		return p.knownBlocks.Len()
	}

	p.lock.RLock()
	defer p.lock.RUnlock()
	return p.status[key]
}
func (p *Peer) Set(key string, value interface{}) {
	p.lock.Lock()
	p.status[key] = value
	p.lock.Unlock()
}

func (p *Peer) ID() discover.NodeID {
	return p.id
}
func (p *Peer) Verion() int {
	return p.version
}
func (p *Peer) RW() p2p.MsgReadWriter {
	return p.rw
}
func (p *Peer) Closed() bool {
	select {
	case <-p.term:
		return true
	default:
		return false
	}
}
func (p *Peer) ReadMsg() (p2p.Msg, error) {
	return p.rw.ReadMsg()
}

// Head retrieves a copy of the current head hash and total difficulty of the
// Peer.
func (p *Peer) SysChainHead() types.UnitID {
	p.lock.RLock()
	defer p.lock.RUnlock()
	return p.sysChainHead
}

// SetHead updates the head hash and total difficulty of the Peer.
func (p *Peer) SetSysChainHead(header *types.Header) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.sysChainHead = header.ID()
}

// MarkBlock marks a block as known for the Peer, ensuring that the block will
// never be propagated to this particular Peer.
func (p *Peer) MarkBlock(hash common.Hash) {
	p.knownBlocks.Add(hash.String())
}

// HasUnit 是否该节点存在此单元
func (p *Peer) HasUnit(hash common.Hash) bool {
	return p.knownBlocks.Has(hash.String())
}

// MarkTransaction marks a transaction as known for the Peer, ensuring that it
// will never be propagated to this particular Peer.
func (p *Peer) MarkTransaction(hash common.Hash) {
	if p.opt(DisableTx) {
		if p.txPipes != nil {
			if p2 := p.txPipes.Peer(p.id); p2 != nil {
				p2.MarkTransaction(hash)
			}
		}
		return
	}
	p.knownTxs.Add(hash.String())
}

// SendTransactions sends transactions to the Peer and includes the hashes
// in its transaction hash set for future reference.
func (p *Peer) sendTransactions(txs types.Transactions) error {
	if len(txs) == 0 {
		return nil
	}
	if p.opt(DisableTx) {
		if p.txPipes != nil {
			if p2 := p.txPipes.Peer(p.id); p2 != nil {
				return p2.sendTransactions(txs)
			}
		}
		return errors.New("disable send tx")
	}
	txs = clearExpiredTx(txs, time.Now().Add(time.Second*6))
	if len(txs) == 0 {
		return errors.New("no tx need send")
	}
	return p2p.Send(p.rw, uint64(protocol.TxMsg), txs)
}

func clearExpiredTx(txs types.Transactions, now time.Time) types.Transactions {
	for i := 0; i < len(txs); i++ {
		if !txs[i].Expired(now) {
			continue
		}
		if len(txs) == 1 {
			return nil
		}
		if i < len(txs)-1 {
			txs[i] = txs[len(txs)-1]
			i--
		}
		txs = txs[:len(txs)-1]
	}
	return txs
}

// SendTransactions sends transactions to the Peer and includes the hashes
// in its transaction hash set for future reference.
func (p *Peer) SendTransactions(txs types.Transactions) error {
	for _, tx := range txs {
		p.MarkTransaction(tx.Hash())
	}
	return p.sendTransactions(txs)
}

// AsyncSendTransactions queues list of transactions propagation to a remote
// Peer. If the Peer's broadcast queue is full, the event is silently dropped.
func (p *Peer) AsyncSendTransactions(txs []*types.Transaction) {
	if p.queuedTxs == nil {
		return
	}
	select {
	case <-p.term:
	default:
		select {
		case p.queuedTxs <- txs:
			for _, tx := range txs {
				p.MarkTransaction(tx.Hash())
			}
		default:
			p.Log().Debug("Dropping transaction propagation", "count", len(txs))
		}
	}
}

// SendSyncMessage 发送同步消息
func (p *Peer) SendSyncMessage(msg interface{}) error {
	return p2p.Send(p.rw, uint64(protocol.RealTimeSyncMsg), msg)
}

// MarkTTLMsg 标记TTL消息
func (p *Peer) MarkTTLMsg(hash common.Hash) (exist bool) {
	if p.knownTTLMsg.Has(hash.String()) {
		return true
	}
	p.knownTTLMsg.Add(hash.String())
	return false
}

// SendTTLMsg 实时发送TTL消息
func (p *Peer) SendTTLMsg(msg protocol.TTLMsg) error {
	defer tracetime.New().SetMin(time.Second).Stop()
	p.MarkTTLMsg(msg.PayloadHash)
	return p.rw.WriteMsg(msg.Msg)
}

// AsyncSendTTLMsg 异步发送TTL消息，消息有可能被丢失
func (p *Peer) AsyncSendTTLMsg(msg protocol.TTLMsg) {
	select {
	case <-p.term:
	default:
		select {
		case p.queuedTTLMsgs <- msg:
			p.MarkTTLMsg(msg.PayloadHash)
		default:
			p.Log().Debug("Dropping TTLMsg announcement", "code", msg.Code, "Expiry", msg.Expiry, "hash", msg.PayloadHash)
		}
	}
}

// AsyncSendNewBlockHash queues the availability of a block for propagation to a
// remote Peer. If the Peer's broadcast queue is full, the event is silently
// dropped.
func (p *Peer) AsyncSendNewBlockHash(block *types.Unit) {
	select {
	case <-p.term:
	default:
		select {
		default:
			select {
			case p.queuedAnns <- block:
				p.MarkBlock(block.Hash())
			default:
				p.Log().Debug("Dropping block announcement", "number", block.Number(), "hash", block.Hash())
			}
		}
	}
}

// SendNewBlock propagates an entire block to a remote Peer.
func (p *Peer) SendNewBlock(block *types.Unit) error {
	defer tracetime.New().SetMin(time.Second).Stop()
	p.MarkBlock(block.Hash())
	return p2p.Send(p.rw, uint64(protocol.NewUnitMsg), block)
}

// AsyncSendNewBlock queues an entire block for propagation to a remote Peer. If
// the Peer's broadcast queue is full, the event is silently dropped.
func (p *Peer) AsyncSendNewBlock(block *types.Unit) {
	select {
	case <-p.term:
	default:
		select {
		default:
			select {
			case p.queuedProps <- &propEvent{block: block}:
				p.MarkBlock(block.Hash())
			default:
				p.Log().Debug("Dropping block propagation", "number", block.Number(), "hash", block.Hash())
			}
		}
	}
}

// 异步发送消息
func (p *Peer) AsyncSendMessage(code uint8, data interface{}) error {
	if code == protocol.BftMsg { //如果是共识消息,优先通过队列发送,否则
		select {
		case <-p.term:
			return errClosed
		default:
			select {
			case p.queuedConsensus <- data:
				p.Log().Trace("polled new consensus message")
			default:
				//记录丢失
				tracetime.Info("status", "discard",
					"more", "queue full",
					"Peer", p.ID().TerminalString(), "msg_code", code,
					"mid", utils.New64a(data.(bft.BFTMsg).Payload.([]byte)),
					"createat", data.(bft.BFTMsg).Create)

				p.Log().Debug("Dropping consensus propagation")
				return errors.New("queue is full,ignore this message")
			}
		}
		return nil
	}
	return p.SendAnyData(code, data)
}

func (p *Peer) SetWitnessType() {
	p.t = witnessPeerType
}

func (p *Peer) IsWitnessPeer() bool {
	return p.t == witnessPeerType
}

func (p *Peer) SendAnyData(code uint8, data interface{}) error {
	switch v := data.(type) {
	case p2p.Msg: //如果是转发原生数据，则直接发送
		b, err := ioutil.ReadAll(v.Payload)
		if err != nil {
			return err
		}
		v.Payload = bytes.NewReader(b)
		return p.rw.WriteMsg(v)
	default:
		switch code {
		case protocol.NewUnitMsg:
			return p.SendNewBlock(data.(*types.Unit))
		case protocol.TxMsg:
			return p.SendTransactions([]*types.Transaction{data.(*types.Transaction)})
		default:
			//TODO:需要判断该类消息是否在该Peer存在
			p.Log().Trace("send message", "code", protocol.CodeString(code))
			err := p2p.Send(p.rw, uint64(code), data)
			return err
		}
	}
}

//链接后的一次握手处理，将检查双方状态是否一致。
//包括检查创世哈希、协议版本等。
func (p *Peer) Handshake(network uint64, headID types.UnitID, genesis common.Hash, subPipeNode *discover.Node) error {
	status, err := Handshake(p.rw, uint32(p.version), network, headID, genesis, subPipeNode)
	if err != nil {
		return err
	}
	p.sysChainHead = status.CurrSysChainUnit
	p.status[CfgSubPipeNode] = status.SubPipeNode
	return nil
}

// String implements fmt.Stringer.
func (p *Peer) String() string {
	return fmt.Sprintf("Peer %s [%s]", p.id,
		fmt.Sprintf("nts/%2d", p.version),
	)
}

//链接后的一次握手处理，将检查双方状态是否一致。
//包括检查创世哈希、协议版本等。
func Handshake(rw p2p.MsgReadWriter, version uint32, network uint64,
	headID types.UnitID, genesis common.Hash, subPipeNode *discover.Node) (StatusData, error) {
	// Send out own handshake in a new thread
	errc := make(chan error, 2)
	var status StatusData // safe to read after two values have been received from errc

	go func() {
		var snode string
		if subPipeNode != nil {
			snode = subPipeNode.String()
		}
		errc <- p2p.Send(rw, protocol.StatusMsg, &StatusData{
			ProtocolVersion:  version,
			NetworkId:        network,
			CurrSysChainUnit: headID,
			GenesisBlock:     genesis,
			SubPipeNode:      snode,
		})
	}()
	go func() {
		errc <- readStatus(rw, version, network, &status, genesis)
	}()
	timeout := time.NewTimer(handshakeTimeout)
	defer timeout.Stop()
	for i := 0; i < 2; i++ {
		select {
		case err := <-errc:
			if err != nil {
				return status, err
			}
		case <-timeout.C:
			return status, p2p.DiscReadTimeout
		}
	}
	return status, nil
}

func readStatus(rw p2p.MsgReadWriter, version uint32, network uint64, status *StatusData, genesis common.Hash) (err error) {
	msg, err := rw.ReadMsg()
	if err != nil {
		return err
	}

	if msg.Code != protocol.StatusMsg {
		return protocol.Error(protocol.ErrNoStatusMsg, "first msg has code %x (!= %x)", msg.Code, protocol.StatusMsg)
	}
	if msg.Size > protocol.MaxMsgSize {
		return protocol.Error(protocol.ErrMsgTooLarge, "%v > %v", msg.Size, protocol.MaxMsgSize)
	}
	// Decode the handshake and make sure everything matches
	if err := msg.Decode(&status); err != nil {
		return protocol.Error(protocol.ErrDecode, "msg %v: %v", msg, err)
	}
	if status.GenesisBlock != genesis {
		return protocol.Error(protocol.ErrGenesisBlockMismatch, "%x (!= %x)", status.GenesisBlock[:8], genesis[:8])
	}
	if status.NetworkId != network {
		return protocol.Error(protocol.ErrNetworkIdMismatch, "%d (!= %d)", status.NetworkId, network)
	}
	if uint32(status.ProtocolVersion) != version {
		return protocol.Error(protocol.ErrProtocolVersionMismatch, "%d (!= %d)", status.ProtocolVersion, version)
	}
	return nil
}
