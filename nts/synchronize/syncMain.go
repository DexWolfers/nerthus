package synchronize

import (
	"fmt"
	"sync"
	"time"

	"gitee.com/nerthus/nerthus/p2p/discover"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/mclock"
	"gitee.com/nerthus/nerthus/consensus"
	"gitee.com/nerthus/nerthus/core/config"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/nts/protocol"
	mp "gitee.com/nerthus/nerthus/nts/protocol"
	"gitee.com/nerthus/nerthus/nts/synchronize/bysolt"
	"gitee.com/nerthus/nerthus/nts/synchronize/bytime"
	"gitee.com/nerthus/nerthus/nts/synchronize/message"
	"gitee.com/nerthus/nerthus/nts/synchronize/status"
	"gitee.com/nerthus/nerthus/nts/synchronize/ucache"
	"gitee.com/nerthus/nerthus/nts/synchronize/util"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/rlp"

	"github.com/pkg/errors"
)

const (
	defFetchMissSyncCanWorkTime = time.Hour
)

type HandleMsg = message.HandleMsg

type SyncMain struct {
	db      ntsdb.Database
	peers   sync.Map // key : discover.NodeID, value=*SyncPeer
	peerSet mp.PeerSet
	api     APIBackend
	cr      ChainWriteRead

	peerStatus *status.PeerStatusSet
	msgHandles map[message.Code]HandleMsg
	logger     log.Logger
	unitCache  *ucache.UnitCache

	//Service
	syncByTime *bytime.Sync
	// 实时同步
	realtimeServer *bysolt.RealTimeSync

	exitw sync.WaitGroup
	quit  chan struct{}
}

// 实例化服务，
// 将采用深度优先模式同步数据，并实时记录已连接节点的状态
func New(api APIBackend, peerSet mp.PeerSet, db ntsdb.Database, rd ntsdb.RD, cwr ChainWriteRead) (*SyncMain, error) {

	fs := SyncMain{
		cr:         cwr,
		db:         db,
		api:        api,
		logger:     log.New("module", "sync"),
		msgHandles: make(map[message.Code]HandleMsg),
		quit:       make(chan struct{}),
		peerSet:    peerSet,
		peerStatus: status.NewPeerStatus(cwr),
	}

	//cache
	cache, err := ucache.NewCache(&fs, db, rd, cwr)
	if err != nil {
		return nil, err
	}
	fs.unitCache = cache

	service, err := bytime.New(db, &fs, cwr)
	if err != nil {
		return nil, err
	}
	fs.syncByTime = service

	// 实时同步  **启动同步结束后，才启动实时同步
	backend := NewSyncBackend(&fs)
	fs.realtimeServer = bysolt.NewRealTimeSync(backend, api.DagChain(), db)

	if err := fs.initMsgHandle(); err != nil {
		return nil, err
	}
	return &fs, nil
}

func (s *SyncMain) Start() {
	go s.listenStatus()
	go s.listen()
	//s.unitCache.Start()
	s.syncByTime.Start()
	log.Info("started sync service")
}
func (s *SyncMain) Stop() {
	log.Debug("stopping sync service")
	close(s.quit)
	s.syncByTime.Stop()
	s.unitCache.Stop()
	s.realtimeServer.Stop()
	s.exitw.Wait()
	log.Info("stopped sync service")
}

func (s *SyncMain) initMsgHandle() error {
	s.msgHandles[message.CodeFetchUnitByID] = s.handleFetchUnitByID
	s.msgHandles[message.CodeFetchUnitsResponse] = s.handleFetchUnitResponse
	s.msgHandles[status.CodePeerStatus] = s.handleNodeStatus
	s.msgHandles[message.CodeRealTimeMsg] = s.realtimeServer.Handle
	s.msgHandles[message.CodeFetchUnitByHash] = s.handleFetchUnitByHash
	for code, h := range s.syncByTime.Protocol() {
		if _, ok := s.msgHandles[code]; ok {
			return errors.Errorf("repeat message handle,code is %d", code)
		}
		s.msgHandles[code] = h
	}
	return nil
}
func (s *SyncMain) PeerStatusSet() *status.PeerStatusSet {
	return s.peerStatus
}

func WarpMessage(msg interface{}) *message.Message {
	var code message.Code
	switch msg.(type) {
	case status.NodeStatus, *status.NodeStatus:
		code = status.CodePeerStatus
	case message.FetchUnitById, *message.FetchUnitById:
		code = message.CodeFetchUnitByID
	case message.FetchUnitsResponse, *message.FetchUnitsResponse:
		code = message.CodeFetchUnitsResponse
	case []common.Hash:
		code = message.CodeFetchUnitByHash
	default:
		panic(errors.Errorf("unknown msg type %T", msg))
	}
	return message.NewMsg(0, code, msg)
}

func (s *SyncMain) SendMessage(msg *message.Message, peers ...message.PeerID) error {
	log.Trace("sending message", "seq", msg.Seq, "req", msg.ResSeq, "code", msg.Code)
	//广播给所有节点
	if len(peers) == 0 {
		ps := s.peerSet.Peers()
		if len(ps) == 0 {
			return errors.Errorf("no peer connected when send msg(%d)", msg.Code)
		}
		for _, p := range ps {
			if err := p.AsyncSendMessage(message.MainCode, msg); err != nil {
				return errors.Wrapf(err, "send msg(%d) failed", msg.Code)
			}
		}
		return nil
	}
	var done bool
	for _, p := range peers {
		pp := s.peerSet.Peer(p)
		if pp != nil {
			if err := pp.AsyncSendMessage(message.MainCode, msg); err != nil {
				return errors.Wrapf(err, "send msg(%d) failed", msg.Code)
			}
			done = true
		}
	}
	if !done {
		return errors.Errorf("no peer find when send msg(%d)", msg.Code)
	}
	return nil
}

func (s *SyncMain) SendMessageWitCode(code message.Code, msg interface{}, peers ...message.PeerID) error {
	var warp *message.Message
	if code == 0 {
		warp = WarpMessage(msg)
	} else {
		warp = message.NewMsg(0, code, msg)
	}
	return s.SendMessage(warp, peers...)
}

func (s *SyncMain) createPeerEvent() (chan protocol.PeerEvent, event.Subscription) {
	ch := make(chan protocol.PeerEvent, 25)
	sub := s.api.SubscribePeerEvent(ch)
	return ch, sub
}
func (s *SyncMain) createChainEvent() (chan types.ChainEvent, event.Subscription) {
	ch := make(chan types.ChainEvent, 1024)
	sub := s.cr.SubscribeChainEvent(ch)
	return ch, sub
}
func (s *SyncMain) createChainStatusEvent() (chan types.ChainMCProcessStatusChangedEvent, event.Subscription) {
	ch := make(chan types.ChainMCProcessStatusChangedEvent, 100)
	sub := s.cr.SubscribeChainProcessStatusCHangedEvent(ch)
	return ch, sub
}

// 开启监听，
// 将监听本地新单元，如果本地链高度高于其他节点，则将此新高度信息发送给其他节点。
func (s *SyncMain) listen() {
	s.exitw.Add(1)
	defer s.exitw.Done()

	peerEvent, peerSub := s.createPeerEvent()
	defer peerSub.Unsubscribe()

	for {
		select {
		case <-s.quit:
			// release
			s.peerStatus.Range(func(key, value interface{}) bool {
				s.peerStatus.Delete(key)
				return true
			})
			return
		case ev := <-peerEvent:
			s.logger.Trace("peer event", "peer", ev.PeerID, "connected", ev.Connected)
			if ev.Connected {
				s.SendMyStatus(ev.PeerID)
			} else {
				s.peerStatus.Delete(ev.PeerID)
			}
		}
	}
}

func (s *SyncMain) listenStatus() {
	s.exitw.Add(1)
	defer s.exitw.Done()

	var (
		diffStarted bool
		missStarted bool
		ok          bool
	)
	checkStatus := func(unit *types.Unit) {
		var h *types.Header
		if unit == nil {
			h = s.api.DagChain().GetChainTailHead(params.SCAccount)
		} else {
			h = unit.Header()
		}
		if !diffStarted {
			// 如果系统链单元最后时间在允许的时间范围内则将开启同步
			if h.Number > 0 {
				diffStarted = true
				//异步启动，避免因为加载链过多而引起其他阻塞
				go func() {
					s.realtimeServer.Start()
					if unit != nil {
						s.realtimeServer.OnNewUnit(unit)
					}
					ok = true
				}()
			}
		}
		if !missStarted {
			if h.Number > 0 && time.Now().Sub(time.Unix(0, int64(h.Timestamp))) < defFetchMissSyncCanWorkTime {
				missStarted = true
				s.unitCache.Start()
			}
		}

	}
	if config.InStress { //压力测试时，账户信息不完整，不能开启
		diffStarted = true
	}
	//启动时检查
	checkStatus(nil)

	c, sub := s.createChainEvent() //注意：当前单元数量非常多时，如果此段代码处理过慢，会引起一定堵塞。因此需要设置一定的缓存大小。
	defer sub.Unsubscribe()
	for {
		select {
		case <-s.quit:
			return
		case ev := <-c:
			//更新本地状态成功
			s.broadcastNewUnit(ev.Unit)
			s.peerStatus.CheckMyStatus(ev.Unit)

			if ev.Unit.MC() == params.SCAccount {
				checkStatus(ev.Unit)
			}
			if ok {
				s.realtimeServer.OnNewUnit(ev.Unit)
			}
			//s.realtimeServer.NewChainEvent(realtime.ChainStatus{Addr: ev.Unit.MC(), Number: ev.Unit.Number()})
		}
	}
}

func (s *SyncMain) broadcastNewUnit(unit *types.Unit) {
	//说明该单元不是来自于网络，而是由本地产生（见证人所出单元），此时不需要广播，因为有专门的广播通道广播
	if unit.ReceivedAt.IsZero() && !unit.IsBlackUnit() {
		return
	}
	begin := mclock.Now()

	var peers []protocol.Peer
	currentWitness := s.api.CurrentMiner()
	if currentWitness.Empty() {
		peers = s.peerSet.Peers()
	} else {
		//if unit.ReceivedAt.IsZero() { // 本地的单元
		//	voteList, err := types.GetUnitVoter(unit)
		//	if err != nil {
		//		s.logger.Debug("failed:get unit vote list", "unitHash", unit.Hash(), "err", err)
		//		return
		//	}
		//	if voteList.HaveIdx(currentWitness) < voteList.Len()/3 {
		//		peers = s.peerSet.Peers()
		//	}
		//} else {

		// 同组见证人广播过来的单元不广播到见证组内
		if !s.api.IsJoinWitnessGroup(unit.Proposer()) {
			peers = s.peerSet.Peers()
		}

	}
	//广播新单元给其他节点
	for _, p := range peers {
		//如果单元来自此 Peer 则不再转发给他
		if id, ok := unit.ReceivedFrom.(discover.NodeID); ok && p.ID() == id {
			continue
		}

		//如果Peer的链高度低且不存在此单元时，异步发送给他

		p.AsyncSendMessage(protocol.NewUnitMsg, unit)
	}
	s.logger.Trace("broadcast new unit ", "uhash", unit.Hash(), "sends", len(peers),
		"cost", common.PrettyDuration(mclock.Now()-begin))
}

// HandleMessage 处理消息，收到请求进行处理
func (s *SyncMain) HandleMessage(peerID message.PeerID, raw message.Decoder) error {
	select {
	case <-s.quit:
		return errors.New("full sync is closed")
	default:
		msg, err := message.DecodeMsg(peerID, raw)
		if err != nil {
			return err
		}
		h := s.msgHandles[msg.Code]
		s.logger.Trace("received sync message",
			"seq", msg.Seq, "code", msg.Code, "resseq", msg.ResSeq,
			"src", peerID,
			"handle", h == nil)

		if h == nil {
			return fmt.Errorf("can not handle message with code %d", msg.Code)
		}
		err = h(msg)
		msg.Playload = nil //保险的做法，用于让 GC 识别到 msg.Playload 已无人使用

		if err != nil {
			s.logger.Trace("handel sync message failed",
				"seq", msg.Seq, "code", msg.Code, "resseq", msg.ResSeq,
				"src", msg.Src, "err", err)
			return errors.Wrapf(err, "failed to handle message %d", msg.Code)
		}
	}
	return nil
}

func (s *SyncMain) SendMyStatus(pid message.PeerID) {
	p := s.peerSet.Peer(pid)
	if p == nil {
		return
	}
	s.SendMessage(WarpMessage(s.peerStatus.MyStatus()), pid)
}

// 处理状态
func (s *SyncMain) handleNodeStatus(msg *message.Message) error {
	var status status.NodeStatus
	if err := rlp.DecodeBytes(msg.Playload, &status); err != nil {
		return err
	}
	g := s.api.DagChain().Genesis()
	if status.LastUintTime < g.Timestamp() {
		return errors.New("invalid uint time")
	}
	if h := s.cr.GetHeaderByHash(status.LastUintHash); h != nil && h.Timestamp != status.LastUintTime {
		return errors.New("invalid uint time")
	}
	if h := s.cr.GetHeaderByHash(status.SysTailHash); h != nil && h.Number != status.SysTailNumber {
		return errors.New("invalid system tail")
	}

	s.peerStatus.Update(msg.Src, status)
	return nil
}

// 通过单元ID 拉取单元
func (s *SyncMain) FetchUnitByID(uid types.UnitID) error {
	if uid.IsEmpty() {
		return errors.New("invalid uid")
	}
	best := s.peerStatus.Best(nil)
	if best.IsEmpty() {
		return errors.New("peers is empty")
	}
	return s.SendMessage(WarpMessage(message.FetchUnitById{uid}), best)
}

// 通过单元哈希 拉取单元
func (s *SyncMain) FetchUnitByHash(hashes []common.Hash, peer ...message.PeerID) error {
	var p message.PeerID
	if len(peer) > 0 {
		p = peer[0]
	} else {
		p = s.peerStatus.Best(nil)
	}
	if p.IsEmpty() {
		return errors.New("peers is empty")
	}
	log.Trace("request fetch unit", "len", len(hashes), "peer", p)

	return s.SendMessage(WarpMessage(hashes), p)
}

func (s *SyncMain) handleFetchUnitByHash(msg *message.Message) error {
	var hashes []common.Hash
	if err := rlp.DecodeBytes(msg.Playload, &hashes); err != nil {
		return errors.Wrap(err, "rlp decode message failed")
	}
	if len(hashes) == 0 {
		return errors.New("invalid message")
	}
	unitrlp := util.GetUnitRawRLP(s.db, hashes, message.DefMaxUnitSize)
	log.Debug("response fetch unit", "hashes", len(hashes), "dat.len", len(unitrlp))
	if len(unitrlp) == 0 {
		s.logger.Trace("missing unit", "hashes", len(hashes))
		return nil
	}
	//发送给对方
	response := WarpMessage(&message.FetchUnitsResponse{unitrlp})
	response.ResSeq = msg.Seq
	err := s.SendMessage(response, msg.Src)
	if err != nil {
		log.Trace("failed to send message", "size", common.StorageSize(len(unitrlp)), "err", err)
	}
	return nil
}
func (s *SyncMain) handleFetchUnitByID(msg *message.Message) error {
	var req message.FetchUnitById
	if err := rlp.DecodeBytes(msg.Playload, &req); err != nil {
		return errors.Wrap(err, "rlp decode message failed")
	}
	if err := req.Check(); err != nil {
		return errors.Wrap(err, "invalid message")
	}

	//需要根据高度拉取一批单元
	if req.UID.Hash.Empty() {
		return s.handleFetchUnitByNumber(msg, req)
	}
	b := util.EncodeUnit(s.db, req.UID)
	if len(b) == 0 {
		s.logger.Trace("missing unit", "chain", req.UID.ChainID, "number", req.UID.Height, "uhash", req.UID.Hash)
		return nil
	}
	//发送给对方
	response := WarpMessage(&message.FetchUnitsResponse{b})
	response.ResSeq = msg.Seq
	err := s.SendMessage(response, msg.Src)
	if err != nil {
		log.Trace("failed to send message", "size", common.StorageSize(len(b)), "err", err)
	}
	return nil
}

//获取链的一部分数据
func (s *SyncMain) handleFetchUnitByNumber(msg *message.Message, req message.FetchUnitById) error {

	//一次性不需要拉取太多
	var (
		begin           = time.Now()
		number          = req.UID.Height
		maxUnits uint64 = 11
	)
	data := util.DyncGetUnitRawRLPByIds(s.db, message.DefMaxUnitSize, func() types.UnitID {
		if number-req.UID.Height > maxUnits {
			return types.UnitID{}
		}
		hash := s.cr.GetStableHash(req.UID.ChainID, number)
		if hash.Empty() {
			return types.UnitID{}
		}
		uid := types.UnitID{
			ChainID: req.UID.ChainID,
			Hash:    hash,
			Height:  number,
		}
		number++
		return uid
	})
	if len(data) == 0 {
		return nil
	}
	response := WarpMessage(&message.FetchUnitsResponse{data})
	response.ResSeq = msg.Seq
	err := s.SendMessage(response, msg.Src)
	if err != nil {
		log.Trace("failed to send message", "size", common.StorageSize(len(data)), "err", err)
	}
	log.Trace("response fetch unit by number", "req", msg.Seq,
		"chain", req.UID.ChainID, "begin", req.UID.Height, "end", number, "cost", time.Since(begin))
	return nil
}

func (s *SyncMain) HandleFetchUnitResponse(msg *message.Message) (success int, units []*types.Unit, err error) {
	var res message.FetchUnitsResponse
	if err := rlp.DecodeBytes(msg.Playload, &res); err != nil {
		return 0, units, errors.Wrap(err, "rlp decode message failed")
	}

	units, err = util.DecodeUnitRawRLP(res.UnitsRLP)
	if err != nil {
		return 0, units, errors.Wrap(err, "rlp decode units")
	}
	if len(units) == 0 {
		return 0, units, errors.New("response is empty")
	}
	success, err = s.ProcessUnits(msg.Seq, msg.Src, units)
	s.logger.Trace("process unit done", "peer", msg.Src,
		"seq", msg.Seq, "req", msg.ResSeq,
		"size", common.StorageSize(len(msg.Playload)),
		"sum", len(units), "success", success, "failed", len(units)-success,
		"lastUnitTime", units[len(units)-1].Timestamp())
	return success, units, err
}
func (s *SyncMain) handleFetchUnitResponse(msg *message.Message) error {
	_, _, err := s.HandleFetchUnitResponse(msg)
	return err
}

//处理接收到的单元
//当单元处理完毕时，如果无法落地成功的单元将进入 Cache 中。
func (s *SyncMain) ProcessUnits(seq uint64, src message.PeerID, units []*types.Unit) (success int, err error) {
	var oks int
	var badErr error
	for i := 0; i < len(units); i++ {
		select {
		case <-s.quit:
			return oks, nil
		default:
		}

		unit := units[i]
		unit.ReceivedAt = time.Now() //标记为网络接收时间
		unit.ReceivedFrom = src
		_, err := s.cr.InsertChain(types.Blocks{unit}) //写入数据
		if err != nil {
			if consensus.IsBadUnitErr(err) {
				//否则，作为坏 Peer，由外部处理
				badErr = err
			}
			if err == consensus.ErrKnownBlock {
				oks++
			}
			s.logger.Trace("try insert unit failed", "seq", seq, "timestamp", unit.Timestamp(),
				"chain", unit.MC(), "number", unit.Number(), "uhash", unit.Hash(), "err", err)

			if err == consensus.ErrUnknownAncestor {
				select {
				case <-s.quit:
				default:
					//如果是因为缺祖先而无法落地单元时，直接找此人拉取祖先单元。
					//此时可以直接找此节点拉取，拉取时取链
					s.unitCache.FetchAncestor(src, unit)
				}
			}
		} else {
			oks++
		}
	}
	//如果有成功内容，则发送
	if oks > 0 {
		s.unitCache.ProcessCache()
	}
	return oks, badErr
}

func (s *SyncMain) RealtimeEngine() *bysolt.RealTimeSync {
	return s.realtimeServer
}
