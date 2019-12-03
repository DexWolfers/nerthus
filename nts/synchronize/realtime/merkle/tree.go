package merkle

import (
	"errors"
	"hash"
	"hash/fnv"

	"gitee.com/nerthus/nerthus/log"
)

//func NewTree(nodes []Content) *Tree {
//	return newTree(true, nodes)
//}
func NewTreeAppend(nodes []Content, findFunc func(content Content) int) *Tree {
	t := newTree(nodes)
	t.findFunc = findFunc
	return t
}
func newTree(nodes []Content) *Tree {
	ns := nodes
	if l := len(ns); l == 0 {
		return &Tree{hasher: fnv.New32()}
	} else if l == 1 {
		t := &Tree{hasher: fnv.New32()}
		t.leafs = []Node{NewLeaf(ns[0], 0, t)}
		return t
	}
	t := &Tree{hasher: fnv.New32()}
	if len(nodes) == 0 {
		return t
	}
	for i := range nodes {
		t.Append(nodes[i])
	}
	log.Debug("generate merkle tree", "input", len(nodes), "leafs", len(t.leafs), "root", t.Root().Hash())
	return t
}

type Tree struct {
	root   Node
	leafs  []Node
	hasher hash.Hash32
	//ordered  bool
	findFunc func(content Content) int
}

func (t *Tree) Root() Node {
	if t.root == nil {
		t.refreshTree()
	}
	return t.root
}
func (t *Tree) Hash(b []byte) []byte {
	t.hasher.Reset()
	t.hasher.Write(b)
	return t.hasher.Sum(nil)
	//hh := t.hasher.Sum(nil)
	//fmt.Printf("--input:%v >>>result:%v\n", b, hh)
	//return hh
}
func (t *Tree) Copy() *Tree {
	lf := make([]Node, t.LeafCount())
	for i, v := range t.leafs {
		lf[i] = v.(*LeafBranch).Copy()
	}
	newTree := Tree{
		leafs:    lf,
		findFunc: t.findFunc,
		hasher:   t.hasher,
	}
	return &newTree
}
func (t *Tree) Clear() {
	t.leafs = nil
	t.root = nil
}
func (t *Tree) LeafCount() int {
	return len(t.leafs)
}
func (t *Tree) Update(cont Content) error {
	if len(t.leafs) == 0 {
		t.leafs = []Node{NewLeaf(cont, 0, t)}
		return nil
	}
	var (
		v    Node
		idex int
	)
	lf := t.newLeaf(cont)
	if t.findFunc != nil {
		idex = t.findFunc(cont)
		if idex >= 0 {
			if idex >= len(t.leafs) {
				return errors.New("find index failed")
			}
			v = t.leafs[idex]
		} else {
			idex = len(t.leafs)
		}
	} else {
		v, idex = findLeafOrder(t.leafs, lf)
	}

	//fmt.Println("find index", idex, t.root)
	if v == nil {
		// 插入叶子，插入点后面的树路径全部重新构造
		//首先清除单边父节点
		t.clearSingleParent(t.leafs[idex-1])
		t.clearParent(idex)
		if idex >= len(t.leafs) {
			t.leafs = append(t.leafs, lf)
		} else {
			t.leafs = append(t.leafs[:idex+1], t.leafs[idex:]...)
			t.leafs[idex] = lf
		}
		// root 重置
		t.root = nil
		// 刷新树
		//t.refreshTree()
	} else {
		v.Update(cont)
	}
	// 刷新hash
	//t.root.Hash()
	return nil
}
func (t *Tree) Append(cont ...Content) {
	lf := make([]Node, t.LeafCount()+len(cont))
	for i := range cont {
		lf[t.LeafCount()+i] = t.newLeaf(cont[i])
	}
	if len(t.leafs) == 0 {
		t.leafs = lf
		return
	}
	// 清除右侧单边节点
	t.clearSingleParent(t.leafs[len(t.leafs)-1])
	copy(lf, t.leafs)
	// root 重置
	t.root = nil
}
func (t *Tree) newLeaf(cont Content) Node {
	return NewLeaf(cont, uint64(t.LeafCount()), t)
}

// 重新构造树root
func (t *Tree) refreshTree() {
	//log.Debug("refresh tree before", "leafs", t.LeafCount(), "root", t.root)
	ns := t.leafs
	for {
		if ns = t.buildAbove(ns); len(ns) == 1 {
			t.root = ns[0]
			break
		}
	}
	log.Debug("refresh tree", "leafs", t.LeafCount())
}
func (t *Tree) clearSingleParent(node Node) {
	// 往上找，直到找到单边节点或者到顶
	if node.Parent() == nil {
		return
	}
	if _, r := node.Parent().Children(); r == nil {
		node.SetParent(nil)
	} else {
		t.clearSingleParent(node.Parent())
	}
}

// 清除index右侧全部父节点
func (t *Tree) clearParent(index int) {
	for {
		if index >= len(t.leafs) {
			break
		}
		if t.leafs[index].Parent() == nil {
			// 已清除过，停止
			break
		}
		t.leafs[index].SetParent(nil)
		index++
	}
}

func (t *Tree) buildAbove(nodes []Node) []Node {
	parents := make([]Node, 0)
	for i := 0; i < len(nodes); i += 2 {
		// 不需要变动的路径跳过，节约性能
		if p := nodes[i].Parent(); p != nil {
			if nodes[i+1].Parent() == p {
				parents = append(parents, p)
				continue
			}
		}
		var c1, c2 Node
		if i+1 < len(nodes) {
			c1 = nodes[i]
			c2 = nodes[i+1]
		} else {
			c1 = nodes[i]
		}
		p := NewNode(c1, c2, t)
		parents = append(parents, p)
		c1.SetParent(p)
		if c2 != nil {
			c2.SetParent(p)
		}
	}
	return parents
}

// 折半查找
// return(结果，索引)
func findLeafHalf2(startIndex int, leafs []Node, dist Node) (Node, int) {
	//fmt.Printf("%d,", startIndex)
	i := len(leafs) / 2
	if c := leafs[i].Cmp(dist); c == 0 {
		return leafs[i], i + startIndex
	} else if c == 1 {
		if i == 0 {
			return nil, i + startIndex
		}
		return findLeafHalf2(startIndex, leafs[:i], dist)
	} else {
		if i+1 >= len(leafs) {
			return nil, i + 1 + startIndex
		}
		return findLeafHalf2(startIndex+i+1, leafs[i+1:], dist)
	}
}

// 顺序查找
func findLeafOrder(leafs []Node, dist Node) (Node, int) {
	if len(leafs) == 0 {
		return nil, 0
	}
	for i, _ := range leafs {
		if leafs[i].Cmp(dist) == 0 {
			return leafs[i], i
		}
	}
	return nil, len(leafs)
}

func sortContent(content []Content) {
	t := len(content) - 1
	for i := 0; i < t; i++ {
		for j := 0; j < t-i; j++ {
			old := content[j]
			if old.Cmp(content[j+1]) > 0 {
				content[j] = content[j+1]
				content[j+1] = old
			}
		}
	}
}
