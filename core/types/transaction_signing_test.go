package types

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/crypto"
)

func TestSignCache(t *testing.T) {
	key, _ := crypto.GenerateKey()
	addr := crypto.ForceParsePubKeyToAddress(key.PublicKey)
	fmt.Println("--pubkey--1-->>>", key.PublicKey)

	signer := NewSigner(big.NewInt(128))
	tx, err := SignTx(NewTransaction(addr, addr, new(big.Int), 0, 0, 0, nil), signer, key)
	if err != nil {
		t.Fatal(err)
	}

	from, err := Sender(signer, tx)
	require.Nil(t, err)
	require.Equal(t, addr, from, "exected from and address to be equal")

	load := tx.from
	require.NotNil(t, load, "exected cache sender after call Sender")
	require.IsType(t, common.Address{}, load, "should be type sigCache")
	require.Equal(t, addr, load, "exected from and address to be equal")
}

func TestSignTx(t *testing.T) {
	key, _ := crypto.GenerateKey()
	addr := crypto.ForceParsePubKeyToAddress(key.PublicKey)

	t.Run("default", func(t *testing.T) {

		signer := NewSigner(big.NewInt(128))
		tx, err := SignTx(NewTransaction(addr, addr, new(big.Int), 0, 0, 0, nil), signer, key)
		if err != nil {
			t.Fatal(err)
		}

		from, err := Sender(signer, tx)
		require.NoError(t, err)
		require.Equal(t, addr, from, "exected from and address to be equal")

		//取值相同
		require.Equal(t, addr, tx.SenderAddr())

	})
	t.Run("bad", func(t *testing.T) {
		bad := common.StringToAddress("badAddress")

		signer := NewSigner(big.NewInt(128))
		tx, err := SignTx(NewTransaction(bad, addr, new(big.Int), 0, 0, 0, nil), signer, key)
		if err != nil {
			t.Fatal(err)
		}

		_, err = Sender(signer, tx)
		require.Error(t, err)
	})
}

func TestChainId(t *testing.T) {
	key, addr := defaultTestKey()

	tx := NewTransaction(addr, common.Address{}, new(big.Int), 0, 0, 0, nil)

	var err error
	tx, err = SignTx(tx, NewSigner(big.NewInt(1)), key)
	if err != nil {
		t.Fatal(err)
	}

	_, err = Sender(NewSigner(big.NewInt(2)), tx)
	if err != ErrInvalidChainId {
		t.Error("expected error:", ErrInvalidChainId)
	}

	_, err = Sender(NewSigner(big.NewInt(1)), tx)
	if err != nil {
		t.Error("expected no error")
	}
}
