package core

import (
	"context"
	"math/big"
	"reflect"
	"sync"
	"testing"

	"gopkg.in/karalabe/cookiejar.v2/collections/prque"

	"gitee.com/nerthus/nerthus/log"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/consensus/protocol"
	"github.com/stretchr/testify/require"
)

func TestCheckMessage(t *testing.T) {
	c := &core{
		state: StateAcceptRequest,
		current: newRoundState(&bft.View{
			Sequence: big.NewInt(1),
			Round:    big.NewInt(0),
		}, newTestValidatorSet(4), common.EmptyAddress, nil, nil, nil, nil),
	}

	// invalid view format
	{
		err := c.checkMessage(context.TODO(), protocol.MsgPrepare, nil)
		require.Equal(t, err, errInvalidMessage)
	}

	testStates := []State{StateAcceptRequest, StatePreprepared, StatePrepared, StateCommitted}
	testCode := []protocol.ConsensusType{protocol.MsgPrepare, protocol.MsgPrepare, protocol.MsgCommit, protocol.MsgRoundChange}

	// future sequence
	v := &bft.View{
		Sequence: big.NewInt(2),
		Round:    big.NewInt(0),
	}
	for i := 0; i < len(testStates); i++ {
		c.state = testStates[i]
		for j := 0; j < len(testCode); j++ {
			err := c.checkMessage(context.TODO(), testCode[j], v)
			require.Equal(t, err, errFutureMessage)
		}
	}

	// future round
	v = &bft.View{
		Sequence: big.NewInt(1),
		Round:    big.NewInt(1),
	}
	for i := 0; i < len(testStates); i++ {
		c.state = testStates[i]
		for j := 0; j < len(testCode); j++ {
			err := c.checkMessage(context.TODO(), testCode[j], v)
			if testCode[j] == protocol.MsgRoundChange {
				require.Nil(t, err)
			} else {
				require.Equal(t, err, errFutureMessage)
			}
		}
	}

	// current view but waiting for round change
	{
		v = &bft.View{
			Sequence: big.NewInt(1),
			Round:    big.NewInt(0),
		}
		c.waitingForRoundChange = true
		for i := 0; i < len(testStates); i++ {
			c.state = testStates[i]
			for j := 0; j < len(testCode); j++ {
				err := c.checkMessage(context.TODO(), testCode[j], v)
				if testCode[j] == protocol.MsgRoundChange {
					require.Nil(t, err)
				} else if err != errFutureMessage {
					require.Equal(t, err, errFutureMessage)
				}
			}
		}
	}

	// current view, state = StatePrepared
	{
		c.state = StatePrepared
		for i := 0; i < len(testCode); i++ {
			err := c.checkMessage(context.TODO(), testCode[i], v)
			if testCode[i] == protocol.MsgRoundChange {
				require.Nil(t, err)
			} else {
				require.Nil(t, err)
			}
		}
	}

	// current view, state = StateCommitted
	{
		c.state = StateCommitted
		for i := 0; i < len(testCode); i++ {
			err := c.checkMessage(context.TODO(), testCode[i], v)
			if testCode[i] == protocol.MsgRoundChange {
				require.Nil(t, err)
			} else if err != nil {
				require.Nil(t, err)
			}
		}
	}
}

func TestStoreBacklog(t *testing.T) {
	c := &core{
		logger:     log.New("backend", "test", "id", 0),
		backlogs:   make(map[bft.Validator]*prque.Prque),
		backlogsMu: new(sync.Mutex),
	}
	v := &bft.View{
		Round:    big.NewInt(10),
		Sequence: big.NewInt(10),
	}

	p := bft.NewValidator(common.StringToAddress("1234567890"))
	// push preprepare msg
	{
		preprepare := &bft.Preprepare{
			View:     v,
			Proposal: makeUnit(p.Address(), common.Hash{}, common.Hash{}, 1),
		}
		prepreparePayload, _ := Encode(preprepare)
		m := &protocol.Message{
			Code: protocol.MsgPreprepare,
			Msg:  prepreparePayload,
		}
		c.storeBacklog(m, p)

		msg := c.backlogs[p].PopItem()
		if !reflect.DeepEqual(msg, m) {
			t.Errorf("message mismatch: have %v, want %v", msg, m)
		}
	}

	// push prepare msg
	subject := &bft.Subject{
		View:   v,
		Digest: common.StringToHash("1234567890"),
	}
	subjectPayload, _ := Encode(subject)
	{
		m := &protocol.Message{
			Code: protocol.MsgPrepare,
			Msg:  subjectPayload,
		}
		c.storeBacklog(m, p)
		msg := c.backlogs[p].PopItem()
		if !reflect.DeepEqual(msg, m) {
			t.Errorf("message mismatch: have %v, want %v", msg, m)
		}
	}

	// push commit msg
	{
		m := &protocol.Message{
			Code: protocol.MsgCommit,
			Msg:  subjectPayload,
		}
		c.storeBacklog(m, p)
		msg := c.backlogs[p].PopItem()
		if !reflect.DeepEqual(msg, m) {
			t.Errorf("message mismatch: have %v, want %v", msg, m)
		}
	}

	// push roundChange msg
	{
		m := &protocol.Message{
			Code: protocol.MsgRoundChange,
			Msg:  subjectPayload,
		}
		c.storeBacklog(m, p)
		msg := c.backlogs[p].PopItem()
		if !reflect.DeepEqual(msg, m) {
			t.Errorf("message mismatch: have %v, want %v", msg, m)
		}
	}
}

// TODO
func TestProcessFutureBacklog(t *testing.T) {

}
