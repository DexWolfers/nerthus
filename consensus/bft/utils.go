package bft

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/crypto"
)

func GetSignatureAddress(data []byte, sig []byte) (common.Address, error) {
	// 1. Keccak data
	hashData := crypto.Keccak256(data)
	// 2. Recover public key
	pubkey, err := crypto.SigToPub(hashData, sig)
	if err != nil {
		return common.Address{}, err
	}
	return crypto.ParsePubKeyToAddress(*pubkey)
}
