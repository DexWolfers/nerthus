// Author: @ysqi

// Package node 是节点运行的起点
package node

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gitee.com/nerthus/nerthus/accounts"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/internal/debug"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/p2p"
	"gitee.com/nerthus/nerthus/rpc"

	"github.com/prometheus/prometheus/util/flock"
)

// Node 携带服务的节点
type Node struct {
	eventmux *event.TypeMux // 多路事件服务
	config   *Config
	accman   *accounts.Manager

	ephemeralKeystore string         // 如果不为空，则在停止节点时将销毁此目录
	instanceDirLock   flock.Releaser // 对节点实例文件提供锁

	serverConfig p2p.Config  // p2p配置
	server       *p2p.Server // 当前所允许的点对点网络服务

	serviceFuncs []ServiceConstructor     // 服务构建器 （有顺序关系）
	services     map[reflect.Type]Service // 当前所允许的服务清单

	// Author: @kulics 补充rpc相关的属性
	rpcAPIs       []rpc.API   // RPC的api
	inprocHandler *rpc.Server // RPC的服务端

	ipcEndpoint string       // IPC 端点服务服务运行地址，空表示禁用IPC
	ipcListener net.Listener // IPC RPC 监听器
	ipcHandler  *rpc.Server  // IPC RPC 请求handle

	httpEndpoint  string       // HTTP 服务运行地址，空表示禁用 HTTP PRC
	httpWhitelist []string     // HTTP RPC 模块的白名单端点
	httpListener  net.Listener // HTTP RPC 监听器
	httpHandler   *rpc.Server  // HTTP RPC 请求handle

	wsEndpoint string       // Websocket 服务运行地址，空表示禁用 websocket
	wsListener net.Listener // Websocket RPC 监听器
	wsHandler  *rpc.Server  // Websocket RPC 请求handle

	stop chan struct{} // 等待退出节点运行的信号Channel
	lock sync.RWMutex
}

var (
	// Node实例名称只允许使用数字、字母和特殊字符"."、"-"、"_"
	regNodeName = regexp.MustCompile(`^[\w\.-]+[^.]$`)
)

// NewDag 创建一个新的P2P节点，可注册服务
func New(conf *Config) (*Node, error) {
	// 复制一份配置，防止污染默认的Config信息
	confCopy := *conf
	conf = &confCopy
	if conf.DataDir != "" {
		absdatadir, err := filepath.Abs(conf.DataDir)
		if err != nil {
			return nil, err
		}
		conf.DataDir = absdatadir
	}

	if conf.Name != "" {
		// 不得使用特殊名称
		if conf.Name == datadirDefaultKeyStore {
			return nil, fmt.Errorf("Config.Name cannot be %q", datadirDefaultKeyStore)
		}
		if !regNodeName.MatchString(conf.Name) {
			return nil, fmt.Errorf("Config.Name can only contains [a-zA-Z0-9_.-] and cannot end with '.',the current is %q", conf.Name)
		}
		if strings.HasSuffix(strings.ToLower(conf.Name), ".ipc") {
			return nil, errors.New("Config.Name cannot end with .ipc")
		}
	}

	// 在启动节点实例前，需正常加载Account管理器
	am, ephemeralKeystore, err := makeAccountManager(conf)
	if err != nil {
		return nil, err
	}

	// 返回实例对象，但需注意：需要延长到节点启动后，才读取、创建文件
	return &Node{
		accman:            am,
		ephemeralKeystore: ephemeralKeystore,
		config:            conf,
		serviceFuncs:      []ServiceConstructor{},
		ipcEndpoint:       conf.IPCEndpoint(),
		httpEndpoint:      conf.HTTPEndpoint(),
		wsEndpoint:        conf.WSEndpoint(),
		eventmux:          new(event.TypeMux),
	}, nil
}

// Register 注册服务到节点实例。
// 需确保一种服务类型只注册一次,且只能在节点实例启动前注册
func (n *Node) Register(constructor ServiceConstructor) error {
	n.lock.Lock()
	defer n.lock.Unlock()

	if n.server != nil {
		return ErrNodeRunning
	}
	n.serviceFuncs = append(n.serviceFuncs, constructor)
	return nil
}

// Start 开启P2P并运行节点实例
func (n *Node) Start() error {
	n.lock.Lock()
	defer n.lock.Unlock()

	// 不得重复启动
	if n.server != nil {
		return ErrNodeRunning
	}
	if err := n.openDataDir(); err != nil {
		return err
	}

	// 开启P2P服务
	n.serverConfig = n.config.P2P
	n.serverConfig.PrivateKey = n.config.NodeKey()
	n.serverConfig.Name = n.config.NodeName()
	if n.serverConfig.StaticNodes == nil {
		n.serverConfig.StaticNodes = n.config.StaticNodes()
	}

	if n.serverConfig.TrustedNodes == nil {
		n.serverConfig.TrustedNodes = n.config.TrustedNodes()
	}

	// @zwj
	// 节点信息数据库文件
	if n.serverConfig.NodeDatabase == "" {
		n.serverConfig.NodeDatabase = n.config.NodeDB()
	}

	running := &p2p.Server{Config: n.serverConfig}
	log.Info("Starting peer-to-peer node", "instance", n.serverConfig.Name)

	services := make(map[reflect.Type]Service)
	for _, constructor := range n.serviceFuncs {
		// 给服务构造器提供上下文
		ctx := &ServiceContext{
			config:         n.config,
			ServerConfig:   n.serverConfig,
			Server:         running,
			services:       make(map[reflect.Type]Service),
			EventMux:       n.eventmux,
			AccountManager: n.accman,
			node:           n,
		}

		//复制已有服务到ctx
		for kind := range services {
			ctx.services[kind] = services[kind]
		}
		// 构造并保存服务
		service, err := constructor(ctx)
		if err != nil {
			return err
		}
		if service == nil {
			panic("can not reg nil service")
		}
		kind := reflect.TypeOf(service)
		if _, exists := services[kind]; exists {
			return &DuplicateServiceError{Kind: kind}
		}
		services[kind] = service
	}
	// 收集服务器所提供的协议
	for _, service := range services {
		running.Protocols = append(running.Protocols, service.Protocols()...)
	}
	// Author: @ysqi 将协议复制给P2P
	n.serverConfig.Protocols = running.Protocols

	// 开启P2P
	if err := running.Start(); err != nil {
		return convertFileLockError(err)
	}

	var (
		started []reflect.Type
		ok      bool
	)
	// 如果后续运行失败时，需关闭已开启的服务
	defer func() {
		if err := recover(); err != nil {
			ok = false
		}
		if !ok {
			for _, kind := range started {
				services[kind].Stop()
			}
			running.Stop()
		}
	}()

	// 开启各服务
	for kind, service := range services {
		if err := service.Start(running); err != nil {
			return err
		}
		// 记录已开启的服务，方便启动失败时的清理
		started = append(started, kind)
	}

	// Author: @kulics 开启RPC
	if err := n.startRPC(services); err != nil {
		return err
	}
	// Finish initializing the startup
	n.services = services
	n.server = running
	n.stop = make(chan struct{})
	ok = true

	log.Info("Node is running")
	return nil
}

func (n *Node) openDataDir() error {
	if n.config.DataDir == "" {
		return nil // 临时
	}

	instdir := n.config.instanceDir()
	if err := os.MkdirAll(instdir, 0700); err != nil {
		return err
	}
	// 锁定当前实例目录，防止其他实例并发访问
	release, _, err := flock.New(filepath.Join(instdir, "LOCK"))
	if err != nil {
		return convertFileLockError(err)
	}
	n.instanceDirLock = release
	return nil
}

// startRPC 启动RPC
// Author: @kulics
func (n *Node) startRPC(services map[reflect.Type]Service) error {
	// Gather all the possible APIs to surface
	// 获取api
	apis := n.apis()
	for _, service := range services {
		apis = append(apis, service.APIs()...)
	}
	// Start the various API endpoints, terminating all in case of errors
	// 按顺序启动所有的rpc服务，如果有一个出错，就全部退出
	if err := n.startInProc(apis); err != nil {
		return err
	}
	if err := n.startIPC(apis); err != nil {
		n.stopInProc()
		return err
	}
	if err := n.startHTTP(n.httpEndpoint, apis, n.config.HTTPModules, n.config.HTTPCors); err != nil {
		n.stopIPC()
		n.stopInProc()
		return err
	}
	if err := n.startWS(n.wsEndpoint, apis, n.config.WSModules, n.config.WSOrigins, n.config.WSExposeAll); err != nil {
		n.stopHTTP()
		n.stopIPC()
		n.stopInProc()
		return err
	}
	// All API endpoints started successfully
	// 全部启动通过
	// 载入API
	n.rpcAPIs = apis
	return nil
}

// startInProc 启动进程内服务
// Author: @kulics
func (n *Node) startInProc(apis []rpc.API) error {
	// Register all the APIs exposed by the services
	// 注册所有API
	handler := rpc.NewServer()
	for _, api := range apis {
		if err := handler.RegisterName(api.Namespace, api.Service); err != nil {
			return err
		}
		log.Debug(fmt.Sprintf("InProc registered %T under '%s'", api.Service, api.Namespace))
	}
	n.inprocHandler = handler
	return nil
}

// stopInProc 停止进程内服务
// Author: @kulics
func (n *Node) stopInProc() {
	if n.inprocHandler != nil {
		n.inprocHandler.Stop()
		n.inprocHandler = nil
	}
}

// startIPC 启动ipc服务
// Author: @kulics
func (n *Node) startIPC(apis []rpc.API) error {
	// Short circuit if the IPC endpoint isn't being exposed
	// 检查端点
	if n.ipcEndpoint == "" {
		return nil
	}
	// Register all the APIs exposed by the services
	// 注册所有API
	handler := rpc.NewServer()
	if err := n.regAPI("IPC", handler, apis, []string{"*"}); err != nil {
		return err
	}
	// All APIs registered, start the IPC listener
	// 启动ipc监听器
	var (
		listener net.Listener
		err      error
	)
	if listener, err = rpc.CreateIPCListener(n.ipcEndpoint); err != nil {
		return err
	} else if listener == nil {
		return nil
	}

	go func() {
		log.Info(fmt.Sprintf("IPC endpoint opened: %s", n.ipcEndpoint))
		var sum int64
		var closedCount int64
		for {
			conn, err := listener.Accept()
			if err != nil {
				// Terminate if the listener was closed
				n.lock.RLock()
				closed := n.ipcListener == nil
				n.lock.RUnlock()
				if closed {
					return
				}
				// Not closed, just some error; report and continue
				log.Error(fmt.Sprintf("IPC accept failed: %v", err))
				continue
			}
			sum++
			addr := conn.RemoteAddr()
			log.Debug(fmt.Sprint("accepted conn", addr), "sumConn", sum)
			go func(addr net.Addr) {
				bt := time.Now()
				handler.ServeCodec(rpc.NewJSONCodec(conn), rpc.OptionMethodInvocation|rpc.OptionSubscriptions)
				atomic.AddInt64(&closedCount, 1)
				log.Debug("when closed", "closedAddr", addr,
					"closed", closedCount, "sum", sum, "cost", common.PrettyDuration(time.Now().Sub(bt)))
			}(addr)
		}
	}()
	// All listeners booted successfully
	// 挂载到属性上
	n.ipcListener = listener
	n.ipcHandler = handler

	return nil
}

// stopIPC 停止ipc服务
// Author: @kulics
func (n *Node) stopIPC() {
	if n.ipcListener != nil {
		n.ipcListener.Close()
		n.ipcListener = nil

		log.Info(fmt.Sprintf("IPC endpoint closed: %s", n.ipcEndpoint))
	}
	if n.ipcHandler != nil {
		n.ipcHandler.Stop()
		n.ipcHandler = nil
	}
}

func (n *Node) RegAPI(apis []rpc.API) error {
	if n.inprocHandler != nil {
		if err := n.regAPI("InProc", n.inprocHandler, apis, []string{"*"}); err != nil {
			return err
		}
	}

	if n.ipcHandler != nil {
		if err := n.regAPI("IPC", n.ipcHandler, apis, []string{"*"}); err != nil {
			return err
		}
	}
	if n.httpHandler != nil {
		if err := n.regAPI("HTTP", n.httpHandler, apis, n.config.HTTPModules); err != nil {
			return err
		}
	}
	if n.wsHandler != nil {
		if err := n.regAPI("WebSocket", n.wsHandler, apis, n.config.WSModules); err != nil {
			return err
		}
	}
	return nil
}

func (n *Node) regAPI(category string, handler *rpc.Server, apis []rpc.API, modules []string) error {
	// Generate the whitelist based on the allowed modules
	// 生成白名单
	whitelist := make(map[string]bool)
	for _, module := range modules {
		whitelist[module] = true
	}
	// Register all the APIs exposed by the services
	for _, api := range apis {
		if whitelist[api.Namespace] || (len(whitelist) == 0 && api.Public) || whitelist["*"] {
			if err := handler.RegisterName(api.Namespace, api.Service); err != nil {
				return err
			}
			log.Debug(fmt.Sprintf("%s registered %T under '%s'", category, api.Service, api.Namespace))
		}
	}
	return nil
}

// startHTTP 启动http服务
// Author: @kulics
func (n *Node) startHTTP(endpoint string, apis []rpc.API, modules []string, cors []string) error {
	// Short circuit if the HTTP endpoint isn't being exposed
	// 检测端点
	if endpoint == "" {
		return nil
	}
	handler := rpc.NewServer()
	if err := n.regAPI("http", handler, apis, modules); err != nil {
		return err
	}
	// All APIs registered, start the HTTP listener
	// 启动监听器
	var (
		listener net.Listener
		err      error
	)
	if listener, err = net.Listen("tcp", endpoint); err != nil {
		return err
	}
	go rpc.NewHTTPServer(cors, handler).Serve(listener)
	log.Info(fmt.Sprintf("HTTP endpoint opened: http://%s", endpoint))

	// All listeners booted successfully
	// 挂载属性
	n.httpEndpoint = endpoint
	n.httpListener = listener
	n.httpHandler = handler

	return nil
}

// stopHTTP 停止http服务
// Author: @kulics
func (n *Node) stopHTTP() {
	if n.httpListener != nil {
		n.httpListener.Close()
		n.httpListener = nil

		log.Info(fmt.Sprintf("HTTP endpoint closed: http://%s", n.httpEndpoint))
	}
	if n.httpHandler != nil {
		n.httpHandler.Stop()
		n.httpHandler = nil
	}
}

// startWS 启动websocket服务
// Author: @kulics
func (n *Node) startWS(endpoint string, apis []rpc.API, modules []string, wsOrigins []string, exposeAll bool) error {
	// Short circuit if the WS endpoint isn't being exposed
	// 检测端点
	if endpoint == "" {
		return nil
	}
	handler := rpc.NewServer()
	if err := n.regAPI("WebSocket", handler, apis, modules); err != nil {
		return err
	}

	// All APIs registered, start the HTTP listener
	// 启动监听器
	var (
		listener net.Listener
		err      error
	)
	if listener, err = net.Listen("tcp", endpoint); err != nil {
		return err
	}
	go rpc.NewWSServer(wsOrigins, handler).Serve(listener)
	log.Info(fmt.Sprintf("WebSocket endpoint opened: ws://%s", listener.Addr()))

	// All listeners booted successfully
	// 挂载属性
	n.wsEndpoint = endpoint
	n.wsListener = listener
	n.wsHandler = handler

	return nil
}

// 停止websocket服务
// Author: @kulics
func (n *Node) stopWS() {
	if n.wsListener != nil {
		n.wsListener.Close()
		n.wsListener = nil

		log.Info(fmt.Sprintf("WebSocket endpoint closed: ws://%s", n.wsEndpoint))
	}
	if n.wsHandler != nil {
		n.wsHandler.Stop()
		n.wsHandler = nil
	}
}

// Stop 停止实例节点所开启的服务，并终止节点实例。
// 如果未启动节点实例时调用此方法则返回错误信息。
func (n *Node) Stop() error {
	n.lock.Lock()
	defer n.lock.Unlock()

	if n.server == nil {
		return ErrNodeStopped
	}

	log.Info("Node stopping")
	defer log.Info("Node stopped")

	actions := []func(){}
	failure := &StopError{
		Services: make(map[reflect.Type]error),
	}
	// 停止RPC相关服务，包括websocket，http，ipc。进程内的会跟随进程停止
	actions = append(actions, n.stopWS)
	actions = append(actions, n.stopHTTP)
	actions = append(actions, n.stopIPC)

	// 清空API
	actions = append(actions, func() { n.rpcAPIs = nil })
	// 逐个停止服务
	for kind, service := range n.services {
		if service == nil {
			continue
		}
		service := service
		actions = append(actions, func() {
			defer func() {
				if err := recover(); err != nil {
					log.Error("Failed to stop service,continue stop other service",
						"service", reflect.TypeOf(service).String(), "err", err)
				}
			}()
			if service == nil {
				return
			}
			if err := service.Stop(); err != nil {
				failure.Services[kind] = err
			}
		})
	}
	actions = append(actions, func() {
		// Author：@kulics 停止p2p服务
		n.server.Stop()
		n.services = nil
	})

	// 释放锁定
	if n.instanceDirLock != nil {
		actions = append(actions, func() {
			if err := n.instanceDirLock.Release(); err != nil {
				log.Error("Can't release datadir lock", "err", err)
			}
			n.instanceDirLock = nil
		})
	}
	actions = append(actions, func() {
		// 关闭stop 等待
		close(n.stop)
	})

	// 如果是临时存放，则销毁
	if n.ephemeralKeystore != "" {
		actions = append(actions, func() {
			if err := os.RemoveAll(n.ephemeralKeystore); err != nil {
				failure.Server = err
			}
		})
	}

	// 确保所有释放工作都能如期执行
	for _, do := range actions {
		f := func(do func()) {
			defer func() {
				if err := recover(); err != nil {
					log.Error("Recovered stop service error", "err", err)
				}
			}()
			do()
		}
		f(do)
	}

	n.server = nil
	if len(failure.Services) > 0 || failure.Server != nil {
		return failure
	}

	return nil
}

// Wait 等待终止节点完成，如果节点尚未运行，则无需等待
func (n *Node) Wait() {
	n.lock.RLock()
	if n.server == nil {
		n.lock.RUnlock()
		return
	}
	stop := n.stop
	n.lock.RUnlock()

	<-stop
}

// Restart 终止运行中的节点并再次启动，只有节点在运行时方可执行，否则报错。
func (n *Node) Restart() error {
	if err := n.Stop(); err != nil {
		return err
	}
	if err := n.Start(); err != nil {
		return err
	}
	return nil
}

// Attach 创建rpc客户端，挂载在进程内handle上
// Author: @kulics
func (n *Node) Attach() (*rpc.Client, error) {
	n.lock.RLock()
	defer n.lock.RUnlock()

	if n.server == nil {
		return nil, ErrNodeStopped
	}
	return rpc.DialInProc(n.inprocHandler), nil
}

// RPCHandler 返回进程内rpc请求handle
// Author: @kulics
func (n *Node) RPCHandler() (*rpc.Server, error) {
	n.lock.RLock()
	defer n.lock.RUnlock()

	if n.inprocHandler == nil {
		return nil, ErrNodeStopped
	}
	return n.inprocHandler, nil
}

// Server 返回节点的P2P服务对象
func (n *Node) Server() *p2p.Server {
	n.lock.RLock()
	defer n.lock.RUnlock()

	return n.server
}

// Service 取回当前运行中节点下所注册的同类型服务。
func (n *Node) Service(service interface{}) error {
	n.lock.RLock()
	defer n.lock.RUnlock()

	// Short circuit if the node's not running
	if n.server == nil {
		return ErrNodeStopped
	}
	// Otherwise try to find the service to return
	element := reflect.ValueOf(service).Elem()
	if running, ok := n.services[element.Type()]; ok {
		element.Set(reflect.ValueOf(running))
		return nil
	}
	return ErrServiceUnknown
}

// DataDir 返回默认的数据文件夹。
// 此目录不应该被使用，需要通过InstanceDir获取节点实例的数据目录
func (n *Node) DataDir() string {
	return n.config.DataDir
}

// InstanceDir 返回当前节点实例的数据目录
func (n *Node) InstanceDir() string {
	return n.config.instanceDir()
}

// AccountManager 返回当前的Account管理器
func (n *Node) AccountManager() *accounts.Manager {
	return n.accman
}

// IPCEndpoint 返回当前的IPC端点
// Author: @kulics
func (n *Node) IPCEndpoint() string {
	return n.ipcEndpoint
}

// HTTPEndpoint 返回当前的http端点
// Author: @kulics
func (n *Node) HTTPEndpoint() string {
	return n.httpEndpoint
}

// WSEndpoint 返回当前的websocket端点
// Author: @kulics
func (n *Node) WSEndpoint() string {
	return n.wsEndpoint
}

// EventMux  返回当前多重使用的事件路由
// Author: @kulics
func (n *Node) EventMux() *event.TypeMux {
	return n.eventmux
}

// OpenDatabase opens an existing database with the given name (or creates one if no
// previous can be found) from within the node's instance directory. If the node is
// ephemeral, a memory database is returned.
func (n *Node) OpenDatabase(name string, cache, handles int) (ntsdb.Database, error) {
	return openDatabase(n.config, name, cache, handles)
}

// ResolvePath 返回当前节点实例工作目录下的文件路径
func (n *Node) ResolvePath(x string) string {
	return n.config.ResolvePath(x)
}

// apis 返回节点本身所提供的API清单
func (n *Node) apis() []rpc.API {
	return []rpc.API{
		{
			Namespace: "admin",
			Version:   "1.0",
			Service:   NewPrivateAdminAPI(n),
		}, {
			Namespace: "admin",
			Version:   "1.0",
			Service:   NewPublicAdminAPI(n),
			Public:    true,
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   debug.Handler,
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPublicDebugAPI(n),
			Public:    true,
		}, {
			Namespace: "web3",
			Version:   "1.0",
			Service:   NewPublicWeb3API(n),
			Public:    true,
		},
	}
}
