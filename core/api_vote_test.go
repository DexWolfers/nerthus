package core

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/ntsdb"
)

func TestDAG_SetUnitVoteMsg(t *testing.T) {
	db, err := ntsdb.NewMemDatabase()
	if err != nil {
		t.Fatalf("fail to new membase: %v", err)
	}

	dag, _ := NewDag(db, nil, nil)

	var votes []types.VoteMsg
	var addresses []common.Address
	header := types.Header{MC: common.StringToAddress("test"), Number: 100, ParentHash: common.StringToHash("parent")}
	signer := types.NewSigner(big.NewInt(1))

	batch := db.NewBatch()
	for i := 0; i < 5; i++ {
		key, _ := crypto.GenerateKey()
		v := types.VoteMsg{Extra: nil, UnitHash: header.Hash()}
		types.SignBySignHelper(&v, signer, key)
		votes = append(votes, v)
		addresses = append(addresses, crypto.ForceParsePubKeyToAddress(key.PublicKey))

		// 存储
		//err := rawdb.WriteVoteMsg(batch, header.Hash(), []types.WitenssVote{{Extra: v.Extra, Sign: v.Sign}})
		//if err != nil {
		//	t.Fatalf("set vote hash fail:%v", err)
		//}
	}
	require.NoError(t, batch.Write(), "failed to write")

	getVotes, err := dag.GetVoteMsg(header.Hash())
	if err != nil {
		t.Fatalf("fail to get vote msg:%v", err)
	}

	for i := 0; i < 5; i++ {
		voter, _ := getVotes[i].Sender()
		if addresses[i] != voter {
			t.Fatalf("not match:%v, %v", addresses[i].String(), voter.String())
		}
	}
}
