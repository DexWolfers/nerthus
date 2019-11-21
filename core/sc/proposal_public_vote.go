package sc

import (
	"math/big"

	"gitee.com/nerthus/nerthus/common"
)

type (
	PublicVote struct {
		Addr  common.Address `json:"holder"`      // 地址
		Value *big.Int       `json:"weights"`     // 币龄
		Op    VotingResult   `json:"result"`      // 投票
		Time  uint64         `json:"voting_time"` // 时间
	}
	PublicVoteList []PublicVote
)

// getPublicVoteItem 获取提案公众投票DB前缀
func getPublicVoteItem(proposalHash common.Hash) SubItems {
	return SubItems([]interface{}{publicVoteTag, proposalHash})
}

// writePublicVoteCount 写入提案公众投票数
func writePublicVoteCount(db StateDB, proposalHash common.Hash, count uint64) {
	item := getPublicVoteItem(proposalHash)
	item.SaveSub(db, nil, count)
}

// getPublicVoteCount 获取提案公众投票数
func getPublicVoteCount(db StateDB, proposalHash common.Hash) uint64 {
	item := getPublicVoteItem(proposalHash)
	hash := item.GetSub(db, nil)
	if hash.Empty() {
		return 0
	}
	return hash.Big().Uint64()
}

// writePublicVoteInfo 写入提案公众投票信息
func writePublicVoteInfo(db StateDB, proposalHash common.Hash, flag uint64, vote *PublicVote) {
	item := getPublicVoteItem(proposalHash)
	item.SaveSub(db, flag, vote.Addr)
	item = item.Join(vote.Addr)
	item.SaveSub(db, nil, flag)
	item.SaveSub(db, "value", vote.Value)
	item.SaveSub(db, "op", vote.Op)
	item.SaveSub(db, "time", vote.Time)

	AddVoteEventLog(db, proposalHash, vote.Addr, vote.Op, vote.Value)
}

// getPublicVoteInfoByAddr 根据地址获取提案公众投票信息
func getPublicVoteInfoByAddr(db StateDB, proposalHash common.Hash, address common.Address) (*PublicVote, error) {
	item := getPublicVoteItem(proposalHash).Join(address)
	if item.GetSub(db, nil).Empty() {
		return nil, ErrNotFindProposalPublicVote
	}
	var vote PublicVote
	vote.Addr = address
	vote.Value = item.GetSub(db, "value").Big()
	vote.Op = VotingResult(item.GetSub(db, "op").Big().Uint64())
	vote.Time = item.GetSub(db, "time").Big().Uint64()
	return &vote, nil
}

// getPublicVoteInfoByFlag 根据索引获取提案公众投票信息
func getPublicVoteInfoByFlag(db StateDB, proposalHash common.Hash, flag uint64) (*PublicVote, error) {
	item := getPublicVoteItem(proposalHash)
	hash := item.GetSub(db, flag)
	if hash.Empty() {
		return nil, ErrNotFindProposalPublicVote
	}
	addr := hash.Address()
	return getPublicVoteInfoByAddr(db, proposalHash, addr)
}

// isMayPublicVote 地址是否可以投票
func isMayPublicVote(db StateDB, proposalHash common.Hash, address common.Address) bool {
	item := getPublicVoteItem(proposalHash).Join(address)
	if item.GetSub(db, nil).Empty() {
		return true
	}
	return false
}

// getPublicVoteList 获取提案投票列表
func getPublicVoteList(db StateDB, proposalHash common.Hash, fun func(vote *PublicVote) bool) error {
	count := getPublicVoteCount(db, proposalHash)
	if count == 0 {
		return ErrNotFindProposalPublicVote
	}
	for i := uint64(1); i <= count; i++ {
		vote, err := getPublicVoteInfoByFlag(db, proposalHash, i)
		if err != nil {
			return err
		}
		if !fun(vote) {
			return nil
		}
	}
	return nil
}

// addPublicVote 添加提案公众投票
func addPublicVote(db StateDB, proposalHash common.Hash, vote *PublicVote) {
	count := getPublicVoteCount(db, proposalHash)
	count++
	writePublicVoteInfo(db, proposalHash, count, vote)
	writePublicVoteCount(db, proposalHash, count)
}
