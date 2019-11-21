package sortslice

import (
	"container/heap"
	"sort"
	"sync"
)

type Item interface {
	Compare(other Item) int
	Key() interface{}
}

type ItemsHeap []Item

func (p ItemsHeap) Len() int {
	return len(p)
}
func (p ItemsHeap) Less(i, j int) bool {
	// We want Pop to give us the highest, not lowest, priority so we use greater than here.
	return p[i].Compare(p[j]) > 0
}
func (p ItemsHeap) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

func (p *ItemsHeap) Push(x interface{}) {
	*p = append(*p, x.(Item))
}
func (p ItemsHeap) Get(number int) []Item {
	if number > p.Len() {
		return p[:]
	}
	return p[:number]
}

func (h *ItemsHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

type List struct {
	items    ItemsHeap
	itemKeys map[interface{}]struct{}
	lock     sync.RWMutex
}

func (l *List) Add(item Item) bool {
	l.lock.Lock()
	defer l.lock.Unlock()

	k := item.Key()
	if _, ok := l.itemKeys[k]; ok {
		return false
	}
	l.itemKeys[k] = struct{}{}
	heap.Push(&l.items, item)
	return true
}
func (l *List) Get(index int) Item {
	l.lock.RLock()
	defer l.lock.RUnlock()

	if l.items.Len()-1 < index {
		return nil
	}
	return l.items[index]
}

func (l *List) List() []Item {
	if l.items.Len() == 0 {
		return nil
	}
	l.lock.Lock()
	defer l.lock.Unlock()

	cpy := make([]Item, l.items.Len())
	copy(cpy, l.items)
	return cpy
}
func (l *List) Range(f func(item Item) bool) {
	l.lock.Lock()
	defer l.lock.Unlock()
	for i := 0; i < l.items.Len(); i++ {
		if !f(l.items[i]) {
			break
		}
	}
}
func (l *List) Sort() {
	l.lock.Lock()
	defer l.lock.Unlock()
	sort.Sort(l.items)
}

// 有序遍历
func (l *List) SortRange(f func(item Item) bool) {
	l.lock.Lock()
	defer l.lock.Unlock()

	if l.items.Len() == 0 {
		return
	}
	//因为第一个已经进行排序，因此在排序访问时，先直接处理第一个元素。
	if !f(l.items[0]) {
		return
	}
	//如果还需要更多，则排序再返回
	sort.Sort(l.items)
	for i := 1; i < l.items.Len(); i++ {
		if !f(l.items[i]) {
			break
		}
	}
}

func (l *List) Pop() Item {
	if l.items.Len() == 0 {
		return nil
	}
	l.lock.Lock()
	defer l.lock.Unlock()
	if l.items.Len() == 0 {
		return nil
	}

	return l.pop()
}

func (l *List) pop() Item {
	top := heap.Pop(&l.items)
	delete(l.itemKeys, top.(Item).Key())
	return top.(Item)
}
func (l *List) Exist(key interface{}) bool {
	if l.items.Len() == 0 {
		return false
	}
	l.lock.RLock()
	_, ok := l.itemKeys[key]
	l.lock.RUnlock()
	return ok
}
func (l *List) Remove(key interface{}) bool {
	if l.items.Len() == 0 {
		return false
	}
	l.lock.Lock()
	defer l.lock.Unlock()
	if l.items.Len() == 0 {
		return false
	}

	if _, ok := l.itemKeys[key]; !ok {
		return false
	}

	//遍历删除
	for i := 0; i < l.items.Len(); i++ {
		if l.items[i].Key() == key {
			l.remove(i)
			return true
		}
	}
	return false
}

//移除指定位置的项，这里是根据情况采用高效算法。
func (l *List) remove(index int) {
	//find,如果刚好是在第一个，则直接取出
	if index == 0 {
		l.pop()
		return
	}
	//如果不是末尾单元，则将最后一个单元和当前位置交换
	//且不需要重新排序
	lastIndex := l.items.Len() - 1
	if index != lastIndex {
		l.items.Swap(index, lastIndex)
	}
	//此时需要删除的项，在末尾
	item := l.items[lastIndex]
	delete(l.itemKeys, item.Key())
	l.items[lastIndex] = nil      //释放
	l.items = l.items[:lastIndex] //截断掉最后一项
}

func (l *List) Len() int {
	l.lock.RLock()
	defer l.lock.RUnlock()
	return l.items.Len()
}

func (l *List) Empty() bool {
	return l.Len() == 0
}

func (l *List) Release() {
	l.lock.Lock()
	defer l.lock.Unlock()
	for i := 0; i < l.items.Len(); i++ {
		l.items[i] = nil
	}
	for k := range l.itemKeys {
		delete(l.itemKeys, k)
	}
	l.items = l.items[:0]
}

func NewList(hint int) *List {
	list := List{
		items:    make(ItemsHeap, 0, hint),
		itemKeys: make(map[interface{}]struct{}, hint),
	}
	heap.Init(&list.items)
	return &list
}
