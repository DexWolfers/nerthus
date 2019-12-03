package rwit

import (
	"crypto/ecdsa"
	"math/big"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/params"

	"github.com/stretchr/testify/require"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/crypto"
)

func TestChainData(t *testing.T) {

	me, mekey := createAcct(t)
	data := newChainDataInfo(testSigner)

	job := Job{
		Chain:         common.StringToAddress("chainA"),
		SysUnitHash:   common.StringToHash("uint100"),
		SysUnitNumber: 100,
	}

	data.Reset(job)

	t.Run("reset", func(t *testing.T) {
		job := Job{
			Chain:         common.StringToAddress("chainA"),
			SysUnitHash:   common.StringToHash("uint101"),
			SysUnitNumber: 101,
		}

		data.Reset(job)

		require.Equal(t, job, data.Job())
		require.Equal(t, 0, data.VotesLen())
		require.Equal(t, common.Hash{}, data.VoteHash())
		require.False(t, data.VoteEnough())
	})

	t.Run("addVote", func(t *testing.T) {
		myChoose := types.UnitID{
			ChainID: job.Chain,
			Height:  5,
			Hash:    common.StringToHash("chainUnit5"),
		}
		data.ResetMyVote(me, createVote(t, job, myChoose, mekey))

		t.Run("witnessVotes", func(t *testing.T) {

			//不相同的见证人投票不会被加入
			t.Run("invalid", func(t *testing.T) {
				curr := data.VotesLen()

				one, onekey := createAcct(t)
				choose := types.UnitID{
					ChainID: job.Chain,
					Height:  6,
					Hash:    common.StringToHash("chainUnit6"),
				}

				err := data.AddVote(one, createVote(t, job, choose, onekey))
				require.Error(t, err)
				//投票数量不会变化
				require.Equal(t, curr, data.VotesLen())
			})

			t.Run("valid", func(t *testing.T) {
				curr := data.VotesLen()

				count := params.GetChainMinVoting(params.SCAccount)

				for i := 0; i < count; i++ {
					one, onekey := createAcct(t)
					err := data.AddVote(one, createVote(t, job, myChoose, onekey))
					require.NoError(t, err)
				}
				//投票数增加
				require.Equal(t, curr+count, data.VotesLen())
			})

		})

	})
}

func TestChainDataSet(t *testing.T) {

	set := NewVoteSet(testSigner)

	chain := common.StringToAddress("chainA")
	set.LoadOrStore(chain)
	require.True(t, set.Contains(chain))

	_, ok := set.GetChainData(chain)
	require.True(t, ok)

	set.Remove(chain)
	require.False(t, set.Contains(chain))

}

var testSigner = types.NewSigner(big.NewInt(101))

func createAcct(t *testing.T) (common.Address, *ecdsa.PrivateKey) {
	k, err := crypto.GenerateKey()
	require.NoError(t, err)
	return crypto.ForceParsePubKeyToAddress(k.PublicKey), k
}

func createVote(t *testing.T, job Job, choose types.UnitID, key *ecdsa.PrivateKey) *types.ChooseVote {
	vote := ChooseVote{
		Chain:       job.Chain,
		ApplyNumber: job.SysUnitNumber,
		ApplyUHash:  job.SysUnitHash,
		Choose:      choose,
		CreateAt:    time.Now(),
	}

	err := types.SignBySignHelper(&vote, testSigner, key)
	require.NoError(t, err)

	return &vote
}
