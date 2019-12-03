// Author: @ysqi

package node

import (
	"reflect"

	"gitee.com/nerthus/nerthus/accounts"
	"gitee.com/nerthus/nerthus/core/config"
	"gitee.com/nerthus/nerthus/event"
	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/ntsdb"
	"gitee.com/nerthus/nerthus/p2p"
	"gitee.com/nerthus/nerthus/rpc"
)

// ServiceContext 是给构建服务所提供的独立的节点信息上下文
type ServiceContext struct {
	node           *Node
	config         *Config
	ServerConfig   p2p.Config
	Server         *p2p.Server              // 本身p2p服务器
	services       map[reflect.Type]Service // 已构建的服务清单
	EventMux       *event.TypeMux           // 可复用事件
	AccountManager *accounts.Manager        // Account管理器
}

// OpenDatabase 根据给定文件名打开节点 data 文件目录下已存在的数据库（不存在则创建）。
func openDatabase(cfg *Config, name string, cache int, handles int) (ntsdb.Database, error) {
	if cfg.DataDir == "" {
		return ntsdb.NewMemDatabase()
	}

	db, err := func() (ntsdb.Database, error) {
		switch cfg.DBDriver {
		case "leveldb":
			return ntsdb.NewLDBDatabase(cfg.ResolvePath(name), cache, handles)
		case "badger":
			return ntsdb.NewBadger(cfg.ResolvePath(name))
		default:
			log.Warn("missing database driver config,default use leveldb")
			return ntsdb.NewLDBDatabase(cfg.ResolvePath(name), cache, handles)
		}
	}()
	if err != nil {
		return nil, err
	}

	if config.MustBool("runtime.memory_database_enable", false) {
		log.Warn("In-memory database is enabled")
		return ntsdb.NewInMemoryDatabase(db)
	}
	return db, nil
}

// OpenDatabase 根据给定文件名打开节点 data 文件目录下已存在的数据库（不存在则创建）。
func (ctx *ServiceContext) OpenDatabase(name string, cache int, handles int) (ntsdb.Database, error) {
	return openDatabase(ctx.config, name, cache, handles)
}

func (ctx *ServiceContext) OpenRD(name string) (ntsdb.RD, error) {
	if ctx.config.DataDir == "" {
		return ntsdb.NewTempRD()
	}
	return ntsdb.NewRD(ctx.config.ResolvePath(name))
}

// ResolvePath 取回节点实例下文件存储路径，如果未设置节点数据存放目录，则返回空地址，便相当于在当前路径
func (ctx *ServiceContext) ResolvePath(path string) string {
	return ctx.config.ResolvePath(path)
}

// Service 从运行中的节点上提取运行中的同类型服务
func (ctx *ServiceContext) Service(service interface{}) error {
	element := reflect.ValueOf(service).Elem()
	if running, ok := ctx.services[element.Type()]; ok {
		element.Set(reflect.ValueOf(running))
		return nil
	}
	return ErrServiceUnknown
}
func (ctx *ServiceContext) RegAPI(apis ...rpc.API) error {
	return ctx.node.RegAPI(apis)
}

// ServiceConstructor 是服务构造器函数，用于服务注册。
type ServiceConstructor func(ctx *ServiceContext) (Service, error)

// Service 是一个可注册到节点上的单一服务。
//
// 注意:
//
// • 服务生命周期有节点管理，允许服务在构建时初始化自身，但不应该在Start方法外部启动goroutines。
// 尽量将服务内容在Start中处理。
//
// • Restart 功能不是必须的，因为节点在启动时便会创建新的服务并启动它。
type Service interface {
	// Protocols 获取该服务所期望的P2P协议。
	Protocols() []p2p.Protocol

	// APIs 获取该服务所提供的PRC API清单
	APIs() []rpc.API

	// Start 在所有服务构建完成后，节点准备就绪后将被调用。需要内部基于goroutines去运行服务
	Start(server *p2p.Server) error

	// Stop 终止服务内所有工作，包括停止goroutines、解锁等。
	Stop() error
}
