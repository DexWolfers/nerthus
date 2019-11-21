// Author: @ysqi

package testutil

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"

	"gitee.com/nerthus/nerthus/core/config"
	"gitee.com/nerthus/nerthus/log"

	"github.com/spf13/viper"
)

// SetupTestLogging setup the logging during test execution
func SetupTestLogging() {
	level, err := log.LvlFromString(strings.ToLower(viper.GetString("logging.verbosity")))
	if err != nil {
		level = log.LvlDebug
	}
	EnableLog(level, viper.GetString("logging.vmodule"))
}
func EnableLog(lvl log.Lvl, vmodule string) {
	handler := log.StreamHandler(os.Stdout, log.TerminalFormat(false))
	glog := log.NewGlogHandler(handler)
	log.PrintOrigins(true)
	glog.Verbosity(lvl)
	glog.Vmodule(vmodule)
	log.Root().SetHandler(glog)
}
func ResetLvl(lvl log.Lvl) {
	h := log.Root().GetHandler()
	if h, ok := h.(*log.GlogHandler); ok {
		h.Verbosity(lvl)
	}
}

var once sync.Once

func SetupTestConfig() {
	once.Do(setupTestConfig)
}

func ResetTestConfig() {
	setupTestConfig()
}

func setupTestConfig() {
	flag.Parse()

	viper.Reset()

	// Now set the configuration file
	err := config.InitViper(nil, "cnts.dev")
	if err != nil {
		panic(fmt.Errorf("Fatal error adding DevConfigPath: %s \n", err))
	}

	err = viper.ReadInConfig() // Find and read the config file
	if err != nil {            // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}

	SetupTestLogging()
}
