package cli

import (
	"fmt"
	"os"

	"github.com/smm-h/howmuchleft/internal/config"
)

// showConfig prints the configuration file path and resolved settings.
func showConfig() {
	cfgPath := config.ConfigPath()
	oldPath := config.OldConfigPath()

	fmt.Printf("Config file: %s\n\n", cfgPath)

	// Check if config was migrated from JSON
	if _, err := os.Stat(oldPath + ".bak"); err == nil {
		fmt.Println("(Migrated from JSON — backup at " + oldPath + ".bak)")
		fmt.Println()
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		return
	}

	// Check if config file exists
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		fmt.Println("No config file found. Using defaults.")
		fmt.Println()
	}

	fmt.Println("Resolved settings:")
	fmt.Printf("  color_mode:               %s\n", cfg.ColorMode)
	fmt.Printf("  progress_length:          %d\n", cfg.ProgressLength)
	fmt.Printf("  partial_blocks:           %s\n", cfg.PartialBlocks)
	fmt.Printf("  progress_bar_orientation: %s\n", cfg.ProgressBarOrientation)
	fmt.Printf("  cwd_max_length:           %d\n", cfg.CwdMaxLength)
	fmt.Printf("  cwd_depth:                %d\n", cfg.CwdDepth)

	if cfg.ShowTimeBars != nil {
		fmt.Printf("  show_time_bars:           %v\n", *cfg.ShowTimeBars)
	}
	if cfg.TimeBarDim != nil {
		fmt.Printf("  time_bar_dim:             %.2f\n", *cfg.TimeBarDim)
	}

	if len(cfg.Colors) > 0 {
		fmt.Printf("  colors:                   %d entries\n", len(cfg.Colors))
	} else {
		fmt.Printf("  colors:                   (using built-in defaults)\n")
	}

	if cfg.Lines != nil {
		fmt.Printf("  lines:                    custom\n")
	} else {
		fmt.Printf("  lines:                    (using defaults)\n")
	}

	fmt.Println()

	if len(cfg.Profiles) > 0 {
		fmt.Printf("Registered profiles (%d):\n", len(cfg.Profiles))
		for _, p := range cfg.Profiles {
			fmt.Printf("  %s\n", p)
		}
	} else {
		fmt.Println("Profiles: (none registered -- run howmuchleft profile install)")
	}
}
