package sc

import (
	"fmt"
	"math/big"
	"sort"
	"strconv"

	"errors"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/hexutil"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"
)

type ConfigItem struct {
	Key        string `json:"key"`
	CurrentVal uint64 `json:"current_val"`
	IsUpdate   bool   `json:"is_update"`
}

//提案投票汇总信息
type HolderVoting struct {
	HolderCount    uint64         `json:"holder_count"`
	WeightSum      *big.Int       `json:"weights_sum"`
	WeightAgree    *big.Int       `json:"weights_agree"`
	WeightDisagree *big.Int       `json:"weights_disagree"`
	VoteList       PublicVoteList `json:"votings"`
}

func NewHolderVoting() *HolderVoting {
	return &HolderVoting{
		HolderCount:    0,
		WeightSum:      big.NewInt(0),
		WeightAgree:    big.NewInt(0),
		WeightDisagree: big.NewInt(0),
		VoteList:       make([]PublicVote, 0),
	}
}

type SCAPI struct {
	chain DagChain
	b     Backend
}

func NewPublicScApi(b Backend, chain DagChain) *SCAPI {
	return &SCAPI{b: b, chain: chain}
}

// GetCouncils 获取理事列表
func (api *SCAPI) GetCouncils() (common.AddressList, error) {
	stateDB, err := api.chain.GetChainTailState(params.SCAccount)
	if err != nil {
		return common.AddressList{}, nil
	}
	return GetCouncilList(stateDB)
}

// getStateDB 获取系统链最后一个stateDB
func (api *SCAPI) getStateDB() (*state.StateDB, error) {
	hash := api.chain.GetChainTailHash(params.SCAccount)
	stateDB, err := api.chain.GetUnitState(hash)

	if err != nil {
		return nil, err
	}
	return stateDB, nil
}

// Page 分页
func Page(pageIndex, pageSize uint64) (uint64, uint64) {
	if pageSize == 0 {
		pageSize = DefaultPageSize
	}
	var firstIndex uint64
	if pageIndex == 0 {
		firstIndex = 0
	} else {
		firstIndex = pageIndex * pageSize
	}
	return firstIndex, firstIndex + pageSize
}

// GetProposals 获取提案列表
func (api *SCAPI) GetProposals(pageIndex, pageSize uint64, status ListProposalStatus) (*types.PageList, error) {
	proposals := types.NewPageList()
	stateDB, err := api.getStateDB()
	if err != nil {
		return proposals, err
	}
	numberCurrent := api.chain.GetChainTailHead(params.SCAccount).Number
	err = iterationProposal(stateDB, numberCurrent, func(proposal *Proposal) bool {
		if len(status) == 0 || status.Have(proposal.GetStatus()) {
			proposals.Add(*proposal)
		}
		return true
	})
	if err != nil {
		return proposals, err
	}
	return proposals.Page(pageIndex, pageSize), nil
}

// GetProposalDetail 提案基本信息
func (api *SCAPI) GetProposalDetail(hash common.Hash) (Proposal, error) {
	stateDB, err := api.getStateDB()
	if err != nil {
		return Proposal{}, err
	}
	p, err := GetProposalInfoByHash(stateDB, hash, api.chain.GetChainTailHead(params.SCAccount).Number)
	if err != nil {
		return Proposal{}, err
	}
	return *p, nil
}

// GetProposalApplys 提案审批信息
func (api *SCAPI) GetProposalApplys(hash common.Hash) (CouncilVoteList, error) {
	var arrMemVote CouncilVoteList
	stateDB, err := api.getStateDB()
	if err != nil {
		return arrMemVote, err
	}
	// 提案是否存在
	proposal, err := api.GetProposalDetail(hash)
	if err != nil {
		return arrMemVote, err
	}
	err = GetProposalCouncilVote(stateDB, proposal.Key, func(vote *CouncilVote) bool {
		arrMemVote = append(arrMemVote, *vote)
		return true
	})
	return arrMemVote, err
}

// GetProposalVotings 提案投票汇总信息
func (api *SCAPI) GetProposalVotings(proposalHash common.Hash, pageIndex, pageSize uint64) (HolderVoting, error) {
	firstIndex, lastIndex := Page(pageIndex, pageSize)
	holderVoting := NewHolderVoting()
	stateDB, err := api.getStateDB()
	if err != nil {
		return *holderVoting, err
	}
	_, err = GetProposalInfoByHash(stateDB, proposalHash, api.chain.GetChainTailHead(params.SCAccount).Number)
	if err != nil {
		return *holderVoting, err
	}
	getPublicVoteList(stateDB, proposalHash, func(vote *PublicVote) bool {
		holderVoting.HolderCount += 1
		holderVoting.VoteList = append(holderVoting.VoteList, *vote)
		if vote.Op == VotingResultAgree {
			holderVoting.WeightAgree.Add(holderVoting.WeightAgree, vote.Value)
		} else if vote.Op == VotingResultDisagree {
			holderVoting.WeightDisagree.Add(holderVoting.WeightDisagree, vote.Value)
		}
		return true
	})
	if holderVoting.HolderCount == 0 {
		return *holderVoting, nil
	}
	holderVoting.WeightSum.Add(holderVoting.WeightAgree, holderVoting.WeightDisagree)
	if holderVoting.HolderCount < firstIndex {
		holderVoting.VoteList = nil
	}
	if holderVoting.HolderCount > lastIndex {
		holderVoting.VoteList = holderVoting.VoteList[firstIndex:lastIndex]
	} else {
		holderVoting.VoteList = holderVoting.VoteList[firstIndex:]
	}
	return *holderVoting, nil
}

// GetAllowChangeChainConfigItems 获取允许通过提案申请变更的共识参数项
func (api *SCAPI) GetAllowChangeChainConfigItems() ([]ConfigItem, error) {
	consensusConfig := api.b.ChainConfig().GetConfigs()
	ProposalConfig := api.b.ChainConfig().GetProposal()
	var configItem ConfigItem
	var listConfigItem []ConfigItem
	for k := range consensusConfig {
		configItem.Key = k
		configItem.CurrentVal = consensusConfig[k]
		if ok := ProposalConfig[k]; ok {
			configItem.IsUpdate = true
		} else {
			configItem.IsUpdate = false
		}
		listConfigItem = append(listConfigItem, configItem)
	}
	return listConfigItem, nil
}

// GetChainArbitration 获取链仲裁结果
func (api *SCAPI) GetChainArbitration(chain common.Address, number uint64) (common.Hash, error) {
	db, err := api.getStateDB()
	if err != nil {
		return common.Hash{}, err
	}
	result := GetArbitrationResult(db, chain, number)
	if result.Empty() {
		return result, errors.New("not found")
	}
	return result, nil
}

// GetProposalNextActionForAccount 获取该用户在当前提案的状态
// hash 提案ID
// address 用户
func (api *SCAPI) GetProposalNextActionForAccount(hash common.Hash, address common.Address) (ProposalOption, error) {
	option := ProposalOptionPass
	stateDB, err := api.getStateDB()
	if err != nil {
		return option, err
	}
	proposal, err := api.GetProposalDetail(hash)
	if err != nil {
		return option, err
	}
	// 提案类型
	switch proposal.Type {
	case ProposalTypeConfigChange, ProposalTypeCouncilAdd, ProposalTypeCouncilFire: // 理事提案,修改共识配置提案
		// 提案状态
		switch proposal.GetStatus() {
		case ProposalStatusInApproval: // 审批中
			if isMayCouncilAudit(stateDB, proposal.Key, address) {
				option = ProposalOptionApply
			}
		case ProposalStatusInVoting:
			if isMayPublicVote(stateDB, hash, address) {
				option = ProposalOptionVoting
			}
		case ProposalStatusPendingJudge, ProposalStatusExpired:
			if proposal.Status == ProposalStatusExpired {
				break
			}
			option = ProposalOptionFinalize
		}
	case ProposalTypeSysWitnessCampaign, ProposalTypeUserWitnessCampaign: // 见证人提案
		switch proposal.GetStatus() {
		case ProposalStatusApply: // 申请中
			// 是否参与
			if emptyCampaign(stateDB, hash, address) {
				option = ProposalOptionJoin
			} else {
				option = ProposalOptionMarkup
			}
		case ProposalStatusPendingJudge:
			option = ProposalOptionFinalize
		}
	default:
		return ProposalOptionPass, errors.New("proposal type error")
	}

	op := api.getProposalOption(address, option)
	log.Debug("proposal next action", "hash", hash,
		"addr", address, "type", proposal.Type,
		"status", proposal.Status, "real status", proposal.GetStatus(), "op", op)

	return op, nil
}

// getProposalOption 返回用户对提案的可操作状态
func (api *SCAPI) getProposalOption(address common.Address, option ProposalOption) ProposalOption {
	var t types.TransactionType
	switch option {
	case ProposalOptionApply:
		t = types.TransactionTypeProposalCouncilVote
	case ProposalOptionVoting:
		t = types.TransactionTypeProposalPublicVote
	case ProposalOptionFinalize:
		t = types.TransactionTypeProposalFinalize
	case ProposalOptionMarkup:
		t = types.TransactionTypeProposalCampaignAddMargin
	case ProposalOptionJoin:
		t = types.TransactionTypeProposalCampaignJoin
	default:
		return ProposalOptionPass
	}
	status, err := api.b.GetLastTxStatusByType(address, t)
	log.Debug("get Status", "chain", address, "status", status, "op", option, "err", err)
	if err != nil {
		return option
	}
	if status == types.TransactionStatusUnderway {
		return ProposalOptionUnderway
	}
	return option
}

// GetAbiCode 获取内置合约abi
func (api *SCAPI) GetAbiCode() (string, error) {
	return AbiCode, nil
}

// GetProposalParticipateIn 可参与议案
func (api *SCAPI) GetProposalJoin(address common.Address, pageIndex, pageSize uint64) (*types.PageList, error) {
	proposals := types.NewPageList()
	stateDB, err := api.getStateDB()
	if err != nil {
		return proposals, err
	}
	if !IsActiveCouncil(stateDB, address) {
		return proposals, ErrNotCouncil
	}
	err = iterationProposal(stateDB, api.chain.GetChainTailHead(params.SCAccount).Number, func(proposal *Proposal) bool {
		if proposal.GetStatus() == ProposalStatusInApproval && proposal.Type != ProposalTypeSysWitnessCampaign {
			if isMayCouncilAudit(stateDB, proposal.Key, address) {
				proposals.Add(*proposal)
			}
		}
		return true
	})
	return proposals.Page(pageIndex, pageSize), err
}

// GetProposalParticipateIn 已参与议案
func (api *SCAPI) GetProposalHaveJoin(address common.Address, pageIndex, pageSize uint64) (*types.PageList, error) {
	proposals := types.NewPageList()
	stateDB, err := api.getStateDB()
	if err != nil {
		return proposals, err
	}
	err = iterationProposal(stateDB, api.chain.GetChainTailHead(params.SCAccount).Number, func(proposal *Proposal) bool {
		err = GetProposalCouncilVote(stateDB, proposal.Key, func(vote *CouncilVote) bool {
			if vote.Addr == address && vote.Op != VotingResultUnknown {
				proposals.Add(proposal)
				return false
			}
			return true
		})
		if err != nil && err != ErrNotFindProposalCouncilVote {
			return false
		}
		return true
	})
	return proposals.Page(pageIndex, pageSize), err
}

// GetCampaignList 获取竞选列表
func (api *SCAPI) GetCampaignList(proposalHash common.Hash, pageIndex, pageSize uint64) (*types.PageList, error) {
	list := types.NewPageList()
	stateDB, err := api.getStateDB()
	if err != nil {
		return list, err
	}
	proposal, err := GetProposalInfoByHash(stateDB, proposalHash, api.chain.GetChainTailHead(params.SCAccount).Number)
	if err != nil {
		return list, err
	}
	if !(proposal.Type == ProposalTypeSysWitnessCampaign || proposal.Type == ProposalTypeUserWitnessCampaign) {
		return list, errors.New("proposal type is not right")
	}
	var listCampaign CampaignList
	err = getProposalCampaignList(stateDB, proposalHash, func(campaign *Campaign) bool {
		listCampaign = append(listCampaign, *campaign)
		return true
	})
	if err != nil {
		return list, err
	}
	if listCampaign.Len() == 0 {
		return list, nil
	}
	sort.Sort(listCampaign)
	for _, v := range listCampaign {
		list.Add(v)
	}
	return list.Page(pageIndex, pageSize), nil
}

type ResultCampaign struct {
	Ranking uint64
	Margin  uint64
}

// GetCampaignRanking 获取竞选名次
func (api *SCAPI) GetCampaignRanking(proposalHash common.Hash, address common.Address) (ResultCampaign, error) {
	stateDB, err := api.getStateDB()
	if err != nil {
		return ResultCampaign{}, err
	}
	proposal, err := GetProposalInfoByHash(stateDB, proposalHash, api.chain.GetChainTailHead(params.SCAccount).Number)
	if err != nil {
		return ResultCampaign{}, err
	}
	if !(proposal.Type == ProposalTypeSysWitnessCampaign || proposal.Type == ProposalTypeUserWitnessCampaign) {
		return ResultCampaign{}, errors.New("proposal type is not right")
	}
	if emptyCampaign(stateDB, proposalHash, address) {
		return ResultCampaign{}, nil
	}

	var listCampaign CampaignList
	err = getProposalCampaignList(stateDB, proposalHash, func(campaign *Campaign) bool {
		listCampaign = append(listCampaign, *campaign)
		return true
	})
	if err != nil {
		return ResultCampaign{}, err
	}
	if listCampaign.Len() == 0 {
		return ResultCampaign{}, nil
	}
	listCampaign.Sort(true)
	for k, v := range listCampaign {
		if v.Address == address {
			return ResultCampaign{
				Ranking: uint64(k + 1),
				Margin:  v.Margin,
			}, nil
		}
	}
	return ResultCampaign{}, ErrNotCampaign
}

type AccountRoleInfo struct {
	Role   types.AccountRole `json:"role"`
	Active bool              `json:"active"`
}

func GetAccountStatus(db StateDB, addr common.Address) AccountRoleInfo {
	if addr.IsContract() {
		//如果是合约，则判断是否是以存在的合约地址
		return AccountRoleInfo{
			Active: addr == params.SCAccount || ExistContractAddress(db, addr),
			Role:   types.AccContract,
		}
	}
	//再失败是否是见证人，见证人分系统主见证人和备选见证人以及用户见证人
	status, isWitness := GetWitnessStatus(db, addr)
	if isWitness {
		info := AccountRoleInfo{
			Active: status == WitnessNormal,
		}
		index, err := GetWitnessGroupIndex(db, addr)
		if err != nil {
			info.Role = types.AccNormal
		} else if index == DefSysGroupIndex {
			if IsMainSystemWitness(db, addr) {
				info.Role = types.AccMainSystemWitness
			} else {
				info.Role = types.AccSecondSystemWitness
			}
		} else {
			info.Role = types.AccUserWitness
		}
		return info
	}
	//判断是否是理事
	s2, isCouncil := GetCouncilStatus(db, addr)
	if isCouncil {
		return AccountRoleInfo{
			Active: s2 == CouncilStatusValid,
			Role:   types.AccCouncil,
		}
	}
	_, err := GetGroupIndexFromUser(db, addr)
	return AccountRoleInfo{
		Active: err == nil,
		Role:   types.AccNormal,
	}
}

// 批量获取账户状态，一次性最多只能查询 200 个地址信息
func (api *SCAPI) GetAccountStatus(accounts []common.Address) (map[common.Address]AccountRoleInfo, error) {
	if len(accounts) == 0 {
		return nil, nil
	}
	if len(accounts) > 200 {
		return nil, errors.New("just can search 200 account status")
	}
	db, err := api.getStateDB()
	if err != nil {
		return nil, err
	}
	info := make(map[common.Address]AccountRoleInfo)
	for _, a := range accounts {
		if _, ok := info[a]; !ok {
			info[a] = GetAccountStatus(db, a)
		}
	}
	return info, nil
}

type RPCWitnessInfo struct {
	Witness common.Address `json:"witness"`
	Margin  hexutil.Uint64 `json:"margin"`
}
type RPCWitnessGroupInfo struct {
	Group   uint64           `json:"group"`
	Witness []RPCWitnessInfo `json:"witness"`
}

// 获取见证组信息，
// 通过一个见证人地址获取同组见证人信息
func (api *SCAPI) GetWitnessGroupInfoByAddr(witness common.Address) (*RPCWitnessGroupInfo, error) {
	db, err := api.chain.GetChainTailState(params.SCAccount)
	if err != nil {
		return nil, err
	}

	var result RPCWitnessGroupInfo

	info, err := GetWitnessInfo(db, witness)
	if err != nil {
		return nil, err
	}
	if info.Status != WitnessNormal {
		return nil, fmt.Errorf("witness status is %s", info.Status)
	}
	result.Group = info.GroupIndex

	list := GetWitnessListAt(db, info.GroupIndex)
	for _, w := range list {
		winfo, err := GetWitnessInfo(db, w)
		if err != nil {
			return nil, err
		}
		result.Witness = append(result.Witness, RPCWitnessInfo{Witness: w, Margin: hexutil.Uint64(winfo.Margin)})
	}
	return &result, nil
}

func (api *SCAPI) QueryAdditional(addr common.Address, period uint64) (interface{}, error) {
	last := api.chain.GetChainTailHead(params.SCAccount)

	//period
	//计算出周期对应的最后一个单元高度
	minNumber, maxNumber := GetPeriodRange(period)
	if last.Number < maxNumber {
		return nil, errors.New("the period has not yet arrived")
	}
	sdb, err := api.chain.GetStateByNumber(params.SCAccount, maxNumber)
	if err != nil {
		return nil, err
	}

	cotr := newDefaultAddController(sdb, api.chain, maxNumber+1)
	total, reward, err := cotr.calcPeriodBalance2(addr, period)
	if err != nil {
		return nil, err
	}
	report := api.chain.GetWitnessReport(addr, period)

	toStr := func(f float64) string {
		return strconv.FormatFloat(f, 'f', 24, 64)
	}

	return map[string]interface{}{
		"address":      addr,
		"period":       period,
		"begin":        minNumber,
		"end":          maxNumber,
		"total":        toStr(total),
		"reward":       toStr(reward),
		"interest":     toStr(total - reward),
		"fee":          report.Fee,
		"votes":        report.Votes,
		"create_uints": report.CreateUnits,
		"should_votes": report.ShouldVote,
	}, nil
}
