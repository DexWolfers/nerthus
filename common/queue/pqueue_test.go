package queue

import (
	"math/rand"
	"testing"

	"sync"

	"github.com/stretchr/testify/assert"
)

func TestPriorityPut(t *testing.T) {
	q := NewPriorityQueue(1, false)

	q.Push(mockItem(2))

	assert.Len(t, q.items, 1)
	assert.Equal(t, mockItem(2), q.items[0])

	q.Push(mockItem(1))

	if !assert.Len(t, q.items, 2) {
		return
	}
	assert.Equal(t, mockItem(2), q.items[0])
	assert.Equal(t, mockItem(1), q.items[1])
}

func TestPriorityPut3(t *testing.T) {
	q := NewPriorityQueue(1, false)

	inputs := []Item{
		mockItem(20),
		mockItem(10),
		mockItem(50),
		mockItem(40),
	}
	for _, v := range inputs {
		q.Push(v.(mockItem))
	}
	assert.Equal(t, mockItem(50), q.Peek())
	items, _ := q.MustPop(5)
	assert.Equal(t, items, []Item{mockItem(50), mockItem(40), mockItem(20), mockItem(10)})
}

func TestPriorityGet(t *testing.T) {
	q := NewPriorityQueue(1, false)

	q.Push(mockItem(2))
	result, err := q.MustPop(2)
	if !assert.Nil(t, err) {
		return
	}

	if !assert.Len(t, result, 1) {
		return
	}

	assert.Equal(t, mockItem(2), result[0])
	assert.Len(t, q.items, 0)

	q.Push(mockItem(2))
	q.Push(mockItem(1))

	result, err = q.MustPop(1)
	if !assert.Nil(t, err) {
		return
	}

	if !assert.Len(t, result, 1) {
		return
	}

	assert.Equal(t, mockItem(2), result[0])
	assert.Len(t, q.items, 1)

	result, err = q.MustPop(2)
	if !assert.Nil(t, err) {
		return
	}

	if !assert.Len(t, result, 1) {
		return
	}

	assert.Equal(t, mockItem(1), result[0])
}

func TestAddEmptyPriorityPut(t *testing.T) {
	q := NewPriorityQueue(1, false)

	q.Push()

	assert.Len(t, q.items, 0)
}

func TestPriorityGetNonPositiveNumber(t *testing.T) {
	q := NewPriorityQueue(1, false)

	q.Push(mockItem(1))

	result, err := q.MustPop(0)
	if !assert.Nil(t, err) {
		return
	}

	assert.Len(t, result, 0)

	result, err = q.MustPop(-1)
	if !assert.Nil(t, err) {
		return
	}

	assert.Len(t, result, 0)
}

func TestPriorityEmpty(t *testing.T) {
	q := NewPriorityQueue(1, false)
	assert.True(t, q.Empty())

	q.Push(mockItem(1))

	assert.False(t, q.Empty())
}

func TestPriorityGetEmpty(t *testing.T) {
	q := NewPriorityQueue(1, false)

	go func() {
		q.Push(mockItem(1))
	}()

	result, err := q.MustPop(1)
	if !assert.Nil(t, err) {
		return
	}

	if !assert.Len(t, result, 1) {
		return
	}
	assert.Equal(t, mockItem(1), result[0])
}

func TestMultiplePriorityGetEmpty(t *testing.T) {
	q := NewPriorityQueue(1, false)
	var wg sync.WaitGroup
	wg.Add(1)
	results := make([][]Item, 2)

	go func() {
		local, _ := q.MustPop(1)
		results[0] = local

		local2, _ := q.MustPop(1)
		results[1] = local2
		wg.Done()
	}()

	q.Push(mockItem(1), mockItem(3), mockItem(2))
	wg.Wait()

	if !assert.Len(t, results[0], 1) || !assert.Len(t, results[1], 1) {
		return
	}

	//assert.True(
	//	t, (results[0][0] == mockItem(3) && results[1][0] == mockItem(2)) ||
	//		results[0][0] == mockItem(3) && results[1][0] == mockItem(2),
	//)
}

func TestEmptyPriorityGetWithDispose(t *testing.T) {
	q := NewPriorityQueue(1, false)
	var wg sync.WaitGroup
	wg.Add(1)

	var err error
	go func() {
		wg.Done()
		_, err = q.MustPop(1)
		wg.Done()
	}()

	wg.Wait()
	wg.Add(1)

	q.Dispose()

	wg.Wait()

	assert.IsType(t, ErrDisposed, err)
}

func TestPriorityGetPutDisposed(t *testing.T) {
	q := NewPriorityQueue(1, false)
	q.Dispose()

	_, err := q.MustPop(1)
	assert.IsType(t, ErrDisposed, err)

	err = q.Push(mockItem(1))
	assert.IsType(t, ErrDisposed, err)
}

func BenchmarkPriorityQueue(b *testing.B) {
	q := NewPriorityQueue(b.N, false)
	var wg sync.WaitGroup
	wg.Add(1)
	i := 0

	go func() {
		for {
			q.MustPop(1)
			i++
			if i == b.N {
				wg.Done()
				break
			}
		}
	}()

	for i := 0; i < b.N; i++ {
		q.Push(mockItem(i))
	}

	wg.Wait()
}

func TestPriorityPeek(t *testing.T) {
	q := NewPriorityQueue(1, false)
	q.Push(mockItem(1))

	assert.Equal(t, mockItem(1), q.Peek())
}

func TestInsertDuplicate(t *testing.T) {
	q := NewPriorityQueue(1, false)
	q.Push(mockItem(1))
	q.Push(mockItem(1))

	assert.Equal(t, 1, q.Len())
}

func TestAllowDuplicates(t *testing.T) {
	q := NewPriorityQueue(2, true)
	q.Push(mockItem(1))
	q.Push(mockItem(1))

	assert.Equal(t, 2, q.Len())
}

func BenchmarkNewPriorityQueue(b *testing.B) {

	b.Run("orderPush", func(b *testing.B) {
		q := NewPriorityQueue(20000, false)
		b.ReportAllocs()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var id int
			for pb.Next() {
				q.Push(mockItem(id))
				id++
			}
		})
	})
	b.Run("randPush", func(b *testing.B) {
		q := NewPriorityQueue(20000, false)
		b.ReportAllocs()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				q.Push(mockItem(rand.Int()))
			}
		})
	})

}
