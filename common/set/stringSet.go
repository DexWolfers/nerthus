package set

import (
	"sync"
	"sync/atomic"
)

type StringSet struct {
	shared      []*stringSetShared
	sharedCount int //max count
	sharedCap   int //单个shard的容量限制
	itemsLen    uint32
}

type stringSetShared struct {
	items        map[string]struct{}
	sync.RWMutex // Read Write mutex, guards access to internal map.
}

// 创建一个高并发的字符串数据集合，
// sharedCount 指定需要分片存放的数目，sharedCap表示每个分片的最大容量，如果容量为0则表示无上限
func NewStrSet(sharedCount int, sharedCap int) *StringSet {
	s := StringSet{
		shared:      make([]*stringSetShared, sharedCount),
		sharedCount: sharedCount,
		sharedCap:   sharedCap,
	}
	for i := 0; i < sharedCount; i++ {
		s.shared[i] = &stringSetShared{items: make(map[string]struct{})}
	}
	return &s
}

func (m *StringSet) shardIndex(item string) int {
	return int(uint(fnv32(item)) % uint(m.sharedCount))
}

// 获取元素所在分片组
func (m *StringSet) getShard(item string) *stringSetShared {
	return m.shared[m.shardIndex(item)]
}

// 往集合中添加一个元素。当该分片组数目超过限制时将进行一次随机删除。
func (m *StringSet) Add(item string) bool {
	shard := m.getShard(item)
	shard.Lock()
	if _, ok := shard.items[item]; ok {
		shard.Unlock()
		return false
	}
	m.store(shard, item)
	shard.Unlock()
	return true
}

func (m *StringSet) store(shard *stringSetShared, item string) {
	ins := true
	//是否增长
	//如果超过容量，则随机移除一个
	if m.sharedCap > 0 && len(shard.items) == m.sharedCap {
		for k := range shard.items {
			delete(shard.items, k)
			ins = false
			break
		}
	}
	shard.items[item] = struct{}{}
	if ins {
		m.add()
	}
}

func (s *StringSet) Delete(item string) bool {
	shard := s.getShard(item)
	shard.Lock()
	_, ok := shard.items[item]
	if ok {
		delete(shard.items, item)
	}
	shard.Unlock()
	if ok {
		s.sub()
	}
	return ok
}

// 判断指定元素是否存在
func (m *StringSet) Has(item string) bool {
	shard := m.getShard(item)
	shard.RLock()
	_, ok := shard.items[item]
	shard.RUnlock()
	return ok
}
func (m *StringSet) Len() uint32 {
	return m.itemsLen
}

func (m *StringSet) sub() {
	if m.itemsLen == 0 {
		return
	}
	atomic.AddUint32(&m.itemsLen, ^uint32(0))
}
func (m *StringSet) add() {
	atomic.AddUint32(&m.itemsLen, 1)
}

func (m *StringSet) List() []string {
	l := make([]string, 0, len(m.shared)*m.sharedCap)
	for _, s := range m.shared {
		s.RLock()
		for k := range s.items {
			l = append(l, k)
		}
		s.RUnlock()
	}
	return l
}

func (m *StringSet) Pop() (string, bool) {
	for _, s := range m.shared {
		s.Lock()
		// random select one
		for k := range s.items {
			delete(s.items, k)
			m.sub()
			s.Unlock()
			return k, true
		}
		s.Unlock()
	}
	return "", false
}

func (m *StringSet) PopN(n int) []string {
	items := make([]string, 0, n)
	for _, s := range m.shared {
		s.Lock()
		// random select one
		for k := range s.items {
			delete(s.items, k)
			items = append(items, k)
			m.sub()

			// full
			if len(items) == n {
				s.Unlock()
				return items
			}
		}
		s.Unlock()
	}
	return items
}
func (m *StringSet) Range(f func(item string) bool) {
	for _, s := range m.shared {
		s.RLock()
		for k := range s.items {
			if !f(k) {
				s.RUnlock()
				return
			}
		}
		s.RUnlock()
	}
}
func (m *StringSet) Reset() {
	for _, s := range m.shared {
		s.Lock()
		for k := range s.items {
			delete(s.items, k)
		}
		s.Unlock()
	}
}

func fnv32(key string) uint32 {
	hash := uint32(2166136261)
	const prime32 = uint32(16777619)
	for i := 0; i < len(key); i++ {
		hash *= prime32
		hash ^= uint32(key[i])
	}
	return hash
}
