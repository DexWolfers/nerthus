package sc

import (
	"testing"

	"gitee.com/nerthus/nerthus/params"

	"math/big"

	"encoding/json"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/rlp"
	"github.com/stretchr/testify/require"
)

func TestJson(t *testing.T) {

	//type MyD struct {
	//	Status ProposalStatus
	//}

	var p Proposal
	p.Type = ProposalTypeCouncilFire
	p.Status = ProposalStatusFailed
	p.TimeApply = uint64(1232321)
	data, err := json.Marshal(p)
	require.NoError(t, err)

	t.Log(string(data))

	var got Proposal
	err = json.Unmarshal(data, &got)
	require.NoError(t, err)

	t.Log(got.Status.String())

}

func TestPubVoteRLP(t *testing.T) {
	d := PublicVote{
		Value: big.NewInt(19999999999689100),
		Op:    VotingResultAgree,
		Time:  100000,
	}
	data, err := rlp.EncodeToBytes(d)
	require.NoError(t, err)

	var got PublicVote
	err = rlp.DecodeBytes(data, &got)
	require.NoError(t, err)

	require.Equal(t, got.Time, d.Time)
	require.Equal(t, got.Addr.String(), d.Addr.String())
	require.Equal(t, got.Op, d.Op)
	require.Equal(t, got.Value.String(), d.Value.String())

	t.Log(got)
	t.Log(d)

}

func TestProposalID(t *testing.T) {

	db, _ := ntsdb.NewMemDatabase()
	stateDB, _ := state.New([]byte{1, 2, 3}, common.Hash{}, state.NewDatabase(db))
	count := getProposalCount(stateDB)
	require.Equal(t, count, uint64(0))

	key := getProposalId(stateDB)
	t.Log(key)
}

func TestProposalApply(t *testing.T) {
	db, _ := ntsdb.NewMemDatabase()
	stateDB, _ := state.New(nil, common.StringToHash(""), state.NewDatabase(db))
	proposalID := getProposalId(stateDB)

	t.Log(proposalID)
	proposal := Proposal{
		Key:            proposalID,
		Type:           ProposalTypeSysWitnessCampaign,
		Status:         ProposalStatusInVoting,
		NumberApply:    0,
		NumberVote:     100,
		NumberFinalize: 1000,
	}
	addProposal(stateDB, &proposal)

	count := getProposalCount(stateDB)
	t.Log(count)

	proposalHash := getProposalId(stateDB)

	t.Log(proposalHash)

	proposal2, err := GetProposalInfoByHash(stateDB, proposalHash, 1110)

	t.Log(proposal2.GetStatus())

	err = ProposalIsCreate(stateDB, ProposalTypeSysWitnessCampaign, 1110)
	require.Nil(t, err)

}

func TestProposalMemberVote(t *testing.T) {

	db, _ := ntsdb.NewMemDatabase()
	stateDB, _ := state.New([]byte{1, 2, 3}, common.Hash{}, state.NewDatabase(db))

	contract := &Contract{
		caller:  common.StringToAddress("aaaaa"),
		gaspool: big.NewInt(math.MaxInt64),
		value:   big.NewInt(1),
	}
	proposalId := getProposalId(stateDB)

	proposal := &Proposal{
		Key:         proposalId,
		Type:        ProposalTypeCouncilAdd,
		TimeApply:   0,
		Status:      ProposalStatusInApproval,
		NumberApply: 0,
		NumberVote:  100,
	}

	var council Council
	var councils CouncilList
	for i := 0; i < 3; i++ {
		council = Council{
			Address: common.StringToAddress("council" + string(i)),
			Status:  CouncilStatusValid,
			Margin:  math.MaxInt64,
		}
		councils = append(councils, council)
	}
	for _, v := range councils {
		err := AddCouncil(stateDB, &v, 11)
		require.Nil(t, err)
	}

	_, err := applyProposal(contract, stateDB, proposal)
	require.Nil(t, err)

	err = updateMemberProposalVote(stateDB, contract, proposal, councils[0].Address, true, 100)
	require.Nil(t, err)
	err = updateMemberProposalVote(stateDB, contract, proposal, councils[1].Address, false, 200)
	require.Nil(t, err)
	var arrMemVote CouncilVoteList
	err = GetProposalCouncilVote(stateDB, proposal.Key, func(vote *CouncilVote) bool {
		arrMemVote = append(arrMemVote, *vote)
		return true
	})
	for _, v := range arrMemVote {
		t.Log("--->>>", "addr", v.Addr.Hex(), "op", v.Op, "time", v.Time)
	}
}

func TestFixedBug(t *testing.T) {
	type P struct {
		Key   string
		Value uint64
	}
	str := "good"
	val := uint64(123456)
	b, err := abiObj.Pack("SetConfigApply", str, val)
	require.Nil(t, err)

	//t.Log(b, common.Bytes2Hex(b))
	//b := common.FromHex("0x6bd13b6400000000000000000000000000000000000000000000000000000000000000400000000000000000000000000000000000000000000000000000000035a4e900000000000000000000000000000000000000000000000000000000000000000a75634761734c696d697400000000000000000000000000000000000000000000")

	var p P
	err = abiObj.Methods["SetConfigApply"].Inputs.Unpack(&p, b[4:])
	require.Nil(t, err)
	require.Equal(t, p.Key, "good", str)
	require.Equal(t, p.Value, val)
}

func TestCheckMinVote(t *testing.T) {
	db, _ := ntsdb.NewMemDatabase()
	stateDB, _ := state.New([]byte{1, 2, 3}, common.Hash{}, state.NewDatabase(db))

	//设置最低票
	WriteConfig(stateDB, params.MinConfigPvoteAgreeKey, 10)
	WriteConfig(stateDB, params.MinCouncilPvoteAgreeKey, 100)

	caces := []struct {
		Typ      ProposalType
		Votes    *big.Int
		WantPass bool
	}{
		{ProposalTypeConfigChange, params.NTS2DOT(big.NewInt(0)), false},
		{ProposalTypeConfigChange, params.NTS2DOT(big.NewInt(5)), false},
		{ProposalTypeConfigChange, params.NTS2DOT(big.NewInt(10)), true},
		{ProposalTypeConfigChange, params.NTS2DOT(big.NewInt(11)), true},

		{ProposalTypeCouncilAdd, params.NTS2DOT(big.NewInt(0)), false},
		{ProposalTypeCouncilAdd, params.NTS2DOT(big.NewInt(50)), false},
		{ProposalTypeCouncilAdd, params.NTS2DOT(big.NewInt(99)), false},
		{ProposalTypeCouncilAdd, params.NTS2DOT(big.NewInt(100)), true},
		{ProposalTypeCouncilAdd, params.NTS2DOT(big.NewInt(101)), true},
	}

	for _, c := range caces {
		got := checkMinVote(stateDB, c.Typ, c.Votes)
		require.Equal(t, c.WantPass, got)
	}
}

func TestGetRecruitingSum(t *testing.T) {
	db, _ := ntsdb.NewMemDatabase()
	stateDB, _ := state.New([]byte{1, 2, 3}, common.Hash{}, state.NewDatabase(db))

	//设置最低票
	WriteConfig(stateDB, params.SysWitnessCapKey, 10)
	WriteConfig(stateDB, params.ConfigParamsUCWitnessCap, 100)

	need := getRecruitingSum(stateDB, true)
	require.Equal(t, int(10), int(need))

}
