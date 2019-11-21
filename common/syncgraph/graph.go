package syncgraph

import (
	"bytes"
	"container/list"
	"encoding/json"
	"fmt"
	"strings"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/log"
	"github.com/tmc/dot"
)

type NodeName = common.Hash
type SubDagIterFn = func(node Node) bool

type Node struct {
	name NodeName
	val  interface{}

	inDegree  int        // 入度
	parents   []NodeName // Just for test
	outDegree int        // 出度
	children  []NodeName
}

// it not thread safe struct
type Dag struct {
	nodes map[NodeName]*Node
}

func New() *Dag {
	return &Dag{
		nodes: make(map[NodeName]*Node),
	}
}

func (dag *Dag) AddVertex(name NodeName, val interface{}) bool {
	if _, ok := dag.nodes[name]; ok {
		return ok
	}
	node := Node{
		name:     name,
		val:      val,
		inDegree: 0,
	}

	dag.nodes[name] = &node
	return false
}

/**
	 A
	/
   B
from B to A: B--->A
*/
func (dag *Dag) AddEdge(fromName, toName NodeName) {
	fromNode, ok := dag.nodes[fromName]
	if !ok {
		return
	}
	toNode, ok := dag.nodes[toName]
	if !ok {
		return
	}
	fromNode.children = append(fromNode.children, toName)
	fromNode.outDegree++
	toNode.inDegree++
	toNode.parents = append(toNode.parents, fromName)
}

//func (dag *Dag) UpdateInDegree(name NodeName, inDegree int) bool {
//	if node, ok := dag.nodes[name]; !ok {
//		return ok
//	} else {
//		node.inDegree = inDegree
//		return true
//	}
//}

func (dag *Dag) Nodes() []Node {
	var nodes = make([]Node, 0, len(dag.nodes))
	for _, node := range dag.nodes {
		nodes = append(nodes, *node)
	}
	return nodes
}

func (dag *Dag) ExistNode(nodeName NodeName) bool {
	_, ok := dag.nodes[nodeName]
	return ok
}

// 找出name的父节点
func (dag *Dag) ParentNodes(name NodeName) []*Node {
	var nodes []*Node
	for _, node := range dag.nodes {
		for _, child := range node.Children() {
			if child == name {
				nodes = append(nodes, node)
				break
			}
		}
	}
	return nodes
}

func (dag *Dag) ZeroDegreeNodes() []Node {
	var nodes []Node
	for _, node := range dag.nodes {
		if node.inDegree == 0 {
			nodes = append(nodes, *node)
		}
	}
	return nodes
}

// 如果不存在该节点也会返回false
func (dag *Dag) NoInDegree(nodeName NodeName) bool {
	if node, ok := dag.nodes[nodeName]; ok && node.inDegree == 0 {
		return true
	}
	return false
}

func (dag *Dag) IsEdge(fromName, toName NodeName) bool {
	fromNode, ok := dag.nodes[fromName]
	if !ok {
		return ok
	}

	if fromNode.name != fromName {
		return false
	}

	for _, child := range fromNode.Children() {
		if child == toName {
			return true
		}
	}
	return false
}

/// 性能较差，不可随便使用
//func (dag *Dag) SplitGraph() []Dag {
//	zeroIndegreeNodes := dag.ZeroDegreeNodes()
//
//	graphs := map[NodeName]*Dag{}
//	travelNodes := map[NodeName]NodeName{}
//
//	for _, topNode := range zeroIndegreeNodes {
//		subDag := New()
//		subDag.nodes[topNode.name] = &topNode
//		queue := list.New()
//		queue.PushFront(topNode.name)
//		/// 是否为独立子图
//		belong := common.Hash{}
//		for {
//			item := queue.Front()
//			if item == nil {
//				break
//			}
//			name := item.Value.(NodeName)
//			node, ok := dag.nodes[name]
//			if !ok {
//				break
//			}
//			queue.Remove(item)
//
//			/// 如果已被遍历过，则归属于其他子图
//			if _, ok := travelNodes[name]; ok {
//				belong = travelNodes[name]
//				break
//			} else {
//				travelNodes[name] = topNode.name
//			}
//			subDag.nodes[name] = node
//
//			for _, child := range node.Children() {
//				queue.PushFront(child)
//			}
//		}
//		if belong.Empty() {
//			//fmt.Println("--->sub graph", subDag.ZeroDegreeNodes()[0].name.String())
//			graphs[topNode.name] = subDag
//		} else {
//			//fmt.Println("Belong", belong.Hex())
//			belongGraph := graphs[belong]
//			for _, node := range subDag.nodes {
//				belongGraph.AddVertex(node.name, node.val)
//			}
//		}
//	}
//
//	var dags []Dag
//	for _, dag := range graphs {
//		dags = append(dags, *dag)
//	}
//	return dags
//}

/// 删除"坏"子图, 返回子图节点
func (dag *Dag) DelBadSubDag(name NodeName) []*Node {
	var (
		badNode = map[NodeName]struct{}{name: {}}
		queue   = list.New()
	)
	queue.PushFront(name)

	for {
		// Peek stack
		item := queue.Front()
		if item == nil {
			break
		}
		name := item.Value.(NodeName)
		node, ok := dag.nodes[name]
		if !ok {
			break
		}

		// 处理父节点
		{
			// 找出父节点
			parentsNodes := dag.ParentNodes(name)
			// 删除父节点的引用关系，不包括Bad节点
			for _, parentNode := range parentsNodes {
				// 这些父节点不可包含在BadNode集合里
				if _, ok := badNode[parentNode.name]; !ok {
					dfsTravelParent(dag, name, parentNode)
				}
			}
		}
		// Pop stack
		queue.Remove(item)

		// 递归更新子节点
		for _, child := range node.Children() {
			queue.PushFront(child)
			badNode[child] = struct{}{}
		}
	}

	// 清除怀节点子图
	return dag.DelSubDag(name)
}

/// 删除中间节点, 返回父节点，以及子节点
/// TODO 优化,增加双向引用
func (dag *Dag) DelCenterNode(name NodeName) (node *Node, parentsNodes, childrenNodes []*Node) {
	var ok bool
	node, ok = dag.nodes[name]
	if !ok {
		return
	}

	/// 处理父节点
	{
		// 找出父节点
		parentsNodes = dag.ParentNodes(name)
		// 删除父节点的引用关系
		for _, parentNode := range parentsNodes {
			idx := 0
			children := parentNode.Children()
			for i, child := range children {
				if child == name {
					idx = i
					break
				}
			}

			// 更新父节点信息
			parentNode.outDegree--
			if len(children) == idx {
				parentNode.children = parentNode.children[:idx]
			} else {
				parentNode.children = append(parentNode.children[:idx], parentNode.children[idx+1:]...)
			}
		}
	}

	/// 处理子节点
	{
		for _, childName := range node.Children() {
			childNode, ok := dag.nodes[childName]
			if !ok {
				panic("it should be not happen")
			}
			childNode.inDegree--
			{
				idx := 0
				for i, parent := range childNode.parents {
					if parent == name {
						idx = i
						break
					}
				}
				if len(childNode.parents) == idx {
					childNode.parents = childNode.parents[:idx]
				} else {
					childNode.parents = append(childNode.parents[:idx], childNode.parents[idx+1:]...)
				}
			}

			childrenNodes = append(childrenNodes, childNode)
		}
	}

	/// 处理需要删除的节点
	delete(dag.nodes, name)
	return
}

/// 只能从顶点处开始删除
func (dag *Dag) DelSubDag(name NodeName) []*Node {
	var nodes []*Node
	if node, ok := dag.nodes[name]; ok {
		if node.inDegree == 0 {
			dfsTravelDel(dag, name, &nodes)
		}
	}

	var uniqNodes = make([]*Node, 0, len(nodes))
	for _, node := range nodes {
		if !existNode(node, uniqNodes) {
			uniqNodes = append(uniqNodes, node)
		}
	}

	return uniqNodes
}

func (dag *Dag) ToDotGraph(buf *bytes.Buffer) {
	buf.WriteString("digraph depgraph {\n\trankdir=LR;\n")
	for _, node := range dag.nodes {
		node.dotGraph(buf)
	}
	buf.WriteString("}\n")
}

func (dag *Dag) ToSubDotGraph(buf *bytes.Buffer, name NodeName) {
	buf.WriteString("digraph depgraph {\n\trankdir=LR;\n")
	defer buf.WriteString("}\n")

	var queue = list.New()
	var travelNodes = make(map[NodeName]struct{})
	if _, ok := dag.nodes[name]; !ok {
		buf.WriteString("Not Found the node")
	} else {
		queue.PushFront(name)
	}

	for {
		item := queue.Front()
		if item == nil {
			return
		}
		nodeName := item.Value.(NodeName)
		node, _ := dag.nodes[nodeName]
		node.dotGraph(buf)

		queue.Remove(item)

		for _, child := range node.Children() {
			if _, ok := travelNodes[child]; !ok {
				queue.PushFront(child)
				travelNodes[child] = struct{}{}
			}
		}
	}
}

func (dag *Dag) allGraphviz(nodeName NodeName) string {
	var (
		graph       = dot.NewGraph("G")
		zeroNodes   []Node
		travelNodes = make(map[NodeName]struct{})
		travelEdges = make(map[string]struct{})
	)
	graph.Set("label", "sync graph")
	graph.Set("color", "sienna")
	if nodeName.Empty() {
		zeroNodes = dag.ZeroDegreeNodes()
	} else {
		node, ok := dag.nodes[nodeName]
		if ok {
			zeroNodes = append(zeroNodes, *node)
		}
	}

	for i, node := range zeroNodes {
		subGraph := dot.NewSubgraph(fmt.Sprintf("G%v", i))
		subGraph.Set("label", "sub graph")
		queue := list.New()
		queue.PushFront(node.name)

		for {
			item := queue.Front()
			if item == nil {
				break
			}
			queue.Remove(item)

			nodeName := item.Value.(NodeName)
			node, _ := dag.nodes[nodeName]
			parentNode := dot.NewNode(FormatGraphvizNode(node))
			if _, ok := travelNodes[nodeName]; !ok {
				subGraph.AddNode(parentNode)
				travelNodes[nodeName] = struct{}{}
			}
			for _, child := range node.Children() {
				childNode, ok := dag.nodes[child]
				if !ok {
					// TODO fix
					log.Debug("child node is nil")
					continue
				}
				if _, ok := travelEdges[nodeName.Hex()+child.Hex()]; !ok {
					travelEdges[nodeName.Hex()+child.Hex()] = struct{}{}
					subGraph.AddEdge(FormatGraphvizEdge(node, childNode))
					queue.PushFront(child)
				}
			}
		}

		graph.AddSubgraph(subGraph)
	}
	return graph.String()
}

func (dag *Dag) ToGraphviz(nodeName NodeName) string {
	return dag.allGraphviz(nodeName)

	//var (
	//	travelNodes = make(map[NodeName]struct{})
	//	travelEdges = make(map[string]struct{})
	//	queue       = list.New()
	//)
	//
	//graph := dot.NewGraph("G")
	//graph.Set("label", "sync graph")
	//
	//if _, ok := dag.nodes[nodeName]; ok {
	//	queue.PushFront(nodeName)
	//}
	//for {
	//	item := queue.Front()
	//	if item == nil {
	//		break
	//	}
	//	queue.Remove(item)
	//
	//	nodeName := item.Value.(NodeName)
	//	node, _ := dag.nodes[nodeName]
	//	parentNode := dot.NewNode(FormatGraphvizNode(node))
	//	if _, ok := travelNodes[nodeName]; !ok {
	//		graph.AddNode(parentNode)
	//		travelNodes[nodeName] = struct{}{}
	//	}
	//	for _, child := range node.Children() {
	//		childNode, _ := dag.nodes[child]
	//		//if _, ok := travelNodes[child]; !ok {
	//		//	//graph.AddNode(dot.NewNode(FormatGraphviz(childNode)))
	//		//	travelNodes[child] = struct{}{}
	//		//}
	//		if _, ok := travelEdges[nodeName.Hex()+child.Hex()]; !ok {
	//			travelEdges[nodeName.Hex()+child.Hex()] = struct{}{}
	//			graph.AddEdge(FormatGraphvizEdge(node, childNode))
	//			queue.PushFront(child)
	//		}
	//	}
	//}
	//return []string{graph.String()}
}

func (node *Node) dotGraph(buf *bytes.Buffer) {
	if len(node.children) == 0 {
		buf.WriteString(fmt.Sprintf("\t\"Parent:%s\";\n", node.name.Hex()))
		return
	}

	for _, child := range node.children {
		buf.WriteString(fmt.Sprintf(`Parent:%s -> Child:%s [label="%v"]`, node.name.Hex(), child.Hex(), node.val))
		buf.WriteString("\r\n")
	}
}

func (node *Node) Children() []NodeName {
	return node.children
}

func (node *Node) Parents() []NodeName {
	return node.parents
}

func (node *Node) Name() NodeName {
	return node.name
}

func FormatNodes(nodes []*Node) string {
	var str []string
	for _, node := range nodes {
		str = append(str, node.name.Hex())
	}
	return strings.Join(str, ",")
}

func FormatNodeNames(names []NodeName) string {
	var str []string
	for _, name := range names {
		str = append(str, name.Hex())
	}
	return strings.Join(str, ",")
}

func FormatGraphvizNode(node *Node) string {
	b, _ := json.Marshal(node.val)
	return fmt.Sprintf("%s\nval=%s\nindegree=%v", node.name.Hex(), string(b), node.inDegree)
}

func FormatGraphvizEdge(from, to *Node) *dot.Edge {
	fromNode := dot.NewNode(FormatGraphvizNode(from))
	toNode := dot.NewNode(FormatGraphvizNode(to))
	return dot.NewEdge(fromNode, toNode)
}

// 更新 被删除节点的父节点信息
func dfsTravelParent(dag *Dag, delName NodeName, parentNode *Node) {
	// 遍历叶子节点
	idx := -1
	for i, childName := range parentNode.children {
		if childName == delName {
			idx = i
			break
		}
	}

	if idx >= 0 {
		// 更新父节点的出度数
		parentNode.outDegree--
		// 更新父节点的孩子
		if idx == len(parentNode.children)-1 {
			parentNode.children = parentNode.children[:idx]
		} else {
			parentNode.children = append(parentNode.children[:idx], parentNode.children[idx+1:]...)
		}

		// 更新删除节点的入度数
		if delNode, ok := dag.nodes[delName]; !ok {
			panic("should be not happen")
		} else {
			delNode.inDegree--
			{
				idx := 0
				for i, parent := range delNode.parents {
					if parent == delName {
						idx = i
						break
					}
				}
				if len(delNode.parents) == idx {
					delNode.parents = delNode.parents[:idx]
				} else {
					delNode.parents = append(delNode.parents[:idx], delNode.parents[idx+1:]...)
				}
			}
		}
	}
}

/// name 将要删除的位置
func dfsTravelDel(dag *Dag, name NodeName, delNodes *[]*Node) {
	node, ok := dag.nodes[name]
	if !ok {
		panic("it should be not happen")
	}
	// 出度为0，则表示其所有的父节点清除
	if node.inDegree == 0 {
		delete(dag.nodes, name)
		*delNodes = append(*delNodes, node)
		//遍历孩子
		// TODO 改为尾递归
		for _, childName := range node.children {
			// 减掉孩子节点的入度
			childNode, ok := dag.nodes[childName]
			if !ok {
				log.Warn("it should be not happen", "pHash", node.name, "cHash", childName, "graph",
					dag.ToGraphviz(common.Hash{}))
				continue
			}
			childNode.inDegree--
			// 删除父节点引用
			{
				idx := 0
				for i, parent := range childNode.parents {
					if parent == name {
						idx = i
						break
					}
				}
				if len(childNode.parents) == idx {
					childNode.parents = childNode.parents[:idx]
				} else {
					childNode.parents = append(childNode.parents[:idx], childNode.parents[idx+1:]...)
				}
			}
			dfsTravelDel(dag, childName, delNodes)
		}
	}
}

func existNode(node *Node, nodes []*Node) bool {
	for _, n := range nodes {
		if n.name == node.name {
			return true
		}
	}
	return false
}
