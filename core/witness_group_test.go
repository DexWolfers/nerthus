package core

import (
	"bytes"
	"net"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/p2p/discover"
	"github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
)

func TestAddrNodeListRlp(t *testing.T) {
	nodeId, err := discover.BytesID(bytes.Repeat([]byte("1"), 64))
	require.Nil(t, err)
	var addrNodeList AddrNodeList
	addrNodeList.Add(&AddrNode{
		Addr:      common.StringToAddress("11111"),
		Node:      discover.NewNode(nodeId, net.IPv4(12, 34, 56, 78), 123, 23),
		Type:      AddrNodeTypeStart,
		PublicKey: []byte("123fdsafds"),
	})
	addrNodeList.Add(&AddrNode{
		Addr:      common.StringToAddress("2222"),
		Node:      nil,
		Type:      AddrNodeTypeStart,
		PublicKey: []byte("fsdagreqte432142"),
	})

	convey.Convey("AddrNode rlp", t, func() {
		for _, v := range addrNodeList.List() {
			b, err := v.Rlp()
			require.Nil(t, err)
			t.Log(v)
			var addrNode2 AddrNode
			err = addrNode2.UnRlp(b)
			require.Nil(t, err)
			require.Equal(t, v.Addr, addrNode2.Addr)
			require.Equal(t, v.PublicKey, addrNode2.PublicKey)
			require.Equal(t, v.Node, addrNode2.Node)
		}
	})

	convey.Convey("addrNodeList rlp", t, func() {
		b, err := addrNodeList.Rlp()
		require.Nil(t, err)
		var addrNodeList2 AddrNodeList
		err = addrNodeList2.UnRlp(b)
		require.Nil(t, err)
		for i := range addrNodeList.List() {
			require.Equal(t, addrNodeList.List()[i].Addr, addrNodeList2.List()[i].Addr)
			require.Equal(t, addrNodeList.List()[i].PublicKey, addrNodeList2.List()[i].PublicKey)
			require.Equal(t, addrNodeList.List()[i].Node, addrNodeList2.List()[i].Node)
		}
	})
}

func TestNewAddrNode(t *testing.T) {

	t.Log(time.Now().Hour())
}
