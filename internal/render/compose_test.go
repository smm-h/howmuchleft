package render

import (
	"os"
	"strings"
	"testing"

	"github.com/smm-h/howmuchleft/internal/config"
)

func TestShortenModelName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// Known patterns
		{"claude-sonnet-4-5-20250514", "S4.5"},
		{"claude-opus-4-6-20250514", "O4.6"},
		{"claude-haiku-3-5", "H3.5"},
		{"claude-sonnet-3-5-2-20250514", "S3.5.2"},
		{"claude-opus-4-0", "O4.0"},
		{"claude-haiku-3-5-0", "H3.5"},   // patch "0" is omitted
		{"claude-sonnet-4-5", "S4.5"},     // no date suffix
		{"claude-opus-4-6", "O4.6"},       // no date suffix
		// Unknown passthrough
		{"gpt-4", "gpt-4"},
		{"some-random-model", "some-random-model"},
		{"", ""},
		{"claude-unknown-4-5", "claude-unknown-4-5"},
	}

	for _, tt := range tests {
		got := ShortenModelName(tt.input)
		if got != tt.want {
			t.Errorf("ShortenModelName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatPercent_Nil(t *testing.T) {
	result := FormatPercent(nil, false)
	if !strings.Contains(result, "?%") {
		t.Errorf("FormatPercent(nil, false) = %q, want to contain '?%%'", result)
	}
	if !strings.Contains(result, Gray) {
		t.Errorf("FormatPercent(nil, false) should use gray color")
	}
}

func TestFormatPercent_Stale(t *testing.T) {
	pct := 75.4
	result := FormatPercent(&pct, true)
	if !strings.Contains(result, "~75%") {
		t.Errorf("FormatPercent(75.4, stale) = %q, want to contain '~75%%'", result)
	}
	if !strings.Contains(result, Dim) {
		t.Errorf("FormatPercent stale should use dim")
	}
}

func TestFormatPercent_Normal(t *testing.T) {
	pct := 42.6
	result := FormatPercent(&pct, false)
	if !strings.Contains(result, "43%") {
		t.Errorf("FormatPercent(42.6, false) = %q, want to contain '43%%'", result)
	}
	if !strings.Contains(result, Cyan) {
		t.Errorf("FormatPercent normal should use cyan")
	}
}

func TestFormatTimeRemaining(t *testing.T) {
	tests := []struct {
		ms   int64
		want string
	}{
		{-100, ""},           // negative -> empty
		{0, "0s"},            // 0 ms
		{5000, "5s"},         // 5 seconds
		{59999, "59s"},       // just under 1 minute
		{60000, "1m"},        // exactly 1 minute
		{300000, "5m"},       // 5 minutes
		{3599999, "59m"},     // just under 1 hour
		{3600000, "1h0m"},    // exactly 1 hour
		{5400000, "1h30m"},   // 1.5 hours
		{86399999, "23h59m"}, // just under 1 day
		{86400000, "1d0h"},   // exactly 1 day
		{90000000, "1d1h"},   // 1 day 1 hour
	}

	for _, tt := range tests {
		got := FormatTimeRemaining(tt.ms)
		if got != tt.want {
			t.Errorf("FormatTimeRemaining(%d) = %q, want %q", tt.ms, got, tt.want)
		}
	}
}

func TestFormatAge(t *testing.T) {
	tests := []struct {
		ms   int64
		want string
	}{
		{0, "0s"},
		{5000, "5s"},
		{59999, "59s"},
		{60000, "1m"},
		{300000, "5m"},
		{3599999, "59m"},
		{3600000, "1h"},
		{7200000, "2h"},
	}

	for _, tt := range tests {
		got := FormatAge(tt.ms)
		if got != tt.want {
			t.Errorf("FormatAge(%d) = %q, want %q", tt.ms, got, tt.want)
		}
	}
}

func TestShortenPath(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		path   string
		maxLen int
		depth  int
		want   string
	}{
		// Empty -> "~"
		{"", 50, 3, "~"},
		// Short enough, no change (with ~ replacement)
		{home + "/Projects", 50, 3, "~/Projects"},
		// Long path gets truncated
		{home + "/very/long/deeply/nested/project/path", 20, 3, "~/.../" + "nested/project/path"},
		// Non-home path
		{"/usr/local/bin/something", 50, 3, "/usr/local/bin/something"},
	}

	for _, tt := range tests {
		got := ShortenPath(tt.path, tt.maxLen, tt.depth)
		if got != tt.want {
			t.Errorf("ShortenPath(%q, %d, %d) = %q, want %q", tt.path, tt.maxLen, tt.depth, got, tt.want)
		}
	}
}

func TestBuildLineText(t *testing.T) {
	elements := map[string]func() string{
		"a": func() string { return "hello" },
		"b": func() string { return "" },      // empty, should be filtered
		"c": func() string { return "world" },
		"d": nil, // nil func, should be filtered
	}

	// Normal case: mixed empty/non-empty
	result := BuildLineText(elements, []string{"a", "b", "c", "d"})
	if result != "hello world" {
		t.Errorf("BuildLineText = %q, want %q", result, "hello world")
	}

	// Only empty elements
	result = BuildLineText(elements, []string{"b", "d"})
	if result != "" {
		t.Errorf("BuildLineText with all empty = %q, want empty", result)
	}

	// Unknown element name
	result = BuildLineText(elements, []string{"unknown", "a"})
	if result != "hello" {
		t.Errorf("BuildLineText with unknown = %q, want %q", result, "hello")
	}

	// Empty order
	result = BuildLineText(elements, []string{})
	if result != "" {
		t.Errorf("BuildLineText with empty order = %q, want empty", result)
	}

	// Nil order
	result = BuildLineText(elements, nil)
	if result != "" {
		t.Errorf("BuildLineText with nil order = %q, want empty", result)
	}
}

func TestRenderLines_NilConfig(t *testing.T) {
	barCfg := &BarConfig{
		Width:         12,
		EmptyBg:       "\x1b[48;2;48;48;48m",
		Gradient:      []GradientStop{NewRgbStop(0, 255, 0), NewRgbStop(255, 0, 0)},
		Truecolor:     true,
		IsRgb:         true,
		PartialBlocks: true,
	}

	data := &RenderData{
		Context: 50,
		Model:   "claude-sonnet-4-5",
		Tier:    "Pro",
	}

	// nil lineElements should produce 3 empty lines
	result := RenderLines(data, barCfg, nil)
	if result != "\n\n" {
		t.Errorf("RenderLines with nil lineElements = %q, want %q", result, "\n\n")
	}
}

func TestRenderLines_ProducesThreeLines(t *testing.T) {
	// Set deterministic environment
	os.Setenv("HOWMUCHLEFT_DARK", "1")
	os.Setenv("COLORTERM", "truecolor")
	ResetDarkModeCache()
	ResetTruecolorCache()
	defer func() {
		os.Unsetenv("HOWMUCHLEFT_DARK")
		os.Unsetenv("COLORTERM")
		ResetDarkModeCache()
		ResetTruecolorCache()
	}()

	barCfg := &BarConfig{
		Width:         12,
		EmptyBg:       "\x1b[48;2;48;48;48m",
		TimeBarBg:     "\x1b[48;2;38;38;38m",
		Gradient:      []GradientStop{NewRgbStop(0, 255, 0), NewRgbStop(255, 0, 0)},
		Truecolor:     true,
		IsRgb:         true,
		PartialBlocks: true,
	}

	fiveHourPct := 30.0
	weeklyPct := 45.0
	elapsed := int64(120000)

	data := &RenderData{
		Context:  50,
		Model:    "claude-sonnet-4-5-20250514",
		Tier:     "Pro",
		Elapsed:  &elapsed,
		FiveHour: UsageData{Percent: &fiveHourPct, ResetIn: 3600000},
		Weekly:   UsageData{Percent: &weeklyPct, ResetIn: 86400000},
		Git:      GitInfo{Branch: "main", HasGit: true, Changes: 2},
		Cwd:      "~/Projects/test",
	}

	lineElements := &config.LinesConfig{
		Line1: []string{"context", "elapsed", "tier", "model"},
		Line2: []string{"usage5h", "age", "branch"},
		Line3: []string{"usageWeekly", "age", "cwd"},
	}

	result := RenderLines(data, barCfg, lineElements)
	lineCount := strings.Count(result, "\n") + 1
	if lineCount != 3 {
		t.Errorf("RenderLines produced %d lines, want 3. Output: %q", lineCount, result)
	}

	// Should contain model info
	if !strings.Contains(result, "S4.5") {
		t.Errorf("RenderLines should contain shortened model name 'S4.5', got %q", result)
	}

	// Should contain tier
	if !strings.Contains(result, "Pro") {
		t.Errorf("RenderLines should contain tier 'Pro'")
	}

	// Should contain git branch
	if !strings.Contains(result, "main") {
		t.Errorf("RenderLines should contain git branch 'main'")
	}
}

func TestRenderLines_ZeroWidth(t *testing.T) {
	// Width=0 means no bars, just text
	os.Setenv("HOWMUCHLEFT_DARK", "1")
	ResetDarkModeCache()
	defer func() {
		os.Unsetenv("HOWMUCHLEFT_DARK")
		ResetDarkModeCache()
	}()

	barCfg := &BarConfig{
		Width:     0,
		Truecolor: true,
		IsRgb:     true,
	}

	data := &RenderData{
		Context: 25,
		Model:   "claude-opus-4-6",
		Tier:    "Max 5x",
		Git:     GitInfo{HasGit: false},
		Cwd:     "~/test",
	}

	lineElements := &config.LinesConfig{
		Line1: []string{"context", "tier", "model"},
		Line2: []string{"branch"},
		Line3: []string{"cwd"},
	}

	result := RenderLines(data, barCfg, lineElements)
	parts := strings.Split(result, "\n")
	if len(parts) != 3 {
		t.Fatalf("RenderLines zero-width produced %d lines, want 3", len(parts))
	}

	// Line 1 should have context, tier, model
	if !strings.Contains(parts[0], "25%") {
		t.Errorf("Line 1 should contain '25%%', got %q", parts[0])
	}
	if !strings.Contains(parts[0], "O4.6") {
		t.Errorf("Line 1 should contain 'O4.6', got %q", parts[0])
	}

	// Line 2 should have "no .git"
	if !strings.Contains(parts[1], "no .git") {
		t.Errorf("Line 2 should contain 'no .git', got %q", parts[1])
	}

	// Line 3 should have cwd
	if !strings.Contains(parts[2], "~/test") {
		t.Errorf("Line 3 should contain '~/test', got %q", parts[2])
	}
}
