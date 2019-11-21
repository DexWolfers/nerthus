// Copyright 2016 The go-ethereum Authors
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

package types

import (
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"io"
	"math/big"

	"github.com/pkg/errors"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/rlp"
)

var (
	ErrInvalidChainId     = errors.New("invalid chain id for signer")
	ErrInvalidAddressHash = errors.New("invalid address hash")
)

// sigCache is used to cache the derived sender and contains
// the signer used to derive it.
type sigCache struct {
	signer Signer
	from   common.Address
}

// MakeSigner 基于配置返回对于的签名方式
func MakeSigner(config params.ChainConfig) Signer {
	return NewSigner(config.ChainId)
}

// SignTx signs the transaction using the given signer and private key
// @Author: yin
// 更改为接口实现
// Author: @kulics
func SignBySignHelper(sh SignHelper, s Signer, prv *ecdsa.PrivateKey) error {
	h := s.Hash(sh)
	sig, err := crypto.Sign(h[:], prv)
	if err != nil {
		return err
	}
	return s.WithSignature(sh, sig)
}

// DecodeSigner 出解析内容签名者
func DecodeSigner(signer Signer, sign SignContent, content []byte) (common.Address, error) {
	d := SignerDecoder{content, sign}
	return Sender(signer, &d)
}

func sender(signer Signer, sh SignHelper) (common.Address, error) {
	pubkey, err := signer.PublicKey(sh)
	if err != nil {
		return common.Address{}, err
	}
	addr, err := crypto.Pubkey2Address(pubkey)
	if err != nil {
		return common.Address{}, err
	}
	//签名地址不允许和合约地址冲突
	if addr.IsContract() {
		return common.Address{}, ErrInvalidAddressHash
	}
	return addr, nil
}

func Sender(signer Signer, sh SignHelper) (common.Address, error) {
	var cache *common.Address
	switch v := sh.(type) {
	case *Unit:
		cache = &v.sender
	case *Transaction:
		cache = &v.from
	case *VoteMsg:
		cache = &v.sender
	default:
		//如果无法缓存，则直接返回结果
		return sender(signer, sh)
	}
	//优先从缓存中获取
	if cache.Empty() {
		addr, err := sender(signer, sh)
		if err != nil {
			return common.Address{}, err
		}
		switch v := sh.(type) {
		case *Transaction:
			//如果是交易，还需要检查签名与内部设置的发送方地址是否一致
			if v.SenderAddr() != addr {
				return common.Address{}, errors.WithMessage(ErrInvalidSig, "the tx sender is not equal signer")
			}
		}
		//缓存结果
		*cache = addr
	}
	return *cache, nil
}

type Signer interface {
	// Hash returns the rlp encoded hash for signatures
	Hash(SignHelper) common.Hash
	// PubilcKey returns the public key derived from the signature
	PublicKey(SignHelper) ([]byte, error)
	// WithSignature returns a copy of the transaction with the given signature.
	// The signature must be encoded in [R || S || V] format where V is 0 or 1.
	WithSignature(SignHelper, []byte) error
	// 验证签名数是否正确
	ValidateSignatureValues(SignHelper) error
	// SignContent 获取签名结果
	SignContent(SignHelper) ([]byte, error)
	// Checks for equality on the signers
	Equal(Signer) bool

	Sender(sig []byte, s SignHelper) (sender common.Address, err error)
	GetSig([]byte) ([]byte, error)
}

// SignHelper 签名帮助接口，只要实现了以下方法，就可以被签名
type SignHelper interface {
	GetSignStruct() []interface{}
	GetRSVBySignatureHash() []byte
	SetSign([]byte)
}

// Signer implements TransactionInterface using the
type DefSigner struct {
	chainId, chainIdMul *big.Int
}

func NewSigner(chainId *big.Int) DefSigner {
	if chainId == nil {
		chainId = new(big.Int)
	}
	return DefSigner{
		chainId:    chainId,
		chainIdMul: new(big.Int).Mul(chainId, big.NewInt(2)),
	}
}

// RSV2Sig 将RSV转换为sig
func (s DefSigner) GetSig(sign []byte) ([]byte, error) {
	if len(sign) < 65 {
		return nil, errors.Wrap(ErrInvalidSig, "the sign data is too short")
	}
	uv := new(big.Int).SetBytes(sign[64:])
	//检查 chainid
	if chainid := deriveChainId(uv); chainid.Cmp(s.chainId) != 0 {
		return nil, ErrInvalidChainId
	}
	ur := new(big.Int).SetBytes(sign[:32])
	us := new(big.Int).SetBytes(sign[32:64])
	v := byte(new(big.Int).Sub(uv, s.chainIdMul).Uint64() - 35)
	if !crypto.ValidateSignatureValues(v, ur, us, true) {
		return nil, errors.Wrap(ErrInvalidSig, "the sign values is invalid")
	}
	newSign := make([]byte, 65)
	copy(newSign[:32], sign[:32])
	copy(newSign[32:64], sign[32:64])
	newSign[64] = v
	return newSign, nil
}

func (s DefSigner) Sender(sig []byte, sh SignHelper) (sender common.Address, err error) {
	//先交易其合法性
	if sig, err = s.GetSig(sig); err != nil {
		return
	}
	hash := s.Hash(sh)
	pub, err := crypto.Ecrecover(hash[:], sig)
	if err != nil {
		return
	}
	return crypto.Pubkey2Address(pub)
}

func (s DefSigner) Equal(s2 Signer) bool {
	eip155, ok := s2.(DefSigner)
	return ok && eip155.chainId.Cmp(s.chainId) == 0
}

func (s DefSigner) ValidateSignatureValues(sh SignHelper) error {
	_, err := s.SignContent(sh)
	return err
}

// @Author: yin
func (s DefSigner) PublicKey(sh SignHelper) ([]byte, error) {
	sig, err := s.SignContent(sh)
	if err != nil {
		return nil, err
	}
	// recover the public key from the signature
	hash := s.Hash(sh)
	pub, err := crypto.Ecrecover(hash[:], sig)
	if err != nil {
		return nil, err
	}
	if len(pub) == 0 || pub[0] != 4 {
		return nil, errors.New("invalid public key")
	}
	return pub, nil
}

// SignContent 获取签名信息
// Author: @ysqi
func (s DefSigner) SignContent(sh SignHelper) ([]byte, error) {
	//R, S, V, err := s.getrsv(sh)
	sign := sh.GetRSVBySignatureHash()
	return s.GetSig(sign)
}

// WithSignature returns a new transaction with the given signature. This signature
// needs to be in the [R || S || V] format where V is 0 or 1.
// @Author: yin
func (s DefSigner) WithSignature(sh SignHelper, sig []byte) error {
	if len(sig) != 65 {
		panic(fmt.Sprintf("wrong size for signature: got %d, want 65", len(sig)))
	}
	if s.chainId.Sign() != 0 {
		v := big.NewInt(int64(sig[64] + 35))
		v.Add(v, s.chainIdMul)
		sig = append(sig[:64], v.Bytes()...)
	}
	sh.SetSign(sig)
	return nil
}

// Hash returns the hash to be signed by the sender.
// It does not uniquely identify the transaction.
// 注意：这里不允许修改HASH内容数据结构，必须是：[,,,,,chainID,0]。
// 因为第三方如JS均有内部签名算法，需要保持一致。
func (s DefSigner) Hash(sh SignHelper) common.Hash {
	structs := append(sh.GetSignStruct(), s.chainId, uint(0))
	return common.SafeHash256(structs...)
}

// deriveChainId derives the chain id from the given v parameter
func deriveChainId(v *big.Int) *big.Int {
	if v == nil {
		v = big.NewInt(0)
	}
	if v.BitLen() <= 64 {
		v := v.Uint64()
		if v == 27 || v == 28 {
			return new(big.Int)
		}
		return new(big.Int).SetUint64((v - 35) / 2)
	}
	v = new(big.Int).Sub(v, big.NewInt(35))
	return v.Div(v, big.NewInt(2))
}

// deriveSigner makes a *best* guess about which signer to use.
func deriveSigner(V *big.Int) Signer {
	return NewSigner(deriveChainId(V))
}

func DeriveSigner(v *big.Int) Signer {
	return NewSigner(deriveChainId(v))
}

func NewSignContent() *SignContent {
	return &SignContent{}
}

// SignContent 签名内容
type SignContent struct {
	sign []byte
}

func (s *SignContent) Hash() common.Hash {
	return common.SafeHash256(s.sign)
}

func (s *SignContent) Copy() []byte {
	b := make([]byte, len(s.sign))
	copy(b, s.sign)
	return b
}

// Set [R || S || V]
func (s *SignContent) Set(sign []byte) *SignContent {
	s.sign = sign
	return s
}

func (s *SignContent) GetSig() []byte {
	return s.sign
}

// Get [R || S || V]
func (s *SignContent) Get() []byte {
	return s.sign
}

func (s *SignContent) V() *big.Int {
	if s.HasSigned() {
		return new(big.Int).SetBytes(s.sign[64:])
	}
	return common.Big0
}
func (s *SignContent) GetSignStruct() []interface{} {
	panic("no yet implement")
}
func (s *SignContent) GetRSVBySignatureHash() []byte {
	return s.Get()
}
func (s *SignContent) SetSign(sig []byte) {
	s.Set(sig)
}
func (s *SignContent) MustSig() []byte {
	sig, err := deriveSigner(s.V()).SignContent(s)
	if err != nil {
		panic(fmt.Errorf("get sign bytes,err:%s", err))
	}
	return sig
}
func (s *SignContent) HasSigned() bool {
	return s != nil && len(s.sign) >= 65
}

func (s SignContent) MarshalJSON() ([]byte, error) {
	return json.Marshal(&s.sign)
}

func (s *SignContent) UnmarshalJSON(input []byte) error {
	return json.Unmarshal(input, &s.sign)
}

//DecodeRLP implements rlp.Encoder
func (s *SignContent) EncodeRLP(w io.Writer) error {
	if s == nil {
		return rlp.Encode(w, common.ZeroByte)
	}
	return rlp.Encode(w, s.sign)
}

// DecodeRLP implements rlp.Decoder
func (s *SignContent) DecodeRLP(sr *rlp.Stream) error {
	err := sr.Decode(&s.sign)
	return err
}

//func (s *SignContent) String() string {
//	return fmt.Sprintf("[R:%d,S:%d,V:%d]", s.r, s.s, s.v)
//}

type SignerDecoder struct {
	content []byte
	sign    SignContent
}

func (sd *SignerDecoder) GetSignStruct() []interface{} {
	return []interface{}{sd.content}
}
func (sd *SignerDecoder) GetRSVBySignatureHash() []byte {
	return sd.sign.Get()
}
func (sd *SignerDecoder) SetSign(sign []byte) {
	sd.sign.Set(sign)
}
