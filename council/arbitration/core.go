package arbitration

import (
	"fmt"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
	"github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
	"sync"
	"time"
)

var (
	errOldMsg    = errors.New("old message")
	errRepeatMsg = errors.New("repeated council vote")
)

func newCore(addr common.Address, backend Backend, activatedCallback func()) *core {
	cache, _ := lru.New(128)
	return &core{
		address:         addr,
		backend:         backend,
		mux:             sync.Mutex{},
		logger:          log.Root().New("module", "council", "node", addr),
		history:         cache,
		activedCallback: activatedCallback,
	}
}

type core struct {
	isStarted       bool
	address         common.Address
	lockHash        common.Hash
	backend         Backend
	scTail          uint64
	proposal        Proposal
	councils        []common.Address
	votes1          map[common.Address]*CouncilMessage
	votes2          map[common.Address]*CouncilMessage
	state           State
	mux             sync.Mutex
	logger          log.Logger
	timer           *time.Timer
	history         *lru.Cache
	blackList       map[common.Address][]*CouncilMessage // 作恶证据
	activedCallback func()
}

func (t *core) Start() {
	t.mux.Lock()
	defer t.mux.Unlock()
	// 启动仲裁
	var err error
	t.councils, err = t.backend.GetCouncils()
	if err != nil {
		t.logger.Error("start councils arbitration error", "err", err)
		return
	}
	t.votes1 = make(map[common.Address]*CouncilMessage)
	t.votes2 = make(map[common.Address]*CouncilMessage)
	t.blackList = make(map[common.Address][]*CouncilMessage)

	tail := t.backend.ScTail()
	t.scTail = tail.Number
	t.isStarted = true
	t.logger.Info("start council arbitration", "sc number", tail.Number, "councils", len(t.councils))
}
func (t *core) Stop() {
	t.mux.Lock()
	defer t.mux.Unlock()
	t.stopTimer()
	t.lockHash = common.Hash{}
	t.scTail = 0
	t.proposal = nil
	t.councils = nil
	t.votes1 = nil
	t.votes2 = nil
	t.setState(StateRunning)
	t.isStarted = false
	t.logger.Info("stop council arbitration")
}
func (t *core) Started() bool {
	t.mux.Lock()
	defer t.mux.Unlock()
	return t.isStarted
}
func (t *core) Active() {
	t.mux.Lock()
	defer t.mux.Unlock()
	t.active(0)
}

func (t *core) HandleVote(msg *CouncilMessage) error {
	t.mux.Lock()
	defer t.mux.Unlock()
	err := t.verify(msg)
	defer func() {
		t.logger.Debug("handle council vote", "state", t.state, "size1", len(t.votes1), "size2", len(t.votes2), "error", err,
			"msg_state", msg.Status, "msg_tail", msg.ScTail, "msg_dist", msg.Dist, "from", msg.From)
	}()
	if err != nil {
		t.logger.Error("handle council vote err", "error", err)
		if err == errOldMsg {
			return nil
		}
		return err
	}
	switch msg.Status {
	case StateStop:
		err = t.handleVote1(msg)
	case StatePrepare:
		err = t.handleVote2(msg)
	default:
		t.logger.Error("invalid message", "from", msg.From, "msg state", msg.Status)
		return errors.New("invalid message code")
	}
	if err == errRepeatMsg {
		return nil
	}
	return err
}
func (t *core) active(scNumber uint64) {
	if t.state > StateRunning {
		return
	}
	var err error
	defer func() {
		if err != nil {
			t.logger.Error("activate arbitration error", "err", err)
		}
	}()
	activePassive := scNumber > 0 //是主动激活还是被动
	if activePassive {
		// 此高度票数>1/3
		num := 0
		for _, v := range t.votes1 {
			if v.ScTail == scNumber {
				num++
			}
		}
		if num <= t.majority31() {
			err = errors.New("votes count not enough 1/3+1")
			return
		}
	}
	// 本地状态
	tail := t.backend.ScTail()
	if scNumber == 0 {
		scNumber = tail.Number
	}
	if tail.Number > t.scTail {
		// 更新链信息
		t.scTail = tail.Number
		t.councils, err = t.backend.GetCouncils()
		if err != nil {
			return
		}
	}
	if tail.Number != scNumber {
		// 视角不一致，放弃激活
		err = fmt.Errorf("different sc tail local:%d remote:%d", tail.Number, scNumber)
		return
	} else {
		// 构建特殊单元
		if t.proposal, err = t.backend.GenerateProposal(tail); err != nil {
			return
		}
	}
	// 清除无效票
	for k := range t.votes1 {
		if t.votes1[k].ScTail != t.scTail {
			delete(t.votes1, k)
		}
	}
	for k := range t.votes2 {
		if t.votes2[k].ScTail != t.scTail {
			delete(t.votes2, k)
		}
	}
	// 被动激活需要调用回调函数
	if activePassive && t.activedCallback != nil {
		go t.activedCallback()
	}
	t.setState(StateStop)
	t.sendVote1()
	// 启动一个时钟，定时重发投票
	t.startTimer()
	t.logger.Warn("council arbitration vote activating", "sc number", t.scTail, "sc root", tail.StateRoot, "proposal", t.proposal.Hash())
}
func (t *core) verify(msg *CouncilMessage) error {
	// 是否作恶
	if _, ok := t.blackList[msg.From]; ok {
		return errors.New("dishonest council address")
	}
	// 是否有效理事
	valid := false
	for i := range t.councils {
		if msg.From == t.councils[i] {
			valid = true
			break
		}
	}
	if !valid {
		return errors.New("invalid council address")
	}
	// 不能在历史的高度仲裁
	if t.state == StateRunning && msg.ScTail+1 < t.scTail {
		return errors.New("old height message")
	}
	// 必须在同一个系统链高度
	if t.state > StateRunning && msg.ScTail != t.scTail {
		if msg.ScTail > t.scTail {
			// 重新查询本地
			if t.backend.ScTail().Number > t.scTail {
				// 重置本地状态
				t.Stop()
				return nil
			}
		}
		return errors.New("invalid council message of sc number")
	}
	if msg.Status < t.state {
		return errOldMsg
	}
	// 去重
	if _, ok := t.history.Get(msg.Dist); ok {
		return errOldMsg
	} else {
		if t.backend.GetUnitByHash(msg.Dist) != nil {
			t.history.Add(msg.Dist, struct{}{})
			return errOldMsg
		}
	}
	// 签名校验
	if sender, err := t.backend.VerifySign(msg); err != nil {
		return err
	} else if sender != msg.From {
		return errors.New("invalid sign")
	}
	return nil
}
func (t *core) handleVote1(msg *CouncilMessage) error {
	if v, ok := t.votes1[msg.From]; ok {
		if msg.ScTail > v.ScTail {
			t.votes1[msg.From] = msg
		} else if v.Dist == msg.Dist {
			return errRepeatMsg
		} else {
			//理事作恶，从列表移除并保存证据
			delete(t.votes1, msg.From)
			delete(t.votes2, msg.From)
			t.blackList[v.From] = []*CouncilMessage{v, msg}
			t.logger.Error("discover council dishonest", "address", msg.From)
			return errors.New("council dishonest")
		}
	}
	t.votes1[msg.From] = msg
	count := len(t.votes1)
	// 1/3+1时,激活投票
	if t.state == StateRunning && count > t.majority31() {
		t.active(msg.ScTail)
		return nil
	}
	// 投票总数需要大于2/3
	if t.state == StateStop && count > t.majority32() {
		// 统计投票
		result := make(map[common.Hash]int)
		for _, v := range t.votes1 {
			vcount := result[v.Dist]
			vcount++
			result[v.Dist] = vcount
		}
		for k, v := range result {
			// 超过2/3投票一致
			if v > t.majority32() {
				t.lockDist(k)
				t.setState(StatePrepare)
				t.sendVote2()
				return nil
			}
		}
	}
	return nil
}
func (t *core) handleVote2(msg *CouncilMessage) error {
	if _, ok := t.votes2[msg.From]; ok {
		return errRepeatMsg
	}
	if !t.lockHash.Empty() && msg.Dist != t.lockHash {
		return errors.New("invalid council vote")
	}
	t.votes2[msg.From] = msg
	count := len(t.votes2)
	if count > t.majority32() {
		// 统计投票
		result := make(map[common.Hash]int)
		for _, v := range t.votes2 {
			vcount := result[v.Dist]
			vcount++
			result[v.Dist] = vcount
			// >2/3投票一致
			if vcount == t.majority32()+1 {
				if t.lockHash.Empty() {
					t.lockDist(v.Dist)
				}
				t.setState(StateCommited)
				t.logger.Debug("votes enough,commit arbitration unit", "dist", v.Dist)
				t.commit()
				return nil
			}
		}
	}

	return nil
}
func (t *core) sendVote1() {
	msg := &CouncilMessage{
		From:   t.address,
		ScTail: t.scTail,
		Dist:   t.proposal.Hash(),
		Status: StateStop,
	}
	if t.wrapMsg(msg) == nil {
		t.broadcast(msg)
	}
	t.logger.Debug("send a vote1")
}
func (t *core) sendVote2() {
	msg := &CouncilMessage{
		From:   t.address,
		ScTail: t.scTail,
		Dist:   t.lockHash,
		Status: StatePrepare,
	}
	if t.wrapMsg(msg) == nil {
		t.broadcast(msg)
	}
	t.logger.Debug("send a vote2")
}
func (t *core) commit() {
	t.stopTimer()
	if t.lockHash == t.proposal.Hash() {
		votes := make([]types.WitenssVote, 0)
		for k := range t.votes2 {
			if t.votes2[k].Dist == t.lockHash {
				votes = append(votes, types.WitenssVote{Extra: t.votes2[k].Extra(), Sign: t.votes2[k].Sign})
			}
		}
		if err := t.backend.Commit(t.proposal, votes); err != nil {
			t.logger.Error("commit arbitration result error", "err", err)
			return
		}
		t.logger.Debug("commit unit success", "hash", t.proposal.Hash())
		t.history.Add(t.proposal.Hash(), struct{}{})
	} else {
		t.logger.Debug("commit proposal is different to mine", "commit", t.proposal.Hash(), "mine", t.lockHash)
	}
}
func (t *core) lockDist(dist common.Hash) {
	t.lockHash = dist
	// 清理不一致的vote2
	if t.state == StateStop {
		for k := range t.votes2 {
			if t.votes2[k].Dist != dist {
				delete(t.votes2, k)
			}
		}
	}
}
func (t *core) setState(new State) {
	t.state = new
}
func (t *core) wrapMsg(msg *CouncilMessage) error {
	if err := t.backend.Sign(msg); err != nil {
		t.logger.Error("sign error", "err", err)
		return err
	}
	return nil
}
func (t *core) broadcast(msg *CouncilMessage) {
	// to me
	go t.HandleVote(msg)
	// broadcast
	t.backend.Broadcast(msg)
}
func (t *core) startTimer() {
	if t.timer != nil {
		t.timer.Stop()
	}
	t.timer = time.AfterFunc(time.Second*60, func() {
		t.mux.Lock()
		defer t.mux.Unlock()
		if !t.isStarted {
			return
		}
		if t.state > StateStop {
			t.sendVote2()
		}
		t.sendVote1()
		t.startTimer()
	})
}
func (t *core) stopTimer() {
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
}
func (t *core) majority32() int {
	return len(t.councils) * 2 / 3
}
func (t *core) majority31() int {
	return len(t.councils) / 3
}
