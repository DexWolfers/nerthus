package ntsapi

import (
	"context"
	"errors"
	"math/big"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/hexutil"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"
)

var (
	ErrReceiveFailed = errors.New("estimate gas failed")
)

// 模拟器接口
type PublicChainAPI struct {
	b Backend
}

func NewPublicChainAPI(b Backend) *PublicChainAPI {
	return &PublicChainAPI{b}
}

// EstimateGas 本地模拟计算交易大概所需 gas
func (s *PublicChainAPI) EstimateGas(ctx context.Context, args SendTxArgs) (hexutil.Uint64, error) {
	if err := args.init(ctx, s.b); err != nil {
		return 0, err
	}
	usedGas, err := s.doEstimateGas(ctx, args)
	if err != nil {
		return 0, err
	}
	return hexutil.Uint64(usedGas), nil
}

// doEstimateGas 计算gas使用量
func (s *PublicChainAPI) doEstimateGas(ctx context.Context, args SendTxArgs) (uint64, error) {
	if args.To.IsContract() { // 取合约链
		if sc.IsCallCreateContractTx(args.To, args.Data) {
			gas, _, err := s.deployContract(args)
			return gas, err
		}
		gas, _, err := s.executeContract(args)
		return gas, err
	} else { //普通交易获取用户链
		return s.transfer(args)
	}
}

// transfer 转账交易
func (s *PublicChainAPI) transfer(args SendTxArgs) (uint64, error) {
	header := s.b.DagChain().GetHeaderByHash(s.b.DagChain().GetChainTailHash(args.From))
	stateDB, err := s.b.DagChain().StateAt(header.MC, header.StateRoot)
	if err != nil {
		return 0, err
	}
	// 链的第一个单元需要迁移数据
	if header.Number == 0 && args.From != params.SCAccount {
		stateDB, err = stateDB.NewChainState(args.From)
	}
	// 模拟执行前，保证余额充足
	stateDB.AddBalance(args.From, math.MaxBig256)
	tx := types.NewTransaction(args.From, args.To, args.Amount.ToInt(), args.Gas.ToInt().Uint64(),
		args.GasPrice.ToInt().Uint64(), args.Timeout.ToInt().Uint64(), args.Data)
	msg := types.NewMessage(tx, tx.Gas(), args.From, tx.To(), false)

	if err = s.b.TxPool().ValidateTx(args.From, true, tx, math.MaxBig256, true); err != nil {
		return 0, err
	}
	txExec := &types.TransactionExec{Action: types.ActionTransferPayment, TxHash: tx.Hash(), Tx: tx}
	receipt, err := core.ProcessTransaction(s.b.ChainConfig(), s.b.DagChain(), stateDB, header, new(core.GasPool).AddGas(math.MaxBig256), txExec, big.NewInt(0), simulationConfig(), msg)
	if err != nil {
		return 0, err
	}
	if receipt.Failed {
		return 0, ErrReceiveFailed
	}
	return receipt.GasUsed, nil
}

func (s *PublicChainAPI) updateHeader(h *types.Header) {
	if h.MC == params.SCAccount {
		return
	}
	//获取最新系统链地址
	// 因为一些逻辑有实用到系统高度，因此需要设置为正确的系统链地址
	scHead := s.b.DagChain().GetChainTailHead(params.SCAccount)
	h.SCNumber = scHead.Number
	h.SCHash = scHead.Hash()
}

// executeContract 执行合约交易
func (s *PublicChainAPI) executeContract(args SendTxArgs) (uint64, []byte, error) {
	fromHeader := s.b.DagChain().GetHeaderByHash(s.b.DagChain().GetChainTailHash(args.To))
	s.updateHeader(fromHeader)

	fromStateDB, err := s.b.DagChain().StateAt(fromHeader.MC, fromHeader.StateRoot)
	// 链的第一个单元需要迁移数据
	if fromHeader.Number == 0 && args.To != params.SCAccount {
		fromStateDB, err = fromStateDB.NewChainState(args.To)
	}
	if err != nil {
		return 0, nil, err
	}

	// 模拟执行前，保证余额充足
	fromStateDB.AddBalance(args.From, math.MaxBig256)
	tx := types.NewTransaction(args.From, args.To, args.Amount.ToInt(), args.Gas.ToInt().Uint64(),
		args.GasPrice.ToInt().Uint64(), args.Timeout.ToInt().Uint64(), args.Data)
	msg := types.NewMessage(tx, tx.Gas(), args.From, tx.To(), false)

	txExec := &types.TransactionExec{Action: types.ActionContractFreeze, TxHash: tx.Hash(), Tx: tx}
	fromReceipt, err := core.ProcessTransaction(s.b.ChainConfig(), s.b.DagChain(), fromStateDB, fromHeader, new(core.GasPool).AddGas(math.MaxBig256), txExec, big.NewInt(0), simulationConfig(), msg)
	if err != nil {
		return 0, nil, err
	}
	if fromReceipt.Failed {
		return 0, nil, ErrReceiveFailed
	}
	log.Debug("formReceipt", "gas", fromReceipt.GasUsed)

	hash := s.b.DagChain().GetChainTailHash(args.To)
	toHeader := types.CopyHeader(s.b.DagChain().GetHeaderByHash(hash))
	// next header(genesis)
	toHeader.MC = args.To
	toHeader.Number += 1
	toHeader.ParentHash = hash
	s.updateHeader(toHeader)

	toStateDB, err := s.b.DagChain().StateAt(toHeader.MC, toHeader.StateRoot)
	if err != nil {
		return 0, nil, err
	}
	txExec = &types.TransactionExec{Action: types.ActionContractDeal, TxHash: tx.Hash(), Tx: tx}
	toReceipt, err := core.ProcessTransaction(s.b.ChainConfig(), s.b.DagChain(), toStateDB, toHeader, new(core.GasPool).AddGas(math.MaxBig256), txExec, big.NewInt(0), simulationConfig(), msg)
	if err != nil {
		return 0, nil, err
	}
	if toReceipt.Failed {
		return 0, nil, ErrReceiveFailed
	}
	log.Debug("toReceipt", "gas", toReceipt.GasUsed, "failed", toReceipt.Failed)
	return fromReceipt.GasUsed + toReceipt.GasUsed, toReceipt.Output, nil
}

// deployContract 部署合约交易
func (s *PublicChainAPI) deployContract(args SendTxArgs) (uint64, []byte, error) {
	deployGas, outPut, err := s.executeContract(args)
	if err != nil {
		return 0, nil, err
	}
	result, err := sc.GetContractOutput(sc.FuncCreateContract, outPut)
	if err != nil {
		log.Debug("to output", "err", err)
		return 0, nil, err
	}
	contractAddr := result.(sc.NewCtWit).Contract
	log.Debug("deploy contract", "addr", contractAddr)

	toHeader := &types.Header{
		MC:         contractAddr,
		Number:     1,
		ParentHash: s.b.DagChain().GenesisHash(),
	}
	s.updateHeader(toHeader)

	toStateDB, err := s.b.DagChain().StateAt(s.b.DagChain().Genesis().MC(), s.b.DagChain().Genesis().Root())
	if err != nil {
		return 0, nil, err
	}

	tx := types.NewTransaction(args.From, contractAddr, args.Amount.ToInt(), args.Gas.ToInt().Uint64(),
		args.GasPrice.ToInt().Uint64(), args.Timeout.ToInt().Uint64(), args.Data)

	// 该合约属于部署合约, 需要走新建合约的流程, to == params.SCAccount
	msg := types.NewMessage(tx, tx.Gas(), args.From, args.To, false)

	txExec := &types.TransactionExec{Action: types.ActionContractDeal, TxHash: tx.Hash(), Tx: tx}
	receipt, err := core.ProcessTransaction(s.b.ChainConfig(), s.b.DagChain(), toStateDB, toHeader, new(core.GasPool).AddGas(math.MaxBig256), txExec, big.NewInt(0), simulationConfig(), msg)
	if err != nil {
		return 0, nil, err
	}
	if receipt.Failed {
		return 0, nil, ErrReceiveFailed
	}
	log.Debug("receipt", "gas", receipt.GasUsed, "failed", receipt.Failed)
	return deployGas + receipt.GasUsed, receipt.Output, nil
}

// Call 返回交易的output
func (s *PublicChainAPI) Call(args SendTxArgs) (hexutil.Bytes, error) {
	if err := args.init(context.TODO(), s.b); err != nil {
		return nil, err
	}
	if args.To.IsContract() { // 取合约链
		if sc.IsCallCreateContractTx(args.To, args.Data) {
			_, output, err := s.deployContract(args)
			log.Debug("call", "output", common.Bytes2Hex(output))
			return output, err
		}
		_, output, err := s.executeContract(args)
		log.Debug("call", "output", common.Bytes2Hex(output))
		return output, err
	} else { //普通交易获取用户链
		return nil, errors.New("the transaction is not contract deploy")
	}
}

// simulationConfig 标识该交易为模拟器执行的
func simulationConfig() vm.Config {
	return vm.Config{
		EnableSimulation: true,
	}
}
