package cmd

import (
	"fmt"
	"runtime"

	"gitee.com/nerthus/nerthus/params"

	"github.com/spf13/cobra"
)

// versionCmd Version命令
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version numbers",
	Long:  `All software has versions`,
	Run:   version,
}

func init() {
	RootCmd.AddCommand(versionCmd)
}

func version(cmd *cobra.Command, args []string) {
	fmt.Printf("Version:    \t %s\n", params.Version)
	fmt.Printf("Git commit: \t %s\n", params.GITCOMMIT)
	fmt.Printf("Built:      \t %s\n", params.BUILDTIME)
	fmt.Printf("OS/Arch:    \t %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("Go version: \t %s\n", runtime.Version())
}
