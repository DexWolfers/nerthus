package types

import (
	"crypto/ecdsa"

	"gitee.com/nerthus/nerthus/crypto"
)

// SignTx signs the transaction using the given signer and private key
func SignTx(tx *Transaction, s Signer, prv *ecdsa.PrivateKey) (*Transaction, error) {
	h := s.Hash(tx)
	sig, err := crypto.Sign(h[:], prv)
	if err != nil {
		return nil, err
	}
	err = s.WithSignature(tx, sig)
	if err != nil {
		return nil, err
	}
	return tx, nil
}
