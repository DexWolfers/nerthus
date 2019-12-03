package ntsdb

import (
	"math/big"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkByteToString(b *testing.B) {

	value := func() []byte {
		return make([]byte, 32)
	}
	var str string
	b.Run("custom", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			str = bytesToString(value())
		}
	})
	b.Run("warp", func(b *testing.B) {
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			str = string(value())
		}
	})
	b.Log(str)
}

func TestStringToBytes(t *testing.T) {
	keys := []string{
		"1234",
		"4567",
		"99e01F28D7d85aFEbFcBa5Ef2dfB45136060Cfcf",
		"11111111111111111111111111111111111111111111111111111",
		"",
	}
	for _, v := range keys {
		data := stringToBytes(v)
		require.Equal(t, v, bytesToString(data))
	}

}

func TestCacheBatch_DB(t *testing.T) {

	s := NewSuite(t, "cachedb", func(t *testing.T) (Database, func()) {
		db, err := NewMemDatabase()
		require.NoError(t, err)

		cdb, err := NewInMemoryDatabase(db)
		require.NoError(t, err)

		return cdb, func() {
			db.Close()
			cdb.Close()
		}
	})

	s.Run()
}

func TestCacheBatch_Put(t *testing.T) {
	db, err := NewMemDatabase()
	require.NoError(t, err)

	cdb, err := NewInMemoryDatabase(db)
	require.NoError(t, err)

	type Item struct {
		Key, Value []byte
	}

	values := []Item{
		{[]byte("k1"), []byte("v1")},
		{[]byte("k2"), make([]byte, 1, 12*1024)},
		{[]byte("k3"), make([]byte, 3)},
		{[]byte("k4"), []byte("0x123456789abcdefabcedfgood")},
		{[]byte("k5"), new(big.Int).SetUint64(1 << 63).Bytes()},
	}

	var got sync.Map
	var wg sync.WaitGroup
	wg.Add(len(values))
	cdb.onStore = func(key, value []byte) {
		_, loaded := got.LoadOrStore(string(key), Item{Key: key, Value: value})
		require.False(t, loaded, "should be store one times")

		// can find in db
		v, err := db.Get(key)
		require.NoError(t, err)
		require.Equal(t, value, v)

		wg.Done()
	}
	for _, v := range values {
		cdb.Put(v.Key, v.Value)
	}

	wg.Wait()
	for _, v := range values {
		dat, loaded := got.Load(string(v.Key))
		require.True(t, loaded, "should be find key=%s", string(v.Key))
		require.Equal(t, v.Value, dat.(Item).Value, "should be same value")
	}

}
