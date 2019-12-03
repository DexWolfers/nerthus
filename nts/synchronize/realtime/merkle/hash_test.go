package merkle

import (
	"hash/fnv"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common/math"

	"gitee.com/nerthus/nerthus/common"
)

func TestFnv32HashTime(t *testing.T) {

	oneHashTime := getOneHahsTime()
	for i := 1; i < 1000; i++ {
		chains := i * 1000000
		count := getHashTimes(uint64(chains))
		byteSize := count * 4

		t.Logf("chains:%d, nodes=%d  hashTimes=%s, memroySize=%.2fMb",
			chains, count, oneHashTime*time.Duration(count),
			float64(byteSize)/1024/1024,
		)
	}
}

func TestChainHashTimes(t *testing.T) {
	for i := 1; i < 100; i++ {
		chains := i * 1000
		count := getHashTimes(uint64(chains))
		t.Log("chains:", chains, "need hash times:", count)
	}
}

func getOneHahsTime() time.Duration {
	hasher := fnv.New32()

	now := time.Now()
	for i := 0; i < 3; i++ {
		hasher.Write(append(common.Uint64ToBytes(math.MaxUint64), common.Uint64ToBytes(math.MaxUint64)...))
		hasher.Sum(nil)
	}
	return time.Since(now) / 3
}
func getHashTimes(chains uint64) uint64 {
	if chains == 1 {
		return 1
	}
	var sum uint64
	var level uint64
	var preLevelNodes uint64

	preLevelNodes = 1
	for level = 1; ; level++ {
		var nodes uint64
		if level == 1 {
			nodes = 1
		} else if level == 2 {
			nodes = 2
		} else {
			nodes = preLevelNodes * 2
		}
		if nodes >= chains {
			sum += chains
		} else {
			sum += nodes
		}
		preLevelNodes = nodes
		if nodes >= chains {
			break
		}
	}
	return sum
}
