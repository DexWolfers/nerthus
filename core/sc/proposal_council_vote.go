package sc

import (
	"errors"
	"math/big"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/params"
)

type (
	CouncilVote struct {
		Addr common.Address `json:"member"` // 地址
		Op   VotingResult   `json:"result"` // 投票
		Time uint64         `json:"time"`   // 时间
	}
	CouncilVoteList []CouncilVote
)

// getCouncilVoteItem 理事投票db前缀
func getCouncilVoteItem(proposalHash common.Hash) SubItems {
	return SubItems([]interface{}{ProposalApplyTag, proposalHash})
}

// getCouncilVoteCount 获取理事审核数
func getCouncilVoteCount(db StateDB, proposalHash common.Hash) uint64 {
	item := getCouncilVoteItem(proposalHash)
	hash := item.GetSub(db, nil)
	if hash.Empty() {
		return 0
	}
	return hash.Big().Uint64()
}

// writeCouncilVoteCount 写入理事审核数
func writeCouncilVoteCount(db StateDB, proposalHash common.Hash, count uint64) {
	item := getCouncilVoteItem(proposalHash)
	item.SaveSub(db, nil, count)
}

// writeCouncilVoteInfo 写入理事审核信息
func writeCouncilVoteInfo(db StateDB, proposalHash common.Hash, vote *CouncilVote, flag uint64) {
	item := getCouncilVoteItem(proposalHash)
	item.SaveSub(db, flag, vote.Addr)
	item = item.Join(vote.Addr)
	item.SaveSub(db, nil, flag)
	item.SaveSub(db, "op", vote.Op)
	item.SaveSub(db, "time", vote.Time)

	AddVoteEventLog(db, proposalHash, vote.Addr, vote.Op, big.NewInt(0))
}

// updateCouncilVoteInfo 修改理事审核信息
func updateCouncilVoteInfo(db StateDB, proposalHash common.Hash, vote *CouncilVote) {
	item := getCouncilVoteItem(proposalHash).Join(vote.Addr)
	item.SaveSub(db, "op", vote.Op)
	item.SaveSub(db, "time", vote.Time)

	AddVoteEventLog(db, proposalHash, vote.Addr, vote.Op, big.NewInt(0))
}

// getCouncilVoteInfo 根据地址获取理事审核信息
func getCouncilVoteInfo(db StateDB, proposalHash common.Hash, address common.Address) (*CouncilVote, error) {
	item := getCouncilVoteItem(proposalHash).Join(address)
	hash := item.GetSub(db, nil)
	if hash.Empty() {
		return nil, ErrNotFindProposalCouncilVote
	}
	var councilVote CouncilVote
	councilVote.Addr = address
	councilVote.Op = VotingResult(item.GetSub(db, "op").Big().Uint64())
	councilVote.Time = item.GetSub(db, "time").Big().Uint64()
	return &councilVote, nil
}

// getCouncilVoteInfoByFlag 根据索引获取理事审核信息
func getCouncilVoteInfoByFlag(db StateDB, proposalHash common.Hash, flag uint64) (*CouncilVote, error) {
	item := getCouncilVoteItem(proposalHash)
	hash := item.GetSub(db, flag)
	if hash.Empty() {
		return nil, ErrNotFindProposalCouncilVote
	}
	return getCouncilVoteInfo(db, proposalHash, hash.Address())
}

// isMayCouncilAudit 理事是否可以对该提案审核
func isMayCouncilAudit(db StateDB, proposalHash common.Hash, address common.Address) bool {
	if !IsActiveCouncil(db, address) {
		return false
	}
	item := getCouncilVoteItem(proposalHash).Join(address)
	if item.GetSub(db, nil).Empty() {
		return false
	}
	if VotingResult(item.GetSub(db, "op").Big().Uint64()) == VotingResultUnknown {
		return true
	}
	return false
}

// ProposalIsVote 检测投案是否通过审核
func ProposalIsVote(db vm.StateDB, proposal *Proposal) error {
	var passCount, count, failedCount uint64
	err := GetProposalCouncilVote(db, proposal.Key, func(vote *CouncilVote) bool {
		switch vote.Op {
		case VotingResultAgree:
			passCount++
		case VotingResultDisagree:
			failedCount++
		}
		count++
		return true
	})
	if err != nil {
		return err
	}
	if passCount >= (count - count*1/3) {
		proposal.Status = ProposalStatusInVoting
		proposal.TimeVote = proposal.TimeApply + getNSByUnitCount(params.ConfigParamsTimeApprove)
		proposal.NumberFinalize = proposal.NumberVote + params.ConfigParamsTimeVoting
		writeProposalInfo(db, proposal)
	} else if failedCount >= count*2/3 {
		proposal.Status = ProposalStatusFailed
		writeProposalInfo(db, proposal)
	}
	return nil
}

// updateMemberProposalVote 修改理事会成员投票
func updateMemberProposalVote(db vm.StateDB, contract vm.Contract, proposal *Proposal, address common.Address, option bool, time uint64) error {
	if err := proposal.validStatus(ProposalStatusInApproval); err != nil {
		return err
	}
	var op VotingResult
	if option {
		op = VotingResultAgree
	} else {
		op = VotingResultDisagree
	}
	// 消耗
	if !contract.UseGas(params.SstoreLoadGas) {
		return vm.ErrOutOfGas
	}
	if !isMayCouncilAudit(db, proposal.Key, address) {
		return errors.New("the caller can't vote")
	}
	// 消耗
	if !contract.UseGas(params.SstoreSetGas) {
		return vm.ErrOutOfGas
	}
	updateCouncilVoteInfo(db, proposal.Key, &CouncilVote{address, op, time})
	// 消耗
	if !contract.UseGas(params.SloadGas) {
		return vm.ErrOutOfGas
	}
	// 判断审核结果
	return ProposalIsVote(db, proposal)
}

// GetProposalCouncilVote 获取理事会投票信息
func GetProposalCouncilVote(db vm.StateDB, proposalHash common.Hash, fun func(vote *CouncilVote) bool) error {
	count := getCouncilVoteCount(db, proposalHash)
	if count == 0 {
		return ErrNotFindProposalCouncilVote
	}
	var (
		vote *CouncilVote
		err  error
	)
	for i := uint64(1); i <= count; i++ {
		if vote, err = getCouncilVoteInfoByFlag(db, proposalHash, i); err != nil {
			return err
		}
		if !fun(vote) {
			return nil
		}
	}
	return nil
}
