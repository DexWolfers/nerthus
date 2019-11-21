package cmd

import (
	"context"
	"gitee.com/nerthus/nerthus/app/utils"
	"gitee.com/nerthus/nerthus/export"
	"gitee.com/nerthus/nerthus/node"
	"gitee.com/nerthus/nerthus/nts"
	"github.com/spf13/cobra"
)

var (
	importCommand = cobra.Command{
		Use:              "import",
		Short:            "Import chain data from file",
		Long:             `Import chain data from file`,
		Run:              importChainData,
		TraverseChildren: true,
	}
)

func init() {
	importCommand.Flags().AddFlag(utils.ExportPathFlag)

	RootCmd.AddCommand(&importCommand)
	enableDebug(&importCommand)
}
func importChainData(cmd *cobra.Command, args []string) {
	flags := getFlags(cmd)
	source := flags.GetString(utils.ExportPathFlag.Name)
	if len(source) == 0 {
		utils.Fatalf("invalid value of param 'path' ")
	}
	stack, cfg := makeConfigNode(flags)
	cfg.NTS.TxPool.Disabled = true // 禁用交易池
	ctx, cancel := context.WithCancel(context.Background())
	err := stack.Register(func(sctx *node.ServiceContext) (node.Service, error) {
		fullNode, err := nts.New(sctx, &cfg.NTS)
		if err == nil {
			fullNode.ProtocolHandler = nil
			go func() {
				if err := export.ImportChainData(ctx, fullNode.DagChain(), source); err == nil {
					go stack.Stop()
				} else {
					utils.Fatalf("failed import chain data, error:%v", err)
				}
			}()
		}
		return fullNode, err
	})
	if err != nil {
		utils.Fatalf("failed register, error:%v", err)
	}
	utils.StartNode(stack)
	stack.Wait()
	cancel()
	stack.Stop()
}
