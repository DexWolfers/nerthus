package sc

import (
	"fmt"
	"math/big"
	"sort"
	"testing"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/hexutil"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/core/vm"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/params"
	"gitee.com/nerthus/nerthus/rlp"
	"github.com/stretchr/testify/require"
)

func TestAddWitnessGroupChangedEvent(t *testing.T) {
	db := newTestState()

	w := common.StringToAddress("witness")
	oldGp, newGp := uint64(6), uint64(10)

	err := AddWitnessGroupChangedEvent(db, w, oldGp, newGp)
	require.NoError(t, err)
	require.Len(t, db.Logs(), 1)

	w2, old2, new2, err := UnpackWitnessGroupChangedEvent(db.Logs()[0].Data)
	require.NoError(t, err)
	require.Equal(t, w, w2)
	require.Equal(t, oldGp, old2)
	require.Equal(t, newGp, new2)
}

func TestAddChainStatusChangedLog(t *testing.T) {
	db := newTestState()

	info := ChainStatusChanged{
		Chain:  common.StringToAddress("AAA"),
		Status: ChainStatusWitnessReplaceUnderway,
		Group:  math.MaxUint16,
		First:  true,
	}

	err := AddChainStatusChangedLog(db, info.Chain, info.Status, uint16(info.Group), info.First)
	require.NoError(t, err)
	require.Len(t, db.Logs(), 1)

	got, err := UnpackChainStatusChangedEvent(db.Logs()[0].Data)
	require.NoError(t, err)
	require.Equal(t, info, got)
}

func TestAddChainArbitrationEventLog(t *testing.T) {
	db := newTestState()

	chain, number := common.StringToAddress("AAA"), math.MaxUint64
	AddChainArbitrationEventLog(db, chain, number)
	require.Len(t, db.Logs(), 1)

	gotChain, gotNumber, err := UnpackChainArbitrationEventLog(db.Logs()[0].Data)
	require.NoError(t, err)
	require.Equal(t, chain, gotChain)
	require.Equal(t, number, gotNumber)
}

func TestAddChainArbitrationEndEventLog(t *testing.T) {
	db := newTestState()

	chain, number, uhash := common.StringToAddress("AAA"), math.MaxUint64, common.HexToHash("0x779a29e9baa36434497f68c79536b672d929997eeebcabeff18e137dfe9b817f")
	AddChainArbitrationEndEventLog(db, chain, number, uhash)
	require.Len(t, db.Logs(), 1)

	got, err := UnpackChainArbitrationEndEventLog(db.Logs()[0].Data)
	require.NoError(t, err)
	require.Equal(t, chain, got.ChainID)
	require.Equal(t, number, got.Height)
	require.Equal(t, uhash, got.Hash)
}

func TestAddProposalVoteLog(t *testing.T) {
	db := newTestState()

	info := ProposalVoteLog{
		Id:    getProposalId(db),
		Voter: common.ForceDecodeAddress("nts1gjxw3wtnxg49scvvru0qz5dulpcs3q4fv9t52u"),
		Op:    VotingResultAgree,
		Value: math.MaxBig256,
	}

	err := AddVoteEventLog(db, info.Id, info.Voter, info.Op, info.Value)
	require.NoError(t, err)
	require.Len(t, db.Logs(), 1)

	got, err := UnpackVoteLog(db.Logs()[0].Data)
	require.NoError(t, err)
	require.Equal(t, info, got)

}

func TestAddProposalChangeEventLog(t *testing.T) {
	db := newTestState()
	proposal := Proposal{
		Key:            getProposalId(db),
		Type:           ProposalTypeCouncilAdd,
		Margin:         1,
		ConfigKey:      "scWitnessMinMargin",
		ConfigValue:    2,
		Applicant:      common.StringToAddress("applicant" + string(1)),
		FireAddress:    common.StringToAddress("fire_addr" + string(1)),
		TimeApply:      3,
		TimeVote:       4,
		TimeFinalize:   5,
		Status:         ProposalStatusApply,
		NumberApply:    6,
		NumberFinalize: 7,
		NumberVote:     8,
	}

	err := AddProposalChangeEventLog(db, &proposal)
	require.NoError(t, err)
	require.Len(t, db.Logs(), 1)

	event := make(map[string]interface{})
	err = abiObj.Events[ProposalChangeEvent].Inputs.Unpack(&event, db.Logs()[0].Data)
	require.NoError(t, err)
	require.Equal(t, proposal.ConfigKey, string(event["item"].(hexutil.Bytes)))

	got, err := UnpackProposalChangeLog(db.Logs()[0].Data)
	require.NoError(t, err)
	require.Equal(t, ProposalChangeLog{
		Id:             proposal.Key,
		Type:           proposal.Type,
		Item:           proposal.ConfigKey,
		Value:          proposal.ConfigValue,
		Status:         proposal.Status,
		ApplyNumber:    proposal.NumberApply,
		FinalizeNumber: proposal.NumberFinalize,
		VoteNumber:     proposal.NumberVote,
	}, got)

}

func TestAddWitnessChangeEventLog(t *testing.T) {
	db := newTestState()

	witness := WitnessInfo{
		Address:    common.ForceDecodeAddress("nts1gjxw3wtnxg49scvvru0qz5dulpcs3q4fv9t52u"),
		Status:     WitnessInBlackList,
		Margin:     math.MaxUint64,
		GroupIndex: math.MaxUint16,
		Index:      11,
	}
	err := AddWitnessChangeEventLog(db, &witness)
	require.NoError(t, err)
	require.Len(t, db.Logs(), 1)
	//解析
	logInfo := db.Logs()[0]

	event, err := UnpackWitnessChangeLog(logInfo.Data)
	require.NoError(t, err)
	require.Equal(t, witness.Address, event.Witness)
	require.Equal(t, witness.Status, event.Status)
	require.Equal(t, witness.Margin, event.Margin)
	require.Equal(t, uint16(witness.GroupIndex), event.Group)
}

func TestAddUserWitnessChangeEventLog(t *testing.T) {
	db := newTestState()
	err := AddUserWitnessChangeEventLog(db, common.ForceDecodeAddress("nts1gjxw3wtnxg49scvvru0qz5dulpcs3q4fv9t52u"), 0, 101)
	require.NoError(t, err)
	require.Len(t, db.Logs(), 1)
	//解析
	logInfo := db.Logs()[0]

	event, err := UnpackUserWitnessChangeLog(logInfo.Data)
	require.NoError(t, err)
	require.Equal(t, common.ForceDecodeAddress("nts1gjxw3wtnxg49scvvru0qz5dulpcs3q4fv9t52u"), event.User)
	require.Equal(t, uint16(0), event.OldWitnessGroup)
	require.Equal(t, uint16(101), event.NewWitnessGroup)
}

func TestListPeriodCount(t *testing.T) {
	type args struct {
		addr  common.Address
		count uint64
	}
	var tests []args
	for i := int64(1); i <= 100; i++ {
		tests = append(tests, args{addr: common.BigToAddress(big.NewInt(i)), count: 10})
	}

	var listAR ListAdditionalRecord
	for _, tt := range tests {
		listAR = listAR.AddRecord(tt.addr, tt.count)
	}
	require.Len(t, listAR, 100)

	t.Run("SetPeriodCount", func(t *testing.T) {
		var listPeriodCount ListPeriodCount
		listPeriodCount = listPeriodCount.SetPeriodCount(1, listAR)
		require.Len(t, listPeriodCount, 1)

		listPeriodCount = listPeriodCount.SetPeriodCount(2, listAR)
		require.Len(t, listPeriodCount, 2)
	})

	t.Run("Query", func(t *testing.T) {
		var listPeriodCount ListPeriodCount
		listPeriodCount = listPeriodCount.SetPeriodCount(1, listAR)

		// has exist
		{
			result := listPeriodCount.Query(1)
			require.Equal(t, result.Period, uint64(1))
			require.Len(t, result.Records, len(listAR))
			for _, record := range result.Records {
				_record := listAR.Query(record.Addr)
				require.False(t, (&_record).Addr.Empty())
			}
		}

		// not found
		{
			result := listPeriodCount.Query(2)
			require.Zero(t, result.Period)
			require.Empty(t, result.Records)
		}
	})

	t.Run("sort", func(t *testing.T) {
		var listPeriodCount ListPeriodCount
		var args = []uint64{2, 3, 4, 1, 8, 5, 9, 7, 6, 10}
		for _, arg := range args {
			listPeriodCount = listPeriodCount.SetPeriodCount(arg, listAR)
		}
		sort.Sort(listPeriodCount)
		for i := uint64(1); i <= 10; i++ {
			require.Equal(t, i, listPeriodCount[i-1].Period)
			require.Len(t, listPeriodCount[i-1].Records, len(listAR))
		}
	})

	t.Run("ResetPeriodCount", func(t *testing.T) {
		var listPeriodCount ListPeriodCount
		var args = []uint64{2, 3, 4, 1, 8, 5, 9, 7, 6, 10}
		for _, arg := range args {
			listARCopy := make(ListAdditionalRecord, len(listAR))
			copy(listARCopy, listAR)
			listPeriodCount = listPeriodCount.SetPeriodCount(arg, listARCopy)
		}
		newRecord := AdditionalRecord{Addr: listAR[0].Addr, Count: listAR[0].Count * 2, Spend: !listAR[0].Spend}
		listPeriodCount.ResetPeriodCount(2, newRecord)

		record := listPeriodCount.Query(2).Records.Query(listAR[0].Addr)
		require.Equal(t, listAR[0].Count*2, record.Count)
		require.Equal(t, listAR[0].Addr, record.Addr)
		require.Equal(t, !listAR[0].Spend, record.Spend)
	})

	t.Run("AddAdditional", func(t *testing.T) {
		var listPeriodCount ListPeriodCount
		for _, ar := range listAR {
			listPeriodCount = listPeriodCount.AddAdditional(1, ar.Addr, ar.Count)
		}
		require.Empty(t, listPeriodCount)

		listARCopy := make(ListAdditionalRecord, len(listAR))
		copy(listARCopy, listAR)
		listPeriodCount = listPeriodCount.SetPeriodCount(1, listARCopy)

		for _, ar := range listAR {
			listPeriodCount = listPeriodCount.AddAdditional(1, ar.Addr, ar.Count)
		}

		for _, ar := range listAR {
			_record := listPeriodCount.Query(1).Records.Query(ar.Addr)
			require.Equal(t, ar.Count*2, _record.Count)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		var listPeriodCount ListPeriodCount
		listARCopy := make(ListAdditionalRecord, len(listAR))
		copy(listARCopy, listAR)
		listPeriodCount = listPeriodCount.SetPeriodCount(1, listARCopy)
		require.NotEmpty(t, listPeriodCount)
		listPeriodCount = listPeriodCount.Delete(1)
		require.Empty(t, listPeriodCount)
	})

	t.Run("rlp enc/dec", func(t *testing.T) {
		var listPeriodCount ListPeriodCount
		listARCopy := make(ListAdditionalRecord, len(listAR))
		copy(listARCopy, listAR)
		listPeriodCount = listPeriodCount.SetPeriodCount(1, listARCopy)
		b, err := rlp.EncodeToBytes(listPeriodCount)
		require.Nil(t, err)
		var newlistPeriodCount ListPeriodCount
		err = rlp.DecodeBytes(b, &newlistPeriodCount)
		require.Nil(t, err)
		t.Log("newlistPeriodCount", newlistPeriodCount)
	})
}

func TestListAdditionalRecord(t *testing.T) {
	type args struct {
		addr  common.Address
		count uint64
	}
	var tests []args
	for i := int64(1); i <= 100; i++ {
		tests = append(tests, args{addr: common.BigToAddress(big.NewInt(i)), count: 10})
	}

	var listAR ListAdditionalRecord
	for _, tt := range tests {
		listAR = listAR.AddRecord(tt.addr, tt.count)
	}
	require.Len(t, listAR, 100)

	for _, tt := range tests {
		//t.Log(tt.addr.Hex())
		record := listAR.Query(tt.addr)
		require.Equal(t, tt.count, record.Count)
		require.False(t, record.Addr.Empty())
	}

	// Double Add
	{
		for _, tt := range tests {
			listAR = listAR.AddRecord(tt.addr, tt.count)
		}

		for _, tt := range tests {
			//t.Log(tt.addr.Hex())
			record := listAR.Query(tt.addr)
			require.Equal(t, tt.count*2, record.Count)
			require.False(t, record.Addr.Empty())
		}
	}

	// Delete
	{
		for _, tt := range tests {
			listAR = listAR.Delete(tt.addr)
			record := listAR.Query(tt.addr)
			require.True(t, record.Addr.Empty())
		}
	}

}

func TestListHeartbeat(t *testing.T) {
	statedb := TestStateDB{make(map[common.Hash]common.Hash)}
	sysHeart, err := ReadSystemHeartbeat(statedb)
	require.NoError(t, err)
	address := []common.Address{
		common.StringToAddress("1"), common.StringToAddress("2"), common.StringToAddress("3"),
	}
	//var period uint64=10
	for i, _ := range address {
		i := uint64(i)
		sysHeart = sysHeart.Insert(address[i], i, i*10, 1, 10, 0)
	}
	sysHeart = sysHeart.CommitCache()
	for i, _ := range address {
		h := sysHeart.Query(address[i])
		i := uint64(i)
		log.Debug("get address data", "addr", h.Address, "count", h.Count.sub)
		require.Equal(t, i, h.LastTurn)
		require.Equal(t, i*10, h.LastNumber)
		require.Equal(t, Count{1, 10, 0}, h.Count.Query(1))
	}
	temAddr := common.StringToAddress("a")
	sysHeart = sysHeart.AddUnitCount(temAddr, 1, 1, 1)
	sysHeart = sysHeart.CommitCache()
	temp := sysHeart.Query(temAddr)
	require.Equal(t, uint64(1), temp.LastTurn)
	require.Equal(t, uint64(1), temp.LastNumber)
	require.Equal(t, uint64(1), temp.Count.Query(1).UnitCount)

	sysHeart = sysHeart.AddBackCount(temAddr, 2, 2, 1)
	sysHeart = sysHeart.CommitCache()
	temp = sysHeart.Query(temAddr)
	require.Equal(t, uint64(2), temp.LastTurn)
	require.Equal(t, uint64(2), temp.LastNumber)
	require.Equal(t, uint64(1), temp.Count.Query(1).UnitCount)
	require.Equal(t, uint64(1), temp.Count.Query(1).BackCount)

	h := sysHeart.Query(temAddr)
	h.LastNumber = 100
	h.LastTurn = 99
	h.Count.AddUnitCount(2)
	sysHeart = sysHeart.Update(h)

	require.Equal(t, uint64(100), sysHeart.Query(temAddr).LastNumber)
	sysHeart = sysHeart.CommitCache()
	require.Equal(t, uint64(99), sysHeart.Query(temAddr).LastTurn)
	require.Equal(t, uint64(1), sysHeart.Query(temAddr).Count.Query(2).UnitCount)

}

type TestStateDB struct {
	db map[common.Hash]common.Hash
}

func (s TestStateDB) SubBalance(common.Address, *big.Int) {
	panic("implement me")
}

func (s TestStateDB) AddBalance(common.Address, *big.Int) {
	panic("implement me")
}

func (s TestStateDB) GetBalance(common.Address) *big.Int {
	panic("implement me")
}

func (s TestStateDB) AddLog(log2 *types.Log) {
	panic("not yet")
}

func (s TestStateDB) GetBigData(addr common.Address, key []byte) ([]byte, error) {
	return nil, nil
}
func (s TestStateDB) SetBigData(addr common.Address, key []byte, value []byte) error {
	return nil
}
func (s TestStateDB) SetState(addr common.Address, key common.Hash, value common.Hash) {
	s.db[key] = value
}
func (s TestStateDB) GetState(a common.Address, b common.Hash) common.Hash {
	if v, bb := s.db[b]; bb {
		return v
	}
	return common.Hash{}
}

func newTestContext(t *testing.T, sc common.Address, cfg *params.ChainConfig) (*TestContext, []common.Address) {
	statedb := newTestState()
	ctx := TestContext{state: statedb, cfg: cfg, ctx: vm.Context{
		UnitNumber: new(big.Int).SetUint64(0),
		Transfer: func(db vm.StateDB, addresses common.Address, addresses2 common.Address, i *big.Int) {
			t.Log("transfer", "to", addresses2, "value", i)
		},
	}}

	defSysWitness := make([]List, params.ConfigParamsSCWitnessCap) //四组多的见证人
	var addressList []common.Address
	for i := range defSysWitness {
		address := common.StringToAddress(fmt.Sprintf("sysw_%d", i))
		defSysWitness[i] = List{
			Address: address,
			PubKey:  []byte{1, 2, 3},
		}
		statedb.SetBalance(address, new(big.Int).SetUint64(params.ConfigParamsSCWitnessMinMargin))
		addressList = append(addressList, address)
	}
	// 写入配置
	err := SetupChainConfig(statedb, ctx.cfg)
	require.NoError(t, err)
	err = SetupSystemWitness(statedb, defSysWitness)
	require.NoError(t, err)

	//写入理事配置
	for i := 0; i < 10; i++ {
		address := common.StringToAddress(fmt.Sprintf("colo_%d", i))
		statedb.SetBalance(address, new(big.Int).SetUint64(params.ConfigParamsCouncilMargin))
		err = SetupCouncilMember(statedb, []common.Address{address}, 1000)
		require.NoError(t, err)
	}

	return &ctx, addressList
}
