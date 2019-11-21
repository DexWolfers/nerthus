package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"gitee.com/nerthus/nerthus/app/utils"
	"gitee.com/nerthus/nerthus/common"
	"gitee.com/nerthus/nerthus/console"
	"gitee.com/nerthus/nerthus/core"
	"gitee.com/nerthus/nerthus/log"

	"github.com/spf13/cobra"
)

// initCmd represents the init data command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Bootstrap and initialize a new genesis unit",
	Long: `
The init command initializes a new genesis unit and definition for the network.
This is a destructive action and changes the network in which you will be
participating.

It expects the genesis file as argument.`,
	Run: initGenesis,
}
var removeDBCmd = &cobra.Command{
	Use:   "removedb",
	Short: "Remove dagchain and state databases",
	Long: `Remove all data file abount chain data under DATADIR dictionary.

include dir or file:
  1. dagdata
  2. statistics.db3
  3. txs
`,
	Run: remoteDB,
}

func init() {
	//RootCmd.AddCommand(initCmd)
	//enableDebug(initCmd)
	//
	removeDBCmd.Flags().AddFlag(utils.DataDirFlag)
	removeDBCmd.Flags().AddFlag(utils.TxPoolDiskTxPathFlag)
	RootCmd.AddCommand(removeDBCmd)
}

// initGenesis will initialise the given JSON format genesis file and writes it as
// the zero'd unit (i.e. genesis) or will fail hard if it can't succeed.
func initGenesis(cmd *cobra.Command, args []string) {
	if len(args) == 0 || args[0] == "" {
		utils.Fatalf("Must supply path to genesis JSON file from first arg")
	}
	// Make sure we have a valid genesis JSON
	genesisPath := args[0]
	file, err := os.Open(genesisPath)
	if err != nil {
		utils.Fatalf("Failed to read genesis file: %v", err)
	}
	defer file.Close()

	genesis := new(core.Genesis)
	if err := json.NewDecoder(file).Decode(genesis); err != nil {
		utils.Fatalf("invalid genesis file: %v", err)
	}

	flags := utils.Flags{FlagSet: RootCmd.Flags()}
	stack := makeFullNode(&flags)
	for _, name := range []string{"dagdata"} {
		chaindb, err := stack.OpenDatabase(name, 0, 0)
		if err != nil {
			utils.Fatalf("Failed to open database: %v", err)
		}
		_, hash, err := core.SetupGenesisUnit(chaindb, genesis)
		if err != nil {
			utils.Fatalf("Failed to write genesis unit: %v", err)
		}
		log.Info("Successfully wrote genesis state", "database", name, "hash", hash)
	}
}

// remoteDB 执行删除dagdata命令
// Author: @ysqi
func remoteDB(cmd *cobra.Command, args []string) {
	flags := utils.Flags{FlagSet: RootCmd.Flags()}

	nodeCfg := defaultNtsConfig().Node
	nodeCfg.DataDir = utils.GetDataDir(&flags)

	files := []struct {
		path  string
		exist bool
	}{
		{nodeCfg.ResolvePath("dagdata"), true},
		{nodeCfg.ResolvePath("statistics.db3"), true},
	}
	if f := flags.GetString(utils.TxPoolDiskTxPathFlag.Name); f != "" {
		files = append(files, struct {
			path  string
			exist bool
		}{nodeCfg.ResolvePath(f), true})
	}
	isEmpty := true
	for i, v := range files {
		ye := common.FileExist(v.path)
		files[i].exist = ye
		if !ye {
			fmt.Printf("Skip: %s\n", v.path)
		}
		if isEmpty && ye {
			isEmpty = false
		}
	}

	if isEmpty {
		fmt.Println("Nothing needs to be deleted")
		return
	}
	//  确认删除并执行
	fmt.Println("Will delete:")
	for _, v := range files {
		if v.exist {
			fmt.Println("\t", v.path)
		}
	}

	confirm, err := console.Stdin.PromptConfirm("Are you sure you want to delete these files?")
	switch {
	case err != nil:
		utils.Fatalf("%v", err)
	case !confirm:
		fmt.Println("Aborted to delete")
	default:
		start := time.Now()
		for _, v := range files {
			if v.exist {
				if err := os.RemoveAll(v.path); err != nil && !os.IsNotExist(err) {
					fmt.Printf("Failed to delete, %v \n", err)
				}
			}
		}
		fmt.Printf("Deleted completed,elapsed %s\n", common.PrettyDuration(time.Since(start)))
	}
}
