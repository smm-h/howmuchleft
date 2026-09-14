// Package cli builds howmuchleft's strictcli command tree and runs the
// statusline itself: it parses the status object Claude Code pipes on stdin,
// gathers usage, git and profile data, and hands the result to internal/render.
package cli

import (
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/smm-h/howmuchleft/internal/config"
	"github.com/smm-h/howmuchleft/internal/dashboard"
	"github.com/smm-h/howmuchleft/internal/demo"
	"github.com/smm-h/howmuchleft/internal/migrate"
	"github.com/smm-h/howmuchleft/internal/platform"
	"github.com/smm-h/howmuchleft/internal/render"
	"github.com/smm-h/strictcli/go/strictcli"
)

var appVersion string

// SetVersion sets the application version string.
func SetVersion(v string) {
	appVersion = v
}

var migrateOnce sync.Once

// runMigrations runs JSON-to-TOML conversion and fills the config file with
// any settings it is missing. Safe to call multiple times; work is done once.
func runMigrations() {
	migrateOnce.Do(func() {
		if err := config.ConvertJSONToTOML(); err != nil {
			fmt.Fprintf(os.Stderr, "howmuchleft: warning: JSON to TOML conversion failed: %v\n", err)
		}
		configDir := resolveConfigDir()
		if _, err := migrate.EnsureDefaults(configDir); err != nil {
			fmt.Fprintf(os.Stderr, "howmuchleft: warning: config migration failed: %v\n", err)
		}
	})
}

// RunStatuslineDirect checks if stdin is piped and runs the statusline.
// Returns true if it handled the invocation (caller should exit), false otherwise.
func RunStatuslineDirect() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		fmt.Fprintf(os.Stderr, "howmuchleft: %v\n", err)
		os.Exit(1)
	}
	if (fi.Mode() & os.ModeCharDevice) == 0 {
		// stdin is a pipe -- statusline mode
		runMigrations()
		if err := runStatusline(); err != nil {
			fmt.Fprintf(os.Stderr, "howmuchleft: %v\n", err)
			os.Exit(1)
		}
		return true
	}
	return false
}

// NewApp builds and returns the strictcli application.
//
// Every command is classified strictcli.EffectMutating, including the ones
// that only print. That is not a rubber stamp: every handler opens with
// runMigrations(), which converts a legacy ~/.config/howmuchleft.json to TOML
// (renaming the original to .bak) and writes any missing settings into
// ~/.config/howmuchleft/config.toml. Those are user-visible
// filesystem mutations, so no command here can honestly claim read_only.
// classification_test.go pins the table.
func NewApp() *strictcli.App {
	app := strictcli.NewApp("howmuchleft", appVersion, "a Claude Code statusline that shows context window, 5-hour and weekly limit usage as three progress bars with sub-cell precision, shading each from green to red as it fills")

	// version
	app.Command("version", "Print the version", func(ctx *strictcli.Context, kwargs map[string]interface{}) strictcli.Outcome {
		runMigrations()
		fmt.Println(appVersion)
		return strictcli.Exit(0)
	}, strictcli.WithEffect(strictcli.EffectMutating))

	// profile group
	profileGrp := app.Group("profile", "Install, remove and inspect the Claude Code profiles howmuchleft tracks: wire the statusLine into a profile's settings.json, take it back out again, and show every registered profile's token usage side by side in one dashboard")

	profileGrp.Command("install", "Add howmuchleft as the statusLine in a Claude Code profile's settings.json, writing a command entry that points back at that profile directory, then register the profile so it shows up in the dashboard. Reports and overwrites a statusLine another tool configured, and changes nothing when howmuchleft is already installed there", func(ctx *strictcli.Context, kwargs map[string]interface{}) strictcli.Outcome {
		runMigrations()
		var args []string
		if dir := kwargs["dir"]; dir != nil {
			args = append(args, dir.(string))
		}
		claudeDir := resolveClaudeDir(args)
		if err := profileInstall(claudeDir); err != nil {
			fmt.Fprintf(os.Stderr, "howmuchleft: %v\n", err)
			return strictcli.Exit(1)
		}
		return strictcli.Exit(0)
	}, strictcli.WithEffect(strictcli.EffectMutating), strictcli.WithArgs(
		strictcli.NewArg("dir", "Claude Code profile directory to operate on; defaults to $CLAUDE_CONFIG_DIR when that is set, and to ~/.claude otherwise. A leading ~ is expanded to your home directory", strictcli.ArgOptional()),
	))

	profileGrp.Command("uninstall", "Remove howmuchleft's statusLine entry from a Claude Code profile's settings.json and unregister the profile so it no longer appears in the dashboard. Refuses to touch a statusLine that some other tool configured, printing it for inspection instead, and reports plainly when the profile has no statusLine at all", func(ctx *strictcli.Context, kwargs map[string]interface{}) strictcli.Outcome {
		runMigrations()
		var args []string
		if dir := kwargs["dir"]; dir != nil {
			args = append(args, dir.(string))
		}
		claudeDir := resolveClaudeDir(args)
		if err := profileUninstall(claudeDir); err != nil {
			fmt.Fprintf(os.Stderr, "howmuchleft: %v\n", err)
			return strictcli.Exit(1)
		}
		return strictcli.Exit(0)
	}, strictcli.WithEffect(strictcli.EffectMutating), strictcli.WithArgs(
		strictcli.NewArg("dir", "Claude Code profile directory to operate on; defaults to $CLAUDE_CONFIG_DIR when that is set, and to ~/.claude otherwise. A leading ~ is expanded to your home directory", strictcli.ArgOptional()),
	))

	profileGrp.Command("list", "Discover every registered Claude Code profile and render their token usage side by side in one dashboard, a row per profile. Prints a single snapshot and exits by default; with --live it redraws every 30 seconds until interrupted. Reports that none were found, and exits cleanly, when no profile has been registered yet", func(ctx *strictcli.Context, kwargs map[string]interface{}) strictcli.Outcome {
		runMigrations()
		live, _ := kwargs["live"].(bool)
		if err := dashboard.Run(live); err != nil {
			fmt.Fprintf(os.Stderr, "howmuchleft: %v\n", err)
			return strictcli.Exit(1)
		}
		return strictcli.Exit(0)
	}, strictcli.WithEffect(strictcli.EffectMutating), strictcli.WithFlags(
		strictcli.BoolFlag("live", "Redraw the dashboard every 30 seconds until interrupted, instead of printing a single snapshot and exiting immediately", strictcli.Optional()),
	))

	// demo
	app.Command("demo", "Run demo animation", func(ctx *strictcli.Context, kwargs map[string]interface{}) strictcli.Outcome {
		runMigrations()
		duration := 60
		if ds := kwargs["duration_seconds"]; ds != nil {
			d, err := strconv.Atoi(ds.(string))
			if err != nil {
				fmt.Fprintf(os.Stderr, "howmuchleft: invalid duration: %s\n", ds.(string))
				return strictcli.Exit(1)
			}
			duration = d
		}
		if err := demo.Run(duration); err != nil {
			fmt.Fprintf(os.Stderr, "howmuchleft: %v\n", err)
			return strictcli.Exit(1)
		}
		return strictcli.Exit(0)
	}, strictcli.WithEffect(strictcli.EffectMutating), strictcli.WithArgs(
		strictcli.NewArg("duration_seconds", "Duration in seconds", strictcli.ArgOptional()),
	))

	// colors
	app.Command("colors", "Preview gradient colors for your terminal", func(ctx *strictcli.Context, kwargs map[string]interface{}) strictcli.Outcome {
		runMigrations()
		cfg := config.Get()
		barCfg := render.BuildBarConfig(cfg)
		testCfg := *barCfg
		testCfg.Width = 13
		fmt.Print(render.TestColors(&testCfg))
		return strictcli.Exit(0)
	}, strictcli.WithEffect(strictcli.EffectMutating))

	// config
	app.Command("config", "Show config file and current settings", func(ctx *strictcli.Context, kwargs map[string]interface{}) strictcli.Outcome {
		runMigrations()
		showConfig()
		return strictcli.Exit(0)
	}, strictcli.WithEffect(strictcli.EffectMutating))

	return app
}

// resolveConfigDir returns the howmuchleft config directory path.
func resolveConfigDir() string {
	claudeDir := platform.GetClaudeDir()
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	_ = claudeDir
	return home + "/.config/howmuchleft"
}
