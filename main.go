// Command howmuchleft is a Claude Code statusline that shows context window,
// 5-hour and weekly limit usage as three progress bars with sub-cell precision,
// shading each from green to red as it fills.
//
// Claude Code pipes a JSON status object on stdin on every render; the binary
// writes three lines of ANSI text to stdout and exits. Invoked without piped
// stdin it behaves as an ordinary CLI, with commands for installing the
// statusline into a Claude Code profile, listing profiles, running the demo,
// previewing colors, inspecting the config and printing the version.
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
