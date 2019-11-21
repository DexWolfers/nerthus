package monitor

import (
	"sync"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/testutil"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"
)

func TestMonitoring(t *testing.T) {
	testutil.SetupTestLogging()
	ch := make(chan struct{}, 1)
	addr := common.StringToAddress("1111")
	chain := &TestChainReader{}
	monitor := NewMonitorEngine(addr, chain, func(addr common.Address) {
		log.Debug("stop chain", "addr", addr)
		ch <- struct{}{}

	}, func(addr common.Address) {
		log.Debug("recover chain", "addr", addr)
	})
	monitor.Start()

	generateUnit := func() {
		for {
			time.Sleep(10 * time.Second)
			chain.seq++
			chain.tm = time.Now()
			log.Info("Imported new unit", "number", chain.seq, "utime", chain.tm)
			if chain.seq%10 == 0 {
				break
			} else if chain.seq%10 == 1 && chain.seq > 1 {
				monitor.RefreshStatus()
			}
		}
	}
	for {
		// 开始出单元
		go generateUnit()
		<-ch
	}
}

type TestChainReader struct {
	seq int
	tm  time.Time
	sync.Mutex
}

func (t TestChainReader) Config() *params.ChainConfig {
	return nil
}
func (t TestChainReader) GetChainTailHead(chain common.Address) *types.Header {
	t.Lock()
	defer t.Unlock()
	head := &types.Header{
		Number:    uint64(t.seq),
		Timestamp: uint64(t.tm.UnixNano()),
	}
	return head
}
func (t TestChainReader) GetHeaderByNumber(mcaddr common.Address, number uint64) *types.Header {
	return nil
}
func (t TestChainReader) StateAt(chain common.Address, root common.Hash) (*state.StateDB, error) {
	return nil, nil
}
func (t TestChainReader) GetChainTailState(mcaddr common.Address) (*state.StateDB, error) {
	return nil, nil
}
func (t TestChainReader) PostChainEvent(events ...interface{}) {

}
func (t TestChainReader) GetUnitNumber(hash common.Hash) (common.Address, uint64) {
	return common.Address{}, 0
}
func (t TestChainReader) WakChildren(parentHash common.Hash, cb func(children common.Hash) bool) {

}
func (t TestChainReader) GetUnitState(unitHash common.Hash) (*state.StateDB, error) {
	return nil, nil
}
