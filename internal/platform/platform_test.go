package platform

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetProfileName(t *testing.T) {
	tests := []struct {
		claudeDir string
		want      string
	}{
		{"/home/user/.claude", ""},
		{"/home/user/.claude-work", "work"},
		{"/home/user/.claude-", ""},
		{"/home/user/custom-dir", "custom-dir"},
		{"/home/user/.claude-my-profile", "my-profile"},
	}

	for _, tt := range tests {
		t.Run(tt.claudeDir, func(t *testing.T) {
			got := GetProfileName(tt.claudeDir)
			if got != tt.want {
				t.Errorf("GetProfileName(%q) = %q, want %q", tt.claudeDir, got, tt.want)
			}
		})
	}
}

func TestGetCCVersion_Execpath(t *testing.T) {
	t.Setenv("CLAUDE_CODE_EXECPATH", "/usr/local/bin/1-0-3")
	t.Setenv("AI_AGENT", "")

	got := GetCCVersion()
	want := "v1.0.3"
	if got != want {
		t.Errorf("GetCCVersion() = %q, want %q", got, want)
	}
}

func TestGetCCVersion_AIAgent(t *testing.T) {
	t.Setenv("CLAUDE_CODE_EXECPATH", "")
	// Real AI_AGENT format uses dashes between version segments (non-greedy regex
	// would stop too early with underscores since _ is in the capture class).
	t.Setenv("AI_AGENT", "claude-code_1-0-14_user123")

	got := GetCCVersion()
	want := "v1.0.14"
	if got != want {
		t.Errorf("GetCCVersion() = %q, want %q", got, want)
	}
}

func TestGetCCVersion_AIAgentWithDash(t *testing.T) {
	t.Setenv("CLAUDE_CODE_EXECPATH", "")
	t.Setenv("AI_AGENT", "claude-code_1-0-3_somethingelse")

	got := GetCCVersion()
	want := "v1.0.3"
	if got != want {
		t.Errorf("GetCCVersion() = %q, want %q", got, want)
	}
}

func TestGetCCVersion_Neither(t *testing.T) {
	t.Setenv("CLAUDE_CODE_EXECPATH", "")
	t.Setenv("AI_AGENT", "")

	got := GetCCVersion()
	if got != "" {
		t.Errorf("GetCCVersion() = %q, want empty string", got)
	}
}

func TestGetCCVersion_AIAgentNoMatch(t *testing.T) {
	t.Setenv("CLAUDE_CODE_EXECPATH", "")
	t.Setenv("AI_AGENT", "some-other-agent_foo")

	got := GetCCVersion()
	if got != "" {
		t.Errorf("GetCCVersion() = %q, want empty string", got)
	}
}

func TestGetSessionElapsed_Found(t *testing.T) {
	// Create a temporary directory with a session file for the current parent PID.
	dir := t.TempDir()
	sessionsDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatal(err)
	}

	ppid := os.Getppid()
	startedAt := time.Now().Add(-5 * time.Minute).UnixMilli()
	session := map[string]interface{}{
		"startedAt": startedAt,
		"pid":       ppid,
	}
	data, _ := json.Marshal(session)
	sessionFile := filepath.Join(sessionsDir, fmt.Sprintf("%d.json", ppid))
	if err := os.WriteFile(sessionFile, data, 0644); err != nil {
		t.Fatal(err)
	}

	elapsed := GetSessionElapsed(dir)
	if elapsed == nil {
		t.Fatal("expected non-nil elapsed time")
	}
	// Should be approximately 5 minutes (300000 ms), allow some tolerance
	if *elapsed < 299000 || *elapsed > 310000 {
		t.Errorf("elapsed = %d ms, want ~300000 ms", *elapsed)
	}
}

func TestGetSessionElapsed_NotFound(t *testing.T) {
	dir := t.TempDir()
	elapsed := GetSessionElapsed(dir)
	if elapsed != nil {
		t.Errorf("expected nil elapsed, got %d", *elapsed)
	}
}
