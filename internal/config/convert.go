package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// OldConfigPath returns the path to the legacy JSON config file.
func OldConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	return filepath.Join(home, ".config", "howmuchleft.json")
}

// ConvertJSONToTOML detects an old JSON config and converts it to TOML.
// If old exists and new doesn't: converts, writes new, renames old to .bak.
// If both exist or neither exists: does nothing.
func ConvertJSONToTOML() error {
	oldPath := OldConfigPath()
	newPath := ConfigPath()

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

	// Create the new config directory
	newDir := filepath.Dir(newPath)
	if err := os.MkdirAll(newDir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	if err := os.WriteFile(newPath, []byte(toml), 0644); err != nil {
		return fmt.Errorf("writing TOML config: %w", err)
	}

	// Rename old file to .bak
	bakPath := oldPath + ".bak"
	if err := os.Rename(oldPath, bakPath); err != nil {
		return fmt.Errorf("backing up old config: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Migrated config from JSON to TOML\n")
	return nil
}

// StripJSONComments removes // line comments, /* */ block comments, and
// trailing commas before } or ] from JSONC text. Ported from the Node.js
// implementation in lib/statusline.js.
func StripJSONComments(text string) string {
	var result strings.Builder
	result.Grow(len(text))

	inString := false
	escape := false

	for i := 0; i < len(text); i++ {
		ch := text[i]

		if escape {
			result.WriteByte(ch)
			escape = false
			continue
		}

		if inString {
			if ch == '\\' {
				escape = true
			} else if ch == '"' {
				inString = false
			}
			result.WriteByte(ch)
			continue
		}

		if ch == '"' {
			inString = true
			result.WriteByte(ch)
			continue
		}

		// Line comment: //
		if ch == '/' && i+1 < len(text) && text[i+1] == '/' {
			for i < len(text) && text[i] != '\n' {
				i++
			}
			result.WriteByte('\n')
			continue
		}

		// Block comment: /* ... */
		if ch == '/' && i+1 < len(text) && text[i+1] == '*' {
			i += 2
			for i < len(text) && !(text[i] == '*' && i+1 < len(text) && text[i+1] == '/') {
				i++
			}
			if i < len(text) {
				i++ // skip past closing /
			}
			continue
		}

		result.WriteByte(ch)
	}

	// Strip trailing commas before } or ]
	return stripTrailingCommas(result.String())
}

// stripTrailingCommas removes commas that immediately precede (with optional
// whitespace) a closing } or ].
func stripTrailingCommas(s string) string {
	// Equivalent to JS: result.replace(/,\s*([}\]])/g, '$1')
	var out strings.Builder
	out.Grow(len(s))

	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			// Look ahead past whitespace for } or ]
			j := i + 1
			for j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n' || s[j] == '\r') {
				j++
			}
			if j < len(s) && (s[j] == '}' || s[j] == ']') {
				// Skip the comma, keep whitespace and closer
				continue
			}
		}
		out.WriteByte(s[i])
	}
	return out.String()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// buildTOML converts a parsed JSON config map to a TOML string.
func buildTOML(data map[string]interface{}) string {
	var b strings.Builder
	b.WriteString("# Migrated from howmuchleft.json\n\n")

	// Simple scalar fields (order matches config.example.json)
	type fieldMapping struct {
		jsonKey string
		tomlKey string
	}

	scalarFields := []fieldMapping{
		{"colorMode", "color_mode"},
		{"progressLength", "progress_length"},
		{"partialBlocks", "partial_blocks"},
		{"progressBarOrientation", "progress_bar_orientation"},
		{"showTimeBars", "show_time_bars"},
		{"timeBarDim", "time_bar_dim"},
		{"cwdMaxLength", "cwd_max_length"},
		{"cwdDepth", "cwd_depth"},
	}

	for _, f := range scalarFields {
		val, ok := data[f.jsonKey]
		if !ok {
			continue
		}
		b.WriteString(formatTOMLValue(f.tomlKey, val))
	}

	// profiles
	if profiles, ok := data["profiles"]; ok {
		if arr, ok := profiles.([]interface{}); ok && len(arr) > 0 {
			b.WriteString(formatTOMLValue("profiles", arr))
		}
	}

	// lines -> [lines] table
	if lines, ok := data["lines"]; ok {
		if arr, ok := lines.([]interface{}); ok && len(arr) == 3 {
			b.WriteString("\n[lines]\n")
			lineNames := []string{"line1", "line2", "line3"}
			for i, lineName := range lineNames {
				if lineArr, ok := arr[i].([]interface{}); ok {
					b.WriteString(formatTOMLValue(lineName, lineArr))
				}
			}
		}
	}

	// colors -> [[colors]] array-of-tables
	if colors, ok := data["colors"]; ok {
		if arr, ok := colors.([]interface{}); ok && len(arr) > 0 {
			b.WriteString("\n")
			for _, entry := range arr {
				obj, ok := entry.(map[string]interface{})
				if !ok {
					continue
				}
				b.WriteString("[[colors]]\n")
				// dark_mode
				if dm, ok := obj["dark-mode"]; ok {
					b.WriteString(formatTOMLValue("dark_mode", dm))
				}
				// true_color
				if tc, ok := obj["true-color"]; ok {
					b.WriteString(formatTOMLValue("true_color", tc))
				}
				// bg
				if bg, ok := obj["bg"]; ok {
					b.WriteString(fmt.Sprintf("bg = %s\n", formatTOMLInlineValue(bg)))
				}
				// gradient
				if gradient, ok := obj["gradient"]; ok {
					b.WriteString(fmt.Sprintf("gradient = %s\n", formatTOMLInlineValue(gradient)))
				}
				b.WriteString("\n")
			}
		}
	}

	return b.String()
}

// formatTOMLValue formats a key = value line for TOML.
func formatTOMLValue(key string, val interface{}) string {
	return fmt.Sprintf("%s = %s\n", key, formatTOMLInlineValue(val))
}

// formatTOMLInlineValue converts a Go value to its TOML inline representation.
func formatTOMLInlineValue(val interface{}) string {
	switch v := val.(type) {
	case string:
		return fmt.Sprintf("%q", v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case float64:
		// JSON numbers are float64. If it's an integer value, format without decimal.
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%g", v)
	case []interface{}:
		return formatTOMLArray(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// formatTOMLArray formats a Go slice as a TOML inline array.
func formatTOMLArray(arr []interface{}) string {
	parts := make([]string, len(arr))
	for i, elem := range arr {
		parts[i] = formatTOMLInlineValue(elem)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
