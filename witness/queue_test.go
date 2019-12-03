package witness

import (
	"math/big"
	"sync"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/core/types"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/sort/sortslice"
)

func TestChainJobQueue_CallWhenStopped(t *testing.T) {

	m := &ChainJobQueue{
		msgList: sortslice.Get(),
		ser:     nil,
		chain:   common.StringToAddress("chain1"),
	}

	var stoped bool

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for {
			if stoped {
				break
			}
			m.Peek()
			m.Pop()
			m.Len()
		}
	}()
	go func() {
		defer wg.Done()
		tx := types.NewTransaction(m.chain,
			common.StringToAddress("to"), big.NewInt(1), 1, uint64(1), 100, nil)

		for {
			if stoped {
				break
			}
			m.Push(newTask(types.ActionContractFreeze, m.chain, tx))
		}
	}()

	time.Sleep(time.Second * 5)

	m.Stop()

	time.Sleep(time.Second * 5)
	stoped = true
	wg.Wait()
}
