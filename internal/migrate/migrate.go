package migrate

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/smm-h/migrable/config"
	"github.com/smm-h/migrable/engine"
)

// migrationsFS holds the embedded migrations filesystem, set by main via SetFS.
var migrationsFS fs.FS

// SetFS stores the embedded migrations FS for use by RunEmbedded.
func SetFS(fsys fs.FS) {
	migrationsFS = fsys
}

// RunEmbedded applies pending migrations using the FS set via SetFS.
func RunEmbedded(configDir string) (*engine.MigrateResult, error) {
	if migrationsFS == nil {
		return nil, fmt.Errorf("migrations FS not set; call SetFS first")
	}
	return Run(migrationsFS, configDir)
}

// Run applies all pending embedded migrations to the config file at configDir.
// It writes embedded migrations to a temp directory, merges any next/ staging
// files into a versioned migration, then runs engine.Migrate.
func Run(migrations fs.FS, configDir string) (*engine.MigrateResult, error) {
	return run(migrations, configDir, false)
}

// RunDryRun applies all pending embedded migrations in dry-run mode (no writes).
func RunDryRun(migrations fs.FS, configDir string) (*engine.MigrateResult, error) {
	return run(migrations, configDir, true)
}

func run(migrations fs.FS, configDir string, dryRun bool) (*engine.MigrateResult, error) {
	configPath := filepath.Join(configDir, "config.toml")

	// Ensure config file exists (migrable needs something to operate on).
	if err := ensureConfigFile(configPath); err != nil {
		return nil, fmt.Errorf("failed to ensure config file: %w", err)
	}

	// Write embedded migrations to a temp directory.
	tmpDir, err := os.MkdirTemp("", "howmuchleft-migrations-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := writeEmbedded(migrations, tmpDir); err != nil {
		return nil, fmt.Errorf("failed to write embedded migrations: %w", err)
	}

	migrationsDir := filepath.Join(tmpDir, "migrations")

	// Merge any next/ staging files into a versioned migration.
	// Use "0.0.1" as the merge version for next/ files (the lowest possible
	// version above 0.0.0, ensuring it's always applied to fresh configs).
	nextDir := filepath.Join(migrationsDir, "next")
	if entries, err := os.ReadDir(nextDir); err == nil && hasTomlFiles(entries) {
		if _, err := engine.Merge(migrationsDir, "0.0.1"); err != nil {
			return nil, fmt.Errorf("failed to merge next/ migrations: %w", err)
		}
	}

	cfg := &config.Config{
		MigrationsDir: "migrations",
		Files:         map[string]string{"config": configPath},
		BaseDir:       tmpDir,
	}

	return engine.Migrate(cfg, dryRun)
}

// ensureConfigFile creates the config file with a minimal schema version if it
// doesn't exist yet.
func ensureConfigFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	return os.WriteFile(path, []byte("_schema_version = \"0.0.0\"\n"), 0o644)
}

// writeEmbedded writes all files from the embedded FS to the target directory,
// preserving the directory structure.
func writeEmbedded(fsys fs.FS, targetDir string) error {
	return fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		targetPath := filepath.Join(targetDir, path)

		if d.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}

		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("failed to read embedded file %s: %w", path, err)
		}

		return os.WriteFile(targetPath, data, 0o644)
	})
}

// hasTomlFiles returns true if any entry in the list is a .toml file.
func hasTomlFiles(entries []os.DirEntry) bool {
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".toml" {
			return true
		}
	}
	return false
}
