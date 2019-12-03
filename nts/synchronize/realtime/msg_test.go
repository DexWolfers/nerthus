package realtime

import (
	"fmt"
	"testing"

	"github.com/golang/snappy"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/rlp"
	"github.com/stretchr/testify/require"
)

func TestChainSize(t *testing.T) {

	var chains []common.Address

	for i := 0; i < 10000; i++ {
		k, err := crypto.GenerateKey()
		require.NoError(t, err)
		addr := crypto.ForceParsePubKeyToAddress(k.PublicKey)
		chains = append(chains, addr)

		if len(chains)%100 == 0 {
			b, err := rlp.EncodeToBytes(chains)
			require.NoError(t, err)

			compressed := snappy.Encode(nil, b)

			t.Logf("chains:%d, bytes size:%s, rlp Size:%s,compressed:%s",
				len(chains),
				common.StorageSize(common.AddressLength*len(chains)),
				common.StorageSize(len(b)),
				common.StorageSize(len(compressed)),
			)
		}
	}

}

func TestPushChains(t *testing.T) {

	//需要大量发送
	count := maxChainsCount*2 + 1
	var gots []*RealtimeEvent
	pushChains(func(f func(chain common.Address) bool) {
		for i := 0; i < count; i++ {
			chain := common.StringToAddress(fmt.Sprintf("chain_%d", i))
			if !f(chain) {
				return
			}
		}
	}, func(event *RealtimeEvent) {
		gots = append(gots, event)
	})

	require.Len(t, gots, 3, "should be 3 times") //应该组成三个包

	//能正常解包
	var batchNo uint64
	var sum int
	for i, v := range gots {
		info, err := unpackChainsMsg(v, batchNo)
		require.NoError(t, err)
		require.Equal(t, info.End, (i+1) == 3, "should be end on 3") //结束标记出现在最后一次
		require.Equal(t, info.No, uint64(i+1))                       //序号从 1 开始
		sum += len(info.Chains)
		batchNo++
	}
	require.Equal(t, count, sum, "should got all chains")
}
