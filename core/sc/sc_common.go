package sc

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"

	"gitee.com/nerthus/nerthus/common/sync/cmap"

	"gitee.com/nerthus/nerthus/accounts/abi"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/rlp"
)

var (
	FuncReplaceWitnessIDBytes = common.FromHex(FuncReplaceWitnessID)
	FuncApplyWitnessIDBytes   = common.FromHex(FuncApplyWitnessID)
)

// 每组见证人数量
const (
	FuncIdSetMemberJoinApply        = "e087d7bf" // 申请加入理事会提案
	FuncIdSetMemberRemoveApply      = "dd163940" // 移除理事会提案
	FuncIdCounciExit                = "0bd5f72c" // 移除理事会提案
	FuncIdSetConfigApply            = "6bd13b64" // 申请修改共识配置提案
	FuncIdSetConfigApplyVote        = "528781f1" // 理事会投票
	FuncIdSetConfigVote             = "6625f65e" // 公众投票
	FuncIdSetConfigFinalize         = "0f8161ad" // 定案
	FuncIdSetSystemWitness          = "623f1b9e" // 设置系统见证人竞选提案
	FuncIdSetUserWitness            = "1e7ea747" // 设置用户见证人竞选提案
	FuncIdSetSystemWitnessApply     = "86e21b83" // 设置见证人申请竞选
	FuncIdSetSystemWitnessAddMargin = "90ccf25e" // 设置见证人加价
	FuncIdRemoveInvalid             = "d81b740e" //移除失效的见证人或理事
	FuncIdSystemHeartbeat           = "6bb43e01" // 系统空交易
	FuncIdMemberHeartbeat           = "9a42f192" // 成员心跳
	FuncCreateContract              = "09e46464" // 创建合约
	FuncGetAdditionalID             = "c4e542dc" // 领取增发
	FuncSettlementID                = "cda70ec5" // 领取见证费用
	FuncCancelWitnessID             = "b0c6afd4" // 注销见证人
	FuncReplaceWitnessID            = "6f410bb7" // 更新用户/合约见证人
	FuncApplyWitnessID              = "778c2cad" // 申请用户/合约见证人
	FuncReplaceWitnessExecuteID     = "f05b5cf3" // 更新见证人执行
	FuncReportDishonest             = "fd4322ea" // 举报见证人作恶
	FuncIdAllSystemWitnessReplace   = "f82263f5" // 替换全部系统见证人
	FuncIDApplyWitnessWay2          = "4360748c" //付费更换见证人

	// 系统里合约AIB
	// 注意：不能随意变更任意字符，一旦修改将导致合约的方法ID发生变化
	AbiCode = `
[
{"type":"function","name":"SetMemberJoinApply","constant":false, "inputs":[], "outputs":[{"name":"proposalId","type":"bytes32"}]},
{"type":"function","name":"SetMemberRemoveApply","constant":false,"inputs":[{"name":"target","type":"address"}],"outputs":[{"name":"proposalId","type":"bytes32"}]},
{"type":"function","name":"councilExit","constant":false,"inputs":[],"outputs":[]},

{"type":"function","name":"SetConfigApply","constant":false,"inputs":[{"name":"key","type":"string"},{"name":"value","type":"uint64"}],"outputs":[{"name":"proposalId","type":"bytes32"}]},
{"type":"function","name":"SetConfigApplyVote","constant":false,"inputs":[{"name":"hash","type":"bytes32"},{"name":"opinion","type":"bool"}],"outputs":[]},
{"type":"function","name":"SetConfigVote","constant":false,"inputs":[{"name":"hash","type":"bytes32"},{"name":"opinion","type":"bool"}],"outputs":[]},
{"type":"function","name":"SetConfigFinalize","constant":false,"inputs":[{"name":"hash","type":"bytes32"}],"outputs":[]},

{"type":"function","name":"SetSystemWitness","constant":false, "inputs":[], "outputs":[{"name":"proposalId","type":"bytes32"}]},
{"type":"function","name":"SetUserWitness","constant":false, "inputs":[], "outputs":[{"name":"proposalId","type":"bytes32"}]},
{"type":"function","name":"SetSystemWitnessApply","constant":false,"inputs":[{"name":"hash","type":"bytes32"}],"outputs":[]},
{"type":"function","name":"SetSystemWitnessAddMargin","constant":false,"inputs":[{"name":"hash","type":"bytes32"}],"outputs":[]},
{"type":"function","name":"SetSystemWitnessFinalize","constant":false,"inputs":[{"name":"hash","type":"bytes32"}],"outputs":[]},
{"type":"function","name":"removeInvalid","constant":false, "inputs":[{"name":"addresses","type":"address[]"}],"outputs":[]},
{"type":"function","name":"AllSystemWitnessReplace","constant":false, "inputs":[],"outputs":[]},

{"type":"function","name":"SystemHeartbeat","constance":false,"inputs":[],"outputs":[]},
{"type":"function","name":"MemberHeartbeat","constance":false,"inputs":[],"outputs":[]},

{"type":"function","name":"GetAdditional","constant":false,"inputs":[],"outputs":[]},
{"type":"function","name":"Settlement","constance":false,"inputs":[],"outputs":[]},

{"type":"function","name":"ApplyWitness","constance":"false","inputs":[{"name":"who","type":"address"}], "outputs":[{"name":"newGroupIndex","type":"uint64"}]},
{"type":"function","name":"applyWitnessWay2","constance":"false","inputs":[{"name":"who","type":"address"}], "outputs":[]},
{"type":"function","name":"CancelWitness","constance":"false","inputs":[],"outputs":[]},
{"type":"function","name":"ReplaceWitness","constance":"false","inputs":[{"name":"who","type":"address"}], "outputs":[{"name":"newGroupIndex","type":"uint64"}]},
{"type":"function","name":"ReplaceWitnessExecute","constance":"false","inputs":[{"name":"rlpData","type":"bytes"}], "outputs":[{"name":"newGroupIndex","type":"uint64"}]},
{"type":"function","name":"ReportDishonest","constance":"false","inputs":[{"name":"rlpData", "type":"bytes"}],"outputs":[]},
{"type":"function","name":"CreateContract","constance":"false","inputs":[{"name":"code","type":"bytes"}],"outputs":[{"name":"contract","type":"address"},{"name":"newGroupIndex","type":"uint64"}]},
{
		"anonymous": false,
		"inputs": [
			{
				"indexed": false,
				"name": "user",
				"type": "address"
			},
			{
				"indexed": false,
				"name": "oldWitnessGroup",
				"type": "uint16"
			},
			{
				"indexed": false,
				"name": "newWitnessGroup",
				"type": "uint16"
			}
		],
		"name": "userWitnessChangeEvent",
		"type": "event"
},
{
		"anonymous": false,
		"inputs": [
			{
				"indexed": false,
				"name": "witness",
				"type": "address"
			},
			{
				"indexed": false,
				"name": "status",
				"type": "uint8"
			},
			{
				"indexed": false,
				"name": "margin",
				"type": "uint64"
			},
			{
				"indexed": false,
				"name": "group",
				"type": "uint16"
			},
			{
				"indexed":false,
				"name": "index",
				"type": "uint8"
			}
		],
		"name": "witnessChangeEvent",
		"type": "event"
},
{
		"anonymous": false,
		"inputs": [
			{
				"indexed": false,
				"name": "id",
				"type": "bytes32"
			},
			{
				"indexed": false,
				"name": "type",
				"type": "uint8"
			},
			{
				"indexed": false,
				"name": "item",
				"type": "string"
			},
			{
				"indexed": false,
				"name": "value",
				"type": "uint64"
			},
			{
				"indexed": false,
				"name": "status",
				"type": "uint8"
			},
			{
				"indexed": false,
				"name": "applyNumber",
				"type": "uint64"
			},
			{
				"indexed": false,
				"name": "finalizeNumber",
				"type": "uint64"
			},
			{
				"indexed": false,
				"name": "voteNumber",
				"type": "uint64"
			}
		],
		"name": "proposalChangeEvent",
		"type": "event"
},

	{
		"anonymous": false,
		"inputs": [
			{
				"indexed": false,
				"name": "id",
				"type": "bytes32"
			},
			{
				"indexed": false,
				"name": "voter",
				"type": "address"
			},
			{
				"indexed": false,
				"name": "op",
				"type": "uint8"
			},
			{
				"indexed": false,
				"name": "value",
				"type": "uint256"
			}
		],
		"name": "proposalVoteEvent",
		"type": "event"
	},
	{
		"anonymous": false,
		"inputs": [
			{
				"indexed": false,
				"name": "chain",
				"type": "address"
			},
			{
				"indexed": false,
				"name": "number",
				"type": "uint64"
			}
		],
		"name": "chainArbitrationEvent",
		"type": "event"
	},
	{
		"anonymous": false,
		"inputs": [
			{
				"indexed": false,
				"name": "chain",
				"type": "address"
			},
			{
				"indexed": false,
				"name": "number",
				"type": "uint64"
			},
			{
				"indexed": false,
				"name": "goodUnitHash",
				"type": "bytes32"
			}
		],
		"name": "chainArbitrationEndEvent",
		"type": "event"
	},
    {
    	"anonymous": false,
    	"inputs": [
    		{
    			"indexed": false,
    			"name": "chain",
    			"type": "address"
    		},
    		{
    			"indexed": false,
    			"name": "status",
    			"type": "uint8"
    		}, 
    		{
    			"indexed": false,
    			"name": "group",
    			"type": "uint16"
    		},
			{
				"indexed": false,
				"name": "first",
				"type": "bytes1"
			}
    	],
    	"name": "chainStatusChangedEvent",
    	"type": "event"
    },
    {
		"anonymous": false,
		"inputs": [
			{
				"indexed": false,
				"name": "witness",
				"type": "address"
			},
			{
				"indexed": false,
				"name": "oldGroup",
				"type": "uint16"
			},
			{
				"indexed": false,
				"name": "newGroup",
				"type": "uint16"
			}
		],
		"name": "witnessGroupChangedEvent",
		"type": "event"
	}
]`
)

// IsSCIdleContract 检查是不是系统的"空转"合约(包括空单元，以及心跳)
func IsSCIdleContract(inputData []byte) bool {
	if len(inputData) < 4 {
		return false
	}
	switch common.Bytes2Hex(inputData[:4]) {
	case FuncIdSystemHeartbeat, FuncIdMemberHeartbeat, FuncReplaceWitnessExecuteID,
		FuncReportDishonest, FuncIdRemoveInvalid:
		return true
	default:
		return false
	}
}

// CheckContractType 检查是不是系统的"心跳交易"
func CheckContractType(inputData []byte, fnId string) bool {
	if len(inputData) < 4 {
		return false
	}
	return common.Bytes2Hex(inputData[:4]) == fnId
}

// IsDeploySCContract 检查是否是部署合约
func IsDeploySCContract(inputData []byte) bool {
	if len(inputData) < 4 {
		return false
	}
	switch common.Bytes2Hex(inputData[:4]) {
	case FuncCreateContract:
		return true
	default:
		return false
	}
}

// SCContractString 获取内置合约标识
func SCContractString(inputData []byte) string {
	if len(inputData) < 4 {
		return ""
	}
	bitID := inputData[:4]
	return common.Bytes2Hex(bitID)
}

var initABI sync.Once
var abiObj abi.ABI

func init() {
	// 初始化时进行解析
	LoadSysABI()
}

// LoadSysABI 加载内置合约abi
func LoadSysABI() abi.ABI {
	initABI.Do(func() {
		obj, err := abi.JSON(strings.NewReader(AbiCode))
		if err != nil {
			panic(err)
		}
		abiObj = obj
	})
	return abiObj
}

// FuncIdSystemWitnessReplace 系统见证人更换输出
type SysRplWit struct {
	Cgroup []common.Address
	Bgroup []common.Address
}

// FuncCreateContract 创建合约见证人新增输出
type NewCtWit struct {
	Contract      common.Address
	NewGroupIndex uint64
}

const (
	UserWitnessChangeEvent   = "userWitnessChangeEvent"
	WitnessChangeEvent       = "witnessChangeEvent"
	WitnessGroupChangedEvent = "witnessGroupChangedEvent" //见证人的见证组发生变化
	ProposalChangeEvent      = "proposalChangeEvent"
	ProposalVoteEvent        = "proposalVoteEvent"
	ChainArbitrationEvent    = "chainArbitrationEvent"    //链仲裁中事件
	ChainArbitrationEndEvent = "chainArbitrationEndEvent" //链仲裁接收时间
	ChainStatusChangedEvent  = "chainStatusChangedEvent"  //链状态变更事件（正常、更换见证人中）
)

var eventTopicHash = cmap.NewWith(1)

func GetEventTopicHash(event string) common.Hash {
	v, _ := eventTopicHash.LoadOrStore(event, func() interface{} {
		ev, ok := abiObj.Events[event]
		if !ok {
			panic(fmt.Errorf("missing event %s", event))
		}
		return ev.Id()
	})
	return v.(common.Hash)
}

// 添加见证人的见证组变更事件
func AddWitnessGroupChangedEvent(db StateDB, witness common.Address, oldGp, newGp uint64) error {
	ev := abiObj.Events[WitnessGroupChangedEvent]

	data, err := ev.Inputs.Pack(witness, uint16(oldGp), uint16(newGp))
	if err != nil {
		return err
	}
	db.AddLog(&types.Log{
		Address: params.SCAccount,
		Topics:  []common.Hash{ev.Id()},
		Data:    data,
	})
	return nil
}

// 解析见证人更换见证组事件
func UnpackWitnessGroupChangedEvent(data []byte) (witness common.Address, oldGp, newGp uint64, err error) {
	var v struct {
		Witness  common.Address
		OldGroup uint16
		NewGroup uint16
	}

	err = abiObj.Events[WitnessGroupChangedEvent].Inputs.Unpack(&v, data)
	if err != nil {
		return
	}
	return v.Witness, uint64(v.OldGroup), uint64(v.NewGroup), nil
}

func AddChainStatusChangedLog(db StateDB, chain common.Address, status ChainStatus, gpIndex uint16, first bool) error {
	ev := abiObj.Events[ChainStatusChangedEvent]

	var f byte
	if first {
		f = 1
	} else {
		f = 0
	}

	data, err := ev.Inputs.Pack(chain, uint8(status), gpIndex, [1]byte{f})
	if err != nil {
		return err
	}
	db.AddLog(&types.Log{
		Address: params.SCAccount,
		Topics:  []common.Hash{ev.Id()},
		Data:    data,
	})
	return nil
}

type ChainStatusChanged struct {
	Chain  common.Address
	First  bool //是否是第一次出现
	Status ChainStatus
	Group  uint64
}

// 解码链状态变更事件
func UnpackChainStatusChangedEvent(data []byte) (ChainStatusChanged, error) {
	var v struct {
		Chain  common.Address
		Status uint8
		Group  uint16
		First  [1]byte
	}

	err := abiObj.Events[ChainStatusChangedEvent].Inputs.Unpack(&v, data)
	if err != nil {
		return ChainStatusChanged{}, nil
	}

	return ChainStatusChanged{v.Chain, v.First[0] == 1, ChainStatus(v.Status), uint64(v.Group)}, err
}

// 添加投票事件
// event voteEvent(bytes32 id,address voter,uint8 op,uint64 value);
func AddVoteEventLog(db StateDB, id common.Hash, who common.Address, op VotingResult, value *big.Int) error {
	ev := abiObj.Events[ProposalVoteEvent]
	data, err := ev.Inputs.Pack(id, who, uint8(op), value)
	if err != nil {
		return err
	}
	db.AddLog(&types.Log{
		Address: params.SCAccount,
		Topics:  []common.Hash{ev.Id()},
		Data:    data,
	})
	return nil
}

type ProposalVoteLog struct {
	Id    common.Hash
	Voter common.Address
	Op    VotingResult
	Value *big.Int
}

func UnpackVoteLog(data []byte) (ProposalVoteLog, error) {

	var v struct {
		Id    common.Hash
		Voter common.Address
		Op    uint8
		Value *big.Int
	}

	err := abiObj.Events[ProposalVoteEvent].Inputs.Unpack(&v, data)
	if err != nil {
		return ProposalVoteLog{}, nil
	}
	return ProposalVoteLog{Id: v.Id, Voter: v.Voter, Op: VotingResult(v.Op), Value: v.Value}, err
}

// 添加提案变更事件
// event proposalChangeEvent(bytes32 id,uint8 typ,string item,uint64 value,uint8 status,uint64 number_apply,uint64 number_finalize,uint64 number_vote);
func AddProposalChangeEventLog(db StateDB, p *Proposal) error {
	ev := abiObj.Events[ProposalChangeEvent]
	data, err := ev.Inputs.Pack(p.Key, uint8(p.Type), p.ConfigKey, p.ConfigValue, uint8(p.GetStatus()), p.NumberApply, p.NumberFinalize, p.NumberVote)
	if err != nil {
		return err
	}
	db.AddLog(&types.Log{
		Address: params.SCAccount,
		Topics:  []common.Hash{ev.Id()},
		Data:    data,
	})
	return nil
}

// 添加用户见证人更换成功事件
func AddWitnessChangeEventLog(db StateDB, witness *WitnessInfo) error {
	ev := abiObj.Events[WitnessChangeEvent]
	data, err := ev.Inputs.Pack(witness.Address,
		uint8(witness.Status), witness.Margin, uint16(witness.GroupIndex), uint8(witness.Index))
	if err != nil {
		return err
	}
	db.AddLog(&types.Log{
		Address: params.SCAccount,
		Topics:  []common.Hash{ev.Id()},
		Data:    data,
	})
	return nil
}

// 添加用户见证人更换成功事件
func AddUserWitnessChangeEventLog(db StateDB, who common.Address, oldGroup, newGroup uint64) error {
	ev := abiObj.Events[UserWitnessChangeEvent]
	data, err := ev.Inputs.Pack(who, uint16(oldGroup), uint16(newGroup))
	if err != nil {
		return err
	}
	db.AddLog(&types.Log{
		Address: params.SCAccount,
		Topics:  []common.Hash{ev.Id()},
		Data:    data,
	})
	return nil
}

func AddChainArbitrationEventLog(db StateDB, chain common.Address, number uint64) {
	ev := abiObj.Events[ChainArbitrationEvent]
	data, err := ev.Inputs.Pack(chain, number)
	if err != nil {
		panic(err)
	}
	db.AddLog(&types.Log{
		Address: params.SCAccount,
		Topics:  []common.Hash{ev.Id()},
		Data:    data,
	})
}

// 解码链仲裁事件
func UnpackChainArbitrationEventLog(data []byte) (chain common.Address, number uint64, err error) {
	var info struct {
		Chain  common.Address
		Number uint64
	}
	err = abiObj.Events[ChainArbitrationEvent].Inputs.Unpack(&info, data)
	if err != nil {
		return
	}
	return info.Chain, info.Number, nil
}

func AddChainArbitrationEndEventLog(db StateDB, chain common.Address, number uint64, uhash common.Hash) {
	ev := abiObj.Events[ChainArbitrationEndEvent]
	data, err := ev.Inputs.Pack(chain, number, uhash)
	if err != nil {
		panic(err)
	}
	db.AddLog(&types.Log{
		Address: params.SCAccount,
		Topics:  []common.Hash{ev.Id()},
		Data:    data,
	})
}

// 解码链仲裁结束事件
func UnpackChainArbitrationEndEventLog(data []byte) (types.UnitID, error) {
	var info struct {
		Chain        common.Address
		Number       uint64
		GoodUnitHash common.Hash
	}
	err := abiObj.Events[ChainArbitrationEndEvent].Inputs.Unpack(&info, data)
	if err != nil {
		return types.UnitID{}, err
	}
	return types.UnitID{
		ChainID: info.Chain,
		Height:  info.Number,
		Hash:    info.GoodUnitHash,
	}, nil
}

// GetContractOutput 获取内置合约返回值
func GetContractOutput(funcId string, data []byte) (interface{}, error) {
	if len(data) == 0 {
		return nil, errors.New("invalid output length")
	}
	switch funcId {
	case FuncIdSetSystemWitness:
		var proposalId common.Hash
		err := abiObj.Methods["SetSystemWitness"].Outputs.Unpack(&proposalId, data)
		return proposalId, err
	case FuncCreateContract:
		var r NewCtWit
		err := abiObj.Methods["CreateContract"].Outputs.Unpack(&r, data)
		return r, err
	case FuncReplaceWitnessID:
		var r uint64
		err := abiObj.Methods["ReplaceWitness"].Outputs.Unpack(&r, data)
		return r, err
	case FuncApplyWitnessID:
		var r uint64
		err := abiObj.Methods["ApplyWitness"].Outputs.Unpack(&r, data)
		return r, err
	case FuncIdSetMemberJoinApply:
		var r common.Hash
		err := abiObj.Methods["SetMemberJoinApply"].Outputs.Unpack(&r, data)
		return r, err
	case FuncIdSetMemberRemoveApply, FuncIdSetConfigApply, FuncIdSetUserWitness:
		var r common.Hash
		err := abiObj.Methods["SetMemberRemoveApply"].Outputs.Unpack(&r, data)
		return r, err
	default:
		return nil, errors.New("undefined method type")
	}
}
func IsCallReplaceWitness(input []byte) (common.Address, bool) {
	if len(input) <= 4 {
		return common.Address{}, false
	}
	id := common.Bytes2Hex(input[:4])
	switch id {
	case FuncReplaceWitnessID:
		var who common.Address
		err := abiObj.Methods["ReplaceWitness"].Inputs.Unpack(&who, input[4:])
		if err != nil {
			return common.Address{}, false
		}
		return who, true
	case FuncApplyWitnessID:
		var who common.Address
		err := abiObj.Methods["ApplyWitness"].Inputs.Unpack(&who, input[4:])
		if err != nil {
			return common.Address{}, false
		}
		return who, true
	}
	return common.Address{}, false
}

// GetContractInput 获取合约参数
func GetContractInput(input []byte) (interface{}, error) {
	if len(input) <= 4 {
		return 0, errors.New("invalid input length")
	}
	id := common.Bytes2Hex(input[:4])
	params := input[4:]
	switch id {
	case FuncIdSetConfigFinalize:
		var r common.Hash
		err := abiObj.Methods["SetConfigFinalize"].Inputs.Unpack(&r, params)
		return r, err
	case FuncIdRemoveInvalid:
		var list []common.Address
		err := abiObj.Methods["removeInvalid"].Inputs.Unpack(&list, params)
		return list, err
	default:
		return nil, errors.New("undefined method type")
	}
}

// CreateCallInputData 创建系统合约函数调用入参数据
func CreateCallInputData(funcID string, args ...interface{}) (input []byte) {
	var err error
	switch funcID {
	default:
		err = fmt.Errorf("undefined contract function %s", funcID)
	case FuncIDApplyWitnessWay2:
		input, err = abiObj.Pack("applyWitnessWay2", args...)
	case FuncIdRemoveInvalid:
		input, err = abiObj.Pack("removeInvalid", args...)
	case FuncApplyWitnessID:
		input, err = abiObj.Pack("ApplyWitness", args...)
	case FuncCancelWitnessID:
		input, err = abiObj.Pack("CancelWitness", args...)
	case FuncReplaceWitnessID:
		input, err = abiObj.Pack("ReplaceWitness", args...)
	case FuncIdAllSystemWitnessReplace:
		input, err = abiObj.Pack("AllSystemWitnessReplace", args...)

	case FuncSettlementID:
		input, err = abiObj.Pack("Settlement", args...)
	case FuncCreateContract:
		if len(args) != 1 {
			err = errors.New("call createContract should be contains only one parames")
		} else {
			switch v := args[0].(type) {
			case []byte:
				input = append(abiObj.Methods["CreateContract"].Id(), v...)
			default:
				err = errors.New("call createContract args[0] should be type []byte")
			}
		}
	case FuncIdSystemHeartbeat:
		input, err = abiObj.Pack("SystemHeartbeat", args...)
	case FuncReportDishonest:
		input, err = abiObj.Pack("ReportDishonest", args...)
	case FuncIdMemberHeartbeat:
		input, err = abiObj.Pack("MemberHeartbeat")
		// 理事会相关合约
	case FuncIdSetSystemWitness:
		input, err = abiObj.Pack("SetSystemWitness")
	case FuncIdSetUserWitness:
		input, err = abiObj.Pack("SetUserWitness")
	case FuncIdSetSystemWitnessApply:
		input, err = abiObj.Pack("SetSystemWitnessApply", args...)
	case FuncIdSetSystemWitnessAddMargin:
		input, err = abiObj.Pack("SetSystemWitnessAddMargin", args...)
	case FuncIdSetMemberJoinApply:
		input, err = abiObj.Pack("SetMemberJoinApply")
	case FuncIdSetMemberRemoveApply:
		input, err = abiObj.Pack("SetMemberRemoveApply", args...)
	//case FuncIdSetMemberCheckIn:
	//	input, err = abiObj.Pack("SetMemberCheckIn", args...)
	// 系统配置
	case FuncIdSetConfigApply:
		input, err = abiObj.Pack("SetConfigApply", args...)
	case FuncIdSetConfigApplyVote:
		input, err = abiObj.Pack("SetConfigApplyVote", args...)
		//case FuncIdSetConfigApplyPass:
		//	input, err = abiObj.Pack("SetConfigApplyPass", args...)
	//case FuncIdSetConfigCheckIn:
	//	input, err = abiObj.Pack("SetConfigCheckIn", args...)
	case FuncIdSetConfigVote:
		input, err = abiObj.Pack("SetConfigVote", args...)
	case FuncIdSetConfigFinalize:
		input, err = abiObj.Pack("SetConfigFinalize", args...)
	case FuncReplaceWitnessExecuteID:
		input, err = abiObj.Pack("ReplaceWitnessExecute", args...)
	}
	if err != nil {
		panic(err)
	}
	return
}

// 心跳上下文 （读：实时；写：最后需要调用CommitCache)
type ListHeartbeat struct {
	sub          SubItems
	addrs        StateSet
	statedb      StateDB
	writeCache   map[common.Address]Heartbeat
	deletedCache []common.Address
}

type Heartbeat struct {
	Address    common.Address // 地址
	LastTurn   uint64         // 最后心跳轮数
	LastNumber uint64         // 最后高度
	Count      ListCount      // 心跳统计
}

type ListCount struct {
	cache   map[uint64]Count
	statedb StateDB
	sub     SubItems
}

type Count struct {
	Period    uint64 // 周期
	UnitCount uint64 // 产生单元工作量
	BackCount uint64 // 候选累计工作量
}

func (self ListCount) Query(period uint64) Count {
	if v, b := self.cache[period]; b {
		return v
	}
	childSub := append(self.sub, period)

	c := Count{
		period,
		childSub.GetSub(self.statedb, "unitCount").Big().Uint64(),
		childSub.GetSub(self.statedb, "backCount").Big().Uint64(),
	}
	return c
}

func (self ListCount) AddUnitCount(period uint64) ListCount {
	c := self.Query(period)
	c.UnitCount += 1
	self.cache[period] = c
	return self
}

func (self ListCount) AddBackCount(period uint64) ListCount {

	c := self.Query(period)
	c.BackCount += 1
	self.cache[period] = c
	return self
}

func (self ListCount) Delete(period uint64) ListCount {
	c := Count{period, 0, 0}
	self.cache[period] = c
	return self
}
func (self ListCount) CommitCache() ListCount {
	startPeriod, endPeriod := self.GetWorkPeriod()
	for _, v := range self.cache {
		if v.Period == 0 {
			continue
		}
		childSub := append(self.sub, v.Period)
		if v.BackCount == 0 && v.UnitCount == 0 {
			//删除
			childSub.DeleteSub(self.statedb, "unitCount")
			childSub.DeleteSub(self.statedb, "backCount")
		} else {
			childSub.SaveSub(self.statedb, "unitCount", v.UnitCount)
			childSub.SaveSub(self.statedb, "backCount", v.BackCount)
			if startPeriod == 0 || v.Period < startPeriod {
				startPeriod = v.Period
			}
			if v.Period > endPeriod {
				endPeriod = v.Period
			}
		}
	}
	if len(self.cache) > 0 {
		self.cache = make(map[uint64]Count)
	}
	self.sub.SaveSub(self.statedb, "startPeriod", startPeriod)
	self.sub.SaveSub(self.statedb, "endPeriod", endPeriod)
	return self
}

// return work start period and end
func (self ListCount) GetWorkPeriod() (uint64, uint64) {
	startPeriod := self.sub.GetSub(self.statedb, "startPeriod").Big().Uint64()
	endPeriod := self.sub.GetSub(self.statedb, "endPeriod").Big().Uint64()
	return startPeriod, endPeriod
}
func (self ListCount) Clear() {
	startPeriod, endPeriod := self.GetWorkPeriod()
	for ; startPeriod <= endPeriod; startPeriod++ {
		childSub := append(self.sub, startPeriod)
		childSub.DeleteSub(self.statedb, "unitCount")
		childSub.DeleteSub(self.statedb, "backCount")
	}
	self.sub.DeleteSub(self.statedb, "startPeriod")
	self.sub.DeleteSub(self.statedb, "endPeriod")
}

type ListPeriodCount []PeriodCount

func (pc ListPeriodCount) Len() int { return len(pc) }

func (pc ListPeriodCount) Swap(i, j int) {
	pc[i], pc[j] = pc[j], pc[i]
}

// 按照周期升序排序
func (pc ListPeriodCount) Less(i, j int) bool { return pc[i].Period < pc[j].Period }

type PeriodCount struct {
	Period  uint64 // 周期
	Records ListAdditionalRecord
}

func (self ListPeriodCount) Query(period uint64) PeriodCount {
	for i := len(self) - 1; i >= 0; i-- {
		if self[i].Period == period {
			return self[i]
		}
	}
	return PeriodCount{}
}

// 只会执行一次, 如果已存在则跳过
func (self ListPeriodCount) SetPeriodCount(
	period uint64, records ListAdditionalRecord) ListPeriodCount {
	periodCount := PeriodCount{
		Period:  period,
		Records: records}

	for _, v := range self {
		if v.Period == period {
			return self
		}
	}

	return append(self, periodCount)
}

// 只重置已存在的对象
func (self ListPeriodCount) ResetPeriodCount(period uint64, record AdditionalRecord) ListPeriodCount {
	for _, v := range self {
		if v.Period == period {
			idx := -1
			for i, v := range v.Records {
				if v.Addr == record.Addr {
					idx = i
					break
				}
			}
			if idx >= 0 {
				v.Records[idx] = record
			}
			break
		}
	}
	return self
}

func (self ListPeriodCount) AddAdditional(period uint64, addr common.Address, count uint64) ListPeriodCount {
	for i, v := range self {
		if v.Period == period {
			self[i].Records = self[i].Records.AddRecord(addr, count)
			return self
		}
	}
	return self
}

func (self ListPeriodCount) Delete(period uint64) ListPeriodCount {
	for i, v := range self {
		if v.Period == period {
			return append(self[:i], self[i+1:]...)
		}
	}
	return self
}

type ListAdditionalRecord []AdditionalRecord

type AdditionalRecord struct {
	Addr  common.Address
	Count uint64
	Spend bool // true: 已领，false:未领
}

func (self ListAdditionalRecord) Query(addr common.Address) AdditionalRecord {
	for i := len(self) - 1; i >= 0; i-- {
		if self[i].Addr == addr {
			return self[i]
		}
	}
	return AdditionalRecord{}
}

// 存在则叠加，否则新生成
func (self ListAdditionalRecord) AddRecord(addr common.Address, count uint64) ListAdditionalRecord {
	for i := len(self) - 1; i >= 0; i-- {
		if self[i].Addr == addr {
			self[i].Count += count
			return self
		}
	}
	return append(self, AdditionalRecord{Addr: addr, Count: count})
}

func (self ListAdditionalRecord) Delete(addr common.Address) ListAdditionalRecord {
	for i, v := range self {
		if v.Addr == addr {
			return append(self[:i], self[i+1:]...)
		}
	}
	return self
}
func (pc ListAdditionalRecord) Len() int { return len(pc) }

func (pc ListAdditionalRecord) Swap(i, j int) {
	pc[i], pc[j] = pc[j], pc[i]
}

// 按照Address排序，保证顺序绝对一致
func (pc ListAdditionalRecord) Less(i, j int) bool { return pc[i].Addr.Big().Cmp(pc[j].Addr.Big()) < 0 }

func (self ListHeartbeat) Have(addr common.Address) bool {
	_, b := self.writeCache[addr]
	if b {
		return true
	}
	_, exist := self.addrs.Exist(self.statedb, addr)
	return exist
}

func (self ListHeartbeat) Query(addr common.Address) Heartbeat {
	v, b := self.writeCache[addr]
	if b {
		return v
	}
	if !self.Have(addr) {
		return Heartbeat{}
	}
	heart := Heartbeat{}
	childSub := append(self.sub, addr)
	heart.Address = addr
	heart.LastTurn = childSub.GetSub(self.statedb, "lastTurn").Big().Uint64()
	heart.LastNumber = childSub.GetSub(self.statedb, "lastNumber").Big().Uint64()
	heart.Count = ListCount{make(map[uint64]Count), self.statedb, childSub}
	return heart
}
func (self ListHeartbeat) QueryLastTurn(addr common.Address) uint64 {
	v, b := self.writeCache[addr]
	if b {
		return v.LastTurn
	}
	childSub := append(self.sub, addr)
	return childSub.GetSub(self.statedb, "lastTurn").Big().Uint64()
}
func (self ListHeartbeat) QueryLastNumber(addr common.Address) uint64 {
	v, b := self.writeCache[addr]
	if b {
		return v.LastNumber
	}
	childSub := append(self.sub, addr)
	return childSub.GetSub(self.statedb, "lastNumber").Big().Uint64()
}
func (self ListHeartbeat) QueryHeartSub(addr common.Address) ListCount {
	childSub := append(self.sub, addr)
	return ListCount{make(map[uint64]Count), self.statedb, childSub}
}

func (self ListHeartbeat) Range(call func(address common.Address, heartbeat Heartbeat) bool) {
	self.addrs.Range(self.statedb, func(index uint64, value common.Hash) bool {
		addr := common.BigToAddress(value.Big())
		heart := self.Query(addr)
		if heart.Address != common.EmptyAddress {
			return call(addr, heart)
		}
		return true
	})
}

func (self ListHeartbeat) CommitCache() ListHeartbeat {
	for _, v := range self.deletedCache {
		self.sub.DeleteSub(self.statedb, v)
		if i, b := self.addrs.Exist(self.statedb, v); b {
			self.addrs.Remove(self.statedb, i)

			childSub := append(self.sub, v)
			childSub.DeleteSub(self.statedb, "lastTurn")
			childSub.DeleteSub(self.statedb, "lastNumber")
			count := ListCount{statedb: self.statedb, sub: childSub}
			count.Clear()
		}
	}
	if len(self.deletedCache) > 0 {
		self.deletedCache = make([]common.Address, 0)
	}
	for _, v := range self.writeCache {
		childSub := append(self.sub, v.Address)
		childSub.SaveSub(self.statedb, "lastTurn", v.LastTurn)
		childSub.SaveSub(self.statedb, "lastNumber", v.LastNumber)
		// 写key
		if _, b := self.addrs.Exist(self.statedb, v.Address); !b {
			self.addrs.Add(self.statedb, v.Address)
		}
		// 写Count
		v.Count = v.Count.CommitCache()
	}
	if len(self.writeCache) > 0 {
		self.writeCache = make(map[common.Address]Heartbeat)
	}
	return self
}
func (self ListHeartbeat) Insert(Addr common.Address, LastTurn, LastNumber, Period, UnitCount, BackCount uint64) ListHeartbeat {
	c := make(map[uint64]Count)
	if Period > 0 {
		c[Period] = Count{Period, UnitCount, BackCount}
	}
	count := ListCount{c, self.statedb, append(self.sub, Addr)}
	heartbeat := Heartbeat{Address: Addr, LastTurn: LastTurn, LastNumber: LastNumber, Count: count}
	self.writeCache[Addr] = heartbeat
	return self
}

func (self ListHeartbeat) Delete(Addr common.Address) (ListHeartbeat, Heartbeat) {
	h := self.Query(Addr)
	if h.Address != common.EmptyAddress {
		delete(self.writeCache, Addr)
		self.deletedCache = append(self.deletedCache, Addr)
	}
	return self, h
}

func (self ListHeartbeat) Update(h Heartbeat) ListHeartbeat {
	if self.Have(h.Address) {
		self.writeCache[h.Address] = h
	}
	return self
}

func (self ListHeartbeat) AddUnitCount(Addr common.Address, turn, number, period uint64) ListHeartbeat {
	v := self.Query(Addr)
	if common.EmptyAddress == v.Address {
		return self.Insert(Addr, turn, number, period, 1, 0)
	}
	v.Count = v.Count.AddUnitCount(period)
	v.LastTurn = turn
	v.LastNumber = number
	return self.Update(v)
}

func (self ListHeartbeat) AddBackCount(Addr common.Address, turn, number, period uint64) ListHeartbeat {
	v := self.Query(Addr)
	if common.EmptyAddress == v.Address {
		return self.Insert(Addr, turn, number, period, 0, 1)
	}
	if turn <= v.LastTurn {
		return self
	}
	v.Count = v.Count.AddBackCount(period)
	v.LastTurn = turn
	v.LastNumber = number
	return self.Update(v)
}

type StateGetter interface {
	SetState(addr common.Address, key common.Hash, value common.Hash)
	GetState(a common.Address, b common.Hash) common.Hash
}
type StateDB interface {
	StateGetter

	AddLog(*types.Log)
	GetBigData(addr common.Address, key []byte) ([]byte, error)
	SetBigData(addr common.Address, key, value []byte) error

	// SubBalance 减少账户余额
	SubBalance(common.Address, *big.Int)
	// AddBalance 增加账户余额
	AddBalance(common.Address, *big.Int)
	// GetBalance 获取账户余额
	GetBalance(common.Address) *big.Int
}

type ConfigReader interface {
	ReadConfigValue(scHeader *types.Header, key common.Hash) (value common.Hash, err error)
}

func ForceReadConfig(db StateDB, key string) uint64 {
	v, err := ReadConfig(db, key)
	if err != nil {
		//这不应该发生的
		panic(fmt.Errorf("missing config item %q", key))
	}
	return v
}
func ReadConfig(stateDB StateDB, key string) (uint64, error) {
	keyHash := common.StringToHash(key)
	v := stateDB.GetState(params.SCAccount, keyHash)
	if v.Empty() {
		return 0, errors.New("have not value")
	}
	return v.Big().Uint64(), nil
}

func ReadConfigFromUnit(reader ConfigReader, scHeader *types.Header, key string) (uint64, error) {
	v, err := reader.ReadConfigValue(scHeader, common.StringToHash(key))
	if err != nil {
		return 0, err
	}
	if v.Empty() {
		return 0, errors.New("have not value")
	}
	return v.Big().Uint64(), nil
}

func WriteConfig(stateDB StateDB, key string, value uint64) error {
	keyHash := common.StringToHash(key)
	stateDB.SetState(params.SCAccount, keyHash, common.BytesToHash(new(big.Int).SetUint64(value).Bytes()))
	return nil
}

// 获取系统链的当前主见证人，前11位。
// 返回值 numbers 表示系统链的主见证人人数所配置的人数，crr 返回实际人数（有可能因为不积极工作而被移除，导致人数减少）
func GetSystemMainWitness(db StateDB) (numbers uint64, crr common.AddressList, err error) {
	witnessList, err := ReadSystemWitness(db)
	if err != nil {
		return 0, nil, err
	}
	num := params.ConfigParamsSCWitness
	if len(witnessList) < params.ConfigParamsSCWitness {
		num = len(witnessList)
	}
	return params.ConfigParamsSCWitness, witnessList[0:num], nil
}

// 读取系统见证人
func ReadSystemWitness(stateDB StateDB) (lib common.AddressList, err error) {
	return GetWitnessListAt(stateDB, DefSysGroupIndex), nil
}

func IsSystemWitness(db StateDB, address common.Address) (bool, error) {
	index, err := GetWitnessGroupIndex(db, address)
	if err != nil {
		if err == ErrNotFindWitness {
			return false, nil
		}
		return false, err
	}
	return index == DefSysGroupIndex, nil
}

// 是否是主系统见证人
func IsMainSystemWitness(db StateDB, witness common.Address) bool {
	gp, err := GetWitnessGroupIndex(db, witness)
	if err != nil {
		return false
	}
	if gp != DefSysGroupIndex {
		return false
	}
	index := GetWitnessIndex(db, gp, witness)
	return index <= params.ConfigParamsSCWitness-1
}

// 读取理事会成员心跳记录
func ReadCouncilHeartbeat(stateDB StateDB) (ListHeartbeat, error) {
	return readHeartbeat(stateDB, PrefixCouncilHeartbeat)
}

// 读取系统见证人心跳记录
func ReadSystemHeartbeat(stateDB StateDB) (ListHeartbeat, error) {
	return readHeartbeat(stateDB, PrefixSCWitnessHeartbeat)
}

// 读取用户见证人心跳记录
func ReadUserWitnessHeartbeat(stateDB StateDB) (ListHeartbeat, error) {
	return readHeartbeat(stateDB, PrefixUserWitnessHeartbeat)
}

func readHeartbeat(stateDB StateDB, prefix []byte) (ListHeartbeat, error) {
	//bytes, err := stateDB.GetBigData(params.SCAccount, prefix)
	//if err != nil {
	//	return ListHeartbeat{}, err
	//}
	//// 解析
	//var arr ListHeartbeat
	//err = rlp.DecodeBytes(bytes, &arr)
	//if err != nil {
	//	return nil, err
	//}
	//return arr, nil
	addrsKey := []byte("_keys")
	return ListHeartbeat{
		SubItems([]interface{}{prefix}),
		StateSet([]interface{}{prefix, addrsKey}),
		stateDB,
		make(map[common.Address]Heartbeat),
		make([]common.Address, 0),
	}, nil
}

func QueryHeartbeatLastRound(db StateDB, role types.AccountRole, addr common.Address) (lastRound uint64) {
	prefix := getHearbeatRecodePrefix(role)
	childSub := append(SubItems([]interface{}{prefix}), addr)
	return childSub.GetSub(db, "lastTurn").Big().Uint64()
}
func QueryHeartbeatLastNumber(db StateDB, role types.AccountRole, addr common.Address) (lastRound uint64) {
	prefix := getHearbeatRecodePrefix(role)
	childSub := append(SubItems([]interface{}{prefix}), addr)
	return childSub.GetSub(db, "lastNumber").Big().Uint64()
}

func getHearbeatRecodePrefix(role types.AccountRole) []byte {
	switch role {
	default:
		panic(fmt.Errorf("missing heart beat for role %s", role))
	case types.AccMainSystemWitness, types.AccSecondSystemWitness:
		return PrefixSCWitnessHeartbeat
	case types.AccUserWitness:
		return PrefixUserWitnessHeartbeat
	case types.AccCouncil:
		return PrefixCouncilHeartbeat
	}
}

// 新加入成员需要初始化心跳初始值
func initHearbeatLastRound(db StateDB, role types.AccountRole, addr common.Address, curNumber uint64) {
	listHeartbeat, _ := readHeartbeat(db, getHearbeatRecodePrefix(role))
	turns := params.CalcRound(curNumber)
	periods := GetSettlePeriod(curNumber)
	listHeartbeat = listHeartbeat.AddBackCount(addr, turns, curNumber, periods)
	listHeartbeat.CommitCache()
}

// 写入理事会心跳记录
func WriteCouncilHeartbeat(stateDB StateDB, arr ListHeartbeat) error {
	return writeHeartbeat(stateDB, arr, PrefixCouncilHeartbeat)
}

// 写入系统见证人心跳记录
func WriteSystemHeartbeat(stateDB StateDB, arr ListHeartbeat) error {
	return writeHeartbeat(stateDB, arr, PrefixSCWitnessHeartbeat)
}

// 写入用户见证人心跳记录
func WriteUserWitnessHeartbeat(stateDB StateDB, arr ListHeartbeat) error {
	return writeHeartbeat(stateDB, arr, PrefixUserWitnessHeartbeat)
}

// 写入心跳记录
func writeHeartbeat(stateDB StateDB, arr ListHeartbeat, prefix []byte) error {
	//bytes, err := rlp.EncodeToBytes(arr)
	//if err != nil {
	//	return err
	//}
	//err = stateDB.SetBigData(params.SCAccount, prefix, bytes)
	//if err != nil {
	//	return err
	//}
	//return nil
	arr.CommitCache()
	return nil
}

// 读取增发领取记录
func ReadAddtionalPeriod(stateDB StateDB) (ListPeriodCount, error) {
	bytes, err := stateDB.GetBigData(params.SCAccount, prefixAdditionalPeriod)
	if err != nil {
		return ListPeriodCount{}, err
	}
	// 解析
	var pc ListPeriodCount
	err = rlp.DecodeBytes(bytes, &pc)
	if err != nil {
		return nil, err
	}
	return pc, nil
}

// 写入增发领取记录
func WriteAddtionalPeriod(stateDB StateDB, listAddtionlPeriod ListPeriodCount) error {
	bytes, err := rlp.EncodeToBytes(listAddtionlPeriod)
	if err != nil {
		return err
	}
	err = stateDB.SetBigData(params.SCAccount, prefixAdditionalPeriod, bytes)
	if err != nil {
		return err
	}
	return nil
}

func GetVoteNum(address common.Address) (int, int) {
	if address == params.SCAccount {
		return params.ConfigParamsSCWitness, int(params.ConfigParamsSCMinVotings)
	} else {
		return params.ConfigParamsUCWitness, int(params.ConfigParamsUCMinVotings)
	}
}

// 是否是执行创建合约的交易
func IsCallCreateContractTx(to common.Address, input []byte) bool {
	if to == params.SCAccount {
		if len(input) < 4 {
			return false
		}
		if common.Bytes2Hex(input[:4]) == FuncCreateContract {
			return true
		} else {
			return false
		}
	}
	return false
}

// 获取交易类型方法
func GetTxType(tx *types.Transaction) types.TransactionType {
	if tx.To().IsContract() {
		if IsCallCreateContractTx(tx.To(), tx.Data()) {
			return types.TransactionTypeContractDeploy
		} else {
			switch SCContractString(tx.Data()) {
			case FuncIdSetMemberJoinApply:
				return types.TransactionTypeProposalCouncilApply
			case FuncIdSetMemberRemoveApply:
				return types.TransactionTypeProposalCouncilRemove
			case FuncIdSetConfigApply:
				return types.TransactionTypeProposalSetConfig
			case FuncIdSetConfigApplyVote:
				return types.TransactionTypeProposalCouncilVote
			case FuncIdSetConfigVote:
				return types.TransactionTypeProposalPublicVote
			case FuncIdSetConfigFinalize:
				return types.TransactionTypeProposalFinalize
			case FuncIdSetSystemWitness:
				return types.TransactionTypeProposalSysCampaign
			case FuncIdSetUserWitness:
				return types.TransactionTypeProposalUcCampaign
			case FuncIdSetSystemWitnessApply:
				return types.TransactionTypeProposalCampaignJoin
			case FuncIdSetSystemWitnessAddMargin:
				return types.TransactionTypeProposalCampaignAddMargin
			case FuncApplyWitnessID, FuncIDApplyWitnessWay2:
				return types.TransactionTypeWitnessApply
			case FuncReplaceWitnessID:
				return types.TransactionTypeWitnessReplace
			case FuncCancelWitnessID:
				return types.TransactionTypeWitnessCancel
			case FuncSettlementID:
				return types.TransactionTypeSettlement
			default:
				return types.TransactionTypeContractRun
			}
		}
	} else {
		return types.TransactionTypeTransfer
	}
}

// 用户更换见证人事件日志
type UserWitnessChangeLog struct {
	User                             common.Address
	OldWitnessGroup, NewWitnessGroup uint16
}

// 解码用户更换见证人事件日志
func UnpackUserWitnessChangeLog(data []byte) (UserWitnessChangeLog, error) {
	var info UserWitnessChangeLog
	err := abiObj.Events[UserWitnessChangeEvent].Inputs.Unpack(&info, data)
	return info, err
}

type WitnessChangeLog struct {
	Witness common.Address
	Status  WitnessStatus
	Margin  uint64
	Group   uint64
	Index   uint8
}

type ProposalChangeLog struct {
	Id             common.Hash
	Type           ProposalType
	Item           string
	Value          uint64
	Status         ProposalStatus
	ApplyNumber    uint64
	FinalizeNumber uint64
	VoteNumber     uint64
}

// 解码提案状态变更事件日志
func UnpackProposalChangeLog(data []byte) (ProposalChangeLog, error) {
	var info struct {
		Id             common.Hash
		Type           uint8
		Item           string
		Value          uint64
		Status         uint8
		ApplyNumber    uint64
		FinalizeNumber uint64
		VoteNumber     uint64
	}

	err := abiObj.Events[ProposalChangeEvent].Inputs.Unpack(&info, data)
	if err != nil {
		return ProposalChangeLog{}, err
	}
	return ProposalChangeLog{
		Id:             info.Id,
		Type:           ProposalType(info.Type),
		Item:           info.Item,
		Value:          info.Value,
		Status:         ProposalStatus(info.Status),
		ApplyNumber:    info.ApplyNumber,
		FinalizeNumber: info.FinalizeNumber,
		VoteNumber:     info.VoteNumber,
	}, err
}

// 解码见证人状态变更事件日志
func UnpackWitnessChangeLog(data []byte) (WitnessChangeLog, error) {
	var info struct {
		Witness common.Address
		Status  uint8
		Margin  uint64
		Group   uint16
		Index   uint8
	}
	err := abiObj.Events[WitnessChangeEvent].Inputs.Unpack(&info, data)
	if err != nil {
		return WitnessChangeLog{}, err
	}
	status := WitnessStatus(info.Status)
	if !status.IsAWitnessStatus() {
		return WitnessChangeLog{}, errors.New("invalid status value")
	}
	return WitnessChangeLog{
		Witness: info.Witness,
		Status:  status,
		Margin:  info.Margin,
		Group:   uint64(info.Group),
		Index:   info.Index,
	}, nil
}
