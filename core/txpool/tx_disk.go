package txpool

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/rlp"

	"github.com/syndtr/goleveldb/leveldb"
)

var ErrNotFoundTxDisk = errors.New("not found the transaction in disk store")

const TxDiskStoreInterval = 30
const TxDiskStoreTimeout = 30

type TxDiskStoreDec = func(tx *types.Transaction) []byte
type TxDiskStoreEnc = func([]byte) *types.Transaction

type TxDiskStore interface {
	Get(txHash common.Hash) (*types.Transaction, error)
	GetAll() ([]*types.Transaction, error)
	Put(local bool, tx ...*types.Transaction) error
	Close() error
}

type TxItemQueue []*TxItem

type TxItem struct {
	timestamp uint64
	txHash    common.Hash
	local     bool
}

func (txItem TxItemQueue) Len() int {
	return len(txItem)
}

func (txItem TxItemQueue) Swap(i, j int) {
	txItem[i], txItem[j] = txItem[j], txItem[i]
}

func (txItem TxItemQueue) Less(i, j int) bool {
	return txItem[i].timestamp > txItem[j].timestamp
}

func (txItem *TxItemQueue) Pop() interface{} {
	old := *txItem
	n := len(old)
	x := old[n-1]
	*txItem = old[0 : n-1]
	return x
}

func (txItem *TxItemQueue) Push(item interface{}) {
	*txItem = append(*txItem, item.(*TxItem))
}

func NewTxDiskStore(interval, timeout time.Duration, db ntsdb.Database, addTx func(tx *types.Transaction) error) (TxDiskStore, error) {
	if interval == 0 {
		interval = TxDiskStoreInterval
	}
	if timeout == 0 {
		timeout = TxDiskStoreTimeout
	}
	ctx, cancelFn := context.WithCancel(context.Background())
	diskStore := leveldbTxDiskStore{
		db:       db,
		q:        &TxItemQueue{},
		txs:      make(map[common.Hash]struct{}),
		l:        new(sync.RWMutex),
		ctx:      ctx,
		cancelFn: cancelFn,
		interval: interval,
		timeout:  timeout,
		addTx:    addTx,
	}
	// must init queue
	heap.Init(diskStore.q)
	go diskStore.start()
	return &diskStore, nil
}

type leveldbTxDiskStore struct {
	db       ntsdb.Database
	q        *TxItemQueue
	txs      map[common.Hash]struct{}
	l        *sync.RWMutex
	ctx      context.Context
	cancelFn context.CancelFunc
	interval time.Duration
	timeout  time.Duration

	addTx func(tx *types.Transaction) error
}

func (txDiskStore *leveldbTxDiskStore) Get(txHash common.Hash) (*types.Transaction, error) {
	txDiskStore.l.RLock()
	defer txDiskStore.l.RUnlock()
	if _, ok := txDiskStore.txs[txHash]; !ok {
		return nil, ErrNotFoundTxDisk
	}
	tx := txDiskStore.getTx(txHash)
	if tx == nil {
		return nil, ErrNotFoundTxDisk
	}
	return tx, nil
}

func (txDiskStore *leveldbTxDiskStore) GetAll() (txs []*types.Transaction, err error) {
	txDiskStore.l.RLock()
	defer txDiskStore.l.RUnlock()
	txs = make([]*types.Transaction, 0, len(txDiskStore.txs))
	for txHash := range txDiskStore.txs {
		if tx := txDiskStore.getTx(txHash); tx != nil {
			txs = append(txs, tx)
		}
	}
	return
}

func (txDiskStore *leveldbTxDiskStore) Put(local bool, txs ...*types.Transaction) (err error) {
	return txDiskStore.put(local, true, txs...)
}

func (txDiskStore *leveldbTxDiskStore) put(local, toDisk bool, txs ...*types.Transaction) (err error) {
	txDiskStore.l.Lock()
	defer txDiskStore.l.Unlock()

	var dbBatch ntsdb.Batch
	if toDisk {
		dbBatch = txDiskStore.db.NewBatch()
	}
	var now = uint64(time.Now().UnixNano())
	var timeout = uint64(txDiskStore.timeout)
	for _, tx := range txs {
		if _, ok := txDiskStore.txs[tx.Hash()]; ok { //已存在
			continue
		}
		if !local { //如果是非本地交易，则需要过期删除
			if now > timeout+tx.Time() || tx.Expired(time.Now()) { //超时
				continue
			}
		}
		txDiskStore.txs[tx.Hash()] = struct{}{}
		txItem := &TxItem{
			timestamp: tx.Time(),
			txHash:    tx.Hash(),
			local:     local,
		}
		heap.Push(txDiskStore.q, txItem)
		if dbBatch != nil {
			txDiskStore.write(dbBatch, tx)
		}
	}
	if dbBatch != nil {
		return dbBatch.Write()
	}
	return nil
}

func (txDiskStore *leveldbTxDiskStore) Close() error {
	if txDiskStore.ctx.Err() == nil {
		txDiskStore.cancelFn()
		// wait it exit
		<-txDiskStore.ctx.Done()
	}
	return nil
}

// Just for test
func (txDiskStore *leveldbTxDiskStore) pop() *types.Transaction {
	txDiskStore.l.Lock()
	defer txDiskStore.l.Unlock()
	if txDiskStore.q.Len() == 0 {
		return nil
	}
	item := heap.Pop(txDiskStore.q).(*TxItem)
	if item == nil {
		return nil
	}
	delete(txDiskStore.txs, item.txHash)
	return txDiskStore.getTx(item.txHash)
}

func (txDiskStore *leveldbTxDiskStore) start() {
	// reload disk tx
	var (
		wg  sync.WaitGroup
		txs = make(chan *types.Transaction, 10)
	)
	go func() {
		for tx := range txs {
			txDiskStore.put(true, false, tx)
			//如果存在则发送给外部
			if txDiskStore.addTx != nil {
				txHash := tx.Hash()
				if _, ok := txDiskStore.txs[txHash]; ok {
					if err := txDiskStore.addTx(tx); err != nil {
						// delete it from collections
						delete(txDiskStore.txs, txHash) //小心并发修改
						// delete it from disk
						txDiskStore.delFromDB(txHash)
						log.Debug("failed load stored transaction to tx pool,delete it", "txHash", txHash, "err", err)
					}
				}
			}
			wg.Done()
		}
	}()

	txDiskStore.db.Iterator(txPrefix, func(key, value []byte) bool {
		wg.Add(1)
		txs <- rlpTxDiskStoreDec(value)
		return true
	})
	wg.Wait()
	close(txs)

	// 暂不需要清理交易，因为保留所有本地交易
	//go txDiskStore.tick(txDiskStore.interval)

}

// TODO Optz
func (txDiskStore *leveldbTxDiskStore) tick(interval time.Duration) {
	clearFn := func() {
		txDiskStore.l.Lock()
		defer txDiskStore.l.Unlock()

		var now = uint64(time.Now().UnixNano())
		var timeout = uint64(interval)
		for {
			if txDiskStore.q.Len() == 0 {
				break
			}
			txItem := heap.Pop(txDiskStore.q).(*TxItem)
			// check timeout
			if now-txItem.timestamp < timeout {
				if _, ok := txDiskStore.txs[txItem.txHash]; ok {
					// put it again
					heap.Push(txDiskStore.q, txItem)
					break
				}
			}
			// delete it from collections
			delete(txDiskStore.txs, txItem.txHash)
			// delete it from disk
			txDiskStore.delFromDB(txItem.txHash)
		}
	}

	var ticker = time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			clearFn()
		case <-txDiskStore.ctx.Done():
			return
		}
	}
}

var txPrefix = []byte("t_")

func txStoreKey(tx common.Hash) []byte {
	return append(txPrefix, tx.Bytes()...)
}
func (s *leveldbTxDiskStore) write(batch ntsdb.Putter, tx *types.Transaction) {
	b := rlpTxDiskStoreEnc(tx)
	batch.Put(txStoreKey(tx.Hash()), b)
}
func (s *leveldbTxDiskStore) getTx(tx common.Hash) *types.Transaction {
	b, err := s.db.Get(txStoreKey(tx))
	if err == leveldb.ErrNotFound {
		return nil
	}
	if err != nil {
		return nil
	}
	return rlpTxDiskStoreDec(b)
}
func (s *leveldbTxDiskStore) delFromDB(tx common.Hash) {
	s.db.Delete(txStoreKey(tx))
}

func rlpTxDiskStoreEnc(tx *types.Transaction) []byte {
	b, err := rlp.EncodeToBytes(tx)
	if err != nil {
		panic(fmt.Sprintf("%T must be encoding by rlp, err:%v", tx, err))
	}
	return b
}

func rlpTxDiskStoreDec(b []byte) (tx *types.Transaction) {
	tx = new(types.Transaction)
	if err := rlp.DecodeBytes(b, tx); err != nil {
		panic(fmt.Sprintf("%T must be decoding by rlp,len(b)=%d err:%v", tx, len(b), err))
	}
	return
}
