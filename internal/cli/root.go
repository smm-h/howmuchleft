package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/smm-h/howmuchleft/internal/config"
	"github.com/smm-h/howmuchleft/internal/migrate"
	"github.com/smm-h/howmuchleft/internal/platform"
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
	Use:   "install",
	Short: "Install a profile",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("profile install (not yet implemented)")
	},
}

var profileUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall a profile",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("profile uninstall (not yet implemented)")
	},
}

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List profiles",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("profile list (not yet implemented)")
	},
}

var demoCmd = &cobra.Command{
	Use:   "demo",
	Short: "Run demo animation",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("demo (not yet implemented)")
	},
}

var colorsCmd = &cobra.Command{
	Use:   "colors",
	Short: "Test color rendering",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("colors (not yet implemented)")
	},
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show or edit configuration",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("config (not yet implemented)")
	},
}

func init() {
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
