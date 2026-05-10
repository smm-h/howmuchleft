package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestProfileInstall_WritesSettings(t *testing.T) {
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := profileInstall(claudeDir); err != nil {
		t.Fatalf("profileInstall failed: %v", err)
	}

	// Verify settings.json was written
	settingsPath := filepath.Join(claudeDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("failed to parse settings.json: %v", err)
	}

	sl, ok := settings["statusLine"]
	if !ok {
		t.Fatal("expected statusLine key in settings.json")
	}

	slMap, ok := sl.(map[string]interface{})
	if !ok {
		t.Fatal("expected statusLine to be an object")
	}

	if slMap["type"] != "command" {
		t.Errorf("expected type=command, got %v", slMap["type"])
	}

	cmd, ok := slMap["command"].(string)
	if !ok {
		t.Fatal("expected command to be a string")
	}
	if cmd == "" {
		t.Error("expected non-empty command")
	}
	// Command should contain "howmuchleft"
	if !containsStr(cmd, "howmuchleft") {
		t.Errorf("expected command to contain 'howmuchleft', got %q", cmd)
	}

	padding, ok := slMap["padding"].(float64)
	if !ok || padding != 0 {
		t.Errorf("expected padding=0, got %v", slMap["padding"])
	}
}

func TestProfileInstall_AlreadyInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Install once
	if err := profileInstall(claudeDir); err != nil {
		t.Fatalf("first install failed: %v", err)
	}

	// Install again — should detect "already installed" and not error
	if err := profileInstall(claudeDir); err != nil {
		t.Fatalf("second install should not error: %v", err)
	}

	// Verify settings.json still has exactly one statusLine
	settingsPath := filepath.Join(claudeDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("failed to parse settings.json: %v", err)
	}

	sl, ok := settings["statusLine"]
	if !ok {
		t.Fatal("expected statusLine key")
	}
	slMap, ok := sl.(map[string]interface{})
	if !ok {
		t.Fatal("expected statusLine to be object")
	}
	cmd := slMap["command"].(string)
	if !containsStr(cmd, "howmuchleft") {
		t.Errorf("command should still contain 'howmuchleft', got %q", cmd)
	}
}

func TestProfileUninstall_RemovesStatusLine(t *testing.T) {
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Install first
	if err := profileInstall(claudeDir); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	// Uninstall
	if err := profileUninstall(claudeDir); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}

	// Verify statusLine is gone
	settingsPath := filepath.Join(claudeDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings.json: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("failed to parse settings.json: %v", err)
	}

	if _, ok := settings["statusLine"]; ok {
		t.Error("expected statusLine to be removed after uninstall")
	}
}

func TestProfileUninstall_SafetyCheck(t *testing.T) {
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a settings.json with a non-howmuchleft statusLine
	settings := map[string]interface{}{
		"statusLine": map[string]interface{}{
			"type":    "command",
			"command": "some-other-tool",
		},
	}
	data, _ := json.MarshalIndent(settings, "", "  ")
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Uninstall should not remove it
	if err := profileUninstall(claudeDir); err != nil {
		t.Fatalf("uninstall should not error: %v", err)
	}

	// Verify statusLine is still there
	readData, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(readData, &result); err != nil {
		t.Fatal(err)
	}

	sl, ok := result["statusLine"]
	if !ok {
		t.Error("expected statusLine to remain (safety check should prevent removal)")
	}
	slMap := sl.(map[string]interface{})
	if slMap["command"] != "some-other-tool" {
		t.Error("statusLine command should be unchanged")
	}
}

func TestProfileUninstall_NotInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write an empty settings.json
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	// Uninstall when not installed should not error
	if err := profileUninstall(claudeDir); err != nil {
		t.Fatalf("uninstall on empty settings should not error: %v", err)
	}
}

func TestProfileInstall_CreatesClaudeDir(t *testing.T) {
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, "nonexistent", ".claude")

	if err := profileInstall(claudeDir); err != nil {
		t.Fatalf("install to nonexistent dir failed: %v", err)
	}

	// Verify the directory and settings.json were created
	if _, err := os.Stat(filepath.Join(claudeDir, "settings.json")); err != nil {
		t.Errorf("expected settings.json to be created: %v", err)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
