/*
Copyright Greg Haskins <gregory.haskins@gmail.com> 2017, All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cast"

	"gitee.com/nerthus/nerthus/common/homedir"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

func dirExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func addConfigPath(v *viper.Viper, p string) {
	if v != nil {
		v.AddConfigPath(p)
	} else {
		viper.AddConfigPath(p)
	}
}

var ErrMissingGoPath = errors.New("GOPATH not set")

//----------------------------------------------------------------------------------
// GetDevConfigDir()
//----------------------------------------------------------------------------------
// Returns the path to the default configuration that is maintained with the source
// tree.  Only valid to call from a test/development context.
//----------------------------------------------------------------------------------
func GetDevConfigDir() (string, error) {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		return "", ErrMissingGoPath
	}

	for _, p := range filepath.SplitList(gopath) {
		devPath := filepath.Join(p, "src/gitee.com/nerthus/nerthus/params/sampleconfig")
		if !dirExists(devPath) {
			continue
		}

		return devPath, nil
	}

	return "", fmt.Errorf("DevConfigDir not found in %s", gopath)
}

//----------------------------------------------------------------------------------
// GetDevMspDir()
//----------------------------------------------------------------------------------
// Builds upon GetDevConfigDir to return the path to our sampleconfig/msp that is
// maintained with the source tree.  Only valid to call from a test/development
// context.  Runtime environment should use configuration elements such as
//
//   GetPath("peer.mspConfigDir")
//----------------------------------------------------------------------------------
func GetDevMspDir() (string, error) {
	devDir, err := GetDevConfigDir()
	if err != nil {
		return "", fmt.Errorf("Error obtaining DevConfigDir: %s", devDir)
	}

	return filepath.Join(devDir, "msp"), nil
}

//----------------------------------------------------------------------------------
// TranslatePath()
//----------------------------------------------------------------------------------
// Translates a relative path into a fully qualified path relative to the config
// file that specified it.  Absolute paths are passed unscathed.
//----------------------------------------------------------------------------------
func TranslatePath(base, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}

//----------------------------------------------------------------------------------
// TranslatePathInPlace()
//----------------------------------------------------------------------------------
// Translates a relative path into a fully qualified path in-place (updating the
// pointer) relative to the config file that specified it.  Absolute paths are
// passed unscathed.
//----------------------------------------------------------------------------------
func TranslatePathInPlace(base string, p *string) {
	*p = TranslatePath(base, *p)
}

//----------------------------------------------------------------------------------
// GetPath()
//----------------------------------------------------------------------------------
// GetPath allows configuration strings that specify a (config-file) relative path
//
// For example: Assume our config is located in $HOME/.nerthus/cnts.yaml with
// a key "msp.configPath" = "msp/config.yaml".
//
// This function will return:
//      GetPath("msp.configPath") ->  $HOME/.nerthus/msp/config.yaml
//
//----------------------------------------------------------------------------------
func GetPath(key string) string {
	p := viper.GetString(key)
	if p == "" {
		return ""
	}

	return TranslatePath(filepath.Dir(viper.ConfigFileUsed()), p)
}

// 默认的项目Home目录
var OfficialPath = DefaultDataDir()

//----------------------------------------------------------------------------------
// InitViper()
//----------------------------------------------------------------------------------
// Performs basic initialization of our viper-based configuration layer.
// Primary thrust is to establish the paths that should be consulted to find
// the configuration we need.  If v == nil, we will initialize the global
// Viper instance
//----------------------------------------------------------------------------------
func InitViper(v *viper.Viper, configName string) error {
	var altPath = os.Getenv("CNTS_CFG_PATH")
	if altPath != "" {
		// If the user has overridden the path with an envvar, its the only path
		// we will consider
		addConfigPath(v, altPath)
	} else {
		// If we get here, we should use the default paths in priority order:
		//
		// *) CWD
		// *) The $GOPATH based development tree
		// *) /etc/nerthus/cnts
		//

		// CWD
		addConfigPath(v, "./")

		// --dev模式下执行
		if strings.HasSuffix(configName, ".dev") {
			// DevConfigPath
			err := AddDevConfigPath(v)
			if err != nil {
				if err != ErrMissingGoPath {
					return err
				}
			}
		}

		// And finally, the official path
		if dirExists(OfficialPath) {
			addConfigPath(v, OfficialPath)
		}
	}
	if v == nil {
		v = viper.GetViper()
	}
	v.SetEnvPrefix("cnts")
	replacer := strings.NewReplacer(".", "_")
	v.SetEnvKeyReplacer(replacer)
	v.AutomaticEnv()

	// Now set the configuration file.

	v.SetConfigName(configName)
	return nil
}

//----------------------------------------------------------------------------------
// AddDevConfigPath()
//----------------------------------------------------------------------------------
// Helper utility that automatically adds our DevConfigDir to the viper path
//----------------------------------------------------------------------------------
func AddDevConfigPath(v *viper.Viper) error {
	devPath, err := GetDevConfigDir()
	if err != nil {
		return err
	}

	addConfigPath(v, devPath)

	return nil
}

const datadirPathEnv = "NERTHUS_WS_PATH"

// 默认是的文件目录
func DefaultDataDir() string {
	if path := os.Getenv(datadirPathEnv); path != "" {
		return path
	}

	// Try to place the data folder in the user's home dir
	home, err := homedir.Dir()
	if err != nil {
		return ""
	}
	if home != "" {
		if runtime.GOOS == "darwin" {
			return filepath.Join(home, "Library", "Nerthus")
		} else if runtime.GOOS == "windows" {
			return filepath.Join(home, "AppData", "Roaming", "Nerthus")
		} else {
			return filepath.Join(home, ".nerthus")
		}
	}
	// As we cannot guess a stable location, return empty and handle later
	return ""
}

func getValue(key string, defv interface{}) interface{} {
	v := viper.Get(key)
	if v == nil {
		return defv
	}
	if vv, ok := v.(string); ok && vv == "" {
		return defv
	}
	return v
}

// 获取指定Key的配置信息，如果不存在则使用默认值.
// 需要确保获取的值是数字类型，否则将Panic报错，为了防止意外发生，需要强制处理。
func MustInt(key string, defv int) int {
	vv, err := cast.ToIntE(getValue(key, defv))
	if err != nil {
		panic(fmt.Errorf("failed to parse config item %q value: %v", key, err))
	}
	return vv
}

func MustFloat(key string, defv float32) float32 {
	vv, err := cast.ToFloat32E(getValue(key, defv))
	if err != nil {
		panic(fmt.Errorf("failed to parse config item %q value: %v", key, err))
	}
	return vv
}

func MustString(key string, defv string) string {
	vv, err := cast.ToStringE(getValue(key, defv))
	if err != nil {
		panic(fmt.Errorf("failed to parse config item %q value: %v", key, err))
	}
	return vv
}

func MustBool(key string, defv bool) bool {
	vv, err := cast.ToBoolE(getValue(key, defv))
	if err != nil {
		panic(fmt.Errorf("failed to parse config item %q value: %v", key, err))
	}
	return vv
}

func Set(key string, value interface{}) {
	viper.Set(key, value)
}
