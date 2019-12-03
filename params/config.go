// Author: @ysqi

package params

import (
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/math"
	"gitee.com/nerthus/nerthus/log"
	"github.com/spf13/viper"
)

var (
	// 当前程序运行模式
	RunMode = "pro" // pro、test、dev

	NodeStartTime = time.Now() // 记录节点启动时间
)

var (
	// dev
	GenesisUTCTime = time.Date(2019, 11, 6, 0, 0, 0, 0, time.UTC)

	// TODO(ysqi): need replace with a real hash bafter run on public network.
	MainnetGenesisHash = common.HexToHash("0xd03ed5113fc763cb5f71a984d681e85fd11e9d0c72a1f2ea61c78c70c82229fe") // Mainnet genesis hash to enforce below configs on
	TestnetGenesisHash = common.HexToHash("0xb8aca85f4375c1265b3c63789263bf71a21c398d033a0fe89a95048a5153dc41") // Testnet genesis hash to enforce below configs on

	PublicPrivateKey     = "c653af048e8576a004eb27e93e8fc42866c75b31b57ae96c7234961914d53626" // 一个公用的秘钥
	PublicPrivateKeyAddr = common.ForceDecodeAddress("nts1zzgrss9elhevv2ww4q7mqnl9chtaclez5zll3c")
	SCAccount            = common.GenerateContractAddress(common.StringToAddress("genesis"), 0x6060, 1)
	FoundationAccount    = common.ForceDecodeAddress("nts1ptls2jeskwyjzd5fwtfl4q0kzcthh9f7aq6ehu")
)

const (
	//心跳发送间隔，3表示三个轮次，一个轮次系统链出11个单元
	//按一个单元90秒计算，一个轮次时间是16.5分钟(90*11)，3个轮次49.5分钟
	HeartbeatSendInterval     = 3
	SCDeadArbitrationInterval = 24 * 60 * 60 //系统链停止出单元间隔触发理事仲裁, 24小时
	//所允许最多的结算周期，鼓励尽快提取，而不是堆积不取。一个结算周期是 7 天，最多允许提取最近 2 个月的费用，过期作废。
	MaxSettlementPeriod uint64 = 8
)

const (
	ConfigParamsUCWitness          = 11                                    // 每组用户见证人人数
	ConfigParamsUCMinVotings       = uint64(ConfigParamsUCWitness*2/3 + 1) // 用户见证人投票数
	ConfigParamsSCWitness          = 11                                    // 每组系统链人数
	ConfigParamsSCMinVotings       = uint64(ConfigParamsSCWitness*2/3 + 1) // 系统见证人投票数
	ConfigParamsSCWitnessCap       = 55                                    // 系统见证人人数上限
	ConfigParamsUCWitnessMargin    = 600000 * Nts                          // 用户见证人保证金(nts)
	ConfigParamsSCWitnessMinMargin = 2100000 * Nts                         // 系统见证人保证金(nts)
	ConfigParamsCouncilMargin      = 110000 * Nts                          // 理事会成员保证金(nts)

	OneNumberHaveSecondBySys              = 90                                                                  // 系统链一个单元需要的时间(秒)
	OneNumberHaveSecondByUc               = 10                                                                  // 用户链一个单元需要的时间(秒)
	MaxMainSCWitnessHeartbeatRound uint64 = 60 * 60 * 24 * 1 / OneNumberHaveSecondBySys / ConfigParamsSCWitness //系统链见证人自动更换检查的工作周期，11个单元一个工作周期
	MaxHeartbeatRound                     = 60 * 60 * 24 * 2 / OneNumberHaveSecondBySys / ConfigParamsSCWitness // 备选见证人自动更换检查周期
	CounciExitNumber                      = uint64(7*24*time.Hour/time.Second) / OneNumberHaveSecondBySys       //理事退出必须是在7天后
	CounciTimeoutPunish                   = 0.12                                                                //理事无心跳被踢时的保证金扣款比例
	SystemWitnessTimeoutPunish            = 0.15                                                                //系统见证人心跳被踢时的保证金扣款比例

	ConfigParamsBillingPeriod             = 7 * 24 * 60 * 60 / OneNumberHaveSecondBySys  // 一个清算周期的单元数量
	ConfigParamsTimeApprove               = 15 * 60 * 60 * 24 / OneNumberHaveSecondBySys // 理事审核高度
	ConfigParamsTimeVoting                = 45 * 60 * 60 * 24 / OneNumberHaveSecondBySys // 用户投票高度
	ProposalSyswitnessApply               = 1 * 60 * 60 * 24 / OneNumberHaveSecondBySys  // 系统见证人申请高度
	ReplaceWitnessInterval         uint64 = 60 * 60 * 4 / OneNumberHaveSecondBySys       // 更换见证人需要的高度(4小时的理论上生产的单元数)
	ReplaceWitnessUnderwayInterval uint64 = 60 * 10 / OneNumberHaveSecondBySys           // 更换见证人进行中需要的高度(10分钟的理论单元数)

)

var (
	configParamsPublishNtsAmount = NTS2DOT(big.NewInt(30 * 100000000)) //公开发行代币数量
	//系统参数变更提案的最小公投默认为总发行量的30%，单位 NTS
	MinConfigPvoteAgreeVal uint64 = uint64(float64(DOT2NTS(configParamsPublishNtsAmount).Uint64()) * 0.3)
	//默认为总发行量的10%，单位 NTS
	MinCouncilPvoteAgreeVal uint64 = uint64(float64(DOT2NTS(configParamsPublishNtsAmount).Uint64()) * 0.1)
)

var (
	//所有共识参数
	allConsensusConfig = map[string]uint64{
		ConfigParamsUCWitnessCap:   10000,
		ConfigParamsCouncilCap:     10000, //理事人数上限无要求
		ConfigParamsLowestGasPrice: 100000,
		ConfigParamsMinFreePoW:     30,
		ConfigParamsUcGasLimit:     10000000,
		"ucWitness":                ConfigParamsUCWitness,
		"ucMinVotings":             ConfigParamsUCMinVotings,
		"ucWitnessMargin":          ConfigParamsUCWitnessMargin,
		"scWitness":                ConfigParamsSCWitness,
		"scMinVotings":             ConfigParamsSCMinVotings,
		SysWitnessCapKey:           ConfigParamsSCWitnessCap,
		"scWitnessMinMargin":       ConfigParamsSCWitnessMinMargin,
		"councilMargin":            ConfigParamsCouncilMargin,
		"billingPeriod":            ConfigParamsBillingPeriod,
		"timeApprove":              ConfigParamsTimeApprove,
		"timeVoting":               ConfigParamsTimeVoting,
		"sysWitnessApply":          ProposalSyswitnessApply,
		MinConfigPvoteAgreeKey:     MinConfigPvoteAgreeVal,
		MinCouncilPvoteAgreeKey:    MinCouncilPvoteAgreeVal,
	}
	canChangedConfig = map[string]bool{
		ConfigParamsUCWitnessCap:   true,
		ConfigParamsLowestGasPrice: true,
		ConfigParamsMinFreePoW:     true,
		ConfigParamsUcGasLimit:     true,
		MinCouncilPvoteAgreeKey:    true,
		MinConfigPvoteAgreeKey:     true,
	}

	// MainnetChainConfig is the chain parameters to run a node on the main network.
	MainnetChainConfig = &ChainConfig{
		ChainId:         big.NewInt(19),
		consensusConfig: allConsensusConfig,
		// 提案参数
		Proposal: canChangedConfig,
	}

	// TestnetChainConfig contains the chain parameters to run a node on the Ropsten test network.
	TestnetChainConfig = &ChainConfig{
		ChainId:         big.NewInt(1907),
		consensusConfig: allConsensusConfig,
		// 提案参数
		Proposal: canChangedConfig,
	}

	AllProtocolChanges = &ChainConfig{
		ChainId:         big.NewInt(2018),
		consensusConfig: allConsensusConfig,
		// 提案参数
		Proposal: canChangedConfig,
	}

	TestChainConfig = &ChainConfig{
		ChainId:         big.NewInt(66),
		consensusConfig: allConsensusConfig,
		// 提案参数
		Proposal: canChangedConfig,
	}

	TestRules = TestChainConfig.Rules(big.NewInt(0))
)

// 获取Nerthus代币发行总量
func TotalTokens() *big.Int {
	return new(big.Int).Set(NTS2DOT(big.NewInt(100 * 100000000)))
}

// GetChainMinVoting 获取链信息
func GetChainMinVoting(chain common.Address) int {
	if chain == SCAccount {
		return int(ConfigParamsSCMinVotings)
	}
	return int(ConfigParamsUCMinVotings)
}
func GetChainWitnessCount(chain common.Address) int {
	if chain == SCAccount {
		return int(ConfigParamsSCWitness)
	}
	return int(ConfigParamsUCWitness)
}

func CalcRound(currentNumber uint64) uint64 {
	round := currentNumber / ConfigParamsSCWitness
	if currentNumber%ConfigParamsSCWitness == 0 {
		return round
	}
	return round + 1
}

//一个轮次的实际
func OneRoundTime() time.Duration {
	return time.Duration(ConfigParamsSCWitness*OneNumberHaveSecondBySys) * time.Second
}

// ChainConfig is the core config which determines the blockchain settings.
//
// ChainConfig is stored in the database on a per block basis. This means
// that any network, identified by its genesis block, can have its own
// set of configuration options.
type ChainConfig struct {
	ChainId         *big.Int `json:"chain_id" yaml:"chainId"` // Chain id identifies the current chain and is used for replay protection
	mux             sync.RWMutex
	consensusConfig map[string]uint64 `json:"-" yaml:"-"`
	Proposal        map[string]bool   `json:"-" yaml:"-"`
}

func NewChainConfig() *ChainConfig {
	return &ChainConfig{
		ChainId:         big.NewInt(0),
		mux:             sync.RWMutex{},
		consensusConfig: make(map[string]uint64),
		Proposal:        make(map[string]bool),
	}
}

// 获取见证人数，最低票数
func (c *ChainConfig) GetVoteNum(mc common.Address) (uint64, uint64) {
	if mc == SCAccount {
		return ConfigParamsSCWitness, ConfigParamsSCMinVotings
	}
	return ConfigParamsUCWitness, ConfigParamsUCMinVotings
}

// PowLimit 免费交易的工作量证明最大要求
func (c *ChainConfig) PowLimit() *big.Int {
	//return new(big.Int).SetUint64(c.POWLowerLimit)
	return new(big.Int).SetUint64(c.Get(ConfigParamsMinFreePoW))
}

// 获取链ID
func (c *ChainConfig) GetChainID() *big.Int {
	return c.ChainId
}

// 获取配置信息
func (c *ChainConfig) Get(id string) uint64 {
	c.mux.RLock()
	defer c.mux.RUnlock()

	if v, ok := c.consensusConfig[id]; ok {
		return v
	} else {
		panic("invalid config id: " + id)
	}
	//return c.consensusConfig[id]
}

func (c *ChainConfig) Set(id string, v uint64) {
	c.mux.Lock()
	defer c.mux.Unlock()

	//忽略大小写方式查找对应 Key
	//如果存在，则覆盖，否则新增
	key := strings.ToLower(id)
	for k := range c.consensusConfig {
		if strings.ToLower(k) == key {
			c.consensusConfig[k] = v
			return
		}
	}
	c.consensusConfig[id] = v
}

const (
	SysWitnessCapKey           = "scWitnessCap"            //系统链见证人人数
	ConfigParamsUCWitnessCap   = "ucWitnessCap"            // 用户链见证人总数上限
	ConfigParamsCouncilCap     = "councilCap"              // 理事会成员上限
	ConfigParamsLowestGasPrice = "lowestGasPrice"          // 最低燃油单价(GasPrice)
	ConfigParamsMinFreePoW     = "minFreePoW"              // 免费交易工作量证明最低难度值
	ConfigParamsUcGasLimit     = "ucGasLimit"              // 用户gas上限
	MinConfigPvoteAgreeKey     = "min_config_pvote_agree"  //参数变更提案总投票量最低要求(NTS)
	MinCouncilPvoteAgreeKey    = "min_council_pvote_agree" //理事进出提案总投票量最低要求(NTS)
)

// 从 viper 中解析配置
func ParseChainConfig(v *viper.Viper) (cfg ChainConfig, err error) {

	cfg = *MainnetChainConfig //依次作为默认配置
	//如果无配置可用，则直接返回
	if v == nil {
		return cfg, nil
	}
	get := func(key string) (uint64, error) {
		vstr := v.GetString(key)
		val, ok := math.ParseUint64(vstr)
		if !ok {
			return 0, fmt.Errorf(" %q expected uint64 and greater than 0, got %q", key, vstr)
		}
		return val, nil
	}
	var val uint64
	if v.IsSet("chainId") {
		val, err = get("chainId")
		if err != nil {
			return
		}
		cfg.ChainId = new(big.Int).SetUint64(val)
	}

	for key, _ := range cfg.consensusConfig {
		val, err = get(key)
		if err != nil {
			return ChainConfig{}, err
		}
		if val != 0 {
			if key == ConfigParamsCouncilCap {
				log.Warn("disable change council cap")
				continue
			}
			cfg.Set(key, val)
		}
	}

	return cfg, nil
}

// CliqueConfig is the consensus engine configs for proof-of-authority based sealing.
type CliqueConfig struct {
	Period uint64 `json:"period"` // Number of seconds between blocks to enforce
	Epoch  uint64 `json:"epoch"`  // Epoch length to reset votes and checkpoint
}

// String implements the stringer interface, returning the consensus engine details.
func (c *CliqueConfig) String() string {
	return "clique"
}

// String implements the fmt.Stringer interface.
func (c *ChainConfig) String() string {
	return fmt.Sprintf("{ChainID: %d}", c.ChainId)
}

// GasTable returns the gas table corresponding to the current phase (homestead or homestead reprice).
//
// The returned GasTable's fields shouldn't, under any circumstances, be changed.
func (c *ChainConfig) GasTable(unitTime *big.Int) GasTable {
	return GasTableHomestead
}

func (c *ChainConfig) GetConfigs() map[string]uint64 {
	return c.consensusConfig
}

func (c *ChainConfig) GetProposal() map[string]bool {
	return c.Proposal
}

func (c *ChainConfig) SetConfigs(val map[string]uint64) {
	c.mux.Lock()
	defer c.mux.Unlock()
	for k, v := range val {
		c.consensusConfig[k] = v
	}
}

// ConfigCompatError is raised if the locally-stored blockchain is initialised with a
// ChainConfig that would alter the past.
type ConfigCompatError struct {
	What string
	// block numbers of the stored and new configurations
	StoredConfig, NewConfig *big.Int
	// the block number to which the local chain must be rewound to correct the error
	RewindTo uint64
}

func newCompatError(what string, storedblock, newblock *big.Int) *ConfigCompatError {
	var rew *big.Int
	switch {
	case storedblock == nil:
		rew = newblock
	case newblock == nil || storedblock.Cmp(newblock) < 0:
		rew = storedblock
	default:
		rew = newblock
	}
	err := &ConfigCompatError{what, storedblock, newblock, 0}
	if rew != nil && rew.Sign() > 0 {
		err.RewindTo = rew.Uint64() - 1
	}
	return err
}

func (err *ConfigCompatError) Error() string {
	return fmt.Sprintf("mismatching %s in database (have %d, want %d, rewindto %d)", err.What, err.StoredConfig, err.NewConfig, err.RewindTo)
}

// Rules wraps ChainConfig and is merely syntactic sugar or can be used for functions
// that do not have or require information about the block.
//
// Rules is a one time interface meaning that it shouldn't be used in between transition
// phases.
type Rules struct {
	ChainID                                   uint64
	IsHomestead, IsEIP150, IsEIP155, IsEIP158 bool
	IsByzantium, IsConstantinople             bool
}

// Rules ensures c's ChainID is not nil.
func (c *ChainConfig) Rules(num *big.Int) Rules {
	return Rules{
		ChainID:          c.ChainId.Uint64(),
		IsHomestead:      true,
		IsEIP150:         true,
		IsEIP155:         true,
		IsEIP158:         true,
		IsByzantium:      true,
		IsConstantinople: true,
	}
}

// IsHomestead returns whether num is either equal to the homestead block or greater.
func (c *ChainConfig) IsHomestead(num *big.Int) bool {
	return true
}

// IsDAOFork returns whether num is either equal to the DAO fork block or greater.
func (c *ChainConfig) IsDAOFork(num *big.Int) bool {
	return false
}

// IsEIP150 returns whether num is either equal to the EIP150 fork block or greater.
func (c *ChainConfig) IsEIP150(num *big.Int) bool {
	return true
}

// IsEIP155 returns whether num is either equal to the EIP155 fork block or greater.
func (c *ChainConfig) IsEIP155(num *big.Int) bool {
	return true
}

// IsEIP158 returns whether num is either equal to the EIP158 fork block or greater.
func (c *ChainConfig) IsEIP158(num *big.Int) bool {
	return true
}

// IsByzantium returns whether num is either equal to the Byzantium fork block or greater.
func (c *ChainConfig) IsByzantium(num *big.Int) bool {
	return true
}

// IsConstantinople returns whether num is either equal to the Constantinople fork block or greater.
func (c *ChainConfig) IsConstantinople(num *big.Int) bool {
	return true
}

// IsEWASM returns whether num represents a block number after the EWASM fork
func (c *ChainConfig) IsEWASM(num *big.Int) bool {
	return false
}
