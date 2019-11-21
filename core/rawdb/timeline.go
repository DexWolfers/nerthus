package rawdb

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/ntsdb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

var (
	timelinePrefix = append([]byte("ut_"), []byte{255, 255, 255, 255, 255}...)
	timelineEnd    = util.BytesPrefix(timelinePrefix).Limit //遍历的终止位置
)

// 单元的时间线位置 Key= 前缀+时间戳+单元哈希
// 因为 Level DB 数据的排放顺序是按照 Key 升序排列。
// 因此按时间作为Key的好处是：遍历数据时，已经保证顺序，即完整的按时间从早到晚排列的单元。
func getTimeLineKey(utime uint64, uhash common.Hash) []byte {
	return append(append(timelinePrefix, common.Uint64ToBytes(utime)...), uhash.Bytes()...)
}
func parseTimelineKey(key []byte) (uint64, common.Hash) {
	return common.BytesToUInt64(key[len(timelinePrefix) : len(key)-common.HashLength]),
		common.BytesToHash(key[len(key)-common.HashLength-1:])
}

// 在时间线上记录单元
func WriteTimeline(db ntsdb.Putter, uhash common.Hash, utime uint64) {
	//其值为空，因为单元哈希已经作为 key 的一部分
	if err := db.Put(getTimeLineKey(utime, uhash), nil); err != nil {
		log.Crit("Failed to store unit time to hash mapping", "err", err)
	}
}

func DelTimeline(db ntsdb.Deleter, utime uint64, uhash common.Hash) {
	if err := db.Delete(getTimeLineKey(utime, uhash)); err != nil {
		log.Crit("Failed to delete unit time to hash mapping", "err", err)
	}
}

// 获取本地最后一个落地单元
func LastUnitHash(db ntsdb.Database) (common.Hash, uint64) {
	ldb, ok := db.(*ntsdb.LDBDatabase)
	if !ok {
		return common.Hash{}, 0
	}
	iter := ldb.NewIterator(&util.Range{Start: timelinePrefix, Limit: timelineEnd}, nil)
	defer iter.Release()
	if iter.Last() {
		u, h := parseTimelineKey(iter.Key())
		return h, u
	}
	return common.Hash{}, 0
}

// 从起始时间获取时间线上的单元
func UnitsByTimeLine(db ntsdb.Database, start uint64, count int) []common.Hash {
	if count == 0 {
		return nil
	}
	list := make([]common.Hash, 0, count)
	RangUnitByTimline(db, start, func(hash common.Hash) bool {
		list = append(list, hash)
		return len(list) < count
	})
	return list
}

func RangUnitByTimline(db ntsdb.Database, start uint64, cb func(hash common.Hash) bool) {
	rangeUnitsByTimeline(db, getTimeLineKey(start, common.Hash{}), timelineEnd, func(_ uint64, uhash common.Hash) bool {
		return cb(uhash)
	})
}

// 在指定区间内检索单元
func RangRangeUnits(db ntsdb.Database, start, end uint64, cb func(utime uint64, uhash common.Hash) bool) {
	rangeUnitsByTimeline(db, getTimeLineKey(start, common.Hash{}), getTimeLineKey(end+1, common.Hash{}), cb)
}

func rangeUnitsByTimeline(db ntsdb.Database, start, end []byte, cb func(utime uint64, uhash common.Hash) bool) {
	// 从 Start 到固定数量的单元哈希
	iter := mustLDB(db).NewIterator(&util.Range{Start: start, Limit: end}, nil)
	defer iter.Release()
	for iter.Next() {
		if !cb(parseTimelineKey(iter.Key())) {
			break
		}
	}
}

func mustLDB(db ntsdb.Database) *ntsdb.LDBDatabase {
	ldb, ok := db.(*ntsdb.LDBDatabase)
	if !ok {
		//当前数据库只是有 Level DB 因此当前不支持从其他数据库中查找。以后可以扩展。
		panic("db should be is level db")
	}
	return ldb
}
