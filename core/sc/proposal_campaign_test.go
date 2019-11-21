package sc

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
	"time"

	"gitee.com/nerthus/nerthus/common/math"

	"gitee.com/nerthus/nerthus/crypto"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/ntsdb"
	"github.com/stretchr/testify/require"
)

func TestProposalCampaign(t *testing.T) {
	baseDB, err := ntsdb.NewMemDatabase()
	require.Nil(t, err)
	stateDB, err := state.New([]byte{1, 2, 3}, common.Hash{}, state.NewDatabase(baseDB))
	require.Nil(t, err)
	proposalHash := getProposalId(stateDB)
	require.Equal(t, getProposalCampaignCount(stateDB, proposalHash), uint64(0))
	var campaignList CampaignList
	for i := uint64(1); i <= 10; i++ {
		privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.Nil(t, err)
		campaign := Campaign{
			Address:   common.StringToAddress("campaign" + string(i)),
			Margin:    i + 100000,
			Time:      i + 200000,
			PublicKey: crypto.FromECDSAPub(&privateKey.PublicKey),
		}
		campaignList = append(campaignList, campaign)
		addProposalCampaign(stateDB, proposalHash, &campaign)
	}
	require.Equal(t, getProposalCampaignCount(stateDB, proposalHash), uint64(len(campaignList)))
	valid := func() {
		var i int
		err = getProposalCampaignList(stateDB, proposalHash, func(campaign *Campaign) bool {
			i++
			require.Equal(t, campaign.Address, campaignList[len(campaignList)-i].Address)
			require.Equal(t, campaign.Margin, campaignList[len(campaignList)-i].Margin)
			require.Equal(t, campaign.Time, campaignList[len(campaignList)-i].Time)
			require.Equal(t, campaign.PublicKey, campaignList[len(campaignList)-i].PublicKey)
			return true
		})
		require.Nil(t, err)
	}
	valid()
	for k := range campaignList {
		campaignList[k].Margin += 123
		campaignList[k].Time += 234
		updateProposalCampaignInfo(stateDB, proposalHash, campaignList[k].Address, campaignList[k].Margin, campaignList[k].Time)
	}
	valid()
	updateCampaign := campaignList[1]
	err = updateProposalCampaign(stateDB, proposalHash, updateCampaign.Address, 111, 222)
	require.Nil(t, err)
	campaign, err := getProposalCampaignInfoByAddr(stateDB, proposalHash, updateCampaign.Address)
	require.Nil(t, err)
	require.Equal(t, campaign.Time, uint64(222))
	require.Equal(t, campaign.Margin, uint64(111))
}

func TestUpdateProposalCampaign(t *testing.T) {
	db := newTestState()
	proposalHash := getProposalId(db)

	campaign := Campaign{
		Address:   common.StringToAddress("campaign1"),
		Margin:    math.MaxUint64 - 1,
		Time:      uint64(time.Now().UnixNano()),
		PublicKey: []byte{1, 2, 3},
	}
	addProposalCampaign(db, proposalHash, &campaign)

	err := updateProposalCampaign(db, proposalHash, campaign.Address, 1, 123)
	require.NoError(t, err)

	err2 := updateProposalCampaign(db, proposalHash, campaign.Address, 2, 123)
	require.Error(t, err2)
}

func TestCampaignList_Select(t *testing.T) {

	toAddr := func(v uint8) common.Address {
		return common.BytesToAddress([]byte{v})
	}
	list := CampaignList{
		{Address: toAddr(1), Margin: 1000, Time: 1230001},
		{Address: toAddr(2), Margin: 10000, Time: 1230002},
		{Address: toAddr(3), Margin: 1001, Time: 1230003},
		{Address: toAddr(4), Margin: 1005, Time: 1230003},
		{Address: toAddr(5), Margin: 1009, Time: 1230004},
		{Address: toAddr(6), Margin: 1005, Time: 1230004},
		{Address: toAddr(7), Margin: 1004, Time: 1230004},
		{Address: toAddr(8), Margin: 1008, Time: 1230004},
		{Address: toAddr(9), Margin: 1008, Time: 1230002},
	}

	ck := func(maxSelect uint64, wantWinner []uint8) {
		winnner, loser := list.Select(100000000000, maxSelect)

		require.Len(t, winnner, len(wantWinner)) //数量必须一致
		require.Len(t, loser, len(list)-len(wantWinner))

		for _, w := range wantWinner { //赢得的人集合中必须存在希望获选的人
			want := toAddr(w)
			var find bool
			for _, got := range winnner {
				if got.Address == want {
					find = true
					break
				}
			}
			require.True(t, find, "should be include witness %d on winner list", w)
		}

		var list []uint8
		for _, w := range winnner {
			b := w.Address.Bytes()
			list = append(list, b[len(b)-1])
		}
		t.Logf("select %d, rank: %v, sort result: %v", maxSelect, wantWinner, list)

	}

	//低于 11 个时将根据地址排序
	ck(0, []uint8{})
	ck(1, []uint8{2})
	ck(2, []uint8{2, 5})
	ck(3, []uint8{2, 5, 9})
	ck(5, []uint8{2, 5, 9, 8, 4})
	ck(9, []uint8{2, 5, 9, 8, 4, 6, 7, 3, 1})

	//更多时将随机调整
	//新增一些，测试数量
	need := 120
	want := []uint8{2, 5, 9, 8, 4, 6, 7, 3, 1}
	for i := 10; i <= 255; i++ {
		list = append(list, Campaign{Address: toAddr(uint8(i)), Margin: uint64(500 - i), Time: 1230004})
		if len(want) < need {
			want = append(want, uint8(i))
		}
	}
	ck(uint64(need), want)
}
