package common

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"strings"

	"github.com/pkg/errors"

	"gitee.com/nerthus/nerthus/common/bech32"
	"gitee.com/nerthus/nerthus/rlp"
)

const (
	AccountPrefix  = "nts" //外部账户前缀
	ContractPrefix = "nsc" //合约地址前缀
)

var (
	contractHashPrefix = []byte{1, 1, 1, 1} //合约地址哈希前缀，以区分合约地址和外部账户地址
)

// 地址哈希
// 地址哈希根据公钥推导，可转换为可读字符
type AddressHash [AddressLength]byte

// Address is an Address for a pay-to-witness-pubkey-hash
// (P2WPKH) output. See BIP 173 for further details regarding native segregated
// witness address encoding:
// https://github.com/bitcoin/bips/blob/master/bip-0173.mediawiki
type Address struct {
	hash AddressHash
	hrp  string
	str  string
}

// NewAddress returns a new Address.
func NewAddress(pubkey []byte) (Address, error) {
	return newAddress(AccountPrefix, pubkey)
}

// newAddress is an internal helper function to create an
// Address with a known human-readable part, rather than
// looking it up through its parameters.
func newAddress(hrp string, pubkey []byte) (Address, error) {

	hash, err := getPublicKeyHash(hrp, pubkey)
	if err != nil {
		return Address{}, err
	}
	addr := Address{
		hrp:  strings.ToLower(hrp),
		hash: hash,
	}
	addr.str = addr.String()
	return addr, nil
}

// String returns a human-readable string for the Address.
// This is equivalent to calling EncodeAddress, but is provided so the type
// can be used as a fmt.Stringer.
// Part of the Address interface.
func (a Address) String() string {
	if a.Empty() {
		return emptyAddrJsonDecode
	}
	if a.str != "" {
		return a.str
	}
	str, err := encodeSegWitAddress(a.hrp, a.hash[:])
	if err != nil {
		return "<invalid address>"
	}
	a.str = str
	return str
}

// WitnessProgram returns the public key hash of the Address.
func (a Address) PubKeyHash() []byte {
	return a.hash[:]
}

func (a Address) IsContract() bool {
	return a.hrp == ContractPrefix
}

func (a Address) Bytes() []byte {
	return a.PubKeyHash()
}

func BytesToAddress(b []byte) Address {

	var a Address
	if len(b) > AddressLength {
		b = b[len(b)-AddressLength:]
	}
	copy(a.hash[AddressLength-len(b):], b)
	if isContractAddressHash(a.hash[:]) {
		a.hrp = ContractPrefix
	} else {
		a.hrp = AccountPrefix
	}
	a.str = a.String()
	return a
}

func StringToAddress(s string) Address { return BytesToAddress([]byte(s)) }
func BigToAddress(b *big.Int) Address  { return BytesToAddress(b.Bytes()) }

// Get the string representation of the underlying address
func (a Address) Str() string   { return a.String() }
func (a Address) Big() *big.Int { return new(big.Int).SetBytes(a.hash[:]) }
func (a Address) Hash() Hash    { return BytesToHash(a.hash[:]) }

func (a Address) ShortString() string {
	txt := a.String()
	return fmt.Sprintf("%s...%s", txt[:8], txt[len(txt)-4:])
}

func (a Address) Hex() string {
	return a.String()
}

func (a Address) Text() string {
	return a.String()
}

// Sets a to other
func (a *Address) Set(other Address) {
	a.hrp = other.hrp
	a.str = other.str
	for i, v := range other.hash {
		a.hash[i] = v
	}
}

// MarshalText returns the hex representation of a.
func (a Address) MarshalText() ([]byte, error) {
	return []byte(a.String()), nil
}

func (a Address) Empty() bool {
	em := AddressHash{}
	return bytes.Equal(a.hash[:], em[:])
}
func (a Address) Equal(b Address) bool {
	return bytes.Equal(a.hash[:], b.hash[:])
}

func (a Address) MarshalJSON() ([]byte, error) {
	if a.Empty() {
		return json.Marshal(emptyAddrJsonDecode)
	}
	return json.Marshal(a.String())
}

// UnmarshalText parses a hash in hex syntax.
func (a *Address) UnmarshalText(input []byte) error {
	addr, err := DecodeAddress(string(input))
	if err != nil {
		return err
	}
	a.Set(addr)
	return nil
}

// UnmarshalJSON parses
func (a *Address) UnmarshalJSON(input []byte) error {

	emptyAddrBuffer := fmt.Sprintf("\"%s\"", emptyAddrJsonDecode)
	if len(input) == len(emptyAddrBuffer) &&
		strings.ToLower(string(input)) == emptyAddrBuffer {
		a.Set(Address{})
		return nil
	}
	if !(len(input) >= 2 && input[0] == '"' && input[len(input)-1] == '"') {
		return errors.New("missing double quotes")
	}
	return a.UnmarshalText(input[1 : len(input)-1])
}

// Format implements fmt.Formatter, forcing the byte slice to be formatted as is,
// without going through the stringer interface used for logging.
// 强制使用 String 格式展示数据
func (a Address) Format(s fmt.State, c rune) {
	fmt.Fprintf(s, "%s", a.String())
}

func (a Address) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, a.Bytes())
}
func (a *Address) DecodeRLP(s *rlp.Stream) error {
	var b []byte
	err := s.Decode(&b)
	if err != nil {
		return err
	}

	//长度检查
	if len(b) != AddressLength {
		return errors.Errorf("invalid address hash length,want %d,got %d", AddressLength, len(b))
	}

	a.Set(BytesToAddress(b))
	return nil
}

// 获取账户公钥哈希值
func GetPublicKeyHash(pubkey []byte) (AddressHash, error) {
	return getPublicKeyHash(AccountPrefix, pubkey)
}

func isContractAddressHash(hash []byte) bool {
	return bytes.HasPrefix(hash[:], contractHashPrefix)
}

func getPublicKeyHash(hrp string, pubkey []byte) (AddressHash, error) {
	var hash AddressHash
	switch hrp {
	case ContractPrefix:
		hash16 := calcHash(calcHash(pubkey, sha256.New()), md5.New())
		if len(hash16) != 16 {
			return hash, errors.New("need get 128 bits hash")
		}
		copy(hash[:len(contractHashPrefix)], contractHashPrefix)
		copy(hash[len(contractHashPrefix):], hash16)

	case AccountPrefix:
		hash160 := Hash160(pubkey)
		if isContractAddressHash(hash160) {
			return hash, errors.New("invalid address public key hash")
		}
		copy(hash[:], hash160)

	default:
		return hash, errors.New("invalid hrp")
	}
	return hash, nil
}

// encodeSegWitAddress creates a bech32 encoded address string representation
// from witness version and witness program.
func encodeSegWitAddress(hrp string, pubkeyHash []byte) (string, error) {
	// Group the address bytes into 5 bit groups, as this is what is used to
	// encode each character in the address string.
	converted, err := bech32.ConvertBits(pubkeyHash, 8, 5, true)
	if err != nil {
		return "", err
	}

	bech, err := bech32.Encode(hrp, converted)
	if err != nil {
		return "", err
	}

	// Check validity by decoding the created address.
	hrp, program, err := decodeSegWitAddress(bech)
	if err != nil {
		return "", fmt.Errorf("invalid address: %v", err)
	}
	if !bytes.Equal(program, pubkeyHash) {
		return "", fmt.Errorf("invalid address")
	}
	return bech, nil
}

func ForceDecodeAddress(address string) Address {
	addr, err := DecodeAddress(address)
	if err != nil {
		//过渡期间，为了防止错误发生。强制报错
		panic(fmt.Errorf("decode string address(%s)failed,%v", address, err))
	}
	return addr
}

// 编码地址
func DecodeAddress(address string) (Address, error) {
	if address == "" || address == emptyAddrJsonDecode {
		return Address{}, nil
	}
	hrp, pubkeyHash, err := decodeSegWitAddress(address)
	if err != nil {
		return Address{}, errors.Wrapf(err, "invalid address(%s)", address)
	}
	switch hrp {
	case ContractPrefix:
		if !isContractAddressHash(pubkeyHash) {
			return Address{}, errors.Errorf("invalid address(%s): invalid contract hash", address)
		}
	case AccountPrefix:
		if isContractAddressHash(pubkeyHash) {
			return Address{}, errors.Errorf("invalid address(%s): invalid contract hash", address)
		}
	default:
		return Address{}, errors.Errorf("invalid address(%s): invalid address prefix", address)
	}
	addr := Address{hrp: hrp, str: address}
	copy(addr.hash[:], pubkeyHash)
	return addr, nil
}

// decodeSegWitAddress parses a bech32 encoded segwit address string and
// returns the witness version and witness program byte representation.
func decodeSegWitAddress(address string) (string, []byte, error) {
	// Decode the bech32 encoded address.
	hrp, data, err := bech32.Decode(address)
	if err != nil {
		return "", nil, err
	}
	// The remaining characters of the address returned are grouped into
	// words of 5 bits. In order to restore the original witness program
	// bytes, we'll need to regroup into 8 bit words.
	regrouped, err := bech32.ConvertBits(data[:], 5, 8, false)
	if err != nil {
		return "", nil, err
	}
	// For witness version 0, address MUST be exactly 20  bytes.
	if len(regrouped) != 20 {
		return "", nil, fmt.Errorf("invalid data length for witness "+
			"version 0: %v", len(regrouped))
	}
	return hrp, regrouped, nil
}

// CreateContractAddress 创建合约地址
// 合约地址哈希值：ripemd160(sha256(b,vmmagic,Bech32ContractHRP,nonce))
func GenerateContractAddress(b Address, vmmagic uint16, nonce uint64) Address {
	data, err := rlp.EncodeToBytes([]interface{}{b.PubKeyHash(), vmmagic, ContractPrefix, nonce})
	if err != nil {
		panic(err)
	}
	addr, err := newAddress(ContractPrefix, data)
	if err != nil {
		panic(err)
	}
	return addr
}
