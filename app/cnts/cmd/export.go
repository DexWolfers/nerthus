package cmd

import (
	"gitee.com/nerthus/nerthus/app/utils"
	"gitee.com/nerthus/nerthus/export"
	"gitee.com/nerthus/nerthus/ntsdb"
	"github.com/spf13/cobra"
)

var (
	exportCommand = cobra.Command{
		Use:              "export",
		Short:            "Export chain data to file",
		Long:             `Export chain data to file`,
		Run:              exportChainData,
		TraverseChildren: true,
	}
)

func init() {
	exportCommand.Flags().AddFlag(utils.ExportPathFlag)
	exportCommand.Flags().AddFlag(utils.ExportSinceFlag)

	RootCmd.AddCommand(&exportCommand)
	enableDebug(&exportCommand)
}

func exportChainData(cmd *cobra.Command, args []string) {
	flags := getFlags(cmd)
	_, cfg := makeConfigNode(flags)
	db, err := ntsdb.NewLDBReadOnly(cfg.Node.ResolvePath("dagdata"), 16, 16)
	if err != nil {
		utils.Fatalf("failed init db, error:%v", err)
		return
	}
	output := flags.GetString(utils.ExportPathFlag.Name)
	if len(output) == 0 {
		utils.Fatalf("invalid value of param 'path' ")
	}
	since := flags.GetUint64(utils.ExportSinceFlag.Name)
	if err := export.ExportChainData(db, since, output); err != nil {
		utils.Fatalf("failed export chain data, error:%v", err)
	}
}
