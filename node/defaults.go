// Author @ysqi

package node

import (
	"gitee.com/nerthus/nerthus/core/config"
	"gitee.com/nerthus/nerthus/p2p"
	"gitee.com/nerthus/nerthus/p2p/nat"
)

const (
	DefaultHTTPHost = "127.0.0.1" // 默认的 HTTP RPC 主机地址
	DefaultHTTPPort = 8655        // 默认的 HTTP RPC 端口
	DefaultWSHost   = "127.0.0.1" // 默认的 websocket 主机地址
	DefaultWSPort   = 8656        // 默认的 websocket
)

// DefaultConfig 默认配置信息
var DefaultConfig = Config{
	DataDir:     config.OfficialPath,
	DBDriver:    "leveldb",
	HTTPPort:    DefaultHTTPPort,
	HTTPModules: []string{"net", "web3"},
	WSPort:      DefaultWSPort,
	WSModules:   []string{"net", "web3"},
	P2P: p2p.Config{
		ListenAddr:      ":60101",
		DiscoveryV5Addr: ":60102",
		MaxPeers:        25,
		NAT:             nat.Any(),
	},
}
