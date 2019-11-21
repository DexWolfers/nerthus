// Author: @ysqi

package queue

import (
	"container/heap"
	"sync"
)

// Item is an item that can be added to the priority queue.
type Item interface {
	// Compare returns a bool that can be used to determine
	// ordering in the priority queue.  Assuming the queue
	// is in ascending order, this should return > logic.
	// Return 1 to indicate this object is greater than the
	// the other logic, 0 to indicate equality, and -1 to indicate
	// less than other.
	Compare(other Item) int
}

type PHeap []Item

func (p PHeap) Len() int {
	return len(p)
}
func (p PHeap) Less(i, j int) bool {
	// We want Pop to give us the highest, not lowest, priority so we use greater than here.
	return p[i].Compare(p[j]) > 0
}
func (p PHeap) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

func (p *PHeap) Push(x interface{}) {
	*p = append(*p, x.(Item))
}

func (h *PHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

//PriorityQueue is similar to queue except that it takes
//items that implement the Item interface and adds them
//to the queue in priority order.
type PriorityQueue struct {
	waiters         waiters
	items           PHeap
	itemMap         map[Item]struct{}
	lock            sync.Mutex
	disposeLock     sync.Mutex
	disposed        bool
	allowDuplicates bool
}

// Put adds items to the queue.
func (pq *PriorityQueue) Push(items ...Item) error {
	if len(items) == 0 {
		return nil
	}

	pq.lock.Lock()
	defer pq.lock.Unlock()

	if pq.disposed {
		return ErrDisposed
	}

	for _, item := range items {
		if pq.allowDuplicates {
			heap.Push(&pq.items, item)
		} else if _, ok := pq.itemMap[item]; !ok {
			pq.itemMap[item] = struct{}{}
			heap.Push(&pq.items, item)
		}
	}

	for {
		sema := pq.waiters.get()
		if sema == nil {
			break
		}

		sema.response.Add(1)
		sema.ready <- true
		sema.response.Wait()
		if len(pq.items) == 0 {
			break
		}
	}

	return nil
}

// 取出指定数量的item，不会弹出数据
func (pq *PriorityQueue) Get(number int) ([]Item, error) {
	// 输入检查
	if number < 1 {
		return nil, nil
	}
	if len(pq.items) == 0 {
		return nil, nil
	}
	pq.lock.Lock()
	defer pq.lock.Unlock()
	if number > len(pq.items) {
		return pq.items[:], nil
	}
	return pq.items[:number], nil
}

// Get retrieves items from the queue.
// This will attempt to retrieve [0,number] of items.
func (pq *PriorityQueue) Pop(number int) ([]Item, error) {
	return pq.popn(number, false)
}

// Get retrieves items from the queue.  If the queue is empty,
// this call blocks until the next item is added to the queue.  This
// will attempt to retrieve [1,number] of items.
func (pq *PriorityQueue) MustPop(number int) ([]Item, error) {
	return pq.popn(number, true)
}

func (pq *PriorityQueue) popn(number int, lessOne bool) ([]Item, error) {
	if number < 1 {
		return nil, nil
	}

	pq.lock.Lock()

	if pq.disposed {
		pq.lock.Unlock()
		return nil, ErrDisposed
	}

	var items []Item

	// Remove references to popped items.
	deleteItems := func(items []Item) {
		for _, item := range items {
			delete(pq.itemMap, item)
		}
	}

	if lessOne && len(pq.items) == 0 {
		sema := newSema()
		pq.waiters.put(sema)
		pq.lock.Unlock()

		<-sema.ready

		if pq.Disposed() {
			return nil, ErrDisposed
		}

		items = pq.pop(number)
		if !pq.allowDuplicates {
			deleteItems(items)
		}
		sema.response.Done()
		return items, nil
	}

	items = pq.pop(number)
	deleteItems(items)
	pq.lock.Unlock()
	return items, nil
}

func (pq *PriorityQueue) pop(number int) []Item {
	returnItems := make([]Item, 0, number)
	for i := 0; i < number; i++ {
		if pq.items.Len() == 0 {
			break
		}
		returnItems = append(returnItems, heap.Pop(&pq.items).(Item))
	}
	return returnItems
}

// Peek will look at the next item without removing it from the queue.
func (pq *PriorityQueue) Peek() Item {
	pq.lock.Lock()
	defer pq.lock.Unlock()
	if len(pq.items) > 0 {
		return pq.items[0]
	}
	return nil
}

// Empty returns a bool indicating if there are any items left
// in the queue.
func (pq *PriorityQueue) Empty() bool {
	var ok bool
	pq.lock.Lock()
	ok = pq.items.Len() == 0
	pq.lock.Unlock()
	return ok
}

// Len returns a number indicating how many items are in the queue.
func (pq *PriorityQueue) Len() int {
	pq.lock.Lock()
	defer pq.lock.Unlock()

	return len(pq.items)
}

// Disposed returns a bool indicating if this queue has been disposed.
func (pq *PriorityQueue) Disposed() bool {
	pq.disposeLock.Lock()
	defer pq.disposeLock.Unlock()

	return pq.disposed
}

// Dispose will prevent any further reads/writes to this queue
// and frees available resources.
func (pq *PriorityQueue) Dispose() {
	pq.lock.Lock()
	defer pq.lock.Unlock()

	pq.disposeLock.Lock()
	defer pq.disposeLock.Unlock()

	pq.disposed = true
	for _, waiter := range pq.waiters {
		waiter.response.Add(1)
		waiter.ready <- true
	}

	pq.items = nil
	pq.waiters = nil
}

// NewPriorityQueue is the constructor for a priority queue.
func NewPriorityQueue(hint int, allowDuplicates bool) *PriorityQueue {
	pq := &PriorityQueue{
		items:           make(PHeap, 0, hint),
		itemMap:         make(map[Item]struct{}, hint),
		allowDuplicates: allowDuplicates,
	}
	heap.Init(&pq.items)
	return pq
}
