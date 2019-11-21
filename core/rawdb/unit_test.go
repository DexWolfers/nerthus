package rawdb

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/crypto"

	"gitee.com/nerthus/nerthus/rlp"
	"github.com/stretchr/testify/require"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
)

func rlpEncode(t *testing.T, data interface{}) []byte {
	b, err := rlp.EncodeToBytes(&data)
	require.NoError(t, err)
	return b
}

func TestHeader(t *testing.T) {
	db, close := newDB(t)
	defer close()

	header := types.Header{
		MC:          common.ForceDecodeAddress("nts1x67wl68e9rhyxfkr4nu6jqmpvd2fufju5lk76c"),
		Number:      math.MaxUint64,
		ParentHash:  common.StringToHash("test"),
		SCHash:      common.StringToHash("test1"),
		Timestamp:   uint64(time.Now().UTC().UnixNano()),
		Bloom:       types.BytesToBloom([]byte("bloom...")),
		GasLimit:    999,
		GasUsed:     888,
		StateRoot:   common.StringToHash("StateRoot"),
		ReceiptRoot: common.StringToHash("ReceiptRoot"),
	}

	WriteHeader(db, &header)

	t.Run("empty", func(t *testing.T) {
		_, err := GetHeader(db, common.StringToHash("missing"))
		require.EqualError(t, err, ErrNotFound.Error())
	})

	t.Run("getAndDecode", func(t *testing.T) {
		got, err := GetHeader(db, header.Hash())
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, header.Hash(), got.Hash())
	})
	t.Run("empty", func(t *testing.T) {
		h := GetHeaderRLP(db, common.StringToHash("uint1"))
		require.Len(t, h, 0)
	})
	t.Run("loc", func(t *testing.T) {
		list := UnitsByTimeLine(db, header.Timestamp, 1)
		require.Len(t, list, 1)
		require.Equal(t, list[0], header.Hash())
	})
}

func newAccount(t *testing.T) (common.Address, *ecdsa.PrivateKey) {
	priKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	address, err := crypto.ParsePubKeyToAddress(priKey.PublicKey)
	require.NoError(t, err)

	return address, priKey
}

func createUnit(t *testing.T) *types.Unit {
	proposer, proposerKey := newAccount(t)

	header := types.Header{
		Proposer:    proposer,
		MC:          common.ForceDecodeAddress("nts1x67wl68e9rhyxfkr4nu6jqmpvd2fufju5lk76c"),
		Number:      math.MaxUint64,
		ParentHash:  common.StringToHash("test"),
		SCHash:      common.StringToHash("test1"),
		Timestamp:   uint64(time.Now().UTC().UnixNano()),
		Bloom:       types.BytesToBloom([]byte("bloom...")),
		GasLimit:    rand.Uint64(),
		GasUsed:     rand.Uint64(),
		StateRoot:   common.StringToHash("StateRoot"),
		ReceiptRoot: common.StringToHash("ReceiptRoot"),
	}
	signer := types.NewSigner(big.NewInt(1))

	txs := make(types.TxExecs, 10)
	for i := 0; i < len(txs); i++ {
		txs[i] = &types.TransactionExec{}

		if i%3 == 0 {
			acc, k := newAccount(t)
			tx := types.NewTransaction(acc, acc, big.NewInt(int64(i)), rand.Uint64(), rand.Uint64(), 10, nil)
			err := types.SignBySignHelper(tx, signer, k)
			require.NoError(t, err)

			txs[i].Tx = tx
			txs[i].TxHash = tx.Hash()
			txs[i].Action = types.ActionTransferPayment
		} else {
			txs[i].TxHash = common.StringToHash(fmt.Sprintf("txs_%d", i))
			txs[i].Action = types.ActionContractReceipt
			txs[i].PreStep = types.UnitID{
				ChainID: common.StringToAddress(fmt.Sprintf("from_%d", i)),
				Height:  uint64(i * 5),
				Hash:    common.StringToHash(fmt.Sprintf("unit_%d", i*5)),
			}
		}
	}

	unit := types.NewUnit(&header, txs)
	err := types.SignBySignHelper(unit, signer, proposerKey)
	require.NoError(t, err)

	//见证人投票
	var votes []types.WitenssVote
	for i := 0; i < 11; i++ {
		_, k := newAccount(t)
		msg := types.NewVoteMsg(header.Hash(), createVoteExtra())
		err := types.SignBySignHelper(msg, signer, k)
		if err != nil {
			t.Fatal(err)
		}
		votes = append(votes, msg.ToWitnessVote())
	}
	unit = unit.WithBody(txs, votes, unit.Sign)

	return unit

}

func createVoteExtra() []byte {
	var buffer = make([]byte, types.ExtraSealSize)
	buffer[0] = 3
	binary.LittleEndian.PutUint64(buffer[1:types.ExtraSealSize], 1)
	return buffer
}

func TestUnit(t *testing.T) {
	db, close := newDB(t)
	defer close()

	unit := createUnit(t)

	t.Log("unitHash:", unit.Hash().Hex())
	WriteHeader(db, unit.Header())
	WriteUnitBody(db, unit.MC(), unit.Hash(), unit.Number(), unit.Body())

	t.Run("body", func(t *testing.T) {

		check := func(body *types.Body, t *testing.T) {
			require.NotNil(t, body)

			//检查每一笔交易
			require.Equal(t, len(unit.Transactions()), len(body.Txs))
			for i, tx := range unit.Transactions() {
				gotTx := body.Txs[i]

				//require.Equal(t, tx.PreStep, gotTx.PreStep)
				require.True(t, tx.PreStep.Equal(gotTx.PreStep))
				require.Equal(t, tx.Action, gotTx.Action)
				require.Equal(t, tx.TxHash, gotTx.TxHash)
				if tx.Tx != nil {
					require.NotNil(t, gotTx.Tx)
					require.Equal(t, tx.TxHash, gotTx.Tx.Hash())
				}
			}

			//检查投票
			require.Equal(t, len(unit.WitnessVotes()), len(body.Votes))
			require.Equal(t, unit.Sign.Get(), body.Sign.Get())
		}
		t.Run("get", func(t *testing.T) {
			body := GetUnitBody(db, unit.MC(), unit.Hash(), unit.Number())

			check(body, t)
		})

		t.Run("bodyRLP", func(t *testing.T) {
			b := GetUnitBodyRLP(db, unit.MC(), unit.Hash(), unit.Number())
			body, err := DecodeUnitBody(rlp.NewStream(bytes.NewReader(b), 0))
			require.NoError(t, err)
			check(body, t)
		})

	})

	t.Run("rawrlp", func(t *testing.T) {
		b := GetRawUnit(db, unit.MC(), unit.Hash(), unit.Number())

		got, err := DecodeRawUnit(b)
		require.NoError(t, err)
		require.Equal(t, unit.Hash(), got.Hash())

		//检查每一笔交易
		require.Equal(t, len(unit.Transactions()), len(got.Transactions()))
		for i, tx := range unit.Transactions() {
			gotTx := got.Transactions()[i]

			//require.Equal(t, tx.PreStep, gotTx.PreStep)
			require.True(t, tx.PreStep.Equal(gotTx.PreStep))
			require.Equal(t, tx.Action, gotTx.Action)
			require.Equal(t, tx.TxHash, gotTx.TxHash)
			if tx.Tx != nil {
				require.NotNil(t, gotTx.Tx)
				require.Equal(t, tx.TxHash, gotTx.Tx.Hash())
			}
		}

		//检查投票
		require.Equal(t, len(unit.WitnessVotes()), len(got.WitnessVotes()))
	})

}
