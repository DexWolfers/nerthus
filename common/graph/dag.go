// Author: @ysqi

package graph

import (
	"fmt"
	"io"
	"sort"
)

// Dag 实现的是顶点最多只有一条边的有向无环图
type Dag map[interface{}]*Node

type Node struct {
	name interface{}
	Val  interface{}

	weights       int //顶点的权重值
	indegree      int
	indegreeTotal int
	children      []*Node
	parent        *Node //只允许一个父单元
}

func (n *Node) Name() interface{}  { return n.name }
func (n *Node) Parent() *Node      { return n.parent }
func (n *Node) Indegree() int      { return n.indegree }
func (n *Node) IndegreeTotal() int { return n.indegreeTotal }
func (n *Node) Weights() int       { return n.weights }

// AddWeight 添加权重
func (n *Node) AddWeight(v int) {
	if v == 0 {
		return
	}
	n.weights += v

	// 全部累加
	for p := n.parent; p != nil; {
		p.weights += v
		p = p.parent
	}
}

func (n *Node) addIndegre() {
	if n.parent != nil {
		n.parent.indegree++

		for p := n.parent; p != nil; {
			p.weights++
			p.indegreeTotal++
			p = p.parent
		}
	}
}

type NodeSet []*Node

// Len is part of sort.Interface.
func (s NodeSet) Len() int {
	return len(s)
}

// Swap is part of sort.Interface.
func (s NodeSet) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// NodeSortByTotalIndegree 按累计深度从高到底排序
type ByTotalIndegree struct{ NodeSet }

// Less is part of sort.Interface.
func (s ByTotalIndegree) Less(i, j int) bool {
	return s.NodeSet[i].indegreeTotal > s.NodeSet[j].indegreeTotal
}

// ByWeights 按权重从高到底排序
type ByWeights struct{ NodeSet }

// Less is part of sort.Interface.
func (s ByWeights) Less(i, j int) bool {
	return s.NodeSet[i].weights > s.NodeSet[j].weights
}

// AddVertex 添加顶点
func (g Dag) AddVertex(name, val interface{}) *Node {
	node := g[name]
	if node == nil {
		node = &Node{name: name, Val: val}
		g[name] = node
	}
	return node
}

// AddVertexWithEdge 添加顶点时同步添加边
func (g Dag) AddVertexWithEdge(fromName, toName, fromVal interface{}) *Node {
	n := g.AddVertex(fromName, fromVal)
	g.AddEdge(fromName, toName)
	return n
}

// AddEdge 添加边
func (g Dag) AddEdge(from, to interface{}) Dag {
	fromNode, toNode := g.AddVertex(from, nil), g.AddVertex(to, nil)
	//不重复处理
	if fromNode.parent == toNode {
		return g
	}
	//记录关系
	toNode.children = append(toNode.children, fromNode)
	fromNode.parent = toNode

	//更新深度
	fromNode.addIndegre()
	return g
}

// SubGraph 从图中复制一份子图，注意子图已完成脱离大图
func (g Dag) SubGraph(begin interface{}) Dag {
	b := g[begin]
	if b == nil {
		return nil
	}
	//复制出一份子图
	cpy := make(Dag)
	var allcp func(n *Node)
	allcp = func(n *Node) {
		cpy.AddVertex(n.name, n.Val)
		for _, c := range n.children {
			cpy.AddVertex(c.name, c.Val)
			cpy.AddEdge(c.name, n.name)
			allcp(c)
		}
	}
	allcp(b)
	return cpy
}

type NodeLink struct {
	Node   *Node
	Parent *NodeLink
}

func (l *NodeLink) Indegree() int {
	var indegre int
	for p := l.Parent; p != nil; p = p.Parent {
		indegre += p.Node.indegreeTotal
	}
	return indegre
}
func (l *NodeLink) Weight() int {
	var weight int
	for p := l.Parent; p != nil; p = p.Parent {
		weight += p.Node.weights
	}
	return weight
}

// Contains 判断端点是否存在在此路径中
func (l *NodeLink) Contains(name interface{}) bool {
	if l.Node.name == name {
		return true
	} else if l.Parent != nil {
		return l.Parent.Contains(name)
	}
	return false
}

func (l *NodeLink) Print(w io.Writer) {
	if l.Parent == nil {
		fmt.Fprint(w, l.Node.name)
	} else {
		fmt.Fprintf(w, "%s->", l.Node.name)
		l.Parent.Print(w)
	}
}

// Longest 获取最长路径，默认按最大深度数据确认
func (g Dag) Longest(sortBy ...func([]*Node)) (allPath []*NodeLink) {
	if len(g) == 0 {
		return
	}
	// 设置默认的排序方式
	if len(sortBy) == 0 || sortBy[0] == nil {
		sortBy = []func([]*Node){func(nodes []*Node) {
			sort.Sort(ByTotalIndegree{nodes})
		}}
	}
	//从顶点检索，为所有端点找出最长路径
	sortNodes := func(all interface{}) []*Node {
		var s []*Node
		switch all := all.(type) {
		case Dag:
			s = make([]*Node, 0, len(g))
			for _, n := range all {
				s = append(s, n)
			}
		case []*Node:
			s = all
		}
		sortBy[0](s)
		return s
	}
	var createLink func(node *Node) *NodeLink
	createLink = func(node *Node) *NodeLink {
		link := NodeLink{Node: node}
		if node.parent != nil {
			link.Parent = createLink(node.parent)
		}
		return &link
	}

	nodes := sortNodes(g)
	var maxIndegree int
	for i := len(nodes) - 1; i >= 0; i-- {
		if nodes[i].indegree > 0 {
			break
		}
		link := createLink(nodes[i])
		// 忽略底深度项
		if indegre := link.Indegree(); maxIndegree < indegre {
			maxIndegree = indegre
			//端点
			allPath = []*NodeLink{link} //新累计深度，则去除历史
		} else if maxIndegree == indegre { //相同累计深度，保留
			allPath = append(allPath, link)
		}
	}
	return allPath
}
