package rwit

import (
	"sync"

	"gitee.com/nerthus/nerthus/params"
	"github.com/pkg/errors"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/sync/cmap"
	"gitee.com/nerthus/nerthus/core/types"
)

const defJobKey = "_job" //集合中用于标识 Job

type ChainData struct {
	job      Job
	voteHash common.Hash
	vote     *types.ChooseVote
	signm    map[common.Address][]byte

	signer types.Signer
	locker sync.RWMutex
}

func newChainDataInfo(signer types.Signer) *ChainData {
	return &ChainData{
		signer: signer,
		signm:  make(map[common.Address][]byte, params.ConfigParamsSCWitness),
	}
}

func (cd *ChainData) Reset(job Job) {
	cd.locker.Lock()
	defer cd.locker.Unlock()

	cd.job = job
	cd.voteHash = common.Hash{}
	cd.vote = nil
	for k := range cd.signm {
		delete(cd.signm, k)
	}
}

// 重置自己的投票
func (cd *ChainData) ResetMyVote(witness common.Address, vote *types.ChooseVote) {
	cd.locker.Lock()
	defer cd.locker.Unlock()

	cd.voteHash = cd.signer.Hash(vote)
	cd.vote = vote

	for k := range cd.signm {
		delete(cd.signm, k)
	}
	//保留自己的投票
	cd.signm[witness] = vote.Sign.Get()
}

//添加投票
func (cd *ChainData) AddVote(witness common.Address, vote *types.ChooseVote) error {
	voteHash, err := cd.checkVote(vote)
	if err != nil {
		return err
	}

	cd.locker.Lock()
	defer cd.locker.Unlock()

	if voteHash != cd.voteHash {
		return errors.New("vote hash mismatch")
	}
	if _, ok := cd.signm[witness]; ok {
		return errors.New("repeat vote")
	}
	cd.signm[witness] = vote.Sign.Get()
	return nil
}

// 检查投票基本信息
func (cd *ChainData) checkVote(vote *types.ChooseVote) (common.Hash, error) {

	if vote.ApplyNumber != cd.Job().SysUnitNumber {
		return common.Hash{}, errors.New("apply number mismatch")
	}
	if vote.ApplyUHash != cd.Job().SysUnitHash {
		return common.Hash{}, errors.New("apply unit hash mismatch")
	}

	hash := cd.signer.Hash(vote)
	local := cd.VoteHash()
	// 投票结果必须和本地一致
	if hash != local {
		return hash, errors.Errorf("vote hash mismatch:local=%s,remote=%s", local.String(), hash.String())
	}
	return hash, nil
}

func (cd *ChainData) Job() Job {
	cd.locker.RLock()
	defer cd.locker.RUnlock()
	return cd.job
}
func (cd *ChainData) VoteHash() common.Hash {
	cd.locker.RLock()
	defer cd.locker.RUnlock()
	return cd.voteHash
}
func (cd *ChainData) Vote() *types.ChooseVote {
	cd.locker.RLock()
	defer cd.locker.RUnlock()
	return cd.vote
}
func (cd *ChainData) VotesLen() int {
	cd.locker.RLock()
	defer cd.locker.RUnlock()
	return len(cd.signm)
}

// 投票数是否足够
func (cd *ChainData) VoteEnough() bool {
	cd.locker.RLock()
	defer cd.locker.RUnlock()
	return len(cd.signm) >= params.GetChainMinVoting(params.SCAccount)
}

// 获取全部投票
func (cd *ChainData) GetVotes() (vote *types.ChooseVote, signes map[common.Address][]byte) {
	cd.locker.RLock()
	defer cd.locker.RUnlock()

	signes = make(map[common.Address][]byte, len(cd.signm))
	for k, v := range cd.signm {
		signes[k] = v
	}
	return cd.Vote(), signes
}

// 已投票成员
func (cd *ChainData) Voters() []common.Address {
	cd.locker.RLock()
	defer cd.locker.RUnlock()

	list := make([]common.Address, 0, len(cd.signm))
	for k := range cd.signm {
		list = append(list, k)
	}
	return list
}

// 获取全部的投票签名
func (cd *ChainData) GetVoteSigns() [][]byte {
	cd.locker.RLock()
	defer cd.locker.RUnlock()

	list := make([][]byte, 0, len(cd.signm))
	for _, v := range cd.signm {
		list = append(list, v)
	}
	return list
}

type ChainDataSet struct {
	signer types.Signer
	m      cmap.ConcurrentMap
}

func NewVoteSet(signer types.Signer) *ChainDataSet {
	return &ChainDataSet{signer: signer, m: cmap.NewWith(32)}
}

func (vs ChainDataSet) Len() int {
	return vs.m.Count()
}

//ChainVotes 获取链的信息，包括投票与链信息
func (vs ChainDataSet) GetChainData(chain common.Address) (*ChainData, bool) {
	v, ok := vs.m.Get(chain.String())
	if !ok {
		return nil, ok
	}
	return v.(*ChainData), true
}
func (vs ChainDataSet) Replace(chain common.Address, job Job) {
	vs.LoadOrStore(chain).Reset(job)
}

// 加载单链的投票集
func (vs ChainDataSet) LoadOrStore(chain common.Address) *ChainData {
	chainVotes, _ := vs.m.LoadOrStore(chain.String(), func() interface{} {
		return newChainDataInfo(vs.signer)
	})
	return chainVotes.(*ChainData)
}

// 添加投票
func (vs ChainDataSet) AddVote(witness common.Address, vote *types.ChooseVote) error {
	info, ok := vs.GetChainData(vote.Chain)
	if !ok {
		return errors.New("chain is not working")
	}
	return info.AddVote(witness, vote)
}

func (vs ChainDataSet) Remove(chain common.Address) {
	vs.m.Remove(chain.String())
}
func (vs ChainDataSet) Reset(chain common.Address) {
	info, ok := vs.GetChainData(chain)
	if !ok {
		return
	}
	info.Reset(Job{})
}

func (vs ChainDataSet) Contains(chain common.Address) bool {
	return vs.m.Has(chain.String())
}

func (vs ChainDataSet) Chains() []common.Address {
	list := vs.m.Keys()
	chains := make([]common.Address, len(list))
	for i, v := range list {
		chains[i] = common.ForceDecodeAddress(v)
	}
	return chains
}
