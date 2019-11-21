package syncgraph

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"os"

	"io/ioutil"

	"gitee.com/nerthus/nerthus/common"
	"github.com/stretchr/testify/require"
)

var g = `
          A     E      H     J
		/  \    |      |
       B    |   |      I
     /  \   |  |
	C   D   | /
		 \ //
          F
          |
		  G
`

func buildDag() (*Dag, []*Node, [][]NodeName) {
	var (
		dag = New()
		A   = Node{
			name: common.StringToHash("A"),
			val:  map[string]interface{}{"tm": time.Now()},
		}

		B = Node{
			name: common.StringToHash("B"),
			val:  time.Now(),
		}

		C = Node{
			name: common.StringToHash("C"),
			val:  time.Now(),
		}

		D = Node{
			name: common.StringToHash("D"),
			val:  time.Now(),
		}

		E = Node{
			name: common.StringToHash("E"),
			val:  time.Now(),
		}

		F = Node{
			name: common.StringToHash("F"),
			val:  time.Now(),
		}

		G = Node{
			name: common.StringToHash("G"),
			val:  time.Now(),
		}

		H = Node{
			name: common.StringToHash("H"),
			val:  time.Now(),
		}

		I = Node{
			name: common.StringToHash("I"),
			val:  time.Now(),
		}

		J = Node{
			name: common.StringToHash("J"),
			val:  time.Now(),
		}

		nodes []*Node
	)

	dag.AddVertex(A.name, A.val)
	dag.AddVertex(B.name, B.val)
	dag.AddVertex(C.name, C.val)
	dag.AddVertex(D.name, D.val)
	dag.AddVertex(E.name, E.val)
	dag.AddVertex(F.name, F.val)
	dag.AddVertex(G.name, G.val)
	dag.AddVertex(H.name, H.val)
	dag.AddVertex(I.name, I.val)
	dag.AddVertex(J.name, J.val)

	var edges [][]NodeName
	dag.AddEdge(A.name, B.name)
	dag.AddEdge(B.name, C.name)
	dag.AddEdge(B.name, D.name)
	dag.AddEdge(D.name, F.name)
	dag.AddEdge(A.name, F.name)
	dag.AddEdge(E.name, F.name)
	dag.AddEdge(F.name, G.name)
	dag.AddEdge(H.name, I.name)

	edges = append(edges, []NodeName{A.name, B.name})
	edges = append(edges, []NodeName{B.name, C.name})
	edges = append(edges, []NodeName{B.name, D.name})
	edges = append(edges, []NodeName{D.name, F.name})
	edges = append(edges, []NodeName{A.name, F.name})
	edges = append(edges, []NodeName{E.name, F.name})
	edges = append(edges, []NodeName{F.name, G.name})
	edges = append(edges, []NodeName{H.name, I.name})

	nodes = append(nodes, &A, &B, &C, &D, &E, &F, &G, &H, &I, &J)

	fmt.Println(g)

	fmt.Println("A->", A.name.Hex())
	fmt.Println("B->", B.name.Hex())
	fmt.Println("C->", C.name.Hex())
	fmt.Println("D->", D.name.Hex())
	fmt.Println("E->", E.name.Hex())
	fmt.Println("F->", F.name.Hex())
	fmt.Println("G->", G.name.Hex())
	fmt.Println("H->", H.name.Hex())
	fmt.Println("I->", I.name.Hex())
	fmt.Println("J->", J.name.Hex())

	return dag, nodes, edges
}

func TestGraph(t *testing.T) {
	dag, nodes, edges := buildDag()

	// data struct
	{
		require.Equal(t, len(dag.nodes), len(nodes))

		// test node
		for _, node := range nodes {
			require.Equal(t, node.name, dag.nodes[node.name].name)
		}

		// test edge
		for _, edge := range edges {
			require.True(t, dag.IsEdge(edge[0], edge[1]))
			from, to := edge[0], edge[1]
			node := dag.nodes[from]

			for _, child := range node.Children() {
				if child == to {
					goto Here
				}
			}
			t.Fatal("not found child")

		Here:
			continue
		}

		zeroDegreeNodes := dag.ZeroDegreeNodes()
		require.Len(t, zeroDegreeNodes, 4)

		zeroDegreeNodeNames := []common.Hash{
			common.StringToHash("A"),
			common.StringToHash("E"),
			common.StringToHash("H"),
			common.StringToHash("J"),
		}
		for _, node := range zeroDegreeNodes {
			exist := false
			for _, name := range zeroDegreeNodeNames {
				if name == node.name {
					exist = true
				}
			}
			require.True(t, exist)
		}
	}

	var dotGraph = bytes.NewBuffer(nil)
	dag.ToDotGraph(dotGraph)
	fmt.Println(dotGraph.String())
}

func TestDag_DelSubDag(t *testing.T) {
	dag, _, _ := buildDag()

	require.Empty(t, dag.DelSubDag(common.StringToHash("D")), "不可删除有父节点的节点，即被删除的节点入度必须为0，方可删除")
	require.Empty(t, dag.DelSubDag(common.StringToHash("G")), "不可删除有父节点的节点，即被删除的节点入度必须为0，方可删除")
	require.Empty(t, dag.DelSubDag(common.StringToHash("F")), "不可删除有父节点的节点，即被删除的节点入度必须为0，方可删除")
	require.Empty(t, dag.DelSubDag(common.StringToHash("C")), "不可删除有父节点的节点，即被删除的节点入度必须为0，方可删除")
	require.Empty(t, dag.DelSubDag(common.StringToHash("B")), "不可删除有父节点的节点，即被删除的节点入度必须为0，方可删除")
	require.Empty(t, dag.DelSubDag(common.StringToHash("I")), "不可删除有父节点的节点，即被删除的节点入度必须为0，方可删除")

	require.Empty(t, dag.DelSubDag(common.StringToHash("Z")), "不可删除不存在的节点")

	//
	require.True(t, dag.NoInDegree(common.StringToHash("J")))
	require.True(t, dag.NoInDegree(common.StringToHash("H")))
	require.True(t, dag.NoInDegree(common.StringToHash("E")))
	require.True(t, dag.NoInDegree(common.StringToHash("A")))
	require.False(t, dag.NoInDegree(common.StringToHash("D")))
	require.False(t, dag.NoInDegree(common.StringToHash("Z")))

	// 删除子图
	require.Len(t, dag.DelSubDag(common.StringToHash("J")), 1)
	require.Len(t, dag.ZeroDegreeNodes(), 3)
	require.Len(t, dag.DelSubDag(common.StringToHash("H")), 2)
	require.Len(t, dag.ZeroDegreeNodes(), 2)
	require.Len(t, dag.DelSubDag(common.StringToHash("E")), 1)
	require.Len(t, dag.ZeroDegreeNodes(), 1)
	subNodes := dag.DelSubDag(common.StringToHash("A"))
	{
		require.Len(t, subNodes, 6)
		require.Equal(t, subNodes[0].name, common.StringToHash("A"))
		require.Equal(t, subNodes[1].name, common.StringToHash("B"))
		require.Equal(t, subNodes[2].name, common.StringToHash("C"))
		require.Equal(t, subNodes[3].name, common.StringToHash("D"))
		require.Equal(t, subNodes[4].name, common.StringToHash("F"))
		require.Equal(t, subNodes[5].name, common.StringToHash("G"))
	}

	require.Empty(t, dag.ZeroDegreeNodes())

	require.False(t, dag.NoInDegree(common.StringToHash("J")))
	require.Empty(t, dag.Nodes())
}

func TestDag_DelBadSubDag(t *testing.T) {
	t.Run("删除顶点A", func(t *testing.T) {
		dag, _, _ := buildDag()
		nodes := dag.DelBadSubDag(common.StringToHash("A"))
		require.Len(t, nodes, 6)
		for _, node := range nodes {
			require.Contains(t,
				[]common.Hash{
					common.StringToHash("A"),
					common.StringToHash("B"),
					common.StringToHash("C"),
					common.StringToHash("D"),
					common.StringToHash("F"),
					common.StringToHash("G")},
				node.name)
		}

		/// 剩下的数量
		require.Len(t, dag.Nodes(), 4)

		/// InDegree top
		require.Len(t, dag.ZeroDegreeNodes(), 3)
		for _, node := range dag.ZeroDegreeNodes() {
			require.Contains(t, []common.Hash{
				common.StringToHash("E"),
				common.StringToHash("H"),
				common.StringToHash("J"),
			}, node.name)
		}
	})

	t.Run("删除叶子节点G", func(t *testing.T) {
		dag, _, _ := buildDag()
		nodes := dag.DelBadSubDag(common.StringToHash("G"))
		require.Len(t, nodes, 1)
		for _, node := range nodes {
			require.Contains(t,
				[]common.Hash{common.StringToHash("G")},
				node.name)
		}

		/// 剩下的数量
		require.Len(t, dag.Nodes(), 9)

		/// InDegree top
		require.Len(t, dag.ZeroDegreeNodes(), 4)
		for _, node := range dag.ZeroDegreeNodes() {
			require.Contains(t, []common.Hash{
				common.StringToHash("A"),
				common.StringToHash("E"),
				common.StringToHash("H"),
				common.StringToHash("J"),
			}, node.name)
		}
	})

	t.Run("删除中间节点D", func(t *testing.T) {
		dag, _, _ := buildDag()
		nodes := dag.DelBadSubDag(common.StringToHash("D"))
		require.Len(t, nodes, 3)
		for _, node := range nodes {
			require.Contains(t, []common.Hash{
				common.StringToHash("D"),
				common.StringToHash("F"),
				common.StringToHash("G"),
			}, node.name)
		}

		/// 剩下的数量
		require.Len(t, dag.Nodes(), 7)

		/// InDegree top
		require.Len(t, dag.ZeroDegreeNodes(), 4)
		for _, node := range dag.ZeroDegreeNodes() {
			require.Contains(t, []common.Hash{
				common.StringToHash("A"),
				common.StringToHash("E"),
				common.StringToHash("H"),
				common.StringToHash("J"),
			}, node.name)
		}
	})
}

func TestDag_DelCenterNode(t *testing.T) {
	t.Run("删除顶点A", func(t *testing.T) {
		dag, _, _ := buildDag()
		_, parentNodes, childrenNodes := dag.DelCenterNode(common.StringToHash("A"))
		require.Empty(t, parentNodes)
		require.Len(t, childrenNodes, 2)
		for _, node := range childrenNodes {
			require.Contains(t, []common.Hash{
				common.StringToHash("B"),
				common.StringToHash("F")}, node.name)
		}

		for _, node := range dag.ZeroDegreeNodes() {
			require.Contains(t, []common.Hash{
				common.StringToHash("B"),
				common.StringToHash("E"),
				common.StringToHash("H"),
				common.StringToHash("J"),
			}, node.name)
		}
	})

	t.Run("删除中间节点B", func(t *testing.T) {
		dag, _, _ := buildDag()
		_, parentNodes, childrenNodes := dag.DelCenterNode(common.StringToHash("B"))
		require.Len(t, parentNodes, 1)
		require.Equal(t, common.StringToHash("A"), parentNodes[0].name)

		for _, node := range childrenNodes {
			require.Contains(t, []common.Hash{
				common.StringToHash("C"),
				common.StringToHash("D")}, node.name)
		}

		for _, node := range dag.ZeroDegreeNodes() {
			require.Contains(t, []common.Hash{
				common.StringToHash("A"),
				common.StringToHash("C"),
				common.StringToHash("D"),
				common.StringToHash("E"),
				common.StringToHash("H"),
				common.StringToHash("J"),
			}, node.name)
		}
	})

	t.Run("删除叶子节点C", func(t *testing.T) {
		dag, _, _ := buildDag()
		_, parentNodes, childrenNodes := dag.DelCenterNode(common.StringToHash("C"))
		require.Len(t, parentNodes, 1)
		require.Equal(t, common.StringToHash("B"), parentNodes[0].name)
		require.Empty(t, childrenNodes)

		for _, node := range dag.ZeroDegreeNodes() {
			require.Contains(t, []common.Hash{
				common.StringToHash("A"),
				common.StringToHash("E"),
				common.StringToHash("H"),
				common.StringToHash("J"),
			}, node.name)
		}
	})
}

func TestDag_ToSubDotGraph(t *testing.T) {
	t.Run("", func(t *testing.T) {
		dag, _, _ := buildDag()
		buf := bytes.NewBuffer(nil)
		dag.ToDotGraph(buf)
		t.Log("--->", buf.String())
	})
	t.Run("", func(t *testing.T) {
		dag, _, _ := buildDag()
		buf := bytes.NewBuffer(nil)
		dag.ToSubDotGraph(buf, common.StringToHash("A"))
		t.Log("--->", buf.String())
	})
	t.Run("sub Graphviz", func(t *testing.T) {
		dag, _, _ := buildDag()
		str := dag.ToGraphviz(common.StringToHash("A"))
		fmt.Println(str)

		str = dag.ToGraphviz(common.Hash{})
		fmt.Println(str)
	})
	t.Run("sub Graphviz", func(t *testing.T) {
		dag, _, _ := buildDag()
		str := dag.ToGraphviz(common.StringToHash("A"))
		tmpFile := os.TempDir() + fmt.Sprintf("%v", time.Now().Unix())
		ioutil.WriteFile(tmpFile, []byte(str), os.ModePerm)
		t.Log("file", tmpFile)
		println(str)
	})
	t.Run("Graphviz", func(t *testing.T) {
		dag, _, _ := buildDag()
		str := dag.ToGraphviz(common.Hash{})

		tmpFile := os.TempDir() + fmt.Sprintf("%v", time.Now().Unix())
		ioutil.WriteFile(tmpFile, []byte(str), os.ModePerm)
		t.Log("file", tmpFile)
		println(str)

		println(base64.StdEncoding.EncodeToString([]byte(str)))
	})
}
