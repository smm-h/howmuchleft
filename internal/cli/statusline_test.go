package cli

import (
	"github.com/smm-h/stricttest/go/hygiene"
	"strings"
	"testing"
)

func TestParseStdin_Full(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	input := `{
		"model": "claude-sonnet-4-6-20250514",
		"context_window": {"used_percentage": 45.5},
		"cwd": "/home/user/project",
		"cost": {"total": 1.23, "total_lines_added": 10, "total_lines_removed": 5}
	}`
	data := parseStdin(strings.NewReader(input))

	if data.Cwd != "/home/user/project" {
		t.Errorf("expected cwd /home/user/project, got %q", data.Cwd)
	}
	if data.ContextWindow == nil {
		t.Fatal("expected context_window to be non-nil")
	}
	pct, ok := data.ContextWindow["used_percentage"].(float64)
	if !ok || pct != 45.5 {
		t.Errorf("expected used_percentage 45.5, got %v", pct)
	}
}

func TestParseStdin_Minimal(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	input := `{}`
	data := parseStdin(strings.NewReader(input))

	if data.Cwd != "" {
		t.Errorf("expected empty cwd, got %q", data.Cwd)
	}
	if data.ContextWindow != nil {
		t.Errorf("expected nil context_window, got %v", data.ContextWindow)
	}
}

func TestParseStdin_Malformed(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	input := `not json at all`
	data := parseStdin(strings.NewReader(input))

	// Should fall back to empty struct without panic
	if data.Cwd != "" {
		t.Errorf("expected empty cwd on malformed input, got %q", data.Cwd)
	}
}

func TestParseStdin_Empty(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	data := parseStdin(strings.NewReader(""))

	if data.Cwd != "" {
		t.Errorf("expected empty cwd on empty input, got %q", data.Cwd)
	}
}

func TestExtractModel_String(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	got := extractModel("claude-opus-4-6")
	if got != "claude-opus-4-6" {
		t.Errorf("expected claude-opus-4-6, got %q", got)
	}
}

func TestExtractModel_ObjectDisplayName(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	obj := map[string]interface{}{
		"display_name": "Claude Opus",
		"id":           "claude-opus-4-6",
	}
	got := extractModel(obj)
	if got != "Claude Opus" {
		t.Errorf("expected 'Claude Opus', got %q", got)
	}
}

func TestExtractModel_ObjectIdFallback(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	obj := map[string]interface{}{
		"id": "claude-opus-4-6",
	}
	got := extractModel(obj)
	if got != "claude-opus-4-6" {
		t.Errorf("expected 'claude-opus-4-6', got %q", got)
	}
}

func TestExtractModel_Nil(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	got := extractModel(nil)
	if got != "?" {
		t.Errorf("expected '?', got %q", got)
	}
}

func TestExtractModel_EmptyString(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	got := extractModel("")
	if got != "?" {
		t.Errorf("expected '?', got %q", got)
	}
}

func TestExtractContextPercent(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	tests := []struct {
		name string
		cw   map[string]interface{}
		want float64
	}{
		{"nil map", nil, 0},
		{"empty map", map[string]interface{}{}, 0},
		{"with percent", map[string]interface{}{"used_percentage": 42.5}, 42.5},
		{"wrong type", map[string]interface{}{"used_percentage": "bad"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractContextPercent(tt.cw)
			if got != tt.want {
				t.Errorf("extractContextPercent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractCwd(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	// Direct cwd
	data := stdinData{Cwd: "/some/path"}
	got := extractCwd(data)
	if got != "/some/path" {
		t.Errorf("expected /some/path, got %q", got)
	}

	// Workspace fallback
	data = stdinData{
		Workspace: map[string]interface{}{"current_dir": "/workspace/dir"},
	}
	got = extractCwd(data)
	if got != "/workspace/dir" {
		t.Errorf("expected /workspace/dir, got %q", got)
	}

	// Both empty: falls back to os.Getwd()
	data = stdinData{}
	got = extractCwd(data)
	if got == "" {
		t.Error("expected non-empty cwd from Getwd fallback")
	}
}

func TestHasStdinUsage(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	tests := []struct {
		name       string
		rateLimits map[string]interface{}
		want       bool
	}{
		{"nil", nil, false},
		{"empty", map[string]interface{}{}, false},
		{"no used_percentage in any key", map[string]interface{}{
			"five_hour": map[string]interface{}{"resets_at": 123.0},
		}, false},
		{"only five_hour present", map[string]interface{}{
			"five_hour": map[string]interface{}{"used_percentage": 50.0},
		}, true},
		{"only seven_day_overage_included present", map[string]interface{}{
			"seven_day_overage_included": map[string]interface{}{"used_percentage": 20.0},
		}, true},
		{"only seven_day present", map[string]interface{}{
			"seven_day": map[string]interface{}{"used_percentage": 30.0},
		}, true},
		{"non-map value", map[string]interface{}{
			"extra_usage": "not a map",
		}, false},
		{"map without used_percentage", map[string]interface{}{
			"five_hour": map[string]interface{}{"other_field": 42.0},
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasStdinUsage(tt.rateLimits)
			if got != tt.want {
				t.Errorf("hasStdinUsage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUsageFromStdinRateLimits(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	rateLimits := map[string]interface{}{
		"five_hour": map[string]interface{}{
			"used_percentage": 75.0,
			"resets_at":       float64(1700000000),
		},
		"seven_day": map[string]interface{}{
			"used_percentage": 30.0,
		},
		"extra_usage": map[string]interface{}{
			"is_enabled":  true,
			"utilization": 15.0,
		},
	}

	result := usageFromStdinRateLimits(rateLimits)

	if result.Stale {
		t.Error("expected Stale=false")
	}
	if result.FiveHour == nil {
		t.Fatal("expected FiveHour to be non-nil")
	}
	if result.FiveHour.Percent != 75.0 {
		t.Errorf("expected FiveHour.Percent=75.0, got %v", result.FiveHour.Percent)
	}
	if result.Weekly == nil {
		t.Fatal("expected Weekly to be non-nil")
	}
	if result.Weekly.Percent != 30.0 {
		t.Errorf("expected Weekly.Percent=30.0, got %v", result.Weekly.Percent)
	}
	if result.Extra == nil {
		t.Fatal("expected Extra to be non-nil")
	}
	if !result.Extra.Enabled {
		t.Error("expected Extra.Enabled=true")
	}
	if result.Extra.Percent != 15.0 {
		t.Errorf("expected Extra.Percent=15.0, got %v", result.Extra.Percent)
	}
}

func TestUsageFromStdinRateLimitsAllThreeWindows(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	rateLimits := map[string]interface{}{
		"five_hour": map[string]interface{}{
			"used_percentage": 75.0,
			"resets_at":       float64(1700000000),
		},
		"seven_day": map[string]interface{}{
			"used_percentage": 30.0,
			"resets_at":       float64(1700500000),
		},
		"seven_day_overage_included": map[string]interface{}{
			"used_percentage": 10.0,
			"resets_at":       float64(1700500000),
		},
		"extra_usage": map[string]interface{}{
			"is_enabled":  true,
			"utilization": 15.0,
		},
	}

	result := usageFromStdinRateLimits(rateLimits)

	if result.FiveHour == nil {
		t.Fatal("expected FiveHour to be non-nil")
	}
	if result.FiveHour.Percent != 75.0 {
		t.Errorf("FiveHour.Percent = %v, want 75.0", result.FiveHour.Percent)
	}
	if result.Weekly == nil {
		t.Fatal("expected Weekly to be non-nil")
	}
	if result.Weekly.Percent != 30.0 {
		t.Errorf("Weekly.Percent = %v, want 30.0", result.Weekly.Percent)
	}
	if result.FableWeekly == nil {
		t.Fatal("expected FableWeekly to be non-nil")
	}
	if result.FableWeekly.Percent != 10.0 {
		t.Errorf("FableWeekly.Percent = %v, want 10.0", result.FableWeekly.Percent)
	}
	if result.Extra == nil {
		t.Fatal("expected Extra to be non-nil")
	}
	if !result.Extra.Enabled || result.Extra.Percent != 15.0 {
		t.Errorf("Extra = {Enabled:%v, Percent:%v}, want {true, 15.0}", result.Extra.Enabled, result.Extra.Percent)
	}
}

func TestUsageFromStdinRateLimitsUnknownKeyIgnored(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	rateLimits := map[string]interface{}{
		"five_hour": map[string]interface{}{
			"used_percentage": 50.0,
		},
		"unknown_window": map[string]interface{}{
			"used_percentage": 99.0,
		},
	}

	result := usageFromStdinRateLimits(rateLimits)

	if result.FiveHour == nil {
		t.Fatal("expected FiveHour to be non-nil")
	}
	if result.FiveHour.Percent != 50.0 {
		t.Errorf("FiveHour.Percent = %v, want 50.0", result.FiveHour.Percent)
	}
	// Unknown key should not populate any known field
	if result.Weekly != nil {
		t.Error("expected Weekly to be nil")
	}
	if result.FableWeekly != nil {
		t.Error("expected FableWeekly to be nil")
	}
}

func TestUsageFromStdinRateLimitsFableWeeklyOnly(t *testing.T) {
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	rateLimits := map[string]interface{}{
		"seven_day_overage_included": map[string]interface{}{
			"used_percentage": 42.0,
			"resets_at":       float64(1700500000),
		},
	}

	result := usageFromStdinRateLimits(rateLimits)

	if result.FiveHour != nil {
		t.Error("expected FiveHour to be nil")
	}
	if result.Weekly != nil {
		t.Error("expected Weekly to be nil")
	}
	if result.FableWeekly == nil {
		t.Fatal("expected FableWeekly to be non-nil")
	}
	if result.FableWeekly.Percent != 42.0 {
		t.Errorf("FableWeekly.Percent = %v, want 42.0", result.FableWeekly.Percent)
	}
}
