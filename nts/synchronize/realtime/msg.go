package realtime

import (
	"fmt"

	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/rlp"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/nts/synchronize/realtime/merkle"
)

type batchChainMsg struct {
	No     uint64 //批次号
	End    bool   //是否结束
	Chains []common.Address
}

//因为22727条链，22727 *22= 488.27 KB
const maxChainsCount = 22727

func pushChains(rangeChain func(f func(chain common.Address) bool), send func(event *RealtimeEvent)) {

	var chains []common.Address

	var batchIndex uint64
	sendChains := func(end bool) {
		batchIndex++ //从 1 开始

		msg := batchChainMsg{
			No:     batchIndex,
			End:    end,
			Chains: chains,
		}

		bytes, err := rlp.EncodeToBytes(msg)
		if err != nil {
			panic(err)
		}
		chains = chains[:0] //清理

		log.Trace("send missing chains",
			"batch", batchIndex, "chains", len(chains), "end", end)

		send(&RealtimeEvent{
			Code: RealtimePushChains,
			Data: bytes,
		})
	}

	rangeChain(func(chain common.Address) bool {
		chains = append(chains, chain)
		if len(chains) >= maxChainsCount {
			//开始发送
			sendChains(false)
		}
		return true
	})
	sendChains(true) //结束标记
}
func unpackChainsMsg(ev *RealtimeEvent, curNo uint64) (data batchChainMsg, err error) {
	if err = rlp.DecodeBytes(ev.Data, &data); err != nil {
		return data, err
	}
	if data.No != curNo+1 {
		return data, fmt.Errorf("info.No error,%d != %d+1", data.No, curNo)
	}
	return data, err
}
func (tree *treeCmptor) pushChains(rangeChain func(f func(chain common.Address) bool)) {
	pushChains(rangeChain, tree.sendMsg)
}

func (tree *treeCmptor) handleChainsMsg(ev *RealtimeEvent) error {
	info, err := unpackChainsMsg(ev, tree.curChainMsgBatchNo)
	if err != nil {
		return err
	}
	tree.logger.Trace("received missing chains", "len", len(info.Chains), "no", info.No, "end", info.End)

	tree.curChainMsgBatchNo = info.No

	if len(info.Chains) > 0 {
		cts := make([]merkle.Content, len(info.Chains))
		for i, c := range info.Chains {
			cts[i] = ChainStatus{Addr: c}
		}
		tree.tree.Append(cts...)
		tree.logger.Trace("update tree", "root", tree.tree.Root().Hash(), "leafs", tree.tree.LeafCount())
	}
	//批量发送结束时，更新
	if info.End {
		// 更新树并开始对比
		tree.sendFirstCmp(true)
	}
	return nil
}
