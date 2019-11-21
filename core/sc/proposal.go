package sc

import (
	"bytes"
	"encoding/json"
	"errors"
	"math/big"
	"sort"

	"gitee.com/nerthus/nerthus/log"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/rlp"
)

type VotingResult uint8
type ProposalStatus uint8
type ProposalType uint8
type ProposalOption uint8

//go:generate enumer -type=VotingResult -json -transform=snake -trimprefix=VotingResult
const (
	VotingResultUnknown VotingResult = iota + 1
	VotingResultAgree
	VotingResultDisagree
)

//go:generate enumer -type=ProposalType -json -transform=snake -trimprefix=ProposalType
const (
	ProposalTypeCouncilAdd ProposalType = iota + 1
	ProposalTypeCouncilFire
	ProposalTypeConfigChange
	ProposalTypeSysWitnessCampaign
	ProposalTypeUserWitnessCampaign
)

//go:generate enumer -type=ProposalStatus -json -transform=snake -trimprefix=ProposalStatus
const (
	ProposalStatusInApproval   ProposalStatus = iota + 1 // 审批中
	ProposalStatusInVoting                               // 投票中
	ProposalStatusPassed                                 // 已通过
	ProposalStatusFailed                                 // 未通过
	ProposalStatusExpired                                // 已过期
	ProposalStatusPendingJudge                           // 待定案
	ProposalStatusApply                                  // 申请中
)

//go:generate enumer -type=ProposalOption -json -transform=snake -trimprefix=ProposalOption
const (
	ProposalOptionApply    ProposalOption = iota + 1 // 可审核
	ProposalOptionVoting                             // 可投票
	ProposalOptionFinalize                           // 可定案
	ProposalOptionMarkup                             // 可加价
	ProposalOptionPass                               // 不能执行操作
	ProposalOptionJoin                               // 可竞选
	ProposalOptionUnderway                           // 交易进行中
)

var (
	publicVoteTag                 = []byte("p")
	PrefixProposalTag             = []byte("proposal")
	PrefixCouncilInfo             = []byte("ci_")          //理事信息
	PrefixMemberTag               = []byte("council_m")    // 理事成员
	PrefixCouncilHeartbeat        = []byte("heartbeat_c")  // 读取理事会成员心跳
	PrefixSCWitnessHeartbeat      = []byte("heartbeat_sc") // 系统见证人心跳
	PrefixUserWitnessHeartbeat    = []byte("heartbeat_uc")
	prefixAdditionalPeriod        = []byte("additional_p") // 增发领取
	listCampaignTag               = []byte("c")
	DefaultPageSize               = uint64(10)
	ProposalApplyTag              = []byte("voting")
	NotExistProposal              = errors.New("not exist member proposal")
	EmptyMemberList               = errors.New("member list is empty")
	NotCreateProposal             = errors.New("there is already same proposal, can't create")
	ErrIsNormalWitness            = errors.New("the target is normal witness")
	ErrIsCouncil                  = errors.New("the target is council member")
	ErrNotCampaign                = errors.New("not found campaign record")
	ErrNotApplyCampaign           = errors.New("the target already campaign, can't again apply")
	ErrNotFindProposalCouncilVote = errors.New("the proposal council vote info get failed")
	ErrNotFindProposalPublicVote  = errors.New("the proposal public vote info get failed")
)

type ListProposalStatus []ProposalStatus

func (ps ListProposalStatus) Have(p ProposalStatus) bool {
	if ps == nil {
		return false
	}
	for _, v := range ps {
		if v == p {
			return true
		}
	}
	return false
}

// getProposalItem 返回初始化提案db key
func getProposalItem() SubItems {
	return SubItems([]interface{}{PrefixProposalTag})
}

// getProposalCount 获取提案总数
func getProposalCount(db StateDB) uint64 {
	hash := getProposalItem().GetSub(db, nil)
	if hash.Empty() {
		return 0
	}
	return hash.Big().Uint64()
}

// getProposalInfoByFlag 根据提案索引获取提案详情
// numberCurrent: 当前系统链高度, 因为提案状态是根据当前高度变化的
func getProposalInfoByFlag(db StateDB, flag uint64, numberCurrent uint64) (*Proposal, error) {
	item := getProposalItem()
	proposalHash := item.GetSub(db, flag)
	if proposalHash.Empty() {
		return nil, NotExistProposal
	}
	return GetProposalInfoByHash(db, proposalHash, numberCurrent)
}

// GetProposalInfoByHash 根据提案hash获取提案
// numberCurrent: 当前系统链高度, 因为提案状态是根据当前高度变化的
func GetProposalInfoByHash(db StateDB, proposalHash common.Hash, numberCurrent uint64) (*Proposal, error) {
	item := getProposalItem().Join(proposalHash)
	if item.GetSub(db, nil).Empty() {
		return nil, NotExistProposal
	}
	var proposal Proposal
	proposal.Key = proposalHash
	proposal.Type = ProposalType(item.GetSub(db, "type").Big().Uint64())
	proposal.Margin = item.GetSub(db, "margin").Big().Uint64()
	proposal.ConfigKey = string(bytes.TrimLeftFunc(item.GetSub(db, "config_key").Bytes(), func(r rune) bool {
		if r == 0 {
			return true
		}
		return false
	}))
	proposal.ConfigValue = item.GetSub(db, "config_value").Big().Uint64()
	proposal.Applicant = item.GetSub(db, "applicant").Address()
	proposal.FireAddress = item.GetSub(db, "fire_address").Address()
	proposal.TimeApply = item.GetSub(db, "time_apply").Big().Uint64()
	proposal.TimeVote = item.GetSub(db, "time_vote").Big().Uint64()
	proposal.TimeFinalize = item.GetSub(db, "time_finalize").Big().Uint64()
	proposal.Status = ProposalStatus(item.GetSub(db, "status").Big().Uint64())
	proposal.NumberApply = item.GetSub(db, "number_apply").Big().Uint64()
	proposal.NumberFinalize = item.GetSub(db, "number_finalize").Big().Uint64()
	proposal.NumberVote = item.GetSub(db, "number_vote").Big().Uint64()
	proposal.numberCurrent = numberCurrent

	// 如果提案已结束则取配置中的数量，否则取实际
	switch proposal.GetStatus() {
	case ProposalStatusFailed, ProposalStatusExpired, ProposalStatusPassed:
		proposal.RecruitingSum = item.GetSub(db, "RecruitingSum").Big().Uint64()
	default:
		//如果是进行中的提案
		switch proposal.Type {
		case ProposalTypeUserWitnessCampaign:
			proposal.RecruitingSum = getRecruitingSum(db, false)
		case ProposalTypeSysWitnessCampaign:
			proposal.RecruitingSum = getRecruitingSum(db, true)
		}
	}

	return &proposal, nil
}

// writeProposalInfo 写入提案
func writeProposalInfo(db StateDB, proposal *Proposal) {
	item := getProposalItem().Join(proposal.Key)
	item.SaveSub(db, "type", proposal.Type)
	item.SaveSub(db, "margin", proposal.Margin)
	item.SaveSub(db, "config_key", proposal.ConfigKey)
	item.SaveSub(db, "config_value", proposal.ConfigValue)
	item.SaveSub(db, "applicant", proposal.Applicant)
	item.SaveSub(db, "fire_address", proposal.FireAddress)
	item.SaveSub(db, "time_apply", proposal.TimeApply)
	item.SaveSub(db, "time_vote", proposal.TimeVote)
	item.SaveSub(db, "time_finalize", proposal.TimeFinalize)
	item.SaveSub(db, "status", proposal.Status)
	item.SaveSub(db, "number_apply", proposal.NumberApply)
	item.SaveSub(db, "number_finalize", proposal.NumberFinalize)
	item.SaveSub(db, "number_vote", proposal.NumberVote)
	item.SaveSub(db, "RecruitingSum", proposal.RecruitingSum)
	//更新事件
	AddProposalChangeEventLog(db, proposal)
}

// addProposal 添加提案
func addProposal(db StateDB, proposal *Proposal) {
	count := getProposalCount(db)
	count++
	item := getProposalItem()
	// 写入标识对应的提案hash
	item.SaveSub(db, count, proposal.Key)
	// 写入总数
	item.SaveSub(db, nil, count)
	// 写入提案信息
	item.Join(proposal.Key).SaveSub(db, nil, count)
	writeProposalInfo(db, proposal)
}

// iterationProposal 迭代提案列表
func iterationProposal(db StateDB, numberCurrent uint64, fun func(proposal *Proposal) bool) error {
	count := getProposalCount(db)
	var (
		proposal *Proposal
		err      error
	)
	for count > 0 {
		proposal, err = getProposalInfoByFlag(db, count, numberCurrent)
		if err != nil {
			return err
		}
		if !fun(proposal) {
			return nil
		}
		count--
	}
	return nil
}

// ProposalIsCreate 提案是否可以申请
func ProposalIsCreate(db vm.StateDB, t ProposalType, numberCurrent uint64) error {
	var currentErr error
	err := iterationProposal(db, numberCurrent, func(proposal *Proposal) bool {
		if proposal.Type == t {
			switch proposal.GetStatus() {
			case ProposalStatusPassed, ProposalStatusFailed, ProposalStatusExpired:
				currentErr = nil
			default:
				currentErr = NotCreateProposal
			}
			return false
		}
		return true
	})
	if err != nil {
		return err
	}
	return currentErr
}

// getProposalId 获取提案ID
func getProposalId(db StateDB) common.Hash {
	count := getProposalCount(db)
	key := MakeKey(PrefixProposalTag, count+1)
	return common.SafeHash256(key)
}

// 提案
type Proposal struct {
	Key         common.Hash    `json:"id" gencodec="required"`      // 提案ID
	Type        ProposalType   `json:"type" gencodec="required"`    // 提案类型
	Margin      uint64         `json:"-"`                           // 保证金  *
	ConfigKey   string         `json:"config_key"`                  // 配置参数 *
	ConfigValue uint64         `json:"config_value"`                // 配置参数值  *
	Applicant   common.Address `json:"creater" gencodec="required"` // 提案者
	FireAddress common.Address `json:"fire_address"`

	TimeApply    uint64 `json:"apply_time" gencodec="required"` // 开始时间
	TimeVote     uint64 `json:"voting_time"`                    // 投票时间
	TimeFinalize uint64 `json:"ending_time"`                    // 定案时间

	Status ProposalStatus `json:"status" gencodec="required"` // 提案状态

	NumberApply    uint64 `json:"apply_unumber" gencode="required"` // 申请高度
	NumberFinalize uint64 `json:"ending_unumber"`                   // 定案高度(投票截止高度)
	NumberVote     uint64 `json:"voting_unumber"`                   // 投票高度（审批截止高度）
	numberCurrent  uint64 // 当前链的最新高度

	RecruitingSum uint64 // 见证人招募人数
}

func (p Proposal) MarshalJSON() ([]byte, error) {
	type PP Proposal
	p.Status = p.GetStatus()
	ppp := PP(p)
	return json.Marshal(ppp)
}

// GetStatus 获取提案的状态
func (p *Proposal) GetStatus() ProposalStatus {
	if p.Status == ProposalStatusPassed || p.Status == ProposalStatusFailed {
		return p.Status
	}

	if !needApproval(p.Type) { //不需要理事审批
		switch {
		case p.Status == ProposalStatusInVoting && p.numberCurrent < p.NumberFinalize: // 还在投票中
			return ProposalStatusInVoting
		case p.Status == ProposalStatusInVoting && p.numberCurrent >= p.NumberFinalize: //可以定案
			return ProposalStatusPendingJudge
		default:
			return ProposalStatusExpired
		}
	}

	switch p.Type {
	case ProposalTypeUserWitnessCampaign, ProposalTypeSysWitnessCampaign: // 用户见证人竞选, 系统见证人竞选
		if p.numberCurrent < p.NumberVote {
			return ProposalStatusApply
		}
		return ProposalStatusPendingJudge
	case ProposalTypeConfigChange: // 理事申请/移除, 共识配置修改
		switch {
		case (p.Status == ProposalStatusInApproval || p.Status == ProposalStatusInVoting) && p.numberCurrent < p.NumberVote: // 理事审批
			return ProposalStatusInApproval
		case p.Status == ProposalStatusInVoting && p.numberCurrent < p.NumberFinalize: // 公众投票
			return ProposalStatusInVoting
		case p.Status == ProposalStatusInVoting && p.numberCurrent >= p.NumberFinalize: // 待定案
			return ProposalStatusPendingJudge
		default:
			return ProposalStatusExpired
		}
	}
	return ProposalStatusFailed
}

// validStatus 校验提案状态
// 根据不同的提案状态返回不同的错误信息
func (p *Proposal) validStatus(status ProposalStatus) error {
	currentStatus := p.GetStatus()
	if currentStatus == status {
		return nil
	}
	switch currentStatus {
	case ProposalStatusInApproval:
		return errors.New("the proposal is in the council voting")
	case ProposalStatusInVoting:
		return errors.New("the proposal is in the public voting")
	case ProposalStatusPassed:
		return errors.New("the proposal is pass")
	case ProposalStatusFailed:
		return errors.New("the proposal is failed")
	case ProposalStatusExpired:
		return errors.New("the proposal is expired")
	case ProposalStatusPendingJudge:
		return errors.New("the Proposal to finalize")
	case ProposalStatusApply:
		return errors.New("the proposal is int the campaign apply or add margin")
	}
	return errors.New("the proposal status is not definition")
}

// RlpEncode 提案rlp解析
func (p *Proposal) RlpEncode() ([]byte, error) {
	return rlp.EncodeToBytes(p)
}

// Finalize 提案定案
func (p *Proposal) Finalize(evm vm.CallContext, contract vm.Contract) error {
	p.NumberFinalize = evm.Ctx().UnitNumber.Uint64()
	p.TimeFinalize = evm.Ctx().Time.Uint64()

	switch p.Type {
	case ProposalTypeConfigChange:
		return p.finalizeConfig(evm, contract)
	case ProposalTypeSysWitnessCampaign:
		return p.finalizeSystemWitness(evm, contract)
	case ProposalTypeUserWitnessCampaign:
		return p.finalizeUserWitness(evm, contract)
	case ProposalTypeCouncilAdd, ProposalTypeCouncilFire:
		return p.finalizeCouncil(evm, contract)
	}
	return errors.New("failed proposal type")
}

// finalizeConfig 修改配置文件定案
func (p *Proposal) finalizeConfig(evm vm.CallContext, contract vm.Contract) error {
	stateDB := evm.State()
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return vm.ErrOutOfGas
	}
	// 是否通过投票
	if validProposalPass(stateDB, p) {
		p.Status = ProposalStatusPassed
		// 消耗
		if !contract.UseGas(params.SstoreSetGas) {
			return vm.ErrOutOfGas
		}
		// 存储配置
		err := WriteConfig(stateDB, p.ConfigKey, p.ConfigValue)
		if err != nil {
			return err
		}
	} else {
		p.Status = ProposalStatusFailed
	}
	// 消耗
	if !contract.UseGas(params.SstoreSetGas) {
		return vm.ErrOutOfGas
	}
	// 存储信息
	writeProposalInfo(stateDB, p)
	return nil
}

// finalizeCouncil 理事定案
func (p *Proposal) finalizeCouncil(evm vm.CallContext, contract vm.Contract) error {
	stateDB := evm.State()
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return vm.ErrOutOfGas
	}
	// 是否通过投票
	if validProposalPass(stateDB, p) {
		p.Status = ProposalStatusPassed
	} else {
		p.Status = ProposalStatusFailed
	}
	// 理事申请时申请人非见证人/理事成员
	// 根据定案结果处理保证金
	if p.Type == ProposalTypeCouncilAdd {
		// 申请成为理事成功添加理事,失败退回保证金
		if p.Status == ProposalStatusPassed {
			// 如果该用户是否是理事账户或者见证账户,则提案失败
			if err := CheckMemberType(stateDB, p.Applicant); err != nil {
				p.Status = ProposalStatusFailed
				evm.Ctx().Transfer(stateDB, params.SCAccount, p.Applicant, new(big.Int).SetUint64(p.Margin))
			} else {
				if !contract.UseGas(params.SstoreSetGas) {
					return vm.ErrOutOfGas
				}
				err := AddCouncil(stateDB, &Council{p.Applicant, CouncilStatusValid, p.Margin, evm.Ctx().SCNumber}, evm.ChainConfig().Get(params.ConfigParamsCouncilCap))
				if err != nil {
					p.Status = ProposalStatusFailed
					evm.Ctx().Transfer(stateDB, params.SCAccount, p.Applicant, new(big.Int).SetUint64(p.Margin))
				}
			}
		} else {
			evm.Ctx().Transfer(stateDB, params.SCAccount, p.Applicant, new(big.Int).SetUint64(p.Margin))
		}
	} else if p.Type == ProposalTypeCouncilFire {
		// 成功剔除退回保证金,并删除理事
		if p.Status == ProposalStatusPassed {
			if !contract.UseGas(params.SstoreSetGas) {
				return vm.ErrOutOfGas
			}
			if IsActiveCouncil(stateDB, p.FireAddress) {
				// 注意：理事必须自行提现
				err := RemoveCouncil(stateDB, p.FireAddress, evm.Ctx().UnitNumber.Uint64())
				if err == nil {
					evm.Ctx().Transfer(stateDB, params.SCAccount, p.FireAddress, new(big.Int).SetUint64(p.Margin))
				} else {
					p.Status = ProposalStatusFailed //如果失败，则标记提案失败
				}
			} else {
				p.Status = ProposalStatusFailed //已经不是理事
			}
		}
	}
	// 消耗
	if !contract.UseGas(params.SstoreSetGas) {
		return vm.ErrOutOfGas
	}
	// 存储信息
	writeProposalInfo(stateDB, p)
	return nil
}

// finalizeSystemWitness 系统见证人竞选提案定案
func (p *Proposal) finalizeSystemWitness(evm vm.CallContext, contract vm.Contract) error {
	stateDB := evm.State()
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return vm.ErrOutOfGas
	}
	// 查询需要补多少人
	need := getRecruitingSum(stateDB, true)
	// 排序竞选名单
	//tempList := proposal.List
	var tempList CampaignList
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return vm.ErrOutOfGas
	}
	err := getProposalCampaignList(stateDB, p.Key, func(campaign *Campaign) bool {
		tempList = append(tempList, *campaign)
		return true
	})
	if err != nil {
		return err
	}
	var failed bool
	if need == 0 || tempList.Len() == 0 {
		failed = true
	}
	if tempList.Len() > 0 {
		// 消耗
		if !contract.UseGas(params.SstoreLoadGas) {
			return vm.ErrOutOfGas
		}
		listHeartbeat, err := ReadSystemHeartbeat(stateDB)
		if err != nil {
			return err
		}
		// 消耗
		if !contract.UseGas(params.SloadGas) {
			return vm.ErrOutOfGas
		}
		// 获取当前回合数
		round := params.CalcRound(evm.Ctx().UnitNumber.Uint64())
		periods := GetSettlePeriod(evm.Ctx().Time.Uint64())

		// 排序
		sort.Sort(tempList)
		// 补充人数
		for _, v := range tempList {
			// 加入人数足够时,返还押金
			if need == 0 {
				evm.Ctx().Transfer(stateDB, params.SCAccount, v.Address, new(big.Int).SetUint64(v.Margin))
				continue
			}
			// 判断押金
			if v.Margin < params.ConfigParamsSCWitnessMinMargin {
				evm.Ctx().Transfer(stateDB, params.SCAccount, v.Address, new(big.Int).SetUint64(v.Margin))
				continue
			}
			_, err := applyWitness(stateDB, v.Address, v.Margin, evm.Ctx().UnitNumber.Uint64(), v.PublicKey, true)
			// 加入成功,添加心跳,失败时返还押金
			if err != nil {
				log.Debug("failed witness apply", "addr", v.Address, "err", err)
				evm.Ctx().Transfer(stateDB, params.SCAccount, v.Address, new(big.Int).SetUint64(v.Margin))
				failed = true
				continue
			}
			log.Debug("witness join witness group", "addr", v.Address)

			need--
			// 新加入的见证人设置心跳数据
			listHeartbeat.Insert(v.Address, round, evm.Ctx().UnitNumber.Uint64(), periods, 0, 0)
		}
	}
	if failed {
		p.Status = ProposalStatusFailed
	} else {
		p.Status = ProposalStatusPassed
	}
	// 消耗
	if !contract.UseGas(params.SstoreSetGas) {
		return vm.ErrOutOfGas
	}
	writeProposalInfo(stateDB, p)
	return nil
}

// finalizeUserWitness finalizeUserWitness 用户见证人提案定案
func (p *Proposal) finalizeUserWitness(evm vm.CallContext, contract vm.Contract) error {
	stateDB := evm.State()
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return vm.ErrOutOfGas
	}
	need := getRecruitingSum(stateDB, false)
	// 排序竞选名单
	//tempList := proposal.List
	var tempList CampaignList
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return vm.ErrOutOfGas
	}
	err := getProposalCampaignList(stateDB, p.Key, func(campaign *Campaign) bool {
		tempList = append(tempList, *campaign)
		return true
	})
	if err != nil {
		return err
	}
	number := evm.Ctx().UnitNumber.Uint64()
	winner, loser := tempList.Select(number, need)

	failed := len(winner) == 0

	//将失败者返还押金
	for _, w := range loser {
		evm.Ctx().Transfer(stateDB, params.SCAccount, w.Address, new(big.Int).SetUint64(w.Margin))
	}
	//入选者加入见证组
	for _, w := range winner {
		_, err := applyWitness(stateDB, w.Address, w.Margin, number, w.PublicKey, false)
		// 添加失败,返还押金
		if err != nil {
			log.Debug("failed witness apply", "addr", w.Address, "err", err)
			evm.Ctx().Transfer(stateDB, params.SCAccount, w.Address, new(big.Int).SetUint64(w.Margin))
			failed = true
			continue
		}
		log.Debug("witness join witness group", "addr", w.Address)
	}
	if failed {
		p.Status = ProposalStatusFailed
	} else {
		p.Status = ProposalStatusPassed
	}
	// 消耗
	if !contract.UseGas(params.SstoreSetGas) {
		return vm.ErrOutOfGas
	}
	writeProposalInfo(stateDB, p)
	return nil
}
