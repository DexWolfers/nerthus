package core

import (
	"fmt"
	"math/big"
	"strings"
	"sync"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/consensus/protocol"
)

func newMessageSet(valSet bft.ValidatorSet) *messageSet {
	return &messageSet{
		view: &bft.View{
			Round:    new(big.Int),
			Sequence: new(big.Int),
		},
		rwLock:   new(sync.RWMutex),
		messages: make(map[common.Address]*protocol.Message),
		valSet:   valSet,
	}
}

// 某个高度的某个轮次的共识消息
type messageSet struct {
	view     *bft.View        // 视图
	valSet   bft.ValidatorSet // 见证人
	rwLock   *sync.RWMutex
	messages map[common.Address]*protocol.Message
}

// 视图
func (ms *messageSet) View() *bft.View {
	return ms.view
}

func (ms *messageSet) Add(msg *protocol.Message) (err error) {
	ms.rwLock.Lock()
	if err = ms.verify(msg); err != nil {
		ms.rwLock.Unlock()
		return err
	}
	err = ms.addVerifiedMessage(msg)
	ms.rwLock.Unlock()
	return
}

// NOTE 返回的是消息的指正对象，不可更改消息的内容
func (ms *messageSet) Values() (result []*protocol.Message) {
	ms.rwLock.RLock()
	result = make([]*protocol.Message, 0, len(ms.messages))
	for _, v := range ms.messages {
		result = append(result, v)
	}
	ms.rwLock.RUnlock()
	return
}

func (ms *messageSet) Size() (s int) {
	ms.rwLock.RLock()
	s = len(ms.messages)
	ms.rwLock.RUnlock()
	return
}

func (ms *messageSet) Get(addr common.Address) (msg *protocol.Message) {
	ms.rwLock.RLock()
	msg = ms.messages[addr]
	ms.rwLock.RUnlock()
	return
}

func (ms *messageSet) Messages() map[common.Address]*protocol.Message {
	ms.rwLock.Lock()
	defer ms.rwLock.Unlock()
	return ms.messages
}

func (ms *messageSet) Copy() messageSet {
	ms.rwLock.Lock()
	defer ms.rwLock.Unlock()

	newSet := messageSet{rwLock: new(sync.RWMutex), messages: make(map[common.Address]*protocol.Message), valSet: ms.valSet}
	newSet.view = &bft.View{
		Round:    big.NewInt(0).Set(ms.view.Round),
		Sequence: big.NewInt(0).Set(ms.view.Sequence),
	}
	for addr, msg := range ms.messages {
		newSet.messages[addr] = msg.Copy()
	}
	return newSet
}

/// internal impl

func (ms *messageSet) verify(msg *protocol.Message) error {
	if _, v := ms.valSet.GetByAddress(msg.Address); v == nil {
		return bft.ErrUnauthorizedAddress
	} else if _, ok := ms.messages[msg.Address]; ok {
		return bft.ErrExistVoteMsg
	}

	return nil
}

func (ms *messageSet) addVerifiedMessage(msg *protocol.Message) error {
	ms.messages[msg.Address] = msg
	return nil
}

func (ms *messageSet) String() string {
	ms.rwLock.Lock()
	defer ms.rwLock.Unlock()
	addresses := make([]string, 0, len(ms.messages))
	for _, v := range ms.messages {
		addresses = append(addresses, v.Address.String()[:10])
	}
	return fmt.Sprintf("[%v]", strings.Join(addresses, ", "))
}
