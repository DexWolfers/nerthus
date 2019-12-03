package wconn

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"gitee.com/nerthus/nerthus/common"

	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/p2p/discover"
)

func NewDB() (db ntsdb.Database, close func()) {
	db = ntsdb.NewMemDB()
	return db, func() {
		db.Close()
	}
}

func TestWriteWintessNodeInfo(t *testing.T) {

	db, close := NewDB()
	defer close()

	node := WitnessNode{
		Witness: common.StringToAddress("witnessA"),
		Version: math.MaxUint64,
		Enode: *discover.MustParseNode(
			"enode://e2d822ea16ebef6a138583f5b60bd6fd0dc3d2430fa9c603d204bc6561e7ac909f1e5255c103079b608ab3d7222da3259f7651c44fd2a518cbc88a1a6bc65019@192.168.11.2:61111"),
	}
	node.SetSign([]byte("sign info"))

	got := GetWintessNodeInfo(db, node.Witness)
	require.Nil(t, got)

	WriteWintessNodeInfo(db, node)
	got = GetWintessNodeInfo(db, node.Witness)
	require.NotNil(t, got)
	require.Equal(t, node, *got)

	DelWitnessNodeInfo(db, node.Witness)
	got = GetWintessNodeInfo(db, node.Witness)
	require.Nil(t, got)
}

func TestGetWitnessNodeInfo(t *testing.T) {

	db, close := NewDB()
	defer close()

	items := []WitnessNode{
		WitnessNode{
			Witness: common.StringToAddress("witnessA"),
			Version: 1,
			Enode: *discover.MustParseNode(
				"enode://e2d822ea16ebef6a138583f5b60bd6fd0dc3d2430fa9c603d204bc6561e7ac909f1e5255c103079b608ab3d7222da3259f7651c44fd2a518cbc88a1a6bc65019@192.168.11.2:61111"),
		},
		WitnessNode{
			Witness: common.StringToAddress("witnessB"),
			Version: 2,
			Enode: *discover.MustParseNode(
				"enode://e2d822ea16ebef6a138583f5b60bd6fd0dc3d2430fa9c603d204bc6561e7ac909f1e5255c103079b608ab3d7222da3259f7651c44fd2a518cbc88a1a6bc65019@192.168.11.3:61111"),
		},
		WitnessNode{
			Witness: common.StringToAddress("witnessC"),
			Version: 3,
			Enode: *discover.MustParseNode(
				"enode://e2d822ea16ebef6a138583f5b60bd6fd0dc3d2430fa9c603d204bc6561e7ac909f1e5255c103079b608ab3d7222da3259f7651c44fd2a518cbc88a1a6bc65019@192.168.11.4:61111"),
		},
	}
	for i := range items {
		items[i].SetSign([]byte{1, 2})
		WriteWintessNodeInfo(db, items[i])
	}

	list, err := GetAllWitnessNodeInfo(db)
	require.NoError(t, err)
	require.Len(t, list, len(items))
	for i := 0; i < len(list); i++ {
		require.Equal(t, items[i], *list[i])
	}

}
