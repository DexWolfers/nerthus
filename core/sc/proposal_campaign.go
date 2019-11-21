package sc

import (
	"fmt"
	"sort"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
)

type CampaignList []Campaign
type Campaign struct {
	Address   common.Address
	Margin    uint64
	Time      uint64
	PublicKey []byte `json:"-"`
}

func (c CampaignList) Len() int {
	return len(c)
}
func (c CampaignList) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}
func (c CampaignList) Less(i, j int) bool {
	return c[i].Margin > c[j].Margin
}

func (c CampaignList) Have(address common.Address) bool {
	for _, v := range c {
		if v.Address == address {
			return true
		}
	}
	return false
}
func (c CampaignList) Sort(vsTime bool) {
	if !vsTime {
		sort.Sort(c)
		return
	}

	sort.Slice(c, func(i, j int) bool {
		if c[i].Margin == c[j].Margin {
			return c[i].Time < c[j].Time
		}
		return c[i].Margin > c[j].Margin
	})
}

func (c CampaignList) Select(number uint64, maxSelect uint64) (winner, loser []Campaign) {
	if maxSelect == 0 {
		return nil, c[:]
	}
	//先排序: 保证金优先，时间优先
	c.Sort(true)

	if maxSelect < uint64(c.Len()) {
		loser = c[maxSelect:]
		winner = c[:maxSelect]
	} else {
		winner = c[:]
	}
	if len(winner) <= 2 {
		return
	}
	//对于 winner 需要加入随机因子调整顺序
	//先按账户地址排序，再根据随机因子对调部分账户位置
	sort.SliceStable(winner, func(i, j int) bool {
		return winner[i].Address.Big().Cmp(winner[j].Address.Big()) <= 0
	})

	randv := int(number % uint64(len(winner)))
	if randv <= 11 {
		randv = 11
	}
	for i := 0; i < len(winner) && i+randv < len(winner); i += 2 {
		j := i + randv
		winner[i], winner[j] = winner[j], winner[i]
	}
	return
}

// getProposalCampaignItem 获取提案的竞选初始化DB key
func getProposalCampaignItem(proposalHash common.Hash) SubItems {
	return SubItems([]interface{}{listCampaignTag, proposalHash})
}

// getProposalCampaignCount 获取提案的竞选总数
func getProposalCampaignCount(db StateDB, proposalHash common.Hash) uint64 {
	hash := getProposalCampaignItem(proposalHash).GetSub(db, nil)
	if hash.Empty() {
		return 0
	}
	return hash.Big().Uint64()
}

// getProposalCampaignInfoByAddr 根据地址获取提案的竞选详情
func getProposalCampaignInfoByAddr(db StateDB, proposalHash common.Hash, address common.Address) (*Campaign, error) {
	item := getProposalCampaignItem(proposalHash).Join(address)
	hash := item.GetSub(db, nil)
	if hash.Empty() {
		return nil, ErrNotCampaign
	}
	publicKey, err := item.GetBigData(db, "public_key")
	if err != nil {
		return nil, err
	}
	var campaign Campaign
	campaign.Address = address
	campaign.Margin = item.GetSub(db, "margin").Big().Uint64()
	campaign.Time = item.GetSub(db, "time").Big().Uint64()
	campaign.PublicKey = publicKey
	return &campaign, nil
}

// getProposalCampaignInfoByFlag 根据索引获取提案的竞选详情
func getProposalCampaignInfoByFlag(db StateDB, proposalHash common.Hash, flag uint64) (*Campaign, error) {
	hash := getProposalCampaignItem(proposalHash).GetSub(db, flag)
	if hash.Empty() {
		return nil, ErrNotCampaign
	}
	return getProposalCampaignInfoByAddr(db, proposalHash, hash.Address())
}

// writeProposalCampaignInfo 写入提案竞选详情
func writeProposalCampaignInfo(db StateDB, proposalHash common.Hash, campaign *Campaign) {
	item := getProposalCampaignItem(proposalHash).Join(campaign.Address)
	item.SaveSub(db, "margin", campaign.Margin)
	item.SaveSub(db, "time", campaign.Time)
	item.SaveBigData(db, "public_key", campaign.PublicKey)
}

// updateProposalCampaignInfo 更新提案竞选详情
func updateProposalCampaignInfo(db StateDB, proposalHash common.Hash, address common.Address, margin, time uint64) {
	item := getProposalCampaignItem(proposalHash).Join(address)
	item.SaveSub(db, "margin", margin)
	item.SaveSub(db, "time", time)
}

// addProposalCampaign 添加竞选
func addProposalCampaign(db StateDB, proposalHash common.Hash, campaign *Campaign) {
	count := getProposalCampaignCount(db, proposalHash)
	count++
	item := getProposalCampaignItem(proposalHash)
	// 写入总数
	item.SaveSub(db, nil, count)
	// 写入索引对应的地址
	item.SaveSub(db, count, campaign.Address)
	// 写入地址对应的索引
	item.Join(campaign.Address).SaveSub(db, nil, count)
	writeProposalCampaignInfo(db, proposalHash, campaign)
}

// getProposalCampaignList 获取竞选列表
func getProposalCampaignList(db StateDB, proposalHash common.Hash, fun func(campaign *Campaign) bool) error {
	count := getProposalCampaignCount(db, proposalHash)
	var (
		campaign *Campaign
		err      error
	)
	for i := uint64(1); i <= count; i++ {
		campaign, err = getProposalCampaignInfoByFlag(db, proposalHash, i)
		if err != nil {
			return err
		}
		if !fun(campaign) {
			return nil
		}
	}
	return nil
}

// emptyCampaign 用户是否参与竞选见证人
func emptyCampaign(db StateDB, proposalHash common.Hash, address common.Address) bool {
	if getProposalCampaignItem(proposalHash).Join(address).GetSub(db, nil).Empty() {
		return true
	}
	return false
}

// updateProposalCampaign 更新竞选成员记录
func updateProposalCampaign(db StateDB, proposalHash common.Hash, address common.Address, margin, utime uint64) error {
	c, err := getProposalCampaignInfoByAddr(db, proposalHash, address)
	if err != nil {
		return err
	}

	v, overflow := math.SafeAdd(c.Margin, margin)
	if overflow {
		return fmt.Errorf("the margin is too big, %d+%d > MaxUint64(%d)", c.Margin, margin, math.MaxUint64)
	}
	updateProposalCampaignInfo(db, proposalHash, address, v, utime)
	return nil
}
