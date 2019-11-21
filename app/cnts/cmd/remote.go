package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"gitee.com/nerthus/nerthus/app/utils"
	"gitee.com/nerthus/nerthus/console"
	"gitee.com/nerthus/nerthus/node"
	"gitee.com/nerthus/nerthus/rpc"
)

var (
	remoteCommand = cobra.Command{
		Use:   "attach",
		Short: "start an interactive JavaScript environment (connect to node)",
		Long: `
The cnts console is an interactive shell for the JavaScript runtime environment
which exposes a node admin interface as well as the Ðapp JavaScript API. 
`,
		Run: remoteConsole,
	}
)

func init() {
	remoteCommand.Flags().AddFlag(utils.DataDirFlag)
	remoteCommand.Flags().AddFlag(utils.TestnetFlag)
	remoteCommand.Flags().AddFlag(utils.DevModeFlag)
	remoteCommand.Flags().AddFlag(utils.JSpathFlag)
	remoteCommand.Flags().AddFlag(utils.ExecFlag)
	remoteCommand.Flags().AddFlag(utils.PreloadJSFlag)
	remoteCommand.Flags().StringP("endpoint", "", "", "the endpoint used for connect to node")

	RootCmd.AddCommand(&remoteCommand)
}

// 通过 endpoint 可远程连接到节点，通过 API 访问数据。
func remoteConsole(cmd *cobra.Command, args []string) {
	flags := getFlags(cmd)

	// Attach to a remotely running geth instance and start the JavaScript console
	endpoint := flags.GetString("endpoint")
	if endpoint == "" {
		endpoint = fmt.Sprintf("%s/%s.ipc", utils.GetDataDir(flags), clientIdentifier)
	}

	client, err := dialRPC(endpoint)
	if err != nil {
		utils.Fatalf("Unable to attach to remote geth: %v", err)
	}
	config := console.Config{
		DataDir: utils.GetDataDir(flags),
		DocRoot: flags.GetString(utils.JSpathFlag.Name),
		Client:  client,
		Preload: utils.MakeConsolePreloads(flags),
	}

	console, err := console.New(config)
	if err != nil {
		utils.Fatalf("Failed to start the JavaScript console: %v", err)
	}
	defer console.Stop(false)

	if script := flags.GetString(utils.ExecFlag.Name); script != "" {
		if err := console.Evaluate(script); err != nil {
			utils.Fatalf("Failed to exec script:%v", err)
		}
		return
	}

	// Otherwise print the welcome screen and enter interactive mode
	console.Welcome()
	console.Interactive()

}

// dialRPC returns a RPC client which connects to the given endpoint.
// The check for empty endpoint implements the defaulting logic
// for "geth attach" and "geth monitor" with no argument.
func dialRPC(endpoint string) (*rpc.Client, error) {
	if endpoint == "" {
		endpoint = node.DefaultIPCEndpoint(clientIdentifier)
	} else if strings.HasPrefix(endpoint, "rpc:") || strings.HasPrefix(endpoint, "ipc:") {
		// Backwards compatibility with geth < 1.5 which required
		// these prefixes.
		endpoint = endpoint[4:]
	}
	return rpc.Dial(endpoint)
}
