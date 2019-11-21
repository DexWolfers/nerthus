// Author: @kulics
package core

import (
	"math/big"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/tracetime"
	"gitee.com/nerthus/nerthus/params"
)

// GetBalance 获取账户金额
func (self *DagChain) GetBalance(addr common.Address) *big.Int {
	defer tracetime.New().Stop()
	b, err := self.dag.GetBalanceByAddress(addr)
	if err != nil {
		return common.Big0
	}
	return b
}

// GetBalanceByAddress 获取某个地址的余额
func (self *DAG) GetBalanceByAddress(mcaccount common.Address) (*big.Int, error) {
	stateDB, err := self.dagchain.GetChainTailState(mcaccount)
	if err != nil {
		return nil, err
	}
	return stateDB.GetBalance(mcaccount), nil
}

// GetTransactionStatus 根据交易hash迭代交易状态详情
func (self *DAG) GetTransactionStatus(txHash common.Hash, fun func(info TxStatusInfo) (bool, error)) error {
	return GetTransactionStatus(self.db, txHash, fun)
}

// GetLastTransactionStatus 根据交易hash获取最后一个交易状态详情
func (self *DAG) GetLastTransactionStatus(txHash common.Hash) (TxStatusInfo, error) {
	return GetLastTransactionStatus(self.db, txHash)
}

// GetSystemConfig 获取系统配置
func (self *DAG) GetSystemConfig(key []byte) (uint64, error) {
	db, err := self.dagchain.GetChainTailState(params.SCAccount)
	if err != nil {
		return 0, err
	}
	b, err := db.GetBigData(params.SCAccount, key)
	if err != nil {
		return 0, err
	}
	return common.BytesToHash(b).Big().Uint64(), nil
}
