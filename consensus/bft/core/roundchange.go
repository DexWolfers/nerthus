package core

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/tracetime"
	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/consensus/protocol"
	"gitee.com/nerthus/nerthus/log"
	"math/big"
	"sync"
)

// 申请round变更
func (c *core) sendNextRoundChange() {
	// 已收到F+1的目标轮次变更，直接发送该轮次变更
	cv := c.currentView()
	maxRound := c.roundChangeSet.MaxRound(c.valSet.F() + 1)
	if maxRound != nil && maxRound.Cmp(cv.Round) > 0 {
		c.sendRoundChange(maxRound)
	} else {
		c.sendRoundChange(new(big.Int).Add(cv.Round, big.NewInt(1)))
	}
}

// 广播round change消息
func (c *core) sendRoundChange(round *big.Int, trace ...*tracetime.TraceTime) {
	var tr *tracetime.TraceTime
	if len(trace) == 0 {
		tr = tracetime.New()
		defer tr.Stop()
	} else {
		tr = trace[0]
	}
	tr.Tag("sendrd")
	c.addDebugLog("do", "sendRoundChange")

	tr.Tag()
	cv := c.currentView()
	c.catchUpRound(&bft.View{
		Round:    new(big.Int).Set(round),
		Sequence: new(big.Int).Set(cv.Sequence),
	})

	tr.Tag()
	rc := &bft.Subject{
		View: &bft.View{
			Round:    new(big.Int).Set(round),
			Sequence: new(big.Int).Set(cv.Sequence),
		},
		Digest: common.Hash{},
	}

	payload, err := Encode(rc)
	if err != nil {
		c.logger.Error("Failed to encode ROUND CHANGE", "rc", rc, "err", err)
		return
	}
	tr.Tag()
	c.broadcast(&protocol.Message{
		Code: protocol.MsgRoundChange,
		Msg:  payload,
	})
	c.logger.Debug("send a roundchange msg", "view", rc.View)
}

// 处理round change消息
func (c *core) handleRoundChange(msg *protocol.Message, src bft.Validator, trace ...*tracetime.TraceTime) error {
	var tr *tracetime.TraceTime
	if len(trace) == 0 {
		tr = tracetime.New("chain", c.chainAddress, "number", c.current.sequence, "round", c.current.round, "createat", c.roundchangeResetTime).SetMin(0)
		defer tr.Stop()
	} else {
		tr = trace[0]
	}
	tr.Tag("round")
	logger := c.logger.New("state", c.state, "from", src.Address().Hex(), "wait", c.waitingForRoundChange)

	var rc bft.Subject
	if err := msg.Decode(&rc); err != nil {
		logger.Error("Failed to decode ROUND CHANGE", "err", err)
		return errInvalidMessage
	}
	tr.Tag()
	if err := c.checkMessage(protocol.MsgRoundChange, rc.View); err != nil {
		return err
	}
	tr.Tag()
	cv := c.currentView()
	roundView := rc.View
	// Add the ROUND CHANGE message to its message set and return how many
	// messages we've got with the same round number and sequence number.
	num, err := c.roundChangeSet.Add(roundView.Round, msg)
	tr.AddArgs("size", num, "from", src.Address())
	if err != nil {
		logger.Trace("Failed to add round change message", "from", src, "votes", num, "msgRound", roundView.Round, "msg", msg, "err", err)
		return err
	}
	tr.Tag()
	// f+1时跟投 ps:允许跟投旧的round消息
	if num == c.valSet.F()+1 && src.Address() != c.address {
		c.sendRoundChange(roundView.Round)
		tr.Tag()
	} else if num > c.valSet.TwoThirdsMajority() && cv.Round.Cmp(roundView.Round) < 0 {
		// round 只能向上增加
		if roundView.Round.Cmp(cv.Round) > 0 {
			c.startNewRound(roundView.Round, nil, tr)
		}
		tr.Tag()
	}
	logger.Debug("Handle round change finish", "msgRound", roundView.Round, "votes", num)
	return nil
}

// ----------------------------------------------------------------------------

func newRoundChangeSet(valSet bft.ValidatorSet, preRoundChangeSignBytes [][]byte) *roundChangeSet {
	return &roundChangeSet{
		validatorSet:       valSet,
		roundChanges:       make(map[uint64]*messageSet),
		preRoundChangeSign: preRoundChangeSignBytes,
		lock:               new(sync.Mutex),
	}
}

type roundChangeSet struct {
	validatorSet       bft.ValidatorSet
	preRoundChangeSign [][]byte
	roundChanges       map[uint64]*messageSet // 索引为round
	lock               *sync.Mutex
}

func (rcs *roundChangeSet) Add(r *big.Int, msg *protocol.Message) (int, error) {
	rcs.lock.Lock()
	defer rcs.lock.Unlock()

	round := r.Uint64()
	if rcs.roundChanges[round] == nil {
		rcs.roundChanges[round] = newMessageSet(rcs.validatorSet)
	}
	err := rcs.roundChanges[round].Add(msg)
	return rcs.roundChanges[round].Size(), err
}

func (rcs *roundChangeSet) RoundMessageSet(r *big.Int) *messageSet {
	rcs.lock.Lock()
	defer rcs.lock.Unlock()

	round := r.Uint64()
	if rcs.roundChanges[round] == nil {
		return nil
	}
	msgSet := rcs.roundChanges[round].Copy()
	return &msgSet
}

func (rcs *roundChangeSet) PreRoundChangeBytes() [][]byte {
	rcs.lock.Lock()
	defer rcs.lock.Unlock()
	return rcs.preRoundChangeSign
}

func (rcs *roundChangeSet) MaxRoundChangesBytes(num int) (*big.Int, [][]byte) {
	if rcs == nil {
		return nil, nil
	}
	round := rcs.MaxRound(num)
	if round == nil {
		return nil, nil
	}
	rcs.lock.Lock()
	defer rcs.lock.Unlock()
	if msgSet := rcs.roundChanges[round.Uint64()]; msgSet == nil {
		return round, nil
	} else {
		var buf [][]byte
		for _, msg := range msgSet.Values() {
			buf = append(buf, msg.PacketProveTreeBytes())
			log.Trace("Single round change prove tree size", "size", len(buf[len(buf)-1]))
		}
		return round, buf
	}
}

// 清理小于round或者消息为空的消息集
func (rcs *roundChangeSet) Clear(round *big.Int) {
	rcs.lock.Lock()
	defer rcs.lock.Unlock()

	for k, rms := range rcs.roundChanges {
		if len(rms.Values()) == 0 || k < round.Uint64() {
			delete(rcs.roundChanges, k)
		}
	}
}

// MaxRound returns the max round which the number of messages is equal or larger than num
func (rcs *roundChangeSet) MaxRound(num int) *big.Int {
	rcs.lock.Lock()
	defer rcs.lock.Unlock()

	var maxRound *big.Int
	for k, rms := range rcs.roundChanges {
		if rms.Size() < num {
			continue
		}

		r := big.NewInt(int64(k))
		if maxRound == nil || maxRound.Cmp(r) < 0 {
			maxRound = r
		}
	}

	return maxRound
}
