package types

import (
	"math/big"
	"testing"

	"gitee.com/nerthus/nerthus/common"
	"github.com/stretchr/testify/assert"
)

func TestDefSigner_DecodeSender(t *testing.T) {
	key, addr := defaultTestKey()

	tx := NewTransaction(addr, common.Address{}, new(big.Int), 0, 0, 0, nil)

	signer := NewSigner(big.NewInt(454))
	var err error
	tx, err = SignTx(tx, signer, key)
	if err != nil {
		t.Fatal(err)
	}

	// 通过正常方式获取签名者
	got, err := Sender(signer, tx)
	if assert.NoError(t, err) {
		assert.Equal(t, addr, got, "should be equal private key address")
	}

	t.Run("sig1", func(t *testing.T) {
		sign := tx.GetRSVBySignatureHash()
		_, err := signer.GetSig(sign)
		if assert.NoError(t, err) {
			assert.Equal(t, addr, got, "should be equal private key address")
		}
		got, err = signer.Sender(sign, tx)
		if assert.NoError(t, err) {
			assert.Equal(t, addr, got, "should be equal private key address")
		}
	})

	t.Run("sig2", func(t *testing.T) {
		sig := tx.data.Sign.GetSig()
		got, err = signer.Sender(sig, tx)
		if assert.NoError(t, err) {
			assert.Equal(t, addr, got, "should be equal private key address")
		}
	})
}
