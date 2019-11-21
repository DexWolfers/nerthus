package types

import (
	"testing"

	"github.com/stretchr/testify/require"

	"encoding/json"
	"math/big"

	"gitee.com/nerthus/nerthus/common"
)

func TestVoteMsgSign(t *testing.T) {

	header := Header{MC: common.StringToAddress("addr1"), Number: 1}
	msg := NewVoteMsg(header.Hash(), createVoteExtra())

	key, addr := defaultTestKey()
	signer := NewSigner(big.NewInt(18))
	err := SignBySignHelper(msg, signer, key)
	if err != nil {
		t.Fatal(err)
	}

	// 反向获取签名者成功
	got, err := Sender(signer, msg)
	if err != nil {
		t.Fatal(err)
	}
	if got != addr {
		t.Fatalf("expect %s,unexpected %s", addr, got)
	}

	// json 传递
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var newMsg VoteMsg
	err = json.Unmarshal(data, &newMsg)
	if err != nil {
		t.Fatal(err)
	}
	if newMsg.Hash() != msg.Hash() {
		t.Fatalf("expect %s,unexpected %s", msg.Hash(), newMsg.Hash())
	}
	sender, err := newMsg.Sender()
	require.NoError(t, err)
	require.Equal(t, addr, sender)
	require.Equal(t, msg, &newMsg)

	got, err = msg.Sender()
	if err != nil {
		t.Fatal(err)
	}
	if got != addr {
		t.Fatalf("expect %s,unexpected %s", addr, got)
	}

}

func TestVoteHasSigned(t *testing.T) {
	header := Header{MC: common.StringToAddress("addr1"), Number: 1}
	msg := *NewVoteMsg(header.Hash(), nil)

	key, _ := defaultTestKey()
	signer := NewSigner(big.NewInt(18))
	err := SignBySignHelper(&msg, signer, key)
	if err != nil {
		t.Fatal(err)
	}
	if got := msg.Sign.HasSigned(); got == false {
		t.Fatal("expect has signed,but not")
	}
}
