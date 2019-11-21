// 一个高性能队列
package lfreequeue

import (
	"sync/atomic"
	"unsafe"
)

// private structure
type node struct {
	value interface{}
	next  *node
}

type queue struct {
	size  int64
	dummy *node
	tail  *node
}

func newQueue() *queue {
	q := new(queue)
	q.dummy = new(node)
	q.tail = q.dummy

	return q
}

func (q *queue) enqueue(v interface{}) {
	var oldTail, oldTailNext *node

	newNode := new(node)
	newNode.value = v

	newNodeAdded := false

	for !newNodeAdded {
		oldTail = q.tail
		oldTailNext = oldTail.next

		if q.tail != oldTail {
			continue
		}

		if oldTailNext != nil {
			atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail)), unsafe.Pointer(oldTail), unsafe.Pointer(oldTailNext))
			continue
		}

		newNodeAdded = atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&oldTail.next)), unsafe.Pointer(oldTailNext), unsafe.Pointer(newNode))
	}
	atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail)), unsafe.Pointer(oldTail), unsafe.Pointer(newNode))
	atomic.AddInt64(&q.size, 1)
}

func (q *queue) dequeue() (interface{}, bool) {
	var temp interface{}
	var oldDummy, oldHead *node

	removed := false

	for !removed {
		oldDummy = q.dummy
		oldHead = oldDummy.next
		oldTail := q.tail

		if q.dummy != oldDummy {
			continue
		}

		if oldHead == nil {
			return nil, false
		}

		if oldTail == oldDummy {
			atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail)), unsafe.Pointer(oldTail), unsafe.Pointer(oldHead))
			continue
		}

		temp = oldHead.value
		removed = atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.dummy)), unsafe.Pointer(oldDummy), unsafe.Pointer(oldHead))
	}
	atomic.AddInt64(&q.size, -1)
	return temp, true
}

// Public structure
type Queue struct {
	q     *queue
	watch chan interface{}
}

func NewQueue() *Queue {
	return &Queue{
		q:     newQueue(),
		watch: make(chan interface{}, 1),
	}
}

func (q *Queue) Enqueue(v interface{}) {
	if q.seq() > 0 {
		q.q.enqueue(v)
		return
	}
	select {
	case q.watch <- v:
	default:
		q.q.enqueue(v)
		select {
		case q.watch <- v:
			q.q.dequeue()
		default:
		}
	}
}

func (q *Queue) Dequeue() (interface{}, bool) {
	select {
	case e, ok := <-q.watch:
		return e, ok
	default:
		return q.q.dequeue()
	}
}

func (q *Queue) Clear() {
	for {
		_, ok := q.Dequeue()
		if !ok {
			return
		}
	}
}
func (q *Queue) seq() int64 {
	return atomic.LoadInt64(&q.q.size)
}
func (q *Queue) Len() int64 {
	return q.seq() + int64(len(q.watch))
}

func (q *Queue) DequeueChan() <-chan interface{} {
	if len(q.watch) == 0 {
		if v, ok := q.Dequeue(); ok {
			select {
			case q.watch <- v:
			default:
				//不能堵塞外部，并将已取出数据再次
				go func() {
					q.watch <- v
				}()
			}
		}
	}
	return q.watch
}
