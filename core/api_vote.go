// 投票信息存储
package core

import (
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
)

var (
	voteMsgPrefix = []byte("vote_")
)

func (self *DagChain) GetVoteMsg(hash common.Hash) (types.Votes, error) {
	return self.dag.GetVoteMsg(hash)
}

// GetVoteMsg 获取投票信息
func (self *DAG) GetVoteMsg(hash common.Hash) (types.Votes, error) {
	votes, err := GetVoteMsg(self.db, hash)
	if err != nil {
		return nil, err
	}

	var voteMsgs types.Votes
	for _, v := range votes {
		voteMsgs = append(voteMsgs, &types.VoteMsg{Extra: v.Extra, Sign: v.Sign, UnitHash: hash})
	}
	return voteMsgs, nil
}

// 获GetVoteMsgAddresses 取所有投票信息的发起者账号
func (self *DAG) GetVoteMsgAddresses(hash common.Hash) ([]common.Address, error) {
	var addrs []common.Address
	votes, err := self.GetVoteMsg(hash)
	if err != nil {
		return addrs, err
	}

	for _, v := range votes {
		addr, err := v.Sender()
		if err != nil {
			return addrs, err
		}

		addrs = append(addrs, addr)
	}

	return addrs, nil
}
