package core

import (
	"crypto/ecdsa"
	"math/big"

	"gitee.com/nerthus/nerthus/consensus/bft"

	"reflect"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
)

func makeUnit(mc common.Address, parentHash, sc common.Hash, number uint64) *types.Unit {
	header := &types.Header{
		MC:         mc,
		Number:     number,
		ParentHash: parentHash,
		SCHash:     sc,
	}
	unit := types.NewUnit(header, nil)
	return unit
}

func newTestZeroProposal() bft.Proposal {
	return newTestProposal(common.EmptyAddress, common.Hash{}, common.Hash{})
}

func newTestProposal(mc common.Address, parentHash, sc common.Hash) bft.Proposal {
	return makeUnit(mc, parentHash, sc, 1)
}

func signHelper(chainID *big.Int, sh types.SignHelper, key *ecdsa.PrivateKey) {
	if chainID == nil {
		panic("chain id is nil")
	}
	if sh == nil {
		panic("signHelper is nil")
	}
	if key == nil {
		panic("privatekey is nil")
	}
	singer := types.NewSigner(chainID)
	err := types.SignBySignHelper(sh, singer, key)
	if err != nil {
		panic(err)
	}
}

func TestNewRequest(t *testing.T) {
	//testLogger.SetHandler(elog.StdoutHandler)

	N := uint64(4)
	F := uint64(1)

	sys := NewTestSystemWithBackend(N, F)

	close := sys.Run(true)
	defer close()

	request1 := makeUnit(common.EmptyAddress, common.Hash{}, common.Hash{}, 1)
	sys.backends[0].NewRequest(request1)

	<-time.After(1 * time.Second)

	request2 := makeUnit(common.EmptyAddress, common.Hash{}, common.Hash{}, 1)
	sys.backends[0].NewRequest(request2)

	<-time.After(1 * time.Second)

	for _, backend := range sys.backends {
		if len(backend.committedMsgs) != 2 {
			t.Errorf("the number of executed requests mismatch: have %v, want 2", len(backend.committedMsgs))
		}
		if !reflect.DeepEqual(request1.Number(), backend.committedMsgs[0].commitProposal.Number()) {
			t.Errorf("the number of requests mismatch: have %v, want %v", request1.Number(), backend.committedMsgs[0].commitProposal.Number())
		}
		if !reflect.DeepEqual(request2.Number(), backend.committedMsgs[1].commitProposal.Number()) {
			t.Errorf("the number of requests mismatch: have %v, want %v", request2.Number(), backend.committedMsgs[1].commitProposal.Number())
		}
	}
}
