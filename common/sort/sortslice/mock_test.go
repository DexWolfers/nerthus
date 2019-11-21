package sortslice

import "fmt"

type mockItem int

func (m mockItem) Key() interface{} {
	return fmt.Sprintf("key-%d", m)
}

func (mi mockItem) Compare(other Item) int {
	omi := other.(mockItem)
	if mi > omi {
		return 1
	} else if mi == omi {
		return 0
	}
	return -1
}
