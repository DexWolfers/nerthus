package sc

import (
	"math/big"

	"gitee.com/nerthus/nerthus/accounts"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/rpc"
)

type Backend interface {
	AccountManager() *accounts.Manager
	ChainConfig() *params.ChainConfig
	GetLastTxStatusByType(address common.Address, t types.TransactionType) (types.TransactionStatus, error)
}

type DagChain interface {
	GetChainTailState(mcaddr common.Address) (*state.StateDB, error)
	GetChainTailHash(mc common.Address) common.Hash
	GetUnitState(unitHash common.Hash) (*state.StateDB, error)
	GetChainTailHead(mcaddr common.Address) *types.Header

	GetTransaction(txHash common.Hash) *types.Transaction
	SubscribeChainEvent(ch chan<- types.ChainEvent) event.Subscription
	GetChainWitnessLib(addr common.Address, scUnitHash ...common.Hash) (lib []common.Address, err error)

	// GetHeader 根据账户链账户和单元编号获取头信息
	GetHeader(hash common.Hash) *types.Header

	// GetBalance 获取余额
	GetBalance(address common.Address) *big.Int

	// GetWitnessReport 获取见证人周期工作报表数据
	GetWitnessWorkReport(period uint64) map[common.Address]types.WitnessPeriodWorkInfo
	GetWitnessReport(witness common.Address, period uint64) types.WitnessPeriodWorkInfo
	GenesisHash() common.Hash
	GetHeaderByHash(hash common.Hash) *types.Header
	GetStateByNumber(chain common.Address, number uint64) (*state.StateDB, error)
}

func GetApis(apiBackend Backend, dagchain DagChain) []rpc.API {
	return []rpc.API{
		{
			Namespace: "nts",
			Version:   "1.0",
			Service:   NewPublicScApi(apiBackend, dagchain),
			Public:    true,
		},
	}
}
