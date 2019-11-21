package threadpool

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewBaseTreadPoll(t *testing.T) {
	n := int32(0)
	fn := func(ctx context.Context, arg []interface{}) {
		if err := ctx.Err(); err != nil {
			fmt.Println(err)
		}
		atomic.AddInt32(&n, 1)
	}

	tp := NewBaseTreadPoll(100, 10)
	startOk := <-tp.Start()
	require.Nil(t, nil, startOk)
	fmt.Println("Start work")
	for i := 0; i < 100; i++ {
		require.Nil(t, tp.PushTask(fn, []interface{}{"hello word"}))
	}

	time.Sleep(time.Second)
	tp.Stop()

	require.Equal(t, n, int32(100))
	require.NotNil(t, tp.PushTask(fn, []interface{}{"hello word"}))
}

func BenchmarkNewBaseTreadPoll(b *testing.B) {
	b.Run("threadPool", func(b *testing.B) {
		tp := NewBaseTreadPoll(1, 1000)
		<-tp.Start()
		var wg sync.WaitGroup
		for i := 0; i < b.N; i++ {
			wg.Add(1)
			tp.PushTask(func(ctx context.Context, args []interface{}) {
				time.Sleep(time.Millisecond * 10)
				wg.Done()
			}, []interface{}{})
		}
		wg.Wait()
	})
	b.Run("threadTask", func(b *testing.B) {
		pool := NewDynamicPool(1000)
		var wg sync.WaitGroup
		for i := 0; i < b.N; i++ {
			wg.Add(1)
			pool.PushTask(func(i []interface{}) {
				time.Sleep(time.Millisecond * 10)
				wg.Done()
			}, []interface{}{})
		}
		wg.Wait()
		//fmt.Println("len worker", len(pool.workers))
	})
}
