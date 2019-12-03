package merkle

func NewNode(left, right Node, t *Tree) Node {
	return &NodeBranch{
		childLeft:  left,
		childRight: right,
		tree:       t,
	}
}
func NewLeaf(content Content, index uint64, t *Tree) Node {
	nd := LeafBranch{
		index: index,
	}
	nd.hash = content.Hash(t.Hash)
	nd.tree = t
	return &nd
}

type LeafBranch struct {
	NodeBranch
	index uint64
}

func (t *LeafBranch) Cmp(n Node) int {
	n1, ok := n.(*LeafBranch)
	if !ok {
		return 0
	}
	if t.index > n1.index {
		return 1
	} else if t.index == n1.index {
		return 0
	}
	return -1
}
func (t *LeafBranch) IsLeaf() bool {
	return true
}
func (t *LeafBranch) Index() uint64 {
	return t.index
}
func (t *LeafBranch) Copy() *LeafBranch {
	c := &LeafBranch{
		index: t.index,
	}
	c.tree = t.tree
	c.hash = t.hash
	return c
}

type NodeBranch struct {
	hash       []byte
	childLeft  Node
	childRight Node
	parent     Node
	tree       *Tree
}

func (t *NodeBranch) Cmp(n Node) int {
	return 0
}
func (t *NodeBranch) Hash() []byte {
	if t == nil {
		return nil
	}
	if len(t.hash) != 0 {
		return t.hash
	}
	if t.childLeft == nil {
		// 是叶子节点
		//t.hash = t.content.Hash()
	} else if t.childRight == nil {
		// 末尾节点落单
		t.hash = t.childLeft.Hash()
	} else {
		// 有且只有2个子节点
		b := append(t.childLeft.Hash(), t.childRight.Hash()...)
		t.hash = t.tree.Hash(b)
	}
	return t.hash
}
func (t *NodeBranch) Reset() {
	t.hash = nil
}
func (t *NodeBranch) Children() (Node, Node) {
	return t.childLeft, t.childRight
}
func (t *NodeBranch) IsLeaf() bool {
	return t.childLeft == nil && t.childRight == nil
}
func (t *NodeBranch) Index() uint64 {
	return 0
}
func (t *NodeBranch) Parent() Node {
	return t.parent
}

func (t *NodeBranch) SetParent(node Node) {
	if node != nil || t.Parent() == nil {
		t.parent = node
	} else {
		p := t.Parent()
		t.parent = node
		c1, c2 := p.Children()
		if c1 != nil {
			c1.SetParent(nil)
		}
		if c2 != nil {
			c2.SetParent(nil)
		}
	}
}
func (t *NodeBranch) Update(content Content) {
	t.hash = content.Hash(t.tree.Hash)
	// 重置整条路径hash
	for p := t.Parent(); p != nil; {
		p.Reset()
		p = p.Parent()
	}
}
