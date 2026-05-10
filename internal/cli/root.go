package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		fi, err := os.Stdin.Stat()
		if err != nil {
			return err
		}
		if (fi.Mode() & os.ModeCharDevice) == 0 {
			// stdin is a pipe — statusline mode
			fmt.Println("statusline mode (not yet implemented)")
			return nil
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
