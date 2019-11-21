// Author: @ysqi

package utils

import (
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"gitee.com/nerthus/nerthus/common/homedir"
	"gitee.com/nerthus/nerthus/common/math"
)

// FilepathString 文件路径字符串，在复制时将自动解析其中的环境变量，并清理文件路径
type FilepathString string

func (f *FilepathString) String() string {
	return string(*f)
}

// Set a file path.
func (f *FilepathString) Set(val string) error {
	p := expandPath(val)
	if _, err := os.Stat(p); err != nil {
		return err
	}
	*f = FilepathString(p)
	return nil
}
func (f *FilepathString) Type() string {
	return "string"
}

type ChooseString struct {
	items []string
	val   string
}

func (c *ChooseString) String() string {
	return c.val
}

func (c *ChooseString) Set(val string) error {
	if len(c.items) == 0 {
		return errors.New("choose list is empty")
	}
	lower := strings.ToLower(val)
	for _, v := range c.items {
		if lower == strings.ToLower(v) {
			c.val = val
			return nil
		}
	}
	return fmt.Errorf("you must choose from %v", c.items)
}
func (c *ChooseString) Type() string {
	return "string"
}

// BigValue turns *big.Int into a flag.Value
type BigValue big.Int

func (b *BigValue) String() string {
	if b == nil {
		return ""
	}
	return (*big.Int)(b).String()
}

func (b *BigValue) Set(val string) error {
	v, ok := math.ParseBig256(val)
	if !ok {
		return errors.New("invalid integer syntax")
	}
	*b = (BigValue)(*v)
	return nil
}

func (b *BigValue) Type() string {
	return "bigint"
}

// 扩展文件路径
// 1. 将"~/" 和"~\\"替换为HOME目录
// 2. 扩展环境变量
// 3. 清理路径：如. /a/b/../c -> /a/c
func expandPath(p string) string {
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home := HomeDir(); home != "" {
			p = filepath.Join(home, p[2:])
		}
	}
	return filepath.Clean(os.ExpandEnv(p))
}

func HomeDir() string {
	home, err := homedir.Dir()
	if err != nil {
		Fatal(err)
	}
	return home
}
