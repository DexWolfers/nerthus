package wconn

import (
	"crypto/ecdsa"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common/math"

	rlp2 "gitee.com/nerthus/nerthus/rlp"

	"gitee.com/nerthus/nerthus/common/testutil"
	"gitee.com/nerthus/nerthus/log"

	"gitee.com/nerthus/nerthus/accounts"

	"gitee.com/nerthus/nerthus/accounts/keystore"

	"gitee.com/nerthus/nerthus/crypto"
	"github.com/stretchr/testify/require"

	"gitee.com/nerthus/nerthus/p2p"

	"github.com/pkg/errors"

	"gitee.com/nerthus/nerthus/core/types"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/nts/protocol"
	"gitee.com/nerthus/nerthus/p2p/discover"
	"gitee.com/nerthus/nerthus/params"
)

type TestPeer struct {
	asyncSendMessage func(code uint8, data interface{}) error
	sendTTLMsg       func(msg protocol.TTLMsg) error

	closed func() bool
	id     discover.NodeID
}

func (p *TestPeer) AsyncSendMessage(code uint8, data interface{}) error {
	if p.asyncSendMessage != nil {
		return p.asyncSendMessage(code, data)
	}
	return errors.New("implement me")
}

func (p *TestPeer) SendTTLMsg(msg protocol.TTLMsg) error {
	if p.sendTTLMsg != nil {
		return p.sendTTLMsg(msg)
	}
	return errors.New("implement me")
}

func (p *TestPeer) Closed() bool {
	if p.closed != nil {
		return p.closed()
	}
	return false
}

func (p *TestPeer) ID() discover.NodeID {
	return p.id
}

func (p *TestPeer) SetWitnessType() {

}

func (p *TestPeer) Close(reason p2p.DiscReason) {

}

func (p *TestPeer) SendTransactions(txs types.Transactions) error {
	panic("implement me")
}

type TestPeerSet struct {
	myself       *TestWitnessInfo
	keystore     *keystore.KeyStore
	allWitness   []*TestPeerSet
	connectPeers []*TestPeer
	conns        *WitnessGroupConnControl
	port         int

	feed event.Feed
}

func (ps *TestPeerSet) WitnessGroup() uint64 {
	return 2
}

func (ps *TestPeerSet) Address() common.Address {
	return ps.myself.account.Address
}

func (ps *TestPeerSet) ENode() discover.Node {
	return *discover.MustParseNode(fmt.Sprintf("enode://%s@127.0.0.1:%d", ps.myself.enode.String(), ps.port))
}

func (ps *TestPeerSet) Sign(info types.SignHelper) error {
	_, err := ps.keystore.SignThat(ps.myself.account, info, ps.Config().ChainId)
	return err
}

func (ps *TestPeerSet) Encrypt(pubkey []byte, data []byte) ([]byte, error) {
	return ps.keystore.Encrypt(pubkey, data)
}

func (ps *TestPeerSet) Decrypt(data []byte) ([]byte, error) {
	return ps.keystore.Decrypt(ps.myself.account, data)
}

func (ps *TestPeerSet) Config() *params.ChainConfig {
	return params.MainnetChainConfig
}

func (ps *TestPeerSet) GetWitnessAtGroup(gp uint64) (common.AddressList, error) {
	if gp != ps.WitnessGroup() {
		panic("bad witenss group")
	}
	var list []common.Address
	for _, w := range ps.allWitness {
		list = append(list, w.myself.account.Address)
	}
	return list, nil
}

func (ps *TestPeerSet) GetWitnessInfo(address common.Address) (*sc.WitnessInfo, error) {
	for _, w := range ps.allWitness {
		if w.myself.info.Address == address {
			return w.myself.info, nil
		}
	}
	return nil, errors.New("not found")
}

func (ps *TestPeerSet) Peer(id discover.NodeID) protocol.Peer {
	for _, w := range ps.connectPeers {
		if id == w.ID() {
			return w
		}
	}
	return nil
}

func (ps *TestPeerSet) Peers() (list []protocol.Peer) {
	for _, w := range ps.connectPeers {
		list = append(list, w)
	}
	return list
}

func (ps *TestPeerSet) AddPeer(node *discover.Node) error {
	go func() {
		for _, w := range ps.connectPeers {
			if w.ID() == node.ID {
				return
			}
		}
		for _, w := range ps.allWitness {
			if w.myself.enode == node.ID {
				ps.AddConn(w.myself.enode, w.conns)
			}
		}
	}()
	return nil
}

func (ps *TestPeerSet) ClearWhitelist(id discover.NodeID) {

}

func (ps *TestPeerSet) JoinWhitelist(id discover.NodeID) {

}

func (ps *TestPeerSet) SubscribePeerEvent(ch chan protocol.PeerEvent) event.Subscription {
	fmt.Println("---sub")
	return ps.feed.Subscribe(ch)
}

func (ps *TestPeerSet) AddConn(id discover.NodeID, added *WitnessGroupConnControl) {
	peer := TestPeer{
		id: id,
		sendTTLMsg: func(msg protocol.TTLMsg) error {
			return added.Handle(ps.myself.enode, msg)
		},
		asyncSendMessage: func(code uint8, data interface{}) error {
			b, err := rlp2.EncodeToBytes(data)
			if err != nil {
				panic(err)
			}
			return added.Handle(ps.myself.enode, b)
		},
	}
	ps.connectPeers = append(ps.connectPeers, &peer)
	ps.feed.Send(protocol.PeerEvent{
		Connected: true,
		PeerID:    id,
		Peer:      &peer,
	})
	log.Debug("add conn", "node", id)
}

type TestWitnessInfo struct {
	account accounts.Account
	info    *sc.WitnessInfo
	enode   discover.NodeID
	peer    *TestPeer

	prikey *ecdsa.PrivateKey
}

func createWitnessInfo(t *testing.T, addr common.Address, ks *keystore.KeyStore) *TestWitnessInfo {
	acct, err := ks.Find(accounts.Account{Address: addr})
	require.NoError(t, err)

	p, err := ks.GetPrivateKeyByAuth(acct, "")
	require.NoError(t, err)
	ks.Unlock(acct, "")

	info := sc.WitnessInfo{
		Address: acct.Address, Pubkey: crypto.FromECDSAPub(&p.PublicKey),
	}

	peer := TestPeer{
		id: discover.PubkeyID(&p.PublicKey),
	}
	return &TestWitnessInfo{
		account: acct,
		info:    &info,
		enode:   peer.ID(),
		prikey:  p,
		peer:    &peer,
	}
}

func TestWitnessConns_Handle(t *testing.T) {
	testutil.EnableLog(log.LvlTrace, "")

	db, close := NewDB()
	defer close()

	tmp, err := ioutil.TempDir("", "nerthustest")
	require.NoError(t, err)
	defer os.Remove(tmp)
	ks := keystore.NewKeyStore(tmp, keystore.StandardScryptN, keystore.StandardScryptP)

	//创建账户
	createAcct := func() common.Address {
		acct, err := ks.NewAccount("")
		require.NoError(t, err)
		ks.Unlock(acct, "")
		return acct.Address
	}

	createConn := func() *TestPeerSet {
		w := createAcct()
		info := createWitnessInfo(t, w, ks)
		help := TestPeerSet{
			myself:   info,
			keystore: ks,
			port:     len(ks.Accounts()),
		}
		help.conns = New(db, &help, &help)

		return &help
	}

	one := createConn()
	two := createConn()
	three := createConn()

	func(list ...*TestPeerSet) {
		for _, w := range list {
			w.allWitness = list
			err := w.conns.Start(w)
			require.NoError(t, err)

			//检查信息
			require.NotZero(t, w.conns.witnessGroup.MeInfo.Version)
		}

	}(one, two, three)

	one.AddConn(two.myself.enode, two.conns)
	one.AddConn(three.myself.enode, three.conns)

	info, err := one.conns.GetEncryptConnectInfo()
	require.NoError(t, err)
	require.Len(t, info.Data, 2)

	time.Sleep(5 * time.Second)
	require.Equal(t, 2, one.conns.connectedWitness.Len(), "me is %s", one.myself.enode)
}

func TestWitnessconnectinfo_RLP(t *testing.T) {

	msg := WitnessConnectMsg{
		WitnessGroup: 10,
		Version:      123456,
		Data: []*EncryptData{
			{Receiver: common.StringToAddress("toA"), Data: []byte{1, 2, 4}},
			{Receiver: common.StringToAddress("toB"), Data: []byte{1, 2, 4, 5, 6}},
		},
	}
	msg.SetSign([]byte{1, 3, 5, 7})

	b, err := rlp2.EncodeToBytes(&msg)
	require.NoError(t, err)

	var got WitnessConnectMsg
	err = rlp2.DecodeBytes(b, &got)
	require.NoError(t, err)

	require.Equal(t, msg, got)

}

// 检查将信息机密后总体数据大小，如果数据太大不适合网络发送
func TestCheckWitnessNodeInfoSize(t *testing.T) {
	tmp, err := ioutil.TempDir("", "nerthustest")
	require.NoError(t, err)
	defer os.Remove(tmp)
	ks := keystore.NewKeyStore(tmp, keystore.StandardScryptN, keystore.StandardScryptP)
	acc, err := ks.NewAccount("pwd")
	require.NoError(t, err)
	ks.Unlock(acc, "pwd")

	nodeInfo := &WitnessNode{
		Version: uint64(time.Now().Unix()),
		Enode: *discover.MustParseNode(
			"enode://332c981039b42d1f4e1d7de9a333ab0995a679f0a0a43ed076360c438a925b92280a89211432d47325648dfd81ad1e268589072ca5c27856fdbfd3ab8bb6ba6a@221.122.35.6:60101"),
		Witness:    acc.Address,
		UpdateTime: time.Now(),
	}

	//singer:=types.NewSigner(big.NewInt(1907))
	_, err = ks.SignThat(acc, nodeInfo, big.NewInt(1907))
	require.NoError(t, err)

	nodeData, err := rlp2.EncodeToBytes(nodeInfo)
	require.NoError(t, err)

	msg := WitnessConnectMsg{
		WitnessGroup: math.MaxUint64 / 2,
		Version:      math.MaxUint64,
	}

	for i := 0; i < 10; i++ {
		p, _ := crypto.GenerateKey()
		b, err := ks.Encrypt(crypto.FromECDSAPub(&p.PublicKey), nodeData)
		require.NoError(t, err)

		msg.Data = append(msg.Data, &EncryptData{
			Receiver: crypto.ForceParsePubKeyToAddress(p.PublicKey),
			Data:     b,
		})
	}

	_, err = ks.SignThat(acc, &msg, big.NewInt(1907))
	require.NoError(t, err)

	sendData, err := rlp2.EncodeToBytes(&msg)
	require.NoError(t, err)

	t.Logf("data size: %s", common.StorageSize(len(sendData)))
}
