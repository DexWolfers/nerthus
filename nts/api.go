package nts

import (
	"errors"

	"gitee.com/nerthus/nerthus/rpc"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/hexutil"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/sc"
	"gitee.com/nerthus/nerthus/core/state"
	"gitee.com/nerthus/nerthus/core/types"
	"gitee.com/nerthus/nerthus/params"
)

// PublicEthereumAPI provides an API to access Ethereum full node-related
// information.
type PublicNerthusAPI struct {
	n *Nerthus
}

// NewPublicNerthusAPI creates a new Etheruem protocol API for full nodes.
func NewPublicNerthusAPI(e *Nerthus) *PublicNerthusAPI {
	return &PublicNerthusAPI{e}
}

// 获取见证人合约账号
func (api *PublicNerthusAPI) SystemChain() (common.Address, error) {
	return params.SCAccount, nil
}

// 获取创世哈希
func (api *PublicNerthusAPI) GenesisHash() common.Hash {
	return api.n.dagchain.Genesis().Hash()
}

func (api *PublicNerthusAPI) GetContractAddr(txHash common.Hash) (common.Address, error) {
	tx := api.n.dagchain.GetTransaction(txHash)
	if tx == nil {
		return common.EmptyAddress, errors.New("not found the transaction")
	}
	if !sc.IsCallCreateContractTx(tx.To(), tx.Data()) {
		return common.EmptyAddress, errors.New("the transaction is not contract's transaction")
	}

	txInfos, err := api.GetTxStatus(txHash)
	if err != nil {
		return common.EmptyAddress, err
	}
	for _, t := range txInfos {
		if t.Failed {
			return common.EmptyAddress, errors.New("contract deploy fail")
		}
	}
	if len(txInfos) < 2 {
		return common.EmptyAddress, errors.New("contract is deploying")
	}

	info, err := api.n.dag.GetTxReceiptAtUnit(txInfos[1].UnitHash, txHash)
	if err != nil {
		return common.EmptyAddress, err
	}
	output, err := sc.GetContractOutput(sc.FuncCreateContract, info.Output)
	if err != nil {
		return common.EmptyAddress, err
	}
	return output.(sc.NewCtWit).Contract, nil
}

// ChainConfig 获取链配置信息
func (api *PublicNerthusAPI) ChainConfig() map[string]uint64 {
	chainCfg := api.n.chainConfig
	result := chainCfg.GetConfigs()
	// TODO 获取结构体中Tag的值
	result["chainId"] = chainCfg.ChainId.Uint64()

	return result
}

// 添加前端需要的简单数据结构
// Author: @yin
type SimpleUnitMsg struct {
	UnitHash  string `json:"unit_hash"`
	StampTime uint64 `json:"stamp_time"`
	To        string `json:"to"`
	PayMoney  string `json:"pay_money"`
	App       string `json:"app"`
}

func (api *PublicNerthusAPI) GasPrice() hexutil.Uint64 {
	return hexutil.Uint64(api.n.DagChain().Config().Get(params.ConfigParamsLowestGasPrice))
}

// 查询余额
// Author:@kulics
func (api *PublicNerthusAPI) GetBalance(address common.Address) (hexutil.Big, error) {
	// 查询余额
	balance, err := api.n.dag.GetBalanceByAddress(address)
	if err != nil {
		return hexutil.Big{}, err
	}
	return hexutil.Big(*balance), nil

}

func (api *PublicNerthusAPI) GetBalances(address []common.Address) (map[common.Address]hexutil.Big, error) {
	var dic = make(map[common.Address]hexutil.Big)
	for _, v := range address {
		// 查询余额
		balance, err := api.n.dag.GetBalanceByAddress(v)
		if err != nil {
			return nil, err
		}
		dic[v] = hexutil.Big(*balance)
	}

	return dic, nil
}

// 根据交易哈希获取单元
// Author: @kulics
func (api *PublicNerthusAPI) GetUnitByHash(hash common.Hash) (*Block, error) {
	unit, err := api.n.dag.GetUnitByHash(hash)
	if err != nil {
		return nil, err
	}
	return ToBlockStruct(unit)
}

func (api *PublicNerthusAPI) GetUnitNumber(chain common.Address) uint64 {
	return api.n.DagChain().GetChainTailNumber(chain)
}

// 获取账户指定位置单元
// Author: @kulics
func (api *PublicNerthusAPI) GetUnitByNumber(account common.Address, number int) (*Block, error) {
	// 获取单元
	unit := api.n.dagchain.GetUnitByNumber(account, uint64(number))
	if unit == nil {
		return nil, core.ErrUnitNotFind
	}
	// 重组数据
	return ToBlockStruct(unit)
}
func (api *PublicNerthusAPI) GetHeaderByNumber(chain common.Address, number rpc.BlockNumber) (*types.Header, error) {
	var h *types.Header
	if number == rpc.LatestBlockNumber {
		h = api.n.DagChain().GetChainTailHead(chain)
	} else {
		h = api.n.DagChain().GetHeaderByNumber(chain, number.MustUint64())
	}
	if h == nil {
		return nil, errors.New("not found header by hash")
	}
	return h, nil
}
func (api *PublicNerthusAPI) GetHeaderByHash(uhash common.Hash) (*types.Header, error) {
	h := api.n.dagchain.GetHeaderByHash(uhash)
	if h == nil {
		return nil, errors.New("not found")
	}
	return h, nil
}

// GetChainLastUnit 获取链末端单元
func (api *PublicNerthusAPI) GetChainLastUnit(addr common.Address) (*Block, error) {
	return ToBlockStruct(api.n.dagchain.GetChainTailUnit(addr))
}

// GetBrotherUnits 获取指定位置的多个单元（分叉）
//func (api *PublicNerthusAPI) GetBrotherUnits(addr common.Address, number uint64) common.Hash {
//	return api.n.dag.GetBrothers(addr, number)
//}

// 记录一笔交易的不同阶段信息
type TxActionStatusInfo struct {
	Chain    common.Address `json:"chain"`
	Number   uint64         `json:"number"`
	UnitHash common.Hash    `json:"unit_hash"`
	Status   string         `json:"unit_status"` // TODO:移除， unstable - 待稳定, stabled- 已稳定
	Action   types.TxAction `json:"action"`
	Failed   bool           `json:"failed"`
}

// 获取交易相关的单元哈希
func (api *PublicNerthusAPI) GetTxStatus(hash common.Hash) ([]TxActionStatusInfo, error) {
	return GetTxSetpStatus(api.n.dag, api.n.dagchain, hash)
}

func (api *PublicNerthusAPI) GetTxReceipts(txhash common.Hash) (types.Receipts, error) {
	return core.GetTxReceipts(api.n.chainDb, txhash)
}

// 获取单元所有交易回执
func (api *PublicNerthusAPI) GetUnitReceipts(uhash common.Hash) (types.Receipts, error) {
	mc, number := api.n.DagChain().GetUnitNumber(uhash)
	if mc.Empty() {
		return nil, errors.New("not found unit by hash")
	}
	return core.GetUnitReceipts(api.n.chainDb, mc, uhash, number), nil
}

// IsAccountWitness 检查地址是否见证人
// Author: @kulics
func (api *PublicNerthusAPI) GetWitness(account common.Address) (sc.WitnessInfo, error) {
	var witInfo sc.WitnessInfo
	state, err := api.n.dagchain.GetChainTailState(params.SCAccount)
	if err != nil {
		return witInfo, err
	}

	info, err := sc.GetWitnessInfo(state, account)
	if err != nil {
		return witInfo, err
	}
	return *info, nil
}

func (api *PublicNerthusAPI) GetCouncil(account common.Address) (sc.Council, error) {
	stateDB, err := api.n.DagChain().GetChainTailState(params.SCAccount)
	if err != nil {
		return sc.Council{}, err
	}
	council, err := sc.GetCouncilInfo(stateDB, account)
	if err != nil {
		return sc.Council{}, err
	}
	return *council, nil
}

// GetSystemWitness 获取钱包中的系统见证人
func (api *PublicNerthusAPI) GetSystemWitness() ([]common.Address, error) {
	//var result []common.Address
	//for _, addr := range addresses {
	//	ret, err := api.n.IsSystemWitness(addr)
	//	if err != nil {
	//		return nil, err
	//	}
	//	if ret {
	//		result = append(result, addr)
	//	}
	//}
	//return result, nil
	return api.n.GetSystemWitnessList()
}

func (api *PublicNerthusAPI) GetAllSysWitness() (common.AddressList, error) {
	return api.n.GetAllSysWitness()
}

// GetAccountWitness 获取用户的见证人
// Author: @kulics
func (api *PublicNerthusAPI) GetAccountWitness(address common.Address) ([]common.Address, error) {
	witness, err := api.n.dagchain.GetChainWitnessLib(address)
	if err != nil && err != sc.ErrChainMissingWitness {
		return nil, err
	}
	if err == sc.ErrChainMissingWitness {
		return []common.Address{}, nil
	}
	return witness, nil
}

type RLPWitnessInfo struct {
	Address      common.Address   `json:"witness"` //见证人地址
	Margin       uint64           `json:"margin"`  //提交的保证金
	ApplyHeight  uint64           `json:"join"`    // 加入时的系统链高度
	CancelHeight uint64           `json:"leave"`   // 注销时的系统高度
	Status       sc.WitnessStatus `json:"status"`  //状态
	ChainCount   uint64           `json:"chains"`  //当前分配的需参与见证的链数量
}

// GetChainWitnessInfo 获取链的见证人信息
func (api *PublicNerthusAPI) GetChainWitnessInfo(address common.Address) ([]RLPWitnessInfo, error) {
	state, err := api.n.dagchain.GetChainTailState(params.SCAccount)
	if err != nil {
		return nil, err
	}
	witnessList, err := sc.GetChainWitness(state, address)
	if err != nil {
		if err == sc.ErrChainMissingWitness {
			return []RLPWitnessInfo{}, nil
		}
		return nil, err
	}
	list := make([]RLPWitnessInfo, witnessList.Len())
	var info *sc.WitnessInfo
	for k, v := range witnessList {
		info, err = sc.GetWitnessInfo(state, v)
		if err != nil {
			return nil, err
		}
		list[k] = RLPWitnessInfo{
			Address:      info.Address,
			Margin:       info.Margin,
			ApplyHeight:  info.ApplyHeight,
			CancelHeight: info.CancelHeight,
			Status:       info.Status,
		}
	}
	return list, nil
}

func (api *PublicNerthusAPI) toFullStruct(unit *types.Unit) (interface{}, error) {
	var enc Block

	sender, err := unit.Sender()
	if err != nil {
		return nil, err
	}

	witnesses, err := unit.Witnesses()
	if err != nil {
		return nil, err
	}

	enc.Header = unit.Header()
	enc.General = sender
	enc.ListWit = witnesses
	enc.From = unit.MC()
	enc.Body = unit.Body()
	return enc, nil
}

// GetTransaction 获取交易信息
func (api *PublicNerthusAPI) GetTransaction(txhash common.Hash) (*RPCTransaction, error) {
	// 优先在交易池中查找,如交易池找不到则在db中查找
	tx := api.n.txPool.Get(txhash)
	if tx == nil {
		tx = core.GetTransaction(api.n.chainDb, txhash)
	}

	if tx == nil {
		return nil, errors.New("not found")
	}
	result, err := newRPCTransaction(api.n.DagChain(), tx)
	if err != nil {
		return nil, err
	}
	exeResult, err := core.GetTxReceipts(api.n.chainDb, txhash)
	if err != nil {
		if err == core.ErrMissingTxStatusInfo {
			return result, nil
		}
		return nil, err
	}
	var used uint64 = 0
	for _, v := range exeResult {
		used += v.GasUsed
	}
	result.GasUsed = hexutil.Uint64(used)

	return result, nil
}

type PageData struct {
	Sum  int           `json:"sum"`
	List []interface{} `json:"list"`
}

func (api *PublicNerthusAPI) GetTransactionList(address []common.Address, t types.TransactionType, pageIndex, pageSize uint64) (PageData, error) {
	var page PageData
	if len(address) == 0 {
		return page, nil //如果无对应账户，则直接返回
	}

	sum, txHashs, err := api.n.sta.GetTxs(t, pageIndex-1, pageSize, address...) //index 从0开始
	if err != nil {
		return page, err
	}
	if sum == 0 {
		return page, nil
	}
	page.Sum = sum
	page.List = make([]interface{}, 0, pageSize)
	for _, txhash := range txHashs {
		tx := api.n.txPool.GetTxWithDisk(txhash)
		if tx == nil {
			tx = api.n.DagChain().GetTransaction(txhash)
		}
		if tx == nil {
			continue
		}
		info, err := newRPCTransaction(api.n.DagChain(), tx)
		if err != nil {
			continue
		}
		page.List = append(page.List, info)
	}
	return page, nil
}

// GetWitnessCurrentUsers 获取给定见证下当前所有链地址
func (api *PublicNerthusAPI) GetWitnessCurrentUsers(witness common.Address, pageIndex, pageSize uint64) ([]common.Address, error) {
	return api.n.dag.GetWitnessCurrentUsers(witness, pageIndex, pageSize)
}

// 获取链状态
func (api *PublicNerthusAPI) GetChainStatus(address common.Address) (sc.ChainStatus, error) {
	return api.n.DagChain().GetChainStatus(address)
}

//获取见证报表
func (api *PublicNerthusAPI) GetWitnessReport(address common.Address, period uint64) types.WitnessPeriodWorkInfo {
	if period == 0 {
		last := api.n.dagchain.GetChainTailUnit(params.SCAccount)
		period = sc.GetSettlePeriod(last.Number())
	}
	return api.n.dagchain.GetWitnessReport(address, period)
}

// 获取指定周期的见证人工作报告
func (api *PublicNerthusAPI) GetAllWitnessWorkReport(period uint64) map[common.Address]types.WitnessPeriodWorkInfo {
	if period == 0 {
		last := api.n.DagChain().GetChainTailUnit(params.SCAccount)
		period = sc.GetSettlePeriod(last.Number())
	}
	return sc.GetWitnessPeriodWorkReports(api.n.ChainDb(), period)
}

func (api *PublicNerthusAPI) GetChainWitnessStartNumber(address common.Address) (uint64, error) {
	state, err := api.n.dagchain.GetChainTailState(params.SCAccount)
	if err != nil {
		return 0, err
	}
	return sc.GetChainWitnessStartNumber(state, address), nil
}

// 获取指定见证组信息
func (api *PublicNerthusAPI) GetWitnessGroupInfo(index uint64, sysNumber rpc.BlockNumber) (gp interface{}, err error) {

	info := struct {
		Witness []common.Address
		Chains  []common.Address
	}{}

	var db *state.StateDB

	if sysNumber == rpc.LatestBlockNumber {
		db, err = api.n.DagChain().GetChainTailState(params.SCAccount)
		if err != nil {
			return nil, err
		}
	} else {
		db, err = api.n.DagChain().GetStateByNumber(params.SCAccount, sysNumber.MustUint64())
		if err != nil {
			return nil, err
		}
	}

	info.Witness = sc.GetWitnessListAt(db, index)
	sc.GetChainsAtGroup(db, index, func(index uint64, chain common.Address) bool {
		info.Chains = append(info.Chains, chain)
		return true
	})
	return info, nil
}

// 获取所有见证组汇总信息
func (api *PublicNerthusAPI) GetWitnessGroups(sysNumber rpc.BlockNumber) (info interface{}, err error) {
	type GroupInfo struct {
		Index   uint64
		Witness []common.Address
	}

	var db *state.StateDB

	if sysNumber == rpc.LatestBlockNumber {
		db, err = api.n.DagChain().GetChainTailState(params.SCAccount)
		if err != nil {
			return nil, err
		}
	} else {
		db, err = api.n.DagChain().GetStateByNumber(params.SCAccount, sysNumber.MustUint64())
		if err != nil {
			return nil, err
		}
	}

	count := sc.GetWitnessGroupCount(db)
	list := make([]GroupInfo, count)
	for i := uint64(0); i < count; i++ {
		list[i] = GroupInfo{
			Index:   i,
			Witness: sc.GetWitnessListAt(db, i),
		}
	}
	return list, nil
}
