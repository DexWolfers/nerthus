package ucache

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/pkg/errors"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/consensus"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/nts/synchronize/message"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/p2p/discover"
	"gitee.com/nerthus/nerthus/rlp"
	"github.com/hashicorp/golang-lru"
)

var cachePrefix = []byte("unit_cache_")

type cacheRecord struct {
	Chain     string
	Height    uint64
	Uhash     string
	Timestamp uint64
	Inserted  uint64
}

func unit2Record(u *types.Unit) cacheRecord {
	return cacheRecord{
		Chain:     u.MC().Hex(),
		Height:    u.Number(),
		Uhash:     u.Hash().Hex(),
		Timestamp: u.Timestamp(),
		Inserted:  uint64(time.Now().Unix()),
	}
}

type FetchHistory struct {
	Number uint64
	Time   time.Time
}

type UnitCache struct {
	core         MainSync
	db           ntsdb.Database
	rd           ntsdb.RD
	cached       *lru.Cache
	cwr          ChainWriteRead
	fetchHistory *lru.Cache
	fetchLock    sync.Mutex
	notifyFetchc chan struct{}
	exitw        sync.WaitGroup
	quit         chan struct{}
}

func NewCache(mainSync MainSync, dataDB ntsdb.Database, rd ntsdb.RD, cwr ChainWriteRead) (*UnitCache, error) {
	cached, err := lru.New(5 * 1024 * 1024)
	if err != nil {
		return nil, err
	}
	fetchHistory, _ := lru.New(200)
	c := UnitCache{
		core:         mainSync,
		db:           dataDB,
		rd:           rd,
		cached:       cached,
		cwr:          cwr,
		fetchHistory: fetchHistory,
		notifyFetchc: make(chan struct{}, 1),
		quit:         make(chan struct{}),
	}
	if err := c.prepare(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *UnitCache) Start() {
	go c.autoSyncMissing()
	go c.listen()
}
func (c *UnitCache) Stop() {
	close(c.quit)
}

func (c *UnitCache) IsStopped() bool {
	select {
	case <-c.quit:
		return true
	default:
		return false
	}
}

func (c *UnitCache) prepare() error {
	return c.rd.Exec(func(conn *sql.Conn) error {
		//检查表是否存在
		_, err := conn.ExecContext(context.Background(), `
CREATE TABLE IF NOT EXISTS "cache_units" ( 
"chain" TEXT, "height" INTEGER, "uhash" TEXT, "timestamp" INTEGER, "inserted" INTEGER, PRIMARY KEY("uhash") 
) WITHOUT ROWID`)
		return err
	})
}

func (c *UnitCache) makeKey(key []byte) []byte {
	return append(cachePrefix, key...)
}

func (c *UnitCache) RD() ntsdb.RD {
	return c.rd
}

type FetchByNumberMsg struct {
	message.FetchUnitById
	tailTime uint64
	missing  types.UnitID
}

// 拉取最新
func (c *UnitCache) FetchAncestor(src discover.NodeID, unit *types.Unit) error {
	c.fetchLock.Lock()
	defer c.fetchLock.Unlock()

	if c.IsStopped() {
		return errors.New("stopped")
	}

	selectPeer := func() {
		before := src
		if src.IsEmpty() {
			src = c.core.PeerStatusSet().Best(nil)
		} else if c.core.PeerStatusSet().Get(src).LastUintHash.Empty() {
			src = c.core.PeerStatusSet().Best(nil)
		}
		log.Trace("select fetch peer", "before", before, "peer", src)
		if src.IsEmpty() {
			select {
			case <-c.quit:
				src = discover.NodeID{}
			default:
				src = c.core.PeerStatusSet().Best(nil)
			}
		}
	}

	selectPeer()
	if src.IsEmpty() {
		return nil
	}

	missing := missingWhich(unit, func(hashes common.Hash) bool {
		mc, _ := c.cwr.GetUnitNumber(hashes)
		return !mc.Empty()
	})
	//可能运气不错，缺失的单元已经补齐，那么再依次尝试写入
	if len(missing) == 0 {
		unit.ReceivedAt = time.Now()
		_, err := c.cwr.InsertChain(types.Blocks{unit})
		if err == nil || err == core.ErrExistStableUnit || err == consensus.ErrKnownBlock {
			c.Del(unit.Hash())
			//可以以高度删除一次该链单元
			list, err := c.QueryChainUnitsByMaxNumber(unit.MC(), unit.Number())
			if err != nil {
				log.Error("query failed", "err", err)
			} else {
				c.BatchDel(list)
			}
		}
		return nil
	}

	//需要从此链的第一个高度拉取
	do := make(map[common.Address]struct{})
	var fetchMsg []FetchByNumberMsg
	now := time.Now()
	for _, unit := range missing {

		if c.IsStopped() {
			return errors.New("stopped")
		}

		if _, ok := do[unit.ChainID]; ok {
			continue
		}
		do[unit.ChainID] = struct{}{}

		//短期内部允许重复拉取
		if v, ok := c.fetchHistory.Get(unit.ChainID); ok {
			if now.Sub(v.(FetchHistory).Time) <= time.Minute {
				continue
			}
		}

		//取值尾部
		tail := c.cwr.GetChainTailHead(unit.ChainID)
		if tail.Number >= unit.Height {
			//TODO:可以清理 cache
			return errors.New("bad ancestor")
		}

		fetchNumber, err := c.queryChainNextFetchNumber(unit.ChainID, tail.Number+1)
		if err != nil {
			return err
		}

		fetchMsg = append(fetchMsg, FetchByNumberMsg{
			tailTime: tail.Timestamp,
			missing:  unit,
			FetchUnitById: message.FetchUnitById{
				types.UnitID{
					ChainID: unit.ChainID,
					Height:  fetchNumber,
				}}})

	}

	//按时间排序，时间最早的优先访问
	sort.Slice(fetchMsg, func(i, j int) bool {
		return fetchMsg[i].tailTime < fetchMsg[i].tailTime
	})
	for _, msg := range fetchMsg {
		if c.IsStopped() {
			return errors.New("stopped")
		}
		//向此节点拉取此高度后的单元
		err := c.core.SendMessageWitCode(0, msg.FetchUnitById, src)

		log.Trace("fetch missing unit",
			"chain", msg.UID.ChainID, "missing.number", msg.missing.ChainID, "missing.uhash", msg.missing.Hash,
			"fetch.number", msg.UID.Height, "tail.time", msg.tailTime, "src", src, "err", err)

		//如果发生成功则记录拉取历史
		if err == nil {
			c.fetchHistory.Add(msg.UID.ChainID, FetchHistory{msg.UID.Height, time.Now()})
		} else {
			selectPeer()
			if src.IsEmpty() {
				return nil
			}
		}
	}
	return nil
}

func (c *UnitCache) Put(u *types.Unit) error {
	// 先检查是否有缓存
	if ok, _ := c.cached.ContainsOrAdd(u.Hash(), u); ok {
		return nil
	}
	data, err := rlp.EncodeToBytes(u)
	if err != nil {
		panic(err)
	}
	err = c.db.Put(c.makeKey(u.Hash().Bytes()), data)
	if err != nil {
		return err
	}
	//需要记录位置信息
	return c.put(unit2Record(u))
}

func (c *UnitCache) put(r cacheRecord) error {
	err := c.rd.Exec(func(conn *sql.Conn) error {
		_, err := conn.ExecContext(context.Background(),
			`INSERT OR IGNORE INTO cache_units("chain","height","uhash","timestamp","inserted")VALUES(?,?,?,?,?)`,
			r.Chain, r.Height, r.Uhash, r.Timestamp, r.Inserted)
		return err
	})
	if err == nil {
		log.Trace("cached unit", "chain", r.Chain, "number", r.Height, "uhash", r.Uhash)
	}
	return err
}

func (c *UnitCache) BatchPut(units []*types.Unit) error {
	if len(units) == 0 {
		return nil
	}
	list := make([]cacheRecord, len(units))
	for i, u := range units {
		list[i] = unit2Record(u)

		data, err := rlp.EncodeToBytes(u)
		if err != nil {
			panic(err)
		}
		err = c.db.Put(c.makeKey(u.Hash().Bytes()), data)
		if err != nil {
			return err
		}
	}
	//update info
	err := c.rd.Exec(func(conn *sql.Conn) error {
		tx, err := conn.BeginTx(context.Background(), nil)
		if err != nil {
			return err
		}
		stmt, err := tx.PrepareContext(context.Background(),
			`REPLACE INTO cache_units("chain","height","uhash","timestamp","inserted")VALUES(?,?,?,?,?)`)
		if err != nil {
			return err
		}
		for _, r := range list {
			_, err = stmt.Exec(r.Chain, r.Height, r.Uhash, r.Timestamp, r.Inserted)
			if err != nil {
				tx.Rollback()
				stmt.Close()
				return err
			}
			log.Trace("add to cache", "chain",
				r.Chain, "number", r.Height, "uhash", r.Uhash,
				"timestamp", r.Timestamp)
		}
		stmt.Close()
		return tx.Commit()
	})
	return err
}

func (c *UnitCache) Exist(uhash common.Hash) bool {
	// 先检查是否有缓存
	if c.cached.Contains(uhash) {
		return true
	}
	data, _ := c.db.Get(c.makeKey(uhash.Bytes()))
	if len(data) > 0 {
		return true
	}
	mc, _ := c.cwr.GetUnitNumber(uhash)
	return !mc.Empty()
}

func (c *UnitCache) Get(uhash common.Hash) (*types.Unit, error) {
	// 先检查是否有缓存
	if v, ok := c.cached.Get(uhash); ok {
		return v.(*types.Unit), nil
	}
	data, _ := c.db.Get(c.makeKey(uhash.Bytes()))
	if len(data) == 0 {
		return nil, nil // errors.New("unitcache: not found unit in cache db")
	}

	// 编码
	var u types.Unit
	if err := rlp.DecodeBytes(data, &u); err != nil {
		return nil, err
	}
	// 缓存
	c.cached.Add(uhash, &u)
	return &u, nil
}

func (c *UnitCache) queryHashList(sql string, args ...interface{}) ([]common.Hash, error) {
	rows, err := c.rd.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	var list []common.Hash
	// 快速存储单元哈希，以便快速释放DB row 锁
	for rows.Next() { //scan all before use
		var hash string
		err = rows.Scan(&hash)
		if err != nil {
			return nil, err
		}
		list = append(list, common.HexToHash(hash))
	}
	return list, nil
}

// 获取指定链高度前的单元
func (c *UnitCache) QueryChainUnitsByMaxNumber(chain common.Address, maxNumber uint64) ([]common.Hash, error) {
	return c.queryHashList("select uhash from cache_units where height<=? and chain=?", maxNumber, chain.Hex())
}

func (c *UnitCache) GetUnitHashByTime(maxInserted int64) ([]common.Hash, error) {
	if maxInserted > 0 {
		return c.queryHashList("select uhash from cache_units where inserted <= ? order by timestamp", maxInserted)
	}
	// select by time
	return c.queryHashList("select uhash from cache_units order by timestamp")
}

func (c *UnitCache) Del(uhash common.Hash) error {
	c.cached.Remove(uhash)
	if err := c.db.Delete(c.makeKey(uhash.Bytes())); err != nil {
		return err
	}
	var deleted bool
	err := c.rd.Exec(func(conn *sql.Conn) error {
		r, err := conn.ExecContext(context.Background(), "DELETE FROM cache_units WHERE uhash=?", uhash.Hex())
		if err == nil {
			n, _ := r.RowsAffected()
			deleted = n > 0
		}
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to delete from cache,%v", err)
	}
	if deleted {
		log.Trace("unitcache: deleted unit cache", "uhash", uhash)
	}
	return nil
}

func (c *UnitCache) BatchDel(list []common.Hash) error {
	for _, v := range list {
		c.cached.Remove(v)
		if err := c.db.Delete(c.makeKey(v.Bytes())); err != nil {
			return err
		}
	}
	err := c.rd.Exec(func(conn *sql.Conn) error {
		ctx := context.Background()
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		stmt, err := tx.PrepareContext(ctx, "DELETE FROM cache_units WHERE uhash=?")
		if err != nil {
			return err
		}
		for _, v := range list {
			_, err := stmt.ExecContext(ctx, v.Hex())
			if err != nil {
				tx.Rollback()
				return err
			}
			log.Trace("unitcache: delete unit cache", "uhash", v)
		}
		return tx.Commit()
	})
	return err
}

func (c *UnitCache) GetChains() ([]common.Address, error) {
	rows, err := c.rd.Query("select distinct chain from cache_units order by timestamp")
	if err != nil {
		return nil, err
	}
	var list []common.Address
	var addr string
	for rows.Next() {
		if err = rows.Scan(&addr); err != nil {
			return nil, err
		}
		list = append(list, common.ForceDecodeAddress(addr))
	}
	return list, nil
}

func (c *UnitCache) GetChainFirstUnit(chain common.Address, minHeight uint64) (types.UnitID, error) {
	row := c.rd.QueryRow("select height,uhash from cache_units where height>=? and chain=? order by timestamp limit 1",
		minHeight, chain.Hex())
	var (
		height  uint64
		hexHash string
	)
	err := row.Scan(&height, &hexHash)
	if err != nil {
		if err == sql.ErrNoRows {
			return types.UnitID{}, nil
		}
		return types.UnitID{}, err
	}
	return types.UnitID{
		ChainID: chain,
		Hash:    common.HexToHash(hexHash),
		Height:  height,
	}, nil
}

// 根据缓存情况检查指定链可拉取的可以开始单元高度位置，从而尽可能的避免重复拉取已缓存的单元
func (c *UnitCache) queryChainNextFetchNumber(chain common.Address, fetchNumber uint64) (uint64, error) {
	if fetchNumber == 0 {
		return 0, errors.New("invalid param: fetchNumber is zero")
	}

	rows, err := c.rd.Query("select height from cache_units where height>=? and chain=? order by height", fetchNumber, chain.String())
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var (
		number      uint64
		beforeValue = fetchNumber - 1
	)
	// 比如： 需要从高度 10 拉取数据，在缓存中存在几种情况：
	// 1: 结果为空格，则说明无缓存，需要从此高度开始拉取
	// 2. 有结果，但是第一个缓存不是高度 10， 而是 [15,16,20] ，此时：继续从 10 高度拉取
	// 3. 有结果，第一个是 10，但是：
	//		3.1  [10,12, 13,14] ，需要从 11 开始拉取
	//      3.2  [10,11,12,15,16]，需要从 13 开始拉取
	//      3.3  [10,11,12,13,14,15] 需要从 16 开始拉取
	// 算法：
	//	 if len=0 then return fetchNumber
	//	 else if v[n]!=v[n-1]+1 then return  v[n-1]+1
	//	 else return lastV+1
	for rows.Next() {
		if err = rows.Scan(&number); err != nil {
			return 0, err
		}
		if number != beforeValue+1 {
			break
		}
		beforeValue = number
	}
	if number == 0 {
		return fetchNumber, nil //说明结果为空，按原拉取
	}
	return beforeValue + 1, nil
}

func (r *UnitCache) listen() {
	r.exitw.Add(1)
	defer r.exitw.Done()

	// 监听单元落地情况
	ch := make(chan types.RealtimeSyncEvent, 1024*5)
	sub2 := r.cwr.SubscribeUnitStatusEvent(ch)
	defer sub2.Unsubscribe()

	handle := func(ev types.RealtimeSyncEvent) {
		if ev.Unit != nil {
			ev.UnitHash = ev.Unit.Hash()
		}
		switch ev.SyncType {
		// 无效单元或者落地成功，则删除缓存
		case types.RealtimeSyncBad, types.RealtimeSyncBeta:
			//写入成功或者无效单元时，删除缓存
			err := r.Del(ev.UnitHash)
			if err != nil {
				log.Error("failed to delete cache after unit has saved",
					"uhash", ev.UnitHash, "err", err)
			}
			if ev.Unit == nil || ev.SyncType == types.RealtimeSyncBad {
				return
			}
			//当有单元落地时清理缓存
			if history, ok := r.fetchHistory.Get(ev.Unit.MC()); ok && history.(FetchHistory).Number <= ev.Unit.Number() {
				r.fetchHistory.Remove(ev.Unit.MC())
			}
			//如果是未来单元，则存入缓存中，已方便后续处理。
			//但不能保留所有Future单元，间隔太多的也将忽略。
		case types.RealtimeSyncFuture:
			if ev.Unit == nil {
				return
			}
			//TODO: 会引发脏数据攻击，故意推送大量无效数据
			err := r.Put(ev.Unit)
			if err != nil {
				log.Error("failed to cache unit", "uhash", ev.UnitHash, "err", err)
			}
		}
	}

	// 监听，退出时需要继续处理完毕
	for {
		select {
		case <-r.quit:
			sub2.Unsubscribe()
			// save all before quite
			for {
				if len(ch) == 0 {
					break
				}
				handle(<-ch)
			}
			return
		case ev, ok := <-ch:
			if ok {
				handle(ev)
			}
		}
	}
}
