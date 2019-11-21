package core

import (
	"math/big"
	"sync"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/tracetime"
)

type traceState struct {
	id common.Hash

	height *big.Int
	round  *big.Int
	state  State

	uHash        common.Hash
	chainAddress common.Address

	tm time.Time

	waitForRoundChange struct {
		height *big.Int
		round  *big.Int
		f      bool
		tm     time.Time
	}

	lock sync.Locker
}

func NewMockTraceState() *traceState {
	return &traceState{
		height: big.NewInt(0),
		round:  big.NewInt(0),
		tm:     time.Now(),
		waitForRoundChange: struct {
			height *big.Int
			round  *big.Int
			f      bool
			tm     time.Time
		}{height: big.NewInt(0), round: big.NewInt(0), f: false, tm: time.Now()},
		lock: new(sync.RWMutex),
	}
}

func (ts *traceState) Update(toState State, toHeight *big.Int, toRound *big.Int, uHash common.Hash) {
	tr := tracetime.New()
	tr.SetMin(0)
	ts.lock.Lock()
	defer ts.lock.Unlock()

	tr.AddArgs("chain", ts.chainAddress, "from_state", ts.state, "to_state",
		toState, "from_height", ts.height, "to_height", toHeight, "from_round", ts.round, "to_round", toRound, "uhash",
		uHash, "elasp", time.Now().Sub(ts.tm))

	ts.state = toState
	ts.height = toHeight
	ts.round = toRound
	ts.uHash = uHash
	ts.tm = time.Now()

	tr.Stop()
}

func (ts *traceState) UpdateWaitForRoundChange(height, round *big.Int, f bool) {
	tr := tracetime.New()
	tr.SetMin(0)
	ts.lock.Lock()
	defer ts.lock.Unlock()

	if ts.height.Cmp(height) != 0 || ts.round.Cmp(round) >= 0 {
		return
	}

	if ts.waitForRoundChange.f == false || f == true {
		ts.waitForRoundChange.f = f
		ts.waitForRoundChange.height = height
		ts.waitForRoundChange.round = round
		ts.waitForRoundChange.tm = time.Now()
		return
	}

	tr.AddArgs("chain", ts.chainAddress, "from_height", ts.waitForRoundChange.height,
		"to_height", height, "from_round", ts.waitForRoundChange.round, "elasp", time.Now().Sub(ts.waitForRoundChange.tm))

	ts.waitForRoundChange.f = f
	ts.waitForRoundChange.height = height
	ts.waitForRoundChange.round = round
	ts.waitForRoundChange.tm = time.Now()
	tr.Stop()
}
