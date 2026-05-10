package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStripJSONComments_LineComments(t *testing.T) {
	input := `{
  // This is a comment
  "key": "value"
}`
	got := StripJSONComments(input)
	if strings.Contains(got, "//") {
		t.Errorf("line comment not stripped: %q", got)
	}
	if !strings.Contains(got, `"key": "value"`) {
		t.Errorf("value lost after stripping: %q", got)
	}
}

func TestStripJSONComments_BlockComments(t *testing.T) {
	input := `{
  /* block comment */
  "key": "value"
}`
	got := StripJSONComments(input)
	if strings.Contains(got, "/*") || strings.Contains(got, "*/") {
		t.Errorf("block comment not stripped: %q", got)
	}
	if !strings.Contains(got, `"key": "value"`) {
		t.Errorf("value lost after stripping: %q", got)
	}
}

func TestStripJSONComments_TrailingCommas(t *testing.T) {
	input := `{
  "a": 1,
  "b": [1, 2, 3,],
}`
	got := StripJSONComments(input)
	if strings.Contains(got, ",]") || strings.Contains(got, ",\n}") || strings.Contains(got, ",\n]") {
		t.Errorf("trailing comma not stripped: %q", got)
	}
}

func TestStripJSONComments_StringsPreserved(t *testing.T) {
	input := `{
  "url": "http://example.com/path",
  "comment": "this has // slashes and /* stars */"
}`
	got := StripJSONComments(input)
	if !strings.Contains(got, "http://example.com/path") {
		t.Errorf("URL in string was corrupted: %q", got)
	}
	if !strings.Contains(got, "this has // slashes and /* stars */") {
		t.Errorf("comment chars inside string were stripped: %q", got)
	}
}

func TestStripJSONComments_MultilineBlock(t *testing.T) {
	input := `{
  /* multi
     line
     comment */
  "key": 42
}`
	got := StripJSONComments(input)
	if strings.Contains(got, "multi") || strings.Contains(got, "comment") {
		t.Errorf("multiline block comment not stripped: %q", got)
	}
	if !strings.Contains(got, `"key": 42`) {
		t.Errorf("value lost: %q", got)
	}
}

func TestConvertJSONToTOML_FullConfig(t *testing.T) {
	// Set up temp dirs to simulate ~/.config/
	tmpDir := t.TempDir()
	oldPath := filepath.Join(tmpDir, "howmuchleft.json")
	newDir := filepath.Join(tmpDir, "howmuchleft")
	newPath := filepath.Join(newDir, "config.toml")

	jsonContent := `{
  "progressLength": 15,
  // Color mode
  "colorMode": "truecolor",
  "partialBlocks": "auto",
  "progressBarOrientation": "vertical",
  "showTimeBars": true,
  "timeBarDim": 0.3,
  "cwdMaxLength": 60,
  "cwdDepth": 4,
  "lines": [
    ["context", "elapsed", "profile"],
    ["usage5h", "branch"],
    ["usageWeekly", "cwd"]
  ],
  "colors": [
    {
      "dark-mode": true,
      "true-color": true,
      "bg": [48, 48, 48],
      "gradient": [[0,215,0], [255,0,0]]
    },
    {
      "dark-mode": false,
      "true-color": false,
      "bg": 252,
      "gradient": [46, 82, 196],
    }
  ],
}`
	if err := os.WriteFile(oldPath, []byte(jsonContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Override paths for test
	err := convertJSONToTOMLPaths(oldPath, newPath)
	if err != nil {
		t.Fatalf("ConvertJSONToTOML failed: %v", err)
	}

	// Check new file was created
	tomlData, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("new TOML file not created: %v", err)
	}
	toml := string(tomlData)

	// Check header comment
	if !strings.Contains(toml, "# Migrated from howmuchleft.json") {
		t.Error("missing migration header comment")
	}

	// Check scalar fields
	assertContains(t, toml, `color_mode = "truecolor"`)
	assertContains(t, toml, `progress_length = 15`)
	assertContains(t, toml, `partial_blocks = "auto"`)
	assertContains(t, toml, `progress_bar_orientation = "vertical"`)
	assertContains(t, toml, `show_time_bars = true`)
	assertContains(t, toml, `time_bar_dim = 0.3`)
	assertContains(t, toml, `cwd_max_length = 60`)
	assertContains(t, toml, `cwd_depth = 4`)

	// Check lines table
	assertContains(t, toml, `[lines]`)
	assertContains(t, toml, `line1 = ["context", "elapsed", "profile"]`)
	assertContains(t, toml, `line2 = ["usage5h", "branch"]`)
	assertContains(t, toml, `line3 = ["usageWeekly", "cwd"]`)

	// Check colors array-of-tables
	if strings.Count(toml, "[[colors]]") != 2 {
		t.Errorf("expected 2 [[colors]] entries, got %d", strings.Count(toml, "[[colors]]"))
	}
	assertContains(t, toml, `dark_mode = true`)
	assertContains(t, toml, `true_color = true`)
	assertContains(t, toml, `bg = [48, 48, 48]`)
	assertContains(t, toml, `gradient = [[0, 215, 0], [255, 0, 0]]`)
	assertContains(t, toml, `dark_mode = false`)
	assertContains(t, toml, `true_color = false`)
	assertContains(t, toml, `bg = 252`)
	assertContains(t, toml, `gradient = [46, 82, 196]`)

	// Check backup was created
	bakPath := oldPath + ".bak"
	if !fileExists(bakPath) {
		t.Error("backup file not created")
	}

	// Check old file was removed
	if fileExists(oldPath) {
		t.Error("old file still exists after migration")
	}
}

func TestConvertJSONToTOML_ExistingTOMLPreventsConversion(t *testing.T) {
	tmpDir := t.TempDir()
	oldPath := filepath.Join(tmpDir, "howmuchleft.json")
	newDir := filepath.Join(tmpDir, "howmuchleft")
	newPath := filepath.Join(newDir, "config.toml")

	// Create both old and new
	if err := os.WriteFile(oldPath, []byte(`{"colorMode": "256"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte(`color_mode = "truecolor"`), 0644); err != nil {
		t.Fatal(err)
	}

	err := convertJSONToTOMLPaths(oldPath, newPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// New file should be unchanged
	data, _ := os.ReadFile(newPath)
	if string(data) != `color_mode = "truecolor"` {
		t.Errorf("existing TOML was overwritten: %q", string(data))
	}

	// Old file should still exist (not renamed)
	if !fileExists(oldPath) {
		t.Error("old JSON was renamed despite existing TOML")
	}
}

func TestConvertJSONToTOML_NeitherExists(t *testing.T) {
	tmpDir := t.TempDir()
	oldPath := filepath.Join(tmpDir, "howmuchleft.json")
	newPath := filepath.Join(tmpDir, "howmuchleft", "config.toml")

	err := convertJSONToTOMLPaths(oldPath, newPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Neither file should be created
	if fileExists(newPath) {
		t.Error("TOML created from nothing")
	}
}

func TestConvertJSONToTOML_BackupCreation(t *testing.T) {
	tmpDir := t.TempDir()
	oldPath := filepath.Join(tmpDir, "howmuchleft.json")
	newPath := filepath.Join(tmpDir, "howmuchleft", "config.toml")

	originalContent := `{"progressLength": 20}`
	if err := os.WriteFile(oldPath, []byte(originalContent), 0644); err != nil {
		t.Fatal(err)
	}

	err := convertJSONToTOMLPaths(oldPath, newPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bakPath := oldPath + ".bak"
	bakData, err := os.ReadFile(bakPath)
	if err != nil {
		t.Fatalf("backup not readable: %v", err)
	}
	if string(bakData) != originalContent {
		t.Errorf("backup content mismatch: got %q, want %q", string(bakData), originalContent)
	}
}

func TestConvertJSONToTOML_MinimalConfig(t *testing.T) {
	tmpDir := t.TempDir()
	oldPath := filepath.Join(tmpDir, "howmuchleft.json")
	newPath := filepath.Join(tmpDir, "howmuchleft", "config.toml")

	// Minimal config with just one field
	if err := os.WriteFile(oldPath, []byte(`{"cwdDepth": 5}`), 0644); err != nil {
		t.Fatal(err)
	}

	err := convertJSONToTOMLPaths(oldPath, newPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatal(err)
	}
	toml := string(data)
	assertContains(t, toml, `cwd_depth = 5`)
	// Should NOT contain fields that weren't in the JSON
	if strings.Contains(toml, "color_mode") {
		t.Error("TOML contains field not present in source JSON")
	}
}

func TestConvertJSONToTOML_BooleanPartialBlocks(t *testing.T) {
	tmpDir := t.TempDir()
	oldPath := filepath.Join(tmpDir, "howmuchleft.json")
	newPath := filepath.Join(tmpDir, "howmuchleft", "config.toml")

	// partialBlocks as boolean true (not string)
	if err := os.WriteFile(oldPath, []byte(`{"partialBlocks": true}`), 0644); err != nil {
		t.Fatal(err)
	}

	err := convertJSONToTOMLPaths(oldPath, newPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatal(err)
	}
	toml := string(data)
	// In TOML, partialBlocks is stored as a string ("true"/"false"/"auto"),
	// but JSON booleans should be converted to the string representation
	// since the Go config uses string type.
	// Actually, the JSON can have either boolean or string for partialBlocks.
	// We preserve the original type faithfully.
	assertContains(t, toml, `partial_blocks = true`)
}

// convertJSONToTOMLPaths is a testable version that accepts explicit paths.
func convertJSONToTOMLPaths(oldPath, newPath string) error {
	oldExists := fileExists(oldPath)
	newExists := fileExists(newPath)

	if !oldExists || newExists {
		return nil
	}

	raw, err := os.ReadFile(oldPath)
	if err != nil {
		return fmt.Errorf("reading old config: %w", err)
	}

	stripped := StripJSONComments(string(raw))

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(stripped), &data); err != nil {
		return fmt.Errorf("parsing JSON config: %w", err)
	}

	toml := buildTOML(data)

	newDir := filepath.Dir(newPath)
	if err := os.MkdirAll(newDir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	if err := os.WriteFile(newPath, []byte(toml), 0644); err != nil {
		return fmt.Errorf("writing TOML config: %w", err)
	}

	bakPath := oldPath + ".bak"
	if err := os.Rename(oldPath, bakPath); err != nil {
		return fmt.Errorf("backing up old config: %w", err)
	}

	return nil
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected output to contain %q, got:\n%s", substr, s)
	}
}
