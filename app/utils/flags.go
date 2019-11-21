// Author: @ysqi

package utils

import (
	"math/big"
	"reflect"
	"strings"
	"time"

	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/config"
	"gitee.com/nerthus/nerthus/node"
	"gitee.com/nerthus/nerthus/nts"
	"github.com/spf13/pflag"
)

var (
	defaultFlags = &Flags{&pflag.FlagSet{}}
)

// define all flags
var (
	DataDirFlag = newFilepathFlag("datadir", config.OfficialPath, "Data directory for the databases and keystore")
	//暂时移除对badger的支持，因为badger尚未重复测试
	DBDriverFlag = newChooseFlag("dbdriver", "leveldb", "block chain data store database driver type", "leveldb")

	KeyStoreDirFlag = newStringFlag("keystore", "", "Directory for the keystore (default = inside the node workspace)")
	TestnetFlag     = newBoolFlag("testnet", false, "Nerthus test network")
	DevModeFlag     = newBoolFlag("dev", false, "Developer mode: pre-configured private network with several debugging flags")

	IdentityFlag = newStringFlag("identity", "", "Custom node name and node workspace dir name (default = inside the datedir)")
	LightKDFFlag = newBoolFlag("lightkdf", false, "Reduce key-derivation RAM & CPU usage at some expense of KDF strength")

	UnlockedAccountFlag = newStringSliceFlag("unlock", nil, "Comma separated list of accounts to unlock")
	PasswordFileFlag    = newFilepathFlag("password", "", "Password file to use for non-inteactive password input")

	TxPoolNoDiskTxFlag       = newBoolFlag("txpool.nodisktx", nts.DefaultConfig.TxPool.NoDiskTx, "Disables transaction disk persists for received transaction")
	TxPoolDiskTxPathFlag     = newStringFlag("txpool.disktxpath", nts.DefaultConfig.TxPool.DiskTxPath, "Disk persists for receive transaction to survive node restarts")
	TxPoolDiskTxIntervalFlag = newDurationFlag("txpool.disktxinterval", nts.DefaultConfig.TxPool.DiskTxInterval, "Disk persists tx's interval(second)")
	TxPoolDiskTxTimeoutFlag  = newDurationFlag("txpool.disktxtimeout", nts.DefaultConfig.TxPool.DiskTxTimeout, "Disk persists tx's timeout(second)")
	TxPoolDisableTxSetupFlag = newBoolFlag("txpool.disableTxSetup", nts.DefaultConfig.TxPool.DisableTODO, "Whether to disable the tx pool record tx pending details, witness node must enable")

	RPCEnabledFlag    = newBoolFlag("rpc", false, "Enable the HTTP-RPC server")
	RPCListenAddrFlag = newStringFlag("rpcaddr", node.DefaultHTTPHost, "HTTP-RPC server listening interface")
	RPCPortFlag       = newIntFlag("rpcport", node.DefaultHTTPPort, "HTTP-RPC server listening port")
	RPCCORSDomainFlag = newStringFlag("rpccorsdomain", "", "Comma separated list of domains from which to accept cross origin requests (browser enforced)")
	RPCApiFlag        = newStringFlag("rpcapi", "", "API's offered over the HTTP-RPC interface")

	IPCDisabledFlag = newBoolFlag("ipcdisable", false, "Disable the IPC-RPC server")
	IPCPathFlag     = newFilepathFlag("ipcpath", "", "Filename for IPC socket/pipe within the datadir (explicit paths escape it)")

	WSEnabledFlag        = newBoolFlag("ws", false, "Enable the WS_RPC server")
	WSListenAddrFlag     = newStringFlag("wsaddr", node.DefaultWSHost, "WS-RPC server listening interface")
	WSPortFlag           = newIntFlag("wsport", node.DefaultWSPort, "WS-RPC server listening port")
	WSApiFlag            = newStringFlag("wsapi", "", "API's offered over the WS-RPC interface")
	WSAllowedOriginsFlag = newStringFlag("wsorigins", "", "Origins from which to accept websockets requests")

	NodeKeyFileFlag = newStringFlag("nodekey", "", "P2P node key file")
	NodeKeyHexFlag  = newStringFlag("nodekeyhex", "", "P2P node key as hex (for testing)")

	// Network Settings
	MaxPeersFlag        = newIntFlag("maxpeers", 25, "Maximum number of network peers (network disabled if set to 0)")
	MaxPendingPeersFlag = newIntFlag("maxpendpeers", 0, "Maximum number of pending connection attempts (defaults used if set to 0)")
	ListenPortFlag      = newIntFlag("port", 60101, "Network listening port")
	ListenSubPortFlag   = newIntFlag("subport", 60102, "The second network listening port")
	NetworkIPFlag       = newStringFlag("ip", "", "Network listening interface")
	BootnodesFlag       = newStringFlag("bootnodes", "", "Comma separated enode URLs for P2P discovery bootstrap")
	NATFlag             = newStringFlag("nat", "any", "NAT port mapping mechanism (any|none|upnp|pmp|extip:<IP>)")
	NoDiscoverFlag      = newBoolFlag("nodiscover", false, "Disables the peer discovery mechanism (manual peer addition)")
	NetrestrictFlag     = newStringFlag("netrestrict", "", "Restricts network communication to the given IP networks (CIDR masks)")

	// JavaScript Console
	JSpathFlag    = newStringFlag("jspath", ".", "JavaScript root path for `loadScript`")
	ExecFlag      = newStringFlag("exec", "", "Execute JavaScript statement")
	PreloadJSFlag = newStringSliceFlag("preload", nil, "Comma separated list of JavaScript files to preload into the console")

	// Export/Import
	ExportPathFlag  = newStringFlag("path", "", "Export/Import chain data file path")
	ExportSinceFlag = newUnit64Flag("since", 0, "export chain data start at timestamp")
	// 开发调试相关
	//MetricsEnabledFlag = newBoolFlag(metrics.MetricsEnabledFlag, metrics.Enabled, "Enable metrics collection and reporting")
)

func newChooseFlag(name, value string, usage string, items ...string) *pflag.Flag {
	c := ChooseString{items: items, val: value}
	defaultFlags.Var(&c, name, usage)
	return defaultFlags.Lookup(name)
}

func newFilepathFlag(name, val, usage string) *pflag.Flag {
	f := FilepathString(expandPath(val))
	defaultFlags.Var(&f, name, usage)
	return defaultFlags.Lookup(name)
}
func newBoolFlag(name string, value bool, usage string) *pflag.Flag {
	defaultFlags.Bool(name, value, usage)
	return defaultFlags.Lookup(name)
}

func newStringFlag(name string, value string, usage string) *pflag.Flag {
	defaultFlags.String(name, value, usage)
	return defaultFlags.Lookup(name)
}

func newStringSliceFlag(name string, value []string, usage string) *pflag.Flag {
	defaultFlags.StringSlice(name, value, usage)
	return defaultFlags.Lookup(name)
}

func newUnit64Flag(name string, value uint64, usage string) *pflag.Flag {
	defaultFlags.Uint64(name, value, usage)
	return defaultFlags.Lookup(name)
}

func newIntFlag(name string, value int, usage string) *pflag.Flag {
	defaultFlags.Int(name, value, usage)
	return defaultFlags.Lookup(name)
}
func newDurationFlag(name string, value time.Duration, usage string) *pflag.Flag {
	defaultFlags.Duration(name, value, usage)
	return defaultFlags.Lookup(name)
}
func newBigIntFlag(name string, value *big.Int, usage string) *pflag.Flag {
	if value == nil {
		value = common.Big0
	}
	b := (BigValue)(*value)
	defaultFlags.Var(&b, name, usage)
	return defaultFlags.Lookup(name)
}

// Author: @kulics 增加浮点数类型
func newFloat64Flag(name string, value float64, usage string) *pflag.Flag {
	defaultFlags.Float64(name, value, usage)
	return defaultFlags.Lookup(name)
}

// Flags the pflag.Flag Set extended
type Flags struct {
	*pflag.FlagSet
}

func (f *Flags) GetString(name string) string {
	v, err := f.FlagSet.GetString(name)
	if err != nil {
		Fatal(err)
	}
	return v
}

func (f *Flags) GetBool(name string) bool {
	v, err := f.FlagSet.GetBool(name)
	if err != nil {
		Fatal(err)
	}
	return v
}

func (f *Flags) GetStringSlice(name string) []string {
	v, err := f.FlagSet.GetStringSlice(name)
	if err != nil {
		Fatal(err)
	}
	for i, r := range v {
		v[i] = strings.TrimSpace(r)
	}
	return v
}

func (f *Flags) GetUint64(name string) uint64 {
	v, err := f.FlagSet.GetUint64(name)
	if err != nil {
		Fatal(err)
	}
	return v
}

func (f *Flags) GetUint32(name string) uint32 {
	v, err := f.FlagSet.GetUint32(name)
	if err != nil {
		Fatal(err)
	}
	return v
}

func (f *Flags) GetFloat64(name string) float64 {
	v, err := f.FlagSet.GetFloat64(name)
	if err != nil {
		Fatal(err)
	}
	return v
}

func (f *Flags) GetInt(name string) int {
	v, err := f.FlagSet.GetInt(name)
	if err != nil {
		Fatal(err)
	}
	return v
}

func (f *Flags) GetBigInt(name string) *big.Int {
	flag := f.FlagSet.Lookup(name)
	if flag == nil {
		Fatalf("flag accessed but not defined: %s", name)
	}
	if flag.Value.Type() != "bigint" {
		Fatalf("trying to get bigint value of flag of type %s", flag.Value.Type())
	}
	bv, ok := flag.Value.(*BigValue)
	if !ok {
		Fatalf("trying to get bigint value of flag of type %s,but not BigValue", reflect.TypeOf(flag.Value).String())
	}
	return (*big.Int)(bv)
}

func (f *Flags) GetDuration(name string) time.Duration {
	v, err := f.FlagSet.GetDuration(name)
	if err != nil {
		Fatal(err)
	}
	return v
}
