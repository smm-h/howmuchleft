package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	tomledit "github.com/smm-h/go-toml-edit"
)

const (
	ConfigDir  = "~/.config/howmuchleft"
	ConfigFile = "config.toml"
)

// ConfigPath resolves ~ to the actual home directory and returns the full path.
func ConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".config", "howmuchleft", ConfigFile)
}

// Config holds all configuration fields for howmuchleft.
type Config struct {
	ColorMode              string       `toml:"color_mode"`
	ProgressLength         int          `toml:"progress_length"`
	Colors                 []ColorEntry `toml:"colors"`
	PartialBlocks          string       `toml:"partial_blocks"`
	ProgressBarOrientation string       `toml:"progress_bar_orientation"`
	CwdMaxLength           int          `toml:"cwd_max_length"`
	CwdDepth               int          `toml:"cwd_depth"`
	ShowTimeBars           *bool        `toml:"show_time_bars"`
	TimeBarDim             *float64     `toml:"time_bar_dim"`
	Lines                  *LinesConfig `toml:"lines"`
	Profiles               []string     `toml:"profiles"`
}

// ColorEntry defines a gradient with optional conditions.
type ColorEntry struct {
	Gradient  interface{} `toml:"gradient"`
	Bg        interface{} `toml:"bg"`
	DarkMode  *bool       `toml:"dark_mode"`
	TrueColor *bool       `toml:"true_color"`
}

// LinesConfig defines the content of each output line.
type LinesConfig struct {
	Line1 []string `toml:"line1"`
	Line2 []string `toml:"line2"`
	Line3 []string `toml:"line3"`
}

// Default returns a Config with all default values filled in.
func Default() *Config {
	showTimeBars := true
	timeBarDim := 0.25
	return &Config{
		ColorMode:              "auto",
		ProgressLength:         12,
		PartialBlocks:          "auto",
		ProgressBarOrientation: "vertical",
		CwdMaxLength:           50,
		CwdDepth:               3,
		ShowTimeBars:           &showTimeBars,
		TimeBarDim:             &timeBarDim,
	}
}

// Load reads and parses a TOML config file at path.
// If the file doesn't exist, returns default config (not an error).
// If the file has parse errors, logs a warning to stderr and returns default config.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return Default(), nil
	}

	cfg := &Config{}
	if err := tomledit.Unmarshal(data, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "howmuchleft: warning: failed to parse config %s: %v\n", path, err)
		return Default(), nil
	}

	validate(cfg)
	return cfg, nil
}

// validate applies clamping and default-filling to a parsed config.
func validate(cfg *Config) {
	// ProgressLength: int 3-40, default 12
	if cfg.ProgressLength < 3 || cfg.ProgressLength > 40 {
		cfg.ProgressLength = 12
	}

	// ColorMode: must be "truecolor" or "256" or "auto"
	switch cfg.ColorMode {
	case "truecolor", "256", "auto":
		// valid
	default:
		cfg.ColorMode = "auto"
	}

	// PartialBlocks: must be "true" or "false" or "auto"
	switch cfg.PartialBlocks {
	case "true", "false", "auto":
		// valid
	default:
		cfg.PartialBlocks = "auto"
	}

	// ProgressBarOrientation: must be "vertical" or "horizontal"
	switch cfg.ProgressBarOrientation {
	case "vertical", "horizontal":
		// valid
	default:
		cfg.ProgressBarOrientation = "vertical"
	}

	// CwdMaxLength: int 10-100, default 50
	if cfg.CwdMaxLength < 10 || cfg.CwdMaxLength > 100 {
		cfg.CwdMaxLength = 50
	}

	// CwdDepth: int 1-10, default 3
	if cfg.CwdDepth < 1 || cfg.CwdDepth > 10 {
		cfg.CwdDepth = 3
	}

	// ShowTimeBars: boolean, default true
	if cfg.ShowTimeBars == nil {
		t := true
		cfg.ShowTimeBars = &t
	}

	// TimeBarDim: float 0-1, default 0.25
	if cfg.TimeBarDim == nil {
		d := 0.25
		cfg.TimeBarDim = &d
	} else if *cfg.TimeBarDim < 0 || *cfg.TimeBarDim > 1 {
		d := 0.25
		cfg.TimeBarDim = &d
	}

	// Lines: if present, must have Line1, Line2, Line3 each as string arrays
	if cfg.Lines != nil {
		if cfg.Lines.Line1 == nil || cfg.Lines.Line2 == nil || cfg.Lines.Line3 == nil {
			cfg.Lines = nil
		}
	}
}

// Per-process caching
var (
	cachedConfig *Config
	cacheOnce    sync.Once
)

// Get returns the cached config, loading it on first call.
func Get() *Config {
	cacheOnce.Do(func() {
		cfg, _ := Load(ConfigPath())
		cachedConfig = cfg
	})
	return cachedConfig
}

// ResetCache clears the cached config (useful for testing).
func ResetCache() {
	cacheOnce = sync.Once{}
	cachedConfig = nil
}
