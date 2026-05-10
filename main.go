package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/smm-h/howmuchleft/internal/cli"
	"github.com/smm-h/howmuchleft/internal/migrate"
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
	migrate.SetFS(MigrationsFS)

	if err := cli.RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
