package backend

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/objectpool"
	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/consensus/protocol"
)

// handle all message from outside
// 共识消息处理入口
func (sb *backend) OnMessage(ev protocol.MessageEvent) {
	//tr := tracetime.New()
	//defer tr.Stop()
	//tr.AddArgs("mid", ev.Message.ID())

	if !sb.coreStarted {
		return
	}
	sb.core.handleMsg(ev)
}

// 停止共识引擎
func (sb *backend) StopChainForce(chain common.Address) {
	if en := sb.core.getEngine(chain); en != nil {
		sb.core.clt.Unregister(chain)
		sb.core.freeChain(chain, true)
	}
}

// reset engine for next height work
// 清理共识状态，为下一个高度共识准备
func (sb *backend) SendNewUnit(chain common.Address, number uint64, uhash common.Hash) {
	sb.core.newHeadNumber(chain, number, uhash)
}

// 激活链的共识引擎
func (sb *backend) SendNewJob(chain common.Address) {
	sb.core.activate(chain, false)
}

// get current situation
// 获取共识引擎当前所处状态
func (sb *backend) MiningStatus(mc common.Address) *bft.ConsensusInfo {
	en := sb.core.getEngine(mc)
	if en == nil {
		return nil
	}
	_, stateInfo := en.CurrentState()
	return &stateInfo
}

func (sb *backend) NextChainSelector(selector func(func(nextChain common.Address) bool) common.Address) {
	sb.core.doing.SetSelect(func(f func(key objectpool.ObjectKey) bool) objectpool.ObjectKey {
		next := selector(func(nextChain common.Address) bool {
			return f(nextChain)
		})
		if next.Empty() {
			return nil
		}
		return next
	})
}

func (sb *backend) RollBackProposal(newUnit common.Hash, mc common.Address) {
	en := sb.core.getEngine(mc)
	if en != nil {
		en.RollbackPackedTx(newUnit)
	}
}
