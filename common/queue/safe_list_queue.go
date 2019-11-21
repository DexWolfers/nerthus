package queue

import (
	"container/list"
	"sync"
)

type ListQueue struct {
	lock      *sync.Mutex
	inner     *list.List
	discardFn func(element *list.Element) bool
}

func NewListQueue(fn func(element *list.Element) bool) *ListQueue {
	return &ListQueue{
		lock:      &sync.Mutex{},
		inner:     list.New(),
		discardFn: fn,
	}
}

func (listQ *ListQueue) Pop() *list.Element {
	listQ.lock.Lock()
	var element *list.Element
	for {
		// TODO Opz `Do front and remove together`, avoid `do remove`
		element = listQ.inner.Front()
		if element == nil {
			break
		}
		listQ.inner.Remove(element)
		if listQ.discardFn(element) {
			continue
		}
		break
	}
	listQ.lock.Unlock()
	return element
}

func (listQ *ListQueue) PushBack(v interface{}) *list.Element {
	listQ.lock.Lock()
	var element = listQ.inner.PushBack(v)
	listQ.lock.Unlock()
	return element
}
