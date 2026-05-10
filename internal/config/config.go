package config

// Config holds all configuration fields for howmuchleft.
type Config struct {
	ColorMode              string       `toml:"color_mode"`
	ProgressLength         int          `toml:"progress_length"`
	Colors                 []ColorEntry `toml:"colors"`
	PartialBlocks          string       `toml:"partial_blocks"`
	ProgressBarOrientation string       `toml:"progress_bar_orientation"`
	CwdMaxLength           int          `toml:"cwd_max_length"`
	CwdDepth               int          `toml:"cwd_depth"`
	ShowTimeBars           bool         `toml:"show_time_bars"`
	TimeBarDim             float64      `toml:"time_bar_dim"`
	Lines                  LinesConfig  `toml:"lines"`
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

// Load reads and parses a TOML config file. Stub for now.
func Load(path string) (*Config, error) {
	return nil, nil
}
