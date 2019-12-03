package merkle

type Node interface {
	Cmp(n Node) int
	Hash() []byte
	Reset()
	Children() (Node, Node)
	IsLeaf() bool
	Index() uint64
	Parent() Node
	SetParent(node Node)
	Update(content Content)
}

type Content interface {
	Hash(hasher func([]byte) []byte) []byte
	Set(content Content) Content
	Cmp(n Content) int
	Copy() Content
}
