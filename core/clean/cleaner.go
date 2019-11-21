package clean

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/rawdb"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"
	"math/big"
	"time"
)

func NewCleaner(db ntsdb.Database, dagChain *core.DagChain) *StateDBCleaner {
	cfg := NewConfig()
	// 开关
	if !cfg.AutoClearOn {
		return nil
	}
	return &StateDBCleaner{
		period:      cfg.Period,
		interval:    cfg.IntervalPeriod,
		sysInterval: cfg.SysIntervalPeriod,
		db:          db,
		dag:         dagChain,
	}
}

type StateDBCleaner struct {
	period      uint64
	interval    uint64
	sysInterval uint64
	db          ntsdb.Database
	dag         *core.DagChain
	taskCh      chan task
	quit        chan struct{}
}

func (t *StateDBCleaner) Start() {
	t.quit = make(chan struct{})
	t.taskCh = make(chan task, 1024)
	go t.listenUnit()
	go t.loopTask()
	log.Info("state cleaner started", "period", t.period, "user_interval", t.interval, "sys_interval", t.sysInterval)
}
func (t *StateDBCleaner) Stop() {
	close(t.quit)
	log.Info("state cleaner stop")
}

func (t *StateDBCleaner) listenUnit() {
	// 订阅新单元
	newUnitCh := make(chan types.ChainEvent, 1024)
	sub := t.dag.SubscribeChainEvent(newUnitCh)
	defer sub.Unsubscribe()
	for {
		select {
		case <-t.quit:
			return
		case ev := <-newUnitCh:
			n := ev.Unit.Number()
			if n%t.period != 0 {
				continue
			}
			var interval uint64
			if ev.Unit.MC() == params.SCAccount {
				interval = t.sysInterval
			} else {
				interval = t.interval
			}
			// 必须间隔一定的周期
			if n <= interval*t.period {
				continue
			}
			task := task{
				chain: ev.Unit.MC(),
				from:  t.getLastNumber(ev.Unit.MC()),
			}

			if p := n - (interval * t.period); p <= task.from {
				continue
			} else {
				task.to = p
				select {
				case t.taskCh <- task:
				default:
					go func() {
						t.taskCh <- task
					}()
				}
			}
		}
	}
}

func (t *StateDBCleaner) loopTask() {
	for {
		select {
		case <-t.quit:
			return
		case ev := <-t.taskCh:
			ev.from = t.getLastNumber(ev.chain)
			if ev.from >= ev.to {
				continue
			}
			log.Debug("begin to clean", "from", ev.from, "to", ev.to, "chain", ev.chain)
			now := time.Now()
			// 启动一次清理
			stateDB, err := t.dag.GetStateByNumber(ev.chain, ev.to)
			if err != nil {
				log.Error("get stateDB err when process clean")
				continue
			}
			clner := state.NewCleaner(t.db, ev.chain, ev.from, ev.to, stateDB,
				func(chain common.Address, number uint64) common.Hash {
					h := t.dag.GetHeaderByNumber(chain, number)
					if h == nil {
						log.Error("get header nil", "chain", chain, "number", number)
						return common.Hash{}
					}
					return h.StateRoot
				})
			totalRemoved := clner.StartClear()
			t.setLastNumber(ev.chain, ev.to)

			log.Info("clean chain state", "removed", totalRemoved, "from", ev.from, "to", ev.to, "chain", ev.chain, "cost", time.Now().Sub(now))
		}
	}
}

func (t *StateDBCleaner) getLastNumber(chain common.Address) uint64 {
	k := rawdb.CleanerKey(chain)
	data, err := t.db.Get(k)
	if err != nil {
		log.Debug("get clean key err", "err", err)
		return 1
	}
	return big.NewInt(0).SetBytes(data).Uint64()
}

func (t *StateDBCleaner) setLastNumber(chain common.Address, number uint64) {
	k := rawdb.CleanerKey(chain)
	err := t.db.Put(k, big.NewInt(int64(number)).Bytes())
	if err != nil {
		log.Crit("put key err", "err", err)
	}
}
