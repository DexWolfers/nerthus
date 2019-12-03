// Author: @ysqi

package debug

import (
	"fmt"
	"io"
	glog "log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"gitee.com/nerthus/nerthus/log"
	"gitee.com/nerthus/nerthus/log/term"

	"github.com/mattn/go-colorable"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Flags holds all command-line flags required for debugging.
var Flags = &pflag.FlagSet{}

var glogger *log.GlogHandler

func init() {
	Flags.Int("verbosity", 3, "Logging verbosity: 0=silent, 1=error, 2=warn, 3=info, 4=debug, 5=detail")
	Flags.String("vmodule", "", "Per-module verbosity: comma-separated list of <pattern>=<level> (e.g. nerthus/*=5,p2p=4)")
	Flags.String("backtrace", "", "Request a stack trace at a specific logging statement (e.g. \"dagchain.go:271\")")
	Flags.Bool("debug", false, "Prepends log messages with call-site location (file and line number)")
	Flags.Bool("pprof", false, "Enable the pprof HTTP server")
	Flags.Int("pprofport", 6060, "pprof HTTP server listening port")
	Flags.String("pprofaddr", "127.0.0.1", "pprof HTTP server listening interface")
	Flags.Int("memprofilerate", runtime.MemProfileRate, "Turn on memory profiling with the given rate")
	Flags.Int("blockprofilerate", 0, "Turn on block profiling with the given rate")
	Flags.String("cpuprofile", "", "Write CPU profile to the given file")
	Flags.String("trace", "", "Write execution trace to the given file")
	Flags.String("logdir", "", "Write log information to the given dictionary")
	Flags.String("logpath", "", "Write log information to the given file")

}

// Setup initializes profiling and logging based on the CLI flags.
// It should be called as early as possible in the program.
func Setup(cmd *cobra.Command) error {
	debug, err := cmd.Flags().GetBool("debug")
	if err != nil {
		return err
	}
	verbosity, err := cmd.Flags().GetInt("verbosity")
	if err != nil {
		return err
	}
	vmodule, err := cmd.Flags().GetString("vmodule")
	if err != nil {
		return err
	}
	backtrace, err := cmd.Flags().GetString("backtrace")
	if err != nil {
		return err
	}
	memprofilerate, err := cmd.Flags().GetInt("memprofilerate")
	if err != nil {
		return err
	}

	blockprofilerate, err := cmd.Flags().GetInt("blockprofilerate")
	if err != nil {
		return err
	}
	traceFile, err := cmd.Flags().GetString("trace")
	if err != nil {
		return err
	}

	cpuFile, err := cmd.Flags().GetString("cpuprofile")
	if err != nil {
		return err
	}

	logfile, err := cmd.Flags().GetString("logpath")
	if err != nil {
		return err
	}
	logDir, err := cmd.Flags().GetString("logdir")
	if err != nil {
		return err
	}
	switch {
	case logfile != "":
		f, err := os.Create(logfile)
		if err != nil {
			return err
		}
		glog.SetOutput(f) //存入日志流
		glogger = log.NewGlogHandler(log.StreamHandler(f, log.LogfmtFormat()))
	case logDir != "":
		f, err := os.Create(filepath.Join(logDir, fmt.Sprintf("%s.log", time.Now().Format("20060102_150405"))))
		if err != nil {
			return err
		}
		glog.SetOutput(f) //存入日志流

		//go func() {
		//	os.
		//	if s,err:= f.Stat();
		//		s.Size()
		//}()
		glogger = log.NewGlogHandler(log.StreamHandler(f, log.LogfmtFormat()))
	default:
		usecolor := term.IsTty(os.Stderr.Fd()) && os.Getenv("TERM") != "dumb"
		output := io.Writer(os.Stderr)
		glog.SetOutput(output) //存入日志流

		if usecolor {
			output = colorable.NewColorableStderr()
		}
		glogger = log.NewGlogHandler(log.StreamHandler(output, log.TerminalFormat(usecolor)))
	}

	log.PrintOrigins(debug)
	glogger.Verbosity(log.Lvl(verbosity))
	glogger.Vmodule(vmodule)
	glogger.BacktraceAt(backtrace)
	log.Root().SetHandler(glogger)

	// profiling, tracing
	runtime.MemProfileRate = memprofilerate
	Handler.SetBlockProfileRate(blockprofilerate)
	if traceFile != "" {
		if err := Handler.StartGoTrace(traceFile); err != nil {
			return err
		}
	}
	if cpuFile != "" {
		if err := Handler.StartCPUProfile(cpuFile); err != nil {
			return err
		}
	}

	pprof, err := cmd.Flags().GetBool("pprof")
	if err != nil {
		return err
	}
	// pprof server
	if pprof {
		pprofaddr, err := cmd.Flags().GetString("pprofaddr")
		if err != nil {
			return err
		}
		pprofport, err := cmd.Flags().GetInt("pprofport")
		if err != nil {
			return err
		}
		address := fmt.Sprintf("%s:%d", pprofaddr, pprofport)
		go func() {
			log.Info("Starting pprof server", "addr", fmt.Sprintf("http://%s/debug/pprof", address))
			if err := http.ListenAndServe(address, nil); err != nil {
				log.Error("Failure in running pprof server", "err", err)
			}
		}()
	}
	return nil
}

// Exit stops all running profiles, flushing their output to the
// respective file.
func Exit() {
	Handler.StopCPUProfile()
	Handler.StopGoTrace()
}
