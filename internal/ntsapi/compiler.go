package ntsapi

import (
	"fmt"

	"gitee.com/nerthus/nerthus/common/compiler"
)

type CompilerAPI struct {
	name string // 智能合约语言名称
}

// Version 查看编译器版本信息
func (c *CompilerAPI) Version() (interface{}, error) {
	compile := compiler.Get(c.name)
	if compile == nil {
		return nil, fmt.Errorf("missing compiler %s", c.name)
	}
	info, err := compile.Info()
	if err != nil {
		return nil, err
	}

	return struct {
		Version string
		Path    string
	}{
		info.Version, info.Path,
	}, nil
}

// Compile 编译智能合约代码
// 如果只是单个合约，则返回一个合约信息，否则返回多个合约信息。
// 合约信息见：compiler.Contract，包含各类信息（bin,abi）
func (c *CompilerAPI) Compile(src string) (interface{}, error) {
	compile := compiler.Get(c.name)
	if compile == nil {
		return nil, fmt.Errorf("missing compiler %s", c.name)
	}
	result, err := compile.Compile(src)
	if err != nil {
		return nil, err
	}

	if len(result) == 1 {
		return result[0], nil
	}
	return result, err
}
