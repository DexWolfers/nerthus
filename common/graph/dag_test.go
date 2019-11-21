package graph

import (
	"testing"

	"sort"

	"bytes"

	"github.com/stretchr/testify/assert"
)

func TestDag(t *testing.T) {

	dag := make(Dag)

	dag.AddEdge("b", "a")
	dag.AddEdge("c", "b")
	dag.AddEdge("d", "b")
	dag.AddEdge("g", "c")
	dag.AddEdge("f", "c")
	dag.AddEdge("e", "c")

	//              ⤹ -- e
	//   a <- b <-- c <--- F
	//        ↖︎-----d  ↖︎--g
	//

	assert.Equal(t, 6, dag["a"].indegreeTotal)
	assert.Equal(t, 0, dag["e"].indegree)
	assert.Equal(t, 2, dag["b"].indegree)
	assert.Equal(t, 0, dag["e"].indegree)

	if t.Failed() {
		for k, v := range dag {
			t.Logf("%s=%d(%d)", k, v.indegree, v.indegreeTotal)
		}
	}
}

func TestDag_FindLongPath(t *testing.T) {

	t.Run("one", func(t *testing.T) {

		dag := make(Dag)

		dag.AddEdge("b", "a")
		dag.AddEdge("c", "b")
		dag.AddEdge("d", "b")
		dag.AddEdge("f", "c")
		dag.AddEdge("e", "c")
		dag.AddEdge("g", "c")

		//              ⤹ -- e
		//   a <- b <-- c <--- f
		//        ↖︎-----d  ↖︎--g
		//
		paths := dag.Longest()
		assert.Len(t, paths, 3)
		for _, link := range paths {
			if link.Node.Name() == "e" {
				checkLine(t, link, "e->c->b->a")
			} else if link.Node.Name() == "f" {
				checkLine(t, link, "f->c->b->a")
			} else if link.Node.Name() == "g" {
				checkLine(t, link, "g->c->b->a")
			} else {
				t.Error("need e or f or g")
			}
		}
	})

	t.Run("two", func(t *testing.T) {
		dag := make(Dag)
		dag.AddVertex("a", nil)
		dag.AddVertex("b", nil)
		dag.AddVertex("c", nil)

		paths := dag.Longest()
		assert.Len(t, paths, 3)
		for _, link := range paths {
			checkLine(t, link, link.Node.Name().(string))
		}
	})

	t.Run("only one", func(t *testing.T) {
		dag := make(Dag)

		dag.AddEdge("b", "a")
		dag.AddEdge("c", "b")
		dag.AddEdge("d", "c")
		//   a <- b <-- c <--- d
		paths := dag.Longest()
		assert.Len(t, paths, 1)
		checkLine(t, paths[0], "d->c->b->a")
		t.Run("branch", func(t *testing.T) {
			dag.AddEdge("e", "b")
			dag.AddEdge("f", "e")
			dag.AddEdge("g", "f")
			//   a <- b <-- c <--- d
			// 	      ↖︎----e <--- f <---- g
			paths := dag.Longest()
			assert.Len(t, paths, 1, "need find three path( g->f->e->b->a)")

			line := bytes.NewBuffer(nil)
			paths[0].Print(line)
			checkLine(t, paths[0], "g->f->e->b->a")
		})
	})

}
func checkLine(t *testing.T, link *NodeLink, want string) bool {
	line := bytes.NewBuffer(nil)
	link.Print(line)
	return assert.Equal(t, want, line.String())
}
func TestNodeSet_Sort(t *testing.T) {
	s := []*Node{
		{name: "a", indegreeTotal: 100, indegree: 3},
		{name: "b", indegreeTotal: 200, indegree: 2},
		{name: "c", indegreeTotal: 300, indegree: 1},
	}
	sort.Sort(ByTotalIndegree{s})
	assert.Equal(t, s[0].name, "c")
	assert.Equal(t, s[1].name, "b")
	assert.Equal(t, s[2].name, "a")
}

func TestDag_Sub(t *testing.T) {
	dag := make(Dag)

	dag.AddEdge("b", "a")
	dag.AddEdge("c", "b")
	dag.AddEdge("d", "b")
	dag.AddEdge("f", "c")
	dag.AddEdge("e", "c")
	dag.AddEdge("g", "c")
	dag.AddEdge("h", "e")

	//              ⤹ -- e <----h
	//   a <- b <-- c <--- f
	//        ↖︎-----d  ↖︎--g
	//
	subg := dag.SubGraph("c")
	assert.Len(t, subg, 5, "should be have only five node")

	paths := subg.Longest()
	assert.Len(t, paths, 1)
	checkLine(t, paths[0], "h->e->c")
}

func TestNodeLink_Contains(t *testing.T) {
	dag := make(Dag)

	dag.AddEdge("b", "a")
	dag.AddEdge("c", "b")
	dag.AddEdge("d", "b")
	dag.AddEdge("f", "c")
	dag.AddEdge("e", "c")
	dag.AddEdge("g", "c")
	dag.AddEdge("h", "e")

	//              ⤹ -- e <----h
	//   a <- b <-- c <--- f
	//        ↖︎-----d  ↖︎--g
	//
	paths := dag.Longest()
	want := []string{"h", "e", "c", "b", "a"}
	for _, c := range want {
		assert.True(t, paths[0].Contains(c), "should be find the node %q in one path", c)
	}
}

func TestNode_AddWeight(t *testing.T) {
	dag := make(Dag)

	dag.AddEdge("b", "a")
	dag.AddEdge("c", "b")
	dag.AddEdge("d", "b")
	dag.AddVertex("e", "")
	//
	//   a <- b <-- c
	//        ↖︎-----d
	//   e

	for _, n := range dag {
		assert.True(t, n.weights == n.indegreeTotal, "weights and total indegree  should be equal")
	}
	if t.Failed() {
		return
	}

	aw, bw, cw, dw, ew := dag["a"].Weights(), dag["b"].Weights(), dag["c"].Weights(), dag["d"].Weights(), dag["e"].Weights()
	dag["d"].AddWeight(2)

	assert.Equal(t, aw+2, dag["a"].Weights())
	assert.Equal(t, bw+2, dag["b"].Weights())
	assert.Equal(t, dw+2, dag["d"].Weights())

	assert.Equal(t, cw, dag["c"].Weights())
	assert.Equal(t, ew, dag["e"].Weights())

}
