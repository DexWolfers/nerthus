package statistics

import (
	"sync"
	"sync/atomic"
	"time"

	"gitee.com/nerthus/nerthus/accounts"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/ntsdb"
)

const defToHistoryInter = time.Hour * 24 //每天执行一次

type Statistic struct {
	reader  ChainReader
	db      ntsdb.RD
	amg     *accounts.Manager
	working sync.WaitGroup
	quit    chan struct{}

	localAccounts map[common.Address]struct{}
	accountsLock  sync.RWMutex

	jobc chan interface{}
}

func New(reader ChainReader, db ntsdb.RD, amg *accounts.Manager) (*Statistic, error) {
	if err := checkTable(db); err != nil {
		return nil, err
	}
	return &Statistic{
		reader:        reader,
		db:            db,
		amg:           amg,
		quit:          make(chan struct{}),
		jobc:          make(chan interface{}, 10000*50),
		localAccounts: make(map[common.Address]struct{}, 5),
	}, nil
}

func (ts *Statistic) Start() error {
	// 关闭自动的修复，关闭的最大原因是自动修复时需要遍历所有链，如果有 30W 链则需要遍历所有。
	// 因此关闭，关闭的影响面也非常小。因为程序在正常运行时不会出现丢失数据情况，只有刚好在处理时程序被 Kill 才会丢失数据
	// 即使丢失数据，也无关键影响。
	// TOOD: 可以的优化手段是减少遍历量
	// 做法：在接收到新单元后立即记录单元位置信息，交易处理结束时清理标记。重启时根据标记进行恢复
	//ts.autoFix()
	ts.initAccounts()
	go ts.listenAccounts()
	go ts.listenChain()
	go ts.work()

	return nil
}

func (ts *Statistic) Stop() {
	close(ts.quit)
	ts.working.Wait()
}

func (ts *Statistic) work() {
	ts.working.Add(1)
	defer ts.working.Done()

	var count int32
	handle := func(d interface{}) {
		switch v := d.(type) {
		case *types.Transaction:
			if err := ts.saveNewTx(v); err != nil {
				log.Error("failed save tx info", "tx", v.Hash(), "from", v.SenderAddr(), "to", v.To(), "err", err, "len", count)
			}
			atomic.AddInt32(&count, 1)
		case types.ChainEvent:
			if err := ts.afterNewUnit(v); err != nil {
				log.Error("failed save unit info", "uhash", v.UnitHash)
			}
		}
	}

	ts.toHistory()
	toHistoryTk := time.NewTicker(defToHistoryInter)
	defer toHistoryTk.Stop()

	for {
		select {
		case <-ts.quit:
			//退出时，需要继续将其他落地完成
			for {
				select {
				case d := <-ts.jobc:
					handle(d)
				default:
					return
				}
			}
		case <-toHistoryTk.C:
			ts.toHistory()
		case d := <-ts.jobc:
			handle(d)
		}
	}
}

// 将当期交易数据迁移到历史表中
func (ts *Statistic) toHistory() {
	err := toHistory(ts.db)
	if err != nil {
		log.Warn("failed to remove data to history", "err", err)
	}
}

func (ts *Statistic) listenChain() {
	ts.working.Add(1)
	defer ts.working.Done()

	if ts.reader == nil {
		log.Warn("reader is nil,can not listen chain")
		return
	}

	c := make(chan types.ChainEvent, 10000)
	sub := ts.reader.SubscribeChainEvent(c)
	defer sub.Unsubscribe()
	for {
		select {
		case <-ts.quit:
			return
		case ev := <-c:
			ts.jobc <- ev
		}
	}
}

// 从 账户下获取所有钱包中存在的账户
func (ts *Statistic) initAccounts() {
	if ts.amg == nil {
		return
	}
	ts.accountsLock.Lock()
	for _, w := range ts.amg.Wallets() {
		for _, acc := range w.Accounts() {
			ts.localAccounts[acc.Address] = struct{}{}
		}
	}
	ts.accountsLock.Unlock()
}

// 判断某账户是否是本地账户
func (ts *Statistic) isLocalAcct(addr common.Address) bool {
	if addr.IsContract() {
		return false
	}
	ts.accountsLock.RLock()
	_, ok := ts.localAccounts[addr]
	ts.accountsLock.RUnlock()
	return ok
}

// 监听钱包，因为此部分数据只需要为本地服务，非本地账户相关的交易将不记录。
func (ts *Statistic) listenAccounts() {
	sink := make(chan accounts.WalletEvent, 2)
	sub := ts.amg.Subscribe(sink)
	defer sub.Unsubscribe()
	for {
		select {
		case <-sub.Err():
			return
		case <-ts.quit:
			return
		case ev := <-sink:
			switch ev.Kind {
			case accounts.WalletArrived:
				ts.accountsLock.Lock()
				for _, acc := range ev.Wallet.Accounts() {
					ts.localAccounts[acc.Address] = struct{}{}
				}
				ts.accountsLock.Unlock()
			case accounts.WalletDropped:
				ts.accountsLock.Lock()
				for _, acc := range ev.Wallet.Accounts() {
					delete(ts.localAccounts, acc.Address)
				}
				ts.accountsLock.Unlock()
			}
		}
	}
}
