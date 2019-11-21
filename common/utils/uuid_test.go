package utils

import (
	"math/rand"
	"strconv"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
	"github.com/stretchr/testify/require"
)

/**
goos: darwin
goarch: amd64
pkg: gitee.com/nerthus/nerthus/common/utils
BenchmarkRandomHash-4          	 2000000	       928 ns/op
BenchmarkRandomUint64-4        	 1000000	      1124 ns/op
BenchmarkRandomFnv64a-4        	 5000000	       367 ns/op
BenchmarkRandomFnv64aBytes-4   	10000000	       132 ns/op
BenchmarkRandomUUIDHash-4      	 1000000	      1306 ns/op
BenchmarkRandomMathInt63-4     	50000000	        33.3 ns/op
BenchmarkRandomInt63n-4        	50000000	        31.5 ns/op
BenchmarkRandomN-4             	50000000	        27.4 ns/op
PASS
**/
func BenchmarkRandomHash(b *testing.B) {
	for i := 0; i < b.N; i++ {
		RandomHash()
	}
}

func BenchmarkRandomUint64(b *testing.B) {
	for i := 0; i < b.N; i++ {
		RandomUint64()
	}
}

func BenchmarkRandomFnv64a(b *testing.B) {
	var result = make(map[uint64]struct{})
	for i := 0; i < b.N; i++ {
		result[RandomFnv64a([]byte(strconv.FormatInt(int64(i), 10)))] = struct{}{}
	}
	if len(result) != b.N {
		panic(len(result))
	}
}

func BenchmarkRandomFnv64aBytes(b *testing.B) {
	for i := 0; i < b.N; i++ {
		RandomFnv64aBytes()
	}
}

func BenchmarkRandomUUIDHash(b *testing.B) {
	for i := 0; i < b.N; i++ {
		RandomUUIDHash()
	}
}

func BenchmarkRandomMathInt63(b *testing.B) {
	rand.Seed(time.Now().UnixNano())
	for i := 0; i < b.N; i++ {
		RandomMathInt63(math.MaxInt64)
	}
}

func BenchmarkRandomInt63n(b *testing.B) {
	for i := 0; i < b.N; i++ {
		RandomInt63n(math.MaxInt64)
	}
}

func BenchmarkRandomN(b *testing.B) {
	for i := 0; i < b.N; i++ {
		RandomN(math.MaxInt32)
	}
}

func TestRandomHash(t *testing.T) {
	a := RandomHash()
	bigB := a.Big()
	newA := common.BytesToHash(bigB.Bytes())
	t.Log(a.Bytes())
	t.Log(a.Big().Bytes())
	require.Equal(t, a, newA)
}

func TestNewRand(t *testing.T) {
	actul := make(map[int64]struct{})
	for i := 0; i < 1000000; i++ {
		actulVal := RandomInt63n(math.MaxInt64)
		_, ok := actul[actulVal]
		require.False(t, ok)
		actul[actulVal] = struct{}{}
	}
}

func TestRandomFnv64aBytes(t *testing.T) {
	t.Logf("size:%v, %0x\n", len(RandomFnv64aBytes()), RandomFnv64a())
}
