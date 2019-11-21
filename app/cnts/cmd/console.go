// Author: @ysqi

package cmd

import (
	"gitee.com/nerthus/nerthus/app/utils"
	"gitee.com/nerthus/nerthus/console"
	"github.com/spf13/cobra"
)

var (
	consoleCommand = cobra.Command{
		Use:   "console",
		Short: "Start an interactive JavaScript environment",
		Long: `
The cnts console is an interactive shell for the JavaScript runtime environment
which exposes a node admin interface as well as the Ðapp JavaScript API.`,
		Run:              localConsole,
		TraverseChildren: true,
	}
)

func init() {
	consoleCommand.Flags().AddFlag(utils.JSpathFlag)
	consoleCommand.Flags().AddFlag(utils.ExecFlag)
	consoleCommand.Flags().AddFlag(utils.PreloadJSFlag)
	RootCmd.AddCommand(&consoleCommand)
	enableDebug(&consoleCommand)
}

// localConsole starts a new geth node, attaching a JavaScript console to it at the
// same time.
func localConsole(cmd *cobra.Command, args []string) {
	flags := getFlags(cmd)
	// Create and start the node based on the CLI flags
	node := makeFullNode(flags)
	startNode(flags, node)
	defer node.Stop()

	// Attach to the newly started node and start the JavaScript console
	client, err := node.Attach()
	if err != nil {
		utils.Fatalf("Failed to attach to the inproc cnts: %v", err)
	}
	config := console.Config{
		DataDir: node.DataDir(),
		DocRoot: flags.GetString(utils.JSpathFlag.Name),
		Client:  client,
		Preload: utils.MakeConsolePreloads(flags),
	}

	cons, err := console.New(config)
	if err != nil {
		utils.Fatalf("Failed to start the JavaScript console: %v", err)
	}
	defer cons.Stop(false)

	// If only a short execution was requested, evaluate and return
	if script := flags.GetString(utils.ExecFlag.Name); script != "" {
		cons.Evaluate(script)
	}
	// Otherwise print the welcome screen and enter interactive mode
	cons.Welcome()
	cons.Interactive()
}
