package ntsdb

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"github.com/syndtr/goleveldb/leveldb/util"

	"github.com/stretchr/testify/require"
)

type dbSuite struct {
	name  string
	tb    *testing.T
	newdb func(t *testing.T) (Database, func())
}

func NewSuite(tb *testing.T, name string, newdb func(t *testing.T) (Database, func())) *dbSuite {
	s := dbSuite{
		name:  name,
		newdb: newdb,
		tb:    tb,
	}
	return &s
}

func (s *dbSuite) Run() {
	s.tb.Run("put_get", func(t *testing.T) {
		db, close := s.newdb(t)
		defer close()
		testPutGet(db, t)
	})
	s.tb.Run("parallelPutGet", func(t *testing.T) {
		db, close := s.newdb(t)
		defer close()
		testParallelPutGet(db, t)
	})
	s.tb.Run("batch", func(t *testing.T) {
		db, close := s.newdb(t)
		defer close()
		testBatch(t, db)
	})
	s.tb.Run("delete", func(t *testing.T) {
		db, close := s.newdb(t)
		defer close()
		testBatch(t, db)
	})
	s.tb.Run("testBatchWrite", func(t *testing.T) {
		db, close := s.newdb(t)
		defer close()
		testBatch(t, db)
	})
	//s.tb.Run("iterator", func(t *testing.T) {
	//	db, close := s.newdb(t)
	//	defer close()
	//	testIterator(db, t)
	//})

}

func newTestLDB() (*LDBDatabase, func()) {
	dirname, err := ioutil.TempDir(os.TempDir(), "ntsdb_test_")
	if err != nil {
		panic("failed to create test file: " + err.Error())
	}
	dbIns, err := NewLDBDatabase(dirname, 0, 0)
	if err != nil {
		panic("failed to create test database: " + err.Error())
	}

	return dbIns, func() {
		dbIns.Close()
		os.RemoveAll(dirname)
	}
}

var test_values = []string{"", "a", "1251", "\x00123\x00"}

func TestLDB_PutGet(t *testing.T) {
	dbIns, remove := newTestLDB()
	defer remove()
	testPutGet(dbIns, t)
}

func TestMemoryDB_PutGet(t *testing.T) {
	dbIns, _ := NewMemDatabase()
	testPutGet(dbIns, t)
}

func testPutGet(dbIns Database, t *testing.T) {
	t.Parallel()

	for _, v := range test_values {
		err := dbIns.Put([]byte(v), []byte(v))
		if err != nil {
			t.Fatalf("put failed: %v", err)
		}
	}

	for _, v := range test_values {
		data, err := dbIns.Get([]byte(v))
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}
		if !bytes.Equal(data, []byte(v)) {
			t.Fatalf("get returned wrong result, got %q expected %q", string(data), v)
		}
	}

	for _, v := range test_values {
		err := dbIns.Put([]byte(v), []byte("?"))
		if err != nil {
			t.Fatalf("put override failed: %v", err)
		}
	}

	for _, v := range test_values {
		data, err := dbIns.Get([]byte(v))
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}
		if !bytes.Equal(data, []byte("?")) {
			t.Fatalf("get returned wrong result, got %q expected ?", string(data))
		}
	}

	for _, v := range test_values {
		orig, err := dbIns.Get([]byte(v))
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}
		orig[0] = byte(0xff)
		data, err := dbIns.Get([]byte(v))
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}
		if !bytes.Equal(data, []byte("?")) {
			t.Fatalf("get returned wrong result, got %q expected ?", string(data))
		}
	}

	for _, v := range test_values {
		err := dbIns.Delete([]byte(v))
		if err != nil {
			t.Fatalf("delete %q failed: %v", v, err)
		}
	}

	for _, v := range test_values {
		_, err := dbIns.Get([]byte(v))
		if err == nil {
			t.Fatalf("got deleted value %q", v)
		}
	}
}

func TestLDB_ParallelPutGet(t *testing.T) {
	db, remove := newTestLDB()
	defer remove()
	testParallelPutGet(db, t)
}

func TestMemoryDB_ParallelPutGet(t *testing.T) {
	db, _ := NewMemDatabase()
	testParallelPutGet(db, t)
}

func testParallelPutGet(dbIns Database, t *testing.T) {
	const n = 8
	var pending sync.WaitGroup

	pending.Add(n)
	for i := 0; i < n; i++ {
		go func(key string) {
			defer pending.Done()
			err := dbIns.Put([]byte(key), []byte("v"+key))
			if err != nil {
				panic("put failed: " + err.Error())
			}
		}(strconv.Itoa(i))
	}
	pending.Wait()

	pending.Add(n)
	for i := 0; i < n; i++ {
		go func(key string) {
			defer pending.Done()
			data, err := dbIns.Get([]byte(key))
			if err != nil {
				panic("get failed: " + err.Error())
			}
			if !bytes.Equal(data, []byte("v"+key)) {
				panic(fmt.Sprintf("get failed, got %q expected %q", []byte(data), []byte("v"+key)))
			}
		}(strconv.Itoa(i))
	}
	pending.Wait()

	pending.Add(n)
	for i := 0; i < n; i++ {
		go func(key string) {
			defer pending.Done()
			err := dbIns.Delete([]byte(key))
			if err != nil {
				panic("delete failed: " + err.Error())
			}
		}(strconv.Itoa(i))
	}
	pending.Wait()

	pending.Add(n)
	for i := 0; i < n; i++ {
		go func(key string) {
			defer pending.Done()
			_, err := dbIns.Get([]byte(key))
			if err == nil {
				panic("get succeeded")
			}
		}(strconv.Itoa(i))
	}
	pending.Wait()
}

func testIterator(db Database, t *testing.T) {
	iter, ok := db.(Finder)
	if !ok {
		t.Skip("database not implement Finder")
	}
	values := map[string][]byte{
		"k1":     []byte("v1"),
		"k2":     make([]byte, 1, 12*1024),
		"k3":     make([]byte, 3),
		"k4":     []byte("0x123456789abcdefabcedfgood"),
		"k5":     new(big.Int).SetUint64(1 << 63).Bytes(),
		"user_1": []byte("u1"),
		"user_2": []byte("u2"),
	}

	for k, v := range values {
		err := db.Put([]byte(k), v)
		require.NoError(t, err, "should be put successful,key=%v", k)
	}

	iter.Iterator(nil, func(k, v []byte) bool {
		require.NotEmpty(t, k)
		require.NotEmpty(t, v)
		got, ok := values[string(k)]
		require.True(t, ok)
		require.Equal(t, got, v)
		return true
	})

	wants := map[string][]byte{
		"user_1": []byte("u1"),
		"user_2": []byte("u2"),
	}
	iter.Iterator([]byte("user_"), func(k, v []byte) bool {
		require.NotEmpty(t, k)
		require.NotEmpty(t, v)
		got, ok := wants[string(k)]
		require.True(t, ok, "should be contains %s", string(k))
		require.Equal(t, got, v)
		fmt.Println(string(k))
		return true
	})
}

func TestLDBDatabase_Iterator(t *testing.T) {
	db, f := newTestLDB()
	defer f()
	testIterator(db, t)
}

func testBatch(t *testing.T, db Database) {

	batch := db.NewBatch()
	values := []struct {
		key, value []byte
	}{
		{[]byte("k1"), []byte("v1")},
		{[]byte("k2"), make([]byte, 1, 12*1024)},
		{[]byte("k3"), make([]byte, 3)},
		{[]byte("k4"), []byte("0x123456789abcdefabcedfgood")},
		{[]byte("k5"), new(big.Int).SetUint64(1 << 63).Bytes()},
		{common.Hex2Bytes("0326dd2e402f2f905b306cf94013e23ca5b0df4090d10c63bdbd31cb285815dc"),
			common.Hex2Bytes("e19f30a9ff35eb0b54424256620591fffd9346fafe81e958f63e3c7a8f3da2820801"),
		},
	}
	for _, v := range values {
		err := batch.Put(v.key, v.value)
		require.NoError(t, err, "should be put successful,key=%v", v.key)
	}
	//commit
	err := batch.Write()
	require.NoError(t, err, "should be commit")

	for _, v := range values {
		got, err := db.Get(v.key)
		require.NoError(t, err)
		require.Equal(t, v.value, got, "should be equal put value,key=%v", v.key)
	}

	t.Run("delete", func(t *testing.T) {
		batch := db.NewBatch()
		for _, v := range values {
			err := batch.Put(v.key, nil)
			require.NoError(t, err)
		}

		batch.Write()

		t.Run("get", func(t *testing.T) {
			for _, v := range values {
				got, err := db.Get(v.key)
				require.NoError(t, err)
				require.Empty(t, got)
			}
		})
	})

}

func testDelete(t *testing.T, db Database) {
	values := []struct {
		key, value []byte
	}{
		{[]byte("k1"), []byte("v1")},
		{[]byte("k2"), make([]byte, 1, 12*1024)},
		{[]byte("k3"), make([]byte, 3)},
		{[]byte("k4"), []byte("0x123456789abcdefabcedfgood")},
		{[]byte("k5"), new(big.Int).SetUint64(1 << 63).Bytes()},
	}

	for _, v := range values {
		err := db.Put(v.key, v.value)
		require.NoError(t, err, "should be put successful,key=%v", v.key)

	}
	for _, v := range values {
		err := db.Delete(v.key)
		require.NoError(t, err)
	}
	for _, v := range values {
		got, err := db.Get(v.key)
		require.NoError(t, err)
		require.Empty(t, got)
	}
}

func testBatchWrite(t *testing.T, db Database) {
	f, err := os.Open("./testdata/kv.csv")
	require.NoError(t, err)
	defer f.Close()
	r := csv.NewReader(f)

	batch := db.NewBatch()

	values := make(map[string]string)

	var index int
	for {
		index++

		line, err := r.Read()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		k, v := strings.TrimSpace(line[0]), strings.TrimSpace(line[1])
		values[k] = v

		if index%10 == 0 {
			err = db.Put(common.Hex2Bytes(k), common.Hex2Bytes(v))
			require.NoError(t, err)
		} else {
			err = batch.Put(common.Hex2Bytes(k), common.Hex2Bytes(v))
			require.NoError(t, err)
		}
	}

	err = batch.Write()
	require.NoError(t, err)

	//读取
	for k, v := range values {
		got, err := db.Get(common.Hex2Bytes(k))
		require.NoError(t, err)
		require.Equal(t, v, common.Bytes2Hex(got), "key=%s", k)
	}
}

func TestNewLDBDatabase(t *testing.T) {

	db, close := newTestLDB()

	defer close()

	timeItems := make(map[int64]struct{})
	var allTimes []time.Time

	radome := rand.New(rand.NewSource(100))

	now := time.Now()
	for i := 0; i < 25; i++ {
		now = now.Add(time.Duration(radome.Int63n(int64(time.Second))))
		timeItems[now.Unix()] = struct{}{}
		allTimes = append(allTimes, now)
		//t.Log(now.Format("2006-01-02 15:04:05.999999999"), now.UnixNano())

		str := now.Format("2006-01-02 15:04:05.999999999")

		db.Put([]byte(str), common.Uint64ToBytes(uint64(now.UnixNano())))
	}

	//db.Iterator(nil, func(key, value []byte) bool {
	//	var now time.Time
	//	now.UnmarshalBinary(value)
	//	t.Log(string(key), now.String())
	//	return true
	//})

	t.Run("all", func(t *testing.T) {
		//更加时间前缀检索
		for k, _ := range timeItems {
			tt := time.Unix(k, 0)
			key := tt.Format("2006-01-02 15:04:05")

			db.Iterator([]byte(key), func(key, value []byte) bool {
				var now time.Time
				now.UnmarshalBinary(value)
				t.Log("\t", string(key), now.String())
				return true
			})
			//查找下一个时间点

		}
	})

	t.Run("afterTime", func(t *testing.T) {
		firstStart := len(allTimes) / 2

		first := allTimes[firstStart]
		t.Log("Start:", first)

		for i, v := range getTimes(db, first, 6) {
			t.Log(i, v)
		}
	})

	t.Run("range", func(t *testing.T) {

		allTimes := createDateTimes(db)

		db.Put([]byte("keyA"), []byte("valueA"))
		db.Put(timeKey(time.Now().Add(time.Hour*100)), []byte("now time"))

		firstStart := len(allTimes) / 2

		first := allTimes[firstStart]

		key := first.Format("2006-01-02 15:04:05.999999999")
		t.Log("Start:", key)

		var count int
		iter := db.db.NewIterator(&util.Range{Start: timeKey(first), Limit: nil}, nil)
		for iter.Next() {
			t.Log(string(iter.Value()))
			count++
		}
		iter.Release()
		t.Log("count:", count, "firstStart:", firstStart, "need:", len(allTimes[firstStart:]))
	})

	t.Run("getLast", func(t *testing.T) {
		allTimes := createDateTimes(db)

		db.Put([]byte("keyA"), []byte("valueA"))
		db.Put(append(flag, []byte("keyB")...), []byte("valueB"))

		_ = allTimes
		iter := db.db.NewIterator(&util.Range{Start: flag, Limit: nil}, nil)
		if iter.Last() {
			t.Log("Last", string(iter.Value()))
		}
		for iter.Next() {

		}
		iter.Release()
	})
}

var flag = []byte("units_t_")

func timeKey(t time.Time) []byte {
	return append(flag, common.Uint64ToBytes(uint64(t.UnixNano()))...)
}

func createDateTimes(db Database) []time.Time {
	var allTimes []time.Time

	radome := rand.New(rand.NewSource(100))

	now := time.Now()
	for i := 0; i < 25; i++ {
		now = now.Add(time.Duration(radome.Int63n(int64(time.Second))))

		allTimes = append(allTimes, now)
		//t.Log(now.Format("2006-01-02 15:04:05.999999999"), now.UnixNano())
		db.Put(timeKey(now), []byte(now.Format("2006-01-02 15:04:05.999999999")))
	}
	return allTimes
}

func getTimes(db Database, start time.Time, count int) (list []time.Time) {
	//firstUnixNano := uint64(start.UnixNano())
	db.Iterator([]byte(start.Format("2006-01-02 15:04:05")), func(key, value []byte) bool {
		t, err := time.Parse("2006-01-02 15:04:05.999999999", string(key))
		if err != nil {
			panic(err)
		}
		if !t.After(start) {
			return true
		}
		list = append(list, t)
		if len(list) > count {
			return false
		}
		return true
	})
	if len(list) < count { //如果数量不足，还需要继续寻找
		start = time.Unix(start.Unix()+1, 0)
		list = append(list, getTimes(db, start, count-len(list))...)
	}
	return list
}
