// 对见证人的网络连接进行管理和控制
package wconn

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/nts/protocol"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/p2p"
	"gitee.com/nerthus/nerthus/p2p/discover"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/rlp"

	"github.com/pkg/errors"
)

const (
	defStatusMsgHistoryCacheSize = 200 //记录已处理的见证节点信息消息缓存大小
	defWitnessConnInfoCacheSize  = 200 //存储记录见证人链接信息
)

var defMinWitnessCount = int(params.ConfigParamsUCMinVotings)

type Miner interface {
	WitnessGroup() uint64
	Address() common.Address //见证人地址
	ENode() discover.Node
	Sign(info types.SignHelper) error
	Encrypt(pubkey []byte, data []byte) ([]byte, error)
	Decrypt(data []byte) ([]byte, error)
}

type ChainReader interface {
	Config() *params.ChainConfig
	GetWitnessAtGroup(gp uint64) (common.AddressList, error)
	GetWitnessInfo(address common.Address) (*sc.WitnessInfo, error)
}

// 见证组内网络互连控制
// 提供两个基本功能：
//  1. 需要向邻近节点传递自己作为见证节点的加密连接信息，并接收和存储其他见证人所提供的加密连接信息。
//  2. 见证组内连接管理，启动见证服务后，需要尝试同同组见证人互连。
// TODO: 当用户误操作在两个节点上启动了程序时，可以自动切换到最新并移除旧连接
type WitnessGroupConnControl struct {
	db                 ntsdb.Database
	miner              Miner
	connectedWitness   *WitnessSet      //已握手成功的见证人
	connectedPeers     protocol.PeerSet //已连接的网络节点
	witnessGroup       *WitnessMap      //见证组信息
	newConnInfoCh      chan *WitnessNode
	encryptedMyConnMsg *WitnessConnectMsg //已经完成加密的关于我的连接信息
	locker             sync.RWMutex
	statusMsgHistory   *lru.Cache
	wconnCache         *lru.Cache

	dagchain ChainReader
	signer   types.Signer
	logger   log.Logger

	quit chan struct{}
}

func New(db ntsdb.Database, peerset protocol.PeerSet, dag ChainReader) *WitnessGroupConnControl {

	cache, _ := lru.New(defStatusMsgHistoryCacheSize)
	cache2, _ := lru.New(defWitnessConnInfoCacheSize)

	info := WitnessGroupConnControl{
		db:               db,
		connectedPeers:   peerset,
		dagchain:         dag,
		signer:           types.MakeSigner(*dag.Config()),
		statusMsgHistory: cache,
		wconnCache:       cache2,
		newConnInfoCh:    make(chan *WitnessNode, 12),
		quit:             make(chan struct{}),
	}
	info.logger = log.New("module", "wconn")
	info.connectedWitness = NewWitnessSet(info.logger)
	info.witnessGroup = NewWitnessMap(info.logger)

	return &info
}

// 开启同组见证人管理
func (wc *WitnessGroupConnControl) Start(miner Miner) error {
	wc.miner = miner

	if wc.miner.WitnessGroup() == 0 {
		defMinWitnessCount = int(params.ConfigParamsSCMinVotings)
	}
	if wc.miner.Address().Empty() {
		return errors.New("wconn: can not start when miner is empty")
	}
	if err := wc.init(); err != nil {
		return errors.Wrap(err, "init failed")
	}
	//1. 监听网络连接
	go wc.listenConn()
	return nil
}

// 停止同组见证人管理
func (wc *WitnessGroupConnControl) Stop() {
	close(wc.quit)

	//停止服务器后，不再属于白名单节点，但不需要断开连接
	for _, w := range wc.ConnectedWitness().Peers() {
		wc.connectedPeers.ClearWhitelist(w.ID())
	}
}

// reload 重新见证见证人信息
// 当存在新见证人加入或者退出组时，可以重新加载来更新见证人信息
func (wc *WitnessGroupConnControl) Reload() error {
	if !wc.isMinerRunning() {
		return nil
	}
	wc.logger.Debug("reload witness group info")
	before := wc.witnessGroup.All()
	if err := wc.init(); err != nil {
		return err
	}
	for _, w := range before {
		if w.NodeInfo == nil {
			continue
		}
		//说明此节点已经不属于此见证组，可以移除
		if wc.witnessGroup.Load(w.Witness) == nil {
			pid := w.NodeInfo.Enode.ID
			wc.connectedPeers.ClearWhitelist(pid)
			if p := wc.connectedPeers.Peer(pid); p != nil {
				p.Close(p2p.DiscQuitting)
			}
		}
	}

	return nil
}

// 已连接的见证人
func (ws *WitnessGroupConnControl) ConnectedWitness() *WitnessSet {
	return ws.connectedWitness
}

// 连接状态
func (wc *WitnessGroupConnControl) Status() *WitnessMap {
	return wc.witnessGroup
}

// 初始化自己
// 加载已存储的连接信息
func (wc *WitnessGroupConnControl) init() (err error) {

	//更新自己的节点信息
	err = wc.updateMyselfConnectInfo()
	if err != nil {
		return err
	}
	wc.logger.Debug("witness connect info", "wnode", wc.witnessGroup.MeInfo)
	if err := wc.loadWitnessInfo(); err != nil {
		return err
	}

	//删除无效数据
	nodes, err := GetAllWitnessNodeInfo(wc.db)
	if err != nil {
		return err
	}
	//更新
	for _, node := range nodes {
		if node.Witness == wc.miner.Address() {
			continue
		}
		info := wc.witnessGroup.Load(node.Witness)
		if info == nil {
			DelWitnessNodeInfo(wc.db, node.Witness)
		}
	}
	return nil
}

func (wc *WitnessGroupConnControl) updateMyselfConnectInfo() error {
	currNode := wc.miner.ENode()
	me := GetWintessNodeInfo(wc.db, wc.miner.Address())
	//属于新节点数据，需要更新存储
	if me == nil || me.Enode.String() != currNode.String() {
		me = &WitnessNode{
			Enode:   currNode,
			Witness: wc.miner.Address(),
		}
	}
	me.UpdateTime = time.Now()
	me.Version = uint64(me.UpdateTime.Unix())
	//重新签名一次
	if err := wc.miner.Sign(me); err != nil {
		return err
	}
	wc.locker.Lock()
	wc.encryptedMyConnMsg = nil
	wc.locker.Unlock()
	//一旦变化，则清理本地存储
	DelWitnessConnectInfo(wc.db, me.Witness)

	WriteWintessNodeInfo(wc.db, *me)
	wc.witnessGroup.MeInfo = me
	//清理缓存
	return nil
}

// 监听 Peer 连接，
// 根据 Peer 信息管理相对于的见证人
func (wc *WitnessGroupConnControl) listenConn() {
	ch := make(chan protocol.PeerEvent, 100)
	sub := wc.connectedPeers.SubscribePeerEvent(ch)
	defer sub.Unsubscribe()

	onEvent := func(p protocol.PeerEvent) {
		if p.Connected { //如果是新节点连接，则发送我的节点信息
			if msg, err := wc.GetEncryptConnectInfo(); err != nil {
				if err := wc.sendMsg(p.Peer, msg); err != nil {
					wc.logger.Debug("failed send connect info", "err", err)
				}
			}
		}

		//过滤无关节点，如果是本组见证人节点则进行进一步处理
		witness := wc.witnessGroup.LoadById(p.PeerID)
		if witness == nil {
			return
		}
		//链接
		if p.Connected {
			wc.logger.Debug("peer: new witness connected", "witness", witness.Witness, "peer", p.PeerID)
			//握手
			if err := wc.shake(witness, p.Peer); err != nil {
				wc.logger.Trace("failed to send shake", "node", p.PeerID, "err", err)
			}
		} else { //断开
			wc.logger.Debug("peer: witness disconnected", "witness", witness.Witness, "peer", p.PeerID)
			witness.SetStatus(wc.logger, Disconnected)
			wc.connectedWitness.RemoveWitness(witness.Witness)
		}
	}

	//对于已经连接的节点处理
	for _, p := range wc.connectedPeers.Peers() {
		onEvent(protocol.PeerEvent{
			Connected: true,
			PeerID:    p.ID(),
			Peer:      p,
		})
	}

	var (
		sendTimes    uint
		lastSendTime time.Time
		minInter          = time.Second * 20
		maxSendTimes uint = 15 // 约 16 分钟
	)
	//检查本组见证人的连接数量
	checkConnects := func() {
		if len(wc.connectedPeers.Peers()) == 0 {
			return
		}
		if wc.connectedWitness.Len()+1 >= defMinWitnessCount {
			return
		}
		//说明本组见证人人数已经不足，广播已无意义
		if wc.witnessGroup.Len()+1 < defMinWitnessCount {
			return
		}

		now := time.Now()
		// 2**sendTimes 时间间隔
		if !lastSendTime.IsZero() && sendTimes > 0 && now.Sub(lastSendTime) < time.Duration(1<<sendTimes)*minInter {
			return
		}
		//广播前删除自己的版本信息
		if err := wc.updateMyselfConnectInfo(); err != nil {
			wc.logger.Error("failed update info", "err", err)
			return
		}

		//广播
		if success, err := wc.broacastNodeInfo(); err != nil {
			wc.logger.Warn("failed broadcast my node connect info", "err", err)
		} else if success > 0 {
			//成功时更新
			lastSendTime = now
			sendTimes++
			if sendTimes > maxSendTimes {
				sendTimes = 0
			}
			wc.logger.Debug("broadcast my encrypted connect info",
				"sends", success, "connected.peers", len(wc.connectedPeers.Peers()),
				"connected.witness", wc.connectedWitness.Len(), "nextTime", time.Duration(1<<sendTimes)*minInter)
		}
	}

	tk := time.NewTicker(time.Second * 30) //每三十秒检查一次
	defer tk.Stop()

	//监听新连接接入
	for {
		select {
		case <-sub.Err():
			return
		case <-wc.quit:
			return
		case <-tk.C:

			if wc.witnessGroup.Len()+1 < defMinWitnessCount {
				wc.logger.Warn("witness on group is too less", "count", wc.witnessGroup.Len()+1, "need", defMinWitnessCount)
				continue
			}
			//说明有部分节点尚未连接，需要从已连接的 Peer 中检查是否有见证节点
			if wc.witnessGroup.Len() == wc.ConnectedWitness().Len() {
				continue
			}

			//如果没多少链接则需要广播
			wc.logger.Trace("check witness connect status",
				"connected.peers", len(wc.connectedPeers.Peers()),
				"connected.witness", wc.connectedWitness.Len(), "witness.sum", wc.witnessGroup.Len()+1)

			for _, w := range wc.witnessGroup.All() {
				wc.logger.Trace("check witness connect status", "witness", w.Witness, "status", w.Status, "wnode", w.NodeInfo)
				if w.NodeInfo != nil {
					switch w.Status {
					case Disconnected:
						wc.tryConnect(w.NodeInfo, nil)
					case NetConnected:
						//如果已经连接，则只需要变更状态
						wc.joinWitnessNode(w.NodeInfo)
					}
				}

				if w.NodeInfo == nil {
					var (
						node *WitnessNode
						err  error
					)
					if info := GetWitnessConnectInfo(wc.db, w.Witness); info != nil {
						if node, err = wc.processConnectInfo(w.Witness, info); err != nil {
							wc.logger.Trace("failed procee connectInfo", "err", err)
						}
					}
					if node != nil {
						wc.logger.Trace("found witness connect node info", "witness", w.Witness, "wnode", node)
					} else {
						sends := wc.FetchWitnessConnectInfo(w.Witness)
						wc.logger.Trace("fetch witness connect info", "witness", w.Witness, "sends", sends)
					}
				}
			}
			checkConnects()

		case p := <-ch:
			onEvent(p)
		case nodeInfo := <-wc.newConnInfoCh:
			if err := wc.processWitnessNodeInfo(nodeInfo, false); err != nil {
				wc.logger.Trace("failed update witness node connect info", "witness", nodeInfo.Witness, "err", err)
			}
		}
	}
}

// 加载同组见证人信息
func (wc *WitnessGroupConnControl) loadWitnessInfo() error {
	gp := wc.miner.WitnessGroup()

	list, err := wc.dagchain.GetWitnessAtGroup(gp)
	if err != nil {
		return err
	}

	wc.witnessGroup.Clearn()

	for i, w := range list {
		//忽略候选
		if gp == sc.DefSysGroupIndex && i+1 > params.ConfigParamsSCWitness {
			continue
		}
		//忽略自己
		if w == wc.miner.Address() {
			continue
		}
		witness, err := wc.dagchain.GetWitnessInfo(w)
		if err != nil {
			return err
		}

		witnessInfo := NewWintessInfo(witness.Address, witness.Pubkey)

		wc.witnessGroup.Store(witnessInfo)
		if node := GetWintessNodeInfo(wc.db, w); node != nil {
			witnessInfo.NodeInfo = node
		}
		wc.logger.Trace("load witness on same group", "group", gp, "wnode", witnessInfo.NodeInfo)
		if witnessInfo.NodeInfo == nil {
			//检查本地库中是否存在此消息
			info := GetWitnessConnectInfo(wc.db, w)
			wc.logger.Trace("load witness on same group", "group", gp, "witness", w, "node", "nil", "existConnectInfo", info != nil)
			if info != nil {
				wc.processConnectInfo(w, info)
			}
		} else {
			wc.tryConnect(witnessInfo.NodeInfo, nil)
		}
	}

	return nil
}

type P2PMsg interface {
	Decode(interface{}) error
}

// 处理网络外部消息
func (wc *WitnessGroupConnControl) Handle(pid discover.NodeID, data interface{}) error {
	code, err := wc.handle(pid, data)
	wc.logger.Trace("handle wconn message", "peer", pid, "msg.code", code, "err", err)
	return err
}

func (wc *WitnessGroupConnControl) handle(pid discover.NodeID, data interface{}) (SubCode, error) {
	var msg Message

	if d, ok := data.(P2PMsg); ok {
		if err := d.Decode(&msg); err != nil {
			return 0, err
		}
	} else if b, ok := data.([]byte); ok {
		if err := rlp.DecodeBytes(b, &msg); err != nil {
			return 0, err
		}
	} else {
		return 0, errors.New("invalid data type")
	}

	switch msg.Code {
	default:
		return msg.Code, fmt.Errorf("invalid code %d", msg.Code)
	case CodeShake:
		//如果尚未开启挖矿，则不接收此消息
		if !wc.isMinerRunning() {
			return msg.Code, nil
		}

		m := new(ShakeMsg)
		if err := rlp.DecodeBytes(msg.Data, &m); err != nil {
			return msg.Code, err
		}
		return msg.Code, wc.handleShakeMsg(pid, m)
	case CodeStatus:
		m := new(WitnessConnectMsg)
		if err := rlp.DecodeBytes(msg.Data, &m); err != nil {
			return msg.Code, err
		}
		return msg.Code, wc.handleStatusMsg(pid, m)
	case CodeFetchWitnessConn:
		m := new(FetchWitnessConnInfoMsg)
		if err := rlp.DecodeBytes(msg.Data, &m); err != nil {
			return msg.Code, err
		}
		return msg.Code, wc.HandleFetchWitnessConnInfo(pid, *m)
	}
}

func (wc *WitnessGroupConnControl) sendMsg(peer protocol.Peer, msg interface{}) error {
	var code SubCode
	switch msg.(type) {
	default:
		panic(fmt.Errorf("invalid msg type %T", msg))
	case ShakeMsg, *ShakeMsg:
		code = CodeShake
	case WitnessConnectMsg, *WitnessConnectMsg:
		code = CodeStatus
	case FetchWitnessConnInfoMsg, *FetchWitnessConnInfoMsg:
		code = CodeFetchWitnessConn
	}
	b, err := rlp.EncodeToBytes(msg)
	if err != nil {
		panic(err)
	}
	return peer.AsyncSendMessage(MainMessageCode, Message{Code: code, Data: b})
}

// 开始和本组见证节点通信前，需要进行握手，校验和互换信息。
// 首先两者需要交互下各自的见证组，进行一次简要对话
func (wc *WitnessGroupConnControl) shake(witness *WitnessInfo, peer protocol.Peer) error {
	witness.SetStatus(wc.logger, NetConnected)

	all := wc.witnessGroup.All()
	msg := ShakeMsg{
		Mine:         wc.witnessGroup.MeInfo,
		WitnessGroup: wc.miner.WitnessGroup(),
		KownNodes:    make([]*EncryptData, 0, len(all)),
		Connected:    uint64(wc.connectedWitness.Len()),
	}

	//从本地已知的见证人连接信息中获取可以发送给对方的其他见证人连接信息
	for _, w := range all {
		if w.Witness == witness.Witness {
			continue
		}
		if info := GetWitnessConnectInfo(wc.db, w.Witness); info != nil && w.NodeInfo != nil {
			if info.Version >= w.NodeInfo.Version {
				for _, d := range info.Data {
					if d.Receiver == witness.Witness {
						msg.KownNodes = append(msg.KownNodes, d)
					}
				}
			} else {
				//删除版本过时的消息
				wc.DelWitnessConnMsg(w.Witness)
			}
			break
		}
	}
	err := wc.sendMsg(peer, msg)
	if err != nil {
		return errors.Wrap(err, "failed send shake message to new peer")
	}
	return nil
}

// 处理来自同组见证人的连接握手信息
func (wc *WitnessGroupConnControl) handleShakeMsg(pid discover.NodeID, msg *ShakeMsg) error {
	if !wc.isMinerRunning() {
		return nil
	}

	//忽略来自本人自己所发消息
	if msg.Mine == nil || msg.Mine.Witness == wc.miner.Address() {
		return errors.New("invalid data")
	}

	if msg.Mine.Enode.ID != pid {
		return errors.New("invalid node id")
	}

	wc.logger.Trace("handle shake message", "from", msg.Mine.Witness,
		"he.connected", msg.Connected, "witnessGroup", msg.WitnessGroup, "knownNodes", len(msg.KownNodes))

	if err := wc.processWitnessNodeInfo(msg.Mine, true); err != nil {
		return err
	}
	//处理接收信息
	if len(msg.KownNodes) > params.ConfigParamsSCWitness {
		return errors.New("invalid message")
	}

	for _, anther := range msg.KownNodes {
		if anther == nil || anther.Receiver != wc.miner.Address() {
			return errors.New("invalid message")
		}
		if node, err := wc.decryptConnectInfo(anther.Data); err != nil {
			return errors.Wrap(err, "failed to decrypt")
		} else {
			wc.logger.Trace("decrypt connect info", "enode", node)
			wc.newConnInfoCh <- node
		}
	}

	return nil
}

func (wc *WitnessGroupConnControl) joinWitnessNode(node *WitnessNode) {
	if node == nil {
		return
	}
	//如果已经连接则加入并记录
	p := wc.connectedPeers.Peer(node.Enode.ID)
	if p == nil {
		return
	}
	w := wc.witnessGroup.Load(node.Witness)
	if w == nil {
		return
	}
	w.SetStatus(wc.logger, Joined)
	wc.connectedWitness.AddWitness(node.Witness, p)
	wc.logger.Debug("witness joined", "witness", node.Witness, "connected.witness", wc.ConnectedWitness().Len())
}

// 比较两个连接信息是否一致
func compareNodeInfo(a, b *WitnessNode) (int, error) {
	if a == nil && b == nil {
		return 0, nil
	}
	if a == nil {
		return 1, nil
	}
	if b == nil {
		return -1, nil
	}
	//说明是相同版本的连接信息，此时检查是否Node 信息正常
	if a.Version == b.Version {
		if a.Hash() != b.Hash() {
			return 0, errors.Errorf("invalid node info")
		}
		//一切正常时，不需要更新
		return 0, nil
	}

	if a.Version > b.Version {
		return -1, nil
	}
	return 1, nil
}

// 接收到一个节点信息处理
// 当接收到的节点信息是来自本组见证人时，需要根据情况更新见证人连接信息
func (wc *WitnessGroupConnControl) processWitnessNodeInfo(node *WitnessNode, ignoreOld bool) error {
	if node == nil {
		return errors.New("invalid witness node")
	}
	if node.Witness.Empty() || node.Witness.IsContract() {
		return errors.New("invalid witness address")
	}
	if node.Enode.Incomplete() {
		return errors.New("invalid witness node ip")
	}
	if node.Enode.ID.IsEmpty() {
		return errors.New("invalid witness node id")
	}

	//忽略
	if node.Witness == wc.miner.Address() {
		return nil
	}

	witness := wc.witnessGroup.Load(node.Witness)
	if witness == nil {
		return errors.Errorf("missing witness info %s", node.Witness)
	}

	update := func(old *WitnessNode) {
		if old != nil {
			//删除旧数据
			wc.DelWitnessConnMsg(witness.Witness)
		}

		wc.logger.Debug("update witness node info", "wnode", node, "old", old)
		WriteWintessNodeInfo(wc.db, *node)
		wc.tryConnect(node, old)
	}

	if old := witness.NodeInfo; old != nil {
		r, err := compareNodeInfo(old, node)
		if err != nil {
			return err
		}
		switch r {
		case 0: //相同
			wc.tryConnect(node, nil)
		case 1: //新数据
			update(old)
		case -1: //成旧
			if ignoreOld {
				update(old)
			}
		}
	} else {
		update(nil)
	}
	return nil
}

// 尝试连接到见证人的新的节点上
func (wc *WitnessGroupConnControl) tryConnect(node, old *WitnessNode) {
	//否组，接收到新版本的连接信息需要干预处理
	// 1. 删除原连接
	if old != nil {
		//虽然版本有更新，但是链接信息无变化
		if old.Enode.String() != node.Enode.String() {
			wc.connectedWitness.RemoveWitness(node.Witness)
			// 如果 ID 变化，则移除原 ID
			if old.Enode.ID != node.Enode.ID {
				wc.connectedPeers.ClearWhitelist(old.Enode.ID)
				wc.witnessGroup.DelNode(old.Enode.ID)
			}
			if p := wc.connectedPeers.Peer(old.Enode.ID); p != nil {
				wc.logger.Debug("disconnect when witness have a new node",
					"wnode.new", node, "wnode.old", old)
				wc.connectedPeers.ClearWhitelist(old.Enode.ID)
				p.Close(p2p.DiscUselessPeer)
			}
		}
	}
	if !wc.isMinerRunning() {
		return
	}

	info := wc.witnessGroup.Load(node.Witness)
	if info == nil {
		wc.logger.Debug("missing witness info", "witness", node.Witness)
		return
	}

	info.NodeInfo = node
	node.UpdateTime = time.Now()
	wc.witnessGroup.StoreNodeKey(info)             //存储信息
	wc.connectedPeers.JoinWhitelist(node.Enode.ID) //加入网络白名单
	//说明已经连接
	if p := wc.connectedPeers.Peer(node.Enode.ID); p != nil { //如果已经连接，则直接握手并通知
		wc.joinWitnessNode(node)
	} else {
		info.SetStatus(wc.logger, TryConnecting)
		wc.connectedPeers.AddPeer(&node.Enode)
	}
}

func (wc *WitnessGroupConnControl) GetEncryptConnectInfo() (msg WitnessConnectMsg, err error) {
	wc.locker.Lock()
	defer wc.locker.Unlock()
	if wc.encryptedMyConnMsg != nil {
		return *wc.encryptedMyConnMsg, nil
	}

	if wc.witnessGroup.MeInfo == nil {
		return msg, errors.New("node info is nil")
	}

	b, err := rlp.EncodeToBytes(wc.witnessGroup.MeInfo)
	if err != nil {
		return msg, err
	}

	msg = WitnessConnectMsg{
		WitnessGroup: wc.miner.WitnessGroup(),
		Data:         make([]*EncryptData, 0, 10),
		Version:      wc.witnessGroup.MeInfo.Version,
	}
	for _, w := range wc.witnessGroup.All() {
		encrypted, err := wc.miner.Encrypt(w.PubKey, b)
		if err != nil {
			return msg, errors.Wrap(err, "failed encrypt")
		}

		log.Trace("encrypt data", "for", w.Witness)
		msg.Data = append(msg.Data, &EncryptData{
			Receiver: w.Witness,
			Data:     encrypted,
		})
	}
	//再签名
	if err := wc.miner.Sign(&msg); err != nil {
		return msg, err
	}

	wc.encryptedMyConnMsg = &msg

	//存储在本地
	WriteWitnessConnectInfo(wc.db, wc.miner.Address(), &msg)

	return msg, nil
}

// 将我的连接信息加密后广播给所有连接节点
func (wc *WitnessGroupConnControl) broacastNodeInfo() (int, error) {
	if len(wc.connectedPeers.Peers()) == 0 {
		return 0, errors.New("no connected peers")
	}
	msg, err := wc.GetEncryptConnectInfo()
	if err != nil {
		return 0, err
	}

	var sends int
	for _, p := range wc.connectedPeers.Peers() {
		if err := wc.sendMsg(p, msg); err == nil {
			sends++
		}
	}
	return sends, nil
}

func (wc *WitnessGroupConnControl) handleStatusMsg(pid discover.NodeID, msg *WitnessConnectMsg) error {
	//消息哈希
	hash := common.SafeHash256(msg)
	if find, _ := wc.statusMsgHistory.ContainsOrAdd(hash, struct{}{}); find {
		wc.logger.Trace("exist msg", "mid", hash)
		return nil
	}
	if len(msg.Data) == 0 || len(msg.Data) > params.ConfigParamsUCWitness {
		return errors.New("invalid data: empty data or too big")
	}

	sender, err := types.Sender(wc.signer, msg)
	if err != nil {
		return err
	}
	w, err := wc.dagchain.GetWitnessInfo(sender)
	if err != nil {
		//有两种情况：1. 确实不是见证人 2. 本地还没有同步到此数据。 3. 恶意伪造的数据。
		//此时不向外部报告错误信息，避免断开网络连接
		wc.logger.Trace("is not witness", "witness", sender)
		return nil
	}
	if w.Status != sc.WitnessNormal {
		return errors.New("this witness status is not normal")
	}
	if w.GroupIndex != msg.WitnessGroup {
		return errors.New("invalid data:group index mismatch")
	}
	//也许消息是旧版本，则需要忽略
	curr := GetWitnessConnectInfo(wc.db, sender)
	if curr != nil && curr.Version >= msg.Version {
		//忽略旧版本消息
		return nil
	}

	if _, err := wc.processConnectInfo(sender, msg); err != nil {
		return errors.Wrap(err, "failed process connect info")
	}
	wc.StoreWitnessConnMsg(sender, msg)

	//再广播给其他节点
	var sends int
	for _, p := range wc.ConnectedWitness().Peers() {
		if p.ID() == pid {
			continue
		}
		if w := wc.witnessGroup.LoadById(p.ID()); w != nil && w.Witness == sender {
			continue
		}
		if wc.sendMsg(p, msg) == nil {
			sends++
		}
	}
	return nil
}

// 拉取指定见证人的连接信息
// 将随机向邻居警节点拉取
func (wc *WitnessGroupConnControl) FetchWitnessConnectInfo(witness common.Address) int {
	peers := wc.connectedPeers.Peers()
	if len(peers) == 0 {
		return 0
	}
	msg := FetchWitnessConnInfoMsg{Witness: witness}
	count := rand.Intn(len(peers) + 1)
	if count == 0 {
		count = 1
	}
	var success int
	for i := 0; i < count; i++ {
		if err := wc.sendMsg(peers[i], msg); err != nil {
			wc.logger.Trace("failed send msg", "err", err)
		} else {
			success++
		}
	}
	return success
}
func (wc *WitnessGroupConnControl) HandleFetchWitnessConnInfo(pid discover.NodeID, msg FetchWitnessConnInfoMsg) error {
	if msg.Witness.Empty() {
		return errors.New("invalid data: witness is empty")
	}
	m := wc.GetWitnessConnMsg(msg.Witness)
	if m == nil {
		return nil
	}
	p := wc.connectedPeers.Peer(pid)
	if p != nil {
		wc.sendMsg(p, m)
	}
	return nil
}

func (wc *WitnessGroupConnControl) StoreWitnessConnMsg(witness common.Address, msg *WitnessConnectMsg) {
	wc.logger.Trace("store witness node info", "witness", witness)
	wc.wconnCache.Add(witness, msg)
	WriteWitnessConnectInfo(wc.db, witness, msg)
}
func (wc *WitnessGroupConnControl) DelWitnessConnMsg(witness common.Address) {
	wc.logger.Trace("delete witness node info", "witness", witness)
	wc.wconnCache.Remove(witness)
	DelWitnessConnectInfo(wc.db, witness)
}

// 获取指定见证人发送给外部的加密连接信息
func (wc *WitnessGroupConnControl) GetWitnessConnMsg(witness common.Address) *WitnessConnectMsg {
	msg, ok := wc.wconnCache.Get(witness)
	if ok {
		return msg.(*WitnessConnectMsg)
	}
	m := GetWitnessConnectInfo(wc.db, witness)
	if m != nil {
		wc.wconnCache.Add(witness, m)
	}
	return m
}

func (wc *WitnessGroupConnControl) isMinerRunning() bool {
	return wc.miner != nil && !wc.miner.Address().Empty()
}

// 处理见证人的网络连接信息
// 如果自己也是见证人，且已经在工作中，则需要检查是否需要连接到此见证人
func (wc *WitnessGroupConnControl) processConnectInfo(from common.Address, msg *WitnessConnectMsg) (*WitnessNode, error) {
	if !wc.isMinerRunning() {
		return nil, nil
	}
	//忽略其他组
	if msg.WitnessGroup != wc.miner.WitnessGroup() {
		return nil, nil
	}

	// 寻找给自己加密的信息
	for _, info := range msg.Data {
		if info.Receiver != wc.miner.Address() {
			continue
		}
		if node, err := wc.decryptConnectInfo(info.Data); err != nil {
			wc.logger.Trace("failed decrypt connect info", "from", from, "err", err)
			wc.DelWitnessConnMsg(from)
			return nil, err
		} else {
			wc.logger.Debug("decrypt one witness node connection info", "enode", node)
			if wc.isMinerRunning() {
				wc.newConnInfoCh <- node
			}
			return node, nil
		}
	}
	wc.logger.Trace("not found then delete it", "witness", from)
	wc.DelWitnessConnMsg(from)
	return nil, nil
}

// 解密给自己加密的见证人连接信息
func (wc *WitnessGroupConnControl) decryptConnectInfo(data []byte) (*WitnessNode, error) {
	result, err := wc.miner.Decrypt(data)
	if err != nil {
		return nil, err
	}
	nodeInfo := new(WitnessNode)
	if err := rlp.DecodeBytes(result, &nodeInfo); err != nil {
		return nil, err
	}

	singer, err := types.Sender(wc.signer, nodeInfo)
	if err != nil {
		return nil, errors.Wrap(err, "invalid node sign")
	}
	if singer != nodeInfo.Witness {
		return nil, errors.New("signer is not witness")
	}
	return nodeInfo, nil
}

func (wc *WitnessGroupConnControl) broadcastMsg(msg interface{}, skipPeer discover.NodeID) (sends int) {
	for _, p := range wc.connectedPeers.Peers() {
		if !skipPeer.IsEmpty() && p.ID() == skipPeer {
			continue
		}
		if err := wc.sendMsg(p, msg); err == nil {
			sends++
		} else {
			log.Error("err", "err", err)
		}
	}
	return
}
