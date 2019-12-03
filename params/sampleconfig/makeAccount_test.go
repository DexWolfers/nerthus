package sampleconfig

import (
	"crypto/ecdsa"
	"crypto/rand"
	"testing"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/crypto"
	"github.com/stretchr/testify/require"
)

type account struct {
	privateKey *ecdsa.PrivateKey
	address    common.Address
}

type accountList []account

func (a *accountList) have(address common.Address) bool {
	for _, v := range *a {
		if v.address == address {
			return true
		}
	}
	return false
}

func (a *accountList) add(privateKey *ecdsa.PrivateKey) {
	if !crypto.ForceParsePubKeyToAddress(privateKey.PublicKey).IsContract() {
		address := crypto.ForceParsePubKeyToAddress(privateKey.PublicKey)
		if !a.have(address) {
			*a = append(*a, account{privateKey, address})
		}
	}
}

func (a *accountList) len() int {
	return len(*a)
}

func TestMakeAccount(t *testing.T) {
	var accounts accountList
	for i := 0; i < 10; i++ {
		privateKey, err := ecdsa.GenerateKey(crypto.S256(), rand.Reader)
		require.Nil(t, err)
		accounts.add(privateKey)
	}
	var b string
	t.Log("len", accounts.len())
	for i, v := range accounts {
		t.Log(i, v.address.Hex())
		b = common.Bytes2Hex(crypto.FromECDSA(v.privateKey))

	}
	t.Log(b)
	p, err := crypto.HexToECDSA(b)
	require.Nil(t, err)
	t.Log(crypto.ForceParsePubKeyToAddress(p.PublicKey).Hex())
}
