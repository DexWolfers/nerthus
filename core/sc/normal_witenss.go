package sc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/core/config"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/rlp"
)

const Version = "1.0"

const (
	withdrawJD uint64 = 2 //提现的最小周期
)

var (
	defChainInfoPrefix        = []byte("chaininfo")
	defChainSpecialUnitPrefix = []byte("_s_")
	witnessPrefix             = []byte("cw_")
	allWitenssKey             = common.StringToHash("cw_lib_all") //记录全网所有见证人
	chainCreaterPrefix        = []byte("cer_")                    // 链的创建者（当前存储的合约链的创建者）
)

var (
	ErrNoWintessInfo             = errors.New("no witness information")
	ErrChainMissingWitness       = errors.New("chain missing witness")
	ErrMaxWitnessLimit           = errors.New("witness upper limit")
	ErrNotFindWitness            = errors.New("the target not find witness")
	ErrNotWitness                = errors.New("the target is not witness")
	ErrNotCouncil                = errors.New("the target is not council")
	ErrNotExceededCouncilCeiling = errors.New("the council ceiling can't be exceeded")
)

/**
见证人数据存储格式：
//-+--------+--------------+-------------+------------+-----------------------------+-------------+
// | prefix +  key-p1  	   | key-p2 	 |  key-p3    |        Value     			|  remark     |
//-+--------+--------------+-------------+------------+-----------------------------+-------------+
// | "cw_lib_actives_count"|             |            | Active Witness count		| 有效见证人数    |
//----------+--------------+-------------+------------+-----------------------------+-------------+
// | "cw_lib_all_count"    |             |            | All Witness count		    | 历史累计见证人数    |
//----------+--------------+-------------+------------+-----------------------------+-------------+
// | "cw_"  |  address     |                          |   index		                | 见证人信息存储位置 |
//----------+--------------+-------------+------------+-----------------------------+-------------+
// | "cw_lib_s_" | index   |                          |   address                   |  见证人地址 |
//----------+--------------+-------------+------------+-----------------------------+-------------+
// |"cw_lib_" |  index     |                          |   Witness info		        | 见证人信息|
//----------+--------------+-------------+------------+-----------------------------+-------------+
// | "cw_lib_actives"      |                          |   []index		            | 有效见证人Index列表|
//----------+--------------+-------------+------------+-----------------------------+-------------+
// | "cw_mcw_"             |  mc        |             |   gpIndex		            | 链的见证人组|
//----------+--------------+-------------+------------+-----------------------------+-------------+
// | "cw_mcw_sc_"          |  mc     |                |   sc number		            | 链申请见证人的系统链高度|
//----------+--------------+-------------+------------+-----------------------------+-------------+

见证人组信息：
//-+--------+--------------+-------------+------------+-----------------------------+-------------+
// | prefix +  key-p1  	   | key-p2 	 |  key-p3    |        Value     			|  remark     |
//-+--------+--------------+-------------+------------+-----------------------------+-------------+
// | "wgp_count"|          |                          | uint64 		               |    当前见证组数         |
//-+--------+--------------+-------------+------------+-----------------------------+-------------+
// | "wgp_ws_"|         |   gpIndex                   |  uint64                    |   给定组下的见证人数量  |
//-+--------+--------------+-------------+------------+-----------------------------+-------------+
// | "wgp_ws_"|         |   gpIndex        "_"+index  |  address                    |  见证人地址  |
//-+--------+--------------+-------------+------------+-----------------------------+-------------+
// | "wgp_cs_"|         |   gpIndex                | uint64                      |   给定组下的链数量  |
//-+--------+--------------+-------------+------------+-----------------------------+-------------+
// | "wgp_cs_"|         |   gpIndex      | "_"+index      | address                      |   链地址  |
//-+--------+--------------+-------------+------------+-----------------------------+-------------+


// 合约信息
//-+--------+--------------+-------------+------------+-----------------------------+-------------+
// | prefix |  key-p1  	   | key-p2 	 |  key-p3    |        Value     			|  remark     |
//-+--------+--------------+-------------+------------+-----------------------------+-------------+
// | "cer_"|  address            |                |  creater address		    |合约创建者地址  |
//----------+--------------+-------------+------------+-----------------------------+-------------+

*/

// ChainStatus 链状态
//go:generate enumer -type=ChainStatus -json -transform=snake -trimprefix=ChainStatus
type ChainStatus byte

const (
	_                                 ChainStatus = iota // 默认
	ChainStatusNormal                                    // 正常
	ChainStatusWitnessReplaceUnderway                    // 更换见证人中
	ChainStatusNotWitness                                // 无见证人
	ChainStatusInsufficient                              // 见证人不足
)

// WitnessStatus 见证人状态
//go:generate enumer -type=WitnessStatus -json -transform=sname -trimprefix=Witness
type WitnessStatus byte

const (
	WitnessNormal      WitnessStatus = iota + 1 //正常
	WitnessLoggedOut                            //已注销
	WitnessInBlackList                          //黑名单
)

// WitnessInfo 见证人详情
type WitnessInfo struct {
	Index        uint64         //见证人索引
	Address      common.Address //见证人地址
	Margin       uint64         //提交的保证金
	ApplyHeight  uint64         // 加入时的系统链高度
	CancelHeight uint64         // 注销时的系统高度
	Status       WitnessStatus  //状态
	GroupIndex   uint64         // 见证组索引
	Pubkey       []byte         // 公钥
}

// applyWitness 见证人申请
// db:stateDB from:用户地址 value:申请金额 unitNumber:申请高度 publicKey:公钥 isSys:是否是系统见证人
func applyWitness(db StateDB, from common.Address, value, unitNumber uint64, publicKey []byte, isSys bool) ([]byte, error) {
	//log.Debug("apply witness", "isSys", isSys, "address", from, "pubkey", common.Bytes2Hex(publicKey))
	if len(publicKey) == 0 {
		return nil, errors.New("public key can't be empty")
	}
	// 不允许见证人/理事成员申请
	if err := CheckMemberType(db, from); err != nil {
		return nil, err
	}
	// 保存见证人信息
	// 1.保存关系
	// 绑定用户组
	var (
		groupIndex uint64
		err        error
	)
	if isSys {
		groupIndex, err = ApplyWitnessGroupOfSys(db, from)
	} else {
		groupIndex, err = ApplyWitnessGroupOfUser(db, from)
	}
	//log.Debug("apply witness group", "address", from, "index", groupIndex, "isSys", isSys, "err", err)
	if err != nil {
		return nil, err
	}
	info := &WitnessInfo{
		Status:      WitnessNormal,
		Margin:      value,
		ApplyHeight: unitNumber,
		Address:     from,
		GroupIndex:  groupIndex,
		Pubkey:      publicKey,
	}
	err = WriteWitnessInfo(db, info)
	if err != nil {
		return nil, err
	}
	return nil, UpdateAcitvieWitenssLib(db, info)
}
func CancelWitness(evm vm.CallContext, contract vm.Contract) (ret []byte, err error) {
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return nil, vm.ErrOutOfGas
	}
	return cancelWitness(evm, contract, contract.Caller())
}

func removeWitness(evm vm.CallContext, witness common.Address, isBad, isTimeout bool) error {
	info, err := GetWitnessInfo(evm.State(), witness)
	if err != nil {
		return err
	}
	// 只允许自行注销(保险)
	if info.Address != witness {
		return fmt.Errorf("only allow yourself to loggout")
	}
	// 非正常状态，不允许注销
	if info.Status != WitnessNormal {
		return fmt.Errorf("the witness status is abnormal and cannot be cancelled")
	}
	info.Status = WitnessLoggedOut
	info.CancelHeight = evm.Ctx().UnitNumber.Uint64()

	// 将用户保证金从合约中转出
	if info.Margin > 0 {
		refund := info.Margin
		if isTimeout { //需要扣除一部分保证金
			refund = calcPercentMoney(info.Margin, params.SystemWitnessTimeoutPunish)
		}
		evm.Ctx().Transfer(evm.State(), params.SCAccount, witness, big.NewInt(0).SetUint64(refund))
	}
	return onWitnessExit(evm.State(), evm.Ctx().UnitNumber.Uint64(), info)
}

// 计算给定金额的剩余比例= m *(1-percent)
func calcPercentMoney(m uint64, percent float64) uint64 {
	if percent > 1.0 {
		panic("should be <=1.0")
	}
	if percent == 1.0 {
		return 0
	}
	// if percent=0.12
	//  then : m *(1-0.12)*10000 /10000
	p := int64((1 - percent) * 10000)
	v := new(big.Int).Mul(new(big.Int).SetUint64(m), new(big.Int).SetInt64(p))
	return v.Quo(v, big.NewInt(10000)).Uint64()
}

// CancelWitness 见证人注销
func cancelWitness(evm vm.CallContext, contract vm.Contract, from common.Address) (ret []byte, err error) {
	statedb := evm.State()
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return nil, vm.ErrOutOfGas
	}
	info, err := GetWitnessInfo(statedb, from)
	if err != nil {
		return
	}
	// 只允许自行注销(保险)
	if info.Address != from {
		return nil, fmt.Errorf("only allow yourself to loggout")
	}
	// 非正常状态，不允许注销
	if info.Status != WitnessNormal {
		return nil, fmt.Errorf("the witness status is abnormal and cannot be cancelled")
	}
	info.Status = WitnessLoggedOut
	info.CancelHeight = evm.Ctx().UnitNumber.Uint64()

	// 将用户保证金从合约中转出
	if info.Margin > 0 {
		evm.Ctx().Transfer(statedb, params.SCAccount, from, big.NewInt(0).SetUint64(info.Margin))
	}

	// 更新见证人信息
	if !contract.UseGas(params.SstoreSetGas * 10) {
		return nil, vm.ErrOutOfGas
	}
	if err := onWitnessExit(statedb, evm.Ctx().UnitNumber.Uint64(), info); err != nil {
		return nil, err
	}
	return ret, nil
}

// 见证人退出（注销/拉黑）时更新数据
func onWitnessExit(db StateDB, sysNumber uint64, witness *WitnessInfo) error {
	if witness.Status == WitnessNormal {
		panic("invalid status")
	}
	witness.CancelHeight = sysNumber

	DelWitnessInfo(db, witness.Address, witness.CancelHeight, witness.Status)
	if err := CancelWitnessFromWitnessGroup(db, witness.Address, witness.GroupIndex); err != nil {
		return err
	}
	return UpdateAcitvieWitenssLib(db, witness)
}

// ReplaceWitnessExecuteInput 更换见证人投票信息
type ReplaceWitnessExecuteInput struct {
	Chain          common.Address
	ApplyNumber    uint64              // 申请更换见证人的系统链高度
	ApplyUHash     common.Hash         //对应单元信息
	Choose         types.UnitID        //见证人的选择
	UnitVoterSigns []types.WitenssVote //单元的投票集合
	ChooseVoteSign [][]byte            //签名
}

// MakeReplaceWitnessExecuteInput 生成更换见证人投票信息
func MakeReplaceWitnessExecuteInput(info []*ReplaceWitnessExecuteInput) ([]byte, error) {
	b, err := rlp.EncodeToBytes(info)
	if err != nil {
		return nil, err
	}
	return CreateCallInputData(FuncReplaceWitnessExecuteID, b), nil
}

// UnpackReplaceWitnessExecuteInput 解析更换见证人投票信息
func UnpackReplaceWitnessExecuteInput(input []byte) (out []*ReplaceWitnessExecuteInput, err error) {
	if len(input) < 4 {
		return out, errors.New("invalid input data")
	}
	var b []byte
	if err = abiObj.Methods["ReplaceWitnessExecute"].Inputs.Unpack(&b, input[4:]); err != nil {
		return
	}
	err = rlp.DecodeBytes(b, &out)
	return
}

// ReplaceWitnessAssignMcNumber 更换见证人指定用户链
func ReplaceWitnessExecute(evm vm.CallContext, contract vm.Contract, input []byte) error {
	if !contract.UseGas(params.SloadGas + params.SstoreSetGas) {
		return vm.ErrOutOfGas
	}
	data, err := UnpackReplaceWitnessExecuteInput(input)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return errors.New("empty input")
	}
	// 交易必须是单元提案者创建
	if contract.Caller() != evm.Ctx().Coinbase {
		return errors.New("transaction only exec by unit caller")
	}

	for _, one := range data {
		err := handleReplaceWitnessExec(evm, contract, one)
		if err != nil {
			return err
		}
	}
	return nil
}
func handleReplaceWitnessExec(evm vm.CallContext, contract vm.Contract, info *ReplaceWitnessExecuteInput) error {

	if !contract.UseGas(params.SloadGas + params.SstoreLoadGas) {
		return vm.ErrOutOfGas
	}
	log.Trace("handleReplaceWitnessExec", "chain", info.Chain, "choose", info.Choose, "txhash", evm.Ctx().Tx.Hash())

	stateDB := evm.State()
	if info.Choose.Height == 0 { //创世单元
		if info.Choose.Hash != evm.Ctx().Chain.GenesisHash() {
			return errors.New("invalid choose")
		}
	} else if len(info.UnitVoterSigns) < params.GetChainMinVoting(info.Chain) { //投票数是否满足
		return errors.New("witness replace vote sum insufficient")
	}
	if len(info.ChooseVoteSign) < params.GetChainMinVoting(params.SCAccount) {
		return errors.New("witness replace vote sum insufficient")
	}

	if !contract.UseGas(params.SloadGas + params.SstoreLoadGas) {
		return vm.ErrOutOfGas
	}

	// 链是否更换见证人中
	chainStatus, statusNumber := GetChainStatus(stateDB, info.Chain)
	if chainStatus != ChainStatusWitnessReplaceUnderway {
		return errors.New("chain status is not witness replace underway")
	}
	if info.ApplyNumber != statusNumber {
		return fmt.Errorf("mismatch status number:want %d,got %d", statusNumber, info.ApplyNumber)
	}

	// 为链的最后一批见证人选择最后合法位置的投票内容
	vote := types.ChooseVote{
		Chain:       info.Chain,
		ApplyNumber: info.ApplyNumber,
		ApplyUHash:  info.ApplyUHash,
		Choose:      info.Choose,
	}

	signer := types.MakeSigner(*evm.ChainConfig())
	// 投票校验
	for _, v := range info.ChooseVoteSign {
		vote.Sign.SetSign(v)
		if !contract.UseGas(params.EcrecoverGas) {
			return vm.ErrOutOfGas
		}
		send, err := types.Sender(signer, &vote)
		if err != nil {
			return err
		}
		if !contract.UseGas(params.SloadGas * 3) {
			return vm.ErrOutOfGas
		}
		if !IsMainSystemWitness(stateDB, send) {
			return errors.New("sign account is not main system witness")
		}
	}

	//校验选择内容是否正确
	{
		//1. 必须在用户链上
		if info.Choose.ChainID != info.Chain {
			if info.Choose.Height == 0 && info.Choose.ChainID != params.SCAccount {
				return errors.New("invalid choose unit")
			}
		}
		//2. 投票正确 ，如何才能判断是合法的见证人呢？
		// 一种情况是：当前见证人未处理过任何交易，因此选择的单元= beingNumber-1
		//  情况2： 当前见证人有出任意个单元，因此选择的单元将  >=beginNumber
		//beginNumber, err := GetPreGroupIndexFromUser(stateDB, info.Chain)
		//if err != nil {
		//	return err
		//}
		//if beginNumber > 0 {
		//	if beginNumber == 1 {
		//		if info.Choose.Height > 1 {
		//			return fmt.Errorf("invalid choose unit:beginNumber=%d,choose=%d", beginNumber, info.Choose.Height)
		//		}
		//	} else if info.Choose.Height < beginNumber-1 {
		//		return fmt.Errorf("invalid choose unit:beginNumber=%d,choose=%d", beginNumber, info.Choose.Height)
		//	}
		//}

		for _, v := range info.UnitVoterSigns {
			vote := v.ToVote(info.Choose.Hash)
			if !contract.UseGas(params.EcrecoverGas) {
				return vm.ErrOutOfGas
			}
			sender, err := types.Sender(signer, &vote)
			if err != nil {
				return err
			}
			//无法确认见证人是否属实
			if !ExistWitness(stateDB, sender) {
				return fmt.Errorf("%s is not witness", sender)
			}
			//这里不能直接判断单元是否存在，因为每个节点的视图不一样
			//	因此这里得安全性保障由：8个系统见证人不能作恶，否则存在风险。
		}
		//在校验通过后，实际变相的指定用户链指定位置必须是某单元
		if err := setSpecialUnitAtChain(stateDB, info.Choose); err != nil {
			return err
		}
	}

	if !contract.UseGas(params.SloadGas + params.SstoreLoadGas) {
		return vm.ErrOutOfGas
	}
	oldGroupIndex, err := GetGroupIndexFromUser(stateDB, info.Chain)
	if err != nil {
		return err
	}
	if oldGroupIndex == 0 {
		return errors.New("current witness is empty witness list")
	}
	if !contract.UseGas(params.SloadGas + params.SstoreSetGas*5) {
		return vm.ErrOutOfGas
	}
	newGroupIndex, err := ApplyChainFromWitnessGroup(stateDB, info.Chain, evm.Ctx().UnitNumber, &oldGroupIndex, info.Choose.Height+1)
	if err != nil {
		//如果失败,如果缺失见证人，则回退
		if err == errNoWitnessGroupUse {
			log.Debug("witness replace failed,recovery to current group",
				"group", oldGroupIndex, "chain", info.Chain, "err", err)
			WriteChainStatus(stateDB, info.Chain, ChainStatusNormal, evm.Ctx().UnitNumber.Uint64(), oldGroupIndex)
			return nil
		}
		return err
	}
	log.Debug("witness replace done", "chain", info.Chain, "choose", info.Choose.Height, "old.gp", oldGroupIndex, "new.gp", newGroupIndex)
	return nil
}

//因为PoW模式申请和更换见证人是为了避免攻击。但是会造成一定的使用门槛，特别是手机用户，难以快速申请见证人。
//因此，为了降低使用门槛。允许通过付费来帮助他人申请见证人，完成地址登记。
//因为仅仅是首次申请见证人，因此不存在风险。也不需要额外授权，只需要输入新账户地址，便可以完成对新地址的申请见证人工作。
//但为增加系统容错能力，还允许为已分配见证人但是有效见证人人数不足 2/3+1 (8人) 的地址重新分配见证人。
//检查要点：
//	1. 地址不能和交易发起方一致。
//  2. 地址必须是：尚未分配见证人。或，有效见证人人数不足。
func HandleApplyWitnessWay2(evm vm.CallContext, contract vm.Contract, chain common.Address) error {
	if !contract.UseGas(params.SloadGas * 10) {
		return vm.ErrOutOfGas
	}
	if chain.Empty() {
		return errors.New("invalid address")
	}
	if contract.Caller() == chain {
		return errors.New("you can not apply witness to yourself")
	}
	if chain == params.SCAccount {
		return errors.New("disable apply witness for system chain")
	}

	if !contract.UseGas(params.SloadGas * 2) {
		return vm.ErrOutOfGas
	}

	//如果是一个合约地址，则检查是否存在此合约
	if chain.IsContract() {
		if !contract.UseGas(params.SloadGas) {
			return vm.ErrOutOfGas
		}
		if !ExistContractAddress(evm.State(), chain) {
			return errors.New("the contract chain is not exist")
		}
	}

	groupIndex, err := GetGroupIndexFromUser(evm.State(), chain)
	if err == ErrChainMissingWitness { //说明无见证人，直接申请
		if !contract.UseGas(params.SloadGas * 2000) {
			return vm.ErrOutOfGas
		}
		_, err = ApplyChainFromWitnessGroup(evm.State(), chain, evm.Ctx().UnitNumber, nil, 1)
		return err
	} else if err != nil { //否则有错误信息，则返回错误信息
		return err
	}

	//用户有见证人，但需要检查有效见证人是否足够
	if !contract.UseGas(params.SloadGas) {
		return vm.ErrOutOfGas
	}
	active := GetWitnessCountAt(evm.State(), groupIndex)
	if active >= uint64(params.GetChainMinVoting(chain)) {
		var canReplace bool
		//如果当前组见证人掉线（5个轮次时间内无心跳），允许他人协助更换见证
		if !haveGoodHeartbeatOnGroup(evm.State(), groupIndex, math.ForceSafeSub(params.CalcRound(evm.Ctx().UnitNumber.Uint64()), 5)) {
			canReplace = true
		}

		if !canReplace {
			return fmt.Errorf("you can not apply witness when the address %s's witness is enough (%d)", chain, active)
		}
	}
	//可以更换见证人，但有可能失败
	_, err = replaceWitness(evm, contract, chain, true)
	return err
}

// 设置链的特殊单元
func setSpecialUnitAtChain(db StateDB, uid types.UnitID) error {
	if uid.Height == 0 {
		return nil
	}
	//检查是否已存在
	item := SubItems([]interface{}{defChainInfoPrefix, uid.ChainID})

	v := item.GetSub(db, []interface{}{defChainSpecialUnitPrefix, uid.Height})
	if !v.Empty() {
		if v != uid.Hash {
			return fmt.Errorf("exist choose %s,now want choose %s", v.String(), uid.Hash.String())
		}
		//允许重复
		return nil
	}
	item.SaveSub(db, []interface{}{defChainSpecialUnitPrefix, uid.Height}, uid.Hash)
	return nil
}

// 返回对应链在指定高度是否存在特殊
func GetChainSpecialUnit(db StateDB, chain common.Address, number uint64) common.Hash {
	item := SubItems([]interface{}{defChainInfoPrefix, chain})
	return item.GetSub(db, []interface{}{defChainSpecialUnitPrefix, number})
}

// ApplyWitness 申请见证人
func ApplyWitness(evm vm.CallContext, contract vm.Contract, input []byte) (groupIndex uint64, err error) {
	groupIndex, err = createWitness(evm, contract, common.Address{}, input)
	log.Debug("witness apply done", "groupIndex", groupIndex, "chain", contract.Caller(), "err", err)
	return
}

// removeChainWitness 移除见证人
func removeChainWitness(evm vm.CallContext, contract vm.Contract, witness common.Address, isBad bool) error {
	if !isBad { //移除非作恶见证人，形同注销
		_, err := cancelWitness(evm, contract, witness)
		return err
	}

	statedb := evm.State()

	if !contract.UseGas(params.SloadGas) {
		return vm.ErrOutOfGas
	}

	info, err := GetWitnessInfo(statedb, witness)
	if err != nil {
		return err
	}
	if info.Status != WitnessNormal {
		return errors.New("the witness is not active,can not do any")
	}

	info.CancelHeight = evm.Ctx().UnitNumber.Uint64()
	info.Status = WitnessInBlackList

	// 更新见证人信息
	if !contract.UseGas(params.SstoreSetGas * 10) {
		return vm.ErrOutOfGas
	}
	if err := onWitnessExit(statedb, evm.Ctx().UnitNumber.Uint64(), info); err != nil {
		return err
	}
	return nil
}

// 见证人降级
func demoteWitness(evm vm.CallContext, witness common.Address) error {
	statedb := evm.State()
	info, err := GetWitnessInfo(statedb, witness)
	if err != nil {
		return err
	}
	if info.Status != WitnessNormal {
		return errors.New("the witness is not active,can not do any")
	}
	if info.GroupIndex != DefSysGroupIndex {
		return errors.New("demoted witness is not system witness")
	}
	// 先删除
	if err = CancelWitnessFromWitnessGroup(statedb, info.Address, info.GroupIndex); err != nil {
		return err
	}
	// 补充到末尾
	if _, err := ApplyWitnessGroupOfSys(statedb, info.Address); err != nil {
		return err
	}
	return AddWitnessChangeEventLog(statedb, info)
}

// CreateContract 创建合约
// 需先申请新的见证人
func CreateContract(evm vm.CallContext, contract vm.Contract, input []byte) (newAddr common.Address, groupIndex uint64, err error) {
	if len(input) <= 2 {
		err = fmt.Errorf("invalid deploy contract code")
		return
	}
	from := contract.Caller()
	statedb := evm.State()
	vmMagic := binary.BigEndian.Uint16(input[:2])
	// Nerthus 网络用户的Nonce值是一个无意义的字段，因此不能借助此Nonce
	// 但可以直接借助交易时间
	nonce := evm.Ctx().Tx.Time()
	for {
		// 消耗
		if !contract.UseGas(params.Bn256PairingBaseGas) {
			return common.Address{}, 0, vm.ErrOutOfGas
		}
		newAddr = common.GenerateContractAddress(from, vmMagic, nonce)
		// 消耗
		if !contract.UseGas(params.SstoreLoadGas + params.SstoreSetGas) {
			return common.Address{}, 0, vm.ErrOutOfGas
		}
		// 如果不存在则可以使用，否则继续寻找
		if !ExistContractAddress(statedb, newAddr) {
			break
		}
		nonce++
	}
	if err = WriteChainCreater(statedb, contract.Caller(), newAddr); err != nil {
		return common.Address{}, 0, err
	}
	groupIndex, err = createWitness(evm, contract, newAddr, nil)
	return
}

// GetReplaseWitnessTraget 获取更换见证人交易的更改地址
func GetReplaseWitnessTraget(sender common.Address, txInput []byte) (common.Address, error) {
	if len(txInput) == 0 {
		return sender, nil
	}
	var who common.Address
	if err := abiObj.Methods["ReplaceWitness"].Inputs.Unpack(&who, txInput[4:]); err != nil {
		return common.Address{}, fmt.Errorf("invalid input length,can not convert to address,%v", err)
	}
	// 如果只是普通账户，则要求只允许交易发送者更改自己的见证人，无权限修改其他见证人地址
	if !who.IsContract() && who != sender {
		return common.Address{}, errors.New("does not allow change witnesses of other account")
	}
	return who, nil
}

// createWitness 内置合约,申请见证人
func createWitness(evm vm.CallContext, contract vm.Contract, newContactAddr common.Address, input []byte) (uint64, error) {
	target := newContactAddr
	var err error
	if newContactAddr.Empty() {
		target, err = GetReplaseWitnessTraget(contract.Caller(), input)
		if err != nil {
			return 0, err
		}
	}
	if target.Empty() {
		return 0, errors.New("invalid target")
	}
	if target == params.SCAccount {
		return 0, errors.New("not allowed to be replaced system chain witness")
	}
	state := evm.State()
	if !contract.UseGas(params.SstoreLoadGas) {
		return 0, vm.ErrOutOfGas
	}
	// 更换见证组
	if !contract.UseGas(params.SstoreSetGas*2 + params.SloadGas*2) {
		return 0, vm.ErrOutOfGas
	}
	groupIndex, err := GetGroupIndexFromUser(state, target)
	switch err {
	case nil:
		return 0, errors.New("the target already have witness")
	case ErrChainMissingWitness:
		if !contract.UseGas(params.Sha256BaseGas) {
			return 0, vm.ErrOutOfGas
		}
		if newContactAddr.Empty() {
			groupIndex, err = ApplyChainFromWitnessGroup(state, target, evm.Ctx().UnitNumber, nil, 1)
		} else {
			// 是合约地址直接使用caller见证组
			groupIndex, err = GetGroupIndexFromUser(state, contract.Caller())
			if err != nil {
				return 0, err
			}
			groupIndex, err = applyWitnessWithGroup(state, target, evm.Ctx().UnitNumber, nil, &groupIndex, 1)
		}
		if err != nil {
			return 0, err
		}
		return groupIndex, nil
	default:
		return 0, err
	}
}

// replaceWitness 更换见证人
func replaceWitness(evm vm.CallContext, contract vm.Contract, target common.Address, replaceByOther bool) (groupIndex uint64, err error) {
	if target.Empty() {
		return 0, errors.New("invalid target")
	}
	if target == params.SCAccount {
		return 0, errors.New("not allowed to be replaced system chain witness")
	}
	state := evm.State()
	if !contract.UseGas(params.SstoreLoadGas) {
		return 0, vm.ErrOutOfGas
	}
	if number := GetChainArbitrationNumber(state, target); number > 0 {
		return 0, fmt.Errorf("chain in arbitrationing at number %d", number)
	}

	// 更换见证组
	if !contract.UseGas(params.SstoreSetGas*2 + params.SloadGas*2) {
		return 0, vm.ErrOutOfGas
	}
	groupIndex, err = GetGroupIndexFromUser(state, target)
	if err != nil {
		return 0, err
	}
	if !contract.UseGas(params.SloadGas * 2) {
		return 0, vm.ErrOutOfGas
	}
	// 链状态
	chainStatus, number := GetChainStatus(state, target)
	switch chainStatus {
	case ChainStatusNormal:
		if !contract.UseGas(params.SstoreLoadGas) {
			return 0, vm.ErrOutOfGas
		}
		if !replaceByOther {
			// 更换见证人间隔
			lastNumber, err := getChainLastWitnessNumber(state, target)
			if err != nil {
				return 0, err
			}
			if evm.Ctx().UnitNumber.Uint64() < lastNumber+params.ReplaceWitnessInterval {
				witnessSet := GetWitnessSet(nil, groupIndex)
				if !contract.UseGas(params.SloadGas) {
					return 0, vm.ErrOutOfGas
				}
				//检查是否能更换见证人，在给定时间内不允许重复更换，除非见证人数不正常
				if l := witnessSet.Len(state); l >= params.ConfigParamsUCMinVotings {
					log.Debug("replace witness", "len", l, "lastNumber", lastNumber, "nowNumber", evm.Ctx().UnitNumber.Uint64(),
						"needNumber", params.ReplaceWitnessInterval)
					return 0, fmt.Errorf("you can only replace them after system chain number %d,now is %d",
						lastNumber+params.ReplaceWitnessInterval, evm.Ctx().UnitNumber.Uint64())
				}
			}
		}
	case ChainStatusWitnessReplaceUnderway:
		if !contract.UseGas(params.SloadGas) {
			return 0, vm.ErrOutOfGas
		}
		if !GetChainStatusIsTimeout(number, evm.Ctx().UnitNumber.Uint64()) {
			return 0, errors.New("chain status is not underway or number is not meet")
		}
	}
	//尝试进行一次更换
	newGroupIndex, err := selectNewGroup(&groupIndex, evm.Ctx().UnitNumber, state, target)
	if err != nil {
		return 0, err
	}
	if !contract.UseGas(params.SstoreSetGas) {
		return 0, vm.ErrOutOfGas
	}
	// 设置链的状态
	WriteChainStatus(state, target, ChainStatusWitnessReplaceUnderway, evm.Ctx().UnitNumber.Uint64(), *newGroupIndex)
	return 0, nil
}

// UpdateAcitvieWitenssLib 更新有效见证人库
func UpdateAcitvieWitenssLib(statedb StateDB, witenss *WitnessInfo) error {

	set := StateSet([]interface{}{allWitenssKey})
	switch witenss.Status {
	default:
		return fmt.Errorf("can not hande witness status %s", witenss.Status)
	case WitnessNormal:
		set.Add(statedb, witenss.Address)
		CounterItem.AddActiveWitness(statedb, 1)
		p := GetSettlePeriod(witenss.ApplyHeight)
		PeriodInfo{statedb}.AddWitnessCount(p, witenss.GroupIndex == DefSysGroupIndex, 1) //当前周期的见证人人数加1
	case WitnessLoggedOut, WitnessInBlackList:
		CounterItem.AddActiveWitness(statedb, -1)

		p := GetSettlePeriod(witenss.CancelHeight)
		PeriodInfo{statedb}.AddWitnessCount(p, witenss.GroupIndex == DefSysGroupIndex, -1) //当前周期的见证人人数减1
	}
	//添加变化事件
	return AddWitnessChangeEventLog(statedb, witenss)
}

// 获取所有见证人，注意包括历史见证人
func GetAllWitness(db StateDB) (list []WitnessInfo) {
	set := StateSet([]interface{}{allWitenssKey})
	set.Range(db, func(index uint64, value common.Hash) bool {
		info, err := GetWitnessInfo(db, common.BytesToAddress(value.Bytes()))
		if err == nil {
			list = append(list, *info)
		}
		return true
	})
	return
}

// 遍历所有有效的系统见证人和用户见证人
func RangeActiveWitness(db StateDB, f func(witness common.Address) bool) {
	//所有见证人必要出现在见证组中，
	for gp := uint64(0); gp <= GetWitnessGroupCount(db); gp++ {
		for _, w := range GetWitnessListAt(db, gp) {
			if !f(w) {
				break
			}
		}
	}
}

// GetActivWitnessLib 获取全网可用见证人
func GetActivWitnessLib(statedb StateDB) (lib []WitnessInfo, err error) {
	set := StateSet([]interface{}{allWitenssKey})
	set.Range(statedb, func(index uint64, value common.Hash) bool {
		info, err2 := GetWitnessInfo(statedb, common.BytesToAddress(value.Bytes()))
		if err2 != nil {
			err = err2
			return false
		}
		if info.Status == WitnessNormal {
			lib = append(lib, *info)
		}
		return true
	})
	return
}

// GetActiveWitnessCount 获取全网有效见证人人数
func GetActiveWitnessCount(statedb StateDB) uint64 {
	return CounterItem.ActiveWtiness(statedb)
}

// WriteWitnessInfo 写入见证人信息
func WriteWitnessInfo(statedb StateDB, witenss *WitnessInfo) error {
	item := SubItems([]interface{}{witnessPrefix, witenss.Address})
	//保存自己
	item.SaveSub(statedb, nil, byte(1)) //flag
	item.SaveSub(statedb, "margin", witenss.Margin)
	item.SaveSub(statedb, "status", witenss.Status)
	item.SaveSub(statedb, "cancel_height", witenss.CancelHeight)
	item.SaveSub(statedb, "apply_height", witenss.ApplyHeight)
	item.SaveSub(statedb, "group_index", witenss.GroupIndex)
	item.SaveBigData(statedb, "pkey", witenss.Pubkey)

	//初始化心跳记录
	role := types.AccMainSystemWitness
	if witenss.GroupIndex != DefSysGroupIndex {
		role = types.AccUserWitness
	}
	initHearbeatLastRound(statedb, role, witenss.Address, witenss.ApplyHeight)
	return nil
}

// DelWitnessInfo 删除见证人
func DelWitnessInfo(db StateDB, address common.Address, number uint64, status WitnessStatus) {
	set := SubItems([]interface{}{witnessPrefix, address})
	set.SaveSub(db, "status", status)
	set.SaveSub(db, "cancel_height", number)
}

// GetWitnessGroupIndex 获取见证所在的见证组索引
func GetWitnessGroupIndex(db StateDB, witenss common.Address) (uint64, error) {
	item := SubItems([]interface{}{witnessPrefix, witenss})
	gpIndex := item.GetSub(db, "group_index").Big().Uint64()
	if gpIndex == 0 {
		flag := item.GetSub(db, nil)
		if flag.Empty() {
			return 0, ErrNotFindWitness
		}
	}
	return gpIndex, nil
}

// ExistWitness 判断是否是见证人，包括历史见证人
func ExistWitness(statedb StateDB, address common.Address) bool {
	item := SubItems([]interface{}{witnessPrefix, address})
	flag := item.GetSub(statedb, nil)
	return flag.Empty() == false
}
func GetWitnessStatus(db StateGetter, witness common.Address) (status WitnessStatus, isWitness bool) {
	item := SubItems([]interface{}{witnessPrefix, witness})
	flag := item.GetSub(db, nil)
	if flag.Empty() {
		return 0, false
	}
	return WitnessStatus(item.GetSub(db, "status").Big().Int64()), true
}

// GetWitnessInfo 获取见证人信息
func GetWitnessInfo(statedb StateDB, address common.Address) (*WitnessInfo, error) {
	item := SubItems([]interface{}{witnessPrefix, address})
	status := item.GetSub(statedb, "status")
	if status.Empty() {
		return nil, ErrNoWintessInfo
	}
	pkey, err := item.GetBigData(statedb, "pkey")
	if err != nil {
		return nil, err
	}
	var info WitnessInfo
	info.Address = address
	info.Margin = item.GetSub(statedb, "margin").Big().Uint64()
	info.Status = WitnessStatus(status.Big().Int64())
	info.CancelHeight = item.GetSub(statedb, "cancel_height").Big().Uint64()
	info.ApplyHeight = item.GetSub(statedb, "apply_height").Big().Uint64()
	info.GroupIndex = item.GetSub(statedb, "group_index").Big().Uint64()
	info.Pubkey = pkey
	return &info, nil
}

type WitnessInfoLazy struct {
	db     StateDB
	item   SubItems
	addr   common.Address
	status WitnessStatus
}

func CreateWintessInfoLazy(statedb StateDB, witness common.Address) (WitnessInfoLazy, error) {
	item := SubItems([]interface{}{witnessPrefix, witness})
	status := item.GetSub(statedb, "status")
	if status.Empty() {
		return WitnessInfoLazy{}, ErrNoWintessInfo
	}
	return WitnessInfoLazy{
		db:     statedb,
		item:   item,
		addr:   witness,
		status: WitnessStatus(status.Big().Int64()),
	}, nil
}
func (w WitnessInfoLazy) Address() common.Address {
	return w.addr
}
func (w WitnessInfoLazy) Status() WitnessStatus {
	return w.status
}

func (w WitnessInfoLazy) Margin() uint64 {
	return w.item.GetSub(w.db, "margin").Big().Uint64()
}
func (w WitnessInfoLazy) ApplyHeight() uint64 {
	return w.item.GetSub(w.db, "apply_height").Big().Uint64()
}

// 获取见证人在同组见证人中的位置，如果为-1，表示不再此组见证人中
func GetWitnessIndex(db StateDB, gp uint64, witness common.Address) int {
	witnessSet := GetWitnessSet(db, gp)
	index, ok := witnessSet.Exist(db, witness)
	if !ok {
		return -1
	}
	return int(index)
}

// GetChainWitness 获取给定链的见证人
func GetChainWitness(statedb StateDB, mc common.Address) (common.AddressList, error) {
	if mc == params.SCAccount {
		//取系统链见证人
		_, sysWits, err := GetSystemMainWitness(statedb)
		if err != nil {
			return nil, err
		}
		return sysWits, nil
	}

	groupIndex, err := GetGroupIndexFromUser(statedb, mc)
	if err != nil {
		return nil, err
	}
	//去见证人列表
	return GetWitnessListAt(statedb, groupIndex), nil
}

// 判断是否已存在合约地址
func ExistContractAddress(statedb StateDB, chainAddr common.Address) bool {
	key := common.BytesToHash(append(chainCreaterPrefix, chainAddr.Bytes()...))
	return !statedb.GetState(params.SCAccount, key).Empty()
}

// WriteChainCreater 登记合约创建者信息
func WriteChainCreater(statedb StateDB, creater, chainAddr common.Address) error {
	key := common.BytesToHash(append(chainCreaterPrefix, chainAddr.Bytes()...))
	if data := statedb.GetState(params.SCAccount, key); !data.Empty() { //不为空表示已存在
		return errors.New("chain address already exists")
	}
	statedb.SetState(params.SCAccount, key, creater.Hash())
	return nil
}

// GetGroupIndexFromUser 获取给定链的见证组索引
func GetGroupIndexFromUser(db StateDB, address common.Address) (uint64, error) {
	if address == params.SCAccount {
		return DefSysGroupIndex, nil
	}
	item := SubItems([]interface{}{defChainInfoPrefix, address})
	group := item.GetSub(db, "group_index")
	if group.Empty() {
		if config.IgnoreAcctCheck {
			return 1, nil
		}
		return 0, ErrChainMissingWitness
	}
	return group.Big().Uint64(), nil
}

// GetChainLastWitnessNumber 获取链最后一个更换的系统链ID
func getChainLastWitnessNumber(db StateDB, address common.Address) (uint64, error) {
	item := SubItems([]interface{}{defChainInfoPrefix, address})
	number := item.GetSub(db, "apply_number")
	if number.Empty() { //
		return 0, nil
	}
	return number.Big().Uint64(), nil
}

// WriteChainInfo 写入链信息
func WriteChainInfo(db StateDB, address common.Address, sysNumber, groupIndex, chainNumber uint64) {
	item := SubItems([]interface{}{defChainInfoPrefix, address})
	oldNumber := item.GetSub(db, "apply_number")
	oldGroup := item.GetSub(db, "group_index")

	item.SaveSub(db, "apply_number", sysNumber)
	item.SaveSub(db, "apply_chain_number", chainNumber)
	item.SaveSub(db, "group_index", groupIndex)
	item.SaveSub(db, "apply_number_old", oldNumber)
	item.SaveSub(db, "group_index_old", oldGroup)
	WriteChainStatus(db, address, ChainStatusNormal, sysNumber, groupIndex)
}

// WriteChainStatus 设置链的状态
func WriteChainStatus(db StateDB, address common.Address, status ChainStatus, sysNumber uint64, groupIndex uint64) {
	item := SubItems([]interface{}{defChainInfoPrefix, address})
	item.SaveSub(db, "apply_status", status)
	item.SaveSub(db, "apply_status_number", sysNumber)

	//如果old为0，则说明是第一次申请见证人，属于账户激活
	first := item.GetSub(db, "group_index_old").Big().Sign() == 0

	// 见证组不可能超过：1<<12-1 ,直接转换为uint16
	AddChainStatusChangedLog(db, address, status, uint16(groupIndex), first)
}

func updateWitnessGroupIndex(db StateDB, witness common.Address, gp uint64) {
	SubItems([]interface{}{witnessPrefix, witness}).SaveSub(db, "group_index", gp)
}

// GetChainStatus 获取链的状态
func GetChainStatus(db StateDB, address common.Address) (ChainStatus, uint64) {
	item := SubItems([]interface{}{defChainInfoPrefix, address})
	hash := item.GetSub(db, "apply_status")
	if hash.Empty() {
		if config.IgnoreAcctCheck {
			return ChainStatusNormal, 0
		}
		return ChainStatusNotWitness, 0
	}
	return ChainStatus(hash.Big().Int64()), item.GetSub(db, "apply_status_number").Big().Uint64()
}
func GetChainStatusIsTimeout(lastNumber uint64, currNumber uint64) bool {
	return currNumber >= params.ReplaceWitnessUnderwayInterval+lastNumber
}

// GetChainWitnessStartNumber 获取链见证人的起始高度
func GetChainWitnessStartNumber(db StateDB, address common.Address) uint64 {
	item := SubItems([]interface{}{defChainInfoPrefix, address})
	hash := item.GetSub(db, "apply_chain_number")
	if hash.Empty() {
		return 0
	}
	return hash.Big().Uint64()
}
