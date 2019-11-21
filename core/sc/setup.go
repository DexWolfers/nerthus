package sc

import (
	"errors"
	"fmt"
	"math/big"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/params"
)

func setupTransfer(db StateDB, from, to common.Address, value *big.Int) error {
	if db.GetBalance(from).Cmp(value) < 0 {
		return vm.ErrInsufficientBalance
	}
	db.SubBalance(from, value)
	db.AddBalance(to, value)
	return nil
}

// SetupChainConfig 生成创世配置
func SetupChainConfig(stateDB vm.StateDB, config *params.ChainConfig) (err error) {
	stateDB.SetCode(params.SCAccount, []byte{1}) //防止当不存在余额是将合约消耗问题

	for key, val := range config.GetConfigs() {
		err = WriteConfig(stateDB, key, val)
	}
	return
}

type List = struct {
	Address common.Address
	PubKey  []byte
}

// SetupSystemWitness 初始化配置系统见证人数据到DB中
func SetupSystemWitness(stateDB StateDB, lib []List) error {
	margin := params.ConfigParamsSCWitnessMinMargin
	for _, v := range lib {
		if err := setupTransfer(stateDB, v.Address, params.SCAccount, new(big.Int).SetUint64(margin)); err != nil {
			return fmt.Errorf("failed to setup system witness %s,the default margin is %d: %s",
				v.Address.Hex(), margin, err)
		}
		if _, err := applyWitness(stateDB, v.Address, margin, 0, v.PubKey, true); err != nil {
			return err
		}
	}
	return nil
}

// SetupCouncilMember 初始化配置理事会成员到DB中
func SetupCouncilMember(statedb StateDB, members []common.Address, ceiling uint64) error {
	if len(members) == 0 {
		return errors.New("no default council members")
	}
	margin := params.ConfigParamsCouncilMargin
	for _, v := range members {
		if err := setupTransfer(statedb, v, params.SCAccount, new(big.Int).SetUint64(margin)); err != nil {
			return fmt.Errorf("failed to setup council member %s,the default margin is %d: %s",
				v.Hex(), margin, err)
		}
		if err := AddCouncil(statedb, &Council{
			Address: v, Status: CouncilStatusValid,
			Margin:      margin,
			ApplyHeight: 0}, ceiling); err != nil {
			return err
		}
	}
	// 存储默认理事会成员
	return nil
}

// SetupDefaultWitnessForUser 初始化分配默认见证人给各用户
func SetupDefaultWitnessForUser(db StateDB, users ...common.Address) error {
	//为创世中的用户，设置见证人
	for _, v := range users {
		_, err := ApplyChainFromWitnessGroup(db, v, big.NewInt(0), nil, 1)
		if err != nil {
			return err
		}
	}
	return nil
}

// SetupUserWitness 初始化用户见证人
func SetupUserWitness(db StateDB, lib []List) error {
	if len(lib) < params.ConfigParamsUCWitness {
		return errors.New("normal witness less")
	}
	margin := params.ConfigParamsUCWitnessMargin
	for _, v := range lib {
		if err := setupTransfer(db, v.Address, params.SCAccount, new(big.Int).SetUint64(margin)); err != nil {
			return fmt.Errorf("failed to setup user witness %s,the default margin is %d: %s",
				v.Address.Hex(), margin, err)
		}
		if _, err := applyWitness(db, v.Address, margin, 0, v.PubKey, false); err != nil {
			return err
		}
	}
	return nil
}
