package realtime

import (
	"fmt"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/testutil"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/nts/synchronize/message"
	"gitee.com/nerthus/nerthus/nts/synchronize/realtime/merkle"
	"gitee.com/nerthus/nerthus/p2p/discover"
	"math/big"
	"net/http"
	_ "net/http/pprof"
	"testing"
	"time"
)

func TestTwoTree(t *testing.T) {
	cts := make([]merkle.Content, 0)
	cts2 := make([]merkle.Content, 0)
	for i := 1; i <= 3; i++ {
		addr := common.BigToAddress(big.NewInt(int64((i))))
		cts = append(cts, ChainStatus{Addr: addr, Number: uint64(i + 1)})
		cts2 = append(cts2, ChainStatus{Addr: addr, Number: uint64(i + 1)})
	}
	tree1 := merkle.NewTreeAppend(cts, nil)
	t.Log(tree1.Root().Hash())
	tree2 := merkle.NewTreeAppend(cts2, nil)
	t.Log(tree2.Root().Hash())
	if tree1.Root().Hash() != tree2.Root().Hash() {
		t.Error("two tree root difference", tree1.Root().Hash(), tree2.Root().Hash())
	}
}
func TestCmp(t *testing.T) {
	go http.ListenAndServe("127.0.0.1:6060", nil)
	testutil.SetupTestLogging()
	chains1 := make([]ChainStatus, 0)
	chains2 := make([]ChainStatus, 0)
	for i := 0; i < 1000; i++ {
		addr := ChainStatus{Addr: common.BigToAddress(big.NewInt(int64((i + 1)))), Number: uint64(i + 1)}
		chains1 = append(chains1, addr)
		if i < 800 {
			chains2 = append(chains2, addr)
			if i%6 == 0 && i > 0 {
				chains2[i].Number = uint64(i - 3)
			}
		}
		if i == 0 {
			continue
		}
		if i%5 == 0 {
			chains1[i].Number = uint64(i * 2)
		} else if i%6 == 0 && i < len(chains2) {
			chains2[i].Number = uint64(i / 2)
		}
		fmt.Printf("%s  ", addr.Addr.Hex())
		if i < len(chains1) {
			fmt.Printf("peer1>%d  ", chains1[i].Number)
		}
		if i < len(chains2) {
			fmt.Printf("peer2>%d  ", chains2[i].Number)
		}
		fmt.Println()
	}
	fmt.Println("------------------------------------------------------------")
	// 第一个节点
	peer1 := newPeer(1, chains1)
	// 第二个节点
	peer2 := newPeer(2, chains2)

	// 同步
	peer1.backed.syncChain = func(diffs map[common.Address]uint64, peer message.PeerID) {

		temp1(peer1.cmptor.tree.Root(), t)
		temp1(peer2.cmptor.tree.Root(), t)
		syncChains(diffs, peer1, peer2)
	}
	peer2.backed.syncChain = func(diffs map[common.Address]uint64, peer message.PeerID) {
		syncChains(diffs, peer2, peer1)
	}

	// 模拟通信
	go func() {
		for {
			select {
			case ev := <-peer1.backed.msgCh:
				if !peer2.cmptor.started {
					time.Sleep(time.Second)
				}
				peer2.cmptor.handleResponse((ev))
			case ev := <-peer2.backed.msgCh:
				if !peer1.cmptor.started {
					time.Sleep(time.Second)
				}
				peer1.cmptor.handleResponse(ev)
			}
		}
	}()

	// 模拟一次差异消息，来启动对比
	msg := &RealtimeEvent{
		Code:         RealtimeReponse,
		From:         peer2.backed.peer,
		RemoteStatus: NodeStatus{Root: peer2.cmptor.tree.Root().Hash(), Chains: uint64(len(peer2.backed.chains))},
	}
	peer1.cmptor.start(msg, peer1.quit)

	msg2 := &RealtimeEvent{
		Code:         RealtimeRequest,
		From:         peer1.backed.peer,
		RemoteStatus: NodeStatus{Root: peer1.cmptor.tree.Root().Hash(), Chains: uint64(len(peer1.backed.chains))},
	}
	peer2.cmptor.start(msg2, peer2.quit)

	<-peer1.quit
	<-peer2.quit

	r1, r2 := peer1.cmptor.tree.Root().Hash(), peer2.cmptor.tree.Root().Hash()
	//t.Log("peer1 root",r1,"peer2 root",r2)
	if r1 != r2 {
		t.Error("root diff", r1.Hex(), r2.Hex())
		fmt.Println("---------------------peer1 tree leafs-----------------------")
		for _, v := range peer1.cmptor.tree.LeafsAt(0) {
			fmt.Println(v)
		}
		fmt.Println("---------------------peer2 tree leafs-----------------------")
		for _, v := range peer2.cmptor.tree.LeafsAt(0) {
			fmt.Println(v)
		}
		for _, v := range peer1.cmptor.tree.LeafsAt(0) {
			for _, v2 := range peer2.cmptor.tree.LeafsAt(0) {
				if v.Cmp(v2) == 0 {
					if v.(ChainStatus).Number != v2.(ChainStatus).Number {
						t.Error(v, v2)
					}
					break
				}
			}
		}

		//temp1(peer1.cmptor.tree.Root(), t)
		//temp1(peer2.cmptor.tree.Root(), t)

		temp2 := make([][]merkle.Node, 2)
		cmp(peer1.cmptor.tree.Root(), peer2.cmptor.tree.Root(), temp2)
		t.Log(temp2)
	}
}
func temp1(p merkle.Node, t *testing.T) {
	if len(p.Children()) == 0 {
		return
	}
	cHash := p.Children()[0].Hash()
	if len(p.Children()) == 2 {
		cHash = merkle.HashWith(p.Children()[0], p.Children()[1])
	} else if len(p.Children()) == 1 {
		cHash = p.Children()[0].Hash()
	} else {
		cHash = p.Children()[0].Content().Hash()
	}
	if cHash != p.Hash() {
		t.Error(cHash, p)
	}
	for i := range p.Children() {
		if p.Children()[i].Parent() != p {
			t.Error(p, p.Children()[i])
		} else {
			temp1(p.Children()[i], t)
		}
	}
}

func syncChains(diffs map[common.Address]uint64, from, to *testPeer) {
	from.cmptor.tree.Root().Hash()
	//log.Debug("before sync", "tree1", from.cmptor.tree.Root(), "tree2", to.cmptor.tree.Root())
	for k, _ := range diffs {
		//log.Debug("sync chain number>>>>>>>>>>", "from", from.backed.peer.ID(), "to", to.backed.peer.ID(), "chain", k, "number", v)
		for _, c := range from.backed.chains {
			if k.Equal(c.Addr) {
				to.cmptor.tree.Update(c)
			}
		}
	}
}

func newPeer(id int, chains []ChainStatus) *testPeer {
	peer1 := testPeer{}
	pName := fmt.Sprintf("%d", id)
	peer1.quit = make(chan struct{})
	peer1.backed = &TestBackend{
		chains: chains,
		peer:   discover.NodeID{byte(id)},
		msgCh:  make(chan *RealtimeEvent, 1024*1024),
	}
	tree1 := merkle.NewTreeAppend(peer1.backed.GetChainsStatus(), nil)
	peer1.cmptor = newTreeCmptor(tree1, peer1.backed)
	peer1.cmptor.logger = log.Root().New("node", pName)
	return &peer1
}

type testPeer struct {
	backed *TestBackend
	cmptor *treeCmptor
	quit   chan struct{}
}

func cmp(c1, c2 merkle.Node, diff [][]merkle.Node) {
	if len(c1.Children()) == 0 {
		diff[0] = append(diff[0], c1)
		diff[1] = append(diff[1], c2)
		return
	}
	if len(c1.Children()) != len(c2.Children()) {
		panic("children len diff")
	}
	if len(c1.Children()) > 2 {
		panic("len wrong")
	}
	for i := range c1.Children() {
		if c1.Children()[i].Hash() != c2.Children()[i].Hash() {
			fmt.Print(i)
			cmp(c1.Children()[i], c2.Children()[i], diff)
		} else {
			log.Debug(">>>", "c1", c1.Children()[i], "c2", c2.Children()[i])
			//fmt.Print("0")
		}
	}
}
