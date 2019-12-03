// Author: @ysqi

package node

import (
	"crypto/ecdsa"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gitee.com/nerthus/nerthus/accounts"
	"gitee.com/nerthus/nerthus/accounts/keystore"
	"gitee.com/nerthus/nerthus/accounts/usbwallet"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/core/config"
	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/p2p"
	"gitee.com/nerthus/nerthus/p2p/discover"
	"gitee.com/nerthus/nerthus/params"
)

const (
	datadirPrivateKey      = "nodekey"            // Path within the datadir to the node's private key
	datadirDefaultKeyStore = "keystore"           // Path within the datadir to the keystore
	datadirStaticNodes     = "static-nodes.json"  // Path within the datadir to the static node list
	datadirTrustedNodes    = "trusted-nodes.json" // Path within the datadir to the trusted node list
	datadirNodeDatabase    = "nodes"              // Path within the datadir to store the node information
)

// Config 节点配置，节点运行可直接载入使用该配置。
// 配置含有P2P、网络、数据库、数据目录等信息
type Config struct {
	// Name 节点实例名称，不得含有特殊字符‘/‘，'\'，将在p2p网络中作为节点标识符的一部分。
	// 默认节点实例名称为“cnts”，如果不设置此名称，则使用当前运行程序名称。
	Name string `toml:"-"`

	// UserIdent，如果设置，则作为节点标识符的一个附件部分。
	UserIdent string `toml:",omitempty"`

	// Version 当前所运行的cnts版本，也将是节点标识符的一部分。
	Version string `toml:"-"`

	// DataDir 节点本地数据存放目录，该目录文件不被直接共享，仅供当前程序创建、访问。
	DataDir string

	// DBDriver 数据库类型（LevelDB或者BoltDB）
	DBDriver string `toml:",omitempty"`

	// P2P 点对点网络配置。
	P2P p2p.Config

	// KeyStoreDir 私钥文件存放目录。
	//
	// 如果为空，则默认位置为 DataDir 下的子目录 “keystore”。
	// 而此时如果 DataDir 也为空，则在节点运行时自动创建，节点停止运行后自动销毁。
	KeyStoreDir string `toml:",omitempty"`

	// UseLightweightKDF 是否降低秘钥存储器的CPU和内存要求，以牺牲安全性低要求加密KDF。
	UseLightweightKDF bool `toml:",omitempty"`

	// NoUSB 是否禁用硬件钱包的监控与连接。
	NoUSB bool `toml:",omitempty"`

	// IPCPATH 存放进程间通信端点（IPC endpoint）通道文件路径。
	// 如果设置的仅仅是文件名，则默认保存到 DataDir 下。如果是可访问的路径，则强制使用。
	// 如果为空，则表示禁用IPC。
	IPCPath string `toml:",omitempty"`

	// HTTPHost 开启 HTTP PRC 服务的主机地址。如果为空，则不开启HTTP PRC服务。
	HTTPHost string `toml:",omitempty"`

	// HTTPPort 开启 HTTP PRC 服务器的TCP的端口。 如果为0，则随机选择一个端口（实用与临时节点）。
	HTTPPort int `toml:",omitempty"`

	// HTTPCors 跨域请求资源的头部信息。
	//
	// 注意：CORS是浏览器一中安全检查项，对于自定义HTTP请求完全无用。
	HTTPCors []string `toml:",omitempty"`

	// HTTPModules HTTP PRC 服务所允许访问的API清单。
	// 如果为空，则PRC能受理所有API请求
	HTTPModules []string `toml:",omitempty"`

	// WSHost 开启 Websocket PRC 服务的主机地址，如果为空，则不开启Websocket PRC 服务。
	WSHost string `toml:",omitempty"`

	// WSPort 开启 Websocket PRC  服务器的TCP的端口。 如果为0，则随机选择一个端口（实用与临时节点）。
	WSPort int `toml:",omitempty"`

	// WSOrigins 能受理的 Websocket 请求的域白名单。
	//
	// 注意：节点仅能通过HTTP头部信息进行检查，但无法验证请求头的有效性。
	WSOrigins []string `toml:",omitempty"`

	// WSModules 是websocket PRC 服务所允许访问的API清单。
	// 如果为空，则 是websocket PRC能受理所有API请求
	WSModules []string `toml:",omitempty"`

	// WSExposeAll exposes all API modules via the WebSocket RPC interface rather
	// than just the public ones.
	//
	// *WARNING* Only set this if the node is running in a trusted network, exposing
	// private APIs to untrusted users is a major security risk.
	// Author: @kulics 补充属性
	WSExposeAll bool `toml:",omitempty"`

	// 节点模式
	Module string
}

// IPCEndpoint 根据配置选择一个IPC端点通道文件路径。
// 如果设置的IPCPath仅仅是文件名，则默认使用  DataDir 下。如果是 windows 平台，则将保存到\\.\pipe\下。
func (c *Config) IPCEndpoint() string {
	// 为空，则不表示不开启
	if c.IPCPath == "" {
		return ""
	}
	var module string
	if c.Module != "" {
		module = c.Module + "."
	}
	// Windows下，只能使用普通的顶级通道
	if runtime.GOOS == "windows" {
		if strings.HasPrefix(c.IPCPath, `\\.\pipe\`+module) {
			return c.IPCPath
		}
		return `\\.\pipe\` + module + c.IPCPath
	}
	// 如果仅仅是文件名，则将其作为DataDir目录下文件，返回完整的路径
	if filepath.Base(c.IPCPath) == c.IPCPath {
		if c.DataDir == "" {
			return filepath.Join(os.TempDir(), c.IPCPath)
		}
		return filepath.Join(c.DataDir, c.IPCPath)
	}
	return c.IPCPath
}

// DefaultIPCEndpoint 返回默认的 IPC Endpoint 通道地址，其clientIdentifier作为文件名。
func DefaultIPCEndpoint(clientIdentifier string) string {
	if clientIdentifier == "" {
		clientIdentifier = strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
		if clientIdentifier == "" {
			panic("empty executable name")
		}
	}
	cfg := &Config{DataDir: config.OfficialPath, IPCPath: clientIdentifier + ".ipc"}
	return cfg.IPCEndpoint()
}

// HTTPEndpoint 基于配置解析 HTTP PRC 服务地址。格式：HTTPHost:HTTPPort
func (c *Config) HTTPEndpoint() string {
	if c.HTTPHost == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d", c.HTTPHost, c.HTTPPort)
}

// DefaultHTTPEndpoint 返回默认的HTTP PRC 服务地址
func DefaultHTTPEndpoint() string {
	config := &Config{HTTPHost: DefaultHTTPHost, HTTPPort: DefaultHTTPPort}
	return config.HTTPEndpoint()
}

// WSEndpoint 基于配置解析 Websocket PRC 服务地址。格式：WSHost:WSPort
func (c *Config) WSEndpoint() string {
	if c.WSHost == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d", c.WSHost, c.WSPort)
}

// DefaultWSEndpoint 返回默认的Websocket PRC 服务地址
func DefaultWSEndpoint() string {
	config := &Config{WSHost: DefaultWSHost, WSPort: DefaultWSPort}
	return config.WSEndpoint()
}

// NodeName 返回点对点网络节点标识符名称
//
// 比如： Geth/MyIdent/v1.6.7-stable-ab5646c5/darwin-amd64/go1.9
func (c *Config) NodeName() string {
	name := c.name()

	if c.UserIdent != "" {
		name += "/" + c.UserIdent
	}
	if c.Version != "" {
		name += "/v" + c.Version
	}
	return fmt.Sprintf("%s/%s-%s/%s",
		name,
		runtime.GOOS, runtime.GOARCH,
		runtime.Version(),
	)
}

// Node 数据存储文件
func (c *Config) NodeDB() string {
	if c.DataDir == "" {
		return ""
	}
	return c.ResolvePath(datadirNodeDatabase)
}

func (c *Config) name() string {
	if c.Name == "" {
		progname := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
		if progname == "" {
			panic("empty executable name, set Config.Name")
		}
		return progname
	}
	return c.Name
}

// resolvePath 基于配置解析出path实际目录
func (c *Config) ResolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if c.DataDir == "" {
		return ""
	}
	return filepath.Join(c.instanceDir(), path)
}

func (c *Config) instanceDir() string {
	if c.DataDir == "" {
		return ""
	}
	return filepath.Join(c.DataDir, c.name())
}

// NodeKey 检查并获取当前节点所配置的私钥。
// 首先检查是否在P2P中有配置私钥，如果有则直接使用，否则从配置中解析。
// 如果为空，则自动创建并保存。
func (c *Config) NodeKey() *ecdsa.PrivateKey {
	// 使用特定配置
	if c.P2P.PrivateKey != nil {
		return c.P2P.PrivateKey
	}
	// 如果 DataDir 不存在，则自动创建
	if c.DataDir == "" {
		key, err := crypto.GenerateKey()
		if err != nil {
			log.Crit(fmt.Sprintf("Failed to generate ephemeral node key: %v", err))
		}
		return key
	}

	keyfile := c.ResolvePath(datadirPrivateKey)
	if key, err := crypto.LoadECDSA(keyfile); err == nil {
		return key
	} else if !os.IsNotExist(err) {
		log.Crit(fmt.Sprintf("Failed to read node key: %v", err))
	}

	// 无私钥文件，则创建并保存一个新的。
	key, err := crypto.GenerateKey()
	if err != nil {
		log.Crit(fmt.Sprintf("Failed to generate node key: %v", err))
	}
	instanceDir := c.instanceDir()
	if err := os.MkdirAll(instanceDir, 0700); err != nil {
		log.Error(fmt.Sprintf("Failed to persist node key: %v", err))
		return key
	}
	keyfile = filepath.Join(instanceDir, datadirPrivateKey)
	if err := crypto.SaveECDSA(keyfile, key); err != nil {
		log.Error(fmt.Sprintf("Failed to persist node key: %v", err))
	}
	return key
}

// StaticNodes 返回所配置的静态节点清单
func (c *Config) StaticNodes() []*discover.Node {
	return c.parsePersistentNodes(c.ResolvePath(datadirStaticNodes))
}

// TrustedNodes 返回溯配置的可信任节点清单
func (c *Config) TrustedNodes() []*discover.Node {
	return c.parsePersistentNodes(c.ResolvePath(datadirTrustedNodes))
}

// parsePersistentNodes 从json文件中解析节点连接信息
func (c *Config) parsePersistentNodes(path string) []*discover.Node {
	// 如果未启用DataDir则终止
	if c.DataDir == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	// 从配置文件解析多行node
	var nodelist []string
	if err := common.LoadJSON(path, &nodelist); err != nil {
		log.Error(fmt.Sprintf("Can't load node file %s: %v", path, err))
		return nil
	}

	var nodes []*discover.Node
	for _, url := range nodelist {
		if url == "" {
			continue
		}
		// 解析
		node, err := discover.ParseNode(url)
		if err != nil {
			log.Error(fmt.Sprintf("Node URL %s: %v\n", url, err))
			continue
		}
		nodes = append(nodes, node)
	}
	return nodes
}

// makeAccountManager 创建AcccoutManager管理器
// 返回 AccountManager和临时的数据存放存放路径
func makeAccountManager(conf *Config) (*accounts.Manager, string, error) {
	var (
		scryptN int
		scryptP int

		keydir    string
		ephemeral string
		err       error
	)
	if conf.UseLightweightKDF {
		scryptN = keystore.LightScryptN
		scryptP = keystore.LightScryptP
	} else {
		scryptN = keystore.StandardScryptN
		scryptP = keystore.StandardScryptP
	}

	switch {
	case filepath.IsAbs(conf.KeyStoreDir):
		keydir = conf.KeyStoreDir
	case conf.DataDir != "" && conf.KeyStoreDir == "":
		keydir = filepath.Join(conf.DataDir, datadirDefaultKeyStore)
	case conf.KeyStoreDir != "":
		keydir, err = filepath.Abs(conf.KeyStoreDir)
	default:
		keydir, err = ioutil.TempDir("", "cnts-keystore")
		ephemeral = keydir
	}
	if err != nil {
		return nil, "", err
	}
	if err := os.MkdirAll(keydir, 0700); err != nil {
		return nil, "", err
	}

	ks := keystore.NewKeyStore(keydir, scryptN, scryptP)
	// @ysqi load dev account if run with dev mode
	// create it at New function,rather Start function.
	// 使用文件方式复制私钥文件速度最快，而其他导入私钥方式每个账户需要花费1秒时间。
	if params.RunMode == "dev" || params.RunMode == "test" {
		keydatas := params.GetDevAccounts()
		if len(keydatas) > 0 {
			log.Warn("Sync import development account", "sum", len(keydatas))

			var createCount int
			for _, acc := range keydatas {
				if len(acc.KeyContent) == 0 {
					continue
				}
				addr := common.ForceDecodeAddress(acc.Address)
				if ks.HasAddress(addr) {
					continue
				}
				createCount++
				//文件名格式是 keystore 默认的私钥文件命名方式，固定为一个确定值。
				name := filepath.Join(keydir, keystore.KeyFileName(common.ForceDecodeAddress(acc.Address)))
				_, err = os.Stat(name)
				// 不存在此文件时进行添加，否则忽略
				if os.IsNotExist(err) {
					err = ioutil.WriteFile(name, []byte(acc.KeyContent), 0600)
				} else if os.IsExist(err) {
					// 忽略
					err = nil
				}
				if err != nil {
					log.Error("Sync import development account", "address", acc.Address, "err", err)
				}
			}
			log.Warn("Sync import development account done", "created new account", createCount)
		}
	}

	// 构建一个账户管理器
	backends := []accounts.Backend{ks}
	// 如果允许，则启动USB接口的硬件钱包
	if !conf.NoUSB {
		// Ledger硬件钱包
		if ledgerhub, err := usbwallet.NewLedgerHub(); err != nil {
			log.Warn(fmt.Sprintf("Failed to start Ledger hub, disabling: %v", err))
		} else {
			backends = append(backends, ledgerhub)
		}
		// Trezo 硬件钱包
		if trezorhub, err := usbwallet.NewTrezorHub(); err != nil {
			log.Warn(fmt.Sprintf("Failed to start Trezor hub, disabling: %v", err))
		} else {
			backends = append(backends, trezorhub)
		}
	}

	return accounts.NewManager(backends...), ephemeral, nil
}
