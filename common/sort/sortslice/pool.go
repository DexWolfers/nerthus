package sortslice

import "sync"

var defPool = sync.Pool{
	New: func() interface{} {
		return NewList(0)
	},
}

func Get() *List { return defPool.Get().(*List) }
func Put(list *List) {
	list.Release()
	defPool.Put(list)
}
