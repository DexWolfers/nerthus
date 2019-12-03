package ntsdb

import (
	"math/rand"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDataSet_AddWithValue(t *testing.T) {
	db, close := newTestLDB()
	defer close()

	set := NewSet([]byte("like_colors"))

	require.Zero(t, set.Len(db))

	values := []string{"red", "blue", "yellow"}
	for i, v := range values {
		key, value := []byte(v), []byte(v+"_value")

		index := set.AddWithValue(db, key, value)
		require.Equal(t, uint32(i), index, "should be get right index")

		// get
		got := set.GetKey(db, index)
		require.Equal(t, v, string(got), "should be get  value after save")

		gotValue := set.GetValue(db, key)
		require.Equal(t, value, gotValue, "should be get value after save")

		v, ok := set.Exist(db, []byte(v))
		require.True(t, ok, "should be exist")
		require.Equal(t, index, v, "should be get right index")

	}

	for _, v := range values {
		key := []byte(v)

		index, ok := set.Exist(db, []byte(v))
		require.True(t, ok)

		//delete
		ok = set.Remove(db, key)
		require.True(t, ok)
		gotKey := set.GetKey(db, index)
		gotValue := set.GetValue(db, key)
		require.Empty(t, gotKey)
		require.Empty(t, gotValue)
	}

}

func TestStateSet_Add(t *testing.T) {
	db, close := newTestLDB()
	defer close()

	set := NewSet([]byte("like_colors"))

	require.Zero(t, set.Len(db))

	values := []string{"red", "blue", "yellow"}
	for i, v := range values {
		index := set.Add(db, []byte(v))
		require.Equal(t, uint32(i), index, "should be get right index")

		// get
		got := set.GetKey(db, index)
		require.Equal(t, v, string(got), "should be get redl value after save")

		v, ok := set.Exist(db, []byte(v))
		require.True(t, ok, "should be exist")
		require.Equal(t, index, v, "should be get right index")
	}

	t.Run("repeat", func(t *testing.T) {

		for _, v := range values {
			index, _ := set.Exist(db, []byte(v))

			gotIndex := set.Add(db, []byte(v))
			require.Equal(t, index, gotIndex)
		}

	})
}

func TestStateSet_Remove(t *testing.T) {
	db, close := newTestLDB()
	defer close()

	set := NewSet([]byte("like_colors"))

	require.Zero(t, set.Len(db))

	values := make([][]byte, 13)
	for i := 0; i < len(values); i++ {
		values[i] = []byte{byte(i)}
		index := set.Add(db, values[i])
		require.Equal(t, uint32(i), index, "should be get right index")
	}
	//remove first
	set.RemoveByIndex(db, 0)
	require.Equal(t, uint32(len(values))-1, set.Len(db), "set length should be reduced by 1")

	count := set.Len(db)

	set.Range(db, func(index uint32, value []byte) bool {
		t.Logf("index:%d,item=%s", index, value)
		return true
	})

	for i := count; i > 0; i-- {
		cur := set.Len(db)
		removed := set.RemoveByIndex(db, i) //不断移除中间一个
		require.Equal(t, cur-1, set.Len(db), "set length should be reduced by 1")

		set.Range(db, func(index uint32, value []byte) bool {
			_, exist := set.Exist(db, value)
			require.True(t, exist, "should be exist, at index %d, vlaue=%s", index, string(value))
			return true
		})

		_, ok := set.Exist(db, removed)
		require.False(t, ok, "should be not exist")
	}
	require.Zero(t, set.Len(db), "set should be empty after deleted all")

	//check clear all
	for i := 0; i < len(values); i++ {
		v := set.GetKey(db, uint32(i))
		require.Empty(t, v, "should be empty at index %d", i)
	}
}

func TestDataSet_Range(t *testing.T) {

	db, close := newTestLDB()
	defer close()

	t.Run("all", func(t *testing.T) {
		set := NewSet([]byte("like_colors_2"))
		values := []string{"red", "blue", "yellow"}

		for _, v := range values {
			set.Add(db, []byte(v))
		}
		//能得到所有
		var i int
		set.Range(db, func(index uint32, value []byte) bool {
			want := values[i]
			require.Equal(t, i, int(index))
			require.Equal(t, want, string(value))

			i++
			return true
		})
		require.Equal(t, len(values), i, "should be have range all")
	})

	t.Run("removeSome", func(t *testing.T) {
		set := NewSet([]byte("like_colors_removeSome"))

		values := []string{"red", "blue", "yellow", "green", "brown", "peek"}

		for _, v := range values {
			set.Add(db, []byte(v))
		}
		//删除一些
		removed := []string{"red", "yellow", "green", "peek"}
		for _, v := range removed {
			ok := set.Remove(db, []byte(v))
			require.True(t, ok)
		}
		wantLen := len(values) - len(removed)
		curLen := int(set.Len(db))

		//长度应该只剩下：2
		require.Equal(t, wantLen, curLen, "should be has removed")
		//可成功遍历剩余
		var i int
		set.Range(db, func(index uint32, value []byte) bool {
			require.Equal(t, values[index], string(value))
			i++
			return true
		})
		require.Equal(t, wantLen, i, "should be have range all")

	})

}

func TestDataSet_Range_Lock(t *testing.T) {
	db, close := newTestLDB()
	defer close()
	set := NewSet([]byte("like_colors_2"))

	for i := 0; i < 10; i++ {
		set.AddWithValue(db, []byte{byte(i)}, []byte{2})
	}

	var wg sync.WaitGroup
	wg.Add(10)

	go set.Range(db, func(index uint32, key []byte) bool {
		set.GetValue(db, key)
		set.GetKey(db, index)
		wg.Done()
		return true
	})
	wg.Wait()

	t.Run("delete", func(t *testing.T) {
		set.RemoveByIndex(db, 0)
		set.RemoveByIndex(db, 1)
		set.RemoveByIndex(db, 9)
		t.Log("len:", set.Len(db))

		set.Range(db, func(index uint32, key []byte) bool {
			set.GetValue(db, key)
			set.GetKey(db, index)
			return true
		})

	})
	t.Run("rang2", func(t *testing.T) {
		//只取一个
		var count int
		set.Range(db, func(index uint32, key []byte) bool {
			set.GetValue(db, key)
			set.GetKey(db, index)
			count++
			if count >= 2 {
				return false
			}
			return true
		})
		require.Equal(t, 2, count)
	})

	t.Run("delete", func(t *testing.T) {
		set := NewSet([]byte("like_colors_3"))

		var keys [][]byte
		for i := 0; i < 10; i++ {
			keys = append(keys, []byte{byte(i + 1)})
			set.AddWithValue(db, []byte{byte(i + 1)}, []byte{2})
		}

		//随机移除
		deleted := make(map[byte]bool)
		for {
			index := rand.Intn(len(keys))

			var before []byte
			set.Range(db, func(index uint32, key []byte) bool {
				before = append(before, key[0])
				return true
			})
			index2, ok2 := set.Exist(db, keys[index])
			t.Log(keys[index][0], index2, ok2)
			ok := set.Remove(db, keys[index])
			if ok {
				deleted[keys[index][0]] = true
				t.Log("deleted:", keys[index][0])
				finds := make(map[byte]bool)

				var after []byte
				set.Range(db, func(index uint32, key []byte) bool {
					_, exist := deleted[key[0]]
					require.False(t, exist)
					finds[key[0]] = true
					after = append(after, key[0])
					return true
				})
				t.Log("before:", before)
				t.Log("after :", after)
				for _, k := range keys {
					if deleted[k[0]] {
						continue
					}
					require.True(t, finds[k[0]], "should be find %d", k[0])
				}
			}
			if len(deleted) >= len(keys) {
				break
			}
		}

	})
}
