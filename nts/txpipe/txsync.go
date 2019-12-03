package txpipe

import (
	"context"
	"gitee.com/nerthus/nerthus/log"
	"math/rand"
	"time"

	"gitee.com/nerthus/nerthus/core/config"

	"gitee.com/nerthus/nerthus/nts/pman"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/p2p/discover"
)

const (
	// This is the target size for the packs of transactions sent by txsyncLoop.
	// A pack can get larger than this if a single transactions exceeds this size.
	txsyncPackSize = 100 * 1024

	// txChanSize is the size of channel listening to NewTxsEvent.
	// The number is referenced from the size of tx pool.
	txChanSize = 40000

	// 从暂存中每批次提取数目
	defTxCachePopLimit = 50
	// 待广播交易暂存分片数
	defTxPoshCacheSharedSize = 5

	// 定时检查遗漏间隔
	intervalMissingCheck = 5 * time.Minute
	// 每次发送遗漏交易上限
	maxSendMissingTx = 1000
)

type TxPool interface {
	// AddRemotes 将给定的交易tian
	AddRemotes([]*types.Transaction) error
	AddRemotesTxpipe(txs []*types.Transaction) error
	IsFull() bool

	// Pending should return pending transactions.
	// The slice should be modifiable by the caller.
	Pending() (types.Transactions, error)

	// SubscribeTxPreEvent should return an event subscription of
	// TxPreEvent and send events to the given channel.
	SubscribeTxPreEvent(chan<- types.TxPreEvent) event.Subscription

	Get(thash common.Hash) *types.Transaction
}

type txsync struct {
	p   *pman.Peer
	txs types.Transactions
}

type TxSync struct {
	txsyncCh     chan *txsync
	term         chan struct{}
	txpool       TxPool
	broadcastTxs func(ctx context.Context, txs types.Transactions)
	syncSendTxs  func(txs types.Transactions) error
}

func NewTxSync(txpool TxPool, broadcastTxs func(ctx context.Context, txs types.Transactions), syncTxs func(txs types.Transactions) error) *TxSync {
	return &TxSync{
		term:         make(chan struct{}),
		txpool:       txpool,
		txsyncCh:     make(chan *txsync),
		broadcastTxs: broadcastTxs,
		syncSendTxs:  syncTxs,
	}
}

func (t *TxSync) Start() {
	go t.broadcastLoop()
	go t.txsyncLoop()
	go t.txCheckMissingLoop()
}

func (t *TxSync) Stop() {
	close(t.term)
}

func txMsgPackSize() common.StorageSize {
	return common.StorageSize(config.MustInt("runtime.peer_broadcast_tx_pack_size", txsyncPackSize))
}

// txBroadcastLoop 广播交易
func (t *TxSync) broadcastLoop() {
	txCh := make(chan types.TxPreEvent, txChanSize)
	txSub := t.txpool.SubscribeTxPreEvent(txCh)
	defer func() {
		for {
			if len(txCh) == 0 {
				break
			}
			<-txCh //清空
		}
	}()
	defer txSub.Unsubscribe()

	for {
		select {
		case <-txSub.Err():
			return
		case <-t.term:
			return
		case event, ok := <-txCh:
			if !ok { //closed
				return
			}
			t.broadcastTxs(context.Background(), types.Transactions{event.Tx})
		}
	}
}

// syncTransactions starts sending all currently pending transactions to the given peer.
func (t *TxSync) Sync(p *pman.Peer, min time.Time) {
	var txs types.Transactions
	pending, err := t.txpool.Pending()
	if err != nil {
		return
	}
	var minT uint64
	if !min.IsZero() {
		minT = uint64(min.UnixNano())
	}

	for _, batch := range pending {
		if batch.Time() < minT {
			continue
		}
		if batch.Expired(time.Now()) { //忽略过期交易
			continue
		}
		txs = append(txs, batch)
	}
	if len(txs) == 0 {
		return
	}
	select {
	case t.txsyncCh <- &txsync{p, txs}:
	case <-t.term:
	}
}

// txsyncLoop takes care of the initial transaction sync for each new
// connection. When a new peer appears, we relay all currently pending
// transactions. In order to minimise egress bandwidth usage, we send
// the transactions in small packs to one peer at a time.
func (t *TxSync) txsyncLoop() {
	var (
		pending = make(map[discover.NodeID]*txsync)
		sending = false               // whether a send is active
		pack    = new(txsync)         // the pack that is being sent
		done    = make(chan error, 1) // result of the send
	)

	sizeLimit := txMsgPackSize()

	// 开始发送一打未处理的交易集
	// send starts a sending a pack of transactions from the sync.
	send := func(s *txsync) {
		// Fill pack with transactions up to the target size.
		// 分包发送，防止超过包大小
		size := common.StorageSize(0)
		pack.p = s.p
		pack.txs = pack.txs[:0]
		for i := 0; i < len(s.txs) && size < sizeLimit; i++ {
			pack.txs = append(pack.txs, s.txs[i])
			size += s.txs[i].Size()
		}
		// Remove the transactions that will be sent.
		s.txs = s.txs[:copy(s.txs, s.txs[len(pack.txs):])]
		if len(s.txs) == 0 {
			delete(pending, s.p.ID())
		}
		sending = true

		go func() {
			done <- pack.p.SendTransactions(pack.txs)
		}()
	}

	// pick chooses the next pending sync.
	pick := func() *txsync {
		if len(pending) == 0 {
			return nil
		}
		n := rand.Intn(len(pending)) + 1
		for _, s := range pending {
			if n--; n == 0 {
				return s
			}
		}
		return nil
	}

	for {
		select {
		case s := <-t.txsyncCh:
			pending[s.p.ID()] = s
			if !sending {
				send(s)
			}
		case err := <-done:
			sending = false
			// Stop tracking peers that cause send failures.
			if err != nil {
				//pack.p.Log().Debug("Transaction send failed", "err", err)
				delete(pending, pack.p.ID())
			}
			// Schedule the next send.
			if s := pick(); s != nil {
				send(s)
			}
		case <-t.term:
			return
		}
	}
}

func (t *TxSync) txCheckMissingLoop() {
	tk := time.NewTicker(intervalMissingCheck)
	defer tk.Stop()
	for {
		select {
		case <-t.term:
			return
		case <-tk.C:
			pending, err := t.txpool.Pending()
			if err != nil {
				log.Debug("get tx pending err", "err", err)
				continue
			}
			if len(pending) == 0 {
				return
			}
			deadline := time.Now().UnixNano() - int64(intervalMissingCheck)
			seq := maxSendMissingTx
			var needSend types.Transactions
			for i := range pending {
				if seq <= 0 {
					break
				}
				if int64(pending[i].Time()) < deadline {
					needSend = append(needSend, pending[i])
					seq--
				}
			}
			if len(needSend) == 0 {
				continue
			}
			err = t.syncSendTxs(needSend)
			log.Debug("re broad missing tx", "len", len(needSend), "deadline", deadline, "err", err)
		}

	}
}
