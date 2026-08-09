package main

import (
	"os"
	"runtime/debug"

	"github.com/smm-h/howmuchleft/internal/cli"
)

var version string

func main() {
	if version == "" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
			version = info.Main.Version
		} else {
			version = "dev"
		}
	}
	cli.SetVersion(version)

	// If stdin is piped and no subcommand args, run statusline directly.
	if len(os.Args) == 1 {
		if cli.RunStatuslineDirect() {
			return
		}
	}

	app := cli.NewApp()
	app.Run()
}
