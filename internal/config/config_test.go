package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultValues(t *testing.T) {
	cfg := Default()
	if cfg.ColorMode != "auto" {
		t.Errorf("ColorMode: got %q, want %q", cfg.ColorMode, "auto")
	}
	if cfg.ProgressLength != 12 {
		t.Errorf("ProgressLength: got %d, want %d", cfg.ProgressLength, 12)
	}
	if cfg.PartialBlocks != "auto" {
		t.Errorf("PartialBlocks: got %q, want %q", cfg.PartialBlocks, "auto")
	}
	if cfg.ProgressBarOrientation != "vertical" {
		t.Errorf("ProgressBarOrientation: got %q, want %q", cfg.ProgressBarOrientation, "vertical")
	}
	if cfg.CwdMaxLength != 50 {
		t.Errorf("CwdMaxLength: got %d, want %d", cfg.CwdMaxLength, 50)
	}
	if cfg.CwdDepth != 3 {
		t.Errorf("CwdDepth: got %d, want %d", cfg.CwdDepth, 3)
	}
	if cfg.ShowTimeBars == nil || !*cfg.ShowTimeBars {
		t.Errorf("ShowTimeBars: got %v, want true", cfg.ShowTimeBars)
	}
	if cfg.TimeBarDim == nil || *cfg.TimeBarDim != 0.25 {
		t.Errorf("TimeBarDim: got %v, want 0.25", cfg.TimeBarDim)
	}
	if cfg.Lines != nil {
		t.Errorf("Lines: got %v, want nil", cfg.Lines)
	}
}

func TestLoadNonexistentFile(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return defaults
	if cfg.ProgressLength != 12 {
		t.Errorf("ProgressLength: got %d, want %d", cfg.ProgressLength, 12)
	}
	if cfg.ColorMode != "auto" {
		t.Errorf("ColorMode: got %q, want %q", cfg.ColorMode, "auto")
	}
}

func TestLoadValidConfig(t *testing.T) {
	toml := `
color_mode = "truecolor"
progress_length = 20
partial_blocks = "false"
progress_bar_orientation = "horizontal"
cwd_max_length = 80
cwd_depth = 5
show_time_bars = false
time_bar_dim = 0.5

[lines]
line1 = ["elapsed", "profile", "tier"]
line2 = ["context_bar"]
line3 = ["cost", "cwd"]

[[colors]]
gradient = [[255, 0, 0], [0, 255, 0], [0, 0, 255]]
bg = [40, 40, 40]
dark_mode = true
true_color = true
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(toml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ColorMode != "truecolor" {
		t.Errorf("ColorMode: got %q, want %q", cfg.ColorMode, "truecolor")
	}
	if cfg.ProgressLength != 20 {
		t.Errorf("ProgressLength: got %d, want %d", cfg.ProgressLength, 20)
	}
	if cfg.PartialBlocks != "false" {
		t.Errorf("PartialBlocks: got %q, want %q", cfg.PartialBlocks, "false")
	}
	if cfg.ProgressBarOrientation != "horizontal" {
		t.Errorf("ProgressBarOrientation: got %q, want %q", cfg.ProgressBarOrientation, "horizontal")
	}
	if cfg.CwdMaxLength != 80 {
		t.Errorf("CwdMaxLength: got %d, want %d", cfg.CwdMaxLength, 80)
	}
	if cfg.CwdDepth != 5 {
		t.Errorf("CwdDepth: got %d, want %d", cfg.CwdDepth, 5)
	}
	if cfg.ShowTimeBars == nil || *cfg.ShowTimeBars != false {
		t.Errorf("ShowTimeBars: got %v, want false", cfg.ShowTimeBars)
	}
	if cfg.TimeBarDim == nil || *cfg.TimeBarDim != 0.5 {
		t.Errorf("TimeBarDim: got %v, want 0.5", cfg.TimeBarDim)
	}
	if cfg.Lines == nil {
		t.Fatal("Lines: got nil, want non-nil")
	}
	if len(cfg.Lines.Line1) != 3 {
		t.Errorf("Lines.Line1 length: got %d, want 3", len(cfg.Lines.Line1))
	}
	if len(cfg.Colors) != 1 {
		t.Errorf("Colors length: got %d, want 1", len(cfg.Colors))
	}
	if cfg.Colors[0].DarkMode == nil || !*cfg.Colors[0].DarkMode {
		t.Errorf("Colors[0].DarkMode: got %v, want true", cfg.Colors[0].DarkMode)
	}
}

func TestValidationClamping(t *testing.T) {
	tests := []struct {
		name  string
		toml  string
		check func(*Config) string
	}{
		{
			name: "progress_length too high gets clamped to default",
			toml: `progress_length = 100`,
			check: func(cfg *Config) string {
				if cfg.ProgressLength != 12 {
					return "expected 12, got " + itoa(cfg.ProgressLength)
				}
				return ""
			},
		},
		{
			name: "progress_length too low gets clamped to default",
			toml: `progress_length = 1`,
			check: func(cfg *Config) string {
				if cfg.ProgressLength != 12 {
					return "expected 12, got " + itoa(cfg.ProgressLength)
				}
				return ""
			},
		},
		{
			name: "progress_length at minimum boundary is valid",
			toml: `progress_length = 3`,
			check: func(cfg *Config) string {
				if cfg.ProgressLength != 3 {
					return "expected 3, got " + itoa(cfg.ProgressLength)
				}
				return ""
			},
		},
		{
			name: "progress_length at maximum boundary is valid",
			toml: `progress_length = 40`,
			check: func(cfg *Config) string {
				if cfg.ProgressLength != 40 {
					return "expected 40, got " + itoa(cfg.ProgressLength)
				}
				return ""
			},
		},
		{
			name: "invalid color_mode gets default",
			toml: `color_mode = "invalid"`,
			check: func(cfg *Config) string {
				if cfg.ColorMode != "auto" {
					return "expected auto, got " + cfg.ColorMode
				}
				return ""
			},
		},
		{
			name: "invalid partial_blocks gets default",
			toml: `partial_blocks = "maybe"`,
			check: func(cfg *Config) string {
				if cfg.PartialBlocks != "auto" {
					return "expected auto, got " + cfg.PartialBlocks
				}
				return ""
			},
		},
		{
			name: "invalid orientation gets default",
			toml: `progress_bar_orientation = "diagonal"`,
			check: func(cfg *Config) string {
				if cfg.ProgressBarOrientation != "vertical" {
					return "expected vertical, got " + cfg.ProgressBarOrientation
				}
				return ""
			},
		},
		{
			name: "cwd_max_length too low gets default",
			toml: `cwd_max_length = 5`,
			check: func(cfg *Config) string {
				if cfg.CwdMaxLength != 50 {
					return "expected 50, got " + itoa(cfg.CwdMaxLength)
				}
				return ""
			},
		},
		{
			name: "cwd_max_length too high gets default",
			toml: `cwd_max_length = 200`,
			check: func(cfg *Config) string {
				if cfg.CwdMaxLength != 50 {
					return "expected 50, got " + itoa(cfg.CwdMaxLength)
				}
				return ""
			},
		},
		{
			name: "cwd_depth too low gets default",
			toml: `cwd_depth = 0`,
			check: func(cfg *Config) string {
				if cfg.CwdDepth != 3 {
					return "expected 3, got " + itoa(cfg.CwdDepth)
				}
				return ""
			},
		},
		{
			name: "cwd_depth too high gets default",
			toml: `cwd_depth = 20`,
			check: func(cfg *Config) string {
				if cfg.CwdDepth != 3 {
					return "expected 3, got " + itoa(cfg.CwdDepth)
				}
				return ""
			},
		},
		{
			name: "time_bar_dim out of range gets default",
			toml: `time_bar_dim = 2.5`,
			check: func(cfg *Config) string {
				if cfg.TimeBarDim == nil || *cfg.TimeBarDim != 0.25 {
					return "expected 0.25"
				}
				return ""
			},
		},
		{
			name: "time_bar_dim negative gets default",
			toml: `time_bar_dim = -0.5`,
			check: func(cfg *Config) string {
				if cfg.TimeBarDim == nil || *cfg.TimeBarDim != 0.25 {
					return "expected 0.25"
				}
				return ""
			},
		},
		{
			name: "lines with missing line3 gets nil",
			toml: `
[lines]
line1 = ["elapsed"]
line2 = ["context_bar"]
`,
			check: func(cfg *Config) string {
				if cfg.Lines != nil {
					return "expected nil Lines when line3 is missing"
				}
				return ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.toml")
			if err := os.WriteFile(path, []byte(tt.toml), 0644); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if msg := tt.check(cfg); msg != "" {
				t.Error(msg)
			}
		})
	}
}

func TestLoadInvalidTOMLReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	// Write syntactically invalid TOML
	if err := os.WriteFile(path, []byte(`[[[broken`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return defaults
	if cfg.ProgressLength != 12 {
		t.Errorf("ProgressLength: got %d, want %d (default)", cfg.ProgressLength, 12)
	}
	if cfg.ColorMode != "auto" {
		t.Errorf("ColorMode: got %q, want %q (default)", cfg.ColorMode, "auto")
	}
}

func TestConfigPath(t *testing.T) {
	path := ConfigPath()
	if path == "" {
		t.Fatal("ConfigPath returned empty string")
	}
	// Should end with the expected suffix
	expected := filepath.Join(".config", "howmuchleft", "config.toml")
	if !contains(path, expected) {
		t.Errorf("ConfigPath %q does not contain %q", path, expected)
	}
}

func TestGetCaching(t *testing.T) {
	ResetCache()
	// Get should return a non-nil config even if file doesn't exist
	cfg1 := Get()
	if cfg1 == nil {
		t.Fatal("Get() returned nil")
	}
	cfg2 := Get()
	if cfg1 != cfg2 {
		t.Error("Get() returned different pointers, expected caching")
	}
	ResetCache()
}

// helpers

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && findSubstring(s, sub))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

