package txpool

import (
	"sync"
	"time"

	"gitee.com/nerthus/nerthus/log"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/ntsdb"
)

var dataPrekey = []byte("m_")

type ExecMsg struct {
	Chain  common.Address
	TxHash common.Hash
	Action types.TxAction

	IsNew       bool
	expiredTime uint64 //秒级
}

// 交易待处理池，管理所有交易。
// 此部分数据是为有可能成为见证节点的服务器准备，防止新见证人开启见证时数据不完整情况。
// 因此如果节点不可能成为见证节点则不需要记录，已降低 IO和存储。
// 默认情况下：见证节点必须是 Linux 服务器。则 Window 等系统将不需要记录数据。
// 但有一种例外：如果是个人的开发环境需要启动见证服务时，则需要记录。
// 当前团队开发在 Mac 下进行，为了不改动面过大，TODO: 暂时屏蔽 Window 下的存储。
type TODOPool struct {
	disable bool //是否开启存储
	db      ntsdb.Database

	itemCh chan ExecMsg
	exitwg sync.WaitGroup
	quit   chan struct{}
}

func NewTODOPool(db ntsdb.Database, disable bool) *TODOPool {
	if disable {
		log.Warn("Disabled the tx pool record tx pending details, Must be turned on if this node will be use as a witness node")
		return &TODOPool{
			disable: true,
		}
	}

	return &TODOPool{
		disable: false,
		db:      db,
		itemCh:  make(chan ExecMsg, 1024*5),
		quit:    make(chan struct{}),
	}
}
func (pool *TODOPool) Start() {
	if pool.disable {
		return
	}

	go pool.deleteOld()
	//go pool.push()
	go pool.lookup()
}

func (pool *TODOPool) Stop() {
	if pool.disable {
		return
	}
	close(pool.quit)
	pool.exitwg.Wait()
}

// TODO:需要进一步清理可能永远无法处理的交易，而引起数据膨胀
func (pool *TODOPool) deleteOld() {
	tk := time.NewTicker(time.Hour * 24)
	defer tk.Stop()

	ck := func() {
		minTime := uint64(time.Now().Add(-time.Hour * 24 * 7).Unix())
		var todos int
		pool.Range(func(msg ExecMsg) bool {
			if msg.expiredTime == 0 || msg.expiredTime >= minTime {
				todos++
				return true
			}
			//交易过期则需要删除,但不能删除所有，只需要删除第一个阶段无法完成的部分
			if msg.Action == types.ActionTransferPayment ||
				msg.Action == types.ActionContractFreeze ||
				msg.Action == types.ActionPowTxContractDeal {
				msg.IsNew = false
				select {
				case pool.itemCh <- msg:
				case <-pool.quit:
				}
			}
			todos++
			return true
		})
		log.Debug("need process tx message", "count", todos)
	}
	ck()

	for {
		select {
		case <-pool.quit:
			return
		case <-tk.C:
			ck()
		}
	}
}

func (pool *TODOPool) ignoreAction(action types.TxAction) bool {
	switch action {
	case types.ActionTransferPayment,
		types.ActionPowTxContractDeal,
		types.ActionContractFreeze,
		types.ActionSCIdleContractDeal:
		//不记录
		return true
	default:
		return false
	}
}

//推送新消息
func (pool *TODOPool) Push(chain common.Address, txhash common.Hash, tx *types.Transaction, action types.TxAction) {
	if pool.disable {
		return
	}

	if tx != nil && tx.Seed() > 0 {
		//不记录Pow交易
		return
	}
	if pool.ignoreAction(action) {
		return
	}
	msg := ExecMsg{
		Chain:  chain,
		TxHash: txhash,
		Action: action,
		IsNew:  true,
	}
	if tx != nil {
		msg.expiredTime = uint64(tx.ExpirationTime().Unix())
	}

	select {
	case pool.itemCh <- msg:
	case <-pool.quit:
	}
}

//删除给定的消息
func (pool *TODOPool) Del(chain common.Address, txhash common.Hash, action types.TxAction) {
	if pool.disable {
		return
	}
	if pool.ignoreAction(action) {
		return
	}
	msg := ExecMsg{
		Chain:  chain,
		TxHash: txhash,
		Action: action,
		IsNew:  false,
	}
	select {
	case pool.itemCh <- msg:
	case <-pool.quit:
	}
}

//仅仅用于测试
var onWriteTest func(count int)

type reseter interface {
	Reset()
}

func (pool *TODOPool) lookup() {
	pool.exitwg.Add(1)
	defer pool.exitwg.Done()

	const maxItems = 2000
	var sum int

	// 实时监听接收到的数据，但不能频繁处理，需要通过
	batch := pool.db.NewBatch()

	commit := func(force bool) {
		if sum == 0 {
			return
		}
		if !force && sum < maxItems {
			return
		}
		if err := batch.Write(); err != nil {
			log.Crit("failed to commit data", "err", err)
		}
		batch.Discard()
		if onWriteTest != nil {
			onWriteTest(sum)
		}
		sum = 0
	}
	write := func(msg ExecMsg) {

		if msg.IsNew {
			batch.Put(encodeKey(msg), common.Uint64ToBytes(msg.expiredTime))
		} else {
			batch.Delete(encodeKey(msg))
		}
		sum++
		commit(false)
	}
	defer commit(true)

	for {
		select {
		case one := <-pool.itemCh:
			write(one)
		case <-pool.quit:
			for len(pool.itemCh) > 0 {
				write(<-pool.itemCh)
			}
			return
		}
	}

}

//遍历指定链上的消息
func (pool *TODOPool) RangeChain(chain common.Address, cb func(msg ExecMsg) bool) error {
	if pool.disable {
		return nil
	}
	return pool.db.Iterator(getChainPrefix(chain), func(key, value []byte) bool {
		msg := decodeKey(key)
		if msg.TxHash.Empty() {
			return true
		}
		if len(value) == 8 {
			msg.expiredTime = common.BytesToUInt64(value)
		}
		return cb(msg)
	})
}

//遍历所有消息
func (pool *TODOPool) Range(cb func(msg ExecMsg) bool) error {
	if pool.disable {
		return nil
	}
	return pool.db.Iterator(dataPrekey, func(key, value []byte) bool {
		msg := decodeKey(key)
		if msg.TxHash.Empty() {
			return true
		}
		if len(value) == 8 {
			msg.expiredTime = common.BytesToUInt64(value)
		}
		return cb(msg)
	})
}
