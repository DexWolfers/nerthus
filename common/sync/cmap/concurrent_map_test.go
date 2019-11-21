package cmap

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

func BenchmarkSet(b *testing.B) {

	for i := 1; i < 10; i++ {

		s := i * 32
		b.Run("s"+strconv.Itoa(s), func(b *testing.B) {
			m := NewWith(s)
			b.RunParallel(func(pb *testing.PB) {
				pre := strconv.FormatInt(time.Now().UnixNano(), 10) + "_"
				var index int
				for pb.Next() {
					m.Set(pre+strconv.Itoa(index/500), index)
					index++
				}
			})
		})
	}
	b.Run("sync_map", func(b *testing.B) {
		var m sync.Map
		b.RunParallel(func(pb *testing.PB) {
			pre := strconv.FormatInt(time.Now().UnixNano(), 10) + "_"
			var index int
			for pb.Next() {
				m.LoadOrStore(pre+strconv.Itoa(index), index)
				index++
			}
		})
	})

}
