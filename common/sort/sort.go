package sort

import "sort"

func QuicklySort(s SortSlice, cmpFn CmpFn) {
	s.cmpFn = cmpFn
	sort.Sort(s)
	s.cmpFn = nil
}

type CmpFn = func(x, y interface{}) bool

type SortSlice struct {
	Items []interface{}
	cmpFn func(x, y interface{}) bool
}

func (ss SortSlice) Len() int { return len(ss.Items) }

func (ss SortSlice) Swap(x, y int) {
	ss.Items[x], ss.Items[y] = ss.Items[y], ss.Items[x]
}

func (ss SortSlice) Less(x, y int) bool {
	if ss.cmpFn == nil {
		panic("ss.fn is nil")
	}
	return ss.cmpFn(ss.Items[x], ss.Items[y])
}
