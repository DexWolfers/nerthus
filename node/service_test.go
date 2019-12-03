// Author: @ysqi

package node

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

// Tests 测试配置文件正确的上下文
func TestContextDatabases(t *testing.T) {
	// 创建临时文件目录
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("failed to create temporary data directory: %v", err)
	}
	defer os.RemoveAll(dir)

	if _, err := os.Stat(filepath.Join(dir, "database")); err == nil {
		t.Fatalf("non-created database already exists")
	}

	if dbIns, err := openDatabase(&Config{Name: "unit-test", DataDir: dir}, "", 0, 0); err != nil {
		t.Fatalf("faild to open or create sqllite db:%v", err)
	} else {
		dbIns.Close()
	}
	if _, err := os.Stat(filepath.Join(dir, "unit-test", "")); err != nil {
		t.Fatalf("%s database doesn't exists: %v", detadirSqlliteDB, err)
	}
	// Request th opening/creation of an ephemeral database and ensure it's not persisted
	if dbIns, err := openDatabase(&Config{DataDir: ""}, "", 0, 0); err != nil {
		t.Fatalf("faild to open or create sqllite db:%v", err)
	} else {
		dbIns.Close()
	}

	if _, err := os.Stat(filepath.Join(dir, detadirSqlliteDB)); err == nil {
		t.Fatalf("ephemeral database exists")
	}
}
