package sc

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	"gitee.com/nerthus/nerthus/log"

	"gitee.com/nerthus/nerthus/consensus/arbitration/validtor"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/rlp"
)

/**
作恶仲裁信息：
//-+--------+--------------+-------------+------------+-----------------------------+-------------+
// | prefix |  key-p1  	   | key-p2 	 |  key-p3    |        Value     			|  remark     |
//-+--------+--------------+-------------+------------+-----------------------------+-------------+
// | "carb_"  | chain      |   "_last"          |       	  | number                      | 链的最近一次仲裁高度  |
//----------+--------------+-------------+------------+-----------------------------+-------------+
// | "carb_"| chain        |   number                 | 	UnitChoose	            | 仲裁信息    |
//----------+--------------+-------------+------------+-----------------------------+-------------+
// | "carb_result"| chain        |   number                 | 	header hash	        | 仲裁后单元Hash    |
//----------+--------------+-------------+------------+-----------------------------+-------------+
// | "badw_"| address      |                          | 	'1'	                    | 登记作恶见证人    |
//----------+--------------+-------------+------------+-----------------------------+-------------+
**/

var (
	ArbitrationInfoPrefix      = []byte("carb_")        //仲裁信息前缀
	arbitrationResultPrefix    = []byte("carb_result_") //仲裁信息前缀
	inArbitrationNumberSuffix  = []byte("_ing")         //指定链最近一次仲裁高度
	dishonestWitnessFlagPrefix = []byte("badw_")        //作恶见证人标记前缀
)

type UnitChoose struct {
	Header types.UnitID
	Voter  []common.Address
}

// ChainArbitration 仲裁信息
type ChainArbitration []UnitChoose

// AffectedUnit 受影响单元信息
type AffectedUnit struct {
	Header types.Header        //受影响单元
	Votes  []types.WitenssVote //该单元的投票
}

// ReportWitnessDishonest 报告见证人不诚实行为
type ReportWitnessDishonest struct {
	Choose types.Header //系统见证人的选择
	Proof  []types.AffectedUnit
}

// CreateReportDishonestTxInput 创建举报见证人作恶的交易输入信息
func CreateReportDishonestTxInput(choose types.Header, badWitnessProof []types.AffectedUnit) ([]byte, error) {
	info := ReportWitnessDishonest{
		Choose: choose,
		Proof:  badWitnessProof,
	}
	b, err := rlp.EncodeToBytes(info)
	if err != nil {
		return nil, err
	}
	return CreateCallInputData(FuncReportDishonest, b), nil
}

// 举报见证人作恶方法执行输出
type ReportDishonestOutput struct {
	UnitHash common.Hash
	Chain    common.Address
	Number   uint64
}

// PackReportDishonestOutput 获取举报见证人详情
func PackReportDishonestOutput(output []byte) (r ReportDishonestOutput, err error) {
	err = abiObj.Methods["ReportDishonest"].Outputs.Unpack(&r, output)
	return
}

// ReportWitnessDishonest 获取见证人不诚实详情
func UnPackReportDishonestInput(input []byte) (r ReportWitnessDishonest, err error) {
	if len(input) < 4 {
		return r, errors.New("invalid input data")
	}
	var b []byte
	if err = abiObj.Methods["ReportDishonest"].Inputs.Unpack(&b, input[4:]); err != nil {
		return
	}
	var rep ReportWitnessDishonest
	err = rlp.DecodeBytes(b, &rep)
	return rep, err
}

// SystemReportDishonest 举报见证人不诚实行为
func SystemReportDishonest(ctx vm.CallContext, contract vm.Contract, input []byte) (err error) {
	if !contract.UseGas(10000) {
		err = vm.ErrOutOfGas
		return
	}
	rep, err := UnPackReportDishonestInput(input)
	if err != nil {
		return
	}
	return systemReportDishonest(ctx, contract, rep)
}

// systemReportDishonestAboutMulUnit 对见证人在同一高度广播多个单元进行惩罚
// 惩罚分为几个部分工作：
//		1. 校验作恶的有效性，并保证不重复处理。
//		2. 对作恶的见证人实施处罚：没收保证金，拉入黑名单，清理数据。
//		3. 报告无效单元或投票。
func systemReportDishonest(ctx vm.CallContext, contract vm.Contract, rep ReportWitnessDishonest) (err error) {
	if len(rep.Proof) == 0 && rep.Choose.MC.Empty() {
		err = errors.New("the report info is empty")
		return
	}
	stateDB := ctx.State()

	//交易发送者必须是当前系统见证人
	if !contract.UseGas(params.SstoreLoadGas * 2) {
		err = vm.ErrOutOfGas
		return
	}
	_, sysws, err := GetSystemMainWitness(stateDB)
	if err != nil {
		return err
	} else if !sysws.Have(contract.Caller()) {
		return errors.New("reporter is not system main witness")
	}
	if !contract.UseGas(params.Sha256BaseGas * 2) {
		return vm.ErrOutOfGas
	}
	if len(rep.Proof) == 0 {
		if !contract.UseGas(params.SstoreSetGas) {
			return vm.ErrOutOfGas
		}
		// 2. 记录投票
		// 如果存在多个不同高度的仲裁，优先处理低高度的仲裁
		return arbitrationVote(stateDB, rep.Choose, contract.Caller())
	}

	var (
		mc     = rep.Proof[0].Header.MC
		number = rep.Proof[0].Header.Number
	)
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return vm.ErrOutOfGas
	}

	badWitness, err := validtor.CheckProofWithState(types.MakeSigner(*ctx.ChainConfig()), rep.Proof,
		func(uhash common.Hash) (*state.StateDB, error) {
			return ctx.Ctx().Chain.GetUnitState(uhash)
		},
		func(db *state.StateDB, address common.Address) error {
			if status, isW := GetWitnessStatus(db, address); !isW {
				return errors.New("this voter is not witness")
			} else if status != WitnessNormal {
				return errors.New("this voter is not normal witness")
			}
			return nil
		})
	if err != nil {
		return err
	}

	//不管是何种形式的作恶，一旦坐实，且是见证人，则进行惩罚
	//需要排序，已保证正确数据输出顺序
	sort.Slice(badWitness, func(i, j int) bool {
		return bytes.Compare(badWitness[i].Bytes(), badWitness[j].Bytes()) < 0
	})
	for _, bad := range badWitness {
		if !contract.UseGas(params.Bn256PairingBaseGas) {
			return vm.ErrOutOfGas
		}
		if err = writeBadWitness(stateDB, ctx, contract, bad); err != nil {
			return err
		}
	}
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return vm.ErrOutOfGas
	}
	// 消耗
	if !contract.UseGas(params.SstoreSetGas) {
		return vm.ErrOutOfGas
	}
	writeArbitrationNumberFlag(stateDB, mc, number)

	//如果提供作恶证据的同时又携带
	// 简单校验，这里难以判断系统见证人所选择的合法单元的整个完整性，所以只能寄托系统见证人不能集体作恶
	if !rep.Choose.MC.Empty() {
		if rep.Choose.Number != number || rep.Choose.MC != mc {
			return errors.New("invalid choose")
		}
		if !contract.UseGas(params.SstoreSetGas) {
			return vm.ErrOutOfGas
		}
		return arbitrationVote(stateDB, rep.Choose, contract.Caller())
	}
	return nil

}

func arbitrationVote(db StateDB, chooseUnit types.Header, voter common.Address) error {
	choose := chooseUnit.ID()
	if choose.IsEmpty() {
		return errors.New("invalid vote info")
	}

	log.Trace("arbitrationVote", "voter", voter, "chain", choose.ChainID, "number", choose.Height, "uhash", choose.Hash)
	//需要先检查是否存在历史仲裁尚未结束，如果存在，则不能对其他投票
	minNumber := GetChainArbitrationNumber(db, choose.ChainID)
	if minNumber == 0 {
		return errors.New("missing chain arbitration info")
	}
	if minNumber != choose.Height {
		return fmt.Errorf("can not vote for height number,current arbitration number is %d,your vote for %d",
			minNumber, choose.Height)
	}

	// 如果在此高度有仲裁结果则不需要继续
	if result := GetArbitrationResult(db, choose.ChainID, choose.Height); !result.Empty() {
		return errors.New("chain arbitration end")
	}

	// 存储
	voterCount, err := writeChainArbitration(db, choose, voter)
	if err != nil {
		return err
	}

	log.Trace("update chain arbitration vote", "voter", voter, "chain", choose.ChainID, "number", choose.Height, "sumVotes", voterCount)
	// 需要判断是否有超过 2/3 系统见证人投票
	//判断当前新加入的可选单元
	// 最后按本次选择结果，依次判断是否有出现最低票
	if int(voterCount) >= params.GetChainMinVoting(choose.ChainID) {
		//已仲裁选出合法单元，结束仲裁
		// 1. 清理标记
		clearArbitrationNumberFlag(db, choose.ChainID, choose.Height)
		// 2. 登记合法位置并添加事件
		writeArbitrationResult(db, choose)

		log.Trace("chain arbitration end", "chain", choose.ChainID, "number", choose.Height, "uhash", choose.Hash)
	}
	return nil
}

// 获取给定链最近一次仲裁位置，如果返回值为0，则说明无仲裁
func GetChainArbitrationNumber(db StateDB, chain common.Address) uint64 {
	chainIngs := StateSet([]interface{}{ArbitrationInfoPrefix, chain, inArbitrationNumberSuffix})
	var minNumber uint64
	chainIngs.Range(db, func(index uint64, value common.Hash) bool {
		number := hash2uint(value)
		if minNumber == 0 || number < minNumber {
			minNumber = number
		}
		return true
	})
	return minNumber
}

// writeArbitrationNumberFlag 更新给定连的最近一次仲裁位置
func writeArbitrationNumberFlag(db StateDB, chain common.Address, number uint64) {
	chainIngs := StateSet([]interface{}{ArbitrationInfoPrefix, chain, inArbitrationNumberSuffix})
	if _, ok := chainIngs.Exist(db, number); ok {
		return
	}
	chainIngs.Add(db, number)

	//记录事件
	AddChainArbitrationEventLog(db, chain, number)
}

// clearArbitrationNumberFlag 清理标记
func clearArbitrationNumberFlag(db StateDB, chain common.Address, number uint64) {
	chainIngs := StateSet([]interface{}{ArbitrationInfoPrefix, chain, inArbitrationNumberSuffix})
	index, ok := chainIngs.Exist(db, number)
	if ok {
		chainIngs.Remove(db, index)
	}
}

// GetArbitrationResult 判断给定位置是否存在仲裁结果
func GetArbitrationResult(db StateDB, chain common.Address, number uint64) common.Hash {
	key := common.SafeHash256(arbitrationResultPrefix, chain, number)
	return db.GetState(params.SCAccount, key)
}

// writeArbitrationResult 写入仲裁结果单元
func writeArbitrationResult(db StateDB, choose types.UnitID) {
	key := common.SafeHash256(arbitrationResultPrefix, choose.ChainID, choose.Height)
	db.SetState(params.SCAccount, key, choose.Hash)

	AddChainArbitrationEndEventLog(db, choose.ChainID, choose.Height, choose.Hash)
}

//// ExistChainArbitration 是否存在链仲裁
//func ExistChainArbitration(db StateDB, chain common.Address, number uint64) (bool, error) {
//	key := MakeKey(ArbitrationInfoPrefix, chain, number)
//	data, err := db.GetBigData(params.SCAccount, key)
//	if err != nil && err != state.ErrStateNotFind {
//		return false, err
//	}
//	return len(data) > 0, nil
//}

//// GetChainArbitration 获取链仲裁信息
//func GetChainArbitration(db StateDB, chain common.Address, number uint64) (ChainArbitration, error) {
//	set := StateSet([]interface{}{ArbitrationInfoPrefix, chain, number})
//	set.Add(db, common.Hash{})
//	set.Range(db, func(index uint64, value common.Hash) bool {
//
//	})
//
//	key := MakeKey(ArbitrationInfoPrefix, chain, number)
//	data, err := db.GetBigData(params.SCAccount, key)
//	if err != nil {
//		return ChainArbitration{}, err
//	}
//	var arb ChainArbitration
//	err = rlp.DecodeBytes(data, &arb)
//	return arb, err
//}

// writeChainArbitration 写入链仲裁信息
func writeChainArbitration(db StateDB, choose types.UnitID, voter common.Address) (voteCount uint64, err error) {
	chain, number := choose.ChainID, choose.Height

	voterSet := StateSet([]interface{}{ArbitrationInfoPrefix, chain, number, choose.Hash})

	//不允许重复投票
	voters := StateSet([]interface{}{ArbitrationInfoPrefix, chain, number, "voters"})
	if _, ok := voters.Exist(db, voter); ok { //不报错，直接忽略
		return voterSet.Len(db), nil
	}

	set := StateSet([]interface{}{ArbitrationInfoPrefix, chain, number})
	if _, ok := set.Exist(db, choose.Hash); !ok { //新建
		set.Add(db, choose.Hash)
	}

	voterSet.Add(db, voter) //增加对该合法单元的投票数
	voters.Add(db, voter)   //记录该见证人已投票

	// 返回该合法单元的投票数
	return voterSet.Len(db), nil
}

//// IsBadWitness 是否是作恶见证人
//func IsBadWitness(db StateDB, witness common.Address) bool {
//	key := common.SafeHash256(dishonestWitnessFlagPrefix, witness)
//	data := db.GetState(params.SCAccount, key)
//	return len(data) > 0
//}

// writeBadWitness 记录作恶见证人
func writeBadWitness(db StateDB, ctx vm.CallContext, contract vm.Contract, witness common.Address) error {
	key := common.SafeHash256(dishonestWitnessFlagPrefix, witness)
	data := db.GetState(params.SCAccount, key)
	if !data.Empty() {
		//说明已标记为作恶见证人
		return nil //不继续
	}
	// 更新已记录标记
	db.SetState(params.SCAccount, key, common.BytesToHash([]byte{1}))
	return removeChainWitness(ctx, contract, witness, true)
}
