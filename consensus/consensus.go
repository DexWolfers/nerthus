package consensus

import (
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/consensus/bft"
	"gitee.com/nerthus/nerthus/consensus/protocol"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/params"
)

// ChainReader 定义一组在单元校验是所需要访问单元信息的方法集
type ChainReader interface {
	// Config 获取 Unitcahin的链配置信息
	Config() *params.ChainConfig

	InsertChain(chain types.Blocks) (int, error)

	WriteRightUnit(bstart time.Time, state *state.StateDB, unit *types.Unit, receipts types.Receipts, logs []*types.Log) error

	VerifyUnit(unit *types.Unit, verifyHeader func(*types.Unit) error) (receipts types.Receipts, statedb *state.StateDB, logs []*types.Log, err error)

	// GetHeader 通过单元位置获取单元头
	GetHeader(hash common.Hash) *types.Header

	// GetHeaderByNumber 通过单元位置获取单元头
	GetHeaderByNumber(mcaddr common.Address, number uint64) *types.Header

	// GetHeaderByHash 通过单元头Hash获取单元头
	GetHeaderByHash(hash common.Hash) *types.Header

	// GetUnit 通过单元位置信息获取单元
	GetUnit(hash common.Hash) *types.Unit

	// GetChainTail 获取最新高度的单元
	GetChainTailHash(mc common.Address) common.Hash

	// GenesisHash 获取创世hash
	GenesisHash() common.Hash

	//获取
	GetChainWitnessLib(address common.Address, scHash ...common.Hash) ([]common.Address, error)

	GetBrothers(addr common.Address, number uint64) (common.Hash, error)

	GetVoteMsg(uhash common.Hash) (types.Votes, error)
	GetVoteMsgAddresses(hash common.Hash) []common.Address

	GetChainTailState(mcaddr common.Address) (*state.StateDB, error)

	StateAt(chain common.Address, root common.Hash) (*state.StateDB, error)
	GetStateDB(chain common.Address, root common.Hash) (*state.StateDB, error)
	ReadConfigValue(scHeader *types.Header, key common.Hash) (value common.Hash, err error)
	GetUnitState(unitHash common.Hash) (*state.StateDB, error)

	// GetStableHash 获取指定高度的稳定单元哈希
	GetStableHash(chain common.Address, number uint64) common.Hash
	GetTransaction(txHash common.Hash) *types.Transaction

	GetChainTailHead(chain common.Address) *types.Header
	GetUnitByHash(hash common.Hash) *types.Unit
	GetUnitByNumber(mcaddr common.Address, number uint64) *types.Unit
	ClearUnit(uhash common.Hash, delAll bool) error
}

// 单元校验接口
type Engine interface {
	// 取单元的提案人
	Author(unit *types.Unit) (common.Address, error)

	// 校验单元头
	VerifyHeader(chain ChainReader, unit *types.Unit, seal bool) error
	// 单元与已验证的提案对比，如果一致则不需要重新执行单元内的交易
	VerifyWithProposal(unit *types.Unit) (*bft.MiningResult, bool)

	// 校验单元的投票
	VerifySeal(chain ChainReader, unit *types.Unit, chainWitness []common.Address) error
}
type Miner interface {
	Engine
	Start() error
	Stop() error

	// 链共识状态
	MiningStatus(chain common.Address) *bft.ConsensusInfo
	// 共识消息入口
	OnMessage(msg protocol.MessageEvent)
	// 强制停止链工作，一般在发现链作恶时将需要立刻触发
	StopChainForce(chain common.Address)
	// 有新任务信号
	SendNewJob(chain common.Address)
	// 新单元信号
	SendNewUnit(chain common.Address, number uint64, uhash common.Hash)

	NextChainSelector(func(exist func(nextChain common.Address) bool) common.Address)
	// 尝试回滚共识提案内的交易
	RollBackProposal(newUnit common.Hash, mc common.Address)
}
