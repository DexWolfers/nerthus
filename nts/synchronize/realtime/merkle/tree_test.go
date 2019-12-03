package merkle

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/rlp"
	"github.com/stretchr/testify/require"
)

func TestTree_Root(t *testing.T) {
	go http.ListenAndServe("127.0.0.1:6060", nil)
	ls := make([]Content, 0)
	for i := 1; i < 100; i++ {
		ls = append(ls, testContent{seq: rand.Int()})
	}
	tree := NewTreeAppend(ls, nil)
	t.Log(tree.Root().Hash())

	require.NoError(t, checkSort(tree))
	fmt.Println("-----------------------")
	tree.Update(testContent{seq: rand.Int()})
	require.NoError(t, checkSort(tree))
	t.Logf("leafs:%d root:%v", len(tree.leafs), tree.Root().Hash())

	// check hash correct

}
func TestTree_Append(t *testing.T) {
	ls := make([]Content, 0)
	for i := 1; i < 30; i++ {
		ls = append(ls, testContent{seq: i})
	}
	tree := NewTreeAppend(ls, nil)
	t.Log(tree.Root().Hash())

	tree.Append(testContent{seq: 4})
	tree.Root().Hash()
	t.Logf("leafs:%d root:%v", len(tree.leafs), tree.Root().Hash())
	nd := tree.Root()
	for {
		if l, _ := nd.Children(); l == nil {
			break
		}
		fmt.Print(0)
		nd, _ = nd.Children()
	}

	//
	for _, v := range tree.leafs {
		fmt.Print(v)
	}
	fmt.Println()
	checkRelate(tree.Root(), tree, t)
}
func BenchmarkTree_Build(b *testing.B) {
	leafCount := 20000000
	ls := make([]Content, leafCount)
	for i := 0; i < leafCount; i++ {
		ls[i] = testContent{seq: i}
	}
	fmt.Println("ready to build >>>>")
	start := time.Now()
	tree := NewTreeAppend(ls, nil)
	fmt.Printf("leafs:%d root:%v\n", len(tree.leafs), tree.Root().Hash())
	fmt.Printf("build tree spend time:%v\n", time.Now().Sub(start))

	b.ResetTimer()
	b.Run("append", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			tree.Append(testContent{seq: leafCount + i})
			tree.Root().Hash()
		}
	})
}

func BenchmarkTree_Update(b *testing.B) {
	coutN := 1000000
	ls := make([]Content, 0)
	for i := 0; i < coutN; i++ {
		ls = append(ls, testContent{seq: i * 1000})
	}
	fmt.Println(">>>>>>prepared")
	t := time.Now()
	tree := NewTreeAppend(ls, nil)
	fmt.Printf("leafs:%d root:%s\n", len(tree.leafs), tree.Root().Hash())
	fmt.Println("----------------------- cost>", time.Now().Sub(t))
	b.ReportAllocs()
	b.ResetTimer()
	b.Run("update", func(b *testing.B) {
		for n := 0; n < b.N; n++ {
			tc := ls[rand.Intn(coutN-1)].(testContent)
			tc.extr = rand.Int()
			tree.Update(tc)
			tree.Root().Hash()
		}
	})
	fmt.Printf("leafs:%d root:%s\n", len(tree.leafs), tree.Root().Hash())

}

func TestTreeBuildCost(t *testing.T) {

	chains := 50000000
	tree := NewTreeAppend(nil, nil)

	now := time.Now()
	for i := 0; i < chains; i++ {
		tree.Append(testContent{seq: i})
	}
	cost1, now := time.Since(now), time.Now()
	tree.Root().Hash()
	cost := time.Since(now)
	t.Log(cost1, cost)
}

func BenchmarkTree_Find(b *testing.B) {
	coutN := 1000000
	ls := make([]Content, 0)
	for i := 0; i < coutN; i++ {
		ls = append(ls, testContent{seq: i * 1000})
	}
	fmt.Println(">>>>>>prepared")
	tree := NewTreeAppend(ls, nil)
	fmt.Printf("tree1 > leafs:%d root:%s\n", len(tree.leafs), tree.Root().Hash())

	ls2 := make([]Content, len(ls))
	copy(ls2, ls)

	tree2 := NewTreeAppend(ls2, nil)

	updated := make(map[int]Content)
	for i := 0; i < 1000000; i++ {
		index := rand.Intn(len(ls2))
		up := ls2[index].(testContent)
		up.extr = up.extr + 1
		updated[i] = up
		tree2.Append(up)
		//fmt.Println("updated node index", index, "id", up.seq)
		//fmt.Println(findLeaf(0, tree.leafs, NewNode(up)))
		//fmt.Println(findLeaf(0, tree2.leafs, NewNode(up)))
	}
	fmt.Printf("tree2 > leafs:%d root:%s\n", len(tree2.leafs), tree2.Root().Hash())

	diffs := make([][]Node, 2)
	if bytes.Equal(tree.Root().Hash(), tree2.Root().Hash()) {
		cmp(tree.Root(), tree2.Root(), diffs)
	}
	for i, v := range diffs {
		fmt.Printf("------tree %d-------\n", i)
		if len(v) != len(updated) {
			b.Fatal("different nodes len, finded:", len(v), "actually:", len(updated))
		}
		for _, lf := range v {
			if _, ok := updated[int(lf.Index())]; !ok {
				b.Fatal("find wrong", lf)
			}
			//fmt.Println(lf)
		}
	}
	b.Logf("cmp times>%d", cmpTimes)
}

var cmpTimes int

func cmp(c1, c2 Node, diff [][]Node) {
	cmpTimes++
	if l, _ := c1.Children(); l == nil {
		diff[0] = append(diff[0], c1)
		diff[1] = append(diff[1], c2)
		return
	}
	l1, r1 := c1.Children()
	l2, r2 := c2.Children()
	if !bytes.Equal(l1.Hash(), l2.Hash()) {
		cmp(l1, l2, diff)
	}
	if !bytes.Equal(r1.Hash(), r2.Hash()) {
		cmp(r1, r2, diff)
	}
}

func checkSort(tree *Tree) error {
	for i := 0; i < len(tree.leafs)-1; i++ {
		//fmt.Println(tree.leafs[i].Content().(testContent).seq)
		if tree.leafs[i].Cmp(tree.leafs[i+1]) >= 0 {
			return fmt.Errorf("left %d,right:%d", tree.leafs[i].Index(), tree.leafs[i+1].Index())
		}
	}
	return nil
}

type testContent struct {
	seq  int
	extr int
}

func (t testContent) Hash(f func([]byte) []byte) []byte {
	return f([]byte{byte(t.seq), byte(t.extr)})
}
func (t testContent) Set(content Content) Content {
	t.extr = content.(testContent).extr
	return t
}
func (t testContent) Copy() Content {
	return t
}
func (t testContent) Cmp(n Content) int {
	dist := n.(testContent)
	if t.seq > dist.seq {
		return 1
	} else if t.seq == dist.seq {
		return 0
	} else {
		return -1
	}
}

//1173240

func TestMsgSize(t *testing.T) {

	msg := struct {
		H1, H2 common.Hash
		Number uint64
	}{
		H1: common.HexToHash("0xc14a169199fa7d8f5fcf9aaf8935a2c2fb2ba037a43bd1bd6a5daaa05e7365be"),
		H2: common.HexToHash("0x17e1ab2f71da42c04d80e6137541b04054f97604eaf31a202c5fc7d4abb8db47"),
	}

	v, err := rlp.EncodeToBytes(&msg)
	require.NoError(t, err)
	count := 1173240 //1559935
	t.Log(common.StorageSize(count * (len(v) + 10)))
}

func checkRelate(p Node, tr *Tree, t *testing.T) {
	if p.IsLeaf() {
		return
	}
	l, r := p.Children()
	fmt.Println(l, r)
	var cHash []byte
	if r != nil {
		cHash = tr.Hash(append(l.Hash(), r.Hash()...))
	} else if l != nil {
		cHash = l.Hash()
	} else {
		cHash = p.Hash()
	}

	if !bytes.Equal(cHash, p.Hash()) {
		t.Error(cHash, p)
	}

	if l != nil {
		if l.Parent() != p {
			t.Error(p, l)
		} else {
			checkRelate(l, tr, t)
		}
	}
	if r != nil {
		if r.Parent() != p {
			t.Error(p, r)
		} else {
			checkRelate(r, tr, t)
		}
	}
}

func TestNewTreeFnv(t *testing.T) {
	fn := fnv.New32()
	fn.Write([]byte{1, 2})

	hash := fn.Sum(nil)

	t.Log(hash)
}

func BenchmarkHasher(b *testing.B) {

	b.Run("sha256", func(b *testing.B) {

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			crypto.Keccak256Hash([]byte{1, byte(i % 256)})
		}
	})
	b.Run("fnv", func(b *testing.B) {
		fn := fnv.New32()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			fn.Write([]byte{1, 1})
			fn.Sum(nil)
		}
	})
	b.Run("fnv-address", func(b *testing.B) {
		data := append(common.StringToAddress("abc").Bytes(), common.StringToAddress("def").Bytes()...)
		fn := fnv.New32()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			fn.Write(data)
			fn.Sum(nil)
		}
	})
}

func TestCheckHashMemroy(t *testing.T) {

}
