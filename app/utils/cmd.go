// Author: @ysqi

package utils

import (
	"crypto/ecdsa"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"gitee.com/nerthus/nerthus/accounts/keystore"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/common/fdlimit"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/core/config"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/internal/debug"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/node"
	"gitee.com/nerthus/nerthus/nts"
	"gitee.com/nerthus/nerthus/nts/protocol"
	"gitee.com/nerthus/nerthus/p2p"
	"gitee.com/nerthus/nerthus/p2p/discover"
	"gitee.com/nerthus/nerthus/p2p/nat"
	"gitee.com/nerthus/nerthus/p2p/netutil"
	"gitee.com/nerthus/nerthus/params"
	"github.com/spf13/pflag"
)

// Fatalf formats a message to standard error and exits the program.
// The message is also printed to standard output if standard error
// is redirected to a different file.
func Fatalf(format string, args ...interface{}) {
	w := io.MultiWriter(os.Stdout, os.Stderr)
	if runtime.GOOS == "windows" {
		// The SameFile check below doesn't work on Windows.
		// stdout is unlikely to get redirected though, so just print there.
		w = os.Stdout
	} else {
		outf, _ := os.Stdout.Stat()
		errf, _ := os.Stderr.Stat()
		if outf != nil && errf != nil && os.SameFile(outf, errf) {
			w = os.Stderr
		}
	}
	fmt.Fprintf(w, "Fatal: "+format+"\n", args...)

	// Set exit code to zero for go test
	// go test will failed if exit code not equal zero.
	if flag.Lookup("test.v") != nil {
		os.Exit(0)
	} else {
		os.Exit(1)
	}
}
func Fatal(err error) {
	Fatalf("%s", err.Error())
}

func StartNode(stack *node.Node) {
	if err := stack.Start(); err != nil {
		Fatalf("Error starting protocol stack: %v", err)
	}
	go func() {
		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc, os.Interrupt)
		defer signal.Stop(sigc)
		<-sigc
		log.Info("Got interrupt, shutting down...")
		go stack.Stop()
		for i := 10; i > 0; i-- {
			<-sigc
			if i > 1 {
				log.Warn("Already shutting down, interrupt more to panic.", "times", i-1)
			}
		}
		debug.Exit() // ensure trace and CPU profile data is flushed.
		debug.LoudPanic("boom")
	}()
}

// setNodeKey creates a node key from set command line flags, either loading it
// from a file or as a specified hex value. If neither flags were provided, this
// method returns nil and an emphemeral key is to be generated.
func setNodeKey(f *Flags, cfg *p2p.Config) {
	var (
		hex  = f.GetString(NodeKeyHexFlag.Name)
		file = f.GetString(NodeKeyFileFlag.Name)
		key  *ecdsa.PrivateKey
		err  error
	)
	switch {
	case file != "" && hex != "":
		Fatalf("Options %q and %q are mutually exclusive", NodeKeyFileFlag.Name, NodeKeyHexFlag.Name)
	case file != "":
		if key, err = crypto.LoadECDSA(file); err != nil {
			Fatalf("Option %q: %v", NodeKeyFileFlag.Name, err)
		}
		cfg.PrivateKey = key
	case hex != "":
		if key, err = crypto.HexToECDSA(hex); err != nil {
			Fatalf("Option %q: %v", NodeKeyHexFlag.Name, err)
		}
		cfg.PrivateKey = key
	}
}

// setNodeUserIdent creates the user identifier from CLI flags.
func setNodeUserIdent(f *Flags, cfg *node.Config) {
	if identity := f.GetString(IdentityFlag.Name); len(identity) > 0 {
		cfg.UserIdent = identity
	}
}

// setBootstrapNodes creates a list of bootstrap nodes from the command line
// flags, reverting to pre-configured ones if none have been specified.
// Author: @kulics 补充功能
func setBootstrapNodes(f *Flags, cfg *p2p.Config) {
	urls := params.MainnetBootnodes
	switch {
	case f.Changed(BootnodesFlag.Name):
		urls = strings.Split(f.GetString(BootnodesFlag.Name), ",")
	case f.GetBool(TestnetFlag.Name):
		urls = params.TestnetBootnodes
	case f.GetBool(DevModeFlag.Name):
		urls = params.DevBootnodes
	}

	cfg.BootstrapNodes = make([]*discover.Node, 0, len(urls))
	for _, url := range urls {
		node, err := discover.ParseNode(url)
		if err != nil {
			log.Error("Bootstrap URL invalid", "enode", url, "err", err)
			continue
		}
		cfg.BootstrapNodes = append(cfg.BootstrapNodes, node)
	}
}

func IntranetIP() (ips []string, err error) {
	ips = make([]string, 0)

	ifaces, e := net.Interfaces()
	if e != nil {
		return ips, e
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // interface down
		}

		if iface.Flags&net.FlagLoopback != 0 {
			continue // loopback interface
		}

		// ignore docker and warden bridge
		if strings.HasPrefix(iface.Name, "docker") || strings.HasPrefix(iface.Name, "w-") || strings.HasPrefix(iface.Name, "VM") {
			continue
		}

		addrs, e := iface.Addrs()
		if e != nil {
			return ips, e
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil || ip.IsLoopback() {
				continue
			}

			ip = ip.To4()
			if ip == nil {
				continue // not an ipv4 address
			}

			ipStr := ip.String()
			if IsIntranet(ipStr) {
				ips = append(ips, ipStr)
			}
		}
	}

	return ips, nil
}

func IsIntranet(ipStr string) bool {
	if strings.HasPrefix(ipStr, "10.") || strings.HasPrefix(ipStr, "192.168.") {
		return true
	}

	if strings.HasPrefix(ipStr, "172.") {
		// 172.16.0.0-172.31.255.255
		arr := strings.Split(ipStr, ".")
		if len(arr) != 4 {
			return false
		}

		second, err := strconv.ParseInt(arr[1], 10, 64)
		if err != nil {
			return false
		}

		if second >= 16 && second <= 31 {
			return true
		}
	}

	return false
}

func GetLocalIP4() (string, error) {
	// Author: @kulics

	// Author: @yin
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		ips, err := IntranetIP()
		if err != nil {
			return "", err
		}
		if len(ips) > 0 {
			return ips[0], nil
		}
		return "", errors.New("not found local ip4")
	}
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	conn.Close()
	return localAddr.IP.String(), nil
}

// setListenAddress creates a TCP listening address string from set command
// line flags.
// Author: @kulics 补充功能
func setListenAddress(f *Flags, cfg *p2p.Config) {

	// @ysqi: 支持自定义IP
	cfg.ListenAddr = ""
	if f.Changed(NetworkIPFlag.Name) {
		cfg.ListenAddr = f.GetString(NetworkIPFlag.Name)
	} else if params.RunMode == "dev" {
		// TODO 设置监听本地局域网地址，以后有发现机制之后再去除
		ip, err := GetLocalIP4()
		if err != nil {
			Fatal(err)
		}
		cfg.ListenAddr = ip

	}
	var port string
	if f.Changed(ListenPortFlag.Name) {
		port = fmt.Sprintf("%d", f.GetInt(ListenPortFlag.Name))
	} else {
		port = ListenPortFlag.DefValue
	}

	cfg.ListenAddr += ":" + port

}

func setSecondListenAddress(f *Flags, cfg *nts.Config) {
	cfg.SubPipeListenAddr = ""
	if f.Changed(NetworkIPFlag.Name) {
		cfg.SubPipeListenAddr = f.GetString(NetworkIPFlag.Name)
	} else if params.RunMode == "dev" {
		ip, err := GetLocalIP4()
		if err != nil {
			Fatal(err)
		}
		cfg.SubPipeListenAddr = ip
	}
	var port string
	if f.Changed(ListenSubPortFlag.Name) {
		port = fmt.Sprintf("%d", f.GetInt(ListenSubPortFlag.Name))
	} else {
		port = ListenSubPortFlag.DefValue
	}

	cfg.SubPipeListenAddr += ":" + port
}

// setDiscoveryV5Address creates a UDP listening address string from set command
// line flags for the V5 discovery protocol.
// Author: @kulics 补充功能
func setDiscoveryV5Address(f *Flags, cfg *p2p.Config) {
	if f.Changed(ListenPortFlag.Name) {
		cfg.DiscoveryV5Addr = fmt.Sprintf(":%d", f.GetInt(ListenPortFlag.Name)+1)
	}
}

// setNAT creates a port mapper from command line flags.
// Author: @kulics 补充功能
func setNAT(f *Flags, cfg *p2p.Config) {
	if f.Changed(NATFlag.Name) {
		natif, err := nat.Parse(f.GetString(NATFlag.Name))
		if err != nil {
			Fatalf("Option %s: %v", NATFlag.Name, err)
		}
		cfg.NAT = natif
	}
}

// splitAndTrim splits input separated by a comma
// and trims excessive white space from the substrings.
// Author: @kulics 补充功能
func splitAndTrim(input string) []string {
	result := strings.Split(input, ",")
	for i, r := range result {
		result[i] = strings.TrimSpace(r)
	}
	return result
}

// setHTTP creates the HTTP RPC listener interface string from the set
// command line flags, returning empty if the HTTP endpoint is disabled.
// Author: @kulics 补充功能
func setHTTP(f *Flags, cfg *node.Config) {
	if f.GetBool(RPCEnabledFlag.Name) && cfg.HTTPHost == "" {
		cfg.HTTPHost = "127.0.0.1"
		if f.Changed(RPCListenAddrFlag.Name) {
			cfg.HTTPHost = f.GetString(RPCListenAddrFlag.Name)
		}
	}

	if f.Changed(RPCPortFlag.Name) {
		cfg.HTTPPort = f.GetInt(RPCPortFlag.Name)
	}
	if f.Changed(RPCCORSDomainFlag.Name) {
		cfg.HTTPCors = splitAndTrim(f.GetString(RPCCORSDomainFlag.Name))
	}
	if f.Changed(RPCApiFlag.Name) {
		cfg.HTTPModules = splitAndTrim(f.GetString(RPCApiFlag.Name))
	}
}

// setWS creates the WebSocket RPC listener interface string from the set
// command line flags, returning empty if the HTTP endpoint is disabled.
// Author: @kulics 补充功能
func setWS(f *Flags, cfg *node.Config) {
	if f.GetBool(WSEnabledFlag.Name) && cfg.WSHost == "" {
		cfg.WSHost = "127.0.0.1"
		if f.Changed(WSListenAddrFlag.Name) {
			cfg.WSHost = f.GetString(WSListenAddrFlag.Name)
		}
	}

	if f.Changed(WSPortFlag.Name) {
		cfg.WSPort = f.GetInt(WSPortFlag.Name)
	}
	if f.Changed(WSAllowedOriginsFlag.Name) {
		cfg.WSOrigins = splitAndTrim(f.GetString(WSAllowedOriginsFlag.Name))
	}
	if f.Changed(WSApiFlag.Name) {
		cfg.WSModules = splitAndTrim(f.GetString(WSApiFlag.Name))
	}
}

// setIPC creates an IPC path configuration from the set command line flags,
// returning an empty string if IPC was explicitly disabled, or the set path.
func setIPC(f *Flags, cfg *node.Config) {
	checkExclusive(f, IPCDisabledFlag, IPCPathFlag)
	switch {
	case f.GetBool(IPCDisabledFlag.Name):
		cfg.IPCPath = ""
	case f.Changed(IPCPathFlag.Name):
		cfg.IPCPath = f.GetString(IPCPathFlag.Name)
	}
}

// setTxPool
func setTxPool(f *Flags, cfg *nts.Config, stack *node.Node) {
	if f.Changed(TxPoolNoDiskTxFlag.Name) {
		cfg.TxPool.NoDiskTx = f.GetBool(TxPoolNoDiskTxFlag.Name)
	}
	if !cfg.TxPool.NoDiskTx {
		d := f.GetString(TxPoolDiskTxPathFlag.Name)
		if d == "" {
			Fatal(errors.New("params txpool.disktxpath required when txpool.nodisktx = true"))
		}
		cfg.TxPool.DiskTxPath = stack.ResolvePath(d)
		if f.Changed(TxPoolDiskTxTimeoutFlag.Name) {
			cfg.TxPool.DiskTxTimeout = f.GetDuration(TxPoolDiskTxTimeoutFlag.Name)
		}
		if f.Changed(TxPoolDiskTxIntervalFlag.Name) {
			cfg.TxPool.DiskTxInterval = f.GetDuration(TxPoolDiskTxIntervalFlag.Name)
		}
	}
	cfg.TxPool.DisableTODO = f.GetBool(TxPoolDisableTxSetupFlag.Name)
}

// RegisterNTSService adds an nerthus client to the stack.
func RegisterNTSService(stack *node.Node, cfg *nts.Config) {
	var err error
	if cfg.SyncMode == protocol.LightSync {
		//	 TODO(ysqi) create a light client
		Fatalf("light sync: Not yet implemented")
	} else {
		err = stack.Register(func(ctx *node.ServiceContext) (node.Service, error) {
			fullNode, err := nts.New(ctx, cfg)
			if fullNode != nil && cfg.LightServ > 0 {
				Fatalf("light sync: Not yet implemented")
			}
			return fullNode, err
		})
	}
	if err != nil {
		Fatalf("Failed to register the Nerthus service: %v", err)
	}
}

// setMainAccout retrieves the main accout either from the directly specified
// command line flags or from the keystore if CLI indexed.
func setMainAccout(f *Flags, ks *keystore.KeyStore, cfg *nts.Config) {
	if (cfg.MainAccount == common.Address{}) {
		accs := ks.Accounts()
		if len(accs) > 0 {
			cfg.MainAccount = accs[0].Address
		} else {
			//log.Warn("No main account set and no accounts found as default")
		}
	}
}

// MakePasswordList reads password lines from the file specified by the global --password flag.
func MakePasswordList(f *Flags) []string {
	path := f.GetString(PasswordFileFlag.Name)
	if path == "" {
		return nil
	}
	text, err := ioutil.ReadFile(path)
	if err != nil {
		Fatalf("Failed to read password file: %v", err)
	}
	lines := strings.Split(string(text), "\n")
	// Sanitise DOS line endings.
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], "\r")
	}
	return lines
}

func SetP2PConfig(f *Flags, cfg *p2p.Config) {
	// Author: @kulics 补充功能
	setNodeKey(f, cfg)
	setNAT(f, cfg)
	setListenAddress(f, cfg)
	setDiscoveryV5Address(f, cfg)
	setBootstrapNodes(f, cfg)

	if f.Changed(MaxPeersFlag.Name) {
		cfg.MaxPeers = f.GetInt(MaxPeersFlag.Name)
	}
	if f.Changed(MaxPendingPeersFlag.Name) {
		cfg.MaxPendingPeers = f.GetInt(MaxPendingPeersFlag.Name)
	}
	if f.Changed(NoDiscoverFlag.Name) {
		cfg.NoDiscovery = true
	}

	cfg.DiscoveryV5 = false

	if netrestrict := f.GetString(NetrestrictFlag.Name); netrestrict != "" {
		list, err := netutil.ParseNetlist(netrestrict)
		if err != nil {
			Fatalf("Option %q: %v", NetrestrictFlag.Name, err)
		}
		cfg.NetRestrict = list
	}

	//if f.GetBool(DevModeFlag.Name) {
	//	// --dev mode can't use p2p networking.
	//	cfg.MaxPeers = 0
	//	cfg.ListenAddr = ":0"
	//	cfg.DiscoveryV5Addr = ":0"
	//	cfg.NoDiscovery = true
	//	cfg.DiscoveryV5 = false
	//}
}

// 获取DataDir，如果不确定结果，则返回空
// Author: @ysqi
func GetDataDir(f *Flags) string {
	datadir := DataDirFlag.DefValue // default value =  node.DefaultDataDir()
	if f.Changed(DataDirFlag.Name) {
		datadir = f.GetString(DataDirFlag.Name)
	}
	// 如果有设置模式，则确定为不同位置
	switch {
	case f.GetBool(DevModeFlag.Name):
		datadir = filepath.Join(datadir, "dev")
	case f.GetBool(TestnetFlag.Name):
		datadir = filepath.Join(datadir, "testnet")
	}
	return datadir
}

// SetNodeConfig applies node-related command line flags to the config.
func SetNodeConfig(f *Flags, cfg *node.Config) {
	// 优先处理模式设置
	if f.GetBool(DevModeFlag.Name) {
		params.RunMode = "dev"
	} else if f.GetBool(TestnetFlag.Name) {
		params.RunMode = "testnet"
	}

	SetP2PConfig(f, &cfg.P2P)
	setIPC(f, cfg)
	// Author: @kulics 补充功能
	setHTTP(f, cfg)
	setWS(f, cfg)
	setNodeUserIdent(f, cfg)

	cfg.DataDir = GetDataDir(f)

	if f.Changed(DBDriverFlag.Name) {
		cfg.DBDriver = f.GetString(DBDriverFlag.Name)
	}

	if f.Changed(KeyStoreDirFlag.Name) {
		cfg.KeyStoreDir = f.GetString(KeyStoreDirFlag.Name)
	}
	if f.Changed(LightKDFFlag.Name) {
		cfg.UseLightweightKDF = f.GetBool(LightKDFFlag.Name)
	}
}

func checkExclusive(f *Flags, flags ...*pflag.Flag) {
	set := make([]string, 0, 1)
	for _, fv := range flags {
		if f.Changed(fv.Name) {
			set = append(set, "--"+fv.Name)
		}
	}
	if len(set) > 1 {
		Fatalf("flags %v can't be used at the same time", strings.Join(set, ", "))
	}
}

// SetNtsConfig applies eth-related command line flags to the config.
func SetNtsConfig(f *Flags, stack *node.Node, cfg *nts.Config) {
	// Avoid conflicting network flags
	checkExclusive(f, DevModeFlag, TestnetFlag)

	ks := stack.AccountManager().Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)
	setMainAccout(f, ks, cfg)
	setTxPool(f, cfg, stack)

	cfg.DatabaseCache = config.MustInt("leveldb.cache", cfg.DatabaseCache)
	cfg.DatabaseHandles = makeDatabaseHandles()

	cfg.MaxPeers = f.GetInt(MaxPeersFlag.Name)

	var err error
	cfg.Genesis, err = core.LoadGenesisConfig()
	if err != nil {
		Fatalf("failed load genesis config", err)
	}
	cfg.NetworkId = cfg.Genesis.Config.ChainId.Uint64()

	setSecondListenAddress(f, cfg)
}

// MakeConsolePreloads retrieves the absolute paths for the console JavaScript
// scripts to preload before starting.
func MakeConsolePreloads(f *Flags) []string {
	files := f.GetStringSlice(PreloadJSFlag.Name)
	// Skip preloading if there's nothing to preload
	if len(files) == 0 {
		return nil
	}
	// Otherwise resolve absolute paths and return them
	preloads := []string{}

	assets := f.GetString(JSpathFlag.Name)
	for _, file := range files {
		preloads = append(preloads, common.AbsolutePath(assets, strings.TrimSpace(file)))
	}
	return preloads
}

// makeDatabaseHandles raises out the number of allowed file handles per process
// for Geth and returns half of the allowance to assign to the database.
func makeDatabaseHandles() int {
	limit, err := fdlimit.Maximum()
	if err != nil {
		Fatalf("Failed to retrieve file descriptor allowance: %v", err)
	}
	if limit > 10000 {
		limit = 7168
	}
	if err := fdlimit.Raise(uint64(limit)); err != nil {
		Fatalf("Failed to raise file descriptor allowance: %v %d", err, limit)
	}
	//不需要打开太多文件，当程序设置不当时将导致更多文件被打开
	// 默认情况下：
	// 		Mac: 7168
	// 		Windows: 2048
	//
	if limit > 10000 {
		return 7168 / 2
	}
	return limit / 2 // Leave half for networking and other stuff
}
