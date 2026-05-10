package platform

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// GetSessionElapsed returns the session elapsed time in milliseconds by finding
// the Claude Code session file for an ancestor PID. Returns nil if not found.
func GetSessionElapsed(claudeDir string) *int64 {
	pid := os.Getppid()

	maxLevels := 5
	if runtime.GOOS != "linux" {
		// On non-Linux (macOS), we only have os.Getppid() — no /proc to walk further.
		maxLevels = 1
	}

	for i := 0; i < maxLevels && pid > 1; i++ {
		sessionFile := filepath.Join(claudeDir, "sessions", fmt.Sprintf("%d.json", pid))
		data, err := os.ReadFile(sessionFile)
		if err == nil {
			var session struct {
				StartedAt int64 `json:"startedAt"`
			}
			if json.Unmarshal(data, &session) == nil && session.StartedAt > 0 {
				elapsed := time.Now().UnixMilli() - session.StartedAt
				return &elapsed
			}
		}

		// Walk up via /proc (Linux only)
		if runtime.GOOS != "linux" {
			break
		}
		statPath := fmt.Sprintf("/proc/%d/stat", pid)
		statData, err := os.ReadFile(statPath)
		if err != nil {
			break
		}
		// Field 4 is PPID. Fields are space-separated, but field 2 (comm) is in parens
		// and may contain spaces. Find the closing paren to skip it safely.
		stat := string(statData)
		closeParenIdx := strings.LastIndex(stat, ")")
		if closeParenIdx < 0 || closeParenIdx+2 >= len(stat) {
			break
		}
		// After ") " come fields starting at index 3 (state). PPID is the next field (index 4).
		rest := strings.TrimLeft(stat[closeParenIdx+1:], " ")
		fields := strings.Fields(rest)
		if len(fields) < 2 {
			break
		}
		ppid, err := strconv.Atoi(fields[1]) // fields[0]=state, fields[1]=ppid
		if err != nil || ppid <= 1 {
			break
		}
		pid = ppid
	}

	return nil
}

// GetClaudeDir returns the Claude configuration directory path.
// Priority: positional arg > CLAUDE_CONFIG_DIR env > ~/.claude
func GetClaudeDir() string {
	// Check os.Args for a positional claude dir (first non-flag argument after program name)
	if len(os.Args) > 1 {
		arg := os.Args[1]
		if !strings.HasPrefix(arg, "-") {
			return resolvePath(arg)
		}
	}

	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return resolvePath(dir)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ".claude"
	}
	return filepath.Join(home, ".claude")
}

// GetProfileName extracts the profile name from the claude directory basename.
// Returns "" for the default .claude dir (profile not shown).
func GetProfileName(claudeDir string) string {
	base := filepath.Base(claudeDir)
	if base == ".claude" {
		return ""
	}
	if strings.HasPrefix(base, ".claude-") {
		remainder := strings.TrimPrefix(base, ".claude-")
		if remainder == "" {
			return ""
		}
		return remainder
	}
	return base
}

var aiAgentRe = regexp.MustCompile(`^claude-code_([\d_-]+?)_`)

// GetCCVersion extracts the Claude Code version from environment variables.
// Returns "" if neither CLAUDE_CODE_EXECPATH nor AI_AGENT is set.
func GetCCVersion() string {
	if execpath := os.Getenv("CLAUDE_CODE_EXECPATH"); execpath != "" {
		// Split by /, take last segment, replace - with ., prepend v
		parts := strings.Split(execpath, "/")
		last := parts[len(parts)-1]
		return "v" + strings.ReplaceAll(last, "-", ".")
	}

	if agent := os.Getenv("AI_AGENT"); agent != "" {
		m := aiAgentRe.FindStringSubmatch(agent)
		if m != nil {
			version := strings.NewReplacer("_", ".", "-", ".").Replace(m[1])
			return "v" + version
		}
	}

	return ""
}

func resolvePath(p string) string {
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
