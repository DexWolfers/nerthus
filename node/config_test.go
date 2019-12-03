// Author: @ysqi

package node

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gitee.com/nerthus/nerthus/crypto"
	"gitee.com/nerthus/nerthus/p2p"
)

func TestDatadirCreation(t *testing.T) {

	// 创建临时文件夹
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("failed to create manual data dir: %v", err)
	}
	defer os.RemoveAll(dir)

	if _, err := New(&Config{DataDir: dir}); err != nil {
		t.Fatalf("failed to create stack with existing datadir: %v", err)
	}

	// 生成一个长文件夹，测试是否能正常创建
	dir = filepath.Join(dir, "a", "b", "c", "d", "e", "f")
	if _, err := New(&Config{DataDir: dir}); err != nil {
		t.Fatalf("failed to create stack with creatable datadir: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("freshly created datadir not accessible: %v", err)
	}

	// 验证不存在的文件情况
	file, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatalf("failed to create temporary file: %v", err)
	}
	defer os.Remove(file.Name())

	dir = filepath.Join(file.Name(), "invalid/path")
	if _, err := New(&Config{DataDir: dir}); err == nil {
		t.Fatalf("protocol stack created with an invalid datadir")
	}

	// 验证数据合法性
	dir, err = ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("failed to create temporary file: %v", err)
	}
	defer os.RemoveAll(dir)
	testcases := []struct {
		cfg Config
		ok  bool
	}{
		{Config{DataDir: dir}, true},
		{Config{DataDir: dir, Name: ""}, true},
		{Config{DataDir: dir, Name: "good"}, true},
		{Config{DataDir: dir, Name: "bad*()bad"}, false},
		{Config{DataDir: dir, Name: "bad."}, false},
		{Config{DataDir: dir, Name: "."}, false},
		{Config{DataDir: dir, Name: ".good"}, true},
		{Config{DataDir: dir, Name: "go.od"}, true},
		{Config{DataDir: dir, Name: datadirDefaultKeyStore}, false},
		{Config{DataDir: dir, Name: datadirDefaultKeyStore + datadirDefaultKeyStore}, true},
		{Config{DataDir: dir, Name: ".ipc"}, false},
		{Config{DataDir: dir, Name: "ipc"}, true},
		{Config{DataDir: dir, Name: ".Ipc"}, false},
		{Config{DataDir: dir, Name: "bad.ipc"}, false},
		{Config{DataDir: dir, Name: ".ipc.good"}, true},
	}
	for _, c := range testcases {
		if _, err := New(&c.cfg); c.ok && err != nil {
			t.Fatalf("failed to create stack: %v", err)
		} else if !c.ok && err == nil {
			t.Fatalf("want create stack with DataDir %q error but got nil", c.cfg.Name)
		}
	}
}

// Tests 测试 IPC 文件路径，不同平台将在不同存储位置
func TestIPCPathResolution(t *testing.T) {
	var tests = []struct {
		DataDir  string
		IPCPath  string
		Windows  bool
		Endpoint string
	}{
		{"", "", false, ""},
		{"data", "", false, ""},
		{"", "geth.ipc", false, filepath.Join(os.TempDir(), "geth.ipc")},
		{"data", "geth.ipc", false, "data/geth.ipc"},
		{"data", "./geth.ipc", false, "./geth.ipc"},
		{"data", "/geth.ipc", false, "/geth.ipc"},
		{"", "", true, ``},
		{"data", "", true, ``},
		{"", "geth.ipc", true, `\\.\pipe\geth.ipc`},
		{"data", "geth.ipc", true, `\\.\pipe\geth.ipc`},
		{"data", `\\.\pipe\geth.ipc`, true, `\\.\pipe\geth.ipc`},
	}
	for i, test := range tests {
		// 只运行对应平台项
		if (runtime.GOOS == "windows") == test.Windows {
			if endpoint := (&Config{DataDir: test.DataDir, IPCPath: test.IPCPath}).IPCEndpoint(); endpoint != test.Endpoint {
				t.Errorf("test %d: IPC endpoint mismatch: have %s, want %s", i, endpoint, test.Endpoint)
			}
		}
	}
}

// Tests 测试节点私钥文件和临时情况
func TestNodeKeyPersistency(t *testing.T) {
	dir, err := ioutil.TempDir("", "node-test")
	if err != nil {
		t.Fatalf("failed to create temporary data directory: %v", err)
	}
	defer os.RemoveAll(dir)

	keyfile := filepath.Join(dir, "unit-test", datadirPrivateKey)

	// 创建一个临时秘钥
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate one-shot node key: %v", err)
	}
	config := &Config{Name: "unit-test", DataDir: dir, P2P: p2p.Config{PrivateKey: key}}
	config.NodeKey()
	if _, err := os.Stat(filepath.Join(keyfile)); err == nil {
		t.Fatalf("one-shot node key persisted to data directory")
	}

	// 无私钥时应该使用临时文件
	config = &Config{Name: "unit-test", DataDir: dir}
	config.NodeKey()
	if _, err := os.Stat(keyfile); err != nil {
		t.Fatalf("node key not persisted to data directory: %v", err)
	}
	blob1, err := ioutil.ReadFile(keyfile)
	if err != nil {
		t.Fatalf("failed to read freshly persisted node key: %v", err)
	}
	if _, err = crypto.LoadECDSA(keyfile); err != nil {
		t.Fatalf("failed to load freshly persisted node key: %v", err)
	}

	// 再次使用相同节点，应该加载已有的私钥
	config = &Config{Name: "unit-test", DataDir: dir}
	config.NodeKey()
	blob2, err := ioutil.ReadFile(keyfile)
	if err != nil {
		t.Fatalf("failed to read previously persisted node key: %v", err)
	}
	if !bytes.Equal(blob1, blob2) {
		t.Fatalf("persisted node key mismatch: have %x, want %x", blob2, blob1)
	}

	// 空路径时，不得在本地创建私钥
	config = &Config{Name: "unit-test", DataDir: ""}
	config.NodeKey()
	if _, err := os.Stat(filepath.Join(".", "unit-test", datadirPrivateKey)); err == nil {
		t.Fatalf("ephemeral node key persisted to disk")
	}
}
