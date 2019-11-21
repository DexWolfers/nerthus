package core

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"encoding/binary"
	"io"
	"math/big"
	"os"
	"reflect"
	"strings"
	"testing"

	"gitee.com/nerthus/nerthus/rlp"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/utils"
	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/consensus/protocol"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testPreprepare(t *testing.T) {
	pp := &bft.Preprepare{
		View: &bft.View{
			Round:    big.NewInt(1),
			Sequence: big.NewInt(2),
		},
		Proposal: makeUnit(params.SCAccount, common.Hash{}, common.Hash{}, 1),
	}
	prepreparePayload, err := Encode(pp)
	require.Nil(t, err)

	m := &protocol.Message{
		Code:    MsgPreprepare,
		Msg:     prepreparePayload,
		Address: common.StringToAddress("0x1234567890"),
	}

	msgPayload, err := m.Payload()
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	decodedMsg := new(protocol.Message)
	err = decodedMsg.FromPayload(msgPayload, nil)
	require.Nil(t, err)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	var decodedPP *bft.Preprepare
	err = decodedMsg.Decode(&decodedPP)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	require.NotNil(t, decodedPP.Proposal)
	// if block is encoded/decoded by rlp, we cannot to compare interface data type using reflect.DeepEqual. (like istanbul.Proposal)
	// so individual comparison here.
	if !reflect.DeepEqual(pp.Proposal.Hash(), decodedPP.Proposal.Hash()) {
		t.Errorf("proposal hash mismatch: have %v, want %v", decodedPP.Proposal.Hash(), pp.Proposal.Hash())
	}

	if !reflect.DeepEqual(pp.View, decodedPP.View) {
		t.Errorf("view mismatch: have %v, want %v", decodedPP.View, pp.View)
	}

	if !reflect.DeepEqual(pp.Proposal.Number(), decodedPP.Proposal.Number()) {
		t.Errorf("proposal number mismatch: have %v, want %v", decodedPP.Proposal.Number(), pp.Proposal.Number())
	}

	assert.Equal(t, common.Hash{}, common.Hash{})
	assert.NotEqual(t, common.Hash{}, common.StringToHash("10"))
}

func testSubject(t *testing.T) {
	s := &bft.Subject{
		View: &bft.View{
			Round:    big.NewInt(1),
			Sequence: big.NewInt(2),
		},
		Digest: common.StringToHash("1234567890"),
	}

	subjectPayload, _ := Encode(s)

	m := &protocol.Message{
		Code:    protocol.MsgPreprepare,
		Msg:     subjectPayload,
		Address: common.StringToAddress("0x1234567890"),
	}

	msgPayload, err := m.Payload()
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	decodedMsg := new(protocol.Message)
	err = decodedMsg.FromPayload(msgPayload, nil)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	var decodedSub *bft.Subject
	err = decodedMsg.Decode(&decodedSub)
	if err != nil {
		t.Errorf("error mismatch: have %v, want nil", err)
	}

	if !s.Cmp(decodedSub) {
		t.Errorf("subject mismatch: have %v, want %v", decodedSub, s)
	}
}

func TestMessageEncodeDecode(t *testing.T) {
	t.Run("preprepare", func(t *testing.T) {
		testPreprepare(t)
	})

	t.Run("subject", func(t *testing.T) {
		testSubject(t)
	})
}

func TestMessage_ProveSign(t *testing.T) {
	var (
		chainId     = big.NewInt(100)
		defSign     = types.NewSigner(chainId)
		accountKeys = generateAccount(11)
	)

	s := &bft.Subject{
		View: &bft.View{
			Round:    big.NewInt(10),
			Sequence: big.NewInt(2),
		},
		Extra: utils.RandomFnv64aBytes(),
	}
	subjectPayload, err := Encode(s)
	require.Nil(t, err)

	m := &protocol.Message{
		Code:    protocol.MsgRoundCh,
		Msg:     subjectPayload,
		Address: getPublicKeyAddress(accountKeys[0]),
		Extra:   subjectPayload,
	}

	m.Sign = &types.SignContent{}
	require.Nil(t, types.SignBySignHelper(m, defSign, accountKeys[0]))
	proveTree := bft.NewProofRoundChange([][]byte{m.PacketProveTreeBytes()})
	var proveTreeMsg bft.ProofTreeMessage
	err = rlp.DecodeBytes(proveTree.Payload[0], &proveTreeMsg)
	require.Nil(t, err)
	require.Nil(t, proveTreeMsg.FromPayload(proveTree.Payload[0]))

	{
		buf, err := rlp.EncodeToBytes([][]byte{[]byte{1}, []byte{2}})
		require.Nil(t, err)
		var doubleBuffer [][]byte
		require.Nil(t, rlp.DecodeBytes(buf, &doubleBuffer))
		t.Log(doubleBuffer)
	}
}

func TestRoundChangeProveSign(t *testing.T) {
	var (
		accountKeys  = generateAccount(11)
		chainAddress = common.StringToAddress("hello word")
		chainId      = big.NewInt(100)
		defSign      = types.NewSigner(chainId)
		msgs         []*protocol.Message
		payload      [][]byte
		rd           = bufio.NewReader(bytes.NewReader(nil))
		rdWr         = bufio.NewReadWriter(rd, bufio.NewWriter(bytes.NewBuffer(nil)))
	)

	log.Root().SetHandler(bufferLogHandler(rdWr, log.LogfmtFormat()))

	log.Debug("Hello Word")

	s := &bft.Subject{
		View: &bft.View{
			Round:    big.NewInt(10),
			Sequence: big.NewInt(2),
		},
		Extra: utils.RandomFnv64aBytes(),
	}
	subjectPayload, err := Encode(s)
	require.Nil(t, err)

	// 构建消息以及proof tree
	for i := 0; i < len(accountKeys); i++ {
		m := &protocol.Message{
			Code:    protocol.MsgRoundCh,
			Msg:     subjectPayload,
			Address: getPublicKeyAddress(accountKeys[i]),
			Extra:   subjectPayload,
		}
		m.Sign = &types.SignContent{}
		require.Nil(t, types.SignBySignHelper(m, defSign, accountKeys[i]))
		msgs = append(msgs, m)
		payload = append(payload, m.PacketProveTreeBytes())
		t.Log(m.Address.Hex())
	}

	var validators common.AddressList
	var validatorsSet bft.ValidatorSet
	for _, key := range accountKeys {
		validators = append(validators, getPublicKeyAddress(key))
	}
	validatorsSet = bft.NewSet(0, common.Hash{}, validators)

	t.Run("ok", func(t *testing.T) {
		var proofTree bft.ProofTree = bft.NewProofRoundChange(payload)
		_, ok := proofTree.Prove(chainAddress, &bft.View{Sequence: big.NewInt(2), Round: big.NewInt(6)}, validatorsSet, defSign)
		require.True(t, ok)
	})

	t.Run("Signature total less than min vote count", func(t *testing.T) {
		var proofTree bft.ProofTree = bft.NewProofRoundChange(payload[len(payload)-2:])
		_, ok := proofTree.Prove(chainAddress, &bft.View{Sequence: big.NewInt(2), Round: big.NewInt(6)}, validatorsSet, defSign)
		require.False(t, ok)
	})

	t.Run("Data format is invalid", func(t *testing.T) {
		payload := append(payload, []byte{1})
		var proofTree bft.ProofTree = bft.NewProofRoundChange(payload)
		_, ok := proofTree.Prove(chainAddress, &bft.View{Sequence: big.NewInt(2), Round: big.NewInt(6)}, validatorsSet, defSign)
		require.False(t, ok)
	})

	t.Run("Subject's Op is invalid", func(t *testing.T) {
		m := &protocol.Message{
			Code:    protocol.MsgCommit,
			Msg:     subjectPayload,
			Address: getPublicKeyAddress(accountKeys[0]),
			Extra:   subjectPayload,
		}
		m.Sign = &types.SignContent{}
		require.Nil(t, types.SignBySignHelper(m, defSign, accountKeys[0]))
		payload = append(payload, m.GetProveTreeBytes())
		var proofTree bft.ProofTree = bft.NewProofRoundChange(payload)
		_, ok := proofTree.Prove(chainAddress, &bft.View{Sequence: big.NewInt(2), Round: big.NewInt(6)}, validatorsSet, defSign)
		require.False(t, ok)
	})

	t.Run("Signer not included validators", func(t *testing.T) {
		key := generateAccount(1)[0]
		m := &protocol.Message{
			Code:    protocol.MsgRoundCh,
			Msg:     subjectPayload,
			Address: getPublicKeyAddress(key),
			Extra:   subjectPayload,
		}
		m.Sign = &types.SignContent{}
		require.Nil(t, types.SignBySignHelper(m, defSign, key))
		payload = append(payload, m.GetProveTreeBytes())
		var proofTree bft.ProofTree = bft.NewProofRoundChange(payload)
		_, ok := proofTree.Prove(chainAddress, &bft.View{Sequence: big.NewInt(2), Round: big.NewInt(6)}, validatorsSet, defSign)
		require.False(t, ok)
	})

	t.Run("Subject decode fail", func(t *testing.T) {

	})

	t.Run("Subject's round less than current round", func(t *testing.T) {
		var proofTree bft.ProofTree = bft.NewProofRoundChange(payload)
		_, ok := proofTree.Prove(chainAddress, &bft.View{Sequence: big.NewInt(2), Round: big.NewInt(10)}, validatorsSet, defSign)
		require.False(t, ok)
	})

	t.Run("Signers are different", func(t *testing.T) {
		barKey := generateAccount(1)[0]
		m := &protocol.Message{
			Code:    protocol.MsgRoundCh,
			Msg:     subjectPayload,
			Address: getPublicKeyAddress(barKey),
			Extra:   subjectPayload,
		}
		m.Sign = &types.SignContent{}
		require.Nil(t, types.SignBySignHelper(m, defSign, barKey))
		msgs[0] = m
		payload = append(payload, m.GetProveTreeBytes())
		var proofTree bft.ProofTree = bft.NewProofRoundChange(payload)

		_, ok := proofTree.Prove(chainAddress, &bft.View{Sequence: big.NewInt(2), Round: big.NewInt(9)}, validatorsSet, defSign)
		require.False(t, ok)
	})
}

func TestLockHashProveSign(t *testing.T) {
	var (
		accountKeys  = generateAccount(11)
		chainAddress = common.StringToAddress("hello word")
		chainId      = big.NewInt(100)
		defSign      = types.NewSigner(chainId)
		msgs         []*message
		payload      [][]byte
		rd           = bufio.NewReader(bytes.NewReader(nil))
		rdWr         = bufio.NewReadWriter(rd, bufio.NewWriter(bytes.NewBuffer(nil)))
		lockHash     = common.StringToHash("Hello Word")
	)

	log.Root().SetHandler(bufferLogHandler(rdWr, log.LogfmtFormat()))

	log.Debug("Hello Word")

	s := &bft.Subject{
		View: &bft.View{
			Round:    big.NewInt(10),
			Sequence: big.NewInt(2),
		},
		Extra:  utils.RandomFnv64aBytes(),
		Digest: common.StringToHash("foo"),
	}
	subjectPayload, err := Encode(s)
	require.Nil(t, err)

	// 构建消息以及proof tree
	for i := 0; i < len(accountKeys); i++ {
		m := &protocol.Message{
			Code:    protocol.MsgPrepare,
			Msg:     subjectPayload,
			Address: getPublicKeyAddress(accountKeys[i]),
			Extra:   subjectPayload,
		}
		m.Sign = &types.SignContent{}
		require.Nil(t, types.SignBySignHelper(m, defSign, accountKeys[i]))
		msgs = append(msgs, m)
		payload = append(payload, m.PacketProveTreeBytes())
		t.Log(m.Address.Hex())
	}

	var validatorsSet bft.ValidatorSet
	var validators []common.Address
	for _, key := range accountKeys {
		validators = append(validators, getPublicKeyAddress(key))

	}
	validatorsSet = bft.NewSet(0, common.Hash{}, validators)

	t.Run("ok", func(t *testing.T) {
		var proofTree bft.ProofTree = bft.NewProofLockHash(&bft.View{Sequence: big.NewInt(2), Round: big.NewInt(10)}, common.StringToHash("foo"), payload)
		_, ok := proofTree.Prove(chainAddress, lockHash, &bft.View{Sequence: big.NewInt(2), Round: big.NewInt(9)}, validatorsSet, defSign)
		require.True(t, ok)
	})

	t.Run("ok with a commit message", func(t *testing.T) {
		m := &protocol.Message{
			Code:    protocol.MsgCommit,
			Msg:     subjectPayload,
			Address: getPublicKeyAddress(accountKeys[0]),
			Extra:   subjectPayload,
		}
		// 设置commit 签名
		{
			var buffer = make([]byte, ExtraSealSize)
			buffer[0] = byte(protocol.MsgCommit)
			binary.LittleEndian.PutUint64(buffer[1:ExtraSealSize], s.View.Round.Uint64())
			vote := types.VoteMsg{Extra: buffer, UnitHash: s.Digest}
			t.Log(len(buffer))
			m.CommittedSeal = &types.WitenssVote{Extra: vote.Extra, Sign: vote.Sign}
		}

		m.Sign = &types.SignContent{}
		require.Nil(t, types.SignBySignHelper(m, defSign, accountKeys[0]))
		msgs[0] = m
		payload = append(payload, m.PacketProveTreeBytes())

		var proofTree bft.ProofTree = bft.NewProofLockHash(&bft.View{Sequence: big.NewInt(2), Round: big.NewInt(10)}, common.StringToHash("foo"), payload)
		_, ok := proofTree.Prove(chainAddress, lockHash, &bft.View{Sequence: big.NewInt(2), Round: big.NewInt(9)}, validatorsSet, defSign)
		require.True(t, ok)
	})

	t.Run("Lock same hash", func(t *testing.T) {
		var proofTree bft.ProofTree = bft.NewProofLockHash(&bft.View{Sequence: big.NewInt(2), Round: big.NewInt(10)}, common.StringToHash("foo"), payload)
		_, ok := proofTree.Prove(chainAddress, common.StringToHash("foo"), &bft.View{Sequence: big.NewInt(2), Round: big.NewInt(9)}, validatorsSet, defSign)
		require.False(t, ok)
	})

	t.Run("Try to lock little view or same view", func(t *testing.T) {
		var proofTree bft.ProofTree = bft.NewProofLockHash(&bft.View{Sequence: big.NewInt(2), Round: big.NewInt(10)}, common.StringToHash("foo"), payload)
		_, ok := proofTree.Prove(chainAddress, lockHash, &bft.View{Sequence: big.NewInt(2), Round: big.NewInt(10)}, validatorsSet, defSign)
		require.False(t, ok)
	})

	t.Run("Signature total less than min vote count", func(t *testing.T) {
		var proofTree bft.ProofTree = bft.NewProofLockHash(&bft.View{Sequence: big.NewInt(2), Round: big.NewInt(10)}, common.StringToHash("foo"), payload[len(payload)-2:])
		_, ok := proofTree.Prove(chainAddress, lockHash, &bft.View{Sequence: big.NewInt(2), Round: big.NewInt(9)}, validatorsSet, defSign)
		require.False(t, ok)
	})

	t.Run("Data format is invalid", func(t *testing.T) {
		var proofTree bft.ProofTree = bft.NewProofLockHash(&bft.View{Sequence: big.NewInt(2), Round: big.NewInt(10)}, common.StringToHash("foo"), append(payload, []byte{1}))
		_, ok := proofTree.Prove(chainAddress, lockHash, &bft.View{Sequence: big.NewInt(2), Round: big.NewInt(9)}, validatorsSet, defSign)
		require.False(t, ok)
	})

	t.Run("Signer not included validatorsSet", func(t *testing.T) {
		var proofTree bft.ProofTree = bft.NewProofLockHash(&bft.View{Sequence: big.NewInt(2), Round: big.NewInt(10)}, common.StringToHash("foo"), append(payload, []byte{1}))
		barKey := generateAccount(1)[0]
		m := &protocol.Message{
			Code:    MsgPrepare,
			Msg:     subjectPayload,
			Address: getPublicKeyAddress(barKey),
			Extra:   subjectPayload,
		}
		m.Sign = &types.SignContent{}
		require.Nil(t, types.SignBySignHelper(m, defSign, barKey))
		msgs[0] = m
		payload = append(payload, m.PacketProveTreeBytes())
		_, ok := proofTree.Prove(chainAddress, lockHash, &bft.View{Sequence: big.NewInt(2), Round: big.NewInt(9)}, validatorsSet, defSign)
		require.False(t, ok)
	})

	t.Run("Subject decode fail", func(t *testing.T) {

	})

	t.Run("Subject view not equal lock view", func(t *testing.T) {
		s := &bft.Subject{
			View: &bft.View{
				Round:    big.NewInt(11),
				Sequence: big.NewInt(2),
			},
			Extra:  utils.RandomFnv64aBytes(),
			Digest: common.StringToHash("foo"),
		}
		subjectPayload, err := Encode(s)
		require.Nil(t, err)
		m := &protocol.Message{
			Code:    protocol.MsgPrepare,
			Msg:     subjectPayload,
			Address: getPublicKeyAddress(accountKeys[0]),
			Extra:   subjectPayload,
		}
		m.Sign = &types.SignContent{}
		require.Nil(t, types.SignBySignHelper(m, defSign, accountKeys[0]))
		msgs = append(msgs, m)
		payload = append(payload, m.PacketProveTreeBytes())

		var proofTree bft.ProofTree = bft.NewProofLockHash(&bft.View{Sequence: big.NewInt(2), Round: big.NewInt(10)}, common.StringToHash("foo"), payload)
		_, ok := proofTree.Prove(chainAddress, lockHash, &bft.View{Sequence: big.NewInt(2), Round: big.NewInt(9)}, validatorsSet, defSign)
		require.False(t, ok)
	})

	t.Run("Subject digest not equal lock-hash", func(t *testing.T) {
		s := &bft.Subject{
			View: &bft.View{
				Round:    big.NewInt(10),
				Sequence: big.NewInt(2),
			},
			Extra:  utils.RandomFnv64aBytes(),
			Digest: common.StringToHash("bar"),
		}
		subjectPayload, err := Encode(s)
		require.Nil(t, err)
		m := &protocol.Message{
			Code:    protocol.MsgPrepare,
			Msg:     subjectPayload,
			Address: getPublicKeyAddress(accountKeys[0]),
			Extra:   subjectPayload,
		}
		m.Sign = &types.SignContent{}
		require.Nil(t, types.SignBySignHelper(m, defSign, accountKeys[0]))
		msgs = append(msgs, m)
		payload = append(payload, m.PacketProveTreeBytes())

		var proofTree bft.ProofTree = bft.NewProofLockHash(&bft.View{Sequence: big.NewInt(2), Round: big.NewInt(10)}, common.StringToHash("foo"), payload)
		_, ok := proofTree.Prove(chainAddress, lockHash, &bft.View{Sequence: big.NewInt(2), Round: big.NewInt(9)}, validatorsSet, defSign)
		require.False(t, ok)
	})

	t.Run("Subject's Op size less `ExtraSealSize`", func(t *testing.T) {
		m := &protocol.Message{
			Code:    protocol.MsgCommit,
			Msg:     subjectPayload,
			Address: getPublicKeyAddress(accountKeys[0]),
			Extra:   subjectPayload,
		}
		// 设置commit 签名
		{
			var buffer = make([]byte, 1)
			buffer[0] = byte(protocol.MsgCommit) // 丢掉round
			vote := types.VoteMsg{Extra: buffer, UnitHash: s.Digest}
			t.Log(len(buffer))
			m.CommittedSeal = &types.WitenssVote{Extra: vote.Extra, Sign: vote.Sign}
		}

		m.Sign = &types.SignContent{}
		require.Nil(t, types.SignBySignHelper(m, defSign, accountKeys[0]))
		msgs = append(msgs, m)
		payload = append(payload, m.PacketProveTreeBytes())

		var proofTree bft.ProofTree = bft.NewProofLockHash(&bft.View{Sequence: big.NewInt(2), Round: big.NewInt(10)}, common.StringToHash("foo"), payload)
		_, ok := proofTree.Prove(chainAddress, lockHash, &bft.View{Sequence: big.NewInt(2), Round: big.NewInt(9)}, validatorsSet, defSign)
		require.False(t, ok)
	})

	t.Run("Subject's Op should be commit operation", func(t *testing.T) {
		m := &protocol.Message{
			Code:    protocol.MsgCommit,
			Msg:     subjectPayload,
			Address: getPublicKeyAddress(accountKeys[0]),
			Extra:   subjectPayload,
		}
		// 设置commit 签名
		{
			var buffer = make([]byte, ExtraSealSize)
			buffer[0] = byte(protocol.MsgPrepare) // 设置错误的操作码
			binary.LittleEndian.PutUint64(buffer[1:], s.View.Round.Uint64())
			vote := types.VoteMsg{Extra: buffer, UnitHash: s.Digest}
			m.CommittedSeal = &types.WitenssVote{Extra: vote.Extra, Sign: vote.Sign}
		}

		m.Sign = &types.SignContent{}
		require.Nil(t, types.SignBySignHelper(m, defSign, accountKeys[0]))
		msgs = append(msgs, m)
		payload = append(payload, m.PacketProveTreeBytes())

		var proofTree bft.ProofTree = bft.NewProofLockHash(&bft.View{Sequence: big.NewInt(2), Round: big.NewInt(10)}, common.StringToHash("foo"), payload)
		_, ok := proofTree.Prove(chainAddress, lockHash, &bft.View{Sequence: big.NewInt(2), Round: big.NewInt(9)}, validatorsSet, defSign)
		require.False(t, ok)
	})

	t.Run("Signers are different", func(t *testing.T) {
		var proofTree bft.ProofTree = bft.NewProofLockHash(&bft.View{Sequence: big.NewInt(2), Round: big.NewInt(10)}, common.StringToHash("foo"), append(payload, []byte{1}))
		barKey := generateAccount(1)[0]
		m := &protocol.Message{
			Code:    protocol.MsgPrepare,
			Msg:     subjectPayload,
			Address: getPublicKeyAddress(barKey),
			Extra:   subjectPayload,
		}
		m.Sign = &types.SignContent{}
		require.Nil(t, types.SignBySignHelper(m, defSign, barKey))
		msgs[0] = m
		payload = append(payload, m.PacketProveTreeBytes())
		_, ok := proofTree.Prove(chainAddress, lockHash, &bft.View{Sequence: big.NewInt(2), Round: big.NewInt(9)}, validatorsSet, defSign)
		require.False(t, ok)
	})

}

func generateAccount(n int) []*ecdsa.PrivateKey {
	var keys []*ecdsa.PrivateKey
	for i := 0; i < n; i++ {
		key, _ := crypto.GenerateKey()
		keys = append(keys, key)
	}
	return keys
}

func bufferLogHandler(wr io.Writer, format log.Format) log.Handler {
	h := log.FuncHandler(func(r *log.Record) error {
		_, err := wr.Write(format.Format(r))
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(format.Format(r))
		return err
	})
	return log.LazyHandler(log.SyncHandler(h))
}

func matchWriterSubstr(reader io.Reader, sub string) bool {
	rd := bufio.NewReader(reader)
	for {
		line, err := rd.ReadString('\n')
		if err == io.EOF {
			return false
		}
		if strings.IndexAny(line, sub) >= 0 {
			return true
		}
	}
}
