package types

import (
	"math/big"

	"gitee.com/nerthus/nerthus/common"
)

// 见证费用
type WitnessPeriodWorkInfo struct {
	IsSys       bool     `json:"-"`                 //是否是系统链
	Votes       uint64   `json:"voted_units"`       //周期内投票数
	CreateUnits uint64   `json:"created_units"`     //该周期内的作为提案者生成的单元数
	VotedTxs    uint64   `json:"voted_txs"`         //单元交易数（只涉及已投票）
	ShouldVote  uint64   `json:"should_vote_units"` //此周期内该见证人应参与投票的单元数
	Fee         *big.Int `json:"created_unit_fee"`  //统计见证费用

	VotedLastUnit common.Hash `json:"voted_last_unit"` //参与投票的最后一个单元哈希
}
