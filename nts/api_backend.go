// Author: @ysqi

package nts

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"math/big"

	"gitee.com/nerthus/nerthus/accounts"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/consensus"
	consensusProtocol "gitee.com/nerthus/nerthus/consensus/protocol"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/bloombits"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/statistics"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/node"
	"gitee.com/nerthus/nerthus/nts/protocol"
	"gitee.com/nerthus/nerthus/nts/txpipe"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/p2p/discover"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/rpc"
)

var (
	ErrHeaderEmpty      = errors.New("unit's header is empty")
	ErrResultEmpty      = errors.New("unit's result is empty")
	ErrParentHashEmpty  = errors.New("unit's parent'0 hash is empty")
	ErrParent1HashEmpty = errors.New("unit's parent'1 hash is empty")
	ErrParent2HashEmpty = errors.New("unit's parent'2 hash is empty")
	ErrWitnessHashEmpty = errors.New("unit's witness hash is empty")
	ErrVoteHashEmpty    = errors.New("unit's vote hash is empty")
)

// NtsApiBackend 用于对其他服务提供辅助能力
type NtsApiBackend struct {
	nts *Nerthus
	// 添加 以便使用wishper
	ctx *node.ServiceContext
}

func (b *NtsApiBackend) ChainConfig() *params.ChainConfig {
	return b.nts.chainConfig
}
func (b *NtsApiBackend) ChainDb() ntsdb.Database {
	return b.nts.ChainDb()
}

func (b *NtsApiBackend) EventMux() *event.TypeMux {
	return b.nts.EventMux()
}

func (b *NtsApiBackend) AccountManager() *accounts.Manager {
	return b.nts.AccountManager()
}

func (b *NtsApiBackend) ProtocolVersion() int {
	return b.nts.NtsVersion()
}

func (b *NtsApiBackend) ServiceContext() *node.ServiceContext {
	return b.ctx
}

func (b *NtsApiBackend) GetNode() *discover.Node {
	return b.nts.ProtocolHandler.P2PSrv.Self()
}

func (b *NtsApiBackend) PeerSet() protocol.PeerSet {
	return b.nts.ProtocolHandler
}

func (b *NtsApiBackend) GetSysWitList() (common.AddressList, error) {
	return b.nts.getSysWitList()
}

func (b *NtsApiBackend) DagChain() *core.DagChain { return b.nts.dagchain }
func (b *NtsApiBackend) DagReader() *core.DAG     { return b.nts.dag }
func (b *NtsApiBackend) Genesis() *core.Genesis   { return b.nts.config.Genesis }

//发送payment
func (b *NtsApiBackend) SendTransaction(p *types.Transaction) error {
	return b.nts.txPool.AddLocal(p)
}

// 处理rpc发过来的交易
func (b *NtsApiBackend) SendRemoteTransaction(tx *types.Transaction) error {
	return b.nts.txPool.AddRemoteSync(tx)
}

func (b *NtsApiBackend) SendRemoteTransactions(txs []*types.Transaction) error {
	return b.nts.txPool.AddRemotes(txs)
}

// 获取主账号
func (b *NtsApiBackend) GetMainAccount() common.Address {
	return b.nts.mainAccount
}

// 见证人是否存在见证组内
func (b *NtsApiBackend) IsJoinWitnessGroup(address common.Address) bool {
	if !b.nts.IsWitnessing() {
		return false
	}
	return b.nts.witness.WitnessInGroup(address)
}

func (b *NtsApiBackend) SuggestPrice(ctx context.Context) (*big.Int, error) {
	return new(big.Int).SetUint64(b.ChainConfig().Get(params.ConfigParamsLowestGasPrice)), nil
}

func (b *NtsApiBackend) GetPoolNonce(ctx context.Context, addr common.Address) (uint64, error) {
	return 0, nil
}

func (b *NtsApiBackend) TxPool() *core.TxPool {
	return b.nts.txPool
}
func (b *NtsApiBackend) TxPipe() *txpipe.Pipe {
	return b.nts.ProtocolHandler.subPipe
}

func (b *NtsApiBackend) HeaderByNumber(ctx context.Context, mcaddr common.Address, number rpc.BlockNumber) (*types.Header, error) {
	return b.nts.dagchain.GetHeaderByNumber(mcaddr, uint64(number)), nil
}
func (b *NtsApiBackend) UnitByNumber(ctx context.Context, mcaddr common.Address, number rpc.BlockNumber) (*types.Unit, error) {
	return b.nts.dagchain.GetUnitByNumber(mcaddr, uint64(number)), nil
}

func (b *NtsApiBackend) StateAndHeaderByNumber(ctx context.Context, mcaddr common.Address, number rpc.BlockNumber) (*state.StateDB, *types.Header, error) {
	unit, err := b.UnitByNumber(ctx, mcaddr, number)
	if unit == nil || err != nil {
		return nil, nil, err
	}
	stateDb, err := b.nts.dagchain.StateAt(mcaddr, unit.Root())
	return stateDb, unit.Header(), err
}

// 开启见证人服务
// Author:@kulics
func (b *NtsApiBackend) StartWitness(mainAccount common.Address, password string) error {
	return b.nts.StartWitness(mainAccount, password)
}

func (b *NtsApiBackend) GetEngine() consensus.Engine {
	return b.nts.bftEngine
}

// 关闭见证人服务
// Author:@kulics
func (b *NtsApiBackend) StopWitness() error {
	return b.nts.StopWitness()
}

// 开启成员服务
// Author:@kulics
func (b *NtsApiBackend) StartMember(ac common.Address, password string) error {
	return b.nts.StartMember(ac, password)
}

// 关闭成员服务
// Author:@kulics
func (b *NtsApiBackend) StopMember() error {
	return b.nts.StopMember()
}

// 当前见证人是否是系统见证人
func (b *NtsApiBackend) CurrentIsSystemWitness() (bool, error) {
	if b.nts.witness != nil && b.nts.witness.Mining() {
		if b.nts.witness.GroupIndex() == sc.DefSysGroupIndex {
			return true, nil
		}
		return false, nil
	}
	return false, errors.New("witness is stop")
}

// 获取系统链见证人名单
func (b *NtsApiBackend) GetSystemWitnessList() ([]common.Address, error) {
	return b.nts.GetSystemWitnessList()
}

func (b *NtsApiBackend) Sta() *statistics.Statistic {
	return b.nts.sta
}

func (b *NtsApiBackend) GetRD() ntsdb.RD {
	return b.nts.rd
}

func (n *NtsApiBackend) GetPrivateKey() (*ecdsa.PrivateKey, error) {
	if w := n.nts.Witness(); w != nil {
		return w.GetPrivateKey(), nil
	}
	return nil, errors.New("witness not started")
}

// 通过类型获取交易状态
func (n *NtsApiBackend) GetLastTxStatusByType(address common.Address, t types.TransactionType) (types.TransactionStatus, error) {
	txHash, err := n.Sta().GetLastTxHashByType(address, t)
	if err != nil {
		return types.TransactionStatusFailed, err
	}
	if txHash.Empty() {
		return types.TransactionStatusNot, nil
	}
	tx := n.TxPool().GetTxWithDisk(txHash)
	if tx == nil {
		tx = n.DagChain().GetTransaction(txHash)
	}
	if tx == nil {
		return types.TransactionStatusNot, errors.New("transaction not find")
	}
	return GetTxStatus(n.DagChain(), tx.Hash(), tx)
}

// 处理共识消息，不使用事件，而是直接调用见证服务
func (n *NtsApiBackend) HandleConsensusEvent(ev consensusProtocol.MessageEvent) {
	if n.nts.IsWitnessing() {
		n.nts.witness.HandleConsensusMsg(ev)
	}
}

type FilterAPIBackend struct {
	n *Nerthus
}

func newFilterAPIBackend(n *Nerthus) *FilterAPIBackend {
	return &FilterAPIBackend{
		n: n,
	}
}

func (f *FilterAPIBackend) ChainDb() ntsdb.Database {
	return f.n.ChainDb()
}

func (f *FilterAPIBackend) EventMux() *event.TypeMux {
	return f.n.EventMux()
}

func (f *FilterAPIBackend) HeaderByNumber(ctx context.Context, chain common.Address, blockNr rpc.BlockNumber) (*types.Header, error) {
	if blockNr == rpc.LatestBlockNumber {
		return f.n.DagChain().GetChainTailHead(chain), nil
	}
	h := f.n.DagChain().GetHeaderByNumber(chain, blockNr.MustUint64())
	if h == nil {
		return nil, errors.New("not found head by chain and number")
	}
	return h, nil
}

func (f *FilterAPIBackend) GetReceipts(ctx context.Context, blockHash common.Hash) (types.Receipts, error) {
	mc, number := f.n.DagChain().GetUnitNumber(blockHash)
	if mc.Empty() {
		return nil, errors.New("not found unit info by unit hash")
	}
	return core.GetUnitReceipts(f.ChainDb(), mc, blockHash, number), nil
}

func (f *FilterAPIBackend) SubscribeTxPreEvent(ch chan<- types.TxPreEvent) event.Subscription {
	return f.n.txPool.SubscribeTxPreEvent(ch)
}

func (f *FilterAPIBackend) SubscribeChainEvent(ch chan<- types.ChainEvent) event.Subscription {
	return f.n.DagChain().SubscribeChainEvent(ch)
}

func (f *FilterAPIBackend) SubscribeRemovedLogsEvent(ch chan<- types.RemovedLogsEvent) event.Subscription {
	return f.n.DagChain().SubscribeRemovedLogsEvent(ch)
}

func (f *FilterAPIBackend) SubscribeLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return f.n.dagchain.SubscribeLogsEvent(ch)
}

func (f *FilterAPIBackend) BloomStatus() (uint64, uint64) {
	return params.BloomBitsBlocks, 0
}

func (f *FilterAPIBackend) ServiceFilter(ctx context.Context, session *bloombits.MatcherSession) {
	panic("implement me")
}
func (f *FilterAPIBackend) HeaderByHash(ctx context.Context, unitHash common.Hash) (*types.Header, error) {
	header := f.n.DagChain().GetHeader(unitHash)
	if header == nil {
		return nil, errors.New("not found header by hash")
	}
	return header, nil
}
