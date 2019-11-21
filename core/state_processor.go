package core

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"gitee.com/nerthus/nerthus/consensus/arbitration"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/common/tracetime"
	"gitee.com/nerthus/nerthus/consensus"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"
)

// StateProcessor 交易处理器，不断处理单元中交易，将账户状态不断推进
//
// StateProcessor 实现 Processor 接口。
type StateProcessor struct {
	config *params.ChainConfig // Chain 配置选项
	mc     *DagChain           //  主链
	engine consensus.Engine    // 共识
}

// NewStateProcessor 构建一个新的StateProcessor.
func NewStateProcessor(config *params.ChainConfig, mc *DagChain, engine consensus.Engine) *StateProcessor {
	return &StateProcessor{
		config: config,
		mc:     mc,
		engine: engine,
	}
}

// Process 按协议规则逐笔处理单元中交易，并变更账户状态。
//
// Process 返回处理过程中的交易信息和发送的日志，以及本次处理所消耗的Gas量。
// 如果某笔交易执行失败则终止并返回错误信息。
func (p *StateProcessor) Process(unit *types.Unit, statedb *state.StateDB, cfg vm.Config) (types.Receipts, []*types.Log, uint64, error) {
	defer tracetime.New().Stop()
	var (
		header       = unit.Header()
		totalUsedGas = new(big.Int)
		receipts     types.Receipts
		allLogs      []*types.Log
		gp           = new(GasPool).AddGas(new(big.Int).SetUint64(unit.GasLimit()))
	)

	for i, tx := range unit.Transactions() {
		statedb.Prepare(tx.TxHash, unit.Hash(), i, unit.Number())
		receipt, err := ProcessTransaction(p.config, p.mc, statedb, header, gp, tx, totalUsedGas, cfg, nil)
		log.Trace("process unit transaction", "index", i, "txhash", tx.TxHash, "action", tx.Action, "mc", unit.MC(), "number", unit.Number(), "err", err)
		if err != nil {
			return nil, nil, 0, err
		}
		receipts = append(receipts, receipt)
		allLogs = append(allLogs, receipt.Logs...)
	}

	return receipts, allLogs, totalUsedGas.Uint64(), nil
}

// SimulatorTranscation 模拟交易，将执行此交易，并反馈执行信息（返回值，已用gas，是否失败，错误信息）
func SimulatorTranscation(ctx context.Context, config *params.ChainConfig, mc *DagChain, statedb *state.StateDB, header *types.Header,
	msg types.Message, cfg vm.Config) (output []byte, usedGas *big.Int, failed bool, err error) {
	// TODO(ysqi): 模拟器需要修改暂注释
	return nil, new(big.Int).SetUint64(params.TxGas * 8), false, nil
	//var gp GasPool
	//gp.AddGas(msg.Gas())
	//rec, err := applyTransaction(config, mc, statedb, header, &gp, big.NewInt(0), cfg, msg, common.Hash{})
	//if err != nil {
	//	return
	//}
	//output, usedGas, failed = rec.Output, rec.GasUsed, rec.Failed
	return
}

// 批量处理交易
func ProcessMultiTransaction(config *params.ChainConfig, mc *DagChain, statedb *state.StateDB, header *types.Header, txs types.TxExecs, gasLimit uint64, cfg vm.Config) (types.TxExecs, types.Receipts, []*types.Log, uint64, error) {
	var (
		totalUsedGas = new(big.Int) // 总共使用gas
		receipts     types.Receipts // 结果数组
		successTx    types.TxExecs  // 成功交易数组
		allLogs      []*types.Log
		gp           = new(GasPool).AddGas(new(big.Int).SetUint64(gasLimit))
	)

	// 遍历交易
	for i, tx := range txs {
		statedb.Prepare(tx.TxHash, common.Hash{}, i, header.Number)

		// 处理单条交易，错误交易跳过
		receipt, err := ProcessTransaction(config, mc, statedb, header, gp, tx, totalUsedGas, cfg, nil)
		log.Trace("batch exec transaction", "index", i, "txhash", tx.TxHash, "action", tx.Action, "mc", header.MC, "number", header.Number, "err", err)
		if err != nil {
			continue
		}
		// 装填数据
		receipts = append(receipts, receipt)
		allLogs = append(allLogs, receipt.Logs...)
		successTx = append(successTx, tx)
	}

	return successTx, receipts, allLogs, totalUsedGas.Uint64(), nil
}

// ProcessTransaction 在此环境下尝试将执行交易，更新到状态库中。
// 返回交易回执、已用gas以及错误（如果存在），错误代表该交易不会被接受，不会放入单元中。
func ProcessTransaction(config *params.ChainConfig, mc *DagChain, statedb *state.StateDB, header *types.Header, gp *GasPool,
	tx *types.TransactionExec, usedGas *big.Int, cfg vm.Config, msg *types.Message) (*types.Receipt, error) {
	if tx == nil {
		return nil, errors.New("ProcessTransaction: input params tx is nil")
	}

	originTx, err := mc.QueryOriginTx(tx)
	if err != nil {
		if err == ErrUnitNotFind {
			return nil, consensus.ErrUnknownAncestor
		}
		return nil, err
	}
	if originTx == nil {
		return nil, errors.New("not found origin transaction")
	}
	if !tx.PreStep.ChainID.Empty() {
		//禁止一切受影响的单元继续落地
		if err := arbitration.CheckChainIsStopped(mc.chainDB, tx.PreStep.ChainID, tx.PreStep.Height); err != nil {
			return nil, err
		}
	}

	//执行前必须清空所有的交易流水，否则无法准确记录本次交易所产生的流水
	statedb.ClearFlow()
	switch tx.Action {
	case types.ActionTransferPayment: //转账
		return ApplyPayment(config, mc, statedb, header, gp, originTx, usedGas, cfg, msg)
	case types.ActionContractFreeze: //执行合约前冻结
		return ApplyFreeze(config, mc, statedb, header, gp, originTx, usedGas, cfg, msg)
	case types.ActionContractDeal, types.ActionPowTxContractDeal, types.ActionSCIdleContractDeal: //执行合约
		return ApplyToContract(config, mc, statedb, header, gp, originTx, usedGas, cfg, tx, msg)

	case types.ActionTransferReceipt: //收款
		return ApplyReceive(config, mc, statedb, header, gp, originTx, usedGas, cfg, tx)
	case types.ActionContractReceipt: //合约执行完成，收款
		return ActionContractReceipt(config, mc, statedb, header, gp, originTx, usedGas, cfg, tx)
	case types.ActionContractRefund: //合约执行完成，返还多余冻结
		return ApplyRefund(config, mc, statedb, header, gp, originTx, usedGas, cfg, tx)
	}
	return nil, fmt.Errorf("can not handle action %s", tx.Action)
}

// ApplyTransaction 在此环境下尝试将执行交易，更新到状态库中。
// 返回交易回执、已用gas以及错误（如果存在），错误代表该交易不会被接受，不会放入单元中。
func ApplyTransaction(config *params.ChainConfig, mc *DagChain, statedb *state.StateDB, header *types.Header, gp *GasPool, tx *types.Transaction, toalUsedGas *big.Int, cfg vm.Config, msg *types.Message) (*types.Receipt, error) {
	if to := tx.To(); to.IsContract() {
		return ApplyToContract(config, mc, statedb, header, gp, tx, toalUsedGas, cfg, nil, msg)
	}
	return ApplyPayment(config, mc, statedb, header, gp, tx, toalUsedGas, cfg, msg)
}

// ApplyPayment 转账
func ApplyPayment(config *params.ChainConfig, mc *DagChain, statedb *state.StateDB, header *types.Header, gp *GasPool, tx *types.Transaction,
	toalUsedGas *big.Int, cfg vm.Config, msg *types.Message) (receipt *types.Receipt, err error) {
	if msg == nil {
		msg, err = tx.AsMessage(types.MakeSigner(*config), tx.Gas())
		if err != nil {
			return nil, err
		}
	}

	if msg.To().IsContract() {
		err = errors.New("does not allow payment to a contract address")
		return
	}

	action := types.ActionTransferPayment

	// 检查 Action
	if err = CheckTxActionInChain(statedb, header.MC, msg.ID(), action); err != nil {
		return
	}
	output, gas, failed, err := ApplyMessage(nil, statedb, action, msg, gp, header.MC)
	if err != nil {
		return
	}

	receipt = createReceipt(config, types.UnitID{}, header, statedb, tx.Hash(), action, output, failed, tx.Gas(), toalUsedGas, gas)
	return
}

func createReceipt(config *params.ChainConfig, preStep types.UnitID, header *types.Header, statedb *state.StateDB, txHash common.Hash,
	action types.TxAction, output []byte, failed bool,
	remaininGas uint64, toalUsedGas *big.Int, usedGas uint64, keepFlows ...common.Address) *types.Receipt {
	// 获取账户链上的记账流水
	var coinflows types.Coinflows

	root := common.Hash{}
	flows := statedb.GetBalanceChanged()
	if !failed {

		//在1高度1 上清理不属于此链的账户。因为创世中存在多个账号Obj
		if header.Number == 1 {
			statedb.RangObjects(func(address common.Address) bool {
				obj := statedb.GetOrNewStateObject(address)
				if !obj.OnChain(header.MC) {
					statedb.Suicide(address)
					log.Debug("number 0 will delete chain", "chain", address)
				}
				return true
			})
		}

		for _, v := range flows {
			//如果金额是减少，则不需要加入流水。但有可能是合约执行者可允许忽略，由外部负责处理。
			//这样做的目的是：即使合约执行者的v.Diff 为0，则说明合约有转账给他需要额外处理。
			if v.Diff.Sign() <= 0 {
				if len(keepFlows) == 0 {
					continue
				}
				if v.Address != keepFlows[0] {
					continue
				}
			}

			//如果该地址是合约且存在于此链上则不记录
			if v.Address.IsContract() && !statedb.GetCodeHash(v.Address).Empty() {
				continue
			}
			coinflows = append(coinflows, types.Coinflow{
				To:     v.Address,
				Amount: new(big.Int).Set(&v.Diff),
			})
		}
	}

	// 累计已用量
	toalUsedGas.Add(toalUsedGas, new(big.Int).SetUint64(usedGas))

	// 创建交易回执，含中层根、已用gas
	receipt := types.NewReceipt(action, root, failed, toalUsedGas)
	receipt.TxHash = txHash
	receipt.GasUsed = usedGas
	receipt.GasRemaining = math.ForceSafeSub(remaininGas, usedGas) // 当前剩余Gas= 之前剩余Gas - 当前使用
	// 获取交易日志，并创建就一个 bloom 过滤器
	receipt.Logs = statedb.GetLogs(txHash)
	receipt.Bloom = types.CreateBloom(types.Receipts{receipt})
	receipt.PreStep = preStep
	//log.Trace("GasChangeLog", "txhash", txHash, "action", action, "failed", receipt.Failed,
	//	"height", header.Number,
	//	"init", remaininGas,
	//	"used", usedGas,
	//	"remaining", receipt.GasRemaining,
	//	"coninflows", coinflows,
	//)

	// 返回记账流水
	receipt.Coinflows = coinflows
	if !failed { //只有成功时才有有输出
		receipt.Output = output // TODO: 暂不截断，后续该数据将不包含在区块数据中
	}
	return receipt
}

func applyToContract(config *params.ChainConfig, mc *DagChain,
	statedb *state.StateDB, header *types.Header,
	gp *GasPool,
	toalUsedGas *big.Int, cfg vm.Config,
	msg *types.Message, tx *types.Transaction, gasRemaining uint64, currAction types.TxAction,
	source types.UnitID) (receipt *types.Receipt, err error) {

	isIdleExec := currAction == types.ActionSCIdleContractDeal

	action := types.ActionContractDeal
	txHash := tx.Hash()
	isCreateNewContanct := sc.IsCallCreateContractTx(msg.To(), msg.Data())
	var refund uint64

	if isIdleExec {
		if msg.Value().Sign() > 0 { //此类交易不涉及冻结，因为初始输入的 Gas 量不需要返回
			return nil, errors.New("invalid scidle contract")
		}
		if msg.GasPrice() != 0 || msg.Gas() != 0 {
			return nil, errors.New("invalid tx gas or gas price when tx is free")
		}
		if header.MC != params.SCAccount {
			return nil, errors.New("invalid  chain address when action is SCIdleContractDeal")
		}
	}

	//如果是部署合约执行的第二阶段，则需要预执行合约部署
	if isCreateNewContanct && header.MC == params.SCAccount {
		//部署将在基于创世状态下创建第一个高度单元
		newdb, err := mc.StateAt(mc.Genesis().MC(), mc.Genesis().Root())
		if err != nil {
			return nil, err
		}
		newdb.SetBalance(msg.From(), big.NewInt(0))
		freeze, overflow := math.SafeMul(gasRemaining, msg.GasPrice())
		if overflow {
			return nil, vm.ErrOutOfGas
		}
		newdb.AddBalance(msg.From(), new(big.Int).Add(new(big.Int).SetUint64(freeze), msg.Value()))

		newHeader := types.Header{
			MC:         common.GenerateContractAddress(header.MC, msg.VMMagic(), statedb.GetNonce(header.MC)),
			Number:     1,
			SCHash:     header.SCHash,
			SCNumber:   header.SCNumber,
			ParentHash: mc.GenesisHash(),
		}

		before := gp.Gas()
		r, err := applyToContract(config, mc, newdb, &newHeader, gp, big.NewInt(0), cfg, msg, tx, gasRemaining, currAction, source)
		if err != nil || r.Failed { //如果失败，需要创建一个基于此链的回执
			if cfg.Debug {
				log.Debug("failed", "tx", tx.Hash(), "failed", r.Failed, "err", err)
			}
			if err == ErrGasLimitReached {
				return nil, err
			}
			var gasUsed uint64
			if r != nil {
				gasUsed = r.GasUsed
			} else {
				gasUsed = math.ForceSafeSub(before, gp.Gas())
				if err := gp.SubGas(gasUsed); err != nil {
					return nil, err
				}
			}
			receipt = createReceipt(config, source, header, statedb, txHash, action, nil, true, gasRemaining, toalUsedGas, gasUsed)
			return receipt, nil
		}
		//如果成功，则继续，但是需要将使用量返还
		refund = r.GasUsed
		//将已使用量，降低
		msg.GasSub(r.GasUsed)
	}

	ctx := NewEVMContext(msg, header, mc, tx)

	// 创建虚拟机执行所需要的上下文信息
	// 创建全新的虚拟机，拥有交易和回调所需的信息
	vmenv, err := vm.New(msg.VMMagic(), ctx, statedb, config, cfg)
	if err != nil {
		return
	}

	// 执行合约交易时有冻结用户交易费+Value ，故在执行合约前，需将此部分费用存入合约链执行
	// SetBalance 因为外部调用合约只能是外部账户，而合约链不应该存在外部账户余额，故直接添加已冻结
	freeze := new(big.Int).Mul(new(big.Int).SetUint64(gasRemaining), new(big.Int).SetUint64(msg.GasPrice()))

	initBalance := new(big.Int).Add(freeze, msg.Value())

	// 必须先设置为0，再 Add，这样才能进入流水中
	statedb.SetBalance(msg.From(), big.NewInt(0)) //设置起始为0
	statedb.AddBalance(msg.From(), initBalance)
	statedb.GetOrNewStateObject(msg.From()).ResetInitBalance(initBalance)

	// 利用虚拟机执行交易，将交易实施到当前状态库中
	output, gas, failed, err := ApplyMessage(vmenv, statedb, action, msg, gp, header.MC)
	if err != nil {
		return
	}
	if cfg.Debug {
		log.Debug("after apply message", "tx", txHash, "action", action, "chain", header.MC, "gas", gas, "failed", failed, "err", err)
	}

	//如果第一步执行失败且是免费交易，则不需要包含此交易到区块
	if failed && source.IsEmpty() && IsFreeTx(tx) {
		return nil, vm.ErrExecutionReverted
	}

	if failed && refund != 0 {
		var ok bool
		//gas.Add(gas, refund) //如果失败则收取所有
		gas, ok = math.SafeAdd(gas, refund)
		if ok {
			err = vm.ErrOutOfGas
		}
	}
	receipt = createReceipt(config, source, header, statedb, txHash, action, output, failed, gasRemaining, toalUsedGas, gas, msg.From()) /**/
	// 退款= 剩余Gas * Price + Value
	statedb.SetBalance(msg.From(), big.NewInt(0))
	//如果是成功部署合约，则在第二阶段时将产生流水
	if !failed && isCreateNewContanct && header.MC == params.SCAccount {
		receipt.Coinflows = receipt.Coinflows[:0]
	}
	// 部署或执行时，需要重新计算交易方的收款额（来自合约的转账）
	for i, v := range receipt.Coinflows {
		if v.To == msg.From() {
			// 初始额 - value - 手续费 + 收款 = 当前余额
			//=> 收款=当前余额-初始额 + value+ 手续费
			//		= Diff  + value +手续费

			more := big.NewInt(0).Set(v.Amount)
			more.Add(more, msg.Value())
			more.Add(more, new(big.Int).Mul(new(big.Int).SetUint64(msg.GasPrice()), new(big.Int).SetUint64(gas)))

			//if isIdleExec { //此类交易不涉及冻结，因为初始输入的 Gas 量不需要返回
			//	more.Add(more, freeze)
			//}
			//log.Error("----1--->>>", "action", action, "failed", receipt.Failed,
			//	"free", freeze, "init", initBalance, "gas", gas, "v", v.Amount, "value", msg.Value(), "price", msg.GasPrice(),
			//	"value", more)

			if more.Sign() == 0 { //则需要移除
				receipt.Coinflows = append(receipt.Coinflows[:i], receipt.Coinflows[i+1:]...)
			} else {
				receipt.Coinflows[i].Amount = more
			}
			break
		}
	}
	return
}

// ApplyToContract 部署或执行合约
func ApplyToContract(config *params.ChainConfig, mc *DagChain, statedb *state.StateDB, header *types.Header,
	gp *GasPool, tx *types.Transaction, toalUsedGas *big.Int, cfg vm.Config, txexec *types.TransactionExec,
	msg *types.Message) (receipt *types.Receipt, err error) {
	if tx.To().IsContract() == false {
		err = errors.New("applyToContract: only apply transaction about contract (create or call)")
		return
	}

	action, txHash := types.ActionContractDeal, tx.Hash()
	if err = CheckTxActionInChain(statedb, header.MC, txHash, action); err != nil {
		return
	}
	SetTxActionStateInChain(statedb, header.MC, txHash, action)

	var gasRemaining uint64
	//某些交易将缺失Source，如心跳
	if txexec != nil {
		if txexec.PreStep.IsEmpty() { //只允许是在系统链上执行 Pow交易或者空转时才允许为空
			if txexec.Tx == nil { //两者不允许同时为空
				return nil, errors.New("previous unit hash is empty")
			}
			gasRemaining = tx.Gas()
		} else {
			//获取上一阶段执行信息
			preExecInfo, err := mc.dag.GetTxReceiptAtUnit(txexec.PreStep.Hash, txexec.TxHash)
			if err != nil {
				return nil, fmt.Errorf("failed to load previous tx exec info. thash=%v,source=%v,%v",
					txexec.TxHash.Hex(), txexec.PreStep.String(), err)
			}
			if preExecInfo.Failed {
				return nil, fmt.Errorf("previous tx action is failed,can't exec %s. tx %s",
					action, txexec.TxHash.Hex())
			}
			gasRemaining = preExecInfo.GasRemaining
		}
	} else {
		gasRemaining = tx.Gas()
	}
	if msg == nil {
		msg, err = tx.AsMessage(types.MakeSigner(*config), gasRemaining)
		if err != nil {
			return nil, err
		}
	}

	return applyToContract(config, mc, statedb, header, gp, toalUsedGas, cfg, msg, tx, gasRemaining, txexec.Action, txexec.PreStep)
}

// subOutput 按共识要求的最大输出保留，截断输出
func subOutput(src []byte) []byte {
	var will int
	if len(src) > params.MaxOutputDataSize {
		will = params.MaxOutputDataSize
	} else {
		will = len(src)
	}
	dist := make([]byte, will)
	copy(dist, src)
	return dist
}

// ApplyReceive 交易成功后，更新接收方余额
func ApplyReceive(config *params.ChainConfig, mc *DagChain, statedb *state.StateDB, header *types.Header, gp *GasPool, tx *types.Transaction, toalUsedGas *big.Int, cfg vm.Config, txexec *types.TransactionExec) (*types.Receipt, error) {
	send, err := tx.Sender()
	//msg, err := tx.AsMessage(types.MakeSigner(*config), 0)
	if err != nil {
		return nil, err
	}

	if sc.IsCallCreateContractTx(tx.To(), tx.Data()) || tx.To().IsContract() || tx.To() != header.MC {
		return nil, errors.New("invalid receive address")
	}
	action := types.ActionTransferReceipt
	//如果是自己转账给自己，则不应该重复，因为在转账的同时已经将资产完成转移
	if tx.To() == send {
		return nil, fmt.Errorf("don't allow exec %s when transfer to yourself", action)
	}
	if err := CheckTxActionInChain(statedb, header.MC, tx.Hash(), action); err != nil {
		return nil, err
	}
	SetTxActionStateInChain(statedb, header.MC, tx.Hash(), action)

	preExecInfo, err := mc.dag.GetTxReceiptAtUnit(txexec.PreStep.Hash, txexec.TxHash)
	if err != nil {
		return nil, err
	}
	if preExecInfo.Failed {
		return nil, fmt.Errorf("previous tx action is failed,can't exec %s. tx %s",
			action, txexec.TxHash.Hex())
	}

	statedb.AddBalance(tx.To(), tx.Amount())
	statedb.SetNonce(tx.To(), statedb.GetNonce(tx.To())+1) // 更新 nonce

	receipt := createReceipt(config, txexec.PreStep, header, statedb, tx.Hash(), action, nil, false, preExecInfo.GasRemaining, toalUsedGas, 0)
	return receipt, nil
}

// ApplyFreeze 执行合约前，由用户见证人全额冻结交易发送者本次交易最大费用。
// 在合约执行完成后，调用ApplyRefund 函数返还给交易发送者
// 如果冻结失败则此交易将不会出现在单元中
func ApplyFreeze(config *params.ChainConfig, mc *DagChain, statedb *state.StateDB, header *types.Header,
	gp *GasPool, tx *types.Transaction, toalUsedGas *big.Int, cfg vm.Config, msg *types.Message) (*types.Receipt, error) {
	var err error
	if msg == nil {
		msg, err = tx.AsMessage(types.MakeSigner(*config), tx.Gas())
		if err != nil {
			return nil, err
		}
	}

	// 防止外部调用错误
	// if msg.To() != nil && !msg.To().IsContract() {
	if !(sc.IsCallCreateContractTx(msg.To(), msg.Data()) || msg.To().IsContract()) {
		return nil, errors.New("freezing funds can only be used for contract transactions")
	}

	action := types.ActionContractFreeze
	if err := CheckTxActionInChain(statedb, header.MC, msg.ID(), action); err != nil {
		return nil, err
	}
	SetTxActionStateInChain(statedb, header.MC, msg.ID(), action)

	// 此逻辑中已经为合约部署或执行，如果是部署合约则需要执行一次预创建，已防止失败的被提交。
	// 如果部署合约失败，则终止
	vmenv, err := vm.New(msg.VMMagic(), NewEVMContext(msg, header, mc, tx), statedb, config, cfg)
	if err != nil {
		return nil, err
	}

	var (
		failed bool //是否失败
	)

	// 执行合约，无法判断是否执行成功，只能是做简单校验
	//if msg.To() != nil {
	//	st := NewStateTransition(vmenv, statedb, msg, gp)
	//	if err := PreApplayMessage(st, true); err != nil {
	//		return nil, err
	//	}
	//
	//} else { //否则，尝试执行VM，进行合约部署
	//	id := statedb.Snapshot()
	//	_, doUsedGas, failed, newContractAddress, err = ApplyMessage(vmenv, statedb, msg, gp)
	//	// 立刻恢复 statedb
	//	statedb.RevertToSnapshot(id)
	//	if err != nil {
	//		return nil, err
	//	}
	//}
	// 不管是部署合约还是执行合约，冻结环节将不进行预执行，直接进行检查即可
	//if msg.IsCreateContract() { // 创建合约
	//	id := statedb.Snapshot()
	//	_, doUsedGas, failed, err = ApplyMessage(vmenv, statedb,action, msg, gp)
	//	// 立刻恢复 statedb
	//	statedb.RevertToSnapshot(id)
	//	if err != nil {
	//		return nil, err
	//	}
	//} else { // 简单执行合约
	st := NewStateTransition(vmenv, statedb, action, msg, gp, header.MC)
	if err := PreApplayMessage(st, true); err != nil {
		//if err == vm.ErrOutOfGas { //如果是Gas不足，则继续
		//	err = nil
		//	failed = true
		//} else {
		//	return nil, err
		//}
		return nil, err
	}
	//}
	sender, doUsedGas := msg.From(), st.gasUsed()
	// VM执行失败时，将不再按冻结处理。而是直接扣除所费Gas，结束交易。
	//statedb.SubBalance(sender, new(big.Int).Mul(doUsedGas, msg.GasPrice())) //扣除交易费
	if failed == false {
		var freeze *big.Int
		//冻结其他部分
		freeze = new(big.Int).SetUint64(msg.Gas())
		freeze.Mul(freeze, new(big.Int).SetUint64(msg.GasPrice()))
		freeze.Add(freeze, msg.Value())

		if !CanTransfer(statedb, sender, freeze) {
			return nil, vm.ErrInsufficientBalance
		}
		statedb.SubBalance(sender, freeze)
	}
	statedb.SetNonce(sender, statedb.GetNonce(sender)+1) // 更新 nonce

	receipt := createReceipt(config, types.UnitID{}, header, statedb, tx.Hash(), action, nil, failed, tx.Gas(), toalUsedGas, doUsedGas)
	return receipt, nil
}

// ApplyRefund 执行合约完成后，将多冻结的部分返回给原交易发送者
func ApplyRefund(config *params.ChainConfig, mc *DagChain,
	statedb *state.StateDB, header *types.Header, gp *GasPool,
	orignTx *types.Transaction, toalUsedGas *big.Int, cfg vm.Config, tx *types.TransactionExec) (*types.Receipt, error) {

	msg, err := orignTx.AsMessage(types.MakeSigner(*config), 0)
	if err != nil {
		return nil, err
	}
	if header.MC != msg.From() {
		return nil, errors.New("invalid receive address")
	}

	action := types.ActionContractRefund
	if err := CheckTxActionInChain(statedb, header.MC, msg.ID(), action); err != nil {
		return nil, err
	}
	SetTxActionStateInChain(statedb, header.MC, msg.ID(), action)

	preExecInfo, err := mc.dag.GetTxReceiptAtUnit(tx.PreStep.Hash, tx.TxHash)
	if err != nil {
		return nil, err
	}

	// 退款= 剩余Gas * Price + Value
	refund := math.Mul(preExecInfo.GasRemaining, msg.GasPrice())
	// 当交易执行失败，将只扣除已用Gas并解除冻结
	if preExecInfo.Failed {
		// 退款
		refund.Add(refund, msg.Value())
	}

	from := msg.From()
	statedb.SetNonce(from, statedb.GetNonce(from)+1) // 更新 nonce
	statedb.AddBalance(from, refund)
	receipt := createReceipt(config, tx.PreStep, header, statedb, tx.TxHash, action, nil, false,
		preExecInfo.GasRemaining, toalUsedGas, 0)
	return receipt, nil
}

// ActionContractReceipt 执行合约完成后，更新对应收款方余额
func ActionContractReceipt(config *params.ChainConfig, mc *DagChain,
	statedb *state.StateDB, header *types.Header, gp *GasPool,
	orignTx *types.Transaction, toalUsedGas *big.Int, cfg vm.Config, tx *types.TransactionExec) (*types.Receipt, error) {

	action := types.ActionContractReceipt
	preExecInfo, err := mc.dag.GetTxReceiptAtUnit(tx.PreStep.Hash, tx.TxHash)
	if err != nil {
		return nil, err
	}
	if preExecInfo.Failed {
		return nil, fmt.Errorf("previous tx action is failed,can't exec %s. tx %s", action, tx.TxHash.Hex())
	}

	// TODO(ysqi) BUG: 如果交易失败情况下，可能无法正常解冻资产
	if len(preExecInfo.Coinflows) == 0 {
		return nil, fmt.Errorf("the result of coninflows in previous tx action is empty. tx %s", tx.TxHash.Hex())
	}
	if err := CheckTxActionInChain(statedb, header.MC, tx.TxHash, action); err != nil {
		return nil, err
	}
	SetTxActionStateInChain(statedb, header.MC, tx.TxHash, action)

	// 统计发生的记账流水，并更新到自己账户链上去
	addBalance := new(big.Int)
	for _, v := range preExecInfo.Coinflows {
		if v.To == header.MC {
			addBalance.Add(addBalance, v.Amount)
		}
	}
	if addBalance.Sign() <= 0 {
		return nil, fmt.Errorf("the result of coninflows in previous tx action does not include given address %s. tx %s",
			header.MC.Hex(), tx.TxHash.Hex())
	}
	statedb.SetNonce(header.MC, statedb.GetNonce(header.MC)+1) // 更新 nonce
	statedb.AddBalance(header.MC, addBalance)
	receipt := createReceipt(config, tx.PreStep, header, statedb, tx.TxHash, action, nil, false, preExecInfo.GasRemaining, toalUsedGas, 0)
	return receipt, nil
}
