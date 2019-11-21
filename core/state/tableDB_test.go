package state

import (
	"testing"

	"gitee.com/nerthus/nerthus/ntsdb"

	"gitee.com/nerthus/nerthus/common"
	"github.com/stretchr/testify/require"
)

func TestDBTable(t *testing.T) {

	db, _ := ntsdb.NewMemDatabase()

	table := newTable(db, common.Hex2Bytes("nsc1qyqszq0xz2kr7hnwqujwa6z97m382vysnt75v3"))

	type Item struct {
		Key, Value []byte
	}

	items := []Item{
		{[]byte("k1"), []byte("v1")},
		{[]byte("k2"), []byte("v2")},
		{[]byte("k3"), []byte("v3")},
		{[]byte("k3"), []byte("v3_2")}, //重复
	}

	// read
	for _, item := range items {
		got, err := table.Get(item.Key)
		require.Error(t, err, "not found")
		require.Empty(t, got)
	}

	for _, item := range items {
		//put
		err := table.Put(item.Key, item.Value)
		require.NoError(t, err)
		//get
		got, err := table.Get(item.Key)
		require.NoError(t, err)
		require.Equal(t, item.Value, got)
	}

	for _, item := range items {
		err := table.Delete(item.Key)
		require.NoError(t, err)
		got, err := table.Get(item.Key)
		require.Error(t, err, "not found")
		require.Empty(t, got)
	}
}
func TestValuePrefix(t *testing.T) {
	db, _ := ntsdb.NewMemDatabase()

	table := newTable(db, common.Hex2Bytes("nsc1qyqszq0xz2kr7hnwqujwa6z97m382vysnt75v3"))
	table.SetValuePrefix(88)

	type Item struct {
		Key, Value []byte
	}
	items := []Item{
		{[]byte("k1"), []byte("v1")},
		{[]byte("k2"), []byte("v2")},
		{[]byte("k3"), []byte("v3")},
		{[]byte("k3"), []byte("v3_2")}, //重复
	}

	for _, item := range items {
		//put
		err := table.Put(item.Key, item.Value)
		require.NoError(t, err)
		//get
		got, err := table.Get(item.Key)
		require.NoError(t, err)
		require.Equal(t, item.Value, got)

		prefix, v, err := table.GetPrefixValue(item.Key)
		t.Log(prefix, v, err)
	}
}
