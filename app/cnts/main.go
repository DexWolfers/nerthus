package main

import (
	"runtime"

	"gitee.com/nerthus/nerthus/app/cnts/cmd"

	_ "gitee.com/nerthus/nerthus/core/sc"     // 注册内置合约
	_ "gitee.com/nerthus/nerthus/core/vm/evm" // 注册EVM
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	cmd.Execute()
}
