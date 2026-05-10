package cli

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/smm-h/howmuchleft/internal/config"
	"github.com/smm-h/howmuchleft/internal/dashboard"
	"github.com/smm-h/howmuchleft/internal/demo"
	"github.com/smm-h/howmuchleft/internal/migrate"
	"github.com/smm-h/howmuchleft/internal/platform"
	"github.com/smm-h/howmuchleft/internal/render"
)

var appVersion string

// SetVersion sets the application version string.
func SetVersion(v string) {
	appVersion = v
}

// RootCmd is the top-level cobra command.
var RootCmd = &cobra.Command{
	Use:   "howmuchleft",
	Short: "Claude Code statusline tool",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Phase 1.3: startup migration flow
		// 1. Convert old JSON config to TOML if needed
		if err := config.ConvertJSONToTOML(); err != nil {
			fmt.Fprintf(os.Stderr, "howmuchleft: warning: JSON to TOML conversion failed: %v\n", err)
		}

		// 2. Run embedded schema migrations
		configDir := resolveConfigDir()
		if _, err := migrate.RunEmbedded(configDir); err != nil {
			fmt.Fprintf(os.Stderr, "howmuchleft: warning: migration failed: %v\n", err)
		}

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		fi, err := os.Stdin.Stat()
		if err != nil {
			return err
		}
		if (fi.Mode() & os.ModeCharDevice) == 0 {
			// stdin is a pipe — statusline mode
			return runStatusline()
		}
		// TTY — show help
		return cmd.Help()
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(appVersion)
	},
}

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage profiles",
}

var profileInstallCmd = &cobra.Command{
	Use:   "install [dir]",
	Short: "Add howmuchleft to a Claude Code profile",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		claudeDir := resolveClaudeDir(args)
		return profileInstall(claudeDir)
	},
}

var profileUninstallCmd = &cobra.Command{
	Use:   "uninstall [dir]",
	Short: "Remove howmuchleft from a Claude Code profile",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		claudeDir := resolveClaudeDir(args)
		return profileUninstall(claudeDir)
	},
}

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show all profiles' usage",
	RunE: func(cmd *cobra.Command, args []string) error {
		live, _ := cmd.Flags().GetBool("live")
		return dashboard.Run(live)
	},
}

var demoCmd = &cobra.Command{
	Use:   "demo [duration_seconds]",
	Short: "Run demo animation",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		duration := 60
		if len(args) > 0 {
			d, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid duration: %s", args[0])
			}
			duration = d
		}
		return demo.Run(duration)
	},
}

var colorsCmd = &cobra.Command{
	Use:   "colors",
	Short: "Preview gradient colors for your terminal",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := config.Get()
		barCfg := buildBarConfig(cfg)
		// Use a wider bar width for test-colors display
		testCfg := *barCfg
		testCfg.Width = 13
		fmt.Print(render.TestColors(&testCfg))
	},
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show config file and current settings",
	Run: func(cmd *cobra.Command, args []string) {
		showConfig()
	},
}

func init() {
	profileListCmd.Flags().Bool("live", false, "Refresh dashboard every 30s")
	profileCmd.AddCommand(profileInstallCmd, profileUninstallCmd, profileListCmd)
	RootCmd.AddCommand(versionCmd, profileCmd, demoCmd, colorsCmd, configCmd)
}

// resolveConfigDir returns the howmuchleft config directory path.
// Uses the Claude dir's parent structure to find the right location.
func resolveConfigDir() string {
	claudeDir := platform.GetClaudeDir()
	// The migrate package expects the config directory where config.toml lives.
	// That's ~/.config/howmuchleft/ (the TOML config dir, not the claude dir).
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	_ = claudeDir // claudeDir is for Claude state, config dir is separate
	return home + "/.config/howmuchleft"
}
