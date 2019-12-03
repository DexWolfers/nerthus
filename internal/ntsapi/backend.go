// Author: @ysqi

// Package ntsapi implements the general ntsereum API functions.
package ntsapi

import (
	"context"
	"math/big"

	"gitee.com/nerthus/nerthus/nts/txpipe"

	"gitee.com/nerthus/nerthus/accounts"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/compiler/solidity"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/rpc"
)

// Backend interface provides the common API services (that are provided by
// both full and light clients) with access to necessary functions.
type Backend interface {
	// general ntsereum API
	ProtocolVersion() int
	ChainDb() ntsdb.Database
	EventMux() *event.TypeMux
	AccountManager() *accounts.Manager
	// Genesis returns Genesis unit
	Genesis() *core.Genesis

	SendTransaction(signedPayment *types.Transaction) error
	// tx from remote
	SendRemoteTransaction(tx *types.Transaction) error
	// SendRemoteTransactions for stress
	SendRemoteTransactions(txs []*types.Transaction) error

	ChainConfig() *params.ChainConfig

	SuggestPrice(ctx context.Context) (*big.Int, error)

	GetPoolNonce(ctx context.Context, addr common.Address) (uint64, error)
	DagChain() *core.DagChain
	DagReader() *core.DAG
	// 增加见证接口
	// Author:@kulics
	StartWitness(mainAccount common.Address, password string) error
	StopWitness() error
	// 增加成员接口
	// Author:@kulics
	StartMember(ac common.Address, password string) error
	StopMember() error
	TxPool() *core.TxPool
	TxPipe() *txpipe.Pipe
}

func GetAPIs(apiBackend Backend) []rpc.API {
	nonceLock := new(AddrLocker)
	return []rpc.API{
		{
			Namespace: "nts",
			Version:   "1.0",
			Service:   NewPublicNerthusAPI(apiBackend),
			Public:    true,
		}, {
			Namespace: "nts",
			Version:   "1.0",
			Service:   NewPublicChainAPI(apiBackend),
			Public:    true,
		}, {
			Namespace: "nts",
			Version:   "1.0",
			Service:   NewPublicTransactionPoolAPI(apiBackend, nonceLock),
			Public:    true,
		}, {
			Namespace: "personal",
			Version:   "1.0",
			Service:   NewPrivateAccountAPI(apiBackend, nonceLock),
			Public:    false,
		}, {
			Namespace: "solc",
			Version:   "1.0",
			Service:   &CompilerAPI{solidity.Language},
			Public:    true,
		}, {
			Namespace: "witness",
			Version:   "1.0",
			Service:   NewPrivateWitnessAPI(apiBackend),
			Public:    false,
		},
	}
}
