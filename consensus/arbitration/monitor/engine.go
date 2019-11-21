package monitor

import (
	"sync"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/consensus/arbitration"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"
)

func NewMonitorEngine(chainAddress common.Address, chain arbitration.ChainReader, stopChain func(common.Address), recoverChain func(common.Address)) *MonitorEngine {
	conf := NewDefaultConfig()
	eng := &MonitorEngine{
		chainAddress:   chainAddress,
		chain:          chain,
		status:         StatusRunning,
		logger:         log.Root().New("module", "chain monitor", "chain", chainAddress),
		conf:           conf,
		ch:             make(chan struct{}),
		mutx:           sync.Mutex{},
		lastDeadNumber: 0,
	}
	// 注入事件函数
	eng.statusChangeFunc = func(old ChainStatus, new ChainStatus) {
		if old == new {
			return
		}
		eng.logger.Info("chain running status changed", "old", old, "new", new)
		switch new {
		case StatusRunning: // 恢复正常，需要执行恢复操作
			if old == StatusStop {
				recoverChain(eng.chainAddress)
			}
		case StatusError:
		case StatusStop: // 停止相关链
			stopChain(eng.chainAddress)
		}
	}
	return eng
}

type MonitorEngine struct {
	chainAddress      common.Address
	chain             arbitration.ChainReader
	status            ChainStatus
	statusChangeFunc  func(old ChainStatus, new ChainStatus)
	logger            log.Logger
	conf              Config
	ch                chan struct{}
	mutx              sync.Mutex
	lastDeadNumber    uint64
	lastRecoverNumber uint64
}

func (t *MonitorEngine) Start() {
	go t.monitoring()
}
func (t *MonitorEngine) Stop() {
	close(t.ch)
}
func (t *MonitorEngine) Status() ChainStatus {
	t.mutx.Lock()
	defer t.mutx.Unlock()
	return t.status
}
func (t *MonitorEngine) RefreshStatus() {
	if t.Status() > StatusRunning {
		t.ch <- struct{}{}
	}
}

func (t *MonitorEngine) monitoring() {
	// 不能立即启动，需要对比程序启动时间
	startTime := params.NodeStartTime.Add(time.Duration(t.conf.ErrorInterval) * time.Second)
	if time.Now().Before(startTime) {
		t.logger.Info("chain monitoring wait to start", "node start", params.NodeStartTime, "need start", startTime)
		<-time.After(startTime.Sub(time.Now()))
	}
	t.logger.Debug("chain monitoring start")
	defer func() {
		t.logger.Debug("chain monitoring stop")
	}()
	for {
		select {
		case _, ok := <-t.ch:
			if !ok {
				return
			}
		case <-time.After(time.Duration(t.conf.CheckInterval) * time.Second):

		}
		// 检查链是否在指定内时间没有出单元
		last := t.chain.GetChainTailHead(t.chainAddress)
		lastUtime := time.Unix(0, int64(last.Timestamp))
		// 跳过创世
		if last.Number == 0 {
			continue
		}
		if t.lastRecoverNumber < last.Number {
			if last.Proposer == params.PublicPrivateKeyAddr || (t.Status() > StatusRunning && t.lastDeadNumber > 0 && last.Number > t.lastDeadNumber+1) {
				// 收到特殊单元 或者收到链的2个单元 >恢复
				// set running normal
				t.setStatus(StatusRunning)
				t.lastRecoverNumber = last.Number
				t.logger.Debug("chain monitoring status recover", "u number", last.Number, "hash", last.Hash(), "u time", lastUtime)
				continue

			}
		}
		interval := time.Now().Sub(lastUtime)
		t.logger.Debug("chain monitoring check", "lastNumber", last.Number, "hash", last.Hash(), "LastTime", lastUtime, "interval", interval, "status", t.Status())
		switch {
		case interval >= time.Duration(t.conf.ErrorInterval)*time.Second && interval < time.Duration(t.conf.StopInterval)*time.Second:
			// set error
			if t.Status() == StatusRunning {
				t.setStatus(StatusError)
			}
		case interval >= time.Duration(t.conf.StopInterval)*time.Second:
			// set stop
			t.lastDeadNumber = last.Number
			t.setStatus(StatusStop)

		}
	}
}

func (t *MonitorEngine) setStatus(s ChainStatus) {
	t.mutx.Lock()
	if t.status == s {
		t.mutx.Unlock()
		return
	}
	old := t.status
	t.status = s
	t.mutx.Unlock()
	if t.statusChangeFunc != nil {
		t.statusChangeFunc(old, s)
	}
}
func (t *MonitorEngine) SetStopping() {
	last := t.chain.GetChainTailHead(t.chainAddress)
	t.lastDeadNumber = last.Number
	t.setStatus(StatusStop)
}
