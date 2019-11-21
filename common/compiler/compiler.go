// Package compiler wraps the Witstone compiler executable (wist).
package compiler

import "sync"

type CompilerInfo struct {
	Path, Version, FullVersion string
	Major, Minor, Patch        int
}

type Contract struct {
	Name string       `json:"name"`
	Code string       `json:"code"`
	Info ContractInfo `json:"info"`
}

type ContractInfo struct {
	Source          string      `json:"source"`
	Language        string      `json:"language"`
	LanguageVersion string      `json:"languageVersion"`
	CompilerVersion string      `json:"compilerVersion"`
	CompilerOptions string      `json:"compilerOptions"`
	AbiDefinition   interface{} `json:"abi"`
	UserDoc         interface{} `json:"userDoc"`
	DeveloperDoc    interface{} `json:"developerDoc"`
	Metadata        string      `json:"metadata"`
}

type Compiler interface {
	// Info 获取编译器信息
	Info() (CompilerInfo, error)
	// Compile 编译源代码字符串
	Compile(souce string) ([]*Contract, error)
	// CompilerFiles 编译指定的多个源文件
	CompileFiles(sourcefiles ...string) ([]*Contract, error)
}

var (
	compilers = make(map[string]Compiler, 3)

	regMutex sync.RWMutex
)

// Register 重复注册将引发异常
func Register(language string, c Compiler) {
	regMutex.Lock()
	defer regMutex.Unlock()

	if c == nil {
		panic("Register compiler is nil")
	}
	if _, dup := compilers[language]; dup {
		panic("Register called twice for compilers" + language)
	}
	compilers[language] = c
}

// Get 获取已注册的编译器
func Get(language string) Compiler {
	regMutex.Lock()
	defer regMutex.Unlock()
	return compilers[language]
}
