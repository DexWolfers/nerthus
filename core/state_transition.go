package core

import (
	"errors"
	"fmt"
	"math/big"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"
)

var (
	Big0 = big.NewInt(0)
)

type ErrInsufficientBalanceForGas struct {
	balance *big.Int
	paygas  uint64
}

func (err ErrInsufficientBalanceForGas) Error() string {
	return fmt.Sprintf("insufficient balance %d to pay for gas %d", err.balance, err.paygas)
}

func NewErrInsufficientBalanceForGas(balance *big.Int, paygas uint64) error {
	return ErrInsufficientBalanceForGas{
		balance: balance,
		paygas:  paygas,
	}
}

/*
状态转变模型

状态转变是指当一笔交易被应用到当前状态机时的一种变更模型，该模型将处理必要工作，以导出新状态根。

当一笔交易被执行后，将会实时反应到 状态机中，所有交易是执行所必须的工作已找到新的状态根。

具体过程：
	1)  Nonce校验（交易时间有序）
	2)  预付燃油费
	4） 支付交易数据费用（存储费）
	5） 执行VM，VM内部优先进行转账（如果value!=0）
		5a) 接收方不存在，则创建合约，结果将作为合约代码处理。
		5b) 否则，执行合约
    6） 退还剩余的gas和偿还的gas
	7)  返回结果
*/
type StateTransition struct {
	gp         *GasPool
	msg        Message
	action     types.TxAction
	gas        uint64 //退还量
	gasPrice   uint64
	initialGas uint64 // 初始量
	freeGas    uint64 //免费Gas量
	value      *big.Int
	data       []byte
	state      vm.StateDB
	vm         vm.CallContext
	mc         common.Address
}

// Message 代表发送到合约的消息。
type Message interface {
	Time() uint64
	Nonce() uint64

	From() common.Address
	To() common.Address

	GasPrice() uint64
	Gas() uint64
	Value() *big.Int

	Data() []byte
	Seed() uint64
	CheckNonce() bool
	ID() common.Hash
	//IsCreateContract() bool
}

// IntrinsicGas 根据给定数据计算出可确定需支付的燃油费
func IntrinsicGas(data []byte, contractCreation bool) uint64 {
	var igas uint64
	if contractCreation {
		igas = params.TxGasContractCreation
	} else {
		igas = params.TxGas
	}
	return igas + getGasByData(data)
}

func getGasByData(data []byte) uint64 {
	var igas uint64
	// 如果有数据，则需要为数据支付存储费。
	// 存储费分两部分收取：
	//    数据为0部分： 按 TxDataZeroGas 收取
	//    不为0部分：   按 TxDataNonZeroGas 收取
	// 举例：Data = [50,50,0,100,4,0], Gas为：4*TxDataNonZeroGas + 2 * TxDataZeroGas
	if len(data) > 0 {
		var nz uint64
		for _, byt := range data {
			if byt != 0 {
				nz++
			}
		}
		igas += nz * params.TxDataNonZeroGas
		igas += (uint64(len(data)) - nz) * params.TxDataZeroGas
	}
	return igas
}

// NewStateTransition 初始化并返回一个全新的状态交易对象
func NewStateTransition(vm vm.CallContext, state vm.StateDB, action types.TxAction, msg Message, gp *GasPool, mc common.Address) *StateTransition {

	var free uint64
	if vm != nil && vm.Ctx().UnitMC == params.SCAccount { //如果是系统链上的交易则检查是否有免费部分
		//获取可免费的Gas量
		free = vm.Ctx().FreeGas(msg.To(), vm.Ctx().UnitNumber.Uint64(), msg.Data())
	}
	return &StateTransition{
		gp:         gp,
		vm:         vm,
		msg:        msg,
		gasPrice:   msg.GasPrice(),
		initialGas: 0,
		freeGas:    free,
		value:      msg.Value(),
		data:       msg.Data(),
		action:     action,
		state:      state,
		mc:         mc,
	}
}

// ApplyMessage 通过给定消息，依据旧状态计算出新状态。
//
// ApplyMessage 依次返回执行虚拟机的返回结果（如果存在）、所使用的gas量（含退还部分）、交易是否失败以及错误信息（如果失败）。
// 如果有返回错误，那么错误信息必然是核心的错误，意味着该交易永远不可能被接受，被放入单元中。
//
// 需要注意：交易没有完全执行成功不会引发核心错误，即使没有执行完整，也需要为其已执行部分支付 gas。不如：执行时发现余额不足或可用gas不足。
// 此类情况，属于交易失败，而不属于核心错误。此交易会被接受并存入单元中。
func ApplyMessage(vm vm.CallContext, state vm.StateDB, action types.TxAction, msg Message, gp *GasPool, mc common.Address) (output []byte, gasUsed uint64, failed bool, err error) {
	st := NewStateTransition(vm, state, action, msg, gp, mc)

	output, _, gasUsed, failed, err = st.TransitionDb()
	return
}

// PreApplayMessage 交易消息EVM执行前处理（检查消息正确性和扣除交易本身所需燃料费）
// 如果检查不通过，则返回错误信息（如：余额不足已扣除燃料费等）
func PreApplayMessage(st *StateTransition, justCheck bool) error {
	if justCheck {
		//如果仅仅是检查交易是否可行，则在检查完毕后恢复state。但不处理st内容
		snapshot := st.state.Snapshot()
		defer st.state.RevertToSnapshot(snapshot)
	}

	if err := st.preCheck(); err != nil {
		return err
	}
	// 基本费用只在第一个执行阶段收取，其他阶段不在消耗
	switch st.action {
	case types.ActionSCIdleContractDeal:
		// 支付 交易本身的 gas
		intrinsicGas := IntrinsicGas(st.data, false)
		// 扣除
		if err := st.useGas(intrinsicGas); err != nil {
			return err
		}
	case types.ActionContractFreeze, types.ActionTransferPayment:
		// 接收为空，则属于创建合约
		// contractCreation := st.msg.To() == nil
		contractCreation := sc.IsCallCreateContractTx(st.msg.To(), st.msg.Data())

		// 支付 交易本身的 gas
		intrinsicGas := IntrinsicGas(st.data, contractCreation)
		// 扣除
		if err := st.useGas(intrinsicGas); err != nil {
			return err
		}
	}

	return nil
}

func (st *StateTransition) from() vm.AccountRef {
	f := st.msg.From()
	if !st.state.Exist(f) {
		st.state.CreateAccount(f)
	}
	return vm.AccountRef(f)
}

func (st *StateTransition) to() vm.AccountRef {
	if st.msg == nil {
		return vm.AccountRef{}
	}
	to := st.msg.To()
	//if to == nil {
	//	return vm.AccountRef{} // contract creation
	//}

	reference := vm.AccountRef(to)
	if !st.state.Exist(to) {
		st.state.CreateAccount(to)
	}
	return reference
}

func (st *StateTransition) useGas(amount uint64) error {
	if st.gas < amount {
		return vm.ErrOutOfGas
	}
	st.gas -= amount

	return nil
}

func (st *StateTransition) buyGas() error {
	// gas 限制不能超过 max(int64)
	mgas := st.msg.Gas()
	// 总费用
	mgval, overflow := math.SafeMul(mgas, st.gasPrice)
	if overflow {
		return errors.New("gas*price is overflow uint64")
	}

	if st.freeGas > 0 {
		//免费部分不收费
		gas := st.freeGas * st.gasPrice
		if mgval < gas {
			mgval = 0
		} else {
			mgval -= gas
		}
		mgas += st.freeGas
	}

	var (
		state  = st.state
		sender = st.from()
	)

	// 检查是否足额
	if balance := state.GetBalance(sender.Address()); balance.Cmp(new(big.Int).SetUint64(mgval)) < 0 {
		return NewErrInsufficientBalanceForGas(balance, mgval)
	}
	// 从总Gas中扣除，不能超出
	if err := st.gp.SubGas(mgas); err != nil {
		return err
	}
	// 全部作为剩余量
	st.gas += mgas

	// 标记为初始可用gas
	st.initialGas = mgas
	// 扣除费用
	state.SubBalance(sender.Address(), new(big.Int).SetUint64(mgval))
	return nil
}

// preCheck 预先检查
func (st *StateTransition) preCheck() error {
	// TODO(ysqi) 去除nonce检查后，需另外处理不允许交易重复
	//msg := st.msg
	//sender := st.from()
	//
	//// 确保交易时间是有序的
	//if msg.CheckNonce() {
	//	if n := st.state.GetNonce(sender.Address()); n > msg.Nonce() {
	//		return fmt.Errorf("invalid nonce: have %d, expected more than %d", msg.Nonce(), n)
	//	}
	//}
	// 如果交易已存在，则认为是双花，err 为空时表示 id 存在
	//if _, err := st.state.VerifyProof(st.vm.Ctx().UnitMC, st.msg.ID()); err == nil {
	//	return ErrDoubleSpendTx
	//}
	//TODO(ysqi): 下一步优化，需要将处理逻辑其迁移
	//if err := CheckTxActionInChain(st.state,st.vm.Ctx().UnitMC,st.msg.ID(),st.action);err!=nil {
	//	return err
	//}
	// 购买gas并存入可用量中： gaslimit*gasprice
	return st.buyGas()
}

// 交易ID（哈希）存储在State中的标记 value
var (
	txIDStateValue  = common.BytesToHash([]byte{1})
	txIDStatePrefix = []byte("ts_")
)

// CheckTXActionInChain 校验交易操作是否在给定链上存在
func CheckTxActionInChain(state vm.StateDB, mc common.Address, txID common.Hash, action types.TxAction) error {
	if !state.GetState(mc, GetTxActionStoreKey(txID, action)).Empty() {
		//log.Debug("invalid tx action", "mc", mc, "thash", txID, "action", action, "err", "the transaction action already exists in chain")
		return errors.New("repeat transaction in chain")
	}
	return nil
}

// 获取交易信息存储Key
func GetTxActionStoreKey(txID common.Hash, action types.TxAction) common.Hash {
	return common.SafeHash256(txIDStatePrefix, txID, uint64(action))
}

// SetTxActionStateInChain 到链中标记Tx Action
func SetTxActionStateInChain(state vm.StateDB, mc common.Address, txID common.Hash, action types.TxAction) {
	state.SetState(mc, GetTxActionStoreKey(txID, action), txIDStateValue)
}

// TransitionDb 将依据当前消息执行状态转换，并返回执行结果、操心所必须的gas、已使用的 gas 、成功与否。
// 如果返回一个错误信息，这将是一个共识问题，表示该交易不被共识认可。
func (st *StateTransition) TransitionDb() (ret []byte, requiredGas, usedGas uint64, failed bool, err error) {
	if err = PreApplayMessage(st, false); err != nil {
		return
	}
	sender := st.from()

	// 接收为空，则属于创建合约
	// contractCreation := msg.To() == nil
	contractCreation := sc.IsCallCreateContractTx(st.msg.To(), st.msg.Data())

	// 除去余额不足，虚拟机执行错误并不影响一致性
	var vmerr error

	// 执行前标记当前Action在指定链上执行
	SetTxActionStateInChain(st.state, st.mc, st.msg.ID(), st.action)

	//交易中的 Nonce 无用途，只需要根据链进行交易数累计
	// 这里需要屏蔽部署合约时在合约链上的第一个 create 操作，因为涉及nonce检查
	if !(contractCreation && st.mc != params.SCAccount && st.vm.Ctx().UnitNumber.Uint64() == 1) {
		st.state.SetNonce(st.mc, st.state.GetNonce(st.mc)+1)
	}

	if contractCreation {
		if st.vm.Ctx().UnitMC == params.SCAccount { // 如果是系统链则先申请新的见证人
			st.value = big.NewInt(0)
			// 执行合约
			ret, st.gas, vmerr = st.vm.Call(sender, st.to().Address(), st.data, st.gas, st.value)
		} else {
			ret, _, st.gas, vmerr = st.vm.Create(sender, st.data[4:], st.gas, st.value)
		}

	} else if st.msg.To().IsContract() {
		// 执行合约
		ret, st.gas, vmerr = st.vm.Call(sender, st.to().Address(), st.data, st.gas, st.value)
	} else {
		//非合约时为，普通转账
		if !CanTransfer(st.state, sender.Address(), st.msg.Value()) {
			return nil, 0, 0, true, vm.ErrInsufficientBalance
		}
		Transfer(st.state, sender.Address(), st.to().Address(), st.value)
	}
	if vmerr == nil && len(ret) > 0 {
		// 计算output gas使用量，如果Gas被消耗，则作为错误处理
		// 但不能直接返回错误信息，因为这样导致交易处理无法进入单元而被丢弃，Bug#1157242477001000082
		more := getGasByData(ret)
		if err := st.useGas(more); err != nil {
			vmerr = err
		}
	}
	if vmerr != nil {
		logger := log.Trace
		if st.vm != nil && st.vm.VMConfig().Debug {
			logger = log.Debug
		}

		logger("VM returned with error",
			"chain", st.mc, "number", st.vm.Ctx().UnitNumber,
			"txhash", st.msg.ID(), "sender", sender.Address(), "time", st.msg.Time(), "err", vmerr)
		if st.vm != nil && st.vm.VMConfig().EnableSimulation {
			err = vmerr
			return
		}

		// 如果是余额不足导致交易无法执行，则作为共识错误返回。
		if vmerr == vm.ErrInsufficientBalance {
			return nil, 0, 0, true, vmerr
		}
	}
	// 已使用量，作为所必须的。
	requiredGas = st.gasUsed()
	//模拟计算时不考虑
	if st.vm == nil || !st.vm.VMConfig().EnableSimulation {
		// 处理退还
		st.refundGas()
	}
	return ret, requiredGas, st.gasUsed(), vmerr != nil, nil
}

func (st *StateTransition) refundGas() {
	// 将剩余的gas 以原价返还给交易者
	sender := st.from()

	value := st.calcRefundValue()
	if value.Sign() > 0 {
		st.state.AddBalance(sender.Address(), value)
	}
	// 并退还到总量池中，方便下一笔交易。
	st.gp.AddGas(new(big.Int).SetUint64(st.gas))
}

func (st *StateTransition) calcRefundValue() *big.Int {
	// 补偿 = min( 已用量/2 , 补偿 )
	// 在合约中是鼓励删除存储数据的，如果有remove storage 则会补偿gas.
	// uhalf := remaining.Div(st.gasUsed(), common.Big2)
	//预估不处理奖励
	if st.vm != nil && !st.vm.VMConfig().EnableSimulation {
		uhalf := st.gasUsed() / 2
		if uhalf > st.state.GetRefund() {
			uhalf = st.state.GetRefund()
		}
		// 总退还量 =  剩余 + 补偿
		st.gas += uhalf
	}
	userPayGas := st.initialGas - st.freeGas
	//说明使用的还是免费部分，付费部分全部返还
	if used := st.gasUsed(); used <= st.freeGas {
		return math.Mul(userPayGas, st.gasPrice)
	} else { //只对超过部分计算
		return math.Mul(userPayGas-(used-st.freeGas), st.gasPrice)
	}
}

// gasUsed 返回已消耗量= 初始量 - 剩余量
func (st *StateTransition) gasUsed() uint64 {
	return st.initialGas - st.gas
}
