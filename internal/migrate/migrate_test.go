package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smm-h/howmuchleft/internal/config"
	"github.com/smm-h/stricttest/go/hygiene"
)

func TestEnsureDefaultsCreatesConfig(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	configDir := t.TempDir()

	result, err := EnsureDefaults(configDir)
	if err != nil {
		t.Fatalf("EnsureDefaults failed: %v", err)
	}
	if !result.Created {
		t.Error("expected Created to be true for a fresh config directory")
	}
	if len(result.Added) == 0 {
		t.Error("expected the created config to have keys added")
	}

	configPath := filepath.Join(configDir, "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}
	content := string(data)

	assertContains(t, content, `color_mode = "auto"`)
	assertContains(t, content, `progress_length = 12`)
	assertContains(t, content, `partial_blocks = "auto"`)
	assertContains(t, content, `progress_bar_orientation = "vertical"`)
	assertContains(t, content, `cwd_max_length = 50`)
	assertContains(t, content, `cwd_depth = 3`)
	assertContains(t, content, `show_time_bars = true`)
	assertContains(t, content, `time_bar_dim = 0.25`)
	assertContains(t, content, `line1`)
	assertContains(t, content, `line2`)
	assertContains(t, content, `line3`)

	// The written file must round-trip into exactly the in-code defaults.
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("failed to load written config: %v", err)
	}
	want := config.Default()
	if cfg.ColorMode != want.ColorMode || cfg.ProgressLength != want.ProgressLength ||
		cfg.PartialBlocks != want.PartialBlocks || cfg.ProgressBarOrientation != want.ProgressBarOrientation ||
		cfg.CwdMaxLength != want.CwdMaxLength || cfg.CwdDepth != want.CwdDepth ||
		*cfg.ShowTimeBars != *want.ShowTimeBars || *cfg.TimeBarDim != *want.TimeBarDim {
		t.Errorf("written config does not match config.Default(): got %+v", cfg)
	}
	if cfg.Lines == nil {
		t.Fatal("written config has no [lines] table")
	}
	wantLines := config.DefaultLines()
	if strings.Join(cfg.Lines.Line3, ",") != strings.Join(wantLines.Line3, ",") {
		t.Errorf("line3 = %v, want %v", cfg.Lines.Line3, wantLines.Line3)
	}
}

func TestEnsureDefaultsIdempotent(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.toml")

	if _, err := EnsureDefaults(configDir); err != nil {
		t.Fatalf("first EnsureDefaults failed: %v", err)
	}
	first, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	result, err := EnsureDefaults(configDir)
	if err != nil {
		t.Fatalf("second EnsureDefaults failed: %v", err)
	}
	if result.Created {
		t.Error("second run reported Created")
	}
	if len(result.Added) != 0 {
		t.Errorf("second run added keys: %v", result.Added)
	}

	second, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to re-read config file: %v", err)
	}
	if string(first) != string(second) {
		t.Errorf("config changed on second run:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

func TestEnsureDefaultsPreservesUserContent(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.toml")

	// A config written by an older version: a legacy schema marker, one
	// customized value, and a comment.
	existing := "_schema_version = \"0.0.1\"\n# my terminal is 256-color\ncolor_mode = \"256\"\nprogress_length = 30\n"
	if err := os.WriteFile(configPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("failed to seed config: %v", err)
	}

	result, err := EnsureDefaults(configDir)
	if err != nil {
		t.Fatalf("EnsureDefaults failed: %v", err)
	}
	if result.Created {
		t.Error("expected Created to be false for an existing config")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}
	content := string(data)

	// User values, comments and unknown legacy keys survive untouched.
	assertContains(t, content, `color_mode = "256"`)
	assertContains(t, content, `progress_length = 30`)
	assertContains(t, content, `# my terminal is 256-color`)
	assertContains(t, content, `_schema_version = "0.0.1"`)

	// Missing keys are filled in.
	assertContains(t, content, `cwd_depth = 3`)
	assertContains(t, content, `show_time_bars = true`)
	assertContains(t, content, `line1`)

	for _, key := range []string{"color_mode", "progress_length"} {
		for _, added := range result.Added {
			if added == key {
				t.Errorf("EnsureDefaults reported adding an already-present key %q", key)
			}
		}
	}
}

func TestEnsureDefaultsRejectsUnparseableConfig(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.toml")
	broken := "this is not = = toml\n"
	if err := os.WriteFile(configPath, []byte(broken), 0o644); err != nil {
		t.Fatalf("failed to seed config: %v", err)
	}

	if _, err := EnsureDefaults(configDir); err == nil {
		t.Fatal("expected an error for an unparseable config, got nil")
	}

	// The broken file must be left exactly as the user wrote it.
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}
	if string(data) != broken {
		t.Errorf("unparseable config was modified: %q", data)
	}
}

func assertContains(t *testing.T, content, substr string) {
	t.Helper()
	if !strings.Contains(content, substr) {
		t.Errorf("expected config to contain %q, got:\n%s", substr, content)
	}
}
