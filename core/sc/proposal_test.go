package sc

import (
	"math/big"
	"testing"

	"gitee.com/nerthus/nerthus/core/vm"

	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/params"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/ntsdb"
	"github.com/stretchr/testify/require"
)

//测试场景：测试踢出理事不需要理事会审批，直接可以进行公投
func TestRemoveCouncil(t *testing.T) {
	ctx, _ := newTestContext(t, params.SCAccount, params.TestChainConfig)
	ctx.ctx = vm.Context{
		UnitNumber: new(big.Int).SetUint64(1000),
		Time:       new(big.Int).SetUint64(1552551879376869500),
		Transfer: func(db vm.StateDB, address common.Address, address2 common.Address, amount *big.Int) {
			db.AddBalance(address2, amount)
		},
	}
	list, err := GetCouncilList(ctx.state)
	require.NoError(t, err)

	contract := Contract{caller: common.StringToAddress("anyone"), gaspool: math.MaxBig256}
	id, err := SetMemberRemoveApply(ctx, &contract, list[0])
	require.NoError(t, err) //任何人均可以申请踢出理事

	p := getProposal(t, id, ctx.State(), ctx.ctx.UnitNumber.Uint64()+1)
	//状态必须为投票中
	require.Equal(t, ProposalStatusInVoting, p.GetStatus(), "should be voting")
	//现在便可以进行投票
	require.Equal(t, p.NumberApply, p.NumberVote, "should be start vote at next number")
	//投票截止需正确
	voteStopNumber := p.NumberApply + params.ConfigParamsTimeVoting
	require.Equal(t, voteStopNumber, p.NumberFinalize, "should be have right vote stop number")

	//如果已过投票期，则状态为待定案
	p2 := getProposal(t, id, ctx.State(), p.NumberFinalize)
	require.Equal(t, ProposalStatusPendingJudge, p2.GetStatus(), "should be end vote")

	justForTestGetCoin = func(ctx vm.Context, from common.Address, numberVote uint64) (uint64, error) {
		return 100000, nil
	}
	_, err = handlePublicVote(ctx, &contract, p.Key, true)
	require.NoError(t, err)
}

//测试场景：测试申请加入理事会，不需要审批即可进行公众投票
func TestSetMemberJoinApply(t *testing.T) {
	ctx, _ := newTestContext(t, params.SCAccount, params.TestChainConfig)
	ctx.ctx = vm.Context{
		UnitNumber: new(big.Int).SetUint64(1000),
		Time:       new(big.Int).SetUint64(1552551879376869500),
		Transfer: func(db vm.StateDB, address common.Address, address2 common.Address, amount *big.Int) {
			db.AddBalance(address2, amount)
		},
	}
	newMemeber := common.StringToAddress("userMember")
	contract := Contract{caller: newMemeber, gaspool: math.MaxBig256, value: new(big.Int).SetUint64(params.ConfigParamsCouncilMargin)}
	id, err := SetMemberJoinApply(ctx, &contract)
	require.NoError(t, err) //任何人均可以申请加入理事会

	p := getProposal(t, id, ctx.State(), ctx.ctx.UnitNumber.Uint64()+1)
	//状态必须为投票中
	require.Equal(t, ProposalStatusInVoting, p.GetStatus(), "should be voting")
	//现在便可以进行投票
	require.Equal(t, p.NumberApply, p.NumberVote, "should be start vote at next number")
	//投票截止需正确
	voteStopNumber := p.NumberApply + params.ConfigParamsTimeVoting
	require.Equal(t, voteStopNumber, p.NumberFinalize, "should be have right vote stop number")

	//如果已过投票期，则状态为待定案
	p2 := getProposal(t, id, ctx.State(), p.NumberFinalize)
	require.Equal(t, ProposalStatusPendingJudge, p2.GetStatus(), "should be end vote")

	justForTestGetCoin = func(ctx vm.Context, from common.Address, numberVote uint64) (uint64, error) {
		return 100000, nil
	}

	contract = Contract{caller: newMemeber, gaspool: math.MaxBig256}
	_, err = handlePublicVote(ctx, &contract, p.Key, true)
	require.NoError(t, err)
}

func getProposal(t *testing.T, id []byte, state StateDB, currentNumber uint64) *Proposal {
	pid := common.BytesToHash(id)
	p, err := GetProposalInfoByHash(state, pid, currentNumber)
	//应可以成功获取到天
	require.NoError(t, err, "should be get new proposal by id")
	return p
}

func TestCounciExit(t *testing.T) {
	ctx, _ := newTestContext(t, params.SCAccount, params.TestChainConfig)
	ctx.ctx = vm.Context{
		UnitNumber: new(big.Int).SetUint64(1000),
		Time:       new(big.Int).SetUint64(1552551879376869500),
		Transfer: func(db vm.StateDB, address common.Address, address2 common.Address, amount *big.Int) {
			db.AddBalance(address2, amount)
		},
	}

	list, err := GetCouncilList(ctx.state)
	require.NoError(t, err)
	for i, v := range list {
		//理事退出
		balance := ctx.State().GetBalance(v)
		contract := Contract{caller: v, gaspool: math.MaxBig256}

		//此时不能退出
		ctx.ctx.UnitNumber.SetUint64(params.CounciExitNumber - 1)
		err := handleCouncilExit(ctx, &contract)
		require.Error(t, err)
		//直到过了保护期则可以退出
		ctx.ctx.UnitNumber.SetUint64(params.CounciExitNumber)
		err = handleCouncilExit(ctx, &contract)
		if i == len(list)-1 {
			require.Error(t, err)
		} else {
			require.NoError(t, err)

			//再次执行注销则必将失败
			contract := Contract{caller: v, gaspool: math.MaxBig256}
			err = handleCouncilExit(ctx, &contract)
			require.Error(t, err) //you have quit

			//成功退出时，返还保证金
			info, err := GetCouncilInfo(ctx.State(), v)
			require.NoError(t, err)
			//状态已注销
			require.Equal(t, CouncilStatusInvalid, info.Status)
			require.Equal(t, new(big.Int).Add(balance, new(big.Int).SetUint64(info.Margin)), ctx.State().GetBalance(v))

			_ = i
		}
	}

}

func TestSetConfigApply_MinPublicVote(t *testing.T) {
	ctx, _ := newTestContext(t, params.SCAccount, params.TestChainConfig)
	ctx.ctx = vm.Context{
		UnitNumber: new(big.Int).SetUint64(1000),
		Time:       new(big.Int).SetUint64(1552551879376869500),
	}
	addr := common.StringToAddress("addr1")
	contract := Contract{caller: addr, gaspool: math.MaxBig256}

	id, err := SetConfigApply(ctx, &contract, params.MinCouncilPvoteAgreeKey, 100)
	require.NoError(t, err)

	_ = id
	processNewProposal(t, ctx, common.BytesToHash(id))
}

func processNewProposal(t *testing.T, ctx *TestContext, id common.Hash) {

	getProposal := func(currentNumber uint64) *Proposal {
		p, err := GetProposalInfoByHash(ctx.State(), id, currentNumber)
		//应可以成功获取到天
		require.NoError(t, err, "should be get new proposal by id")
		return p
	}

	p := getProposal(ctx.ctx.SCNumber)
	//新提案应在等待理事审批
	require.Equal(t, ProposalStatusInApproval, p.GetStatus())

	//理事投票
	//获取需要参与投票的理事
	GetProposalCouncilVote(ctx.State(), id, func(vote *CouncilVote) bool {
		require.Equal(t, VotingResultUnknown, vote.Op, "should nobote voted")

		contract := Contract{caller: vote.Addr, gaspool: math.MaxBig256}
		//投票赞同
		_, err := SetConfigApplyVote(ctx, &contract, id, true)
		require.NoError(t, err)
		return true
	})
	//此时应投票通过
	p = getProposal(ctx.ctx.SCNumber + params.ConfigParamsTimeVoting)
	//新提案应在等待理事审批
	require.Equal(t, ProposalStatusInApproval, p.GetStatus())

}

func TestNewProposalRoot(t *testing.T) {

	contextInfo := vm.Context{
		UnitNumber: new(big.Int).SetUint64(1000),
		Time:       new(big.Int).SetUint64(1552551879376869500),
	}
	addr := common.StringToAddress("addr1")
	var (
		gotID   string
		gotRoot string
	)
	//重复创建，结果必须保持一致
	for i := 0; i < 3; i++ {
		ctx, _ := newTestContext(t, params.SCAccount, params.TestChainConfig)
		ctx.ctx = contextInfo

		err := AddCouncil(ctx.State(), &Council{Address: addr, Status: CouncilStatusValid, Margin: 100, ApplyHeight: 1}, 100)
		require.NoError(t, err)

		contract := Contract{caller: addr, gaspool: math.MaxBig256, value: nil}
		id, err := SetUserWitness(ctx, &contract)
		require.NoError(t, err)

		idHex := common.Bytes2Hex(id)
		if gotID != "" {
			require.Equal(t, gotID, idHex)
		}
		gotID = idHex

		root, _ := ctx.state.IntermediateRoot(common.Address{})
		if gotRoot != "" {
			require.Equal(t, gotRoot, root.Hex())
		}
		gotRoot = root.Hex()
	}
}

func TestProposal(t *testing.T) {
	baseDB, err := ntsdb.NewMemDatabase()
	require.Nil(t, err)
	stateDB, err := state.New([]byte{1, 2, 3}, common.Hash{}, state.NewDatabase(baseDB))
	require.Nil(t, err)
	require.Equal(t, getProposalCount(stateDB), uint64(0))
	var proposalList []Proposal
	for i := uint64(1); i < 10; i++ {
		proposal := Proposal{
			Key:            getProposalId(stateDB),
			Type:           ProposalTypeCouncilAdd,
			Margin:         i + 1,
			ConfigKey:      "scWitnessMinMargin",
			ConfigValue:    i + 2,
			Applicant:      common.StringToAddress("applicant" + string(i)),
			FireAddress:    common.StringToAddress("fire_addr" + string(i)),
			TimeApply:      i + 3,
			TimeVote:       i + 4,
			TimeFinalize:   i + 5,
			Status:         ProposalStatusApply,
			NumberApply:    i + 6,
			NumberFinalize: i + 7,
			NumberVote:     i + 8,
		}
		proposalList = append(proposalList, proposal)
		addProposal(stateDB, &proposal)
		require.Equal(t, getProposalCount(stateDB), uint64(i))
	}
	for k, v := range proposalList {
		proposalInfo, err := getProposalInfoByFlag(stateDB, uint64(k+1), 100)
		require.Nil(t, err)
		t.Logf("key=>%s", proposalInfo.Key.Hex())
		require.Equal(t, v.Key, proposalInfo.Key)
		require.Equal(t, v.Type, proposalInfo.Type)
		require.Equal(t, v.Margin, proposalInfo.Margin)
		require.Equal(t, v.ConfigKey, proposalInfo.ConfigKey)
		require.Equal(t, v.ConfigValue, proposalInfo.ConfigValue)
		require.Equal(t, v.Applicant, proposalInfo.Applicant)
		require.Equal(t, v.FireAddress, proposalInfo.FireAddress)
		require.Equal(t, v.TimeApply, proposalInfo.TimeApply)
		require.Equal(t, v.TimeVote, proposalInfo.TimeVote)
		require.Equal(t, v.TimeFinalize, proposalInfo.TimeFinalize)
		require.Equal(t, v.Status, proposalInfo.Status)
		require.Equal(t, v.NumberApply, proposalInfo.NumberApply)
		require.Equal(t, v.NumberFinalize, proposalInfo.NumberFinalize)
		require.Equal(t, v.NumberVote, proposalInfo.NumberVote)
	}
	var i int
	err = iterationProposal(stateDB, 200, func(proposal *Proposal) bool {
		i++
		require.Equal(t, proposal.Key, proposalList[len(proposalList)-i].Key)
		return true
	})
	require.Nil(t, err)
	require.Equal(t, i, len(proposalList))
}
