// Code generated by github.com/fjl/gencodec. DO NOT EDIT.

package nts

import (
	"math/big"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/hexutil"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/nts/protocol"
	"gitee.com/nerthus/nerthus/whisper/whisperv5"
)

var _ = (*configMarshaling)(nil)

func (c Config) MarshalTOML() (interface{}, error) {
	type Config struct {
		Genesis                 *core.Genesis `toml:",omitempty"`
		NetworkId               uint64
		SyncMode                protocol.SyncMode
		LightServ               int  `toml:",omitempty"`
		LightPeers              int  `toml:",omitempty"`
		MaxPeers                int  `toml:"-"`
		SkipBcVersionCheck      bool `toml:"-"`
		DatabaseHandles         int  `toml:"-"`
		DatabaseCache           int
		MainAccount             common.Address `toml:",omitempty"`
		MinerThreads            int            `toml:",omitempty"`
		ExtraData               hexutil.Bytes  `toml:",omitempty"`
		GasPrice                *big.Int
		TxPool                  core.TxPoolConfig
		WitvoteCacheDir         string
		WitvoteCachesInMem      int
		WitvoteCachesOnDisk     int
		WitvoteDatasetDir       string
		WitvoteDatasetsInMem    int
		WitvoteDatasetsOnDisk   int
		EnablePreimageRecording bool
		DocRoot                 string              `toml:"-"`
		PowFake                 bool                `toml:"-"`
		MaxMessageSize          uint32              `toml:",omitempty"`
		MinimumAcceptedPOW      float64             `toml:",omitempty"`
		PublicTopic             whisperv5.TopicType `toml:",omitempty"`
		PublicTopicPw           string              `toml:",omitempty"`
	}
	var enc Config
	enc.Genesis = c.Genesis
	enc.NetworkId = c.NetworkId
	enc.SyncMode = c.SyncMode
	enc.LightServ = c.LightServ
	enc.LightPeers = c.LightPeers
	enc.MaxPeers = c.MaxPeers
	enc.SkipBcVersionCheck = c.SkipBcVersionCheck
	enc.DatabaseHandles = c.DatabaseHandles
	enc.DatabaseCache = c.DatabaseCache
	enc.MainAccount = c.MainAccount
	enc.MinerThreads = c.MinerThreads
	enc.ExtraData = c.ExtraData
	enc.GasPrice = c.GasPrice
	enc.TxPool = c.TxPool
	enc.WitvoteCacheDir = c.WitvoteCacheDir
	enc.WitvoteCachesInMem = c.WitvoteCachesInMem
	enc.WitvoteCachesOnDisk = c.WitvoteCachesOnDisk
	enc.WitvoteDatasetDir = c.WitvoteDatasetDir
	enc.WitvoteDatasetsInMem = c.WitvoteDatasetsInMem
	enc.WitvoteDatasetsOnDisk = c.WitvoteDatasetsOnDisk
	enc.EnablePreimageRecording = c.EnablePreimageRecording
	enc.DocRoot = c.DocRoot
	enc.PowFake = c.PowFake
	enc.MaxMessageSize = c.MaxMessageSize
	enc.MinimumAcceptedPOW = c.MinimumAcceptedPOW
	enc.PublicTopic = c.PublicTopic
	enc.PublicTopicPw = c.PublicTopicPw
	return &enc, nil
}

func (c *Config) UnmarshalTOML(unmarshal func(interface{}) error) error {
	type Config struct {
		Genesis                 *core.Genesis `toml:",omitempty"`
		NetworkId               *uint64
		SyncMode                *protocol.SyncMode
		LightServ               *int  `toml:",omitempty"`
		LightPeers              *int  `toml:",omitempty"`
		MaxPeers                *int  `toml:"-"`
		SkipBcVersionCheck      *bool `toml:"-"`
		DatabaseHandles         *int  `toml:"-"`
		DatabaseCache           *int
		MainAccount             *common.Address `toml:",omitempty"`
		MinerThreads            *int            `toml:",omitempty"`
		ExtraData               hexutil.Bytes   `toml:",omitempty"`
		GasPrice                *big.Int
		TxPool                  *core.TxPoolConfig
		WitvoteCacheDir         *string
		WitvoteCachesInMem      *int
		WitvoteCachesOnDisk     *int
		WitvoteDatasetDir       *string
		WitvoteDatasetsInMem    *int
		WitvoteDatasetsOnDisk   *int
		EnablePreimageRecording *bool
		DocRoot                 *string              `toml:"-"`
		PowFake                 *bool                `toml:"-"`
		MaxMessageSize          *uint32              `toml:",omitempty"`
		MinimumAcceptedPOW      *float64             `toml:",omitempty"`
		PublicTopic             *whisperv5.TopicType `toml:",omitempty"`
		PublicTopicPw           *string              `toml:",omitempty"`
	}
	var dec Config
	if err := unmarshal(&dec); err != nil {
		return err
	}
	if dec.Genesis != nil {
		c.Genesis = dec.Genesis
	}
	if dec.NetworkId != nil {
		c.NetworkId = *dec.NetworkId
	}
	if dec.SyncMode != nil {
		c.SyncMode = *dec.SyncMode
	}
	if dec.LightServ != nil {
		c.LightServ = *dec.LightServ
	}
	if dec.LightPeers != nil {
		c.LightPeers = *dec.LightPeers
	}
	if dec.MaxPeers != nil {
		c.MaxPeers = *dec.MaxPeers
	}
	if dec.SkipBcVersionCheck != nil {
		c.SkipBcVersionCheck = *dec.SkipBcVersionCheck
	}
	if dec.DatabaseHandles != nil {
		c.DatabaseHandles = *dec.DatabaseHandles
	}
	if dec.DatabaseCache != nil {
		c.DatabaseCache = *dec.DatabaseCache
	}
	if dec.MainAccount != nil {
		c.MainAccount = *dec.MainAccount
	}
	if dec.MinerThreads != nil {
		c.MinerThreads = *dec.MinerThreads
	}
	if dec.ExtraData != nil {
		c.ExtraData = dec.ExtraData
	}
	if dec.GasPrice != nil {
		c.GasPrice = dec.GasPrice
	}
	if dec.TxPool != nil {
		c.TxPool = *dec.TxPool
	}
	if dec.WitvoteCacheDir != nil {
		c.WitvoteCacheDir = *dec.WitvoteCacheDir
	}
	if dec.WitvoteCachesInMem != nil {
		c.WitvoteCachesInMem = *dec.WitvoteCachesInMem
	}
	if dec.WitvoteCachesOnDisk != nil {
		c.WitvoteCachesOnDisk = *dec.WitvoteCachesOnDisk
	}
	if dec.WitvoteDatasetDir != nil {
		c.WitvoteDatasetDir = *dec.WitvoteDatasetDir
	}
	if dec.WitvoteDatasetsInMem != nil {
		c.WitvoteDatasetsInMem = *dec.WitvoteDatasetsInMem
	}
	if dec.WitvoteDatasetsOnDisk != nil {
		c.WitvoteDatasetsOnDisk = *dec.WitvoteDatasetsOnDisk
	}
	if dec.EnablePreimageRecording != nil {
		c.EnablePreimageRecording = *dec.EnablePreimageRecording
	}
	if dec.DocRoot != nil {
		c.DocRoot = *dec.DocRoot
	}
	if dec.PowFake != nil {
		c.PowFake = *dec.PowFake
	}
	if dec.MaxMessageSize != nil {
		c.MaxMessageSize = *dec.MaxMessageSize
	}
	if dec.MinimumAcceptedPOW != nil {
		c.MinimumAcceptedPOW = *dec.MinimumAcceptedPOW
	}
	if dec.PublicTopic != nil {
		c.PublicTopic = *dec.PublicTopic
	}
	if dec.PublicTopicPw != nil {
		c.PublicTopicPw = *dec.PublicTopicPw
	}
	return nil
}