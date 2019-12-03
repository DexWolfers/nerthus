package params

import (
	"fmt"
)

// Version holds the textual version string.
// 版本信息应在每次发布新版本时修改
const (
	// VersionMajor 主版本号
	VersionMajor = 1
	// VersionMinor 次版本号
	VersionMinor = 0
	// VersionPatch 补丁号
	VersionPatch = 0
	// VersionMeta 版本名称
	VersionMeta = "stable"
)

var (
	// GITCOMMIT 在编译程序时自动覆盖
	GITCOMMIT = "HEAD"
	// BUILDTIME 在编译程序时自动覆盖
	BUILDTIME = "<unknown>"
)

// Version 含版本名称的版本信息
var Version = func() string {
	v := fmt.Sprintf("%d.%d.%d", VersionMajor, VersionMinor, VersionPatch)
	if VersionMeta != "" {
		v += "-" + VersionMeta
	}
	return v
}()

// VersionWithCommit 返回含GitCommitHash的版本信息
func VersionWithCommit() string {
	hash := ShortGitCommit()
	if hash == "" {
		return Version
	}
	return Version + "-" + hash
}

// ShortGitCommit 返回GitCommit的短地址
func ShortGitCommit() string {
	if len(GITCOMMIT) >= 8 {
		return GITCOMMIT[:8]
	}
	return ""
}
