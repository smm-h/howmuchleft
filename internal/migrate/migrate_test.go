package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestRun(t *testing.T) {
	// Read the actual migration file from the repo.
	migrationData, err := os.ReadFile(filepath.Join(repoRoot(t), "migrations", "next", "initial-config.toml"))
	if err != nil {
		t.Fatalf("failed to read migration file: %v", err)
	}

	// Build an in-memory FS matching the embed structure.
	fsys := fstest.MapFS{
		"migrations/next/initial-config.toml": &fstest.MapFile{Data: migrationData},
	}

	// Create a temp config directory.
	configDir := t.TempDir()

	result, err := Run(fsys, configDir)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if result.Applied != 1 {
		t.Errorf("expected 1 migration applied, got %d", result.Applied)
	}

	if result.ToVersion.String() != "0.0.1" {
		t.Errorf("expected version 0.0.1, got %s", result.ToVersion)
	}

	// Read the resulting config file and verify defaults.
	configPath := filepath.Join(configDir, "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}
	content := string(data)

	// Verify _schema_version is set.
	assertContains(t, content, `_schema_version = "0.0.1"`)

	// Verify default values.
	assertContains(t, content, `color_mode = "auto"`)
	assertContains(t, content, `progress_length = 12`)
	assertContains(t, content, `partial_blocks = "auto"`)
	assertContains(t, content, `progress_bar_orientation = "vertical"`)
	assertContains(t, content, `cwd_max_length = 50`)
	assertContains(t, content, `cwd_depth = 3`)
	assertContains(t, content, `show_time_bars = true`)
	assertContains(t, content, `time_bar_dim = 0.25`)

	// Verify lines collection.
	assertContains(t, content, `line1`)
	assertContains(t, content, `line2`)
	assertContains(t, content, `line3`)
}

func TestRunDryRun(t *testing.T) {
	migrationData, err := os.ReadFile(filepath.Join(repoRoot(t), "migrations", "next", "initial-config.toml"))
	if err != nil {
		t.Fatalf("failed to read migration file: %v", err)
	}

	fsys := fstest.MapFS{
		"migrations/next/initial-config.toml": &fstest.MapFile{Data: migrationData},
	}

	configDir := t.TempDir()

	result, err := RunDryRun(fsys, configDir)
	if err != nil {
		t.Fatalf("RunDryRun failed: %v", err)
	}

	if result.Applied != 1 {
		t.Errorf("expected 1 migration applied in dry-run, got %d", result.Applied)
	}

	// In dry-run mode, the config file should still have the initial content.
	configPath := filepath.Join(configDir, "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}
	content := string(data)

	// Should still be the initial empty config (ensureConfigFile writes this).
	if !strings.Contains(content, `_schema_version = "0.0.0"`) {
		t.Errorf("dry-run should not modify the config file, got:\n%s", content)
	}
}

func TestRunIdempotent(t *testing.T) {
	migrationData, err := os.ReadFile(filepath.Join(repoRoot(t), "migrations", "next", "initial-config.toml"))
	if err != nil {
		t.Fatalf("failed to read migration file: %v", err)
	}

	fsys := fstest.MapFS{
		"migrations/next/initial-config.toml": &fstest.MapFile{Data: migrationData},
	}

	configDir := t.TempDir()

	// First run.
	_, err = Run(fsys, configDir)
	if err != nil {
		t.Fatalf("first Run failed: %v", err)
	}

	// Second run should be a no-op.
	result, err := Run(fsys, configDir)
	if err != nil {
		t.Fatalf("second Run failed: %v", err)
	}

	if result.Applied != 0 {
		t.Errorf("expected 0 migrations on second run, got %d", result.Applied)
	}
}

func assertContains(t *testing.T, content, substr string) {
	t.Helper()
	if !strings.Contains(content, substr) {
		t.Errorf("expected config to contain %q, got:\n%s", substr, content)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	// Walk up from the test file location to find go.mod.
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (no go.mod found)")
		}
		dir = parent
	}
}
