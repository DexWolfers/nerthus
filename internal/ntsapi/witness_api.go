// Author: @zwj
// 见证人操作相关API

package ntsapi

import (
	"math/big"

	"gitee.com/nerthus/nerthus/common/hexutil"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/params"
)

type PrivateWitnessAPI struct {
	b Backend
}

func NewPrivateWitnessAPI(b Backend) *PrivateWitnessAPI {
	return &PrivateWitnessAPI{
		b: b,
	}
}

// StartWitness 开启见证人
func (w *PrivateWitnessAPI) StartWitness(mainAccount common.Address, password string) error {
	return w.b.StartWitness(mainAccount, password)
}

// StopWitness 关闭见证人
func (w *PrivateWitnessAPI) StopWitness() error {
	return w.b.StopWitness()
}

// StartMember 开启理事
func (w *PrivateWitnessAPI) StartMember(ac common.Address, password string) (bool, error) {
	if err := w.b.StartMember(ac, password); err != nil {
		return false, err
	} else {
		return true, nil
	}
}

// StopMember 关闭理事
func (w *PrivateWitnessAPI) StopMember() error {
	return w.b.StopMember()
}

// GetWitnessFee 查询见证人/理事 可提现余额
func (api *PrivateWitnessAPI) GetWitnessFee(address common.Address) (interface{}, error) {
	dagChain := api.b.DagChain()
	state, err := dagChain.GetChainTailState(params.SCAccount)
	last := dagChain.GetChainTailUnit(params.SCAccount)
	// 增发
	addi, freeze, err := sc.GetAdditionalBalance(address, last.Number(), state, dagChain)
	if err != nil {
		return nil, err
	}

	var (
		fee        = new(big.Int)
		feeFreezed = new(big.Int)
	)
	//理事无此部分费用
	if sc.ExistWitness(state, address) {
		// 见证手续费
		fee, _, err = sc.GetWithdrawableWitnessFee(state, api.b.ChainDb(), address, last.Number())
		if err != nil {
			return nil, err
		}
		feeFreezed, err = sc.GetFreezingWitnessFee(state, api.b.ChainDb(), address, last.Number())
		if err != nil {
			return nil, err
		}
	}
	// 值不能为空
	result := struct {
		Additional        hexutil.Big
		AdditionalFreezed hexutil.Big
		WitnessFee        hexutil.Big
		WitnessFeeFreezed hexutil.Big
	}{
		hexutil.Big(*addi),
		hexutil.Big(*freeze),
		hexutil.Big(*fee),
		hexutil.Big(*feeFreezed),
	}

	return result, nil
}

// GetSystemWitnessHeartbeat 查询系统见证人心跳
func (api *PrivateWitnessAPI) GetSystemWitnessHeartbeat(address common.Address, period uint64) (interface{}, error) {
	state, err := api.b.DagChain().GetChainTailState(params.SCAccount)
	if err != nil {
		return nil, err
	}
	if period <= 0 {
		period = api.b.DagChain().GetPeriod()
	}
	hc, err := sc.ReadSystemHeartbeat(state)
	if err != nil {
		return nil, err
	}
	addrHc := hc.Query(address)
	if addrHc.Address == common.EmptyAddress {
		return nil, nil
	}
	type resultType struct {
		Address    common.Address
		LastTurn   uint64
		LastNumber uint64
		Count      uint64
	}
	ret := resultType{
		Address:    address,
		LastTurn:   addrHc.LastTurn,
		LastNumber: addrHc.LastNumber,
		Count:      0,
	}
	ret.Count = addrHc.Count.Query(period).UnitCount

	return ret, nil
}

func (api *PrivateWitnessAPI) GetHeartbeatLastTurn(address common.Address) (interface{}, error) {
	last := api.b.DagChain().GetChainTailHead(params.SCAccount)
	db, err := api.b.DagChain().StateAt(last.MC, last.StateRoot)
	if err != nil {
		return nil, err
	}
	role := sc.GetAccountStatus(db, address)
	lastTurn := sc.QueryHeartbeatLastRound(db, role.Role, address)
	lastNumber := sc.QueryHeartbeatLastNumber(db, role.Role, address)
	return struct {
		LastTurn   uint64
		LastNumber uint64
		NowTurn    uint64
	}{lastTurn, lastNumber, params.CalcRound(last.Number)}, nil
}
