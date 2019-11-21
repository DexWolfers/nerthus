package threadpool

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestNewPoolTask(t *testing.T) {
	pool := NewDynamicPool(100)

	pool.PushTask(func(i []interface{}) {
		fmt.Println(i[0], i[1])
	}, []interface{}{111, "2222"})
	time.Sleep(time.Second)
}

func BenchmarkNewPoolTask(b *testing.B) {
	//b.N = 1000
	b.Run("100", func(b *testing.B) {
		pool := NewDynamicPool(100)
		var wg sync.WaitGroup
		for i := 0; i < b.N; i++ {
			wg.Add(1)
			pool.PushTask(func(i []interface{}) {
				//b.Log("o => ", i[0], "len", len(i), "type", reflect.ValueOf(i[0]).Type())
				time.Sleep(time.Millisecond * 10)
				wg.Done()
			}, []interface{}{i})
		}
		wg.Wait()
	})
	b.Run("1000", func(b *testing.B) {
		pool := NewDynamicPool(1000)
		var wg sync.WaitGroup
		for i := 0; i < b.N; i++ {
			wg.Add(1)
			pool.PushTask(func(i []interface{}) {
				//b.Log("o => ", i[0], "len", len(i), "type", reflect.ValueOf(i[0]).Type())
				time.Sleep(time.Millisecond * 10)
				wg.Done()
			}, []interface{}{i})
		}
		wg.Wait()
	})
	b.Run("10000", func(b *testing.B) {
		pool := NewDynamicPool(10000)
		var wg sync.WaitGroup
		for i := 0; i < b.N; i++ {
			wg.Add(1)
			pool.PushTask(func(i []interface{}) {
				//b.Log("o => ", i[0], "len", len(i), "type", reflect.ValueOf(i[0]).Type())
				time.Sleep(time.Millisecond * 10)
				wg.Done()
			}, []interface{}{i})
		}
		wg.Wait()
	})
	b.Run("100000", func(b *testing.B) {
		pool := NewDynamicPool(100000)
		var wg sync.WaitGroup
		for i := 0; i < b.N; i++ {
			wg.Add(1)
			pool.PushTask(func(i []interface{}) {
				//b.Log("o => ", i[0], "len", len(i), "type", reflect.ValueOf(i[0]).Type())
				time.Sleep(time.Millisecond * 10)
				wg.Done()
			}, []interface{}{i})
		}
		wg.Wait()
	})
}

func TestNewDynamicPool(t *testing.T) {
	pool := NewDynamicPool(100)
	var wg sync.WaitGroup
	for i := 0; i < 100000; i++ {
		wg.Add(1)
		pool.PushTask(func(i []interface{}) {
			time.Sleep(time.Millisecond * 10)
			wg.Done()
		}, []interface{}{})
	}
	fmt.Println("push over", time.Now())
	wg.Wait()
	fmt.Println("exec over", time.Now())
}
