package main

import (
	"fmt"
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

	if err := cli.RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
