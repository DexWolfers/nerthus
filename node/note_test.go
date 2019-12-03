// Author: @ysqi

package node

import (
	"errors"
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/p2p"
)

var (
	testNodeKey, _ = crypto.GenerateKey()
)

func testNodeConfig() *Config {
	return &Config{
		Name: "test-node",
		P2P:  p2p.Config{PrivateKey: testNodeKey},
	}
}

// 测试节点的开启、停止和重启
func TestNodeLifeCycle(t *testing.T) {
	stack, err := New(testNodeConfig())
	if err != nil {
		t.Fatalf("failed to create protocol stack: %v", err)
	}
	// 可重复执行停止
	for i := 0; i < 3; i++ {
		if err := stack.Stop(); err != ErrNodeStopped {
			t.Fatalf("iter %d: stop failure mismatch: have %v, want %v", i, err, ErrNodeStopped)
		}
	}
	// 只允许开启一次
	if err := stack.Start(); err != nil {
		t.Fatalf("failed to start node: %v", err)
	}
	if err := stack.Start(); err != ErrNodeRunning {
		t.Fatalf("start failure mismatch: have %v, want %v ", err, ErrNodeRunning)
	}
	// 不允许重复restart
	for i := 0; i < 3; i++ {
		if err := stack.Restart(); err != nil {
			t.Fatalf("iter %d: failed to restart node: %v", i, err)
		}
	}
	// 开启后可再次多次关闭
	if err := stack.Stop(); err != nil {
		t.Fatalf("failed to stop node: %v", err)
	}
	if err := stack.Stop(); err != ErrNodeStopped {
		t.Fatalf("stop failure mismatch: have %v, want %v ", err, ErrNodeStopped)
	}
}

// 测试 节点使用数据文件夹
func TestNodeUsedDataDir(t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("failed to create temporary data directory: %v", err)
	}
	defer os.RemoveAll(dir)

	// 创建基于本目录的节点
	original, err := New(&Config{DataDir: dir})
	if err != nil {
		t.Fatalf("failed to create original protocol stack: %v", err)
	}
	if err := original.Start(); err != nil {
		t.Fatalf("failed to start original protocol stack: %v", err)
	}
	defer original.Stop()

	// 不得重复使用同一目录
	duplicate, err := New(&Config{DataDir: dir})
	if err != nil {
		t.Fatalf("failed to create duplicate protocol stack: %v", err)
	}
	if err := duplicate.Start(); err != ErrDatadirUsed {
		t.Fatalf("duplicate datadir failure mismatch: have %v, want %v", err, ErrDatadirUsed)
	}
}

// 测试是否可有效注册服务
func TestServiceRegistry(t *testing.T) {
	stack, err := New(testNodeConfig())
	if err != nil {
		t.Fatalf("failed to create protocol stack: %v", err)
	}
	// 批量注册服务
	services := []ServiceConstructor{NewNoopServiceA, NewNoopServiceB, NewNoopServiceC}
	for i, constructor := range services {
		if err := stack.Register(constructor); err != nil {
			t.Fatalf("service #%d: registration failed: %v", i, err)
		}
	}
	if err := stack.Start(); err != nil {
		t.Fatalf("failed to start original service stack: %v", err)
	}
	if err := stack.Stop(); err != nil {
		t.Fatalf("failed to stop original service stack: %v", err)
	}
	// 可重复注册服务，但启动失败
	if err := stack.Register(NewNoopServiceB); err != nil {
		t.Fatalf("duplicate registration failed: %v", err)
	}
	if err := stack.Start(); err == nil {
		t.Fatalf("duplicate service started")
	} else {
		if _, ok := err.(*DuplicateServiceError); !ok {
			t.Fatalf("duplicate error mismatch: have %v, want %v", err, DuplicateServiceError{})
		}
	}
}

// 测试 已注册服务的开启与关闭
func TestServiceLifeCycle(t *testing.T) {
	stack, err := New(testNodeConfig())
	if err != nil {
		t.Fatalf("failed to create protocol stack: %v", err)
	}
	// 批量注册服务
	services := map[string]InstrumentingWrapper{
		"A": InstrumentedServiceMakerA,
		"B": InstrumentedServiceMakerB,
		"C": InstrumentedServiceMakerC,
	}
	started := make(map[string]bool)
	stopped := make(map[string]bool)

	for id, maker := range services {
		id := id // 复制
		constructor := func(*ServiceContext) (Service, error) {
			return &InstrumentedService{
				startHook: func(*p2p.Server) { started[id] = true },
				stopHook:  func() { stopped[id] = true },
			}, nil
		}
		if err := stack.Register(maker(constructor)); err != nil {
			t.Fatalf("service %s: registration failed: %v", id, err)
		}
	}
	// 启动节点，服务需启动正常
	if err := stack.Start(); err != nil {
		t.Fatalf("failed to start protocol stack: %v", err)
	}
	for id := range services {
		if !started[id] {
			t.Fatalf("service %s: freshly started service not running", id)
		}
		if stopped[id] {
			t.Fatalf("service %s: freshly started service already stopped", id)
		}
	}
	// 关闭节点，服务需关闭
	if err := stack.Stop(); err != nil {
		t.Fatalf("failed to stop protocol stack: %v", err)
	}
	for id := range services {
		if !stopped[id] {
			t.Fatalf("service %s: freshly terminated service still running", id)
		}
	}
}

// 测试 服务启动后重新注入
func TestServiceRestarts(t *testing.T) {
	stack, err := New(testNodeConfig())
	if err != nil {
		t.Fatalf("failed to create protocol stack: %v", err)
	}
	// 定义不支持重启的服务
	var (
		running bool
		started int
	)
	constructor := func(*ServiceContext) (Service, error) {
		running = false

		return &InstrumentedService{
			startHook: func(*p2p.Server) {
				if running {
					panic("already running")
				}
				running = true
				started++
			},
		}, nil
	}
	// 注册
	if err := stack.Register(constructor); err != nil {
		t.Fatalf("failed to register the service: %v", err)
	}
	if err := stack.Start(); err != nil {
		t.Fatalf("failed to start protocol stack: %v", err)
	}
	defer stack.Stop()

	if !running || started != 1 {
		t.Fatalf("running/started mismatch: have %v/%d, want true/1", running, started)
	}
	// 重启后，服务也许跟随重启
	for i := 0; i < 3; i++ {
		if err := stack.Restart(); err != nil {
			t.Fatalf("iter %d: failed to restart stack: %v", i, err)
		}
	}
	if !running || started != 4 {
		t.Fatalf("running/started mismatch: have %v/%d, want true/4", running, started)
	}
}

// 测试服务启动失败的情况
func TestServiceConstructionAbortion(t *testing.T) {
	stack, err := New(testNodeConfig())
	if err != nil {
		t.Fatalf("failed to create protocol stack: %v", err)
	}
	// 定义四个服务
	services := map[string]InstrumentingWrapper{
		"A": InstrumentedServiceMakerA,
		"B": InstrumentedServiceMakerB,
		"C": InstrumentedServiceMakerC,
	}
	started := make(map[string]bool)
	for id, maker := range services {
		id := id // 复制
		constructor := func(*ServiceContext) (Service, error) {
			return &InstrumentedService{
				startHook: func(*p2p.Server) { started[id] = true },
			}, nil
		}
		if err := stack.Register(maker(constructor)); err != nil {
			t.Fatalf("service %s: registration failed: %v", id, err)
		}
	}
	// 注册一个将失败的服务
	failure := errors.New("fail")
	failer := func(*ServiceContext) (Service, error) {
		return nil, failure
	}
	if err := stack.Register(failer); err != nil {
		t.Fatalf("failer registration failed: %v", err)
	}
	// 启动将失败，其他服务也将停止
	for i := 0; i < 100; i++ {
		if err := stack.Start(); err != failure {
			t.Fatalf("iter %d: stack startup failure mismatch: have %v, want %v", i, err, failure)
		}
		for id := range services {
			if started[id] {
				t.Fatalf("service %s: started should not have", id)
			}
			delete(started, id)
		}
	}
}

// 测试服务启动失败的情况
func TestServiceStartupAbortion(t *testing.T) {
	stack, err := New(testNodeConfig())
	if err != nil {
		t.Fatalf("failed to create protocol stack: %v", err)
	}

	services := map[string]InstrumentingWrapper{
		"A": InstrumentedServiceMakerA,
		"B": InstrumentedServiceMakerB,
		"C": InstrumentedServiceMakerC,
	}
	started := make(map[string]bool)
	stopped := make(map[string]bool)

	for id, maker := range services {
		id := id
		constructor := func(*ServiceContext) (Service, error) {
			return &InstrumentedService{
				startHook: func(*p2p.Server) { started[id] = true },
				stopHook:  func() { stopped[id] = true },
			}, nil
		}
		if err := stack.Register(maker(constructor)); err != nil {
			t.Fatalf("service %s: registration failed: %v", id, err)
		}
	}
	// Register a service that fails to start
	failure := errors.New("fail")
	failer := func(*ServiceContext) (Service, error) {
		return &InstrumentedService{
			start: failure,
		}, nil
	}
	if err := stack.Register(failer); err != nil {
		t.Fatalf("failer registration failed: %v", err)
	}

	for i := 0; i < 100; i++ {
		if err := stack.Start(); err != failure {
			t.Fatalf("iter %d: stack startup failure mismatch: have %v, want %v", i, err, failure)
		}
		for id := range services {
			if started[id] && !stopped[id] {
				t.Fatalf("service %s: started but not stopped", id)
			}
			delete(started, id)
			delete(stopped, id)
		}
	}
}

// 测试即使服务不能顺利关闭，也不影响节点的退出
func TestServiceTerminationGuarantee(t *testing.T) {
	stack, err := New(testNodeConfig())
	if err != nil {
		t.Fatalf("failed to create protocol stack: %v", err)
	}
	services := map[string]InstrumentingWrapper{
		"A": InstrumentedServiceMakerA,
		"B": InstrumentedServiceMakerB,
		"C": InstrumentedServiceMakerC,
	}
	started := make(map[string]bool)
	stopped := make(map[string]bool)

	for id, maker := range services {
		id := id
		constructor := func(*ServiceContext) (Service, error) {
			return &InstrumentedService{
				startHook: func(*p2p.Server) { started[id] = true },
				stopHook:  func() { stopped[id] = true },
			}, nil
		}
		if err := stack.Register(maker(constructor)); err != nil {
			t.Fatalf("service %s: registration failed: %v", id, err)
		}
	}
	// Register a service that fails to shot down cleanly
	failure := errors.New("fail")
	failer := func(*ServiceContext) (Service, error) {
		return &InstrumentedService{
			stop: failure,
		}, nil
	}
	if err := stack.Register(failer); err != nil {
		t.Fatalf("failer registration failed: %v", err)
	}
	// 开启，该关的关
	for i := 0; i < 100; i++ {
		// Start the stack and make sure all is online
		if err := stack.Start(); err != nil {
			t.Fatalf("iter %d: failed to start protocol stack: %v", i, err)
		}
		for id := range services {
			if !started[id] {
				t.Fatalf("iter %d, service %s: service not running", i, id)
			}
			if stopped[id] {
				t.Fatalf("iter %d, service %s: service already stopped", i, id)
			}
		}
		// 停止后，服务关闭失败的信息也正常获得
		err := stack.Stop()
		if err, ok := err.(*StopError); !ok {
			t.Fatalf("iter %d: termination failure mismatch: have %v, want StopError", i, err)
		} else {
			failer := reflect.TypeOf(&InstrumentedService{})
			gotFailer, ok := err.Services[failer]
			if ok && gotFailer != failure {
				t.Fatalf("iter %d: failer termination failure mismatch: have %v, want %v", i, err.Services[failer], failure)
			}
			if !ok && gotFailer != nil {
				t.Fatalf("iter %d: failer termination failure mismatch: have %v, want %v", i, err.Services[failer], failure)
			}
			if len(err.Services) != 1 {
				t.Fatalf("iter %d: failure count mismatch: have %d, want %d", i, len(err.Services), 1)
			}
		}
		for id := range services {
			if !stopped[id] {
				t.Fatalf("iter %d, service %s: service not terminated", i, id)
			}
			delete(started, id)
			delete(stopped, id)
		}
	}
}

//  测试服务检索
func TestServiceRetrieval(t *testing.T) {
	stack, err := New(testNodeConfig())
	if err != nil {
		t.Fatalf("failed to create protocol stack: %v", err)
	}
	if err := stack.Register(NewNoopService); err != nil {
		t.Fatalf("noop service registration failed: %v", err)
	}
	if err := stack.Register(NewInstrumentedService); err != nil {
		t.Fatalf("instrumented service registration failed: %v", err)
	}
	// 节点停止后，无法获取服务的
	var noopServ *NoopService
	if err := stack.Service(&noopServ); err != ErrNodeStopped {
		t.Fatalf("noop service retrieval mismatch: have %v, want %v", err, ErrNodeStopped)
	}
	var instServ *InstrumentedService
	if err := stack.Service(&instServ); err != ErrNodeStopped {
		t.Fatalf("instrumented service retrieval mismatch: have %v, want %v", err, ErrNodeStopped)
	}
	// 开启节点
	if err := stack.Start(); err != nil {
		t.Fatalf("failed to start stack: %v", err)
	}
	defer stack.Stop()

	if err := stack.Service(&noopServ); err != nil {
		t.Fatalf("noop service retrieval mismatch: have %v, want %v", err, nil)
	}
	if err := stack.Service(&instServ); err != nil {
		t.Fatalf("instrumented service retrieval mismatch: have %v, want %v", err, nil)
	}
}

// 测试获取服务协议
func TestProtocolGather(t *testing.T) {
	stack, err := New(testNodeConfig())
	if err != nil {
		t.Fatalf("failed to create protocol stack: %v", err)
	}
	services := map[string]struct {
		Count int
		Maker InstrumentingWrapper
	}{
		"Zero Protocols":  {0, InstrumentedServiceMakerA},
		"Single Protocol": {1, InstrumentedServiceMakerB},
		"Many Protocols":  {25, InstrumentedServiceMakerC},
	}
	for id, config := range services {
		protocols := make([]p2p.Protocol, config.Count)
		for i := 0; i < len(protocols); i++ {
			protocols[i].Name = id
			protocols[i].Version = uint(i)
		}
		constructor := func(*ServiceContext) (Service, error) {
			return &InstrumentedService{
				protocols: protocols,
			}, nil
		}
		if err := stack.Register(config.Maker(constructor)); err != nil {
			t.Fatalf("service %s: registration failed: %v", id, err)
		}
	}

	if err := stack.Start(); err != nil {
		t.Fatalf("failed to start protocol stack: %v", err)
	}
	defer stack.Stop()

	protocols := stack.Server().Protocols
	want := 26 // Author: @ysqi +1 是Node中含有密语服务
	if len(protocols) != want {
		t.Fatalf("mismatching number of protocols launched: have %d, want %d", len(protocols), want)
	}
	for id, config := range services {
		for ver := 0; ver < config.Count; ver++ {
			launched := false
			for i := 0; i < len(protocols); i++ {
				if protocols[i].Name == id && protocols[i].Version == uint(ver) {
					launched = true
					break
				}
			}
			if !launched {
				t.Errorf("configured protocol not launched: %s v%d", id, ver)
			}
		}
	}
}
