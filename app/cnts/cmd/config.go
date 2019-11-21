// Author: @yin

package cmd

import (
	"gitee.com/nerthus/nerthus/node"
	"gitee.com/nerthus/nerthus/params"

	"gitee.com/nerthus/nerthus/nts"
)

type ntsConfig struct {
	NTS  nts.Config
	Node node.Config
}

// defaultNtsConfig load current cnts instance default config
// Author: @ysqi
func defaultNtsConfig() ntsConfig {
	nodeCfg := node.DefaultConfig
	nodeCfg.Name = clientIdentifier
	nodeCfg.Version = params.VersionWithCommit()
	// Author: @kulics 添加其它rpc配置
	nodeCfg.HTTPModules = append(nodeCfg.HTTPModules, "nts", "nccp")
	nodeCfg.WSModules = append(nodeCfg.WSModules, "nts", "nccp")
	nodeCfg.IPCPath = clientIdentifier + ".ipc"

	return ntsConfig{
		Node: nodeCfg,
		NTS:  nts.DefaultConfig,
	}
}
