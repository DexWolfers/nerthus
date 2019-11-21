package vm

import (
	"math/big"
	"sync"

	"fmt"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/params"
)

type ChainContext interface {
	// GetHeader 根据账户链账户和单元编号获取头信息
	GetHeader(hash common.Hash) *types.Header

	// GetBalance 获取余额
	GetBalance(address common.Address) *big.Int

	// GetWitnessReport 获取见证人周期工作报表数据
	GetWitnessWorkReport(period uint64) map[common.Address]types.WitnessPeriodWorkInfo
	GetWitnessReport(witness common.Address, period uint64) types.WitnessPeriodWorkInfo

	GetChainTailHash(chain common.Address) common.Hash

	GenesisHash() common.Hash

	GetHeaderByHash(hash common.Hash) *types.Header

	GetUnitState(hash common.Hash) (*state.StateDB, error)
	GetStateByNumber(chain common.Address, number uint64) (*state.StateDB, error)
}

// Context provides the EVM with auxiliary information. Once provided
// it shouldn't be modified.
type Context struct {
	Chain ChainContext
	// CanTransfer returns whether the account contains
	// sufficient ether to transfer the value
	CanTransfer CanTransferFunc
	// Transfer transfers ether from one account to the other
	Transfer TransferFunc
	// GetHash returns the hash corresponding to n
	GetHash GetHashFunc
	// FreeGas 获取可免费试用的Gas量
	FreeGas FreeGasFunc

	// MessageIsToInnerAccount 检查消息是否是发送至内部账户
	IsToInnerAccount func(to common.Address) bool
	// MessageIsToInnerAccount 处理内部合约
	HandleInnerMessage func(vm CallContext, contract Contract, input []byte, value *big.Int) (ret []byte, err error)

	// Message information
	Origin   common.Address // Provides information for ORIGIN
	GasPrice uint64         // Provides information for GASPRICE

	// Block information
	Coinbase    common.Address // Provides information for COINBASE
	GasLimit    uint64         // Provides information for GASLIMIT
	UnitParent  common.Hash    //父单元
	UnitMC      common.Address // 账户链
	UnitSCHash  common.Hash
	SCNumber    uint64   //系统链高度
	UnitNumber  *big.Int // Provides information for NUMBER
	UnitVersion string
	Time        *big.Int // Provides information for TIME
	Tx          *types.Transaction
}

type Tracer interface{}

// Config are the configuration options for the Interpreter
type Config struct {
	// Debug enabled debugging Interpreter options
	Debug bool
	// Tracer is the op code logger
	Tracer Tracer
	// NoRecursion disabled Interpreter call, callcode,
	// delegate call and create.
	NoRecursion bool
	// Enable recording of SHA3/keccak preimages
	EnablePreimageRecording bool

	// Type of the EWASM interpreter
	EWASMInterpreter string
	// Type of the EVM interpreter
	EVMInterpreter string

	// 启用执行模拟，模拟过程中将不返还Gas
	EnableSimulation bool
}

type VM interface {
	// CodeFlag 代码标识符
	// 不允许为0，max(uint16)=65535
	Magic() uint16
	New(ctx Context, statedb StateDB, chainConfig *params.ChainConfig, vmConfig Config) (CallContext, error)
}

var (
	vms      = make(map[string]VM, 3)
	vmMagics = make(map[uint16]string, 3)

	vmMutex sync.RWMutex
)

// Register 重复注册将引发异常
func Register(name string, vm VM) {
	vmMutex.Lock()
	defer vmMutex.Unlock()

	if vm == nil {
		panic("Register vm is nil")
	}
	if _, dup := vms[name]; dup {
		panic("Register called twice for vms" + name)
	}
	magic := vm.Magic()
	if magic == 0 {
		panic(fmt.Sprintf("Register magic %d must be greater than 0", magic))
	}
	if _, dup := vmMagics[magic]; dup {
		panic(fmt.Sprintf("Register called twice for magic %d", magic))
	}
	vms[name] = vm
	vmMagics[magic] = name
}

// GetVMCreater 通过name或者magic获取对应VM，如果未找到则返回空
func GetVMCreater(nameOrMagic interface{}) (VM, error) {
	vmMutex.Lock()
	var (
		creater VM
		ok      bool
	)

	switch v := nameOrMagic.(type) {
	case string:
		creater, ok = vms[v]
	case int: //防止字面量
		if v <= 1<<16-1 { // 如果能转换为 uint16
			creater, ok = vms[vmMagics[uint16(v)]]
		}
	case uint16:
		creater, ok = vms[vmMagics[v]]
	}

	vmMutex.Unlock()

	if !ok {
		return nil, fmt.Errorf("unknown vm 0x%x (forgotten import?)", nameOrMagic)
	}
	return creater, nil
}

func New(nameOrMagic interface{}, ctx Context, statedb StateDB, chainConfig *params.ChainConfig, vmConfig Config) (CallContext, error) {
	creater, err := GetVMCreater(nameOrMagic)
	if err != nil {
		return nil, err
	}
	return creater.New(ctx, statedb, chainConfig, vmConfig)
}

func Background(ctx Context, chainConfig *params.ChainConfig) CallContext {
	return &emptyCallCtx{
		ctx: ctx,
		cfg: chainConfig,
	}
}

type emptyCallCtx struct {
	ctx Context
	cfg *params.ChainConfig
}

func (c emptyCallCtx) Ctx() Context {
	return c.ctx
}

func (c emptyCallCtx) ChainConfig() *params.ChainConfig {
	return c.cfg
}
func (c emptyCallCtx) VMConfig() Config {
	return Config{}
}
func (c emptyCallCtx) State() StateDB {
	panic("call not allowed")
}
func (c emptyCallCtx) Cancel() {
	panic("call not allowed")
}

func (c emptyCallCtx) Create(caller ContractRef, code []byte, gas uint64, value *big.Int) (ret []byte, contractAddr common.Address, leftOverGas uint64, err error) {
	panic("call not allowed")
}

func (c emptyCallCtx) Call(caller ContractRef, addr common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error) {
	panic("call not allowed")
}

func (c emptyCallCtx) CallCode(caller ContractRef, addr common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, leftOverGas uint64, err error) {
	panic("call not allowed")
}

func (c emptyCallCtx) DelegateCall(caller ContractRef, addr common.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error) {
	panic("call not allowed")
}

func (c emptyCallCtx) StaticCall(caller ContractRef, addr common.Address, input []byte, gas uint64) (ret []byte, leftOverGas uint64, err error) {
	panic("call not allowed")
}
