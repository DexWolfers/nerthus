package sortslice

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestList_Add(t *testing.T) {
	list := NewList(10)

	items := []int{5, 4, 2}

	for _, v := range items {
		ok := list.Add(mockItem(v))
		require.True(t, ok)
	}
	require.Equal(t, len(items), list.Len())

	//不允许重复
	t.Run("ignoreRepeat", func(t *testing.T) {
		for _, v := range items {
			ok := list.Add(mockItem(v))
			require.False(t, ok)
		}
	})
}

func TestList_Pop(t *testing.T) {
	list := NewList(10)

	items := []int{5, 4, 2, 6, 10}
	for _, v := range items {
		list.Add(mockItem(v))
	}
	require.Equal(t, mockItem(10), list.Pop())
	require.Equal(t, mockItem(6), list.Pop())
	require.Equal(t, mockItem(5), list.Pop())
	require.Equal(t, mockItem(4), list.Pop())
	require.Equal(t, mockItem(2), list.Pop())

	require.True(t, list.Empty())
	require.Nil(t, list.Pop(), "should be nil when is empty")
}

func TestList_Remove(t *testing.T) {
	list := NewList(10)
	items := []int{5, 4, 2, 6, 10}
	for _, v := range items {
		list.Add(mockItem(v))
	}

	removeOrders := []struct {
		remove int
		top    int
	}{
		{5, 10},
		{10, 6},
		{2, 6},
		{6, 4},
	}

	for _, c := range removeOrders {
		ok := list.Remove(fmt.Sprintf("key-%d", c.remove))
		require.True(t, ok)
		require.Equal(t, mockItem(c.top), list.Get(0))
	}
}

func TestList_Penter(t *testing.T) {
	list := NewList(10)

	randV := func() int {
		return rand.Intn(10000)
	}
	randItem := func() mockItem {
		return mockItem(randV())
	}
	//随机写、读、删
	var (
		wg   sync.WaitGroup
		quit = make(chan struct{})
		add  int
		get  int
		pop  int
	)

	wg.Add(1000)

	//写
	go func() {
		for {
			select {
			case <-quit:
				return
			default:
				if list.Add(randItem()) {
					add++
				}
			}
		}
	}()
	//读
	go func() {
		for {
			select {
			case <-quit:
				return
			default:
				if list.Get(randV()) != nil {
					get++
				}
			}
		}
	}()
	//删
	go func() {
		var removed int
		for {
			select {
			case <-quit:
				return
			default:
				ok := list.Remove(randItem().Key())
				if ok {
					removed++
					wg.Done()
				}
			}
		}
	}()
	//取
	go func() {
		for {
			select {
			case <-quit:
				return
			default:
				if list.Pop() != nil {
					pop++
				}
			}
		}

	}()

	wg.Wait()
	close(quit)

	t.Logf("add:%d,get:%d,pop:%d", add, get, pop)
}

func BenchmarkList_Pop(b *testing.B) {
	list := NewList(1000)
	for i := 0; i < b.N; i++ {
		list.Add(mockItem(rand.Intn(b.N)))
	}

	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		wait.Done()
		for i := 0; i < b.N; i++ {
			list.Add(mockItem(rand.Intn(b.N)))
		}
	}()
	wait.Wait()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list.Pop()
	}
}
