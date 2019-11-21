package set

import (
	"strconv"
	"sync"
	"testing"

	"github.com/fatih/set"

	"github.com/stretchr/testify/require"
)

func TestStringSet(t *testing.T) {

	s := NewStrSet(3, 3)

	t.Run("add", func(t *testing.T) {
		s.Add("test1")
		s.Add("test2")
		s.Add("test3")

		require.True(t, s.Has("test1"))
		require.True(t, s.Has("test2"))
		require.True(t, s.Has("test3"))

		t.Run("list", func(t *testing.T) {
			got := s.List()
			want := []string{"test1", "test2", "test3"}
			for _, v := range want {
				require.Contains(t, got, v)
			}
		})
	})

	t.Run("delete", func(t *testing.T) {
		s.Add("test123")
		s.Delete("test123")
		require.False(t, s.Has("test123"))
		s.Add("test123")
		require.True(t, s.Has("test123"))
	})
}

func TestStringSet_Pop(t *testing.T) {
	count := 100
	s := NewStrSet(3, count)
	for i := 0; i < count; i++ {
		s.Add(strconv.Itoa(i))
	}
	require.Equal(t, int64(count), s.Len(), "set length should be ok")

	for i := 0; i < count; i++ {
		s.Pop()
	}
	require.Empty(t, s.Len(), "set should be empty after Pop all")
}

func TestStringSet_PopN(t *testing.T) {
	count := 100
	s := NewStrSet(3, count)

	t.Run("popall", func(t *testing.T) {
		for i := 0; i < count; i++ {
			s.Add(strconv.Itoa(i))
		}
		items := s.List()
		got := s.PopN(count)
		for _, v := range items {
			require.Contains(t, got, v, "should be pop")
		}
	})
	t.Run("pop_all2", func(t *testing.T) {
		for i := 0; i < count; i++ {
			s.Add(strconv.Itoa(i))
		}
		items := s.List()
		got := s.PopN(count * 2)
		require.Equal(t, len(items), len(got), "should be pop all")
	})
	t.Run("pop3", func(t *testing.T) {
		for i := 0; i < count; i++ {
			s.Add(strconv.Itoa(i))
		}
		got := s.PopN(3)
		require.Equal(t, 3, len(got), "should be pop all")
	})

}

func TestStringSet_Limit(t *testing.T) {

	limit := 20
	s := NewStrSet(3, limit)

	var id int
	saveAt := make(map[int]int)
	for {
		id++
		item := strconv.Itoa(id)
		index := s.shardIndex(item)

		s.Add(item)

		saveAt[index] = saveAt[index] + 1
		if saveAt[index] > limit {
			//容量不会增长
			require.Equal(t, limit, len(s.getShard(item).items))
			break
		}
	}
}

func BenchmarkAdd(b *testing.B) {

	size := 30000

	b.Run("stringSet", func(b *testing.B) {
		s := NewStrSet(32, size/32)
		b.RunParallel(func(pb *testing.PB) {
			var index int
			for pb.Next() {
				item := strconv.Itoa(index)
				s.Add(item)
				index++
			}
		})
	})
	b.Run("set.Interface", func(b *testing.B) {
		s := set.New(set.ThreadSafe)
		b.RunParallel(func(pb *testing.PB) {
			var index int
			for pb.Next() {
				item := strconv.Itoa(index)
				if s.Size() == size {
					s.Pop()
				}
				s.Add(item)
				index++
			}
		})
	})
	b.Run("sync.map", func(b *testing.B) {
		s := sync.Map{}
		var count int

		b.RunParallel(func(pb *testing.PB) {
			var index int
			for pb.Next() {
				item := strconv.Itoa(index)
				if count == size {
					s.Range(func(key, value interface{}) bool {
						s.Delete(key)
						return false
					})
					count--
				}

				s.Store(item, struct{}{})
				index++
				count++
			}
		})

	})
}

func TestStringSet_Len(t *testing.T) {
	m := NewStrSet(1, 10)
	m.Add("item1")
	require.Equal(t, 1, int(m.Len()))
	m.Delete("item1")
	require.Equal(t, 0, int(m.Len()))
}
