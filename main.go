package main

import (
	"os"
	"runtime/debug"

	"github.com/smm-h/howmuchleft/internal/cli"
)

// Version is set by ldflags at build time: -X main.Version=x.y.z
// The name must stay exported and spelled this way: .goreleaser.yml injects
// main.Version, and the linker silently does nothing when the symbol is absent.
var Version string

func main() {
	if Version == "" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
			Version = info.Main.Version
		} else {
			Version = "dev"
		}
	}
	cli.SetVersion(Version)

	// If stdin is piped and no subcommand args, run statusline directly.
	if len(os.Args) == 1 {
		if cli.RunStatuslineDirect() {
			return
		}
	}

	app := cli.NewApp()
	app.Run()
}
