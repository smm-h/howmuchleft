// Package migrate brings the on-disk howmuchleft config up to the shape the
// current binary expects: it creates ~/.config/howmuchleft/config.toml when it
// is missing, and fills in any key a newer version introduced. Existing values,
// comments and formatting are never touched, and unknown keys are left alone.
//
// The defaults it writes come from internal/config, which is the single source
// of truth -- a new setting becomes part of the seeded file the moment it is
// added to config.Default() or config.DefaultLines().
package migrate

import (
	"fmt"
	"os"
	"path/filepath"

	tomledit "github.com/smm-h/go-toml-edit"
	"github.com/smm-h/howmuchleft/internal/config"
)

// Result reports what EnsureDefaults did to the config file.
type Result struct {
	// Created is true when the config file did not exist and was written fresh.
	Created bool
	// Added lists the config paths that were absent and have been filled in.
	Added []string
}

// defaultEntry is one config path and the default value written when the path
// is absent from the user's file.
type defaultEntry struct {
	path  string
	value any
}

// defaultEntries returns every config path seeded into a config file, in the
// order they are written, derived from the in-code defaults.
func defaultEntries() []defaultEntry {
	d := config.Default()
	lines := config.DefaultLines()
	return []defaultEntry{
		{"color_mode", d.ColorMode},
		{"progress_length", d.ProgressLength},
		{"partial_blocks", d.PartialBlocks},
		{"progress_bar_orientation", d.ProgressBarOrientation},
		{"cwd_max_length", d.CwdMaxLength},
		{"cwd_depth", d.CwdDepth},
		{"show_time_bars", *d.ShowTimeBars},
		{"time_bar_dim", *d.TimeBarDim},
		{"lines.line1", lines.Line1},
		{"lines.line2", lines.Line2},
		{"lines.line3", lines.Line3},
	}
}

// EnsureDefaults creates or completes the config file in configDir. It is
// idempotent: when nothing is missing, the file is not rewritten at all.
func EnsureDefaults(configDir string) (*Result, error) {
	configPath := filepath.Join(configDir, config.ConfigFile)

	data, err := os.ReadFile(configPath)
	created := false
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read %s: %w", configPath, err)
		}
		created = true
		data = nil
	}

	doc, err := tomledit.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", configPath, err)
	}

	result := &Result{Created: created}
	for _, entry := range defaultEntries() {
		if doc.Has(entry.path) {
			continue
		}
		if err := doc.SetCreate(entry.path, entry.value); err != nil {
			return nil, fmt.Errorf("failed to set %s: %w", entry.path, err)
		}
		result.Added = append(result.Added, entry.path)
	}

	if !created && len(result.Added) == 0 {
		return result, nil
	}

	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create %s: %w", configDir, err)
	}
	if err := os.WriteFile(configPath, doc.Bytes(), 0o644); err != nil {
		return nil, fmt.Errorf("failed to write %s: %w", configPath, err)
	}
	return result, nil
}
