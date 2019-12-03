package bytime

import (
	"sync"
	"time"

	"gitee.com/nerthus/nerthus/nts/synchronize/util"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/rawdb"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/nts/synchronize/message"
	"gitee.com/nerthus/nerthus/nts/synchronize/status"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/rlp"

	"github.com/pkg/errors"
)

const (
	defWaitResponseTimeout = time.Second * 30
)

type MsgHandle = message.HandleMsg

// 按时间线同步单元
type Sync struct {
	peers      sync.Map // key : discover.NodeID, value=*SyncPeer
	core       MainSync
	db         ntsdb.Database
	peerStatus *status.PeerStatusSet
	cwr        ChainWriteRead

	canTrySync   chan message.PeerID
	waitResponse chan uint64
	syncLocker   sync.RWMutex //是否是在同步中
	syncNextTime uint64

	skipPeer sync.Map

	logger log.Logger
	exitw  sync.WaitGroup
	quit   chan struct{}
}

// 实例化服务，
// 将采用深度优先模式同步数据，并实时记录已连接节点的状态
func New(db ntsdb.Database, core MainSync, cwr ChainWriteRead) (*Sync, error) {
	s := Sync{
		core:         core,
		cwr:          cwr,
		db:           db,
		logger:       log.New("module", "sync"),
		quit:         make(chan struct{}),
		peerStatus:   core.PeerStatusSet(),
		canTrySync:   make(chan message.PeerID, 1),
		waitResponse: make(chan uint64, 10),
		skipPeer:     sync.Map{},
	}

	return &s, nil
}

func (s *Sync) Start() {
	go s.listenChange()
	go s.listenSync()
}

func (s *Sync) Stop() {
	close(s.quit)
	s.exitw.Wait()
}

func (s *Sync) Protocol() map[message.Code]message.HandleMsg {
	return map[message.Code]message.HandleMsg{
		CodeFetchUnitsByTime:   s.handleFetchUnitRequest,
		CodeFetchUnitsResponse: s.handleFetchUnitResponse,
	}
}

func (s *Sync) SendMessage(req uint64, msg interface{}, peers ...message.PeerID) (uint64, error) {
	var code Code
	switch msg.(type) {
	case FetchUnitsByTime, *FetchUnitsByTime:
		code = CodeFetchUnitsByTime
	case message.FetchUnitsResponse, *message.FetchUnitsResponse:
		code = CodeFetchUnitsResponse
	default:
		panic(errors.Errorf("unknown msg type %T", msg))
	}
	info := message.NewMsg(req, code, msg)
	if code == CodeFetchUnitsByTime {
		log.Trace("send fetch units", "seq", info.Seq, "start", (msg.(FetchUnitsByTime).Start), "myStatus", s.peerStatus.MyStatus().LastUintTime)
	}
	return info.Seq, s.core.SendMessage(info, peers...)
}

func (s *Sync) listenChange() {
	s.exitw.Add(1)
	defer s.exitw.Done()

	// 监听 Peer 状态改变信息，一旦有新的变化。则可以尝试拉取数据
	changec := make(chan message.PeerID, 10)

	sub := s.peerStatus.SubChange(changec)
	defer sub.Unsubscribe()
	for {
		select {
		case <-s.quit:
			return
		case <-changec:
			s.notifySync()
		}
	}
}
func (s *Sync) notifySync(ps ...message.PeerID) {
	var p message.PeerID
	if len(ps) > 0 {
		p = ps[0]
	}
	select {
	case s.canTrySync <- p:
	case <-s.quit:
		return
	default:
	}
}

//监听同步处理
func (s *Sync) listenSync() {
	s.exitw.Add(1)
	defer s.exitw.Done()

	tk := time.NewTicker(time.Minute)
	defer tk.Stop()

	for {
		select {
		case <-s.quit:
			return
		case <-tk.C: //间隔触发
			s.notifySync()
		case p := <-s.canTrySync:
			err := s.do(p)
			if err != nil {
				s.logger.Debug("sync failed", "err", err)
			}
		}
	}

}

// 同步时，寻找到一个最优节点，并向他询问，拉取单元数据。
func (s *Sync) do(better message.PeerID) error {
	//如果已经在同步处理数据，则此时不应该发送新的请求，需要等待处理完毕
	s.syncLocker.RLock()
	defer s.syncLocker.RUnlock()

	if !better.IsEmpty() { //如果 Peer 不为空，则优先使用此 Peer
		peerStatus := s.peerStatus.Get(better)
		if peerStatus.Compare(s.peerStatus.MyStatus()) > 0 {
			better = message.PeerID{}
		}
	}

	if better.IsEmpty() {
		var ok bool
		better, ok = s.peerStatus.Rich(s.peerStatus.MyStatus(), func(id status.PeerID) bool {
			_, ok := s.skipPeer.Load(id)
			if ok { //如果已存在，则删除以便下次可尝试采用
				s.skipPeer.Delete(id)
			}
			return ok
		})
		//说明没有比自己状态更换的节点，则不需要继续。
		if !ok {
			return errors.New("no one is better than me")
		}
	}

	status := s.peerStatus.MyStatus()
	if status.LastUintTime <= s.syncNextTime {
		return errors.Errorf("stop sync by timeline now is %d,should > %d", status.LastUintTime, s.syncNextTime)
	}
	req := FetchUnitsByTime{Start: status.LastUintTime + 1, End: 0}
	//时间必须延长不能一直停留在远处

	begin := time.Now()
	//发送请求消息给富人
	seq, err := s.SendMessage(0, req, better)
	if err != nil {
		return err
	}
	//我们会等待富人的响应
	select {
	case <-s.quit:
		err = errors.New("quit")
	case <-time.After(defWaitResponseTimeout):
		s.skipPeer.Store(better, struct{}{})
		err = errors.New("timeout")
	case <-s.waitResponse:
		_ = seq
	}
	log.Trace("send fetch unit by time request done",
		"target", better, "req", seq,
		"start", req.Start,
		"lastUnit", status.LastUintHash,
		"system.tail", status.SysTailHash,
		"cost", time.Now().Sub(begin), "err", err)
	return err
}

// 处理消息：拉取单元请求
func (s *Sync) handleFetchUnitRequest(msg *message.Message) error {
	var req FetchUnitsByTime
	if err := rlp.DecodeBytes(msg.Playload, &req); err != nil {
		return errors.Wrap(err, "rlp decode message failed")
	}
	if err := req.Check(); err != nil {
		return errors.Wrap(err, "invalid FetchUnitsRequest msg")
	}

	// 响应时，将直接根据总体消息吧大小推送数据,0.9M限制，可以推送约581个单元（<=1kb）
	var (
		units   int
		hashc   = make(chan common.Hash)
		stopAdd = make(chan struct{})
	)

	go func() {
		//按时间线获取单元哈希
		rawdb.RangUnitByTimline(s.db, req.Start, func(hash common.Hash) bool {
			select {
			case hashc <- hash:
				return true
			case <-s.quit:
				return false
			case <-stopAdd:
				return false
			}
		})
		close(hashc)
	}()

	//将载入的单元哈希，进入打包
	b := util.DyncGetUnitRawRLP(s.db, message.DefMaxUnitSize, func() common.Hash {
		select {
		case h, ok := <-hashc:
			if ok {
				units++
				return h
			}
		case <-s.quit:
		}
		return common.Hash{}
	})
	close(stopAdd)

	//如果本地无新数据，则不发送
	if len(b) == 0 {
		return nil
	}
	//发送给对方
	log.Trace("response fetch unit by time", "requester", msg.Src,
		"req", msg.Seq, "units", units, "size", common.PrettyDuration(len(b)))
	_, err := s.SendMessage(msg.Seq, &message.FetchUnitsResponse{b}, msg.Src)
	if err != nil {
		log.Trace("failed to send message", "size", common.StorageSize(len(b)), "err", err)
	}
	return nil
}

func (s *Sync) handleFetchUnitResponse(msg *message.Message) error {
	select {
	case s.waitResponse <- msg.ResSeq:
	default:
	}

	//锁定单元落地，也不需要支持并发。
	//  1. 因为所有单元存在顺序，如果并发可能使得大部分单元无法落地
	//  2. 上锁是告诉其他人，此时正在落地单元
	s.syncLocker.Lock()
	defer s.syncLocker.Unlock()
	//此时，可以优先讯问
	oks, units, err := s.core.HandleFetchUnitResponse(msg)
	//如果拉取到但无法落地时，更新下次拉取开始位置
	if oks != len(units) && len(units) > 0 {
		last := units[len(units)-1].Timestamp()
		if last > s.syncNextTime {
			s.syncNextTime = last
			log.Trace("update next begin sync timeline", "timeline", s.syncNextTime)
		}
	}
	if err == nil && oks == len(units) {
		//如果有拉取到数据，则可以继续向他拉取
		s.notifySync(msg.Src)
	}
	return err
}
