package validtor

import (
	"errors"
	"fmt"

	"gitee.com/nerthus/nerthus/core/state"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/params"
)

// 检查作恶证据是否合法
func CheckProofWithState(signer types.Signer, rep []types.AffectedUnit,
	getUnitState func(uhash common.Hash) (*state.StateDB, error),
	checkWitnessStatus func(db *state.StateDB, address common.Address) error) ([]common.Address, error) {

	if len(rep) <= 1 {
		return nil, errors.New("the report info must more")
	}

	var (
		mc         = rep[0].Header.MC
		number     = rep[0].Header.Number
		hashs      = make(map[common.Hash]struct{})
		badWitness = make(map[common.Address]int, len(rep))
	)

	if mc.Empty() {
		return nil, errors.New("invalid empty chain address")
	}
	if number == 0 {
		return nil, errors.New("invalid unit number")
	}

	// 如果为了严格失败作恶，则可以考虑校验头数据的合法性
	// 但此时不需要如此严格校验，因为每个节点数据不一样，将导致校验不完全一致。
	// 只需要考虑见证人身份
	for _, u := range rep {
		if len(u.Votes) < params.GetChainMinVoting(mc) {
			return nil, fmt.Errorf("can not prove dishonest,bad unit's votes is not enough,expect >%d,actually %d", params.GetChainMinVoting(mc), len(u.Votes))
		}
		if len(u.Votes) > params.GetChainWitnessCount(mc) {
			return nil, errors.New("vote list is too big")
		}

		// 校验是否是在同一个高度
		if u.Header.MC.Empty() {
			return nil, errors.New("empty chain address")
		}
		if u.Header.MC != mc {
			return nil, errors.New("two units is not at the same chain")
		}
		if u.Header.Number != number {
			return nil, errors.New("two units is not at the same number")
		}
		if u.Header.SCHash.Empty() {
			return nil, errors.New("invalid header sc hash")
		}

		//不允许两个单元一样
		h := u.Header.Hash()
		if _, ok := hashs[h]; ok {
			return nil, errors.New("two units is same")
		} else {
			hashs[h] = struct{}{}
		}

		//cr.GetChainTailHead()

		//1. 校验
		statedb, err := getUnitState(u.Header.SCHash)
		if err != nil {
			return nil, err
		}

		senders := make(map[common.Address]struct{}, len(u.Votes))
		for i := 0; i < len(u.Votes); i++ {
			msg := u.Votes[i].ToVote(h)

			badSender, err := types.Sender(signer, &msg)
			if err != nil {
				return nil, err
			}
			if _, ok := senders[badSender]; ok {
				return nil, errors.New("repeat voting information")
			}
			if checkWitnessStatus != nil {
				if err := checkWitnessStatus(statedb, badSender); err != nil {
					return nil, err
				}
			}
			senders[badSender] = struct{}{}
			badWitness[badSender]++
		}
	}
	//必须保证BadWintess 的投票次数超过1
	bads := make([]common.Address, 0, len(badWitness))
	for k := range badWitness {
		bads = append(bads, k)
	}
	if len(bads) == 0 {
		return nil, errors.New("can not prove dishonest")
	}
	return bads, nil
}
