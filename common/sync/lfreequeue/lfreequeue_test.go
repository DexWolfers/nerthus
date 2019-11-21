package lfreequeue

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

const dataLen = 10000

var q *Queue
var data []string
var mdata map[string]bool

func isIn(data []string, datum string) (found bool) {
	for _, d := range data {
		if d == datum {
			found = true
			return
		}
	}

	return
}

func TestInit(t *testing.T) {
	mdata = make(map[string]bool)
}

func TestSingleGoroutine(t *testing.T) {
	q = NewQueue()

	v1 := "scryner"
	v2 := "skdul"

	q.Enqueue(v1)
	q.Enqueue(v2)

	w1, ok := q.Dequeue()
	if !ok || w1 != v1 {
		t.Errorf("not same: %v, %v", v1, w1)

	}

	w2, ok := q.Dequeue()
	if !ok || w2 != v2 {
		t.Errorf("not same: %v, %v", v2, w2)
	}
	if got := q.Len(); got != 0 {
		t.Errorf("not empty,got %d", got)
	}
}

func TestConcurrentWrite(t *testing.T) {

	for i := 0; i < dataLen; i++ {
		datum := fmt.Sprintf("data_%d", i)
		data = append(data, datum)
	}

	var wg sync.WaitGroup

	for i := 0; i < dataLen; i++ {
		datum := data[i]

		wg.Add(1)

		go func() {
			q.Enqueue(datum)
			wg.Done()
		}()
	}

	wg.Wait()
}

type result struct {
	retval string
}

func TestConcurrentRead(t *testing.T) {
	var wg sync.WaitGroup

	var results []*result
	for i := 0; i < dataLen; i++ {
		ret := new(result)
		results = append(results, ret)

		wg.Add(1)

		go func(ret *result) {
			datum, _ := q.Dequeue()
			datum2 := datum.(string)
			ret.retval = datum2

			wg.Done()
		}(ret)
	}

	wg.Wait()

	for i := 0; i < dataLen; i++ {
		ret := results[i]

		if !isIn(data, ret.retval) {
			t.Errorf("datum is not in data: %v", ret.retval)
			return
		}

		if _, ok := mdata[ret.retval]; ok {
			t.Errorf("redundant retrieval: %v", ret.retval)
			return
		}

		mdata[ret.retval] = true
	}

	_, ok := q.Dequeue()
	if ok {
		t.Errorf("queue must be empty")
		return
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	succEnq := make(chan int)
	succDeq := make(chan int)

	for i := 0; i < dataLen*2; i++ {
		go func(i int) {
			if i%2 == 0 {
				// enqueue
				datum := data[i/2]
				q.Enqueue(datum)
				succEnq <- 1

				if (i/2)+1 == dataLen {
					succEnq <- -1
				}
			} else {
				// dequeue
				_, ok := q.Dequeue()
				if ok {
					succDeq <- 1
				} else {
					succDeq <- 0
				}

				if (i/2)+1 == dataLen {
					succDeq <- -1
				}
			}
		}(i)
	}

	var enqSuccess, deqSuccess int
	var endEnq, endDeq bool

	for {
		if endEnq && endDeq {
			break
		}

		select {
		case i := <-succEnq:
			if i < 0 {
				endEnq = true
			} else {
				enqSuccess += i
			}
		case i := <-succDeq:
			if i < 0 {
				endDeq = true
			} else {
				deqSuccess += i
			}
		}
	}

	if enqSuccess != dataLen {
		t.Errorf("some %d enqueing operations is wrong", dataLen-enqSuccess)
		return
	}

	retry := dataLen - deqSuccess

	for i := 0; i < retry; i++ {
		_, ok := q.Dequeue()
		if !ok {
			t.Errorf("retry dequeue failed: total %d retry but, dequeue has %d", retry, i)
			return
		}
	}

	if _, ok := q.Dequeue(); ok {
		t.Errorf("queue must be empty")
		return
	}
}

type Record struct {
	v     int64
	vs    string
	other [32]byte
}

func BenchmarkQueue_Dequeue(b *testing.B) {
	b.Run("w5r1", func(b *testing.B) {
		q := NewQueue()
		b.RunParallel(func(pb *testing.PB) {
			var index int
			for pb.Next() {
				if index%5 == 0 {
					q.Dequeue()
				} else {
					q.Enqueue(index)
				}
				index++
			}
		})
	})
	b.Run("w1r1", func(b *testing.B) {
		q := NewQueue()
		b.RunParallel(func(pb *testing.PB) {
			var index int
			for pb.Next() {
				if index%2 == 0 {
					q.Dequeue()
				} else {
					q.Enqueue(index)
				}
				index++
			}
		})
	})
}

func TestRead(t *testing.T) {
	c := 100000
	q := NewQueue()
	rr := make([]int, 10)
	for index := 0; index < len(rr); index++ {
		go func(gid int) {
			for i := 0; i < c; i++ {
				q.Enqueue(i)
				rr[gid] += i
			}
		}(index)
	}

	s1 := 0
	var wg sync.WaitGroup
	wg.Add(c * len(rr))
	go func() {
		for {
			select {
			case v := <-q.DequeueChan():
				//t.Log(v)
				s1 += v.(int)
				wg.Done()
			}
		}
	}()
	time.Sleep(time.Second)
	wg.Wait()
	r1 := 0
	for i := range rr {
		r1 += rr[i]
	}
	if r1 != s1 {
		t.Log("r1", r1, "s1", s1)
		t.Fail()
	}
}
