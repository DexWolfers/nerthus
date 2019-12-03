package pman

import (
	"strconv"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/core/config"
)

func BenchmarkTxTrafficLimit(b *testing.B) {

	config.Set(cfgKeyTxTrafficOutLimit, strconv.Itoa(b.N*10)+"B")
	initRatio()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		var index uint32
		for pb.Next() {
			txTrafficLimit(index)
			index++
		}
	})
}

func TestTrafficLimit(t *testing.T) {
	config.Set(cfgKeyTxTrafficOutLimit, "100KB")
	initRatio()

	for i := 0; i < 1000; i++ {
		txTrafficLimit(2000)
		if i%20 == 0 {
			t.Log(txOutTrafficMeter.Rate1(), txOutTrafficMeter.Count())
			time.Sleep(time.Second)
		}

	}

}
