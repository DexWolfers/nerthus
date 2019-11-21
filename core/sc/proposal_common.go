package sc

import (
	"errors"
	"fmt"
	"math/big"
	"time"

	"gitee.com/nerthus/nerthus/common/math"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/params"
)

// getNSByUnitCount 根据单元数获取纳秒数
func getNSByUnitCount(count uint64) uint64 {
	return count * params.OneNumberHaveSecondBySys * uint64(time.Second)
}

// addProposalPublicVote 参与人投票
func addProposalPublicVote(stateDB vm.StateDB, contract vm.Contract, proposalHash common.Hash, address common.Address, countCoin *big.Int, time uint64, b bool) error {
	if countCoin.Sign() <= 0 {
		return errors.New("the caller have not coin")
	}
	// 消耗
	if !contract.UseGas(params.SloadGas) {
		return vm.ErrOutOfGas
	}
	// 是否可以投票
	if !isMayPublicVote(stateDB, proposalHash, address) {
		return errors.New("the target already vote, can't vote again")
	}
	// 消耗
	if !contract.UseGas(params.SloadGas) {
		return vm.ErrOutOfGas
	}
	var op VotingResult
	if b {
		op = VotingResultAgree
	} else {
		op = VotingResultDisagree
	}
	addPublicVote(stateDB, proposalHash, &PublicVote{
		Addr:  address,
		Value: countCoin,
		Op:    op,
		Time:  time,
	})
	return nil
}

// CheckMemberType 检查用户的类型（是否是 理事会成员/系统见证人/用户见证人）
func CheckMemberType(state StateDB, address common.Address) error {
	if address == params.FoundationAccount {
		return errors.New("the target is foundation account")
	}
	if address == params.PublicPrivateKeyAddr {
		return errors.New("the target is public address")
	}
	// 是否是见证人
	if ExistWitness(state, address) {
		return ErrIsNormalWitness
	}
	// 是否是理事会成员
	if ExistCounci(state, address) {
		return ErrIsCouncil
	}
	return nil
}

// 检查提案公投是否符合最低票数要求
func checkMinVote(db vm.StateDB, ptype ProposalType, pass *big.Int) bool {
	if pass.Sign() <= 0 {
		return false
	}
	//如果是参数修改类提案则要求最少投票量
	var key string
	if ptype == ProposalTypeConfigChange {
		key = params.MinConfigPvoteAgreeKey
	} else {
		key = params.MinCouncilPvoteAgreeKey
	}
	v, err := ReadConfig(db, key)
	if err != nil {
		//这不应该发生
		panic(fmt.Errorf("missing config item %q:%v", key, err))
	}
	//需要转换到 DOT
	return pass.Cmp(params.NTS2DOT(new(big.Int).SetUint64(v))) >= 0
}

// validProposalPass 提案是否通过
func validProposalPass(db vm.StateDB, p *Proposal) bool {
	pass := big.NewInt(0)
	need := big.NewInt(0)
	getPublicVoteList(db, p.Key, func(vote *PublicVote) bool {
		if vote.Op == VotingResultAgree {
			pass.Add(pass, vote.Value)
		}
		need.Add(need, vote.Value)
		return true
	})
	if need.Sign() <= 0 {
		return false
	}
	if !checkMinVote(db, p.Type, pass) {
		return false
	}

	// 计算需要票数
	// 计算1/3
	need = need.Div(need, big.NewInt(3))
	// 乘2
	need = need.Mul(need, big.NewInt(2))
	// 计算是否大于2/3
	if pass.Cmp(need) == 1 {
		return true
	}
	return false
}

// getCoin 获取币龄
var justForTestGetCoin func(ctx vm.Context, from common.Address, numberVote uint64) (uint64, error)

func getCoin(ctx vm.Context, from common.Address, numberVote uint64) (*big.Int, error) {
	//用于测试
	if justForTestGetCoin != nil {
		v, err := justForTestGetCoin(ctx, from, numberVote)
		if err != nil {
			return nil, err
		}
		return new(big.Int).SetUint64(v), nil
	}

	var countCoin *big.Int
	lastHash := ctx.Chain.GetChainTailHash(from)
	if lastHash == ctx.Chain.GenesisHash() {
		countCoin = ctx.Chain.GetBalance(from)
	} else {
		for i := 0; i < 1024; i++ {
			sourceHeader := ctx.Chain.GetHeaderByHash(lastHash)
			if sourceHeader == nil {
				return nil, errors.New("unit not found")
			}
			if sourceHeader.SCNumber < numberVote {
				actionState, err := ctx.Chain.GetUnitState(sourceHeader.Hash())
				if err != nil {
					return nil, err
				}
				countCoin = actionState.GetBalance(from)
				break
			} else {
				lastHash = sourceHeader.ParentHash
			}
		}
	}
	if countCoin.Sign() <= 0 {
		return nil, errors.New("the caller have not coin")
	}
	return countCoin, nil
}

// getRecruitingSum 获取见证人可竞选人数
var errNotRecruiting = errors.New("don't need witness join")

func getRecruitingSum(stateDB StateDB, isSys bool) uint64 {
	sysCap := GetWitnessCountAt(stateDB, DefSysGroupIndex)

	var now, max uint64
	if isSys {
		max = ForceReadConfig(stateDB, params.SysWitnessCapKey)
		now = sysCap
	} else {
		// 用户见证人人数= 当前有效见证人 - 系统见证人
		now = math.ForceSafeSub(GetActiveWitnessCount(stateDB), sysCap)
		max = ForceReadConfig(stateDB, params.ConfigParamsUCWitnessCap)
	}
	return math.ForceSafeSub(max, now)
}
