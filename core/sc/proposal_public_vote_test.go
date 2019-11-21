package sc

import (
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common/math"

	"github.com/smartystreets/goconvey/convey"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/state"

	"gitee.com/nerthus/nerthus/ntsdb"
	"github.com/stretchr/testify/require"
)

func TestProposalPublicVote(t *testing.T) {
	baseDB, err := ntsdb.NewMemDatabase()
	require.Nil(t, err)
	stateDB, err := state.New([]byte{1, 2, 3}, common.Hash{}, state.NewDatabase(baseDB))
	require.Nil(t, err)
	proposalHash := common.StringToHash("proposal hash")
	var publicVoteList PublicVoteList
	for i := 0; i < 10; i++ {
		publicVoteList = append(publicVoteList, PublicVote{
			Addr:  common.StringToAddress(string(i + 1)),
			Value: math.MaxBig256,
			Op:    VotingResultAgree,
			Time:  uint64(time.Now().Unix()),
		})
	}
	convey.Convey("empty list", t, func() {
		require.Equal(t, getPublicVoteCount(stateDB, proposalHash), uint64(0))
		_, err := getPublicVoteInfoByAddr(stateDB, proposalHash, publicVoteList[0].Addr)
		require.EqualError(t, err, ErrNotFindProposalPublicVote.Error())
		err = getPublicVoteList(stateDB, proposalHash, func(vote *PublicVote) bool {
			return false
		})
		require.EqualError(t, err, ErrNotFindProposalPublicVote.Error())
	})
	convey.Convey("add public vote", t, func() {
		for _, v := range publicVoteList {
			addPublicVote(stateDB, proposalHash, &v)
		}
		require.Equal(t, getPublicVoteCount(stateDB, proposalHash), uint64(len(publicVoteList)))
		var i int
		err := getPublicVoteList(stateDB, proposalHash, func(vote *PublicVote) bool {
			require.Equal(t, vote.Addr, publicVoteList[i].Addr)
			require.Equal(t, vote.Op, publicVoteList[i].Op)
			require.Equal(t, vote.Time, publicVoteList[i].Time)
			require.Equal(t, vote.Value, publicVoteList[i].Value)
			i++
			return true
		})
		require.Nil(t, err)
		for _, v := range publicVoteList {
			require.False(t, isMayPublicVote(stateDB, proposalHash, v.Addr))
		}
		require.True(t, isMayPublicVote(stateDB, proposalHash, common.StringToAddress("other addr")))
	})
}

func BenchmarkProposalPublicVote(b *testing.B) {
	baseDB, _ := ntsdb.NewMemDatabase()
	stateDB, _ := state.New([]byte{1, 2, 3}, common.Hash{}, state.NewDatabase(baseDB))
	proposalHash := common.StringToHash("hash")
	addr := common.StringToAddress("Addr")
	max64 := math.MaxBig256
	public := PublicVote{
		Addr:  addr,
		Value: max64,
		Op:    VotingResultAgree,
		Time:  math.MaxUint64,
	}
	for i := 0; i <= b.N; i++ {
		addPublicVote(stateDB, proposalHash, &public)
	}
}
