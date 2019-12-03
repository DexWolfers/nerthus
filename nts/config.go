// Author: @ysqi

package nts

import (
	"math/big"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/hexutil"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/nts/protocol"
	"gitee.com/nerthus/nerthus/whisper/whisperv5"
)

// DefaultConfig contains default settings for use on the Nerthus main net.
var DefaultConfig = Config{
	NetworkId:     1,
	LightPeers:    20,
	DatabaseCache: 128,
	GasPrice:      big.NewInt(0), //TODO(ysqi):不支持设置

	TxPool: core.DefaultTxPoolConfig,
	//GPO: gasprice.Config{
	//	Blocks:     10,
	//	Percentile: 50,
	//},
	MaxMessageSize:     whisperv5.DefaultConfig.MaxMessageSize,
	MinimumAcceptedPOW: whisperv5.DefaultConfig.MinimumAcceptedPOW,
	PublicTopic:        whisperv5.BytesToTopic(common.FromHex("44c7429f")),
	PublicTopicPw:      "123456",
}

//go:generate gencodec -type Config -field-override configMarshaling -formats toml -out gen_config.go

type Config struct {
	// The genesis unit, which is inserted if the database is empty.
	// If nil, the Nerthus main net unit is used.
	Genesis *core.Genesis `toml:",omitempty"`

	// Protocol options
	NetworkId uint64 // Network ID to use for selecting peers to connect to

	SyncMode protocol.SyncMode
	// Light client options
	LightServ  int `toml:",omitempty"` // Maximum percentage of time allowed for serving LES requests
	LightPeers int `toml:",omitempty"` // Maximum number of LES client peers
	MaxPeers   int `toml:"-"`          // Maximum number of global peers

	// Database options
	SkipBcVersionCheck bool `toml:"-"`
	DatabaseHandles    int  `toml:"-"`
	DatabaseCache      int

	// Witness-related options
	MainAccount  common.Address `toml:",omitempty"`
	MinerThreads int            `toml:",omitempty"`
	ExtraData    []byte         `toml:",omitempty"`
	GasPrice     *big.Int

	// 交易池配置
	TxPool core.TxPoolConfig

	// Witvote options
	WitvoteCacheDir       string
	WitvoteCachesInMem    int
	WitvoteCachesOnDisk   int
	WitvoteDatasetDir     string
	WitvoteDatasetsInMem  int
	WitvoteDatasetsOnDisk int

	// Gas Price Oracle options
	//GPO gasprice.Config

	// Enables tracking of SHA3 preimages in the VM
	EnablePreimageRecording bool

	// Miscellaneous options
	DocRoot string `toml:"-"`
	PowFake bool   `toml:"-"`

	// 密语配置- 最大消息长度限制
	MaxMessageSize uint32 `toml:",omitempty"`
	// 密语配置 - 可接受的最低Pow （防止垃圾消息，需要一定的工作量证明）
	MinimumAcceptedPOW float64 `toml:",omitempty"`

	// 话题
	PublicTopic whisperv5.TopicType `toml:",omitempty"`
	// 话题密码
	PublicTopicPw string `toml:",omitempty"`

	// 运行时的子通道
	SubPipeListenAddr string
}

type configMarshaling struct {
	ExtraData hexutil.Bytes
}
