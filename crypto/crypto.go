// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package crypto

import (
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/big"
	"os"

	"github.com/btcsuite/btcd/btcec"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/crypto/sha3"
)

var (
	secp256k1_N, _  = new(big.Int).SetString("fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141", 16)
	secp256k1_halfN = new(big.Int).Div(secp256k1_N, big.NewInt(2))
)

//var S256 = btcec.S256()

type PrivateKey btcec.PrivateKey
type PublicKey btcec.PublicKey

// Keccak256 calculates and returns the Keccak256 hash of the input data.
func Keccak256(data ...[]byte) []byte {
	d := sha3.NewKeccak256()
	for _, b := range data {
		d.Write(b)
	}
	h := d.Sum(nil)
	sha3.PutKeccak256(d)
	return h
}

// Keccak256Hash calculates and returns the Keccak256 hash of the input data,
// converting it to an internal Hash data structure.
func Keccak256Hash(data ...[]byte) (h common.Hash) {
	d := sha3.NewKeccak256()
	for _, b := range data {
		d.Write(b)
	}
	d.Sum(h[:0])
	sha3.PutKeccak256(d)
	return h
}

// Keccak512 calculates and returns the Keccak512 hash of the input data.
func Keccak512(data ...[]byte) []byte {
	d := sha3.NewKeccak512()
	for _, b := range data {
		d.Write(b)
	}
	return d.Sum(nil)
}

// ToECDSA creates a private key with the given D value.
func ToECDSA(d []byte) (*ecdsa.PrivateKey, error) {

	return toECDSA(d, true)
}

// ToECDSAUnsafe blidly converts a binary blob to a private key. It should almost
// never be used unless you are sure the input is valid and want to avoid hitting
// errors due to bad origin encoding (0 prefixes cut off).
func ToECDSAUnsafe(d []byte) *ecdsa.PrivateKey {
	priv, _ := toECDSA(d, false)
	return priv
}

// toECDSA creates a private key with the given D value. The strict parameter
// controls whether the key's length should be enforced at the curve size or
// it can also accept legacy encodings (0 prefixes).
func toECDSA(d []byte, strict bool) (*ecdsa.PrivateKey, error) {
	curve := S256()
	if strict && 8*len(d) != curve.Params().BitSize {
		return nil, fmt.Errorf("invalid length, need %d bits", curve.Params().BitSize)
	}
	priv, _ := btcec.PrivKeyFromBytes(S256(), d)
	return priv.ToECDSA(), nil
}

// FromECDSA exports a private key into a binary dump.
func FromECDSA(priv *ecdsa.PrivateKey) []byte {
	if priv == nil {
		return nil
	}
	return math.PaddedBigBytes(priv.D, priv.Params().BitSize/8)
}

// 将公钥转换为地址
func Pubkey2Address(pubKey []byte) (common.Address, error) {
	if len(pubKey) == 0 || pubKey[0] != 4 {
		return common.Address{}, errors.New("invalid public key")
	}
	key, err := ParsePubKey(pubKey)
	if err != nil {
		return common.Address{}, err
	}
	return ParsePubKeyToAddress(*key)
}

func ParsePubKey(pk []byte) (key *ecdsa.PublicKey, err error) {
	k, err := btcec.ParsePubKey(pk, btcec.S256())
	if err != nil {
		return nil, err
	}
	return k.ToECDSA(), nil
}
func ToECDSAPub(pub []byte) *ecdsa.PublicKey {
	key, err := ParsePubKey(pub)
	if err != nil {
		panic(err)
	}
	return key
}

func FromECDSAPub(pub *ecdsa.PublicKey) []byte {
	if pub == nil || pub.X == nil || pub.Y == nil {
		return nil
	}
	k := btcec.PublicKey(*pub)
	return k.SerializeUncompressed()
}

// HexToECDSA parses a secp256k1 private key.
func HexToECDSA(hexkey string) (*ecdsa.PrivateKey, error) {

	b, err := hex.DecodeString(hexkey)
	if err != nil {
		return nil, errors.New("invalid hex string")
	}
	return ToECDSA(b)
}

// LoadECDSA loads a secp256k1 private key from the given file.
func LoadECDSA(file string) (*ecdsa.PrivateKey, error) {
	buf := make([]byte, 64)
	fd, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	if _, err := io.ReadFull(fd, buf); err != nil {
		return nil, err
	}

	key, err := hex.DecodeString(string(buf))
	if err != nil {
		return nil, err
	}
	return ToECDSA(key)
}

// SaveECDSA saves a secp256k1 private key to the given file with
// restrictive permissions. The key data is saved hex-encoded.
func SaveECDSA(file string, key *ecdsa.PrivateKey) error {
	k := hex.EncodeToString(FromECDSA(key))
	return ioutil.WriteFile(file, []byte(k), 0600)
}

func GenerateKey() (*ecdsa.PrivateKey, error) {
	//产生的 Key 需要排除一些哈希值和合约哈希值冲突部分
	for {
		k, err := btcec.NewPrivateKey(S256())
		if err != nil {
			return nil, err
		}
		if CheckPubKeyHash(k.PublicKey) {
			return k.ToECDSA(), nil
		}
	}
}

// ValidateSignatureValues verifies whether the signature values are valid with
// the given chain rules. The v value is assumed to be either 0 or 1.
func ValidateSignatureValues(v byte, r, s *big.Int, homestead bool) bool {

	if r.Cmp(common.Big1) < 0 || s.Cmp(common.Big1) < 0 {
		return false
	}
	// reject upper range of s values (ECDSA malleability)
	// see discussion in secp256k1/libsecp256k1/include/secp256k1.h
	if homestead && s.Cmp(secp256k1_halfN) > 0 {
		return false
	}
	// Frontier: allow s to be in full N range
	return r.Cmp(secp256k1_N) < 0 && s.Cmp(secp256k1_N) < 0 && (v == 0 || v == 1)
}

func CheckPubKeyHash(p ecdsa.PublicKey) bool {
	_, err := GetPublicKeyHash(p)
	return err == nil
}

// 获取公钥对应的地址哈希
func GetPublicKeyHash(p ecdsa.PublicKey) ([]byte, error) {
	pubk := btcec.PublicKey(p)
	h, err := common.GetPublicKeyHash((&pubk).SerializeCompressed())
	if err != nil {
		return nil, err
	}
	return h[:], nil
}

// 将公钥转换为地址
func ParsePubKeyToAddress(p ecdsa.PublicKey) (common.Address, error) {
	pk := btcec.PublicKey(p)
	return common.NewAddress(pk.SerializeCompressed())
}

// 强制将公钥转换为地址，如果出错则 panic
func ForceParsePubKeyToAddress(p ecdsa.PublicKey) common.Address {
	addr, err := ParsePubKeyToAddress(p)
	if err != nil {
		panic(err)
	}
	return addr
}
