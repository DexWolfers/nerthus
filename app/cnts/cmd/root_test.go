// Author: @ysqi

package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitee.com/nerthus/nerthus/app/internal/cmdtest"
	"github.com/docker/docker/pkg/reexec"
)

func tmpdir(t *testing.T) string {
	dir, err := ioutil.TempDir("", "cnts-test")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
func tmpfile(t *testing.T) string {
	f, err := ioutil.TempFile("", "cnts-test-file")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

type testcnts struct {
	*cmdtest.TestCmd

	// template variables for expect
	D map[string]string
}

func init() {
	// Run the app if we've been exec'd as "cnts-test" in runMain.
	reexec.Register("cnts-test", func() {
		RootCmd.SetArgs(os.Args[1:])
		if err := RootCmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	})
}

func TestMain(m *testing.M) {
	// check if we have been reexec'd
	if reexec.Init() {

		return
	}
	os.Exit(m.Run())
}

// spawns main with the given command line args. If the args don't set --datadir, the
// child g gets a temporary data directory.
func runMain(t *testing.T, args ...string) *testcnts {
	tt := &testcnts{}
	tt.TestCmd = cmdtest.NewTestCmd(t, tt)
	tt.D = make(map[string]string, len(args)/2+1)
	for i, arg := range args {
		if i < len(args)-1 && strings.HasPrefix(arg, "--") {
			tt.D[arg[2:]] = args[i+1]
		}

	}
	if tt.D["datadir"] == "" {
		dir := tmpdir(t)
		tt.Cleanup = func() { os.RemoveAll(dir) }
		args = append([]string{"--datadir", dir}, args...)
		tt.D["datadir"] = dir
		// Remove the temporary datadir if something fails below.
		defer func() {
			if t.Failed() {
				tt.Cleanup()
			}
		}()
	}

	configPath := filepath.Join(os.Getenv("GOPATH"), "src/gitee.com/nerthus/nerthus", "params/sampleconfig/cnts.dev.yaml")

	args = append([]string{"--config", configPath}, args...)

	// Boot "cnts". This actually runs the test binary but the TestMain
	// function will prevent any tests from running.
	tt.Run("cnts-test", args...)
	return tt
}

func TestConfig(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		cnts := runMain(t, "--port", "0")
		cnts.ExpectExit()
	})
	t.Run("error", func(t *testing.T) {
		cnts := runMain(t, "--port", "0", "--config", "error.toml")
		cnts.Expect("Fatal: open error.toml")

	})
	t.Run("exist", func(t *testing.T) {
		file := tmpfile(t)
		cnts := runMain(t, "--port", "0", "--config", file)
		cnts.Cleanup = func() { os.Remove(file) }
		cnts.Interrupt()
		cnts.ExpectExit()
	})
}
