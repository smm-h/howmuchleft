package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tomledit "github.com/smm-h/go-toml-edit"

	"github.com/smm-h/howmuchleft/internal/oauth"
)

// resolveClaudeDir resolves the Claude configuration directory.
// Priority: positional arg > CLAUDE_CONFIG_DIR env > ~/.claude
func resolveClaudeDir(args []string) string {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			return resolveTildePath(arg)
		}
	}
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return resolveTildePath(dir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".claude"
	}
	return filepath.Join(home, ".claude")
}

// resolveTildePath expands ~ to the user's home directory.
func resolveTildePath(p string) string {
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, p[1:])
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

// replaceHomeWithTilde replaces the home directory prefix with ~ in a path.
func replaceHomeWithTilde(p string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

// profileInstall installs howmuchleft into a Claude Code profile's settings.json.
func profileInstall(claudeDir string) error {
	settingsPath := filepath.Join(claudeDir, "settings.json")

	settings, err := readSettingsJSON(settingsPath)
	if err != nil {
		return fmt.Errorf("reading settings.json: %w", err)
	}

	command := "howmuchleft " + replaceHomeWithTilde(claudeDir)

	// Check if statusLine already exists
	if sl, ok := settings["statusLine"]; ok && sl != nil {
		fmt.Printf("Current statusLine in %s:\n", settingsPath)
		slJSON, _ := json.MarshalIndent(sl, "  ", "  ")
		fmt.Printf("  %s\n\n", slJSON)

		if slMap, ok := sl.(map[string]interface{}); ok {
			if cmd, ok := slMap["command"].(string); ok && strings.Contains(cmd, "howmuchleft") {
				fmt.Println("howmuchleft is already installed. To update:")
				fmt.Println("  howmuchleft profile uninstall && howmuchleft profile install")
				return nil
			}
		}

		fmt.Println("A statusLine is already configured. Overwriting.")
	}

	settings["statusLine"] = map[string]interface{}{
		"type":    "command",
		"command": command,
		"padding": 0,
	}

	if err := writeSettingsJSON(settingsPath, settings); err != nil {
		return fmt.Errorf("writing settings.json: %w", err)
	}

	if err := registerProfile(claudeDir); err != nil {
		fmt.Fprintf(os.Stderr, "howmuchleft: warning: failed to register profile: %v\n", err)
	}

	slJSON, _ := json.MarshalIndent(settings["statusLine"], "  ", "  ")
	fmt.Printf("Installed. Added to %s:\n", settingsPath)
	fmt.Printf("  %s\n\n", slJSON)
	fmt.Println("Restart Claude Code to see the statusline.")
	return nil
}

// profileUninstall removes howmuchleft from a Claude Code profile's settings.json.
func profileUninstall(claudeDir string) error {
	settingsPath := filepath.Join(claudeDir, "settings.json")

	settings, err := readSettingsJSON(settingsPath)
	if err != nil {
		return fmt.Errorf("reading settings.json: %w", err)
	}

	sl, ok := settings["statusLine"]
	if !ok || sl == nil {
		fmt.Printf("No statusLine configured in %s.\n", settingsPath)
		return nil
	}

	// Safety check: refuse to remove a non-howmuchleft statusLine
	if slMap, ok := sl.(map[string]interface{}); ok {
		if cmd, ok := slMap["command"].(string); ok && !strings.Contains(cmd, "howmuchleft") {
			fmt.Printf("statusLine in %s is not howmuchleft:\n", settingsPath)
			slJSON, _ := json.MarshalIndent(sl, "  ", "  ")
			fmt.Printf("  %s\n", slJSON)
			fmt.Println("Not removing. Edit settings.json manually to remove.")
			return nil
		}
	}

	delete(settings, "statusLine")

	if err := writeSettingsJSON(settingsPath, settings); err != nil {
		return fmt.Errorf("writing settings.json: %w", err)
	}

	if err := unregisterProfile(claudeDir); err != nil {
		fmt.Fprintf(os.Stderr, "howmuchleft: warning: failed to unregister profile: %v\n", err)
	}

	fmt.Printf("Removed statusLine from %s.\n", settingsPath)
	fmt.Println("Restart Claude Code to apply.")
	return nil
}

// readSettingsJSON reads and parses a Claude Code settings.json file.
// Returns an empty map if the file doesn't exist or can't be parsed.
func readSettingsJSON(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]interface{}{}, nil
		}
		return nil, err
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return settings, nil
}

// writeSettingsJSON writes settings to a Claude Code settings.json file.
// Creates parent directories if needed. Uses atomic write (tmpfile + rename).
func writeSettingsJSON(path string, settings map[string]interface{}) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return oauth.WriteFileAtomic(path, append(data, '\n'))
}

// registerProfile adds a claude directory to the profiles list in config.toml.
// Uses go-toml-edit to preserve comments and formatting.
func registerProfile(claudeDir string) error {
	absDir, err := filepath.Abs(claudeDir)
	if err != nil {
		absDir = claudeDir
	}

	configPath := configFilePath()

	data, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var doc *tomledit.DocumentNode
	if len(data) > 0 {
		doc, err = tomledit.Parse(data)
		if err != nil {
			return fmt.Errorf("parsing config.toml: %w", err)
		}
	} else {
		doc, _ = tomledit.Parse([]byte{})
	}

	// Read existing profiles
	existing := getProfilesFromDoc(doc)

	// Check if already registered
	for _, p := range existing {
		if p == absDir {
			return nil
		}
	}

	// Add the new profile
	existing = append(existing, absDir)

	// Set the profiles value
	if err := doc.SetCreate("profiles", existing); err != nil {
		return fmt.Errorf("setting profiles: %w", err)
	}

	// Write back atomically
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}
	return oauth.WriteFileAtomic(configPath, doc.Format())
}

// unregisterProfile removes a claude directory from the profiles list in config.toml.
func unregisterProfile(claudeDir string) error {
	absDir, err := filepath.Abs(claudeDir)
	if err != nil {
		absDir = claudeDir
	}

	configPath := configFilePath()

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	doc, err := tomledit.Parse(data)
	if err != nil {
		return fmt.Errorf("parsing config.toml: %w", err)
	}

	existing := getProfilesFromDoc(doc)

	// Filter out the target
	var filtered []string
	for _, p := range existing {
		if p != absDir {
			filtered = append(filtered, p)
		}
	}

	if len(filtered) == len(existing) {
		return nil // not found, nothing to do
	}

	if len(filtered) == 0 {
		// Remove the key entirely
		_ = doc.Delete("profiles")
	} else {
		if err := doc.Set("profiles", filtered); err != nil {
			return fmt.Errorf("setting profiles: %w", err)
		}
	}

	return oauth.WriteFileAtomic(configPath, doc.Format())
}

// getProfilesFromDoc extracts the profiles string array from a parsed TOML doc.
func getProfilesFromDoc(doc *tomledit.DocumentNode) []string {
	node := doc.Get("profiles")
	if node == nil {
		return nil
	}

	val := node.Value()
	arr, ok := val.([]interface{})
	if !ok {
		return nil
	}

	var result []string
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// configFilePath returns the path to howmuchleft's config.toml.
func configFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".config", "howmuchleft", "config.toml")
}
