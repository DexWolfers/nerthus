package utils

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"math"
	"math/big"
	mrand "math/rand"
	"sync"
	"sync/atomic"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/crypto"
	"github.com/pborman/uuid"
)

var subInput = uint64(0)

var grand *Rand

type Rand struct {
	sync.Mutex
	rand *mrand.Rand
}

func init() {
	grand = NewRand()
	grand.init()
}

func NewRand() *Rand {
	rand := &Rand{}
	rand.init()
	return rand
}

func (r *Rand) init() {
	r.reset(time.Now().UnixNano())
}

func (r *Rand) reset(seed int64) {
	r.rand = mrand.New(mrand.NewSource(seed))
}

func (r *Rand) RandomInt63n(n int64) int64 {
	r.Lock()
	i64 := r.rand.Int63n(n)
	r.Unlock()
	return i64
}

func (r *Rand) RandomN(n int) int {
	r.Lock()
	i32 := r.rand.Intn(n)
	r.Unlock()
	return i32
}

func RandomUint64(data ...[]byte) uint64 {
	var b *big.Int
	if len(data) == 0 {
		b = RandomHash().Big()
	} else {
		b = RandomHash(data...).Big()
	}
	hashser := fnv.New64a()
	hashser.Write(b.Bytes())
	return hashser.Sum64()
}

func RandomHash(data ...[]byte) common.Hash {
	newValue := atomic.AddUint64(&subInput, 1)
	var header = make([]byte, binary.MaxVarintLen64*2)
	binary.PutUvarint(header[:binary.MaxVarintLen64], newValue)
	binary.PutVarint(header[binary.MaxVarintLen64:], RandomInt63n(math.MaxInt64))
	var buf bytes.Buffer
	buf.Write(header[:])
	for _, input := range data {
		buf.Write(input)
	}
	return crypto.Keccak256Hash(buf.Bytes())
}

// |atomic(8byte)| random(8byte) | data|
func RandomFnv64a(data ...[]byte) uint64 {
	newValue := atomic.AddUint64(&subInput, 1)
	var header = make([]byte, binary.MaxVarintLen64*2)
	binary.PutUvarint(header[:binary.MaxVarintLen64], newValue)
	binary.PutVarint(header[binary.MaxVarintLen64:], RandomInt63n(math.MaxInt64))
	hasher := fnv.New64a()
	hasher.Write(header[:])
	for _, input := range data {
		hasher.Write(input)
	}
	return hasher.Sum64()
}

func New64a(data []byte) string {
	haser := fnv.New64a()
	haser.Write(data)
	return fmt.Sprintf("0x%x", haser.Sum(nil))
}
func RandomFnv64aBytes(data ...[]byte) (b []byte) {
	newValue := atomic.AddUint64(&subInput, 1)
	var header = make([]byte, binary.MaxVarintLen64*2)
	binary.PutUvarint(header[:binary.MaxVarintLen64], newValue)
	binary.PutVarint(header[binary.MaxVarintLen64:], RandomInt63n(math.MaxInt64))
	hasher := fnv.New64a()
	hasher.Write(header[:])
	for _, input := range data {
		hasher.Write(input)
	}
	return hasher.Sum(nil)
}

func RandomUUIDHash() common.Hash {
	return crypto.Keccak256Hash([]byte(uuid.NewRandom().String()))
}

func RandomInt63n(n int64) int64 {
	return grand.RandomInt63n(n)
}

func RandomN(n int) int {
	return grand.RandomN(n)
}

func RandomMathInt63(n int64) int64 {
	return mrand.Int63n(n)
}

func RandomMathN(n int) int {
	return mrand.Intn(n)
}

func RandomTimeSecond(base time.Duration, n int64) time.Duration {
	return base + time.Second*time.Duration(RandomInt63n(n))
}
