package bysolt

import (
	"fmt"
	"hash"
	"sync"
	"sync/atomic"
	"time"

	"gitee.com/nerthus/nerthus/core/types"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/ntsdb"
	"github.com/pkg/errors"
)

type rangeUnitsFunc func(db ntsdb.Database, start, end uint64, cb func(utime uint64, uhash common.Hash) bool)

type Storage struct {
	db               ntsdb.Database
	sysFirstUnitTime time.Time //系统链第一个单元时间戳
	todoCache        sync.Map  //待更新的清单
	lastPoint        *Point    //已更新的最后一个 Point
	getUnits         rangeUnitsFunc
	updating         uint32 //是否正在更新中
	hasher           hash.Hash
	exitwp           sync.WaitGroup
	quit             chan struct{}
}

func NewStorage(db ntsdb.Database, firstTimestamp uint64, getUnits rangeUnitsFunc) *Storage {
	return &Storage{
		db:               db,
		getUnits:         getUnits,
		sysFirstUnitTime: time.Unix(0, int64(firstTimestamp)),
		quit:             make(chan struct{}),
	}
}

// 获取指定时间点的时间区间
func (sto *Storage) PointTimeRange(number uint64) TimeSize {
	b, e := PointTimeRange(sto.sysFirstUnitTime, number)
	return TimeSize{b, e}
}
func (sto *Storage) LastPoint() *Point {
	last := sto.lastPoint

	if last == nil {
		number := getLastUnitPointNumber(sto.db)
		if number > 0 {
			p := getUnitPoint(sto.db, number)
			if p != nil {
				sto.lastPoint = p
				last = p
			}
		}
	}
	if last == nil {
		return nil
	}

	return &Point{
		Number:   last.Number,
		Parent:   last.Parent,
		TreeRoot: last.TreeRoot,
	}
}
func (sto *Storage) GetUnitPoint(number uint64) *Point {
	return getUnitPoint(sto.db, number)
}

// 创建一个无缓存区间单元遍历器，注意一旦开启，则无法停止
func (sto *Storage) rangeUnits(timeSize TimeSize) <-chan UnitInfo {
	nc := make(chan UnitInfo, 0)
	go func(nc chan UnitInfo) {
		sto.getUnits(sto.db,
			uint64(timeSize.Begin.UnixNano()),
			uint64(timeSize.End.UnixNano()),
			func(utime uint64, uhash common.Hash) bool {
				select {
				case <-sto.quit:
					return false
				case nc <- UnitInfo{utime, uhash}:
				}
				return true
			})
		//结束
		close(nc)
	}(nc)
	return nc
}

func (sto *Storage) updatePoint(number uint64, logger log.Logger) error {
	if number == 0 {
		panic("number is zero")
	}

	var lastRoot Hash
	if number > 1 {
		last := getUnitPoint(sto.db, number-1)
		lastRoot = last.Hash()
	}

	timeRange := sto.PointTimeRange(number)

	if logger == nil {
		logger = log.New("number", number)
	}

	bs := time.Now()
	list := FindNodes(timeRange, maxSlotUnits, make([]UnitInfo, 0, maxSlotUnits), sto.rangeUnits(timeRange))
	logger.Trace("get unit nodes", "len", len(list), "cost", time.Since(bs))
	if sto.stopped() {
		logger.Trace("stopped when quit")
		return errors.New("stopped when quit")
	}

	tree, err := CreateTree(timeRange, list)
	if err != nil {
		logger.Trace("failed to create unit point tree", "err", err)
		return fmt.Errorf("failed to create unit point tree,%s", err.Error())
	}
	logger.Trace("create unit point tree done", "root", tree.Hash())
	if sto.stopped() {
		logger.Trace("stopped when quit")
		return errors.New("stopped when quit")
	}

	//写入数据库中
	point := Point{
		Number:   number,
		Parent:   lastRoot,
		TreeRoot: tree.Hash(),
	}
	//因为不清楚后续将有多少点数据需要更新，因此此处不开启批量提交。而是逐个处理。
	writeUnitPoint(sto.db, point)
	//如果后面有其他则，需要进一步更新 Root
	lastHash, next := point.Hash(), number+1
	for {
		p := getUnitPoint(sto.db, next)
		if p == nil || p.Hash() == lastHash {
			next-- //方便记录真实更新的最后位置
			break
		}
		p.Parent = lastHash
		writeUnitPoint(sto.db, *p)
		lastHash = p.Hash() //更新下一个
		next++
	}
	//结束时，删除标记
	sto.todoDone(point.Number)

	logger.Trace("last update number", "pointHash", point.Hash(), "begin", number, "end", next)
	return nil
}

func (sto *Storage) Start() {
	go sto.autoUpdate()
}
func (sto *Storage) Stop() {
	close(sto.quit)
	sto.exitwp.Wait()
}

// 当出现新单元时，需要根据单元时间戳，失败是否有需要更新的内容
// 一小时内的单元，不进行实时更新。
// 但是如果是接收到一个时间久远的单元，则将需要更新对应单元段的内容。
func (sto *Storage) NewUnitDone(unit *types.Unit) {
	utime := time.Unix(0, int64(unit.Timestamp()))
	//过滤最近一小时内的单元
	if utime.After(time.Now().Add(-pointTimeSpan)) {
		return
	}
	sto.addTODO(CalcPointNumber(sto.sysFirstUnitTime, utime))
}

// 立即更新，并等待到到更新完毕。
func (sto *Storage) UpdateNow(number uint64) error {
	sto.addTODO(number)

	if err := sto.checkUpdate(); err != errUpdateRunning {
		return err
	}
	//如果上面执行时已经有其他任务执行，则需要继续等待执行。
	tk := time.NewTicker(time.Second) //1 秒间隔检查
	defer tk.Stop()
	for {
		select {
		case <-sto.quit:
			return errors.New("stopped")
		case <-tk.C:
			//继续，直到 成功或者失败，而非 errUpdateRunning
			if err := sto.checkUpdate(); err != errUpdateRunning {
				return err
			}
		}
	}

}

func (sto *Storage) addTODO(number uint64) {
	if number == 0 {
		panic("number is zero")
	}
	if _, loaded := sto.todoCache.LoadOrStore(number, struct{}{}); loaded {
		return
	}
	writeUPUpdateTODO(sto.db, number)
	log.Trace("unit point need update", "number", number, "range", sto.PointTimeRange(number))
}
func (sto *Storage) todoDone(number uint64) {
	if number == 0 {
		panic("number is zero")
	}
	sto.todoCache.Delete(number)
	upUpdateDone(sto.db, number)
	log.Trace("unit point update done", "number", number)
}

func (sto *Storage) stopped() bool {
	select {
	default:
		return false
	case <-sto.quit:
		return true
	}
}
func (sto *Storage) autoUpdate() {
	sto.exitwp.Add(1)
	defer sto.exitwp.Done()

	//以单元时间为连接，将所有单元按时间排序。
	//因为最近数据属于频繁变化中，因此不进行更新。
	//定期更新每个时间段的单元树，

	//启动时进行一次更新
	err := sto.checkUpdate()
	if err != nil {
		log.Error("failed check and update unit tree at last time range", "err", err)
	}

	//每分钟执行一次检查，间隔下能快速更新内容。
	// 但如果此区间新单元接收频繁，则引起多次更新。
	// 频繁情况：
	//	  1. 新节点启动同步的 ----> 外部在系统链不到达最近高度时，将不会触发。
	//	  2. 节点实时单元多，----> 不处理最近一小时内的单元
	//    3. 中间缺失的单元补充 -----> 正是需要早日更新部分。
	tk := time.NewTicker(time.Minute)
	defer tk.Stop()

	for {
		select {
		case <-sto.quit:
			return
		case <-tk.C:
			//更新
			err := sto.checkUpdate()
			if err != nil {
				log.Error("failed check and update unit tree at last time range", "err", err)
			}
		}
	}
}

var errUpdateRunning = errors.New("update is running")

func (sto *Storage) checkUpdate() error {
	//不允许多个执行
	if !atomic.CompareAndSwapUint32(&sto.updating, 0, 1) {
		return errUpdateRunning
	}
	defer atomic.CompareAndSwapUint32(&sto.updating, 1, 0)
	logger := log.New("batch", time.Now().Unix())
	logger.Debug("begin check unit tree...")

	//获取上次更新位置
	last := getLastUnitPointNumber(sto.db)

	//如果下一个待更新的位置上的时间是在一个小时后
	needLast := CalcPointNumber(sto.sysFirstUnitTime, time.Now().Add(-pointTimeSpan))
	for next := last + 1; next <= needLast; next++ {
		sto.addTODO(next)
	}

	//将所有需要更新的点进行进行
	//遍历获得的待更新在 leveldb 下是升序的
	var (
		lastUpdated uint64
		updateErr   error
	)
	rangeTodos(sto.db, func(number uint64) bool {
		//可以更新
		err := sto.updatePoint(number, logger)
		if err != nil {
			logger.Trace("failed to update", "number", number, "err", err)
			updateErr = fmt.Errorf("failed to update unit point %d,%s", number, err)
			return false
		}
		lastUpdated = number
		if sto.stopped() {
			return false
		}
		return true
	})

	if lastUpdated > last {
		//更新末尾位置
		writeLastUnitPointNumber(sto.db, lastUpdated)
		sto.lastPoint = nil
	}
	logger.Debug("update done", "lastUpdated", lastUpdated, "lastUpdateErr", updateErr)
	return updateErr
}

// 接口实现

func (sto *Storage) LastPointStatus() NodeStatus {
	return sto.LastPoint().ToNodeStauts()
}

func (sto *Storage) GetPointStatus(number uint64) NodeStatus {
	return sto.GetUnitPoint(number).ToNodeStauts()

}

func (sto *Storage) GetPeriodHashes(number, from uint64, to uint64) []common.Hash {
	list := make([]common.Hash, 0, maxSlotUnits)
	sto.getUnits(sto.db, from, to, func(utime uint64, uhash common.Hash) bool {
		list = append(list, uhash)
		return true
	})
	return list
}

func (sto *Storage) GetTimestamp(number uint64) uint64 {
	size := sto.PointTimeRange(number)
	return uint64(size.Begin.UnixNano())
}

func (sto *Storage) GetPointTree(number uint64) (Node, error) {
	size := sto.PointTimeRange(number)
	items := FindNodes(size, maxSlotUnits, nil, sto.rangeUnits(size))
	log.Debug("generate tree", "first", sto.sysFirstUnitTime, "number", number, "size", size, "items", len(items))
	return CreateTree(size, items)
}
