package witness

import (
	"errors"

	"gitee.com/nerthus/nerthus/params"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/core/types"
)

type WitnessPublicAPI struct {
	w *Witness
}

type RPCTxAction struct {
	Action types.TxAction
	Chain  common.Address
	Tx     common.Hash
}

func NewWitnessPublicAPI(w *Witness) *WitnessPublicAPI {
	return &WitnessPublicAPI{w: w}
}

//获取见证状态
func (a *WitnessPublicAPI) Status() map[string]interface{} {
	if !a.w.Mining() {
		return map[string]interface{}{
			"status": map[string]interface{}{
				"message": "stopped",
			},
		}
	}
	status := "running"
	if a.w.schedule.SysChainDataIsTooOld() {
		status = "running but system chain data is too old"
	} else if len(a.w.schedule.ChainWitness()) < int(params.ConfigParamsUCMinVotings) {
		status = "running but too few witness in group"
	} else if a.w.conns.ConnectedWitness().Len()+1 < int(params.ConfigParamsUCMinVotings) {
		status = "running but too few witness has connected"
	}
	witnessList := a.w.schedule.ChainWitness()
	if a.w.schedule.isSys { //系统链只取主系统见证人
		if len(witnessList) > int(params.ConfigParamsSCWitness) {
			witnessList = witnessList[:int(params.ConfigParamsSCWitness)]
		}
	}

	//a b c d e f g h i j k
	return map[string]interface{}{
		"status": map[string]interface{}{
			"message":           status,
			"joined":            a.w.conns.ConnectedWitness().Len() + 1, //+1 加自己
			"me":                a.w.schedule.curWitness,
			"groupIndex":        a.w.schedule.curWitnessGroup,
			"groupWitnessCount": witnessList.Len(),
			"groupWitness":      a.w.schedule.ChainWitness(),
			"pendingTxs":        a.w.schedule.jobCenter.Txs(),
		},
		"self":  a.w.conns.Status().MeInfo,
		"nodes": a.w.conns.Status().All(),
	}
}

// 获取当前内存中登记的指定链待处理的交易任务
func (a *WitnessPublicAPI) GetPendingTxs(chain common.Address) []RPCTxAction {
	if !a.w.Mining() {
		return nil
	}
	chainJobs := a.w.schedule.jobCenter.Load(chain)
	if chainJobs == nil {
		return nil
	}
	actions := make([]RPCTxAction, 0, chainJobs.Len())
	chainJobs.Range(func(msg *TxExecMsg) bool {
		actions = append(actions, RPCTxAction{
			Action: msg.Action,
			Chain:  chain,
			Tx:     msg.TXHash,
		})
		return true
	})
	return actions
}

type RPCChainStatus struct {
	Pendings []RPCTxAction
}

// 获取所有链的工作状态
func (a *WitnessPublicAPI) GetChainWorkStatus() map[common.Address]RPCChainStatus {
	if !a.w.Mining() {
		return nil
	}
	chains := a.w.schedule.jobCenter.Chains()
	status := make(map[common.Address]RPCChainStatus, len(chains))
	for _, c := range chains {
		status[c] = RPCChainStatus{
			Pendings: a.GetPendingTxs(c),
		}
	}
	return status
}

// 查询指定链的共识状态
func (a *WitnessPublicAPI) GetConsensusState(chain common.Address) (*bft.ConsensusInfo, error) {
	if !a.w.Mining() {
		return nil, errors.New("witness is not running")
	}
	info, err := a.w.GetChainConsensusState(chain)
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, errors.New("not active chain")
	}
	unit := a.w.nts.DagChain().GetChainTailUnit(chain)
	info.LastHeader = unit.Header()
	return info, nil
}

// 查询状态报告
func (a *WitnessPublicAPI) GetReplaceWitnessReport() interface{} {
	return a.w.schedule.replaceWitnessWorker.StatusReport()
}
func (a *WitnessPublicAPI) GetWorkStatus() interface{} {
	type ChainStatus struct {
		Chain     common.Address
		Txs       int64
		Consensus *bft.ConsensusInfo
	}
	var (
		infos       []ChainStatus
		inConsensus int
	)
	//获取所有链信息
	a.w.schedule.jobCenter.RangeChain(func(chain common.Address) bool {

		status := ChainStatus{
			Chain: chain,
		}
		//交易数
		if txQ := a.w.schedule.jobCenter.Load(chain); txQ != nil {
			status.Txs = txQ.Len()
		}
		//共识消息
		status.Consensus, _ = a.w.GetChainConsensusState(chain)
		if status.Consensus != nil {
			inConsensus++
		}
		infos = append(infos, status)
		return true
	})

	return map[string]interface{}{
		"count":       len(infos),
		"inConsensus": inConsensus,
		"chainStatus": infos,
	}
}
