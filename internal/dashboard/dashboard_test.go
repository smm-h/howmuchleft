package dashboard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smm-h/howmuchleft/internal/cache"
	"github.com/smm-h/howmuchleft/internal/render"
)

func TestDiscoverProfiles_IncludesDefault(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}

	defaultDir := filepath.Join(home, ".claude")
	if _, err := os.Stat(defaultDir); os.IsNotExist(err) {
		t.Skip("~/.claude does not exist")
	}

	dirs := DiscoverProfiles()
	found := false
	for _, d := range dirs {
		if d == defaultDir {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("DiscoverProfiles() did not include default dir %s; got %v", defaultDir, dirs)
	}
}

func TestDiscoverProfiles_Deduplicates(t *testing.T) {
	dirs := DiscoverProfiles()
	seen := make(map[string]bool)
	for _, d := range dirs {
		if seen[d] {
			t.Errorf("DiscoverProfiles() returned duplicate: %s", d)
		}
		seen[d] = true
	}
}

func TestDiscoverProfiles_Sorted(t *testing.T) {
	dirs := DiscoverProfiles()
	for i := 1; i < len(dirs); i++ {
		if dirs[i-1] > dirs[i] {
			t.Errorf("DiscoverProfiles() not sorted: %s > %s", dirs[i-1], dirs[i])
		}
	}
}

func TestProfileDisplayName(t *testing.T) {
	cases := []struct {
		dir  string
		want string
	}{
		{"/home/user/.claude", "default"},
		{"/home/user/.claude-work", "work"},
		{"/home/user/.claude-personal", "personal"},
		{"/home/user/custom", "custom"},
	}
	for _, tc := range cases {
		got := profileDisplayName(tc.dir)
		if got != tc.want {
			t.Errorf("profileDisplayName(%q) = %q, want %q", tc.dir, got, tc.want)
		}
	}
}

func TestRenderProfileRows_ProducesThreeLines(t *testing.T) {
	barCfg := &render.BarConfig{
		Width:         3,
		EmptyBg:       "\x1b[48;5;236m",
		Gradient:      render.BuiltinColors[2].Gradient, // dark 256-color
		Truecolor:     false,
		IsRgb:         false,
		PartialBlocks: true,
	}

	fivePct := 45.0
	weekPct := 20.0
	usage := &cache.UsageResult{
		FiveHour: &cache.WindowResult{Percent: fivePct, ResetIn: 3600000},
		Weekly:   &cache.WindowResult{Percent: weekPct, ResetIn: 86400000},
	}

	output := renderProfileRows("test", render.Cyan, "Pro", usage, barCfg)
	lines := strings.Split(output, "\n")
	if len(lines) != 3 {
		t.Errorf("renderProfileRows produced %d lines, want 3", len(lines))
	}

	// First line should contain profile name
	if !strings.Contains(lines[0], "test") {
		t.Errorf("line 0 missing profile name; got %q", lines[0])
	}
	// First line should contain tier
	if !strings.Contains(lines[0], "Pro") {
		t.Errorf("line 0 missing tier; got %q", lines[0])
	}
}

func TestRenderProfileRows_NilUsage(t *testing.T) {
	barCfg := &render.BarConfig{
		Width:         3,
		EmptyBg:       "\x1b[48;5;236m",
		Gradient:      render.BuiltinColors[2].Gradient,
		Truecolor:     false,
		IsRgb:         false,
		PartialBlocks: true,
	}

	usage := &cache.UsageResult{}

	output := renderProfileRows("empty", render.Cyan, "Pro", usage, barCfg)
	lines := strings.Split(output, "\n")
	if len(lines) != 3 {
		t.Errorf("renderProfileRows with nil windows produced %d lines, want 3", len(lines))
	}
}

func TestRenderProfileRows_ExtraUsage(t *testing.T) {
	barCfg := &render.BarConfig{
		Width:         3,
		EmptyBg:       "\x1b[48;5;236m",
		Gradient:      render.BuiltinColors[2].Gradient,
		Truecolor:     false,
		IsRgb:         false,
		PartialBlocks: true,
	}

	usage := &cache.UsageResult{
		FiveHour: &cache.WindowResult{Percent: 80, ResetIn: 1800000},
		Weekly:   &cache.WindowResult{Percent: 100, ResetIn: 86400000},
		Extra:    &cache.ExtraResult{Enabled: true, Percent: 30},
	}

	output := renderProfileRows("maxed", render.Cyan, "Max 5x", usage, barCfg)
	lines := strings.Split(output, "\n")
	if len(lines) != 3 {
		t.Errorf("renderProfileRows with extra produced %d lines, want 3", len(lines))
	}
}

func TestFormatUsageLine(t *testing.T) {
	// nil window
	line := formatUsageLine("5h", nil, false)
	if !strings.Contains(line, "?%") {
		t.Errorf("formatUsageLine with nil window should contain '?%%'; got %q", line)
	}

	// normal window
	window := &cache.WindowResult{Percent: 42.6, ResetIn: 7200000}
	line = formatUsageLine("5h", window, false)
	if !strings.Contains(line, "43%") {
		t.Errorf("formatUsageLine should contain '43%%'; got %q", line)
	}
	if !strings.Contains(line, "2h") {
		t.Errorf("formatUsageLine should contain reset time; got %q", line)
	}

	// stale window
	line = formatUsageLine("7d", window, true)
	if !strings.Contains(line, "~43%") {
		t.Errorf("formatUsageLine stale should contain '~43%%'; got %q", line)
	}
}

func TestResolvePath(t *testing.T) {
	home := "/home/testuser"
	cases := []struct {
		input string
		want  string
	}{
		{"~/foo", "/home/testuser/foo"},
		{"~/.claude", "/home/testuser/.claude"},
		{"/abs/path", "/abs/path"},
	}
	for _, tc := range cases {
		got := resolvePath(tc.input, home)
		if got != tc.want {
			t.Errorf("resolvePath(%q, %q) = %q, want %q", tc.input, home, got, tc.want)
		}
	}
}

func TestDiscoverProfiles_WithTempDirs(t *testing.T) {
	// Create a temp directory simulating a home dir with .claude and .claude-work
	tmpHome := t.TempDir()
	defaultDir := filepath.Join(tmpHome, ".claude")
	workDir := filepath.Join(tmpHome, ".claude-work")

	if err := os.MkdirAll(defaultDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Put a .credentials.json in workDir so it's discovered via source 3
	credPath := filepath.Join(workDir, ".credentials.json")
	if err := os.WriteFile(credPath, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Override HOME so DiscoverProfiles sees our temp dirs
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	dirs := DiscoverProfiles()

	// Should include the default dir
	foundDefault := false
	foundWork := false
	for _, d := range dirs {
		if d == defaultDir {
			foundDefault = true
		}
		if d == workDir {
			foundWork = true
		}
	}
	if !foundDefault {
		t.Errorf("DiscoverProfiles() missing default dir %s; got %v", defaultDir, dirs)
	}
	if !foundWork {
		t.Errorf("DiscoverProfiles() missing work dir %s; got %v", workDir, dirs)
	}
}

func TestFetchAndRender_MissingCredentials(t *testing.T) {
	// FetchAndRender should return an error (not panic) when credentials are absent
	tmpDir := t.TempDir()

	barCfg := &render.BarConfig{
		Width:         3,
		EmptyBg:       "\x1b[48;5;236m",
		Gradient:      render.BuiltinColors[2].Gradient,
		Truecolor:     false,
		IsRgb:         false,
		PartialBlocks: true,
	}

	output, err := FetchAndRender(tmpDir, barCfg)
	if err == nil {
		t.Error("FetchAndRender with no credentials should return error, got nil")
	}
	if output != "" {
		t.Errorf("FetchAndRender with no credentials should return empty output, got %q", output)
	}
	if !strings.Contains(err.Error(), "no credentials") {
		t.Errorf("FetchAndRender error should mention credentials; got %q", err.Error())
	}
}

func TestRenderProfileRows_NonEmpty(t *testing.T) {
	barCfg := &render.BarConfig{
		Width:         3,
		EmptyBg:       "\x1b[48;5;236m",
		Gradient:      render.BuiltinColors[2].Gradient,
		Truecolor:     false,
		IsRgb:         false,
		PartialBlocks: true,
	}

	usage := &cache.UsageResult{
		FiveHour: &cache.WindowResult{Percent: 50, ResetIn: 3600000},
		Weekly:   &cache.WindowResult{Percent: 30, ResetIn: 172800000},
	}

	output := renderProfileRows("myprofile", render.Cyan, "Pro", usage, barCfg)
	if output == "" {
		t.Error("renderProfileRows should produce non-empty output")
	}
	// Should contain all three lines
	lines := strings.Split(output, "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}
	for i, line := range lines {
		if line == "" {
			t.Errorf("line %d is empty", i)
		}
	}
}
