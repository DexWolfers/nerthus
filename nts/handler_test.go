package nts

import (
	"fmt"
	"io/ioutil"
	"math/big"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/spf13/viper"

	"gitee.com/nerthus/nerthus/nts/pman"

	"gitee.com/nerthus/nerthus/log"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/consensus"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/crypto/sha3"
	"gitee.com/nerthus/nerthus/nts/protocol"
	"gitee.com/nerthus/nerthus/p2p"
	"gitee.com/nerthus/nerthus/p2p/discover"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/rlp"

	"github.com/stretchr/testify/require"

	cprotocol "gitee.com/nerthus/nerthus/consensus/protocol"
)

func startTowNode(t testing.TB, waitConnect bool) (*ProtocolManager, *ProtocolManager) {
	p1 := newNode(t)
	nodeA := newTestProtocolManager(p1, t)

	peerEvent := make(chan protocol.PeerEvent, 10)
	if waitConnect {
		nodeA.SubscribePeerEvent(peerEvent)
	}
	p2 := newNode(t, p1)
	nodeB := newTestProtocolManager(p2, nil)

	if waitConnect {
		select {
		case ev := <-peerEvent:
			peerAID := nodeB.PeerSelf().ID
			require.Equal(t, peerAID, ev.PeerID)
			require.True(t, ev.Connected)
		case <-time.After(time.Second * 5):
			require.Fail(t, "time out")
		}
	}
	return nodeA, nodeB
}

func TestConnect(t *testing.T) {
	nodeA, nodeB := startTowNode(t, false)

	peerEvent := make(chan protocol.PeerEvent, 10)
	nodeB.SubscribePeerEvent(peerEvent)

	select {
	case ev := <-peerEvent:
		peerAID := nodeA.PeerSelf().ID
		require.Equal(t, peerAID, ev.PeerID)
		require.True(t, ev.Connected)

		//如果停止PeerA，将接收到退出信号
		nodeA.Stop()
		select {
		case ev := <-peerEvent:
			require.Equal(t, peerAID, ev.PeerID)
			require.False(t, ev.Connected) //drop
		case <-time.After(time.Second * 2):
			require.Fail(t, "time out")
		}

	case <-time.After(time.Second * 2):
		require.Fail(t, "time out")
	}

	nodeA.Stop()
	nodeB.Stop()
}

func TestInvalidMsg(t *testing.T) {
	nodeA, nodeB := startTowNode(t, false)

	peerEvent := make(chan protocol.PeerEvent, 10)
	nodeB.SubscribePeerEvent(peerEvent)

	select {
	case ev := <-peerEvent:
		peerAID := nodeA.PeerSelf().ID
		require.Equal(t, peerAID, ev.PeerID)
		require.True(t, ev.Connected)

		err := nodeB.BroadcastMsg(protocol.BftMsg, "badMsg")
		require.NoError(t, err)

		//发送后将收到断开信号
		select {
		case ev := <-peerEvent:
			require.Equal(t, peerAID, ev.PeerID)
			require.False(t, ev.Connected) //drop
		case <-time.After(time.Second * 2):
			require.Fail(t, "time out")
		}

	case <-time.After(time.Second * 2):
		require.Fail(t, "time out")
	}

	nodeA.Stop()
	nodeB.Stop()
}

func TestPeerHandler_SendTransactions(t *testing.T) {

	log.Root().SetHandler(log.FuncHandler(func(r *log.Record) error {
		return nil
	}))

	nodeA, nodeB := startTowNode(t, true)
	defer nodeA.Stop()
	defer nodeB.Stop()

	txc := make(chan types.TxPreEvent, 10)
	nodeB.txpool.SubscribeTxPreEvent(txc)

	//从 A 广播，将被B收到
	count := 10

	txs := make([]*types.Transaction, count)
	read := make(chan struct{})
	//发送交易
	go func() {
		close(read)

		for i := 0; i < count; i++ {
			tx := types.NewTransaction(common.Address{}, common.StringToAddress(fmt.Sprintf("to_%d", i)),
				big.NewInt(9999), params.MinGasLimit, 1, 0, nil)
			nodeA.BroadcastTxs(tx)

			nodeA.txpool.AddRemotes([]*types.Transaction{tx})

			txs[i] = tx
		}
	}()
	<-read

	var got sync.WaitGroup
	got.Add(count)
	go func() {
		index := 0
		for {
			// 等待收到
			select {
			case ev := <-txc:
				_ = ev
				index++
				got.Done()
				if index >= count {
					return
				}
			case <-time.After(time.Second * 2):
				require.Fail(t, "time out")

			}
		}
	}()
	got.Wait()
}

func TestPeerHandler_SendWitnessStatusMsg(t *testing.T) {
	nodeA, nodeB := startTowNode(t, true)
	defer nodeA.Stop()
	defer nodeB.Stop()

	var wg sync.WaitGroup
	wg.Add(1)
	nodeB.onlyUseInTest = func(_ *pman.Peer, message p2p.Msg, result error) {
		require.Equal(t, protocol.WitnessStatusMsg, uint8(message.Code))
		wg.Done()
	}

	nodeA.eventMux.Post(&types.SendDirectEvent{
		PeerID: nodeB.PeerSelf().ID,
		Code:   protocol.WitnessStatusMsg,
		Data:   []byte{1},
	})

	if waitTimeout(&wg, time.Second) {
		require.Fail(t, "time out")
	}
}

func TestPeerHandler_SendUnitMsg(t *testing.T) {
	nodeA, nodeB := startTowNode(t, true)
	defer nodeA.Stop()
	defer nodeB.Stop()

	unit := types.NewUnit(&types.Header{
		MC:     common.StringToAddress("chain"),
		Number: 1,
	}, nil, nil)

	var wg sync.WaitGroup
	wg.Add(1)

	nodeB.onlyUseInTest = func(_ *pman.Peer, message p2p.Msg, result error) {
		require.EqualError(t, result, consensus.ErrUnknownAncestor.Error())
		require.Equal(t, protocol.NewUnitMsg, uint8(message.Code))
		wg.Done()
	}

	nodeA.BroadcastUnit(unit)

	if waitTimeout(&wg, time.Second) {
		require.Fail(t, "time out")
	}
}

func TestSendTTLMsg(t *testing.T) {
	nodeA, nodeB := startTowNode(t, true)
	defer nodeA.Stop()
	defer nodeB.Stop()

	p3 := newNode(t, nodeB.P2PSrv, nodeA.P2PSrv)
	nodeC := newTestProtocolManager(p3, nil)
	peerEvent := make(chan protocol.PeerEvent, 10)
	nodeC.SubscribePeerEvent(peerEvent)
	<-peerEvent //等待A和B 与 C连接成功
	<-peerEvent

	// 应该只有 B和C 收到消息，但有可能B和C相互发消息，所以消息分别是：
	//  1.   A --> B
	//  2.   A --> C
	//  3.   B --> C (可能)
	//  4.   C --> B (可能)
	wantRute := map[string]int{
		"A->B": 0,
		"A->C": 0,
		"B->C": 0,
		"C->A": 0,
	}
	peerIds := map[discover.NodeID]string{
		nodeA.PeerSelf().ID: "A",
		nodeB.PeerSelf().ID: "B",
		nodeC.PeerSelf().ID: "C",
	}

	t.Log("peers", peerIds)
	var lock sync.Mutex
	gotMsg := func(from, to string, message p2p.Msg, result error) {
		require.Equal(t, protocol.WitnessStatusMsg, uint8(message.Code), "%v", message)
		require.NotZero(t, message.TTL, "%v", message)
		require.NotZero(t, message.Expiry, "%v", message)

		rute := fmt.Sprintf("%s->%s", from, to)

		lock.Lock()
		defer lock.Unlock()
		if v, ok := wantRute[rute]; !ok {
			require.Failf(t, "unexpected message route", "%s", rute)
		} else {
			if v > 0 {
				require.Failf(t, "should be received one times", "%s", rute)
			}
			//登记
			wantRute[rute] = wantRute[rute] + 1
		}
	}

	// A 向 B发送后，B和C均可接受到，且B还发转发给A和B
	nodeA.onlyUseInTest = func(p *pman.Peer, message p2p.Msg, result error) {
		gotMsg(peerIds[p.ID()], "A", message, result)

	}
	nodeB.onlyUseInTest = func(p *pman.Peer, message p2p.Msg, result error) {
		gotMsg(peerIds[p.ID()], "B", message, result)
	}
	nodeC.onlyUseInTest = func(p *pman.Peer, message p2p.Msg, result error) {
		gotMsg(peerIds[p.ID()], "C", message, result)
	}

	nodeA.eventMux.Post(&types.SendBroadcastEvent{
		Data: []byte{1, 2, 3},
		Code: protocol.WitnessStatusMsg,
	})

	time.Sleep(time.Second * 5)
}

func TestPeerHandler_MessageHash(t *testing.T) {
	nodeA, nodeB := startTowNode(t, true)
	defer nodeA.Stop()
	defer nodeB.Stop()

	data := []byte{1, 2, 3}

	var wg sync.WaitGroup
	wg.Add(1)

	nodeB.onlyUseInTest = func(p *pman.Peer, message p2p.Msg, result error) {
		require.NoError(t, result)
		require.Equal(t, protocol.WitnessStatusMsg, uint8(message.Code))

		hw := sha3.NewKeccak256()
		b, err := ioutil.ReadAll(message.Payload)
		require.NoError(t, err)
		_, err = hw.Write(b)
		require.NoError(t, err)
		var hash common.Hash
		t.Log(hw.Sum(hash[:0]))
		t.Log("get message hash:", hash.Hex())

		var got []byte
		err = rlp.DecodeBytes(b, &got) //能解码
		require.NoError(t, err)
		require.Equal(t, data, got)

		wg.Done()

	}
	nodeA.eventMux.Post(&types.SendBroadcastEvent{
		Data: data,
		Code: protocol.WitnessStatusMsg,
	})

	if waitTimeout(&wg, time.Second) {
		require.Fail(t, "time out")
	}
}

//
//func Test_MessageHash(t *testing.T) {
//	unit := types.NewUnit(&types.Header{
//		MC:     common.StringToAddress("chain"),
//		Number: 1,
//	}, nil, nil)
//
//	_, r, err := rlp.EncodeToReader(unit)
//	require.NoError(t, err)
//
//	b, err := ioutil.ReadAll(r)
//	require.NoError(t, err)
//
//	hash, err := messagePayloadHash(b)
//	require.NoError(t, err)
//	require.Equal(t, unit.Hash().Hex(), hash.Hex())
//}

func TestPeerInterface(t *testing.T) {
	p1 := newNode(t)
	nodeA := newTestProtocolManager(p1, nil)
	p := nodeA.Peer(discover.NodeID{})
	if p != nil {
		t.Fatal("need nil")
	}
}

func TestSendVoteMsg(t *testing.T) {

	nodeA, nodeB := startTowNode(t, true)
	defer nodeA.Stop()
	defer nodeB.Stop()

	vote := types.WitnessReplaceVoteEvent{
		Round: 100,
		UnitId: types.UnitID{
			ChainID: common.StringToAddress("123"),
			Height:  121,
			Hash:    common.StringToHash("121"),
		},
	}
	err := types.SignBySignHelper(&vote, types.NewSigner(big.NewInt(1)), testBankKey)
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(1)

	nodeB.onlyUseInTest = func(_ *pman.Peer, message p2p.Msg, result error) {
		require.Equal(t, protocol.ChooseLastUnitMsg, uint8(message.Code))
		wg.Done()
	}

	err = nodeA.SendMsg(nodeB.PeerSelf().ID, protocol.ChooseLastUnitMsg, &vote)
	require.NoError(t, err)
	if waitTimeout(&wg, 5*time.Second) {
		require.Fail(t, "time out")
	}
}

func BenchmarkProtocolManager_SendBFT(b *testing.B) {
	log.Root().SetHandler(log.FuncHandler(func(r *log.Record) error {
		return nil
	}))

	nodeA, nodeB := startTowNode(b, true)
	defer nodeA.Stop()
	defer nodeB.Stop()

	count := b.N * 2

	sub := nodeB.eventMux.Subscribe(cprotocol.MessageEvent{})
	defer sub.Unsubscribe()

	var got sync.WaitGroup
	got.Add(count)

	b.ReportAllocs()
	b.ResetTimer()

	go func() {
		for ev := range sub.Chan() {
			if ev.Time.IsZero() {
				b.Fatal("failed insert")
			}
			got.Done()
		}
	}()

	//从 A 广播，将被B收到
	//发送交易
	var err error
	for i := 0; i < count; i++ {
		data := cprotocol.BFTMsg{
			ChainAddress: common.StringToAddress("abc"),
			Create:       time.Now(),
		}
		msgPayload := make([]byte, 1024*2)
		rand.Read(msgPayload)
		msg := cprotocol.Message{
			Code: cprotocol.MsgCommit,
			Msg:  msgPayload,
		}
		data.Payload, err = msg.Payload()
		require.NoError(b, err)

		err = nodeA.BroadcastMsg(protocol.BftMsg, data)
		require.NoError(b, err)
	}

	got.Wait()
}

func TestConnectEvent(t *testing.T) {
	log.Root().SetHandler(log.FuncHandler(func(r *log.Record) error {
		return nil
	}))

	nodeA, nodeB := startTowNode(t, false)

	peerEvent := make(chan protocol.PeerEvent, 10)
	nodeB.SubscribePeerEvent(peerEvent)
	nodeA.SubscribePeerEvent(peerEvent)

	for {
		select {
		case ev := <-peerEvent:
			t.Log(ev.PeerID.String(), ev.Connected)

		case <-time.After(time.Second * 2):
			return
		}
	}

	nodeA.Stop()
	nodeB.Stop()
}

func TestMsgHandleConfig(t *testing.T) {

	viper.Set("runtime.p2p_msg_parallel_bft", 2)
	viper.Set("runtime.p2p_msg_cache_bft", 1024)

	p, c := getMsgSetting(protocol.BftMsg)
	require.Equal(t, 2, p, "should be equal config item")
	require.Equal(t, 1024, c, "should be equal config item")

	viper.Set("runtime.p2p_msg_parallel_tx", 3)
	viper.Set("runtime.p2p_msg_cache_tx", 3000)

	p, c = getMsgSetting(protocol.TxMsg)
	require.Equal(t, 3, p, "should be equal config item")
	require.Equal(t, 3000, c, "should be equal config item")

	//默认值
	p, c = getMsgSetting(protocol.NewUnitMsg)
	require.Equal(t, defaultMsgSetting[0].parallel, p, "should be equal default value")
	require.Equal(t, defaultMsgSetting[0].cacheSize, c, "should be equal default value")
}
