package core

import (
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common/sync/cmap"
	"gitee.com/nerthus/nerthus/common/utils"

	"github.com/bluele/gcache"
	"github.com/hashicorp/golang-lru"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/ntsdb"
	"github.com/stretchr/testify/require"
)

func TestGetChainTail(t *testing.T) {
	db, _ := ntsdb.NewMemDatabase()
	gspec, err := LoadGenesisConfig()
	require.NoError(t, err)
	gspec.MustCommit(db)

	dagchain, _ := NewDagChain(db, gspec.Config, nil, vm.Config{})
	defer dagchain.Stop()

	addr := common.StringToAddress("addr1")
	unit := types.NewBlockWithHeader(&types.Header{
		MC:         addr,
		Number:     1,
		ParentHash: types.EmptyParentHash,
		SCHash:     types.EmptyParentHash,
	}).WithBody(nil, []types.WitenssVote{}, types.SignContent{})

	//state, err := dagchain.GetChainTailState(addr)
	//require.NoError(t, err)

	//存储之前均对应创世信息
	hash := dagchain.GetChainTailHash(addr)
	require.Equal(t, dagchain.GenesisHash(), hash, "should be equal genesis hash")
	head := dagchain.GetChainTailHead(addr)
	require.Equal(t, dagchain.GenesisHash(), head.Hash(), "should be equal genesis hash")
	number := dagchain.GetChainTailNumber(addr)
	require.Equal(t, uint64(0), number, "should be equal genesis number")

	//存储
	//err = dagchain.writeRightUnit(nil, time.Now(), state, unit, nil, nil)
	//require.NoError(t, err)

	//写入数据后，检查和一封信
	hash = dagchain.GetChainTailHash(addr)
	require.Equal(t, unit.Hash(), hash, "should be equal last write")
	head = dagchain.GetChainTailHead(addr)
	require.Equal(t, unit.Hash(), hash, "should be equal last write")
	number = dagchain.GetChainTailNumber(addr)
	require.Equal(t, unit.Number(), number, "should be equal last write")
}

func TestDagChain_ChainLocker(t *testing.T) {
	dc := new(DagChain)
	dc.chainRW = cmap.New()

	c1 := common.StringToAddress("chain1")
	c2 := common.StringToAddress("chain2")

	//检查锁是否独立
	l1 := dc.chainLocker(c1)
	l2 := dc.chainLocker(c2)

	require.NotEqual(t, fmt.Sprintf("%p", l1), fmt.Sprintf("%p", l2))

	//再次获取锁，依旧是原锁
	l11 := dc.chainLocker(c1)
	l21 := dc.chainLocker(c2)

	require.Equal(t, fmt.Sprintf("%p", l1), fmt.Sprintf("%p", l11))
	require.Equal(t, fmt.Sprintf("%p", l2), fmt.Sprintf("%p", l21))
}

func BenchmarkCache_LRU(b *testing.B) {

	run := func(b *testing.B, count int) {
		cache, _ := lru.New(2048 * 2)
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(count)
		for i := 0; i < count; i++ {
			go func() {
				<-start
				for i := 0; i < b.N; i++ {
					cache.Get(common.StringToAddress(strconv.Itoa(i)).Hex())
				}
				wg.Done()
			}()
		}

		b.ResetTimer()
		close(start)

		//10倍的读取
		// 并发写
		for i := 0; i < b.N; i++ {
			cache.Add(common.StringToAddress(strconv.Itoa(i)).Hex(), struct{}{})
		}
		wg.Wait()
	}
	b.Run("0", func(b *testing.B) {
		run(b, 0)
	})
	b.Run("200", func(b *testing.B) {
		run(b, 200)
	})
	b.Run("2000", func(b *testing.B) {
		run(b, 200)
	})
	b.Run("3000", func(b *testing.B) {
		run(b, 3000)
	})
}

func BenchmarkCache_GCACHE_LFU(b *testing.B) {
	cache := gcache.New(2048 * 2).LFU().Build()
	bench(b, cache)
}

func BenchmarkCache_GCACHE_LRU(b *testing.B) {
	cache := gcache.New(2048 * 2).LRU().Build()
	bench(b, cache)
}

func BenchmarkCache_GCACHE_Expiration(b *testing.B) {
	cache := gcache.New(2048 * 2).Expiration(time.Minute).Build()
	bench(b, cache)
}

func bench(b *testing.B, cache gcache.Cache) {

	run := func(b *testing.B, count int) {

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(count)
		for i := 0; i < count; i++ {
			go func() {
				<-start
				for i := 0; i < b.N; i++ {
					cache.Get(common.StringToAddress(strconv.Itoa(i)).Hex())
				}
				wg.Done()
			}()
		}

		b.ResetTimer()
		close(start)

		//10倍的读取
		// 并发写
		for i := 0; i < b.N; i++ {
			cache.Set(common.StringToAddress(strconv.Itoa(i)).Hex(), struct{}{})
		}
		wg.Wait()
	}
	b.Run("0", func(b *testing.B) {
		run(b, 0)
	})
	b.Run("200", func(b *testing.B) {
		run(b, 200)
	})
	b.Run("2000", func(b *testing.B) {
		run(b, 200)
	})
	b.Run("3000", func(b *testing.B) {
		run(b, 3000)
	})
}

func BenchmarkCache_CMap(b *testing.B) {

	run := func(b *testing.B, count int) {
		cache := cmap.New()
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(count)
		for i := 0; i < count; i++ {
			go func() {
				<-start
				for i := 0; i < b.N; i++ {
					cache.Get(common.StringToAddress(strconv.Itoa(i)).Hex())
				}
				wg.Done()
			}()
		}

		b.ResetTimer()
		close(start)

		//10倍的读取
		// 并发写
		for i := 0; i < b.N; i++ {
			cache.Set(common.StringToAddress(strconv.Itoa(i)).Hex(), struct{}{})
		}
		wg.Wait()
	}
	b.Run("0", func(b *testing.B) {
		run(b, 0)
	})
	b.Run("200", func(b *testing.B) {
		run(b, 200)
	})
	b.Run("2000", func(b *testing.B) {
		run(b, 200)
	})
	b.Run("3000", func(b *testing.B) {
		run(b, 3000)
	})
}

func BenchmarkSyncMap(b *testing.B) {

	run := func(b *testing.B, count int) {
		cache := sync.Map{}
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(count)
		for i := 0; i < count; i++ {
			go func() {
				<-start
				for i := 0; i < b.N; i++ {
					cache.Load(common.StringToAddress(strconv.Itoa(i)).Hex())
				}
				wg.Done()
			}()
		}

		b.ResetTimer()
		close(start)

		//10倍的读取
		// 并发写
		for i := 0; i < b.N; i++ {
			cache.Store(common.StringToAddress(strconv.Itoa(i)).Hex(), struct{}{})
		}
		wg.Wait()
	}
	b.Run("0", func(b *testing.B) {
		run(b, 0)
	})
	b.Run("200", func(b *testing.B) {
		run(b, 200)
	})
	b.Run("2000", func(b *testing.B) {
		run(b, 200)
	})
	b.Run("3000", func(b *testing.B) {
		run(b, 3000)
	})
}

type MyCache struct {
	data  map[string]interface{}
	keys  []string
	mut   sync.RWMutex
	limit int
}

func (c *MyCache) Get(key string) interface{} {
	c.mut.RLock()
	v := c.data[key]
	c.mut.RUnlock()
	return v
}

func (c *MyCache) Set(key string, value interface{}) {
	c.mut.Lock()
	c.data[key] = value
	c.keys = append(c.keys, key)
	if c.limit < len(c.keys) {
		delete(c.data, c.keys[0])
		c.keys = c.keys[1:]
	}
	c.mut.Unlock()
}

func BenchmarkCache_MySelft(b *testing.B) {

	run := func(b *testing.B, count int) {
		cache := MyCache{
			data:  make(map[string]interface{}, 2*2048),
			limit: 2 * 2048,
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(count)
		for i := 0; i < count; i++ {
			go func() {
				<-start
				for i := 0; i < b.N; i++ {
					cache.Get(common.StringToAddress(strconv.Itoa(i)).Hex())
				}
				wg.Done()
			}()
		}

		b.ResetTimer()
		close(start)

		//10倍的读取
		// 并发写
		for i := 0; i < b.N; i++ {
			cache.Set(common.StringToAddress(strconv.Itoa(i)).Hex(), struct{}{})
		}
		wg.Wait()
	}
	b.Run("0", func(b *testing.B) {
		run(b, 0)
	})
	b.Run("200", func(b *testing.B) {
		run(b, 200)
	})
	b.Run("2000", func(b *testing.B) {
		run(b, 200)
	})
	b.Run("3000", func(b *testing.B) {
		run(b, 3000)
	})
}

func BenchmarkUnitValidator_ValidateState(b *testing.B) {

	var wg sync.WaitGroup
	wg.Add(b.N)
	b.ResetTimer()

	var lock sync.RWMutex

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			lock.RLock()
			wg.Add(1)
			lock.RUnlock()
		}

	})
}

func TestDagChain_checkWritenUnits(t *testing.T) {
	db, _ := ntsdb.NewMemDatabase()
	gspec, err := LoadGenesisConfig()
	require.NoError(t, err)
	gspec.MustCommit(db)

	dagchain, _ := NewDagChain(db, gspec.Config, nil, vm.Config{})
	dagchain.Stop()

	require.True(t, dagchain.writenNotOverloaded())

	dagchain.writtenInUnitLimit = 1
	require.True(t, dagchain.writenNotOverloaded())

	dagchain.goodUnitq.Enqueue(struct{}{})
	dagchain.goodUnitq.Enqueue(struct{}{})

	require.False(t, dagchain.writenNotOverloaded())

	var status int
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		//需要等待到取出数据
		dagchain.checkWritenUnits()
		require.Equal(t, 1, status)
		wg.Done()
	}()

	dagchain.goodUnitq.Dequeue()
	status = 1
	dagchain.goodUnitq.Dequeue()
	wg.Wait()
}

func TestDagchain_SaveUnit(t *testing.T) {
	db, _ := ntsdb.NewMemDatabase()
	gspec, err := LoadGenesisConfig()
	require.NoError(t, err)
	gspec.MustCommit(db)

	dagchain, _ := NewDagChain(db, gspec.Config, nil, vm.Config{})
	defer dagchain.Stop()

	gs, count := 10, 100
	var wg sync.WaitGroup
	wg.Add(gs * count * 2) //读完、写完
	dagchain.writtenInUnitLimit = 25

	dagchain.testPopUnit = func(v interface{}) {
		wg.Done()
	}

	for i := 0; i < gs; i++ {
		go func() {
			for i := 0; i < count; i++ {
				dagchain.checkWritenUnits()
				dagchain.goodUnitq.Enqueue(i)
				wg.Done()
			}
		}()
	}
	wg.Wait()
}

func BenchmarkChainLocker(b *testing.B) {

	dc := DagChain{chainRW: cmap.NewWith(1000)}

	chains := make([]common.Address, 500)

	for i := 0; i < len(chains); i++ {
		chains[i] = common.BytesToAddress(utils.RandomUUIDHash().Bytes())
	}

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		var i int
		for pb.Next() {
			locker := dc.chainLocker(chains[i%len(chains)])
			locker.Lock()
			locker.Unlock()
			i++
		}
	})
}
