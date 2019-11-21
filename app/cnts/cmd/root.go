// Author: @ysqi

// Package cmd 实现各类命令接入与命令帮助文档.
// 该cmd是基于github.com/spf13/cobra的实现，能轻松的管理命令、参数以及配置文件加载。
//
// 默认情况下如果$HOME/.cnts/cnts.yaml存在，则使用该文件作为配置文件加载配置。
// 但所支持的配置文件格式还有：json, toml, yaml等。
package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gitee.com/nerthus/nerthus/accounts/keystore"
	"gitee.com/nerthus/nerthus/app/utils"
	"gitee.com/nerthus/nerthus/core/config"
	"gitee.com/nerthus/nerthus/internal/debug"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/metrics"
	"gitee.com/nerthus/nerthus/node"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var (
	cfgFile          string // @ysqi: config file
	clientIdentifier = "cnts"
)

// RootCmd 属基础命令，任何命令均添加到此命令下方可生效
var RootCmd = &cobra.Command{
	Use:              clientIdentifier,
	Short:            "cnts is the main command for nerthus.",
	Long:             `cnts is nerthus command line interface, used to execute most of the commands. `,
	Run:              cntsConsole,
	TraverseChildren: true,
}

var (
	// Author: @kulics 增加部分配置
	nodeFlags = []*pflag.Flag{
		utils.IdentityFlag,
		utils.UnlockedAccountFlag,
		utils.PasswordFileFlag,
		utils.BootnodesFlag,
		utils.DataDirFlag,
		utils.DBDriverFlag,
		utils.KeyStoreDirFlag,
		utils.TxPoolNoDiskTxFlag,
		utils.TxPoolDiskTxPathFlag,
		utils.TxPoolDiskTxIntervalFlag,
		utils.TxPoolDiskTxTimeoutFlag,
		utils.TxPoolDisableTxSetupFlag,
		utils.LightKDFFlag,
		utils.MaxPeersFlag,
		utils.MaxPendingPeersFlag,
		utils.NoDiscoverFlag,
		utils.NodeKeyFileFlag,
		utils.NodeKeyHexFlag,
		utils.DevModeFlag,
		utils.TestnetFlag,
		utils.NetworkIPFlag,
		utils.ListenPortFlag,
		utils.ListenSubPortFlag,
		utils.NetrestrictFlag,
		utils.NATFlag,
	}
	// Author: @kulics 增加部分配置
	rpcFlags = []*pflag.Flag{
		utils.RPCEnabledFlag,
		utils.RPCListenAddrFlag,
		utils.RPCCORSDomainFlag,
		utils.RPCPortFlag,
		utils.RPCApiFlag,
		utils.WSEnabledFlag,
		utils.WSListenAddrFlag,
		utils.WSPortFlag,
		utils.WSApiFlag,
		utils.WSAllowedOriginsFlag,
		utils.IPCDisabledFlag,
		utils.IPCPathFlag,
	}
)

var (
	globalConfig ntsConfig
)

// Execute 执行RootCmd，只能由Main函数触发，且只能执行一次
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		utils.Fatal(err)
	}
}

func init() {
	rootFlags := RootCmd.Flags()

	for _, f := range nodeFlags {
		rootFlags.AddFlag(f)
	}
	for _, f := range rpcFlags {
		rootFlags.AddFlag(f)
	}

	rootFlags.AddFlagSet(debug.Flags)

	enableDebug(RootCmd)

	viper.SupportedExts = []string{"yaml"}
	// 绑定后，其他模块可直接从viper中获取参数
	viper.BindPFlags(rootFlags)

	rootFlags.StringVar(&cfgFile, "config", "",
		fmt.Sprintf("node configuration file,default load from current dictionary or %s", config.DefaultDataDir()))
}

// initConfig reads in config file and ENV variables if set.
func initConfig(name string) {
	if err := config.InitViper(nil, name); err != nil {
		utils.Fatal(err)
	}
	//加载配置
	if cfgFile != "" {
		if _, err := os.Stat(cfgFile); err != nil {
			utils.Fatal(err)
		}
		log.Info("Use custom config", "config", cfgFile)
		viper.SetConfigFile(cfgFile)
	}
	if err := viper.ReadInConfig(); err != nil {
		utils.Fatal(fmt.Errorf("failed to read config data: %v", err))
	}
	log.Info("Read config file done", "file", viper.ConfigFileUsed())
}

func cntsConsole(cmd *cobra.Command, args []string) {
	flags := utils.Flags{cmd.Flags()}
	nodeIns := makeFullNode(&flags)
	startNode(&flags, nodeIns)
	nodeIns.Wait()
	nodeIns.Stop()
}

// 生成节点配置和注册服务
// Author: @kulics
func makeFullNode(flags *utils.Flags) *node.Node {
	stack, cfg := makeConfigNode(flags)
	// 注册服务
	// Author: @ysqi 密语已成为Node内部服务不需要进行服务注册
	//utils.RegisterNccpService(stack, &cfg.Nccp)
	utils.RegisterNTSService(stack, &cfg.NTS)

	return stack
}

// 生成节点和配置
// Author: @kulics
func makeConfigNode(flags *utils.Flags) (*node.Node, ntsConfig) {
	// 读取默认值
	cfg := defaultNtsConfig()
	switch {
	case flags.GetBool(utils.DevModeFlag.Name):
		cfg.Node.Module = "dev"
	case flags.GetBool(utils.TestnetFlag.Name):
		cfg.Node.Module = "testnet"
	}
	if cfg.Node.Module == "" {
		initConfig(clientIdentifier)
	} else {
		initConfig(clientIdentifier + "." + cfg.Node.Module)
	}
	// 从文件获取配置
	//if flags.Changed("config") && cfgFile != "" {
	//	viper.AddConfigPath(cfgFile)
	//
	//	if err := viper.ReadInConfig(); err != nil {
	//		utils.Fatal(err)
	//	}
	//	// Unmarshal config to config struct
	//	// 暂时不经过此配置
	//	//if err := viper.Unmarshal(&globalConfig); err != nil {
	//	//	utils.Fatal(err)
	//	//}
	//}
	// 设置节点配置
	utils.SetNodeConfig(flags, &cfg.Node)
	stack, err := node.New(&cfg.Node)
	if err != nil {
		utils.Fatalf("Failed to create the protocol stack: %v", err)
	}
	// 设置系统配置
	utils.SetNtsConfig(flags, stack, &cfg.NTS)
	// TODO 暂时无此功能
	//	if flags.Changed(utils.EthStatsURLFlag.Name) {
	//		cfg.Ethstats.URL = flags.GetString(utils.EthStatsURLFlag.Name)
	//	}
	// 设置密语协议
	//utils.SetNccpConfig(flags, stack, &cfg.Nccp)
	return stack, cfg
}

// startNode boots up the system node and all registered protocols, after which
// it unlocks any requested accounts, and starts the RPC/IPC interfaces and the
// miner.
func startNode(flags *utils.Flags, stack *node.Node) {
	// Start up the node itself
	utils.StartNode(stack)

	// Unlock any account specifically requested
	ks := stack.AccountManager().Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)

	passwords := utils.MakePasswordList(flags)
	unlocks := flags.GetStringSlice(utils.UnlockedAccountFlag.Name)
	for i, account := range unlocks {
		if trimmed := strings.TrimSpace(account); trimmed != "" {
			unlockAccount(ks, trimmed, i, passwords)
		}
	}
}

// getFlags return all flag set of commands
func getFlags(cmds ...*cobra.Command) *utils.Flags {
	fs := utils.Flags{RootCmd.Flags()}
	for _, c := range cmds {
		fs.AddFlagSet(c.Flags())
	}
	return &fs
}

// enableDebug 启用Debug log,将在命令中处理
// Author: @ysqi
func enableDebug(cmd *cobra.Command) {
	if cmd.PreRun != nil {
		utils.Fatalf("cann't set func to command %s, only work while cmd.PreRun is nil", cmd.Use)
	}

	// Author: @zwj 交互命令操作前后处理
	// 进入前处理
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		// 日志初始化
		if err := debug.Setup(RootCmd); err != nil {
			return err
		}

		// 启动系统运行时指标收集
		// 函数内部将检查是否开启收集
		go metrics.CollectProcessMetrics(time.Second)

		return nil
	}

	// 退出后处理
	cmd.PostRun = func(cmd *cobra.Command, args []string) {
		debug.Exit()
	}
}
