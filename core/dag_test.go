package core

import (
	"testing"

	"gitee.com/nerthus/nerthus/common"

	"github.com/stretchr/testify/require"

	"gitee.com/nerthus/nerthus/ntsdb"
)

func TestDAG_IsBadUnit(t *testing.T) {
	db, _ := ntsdb.NewMemDatabase()

	u := func(id string) common.Hash {
		return common.StringToHash(id)
	}
	//write bad
	db.Put(badUnitKey(u("badUnit1")), []byte{1})
	db.Put(badUnitKey(u("badUnit2")), []byte{1})

	dag, err := NewDag(db, nil, nil)
	require.NoError(t, err)

	require.True(t, dag.IsBadUnit(u("badUnit1")))
	require.True(t, dag.IsBadUnit(u("badUnit2")))
	require.False(t, dag.IsBadUnit(u("badUnit3")))

	//add
	dag.writeBadUnit(u("badUnit3"))
	require.True(t, dag.IsBadUnit(u("badUnit3")))

	//delete
	require.False(t, dag.IsBadUnit(u("badUnit4")))
	dag.removeBad(u("badUnit4"))
	require.False(t, dag.IsBadUnit(u("badUnit4")))

	//add and remove
	require.False(t, dag.IsBadUnit(u("badUnit5")))
	dag.writeBadUnit(u("badUnit5"))
	require.True(t, dag.IsBadUnit(u("badUnit5")))
	dag.removeBad(u("badUnit5"))
	require.False(t, dag.IsBadUnit(u("badUnit5")))

}

func TestDAG_ChainIsInInArbitration(t *testing.T) {
	db, _ := ntsdb.NewMemDatabase()

	u := func(id string) common.Address {
		return common.StringToAddress(id)
	}
	//write bad
	db.Put(getChainArbitrationInKey(u("badChain_1")), []byte{1})
	db.Put(getChainArbitrationInKey(u("badChain_2")), []byte{1})

	dag, err := NewDag(db, nil, nil)
	require.NoError(t, err)

	require.True(t, dag.ChainIsInInArbitration(u("badChain_1")))
	require.True(t, dag.ChainIsInInArbitration(u("badChain_2")))
	require.False(t, dag.ChainIsInInArbitration(u("badChain_3")))

	//add
	dag.writeChainInInArbitration(u("badChain_3"), 0)
	require.True(t, dag.ChainIsInInArbitration(u("badChain_3")))

	//delete
	require.False(t, dag.ChainIsInInArbitration(u("badChain_4")))
	dag.chainArbitrationDone(u("badChain_4"), 1, common.Hash{})
	require.False(t, dag.ChainIsInInArbitration(u("badChain_4")))

	//add and remove
	require.False(t, dag.ChainIsInInArbitration(u("badChain_5")))
	dag.writeChainInInArbitration(u("badChain_5"), 0)
	require.True(t, dag.ChainIsInInArbitration(u("badChain_5")))
	dag.chainArbitrationDone(u("badChain_5"), 1, common.Hash{})
	require.False(t, dag.ChainIsInInArbitration(u("badChain_5")))
}
