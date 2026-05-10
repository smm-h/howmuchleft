package render

import (
	"os"
	"strings"
	"testing"
)

func TestShouldUsePartialBlocks(t *testing.T) {
	// Save and restore env
	origTermProgram := os.Getenv("TERM_PROGRAM")
	origTerm := os.Getenv("TERM")
	defer func() {
		os.Setenv("TERM_PROGRAM", origTermProgram)
		os.Setenv("TERM", origTerm)
	}()

	// Explicit override: "true"
	os.Setenv("TERM_PROGRAM", "")
	os.Setenv("TERM", "")
	if !ShouldUsePartialBlocks("true") {
		t.Error("ShouldUsePartialBlocks(\"true\") = false, want true")
	}

	// Explicit override: "false"
	if ShouldUsePartialBlocks("false") {
		t.Error("ShouldUsePartialBlocks(\"false\") = true, want false")
	}

	// Auto-detect: Apple_Terminal blocklisted via TERM_PROGRAM
	os.Setenv("TERM_PROGRAM", "Apple_Terminal")
	os.Setenv("TERM", "xterm-256color")
	if ShouldUsePartialBlocks("auto") {
		t.Error("ShouldUsePartialBlocks with TERM_PROGRAM=Apple_Terminal should be false")
	}

	// Auto-detect: linux console blocklisted via TERM
	os.Setenv("TERM_PROGRAM", "")
	os.Setenv("TERM", "linux")
	if ShouldUsePartialBlocks("auto") {
		t.Error("ShouldUsePartialBlocks with TERM=linux should be false")
	}

	// Auto-detect: normal terminal (not blocklisted)
	os.Setenv("TERM_PROGRAM", "iTerm2")
	os.Setenv("TERM", "xterm-256color")
	if !ShouldUsePartialBlocks("auto") {
		t.Error("ShouldUsePartialBlocks with iTerm2 should be true")
	}

	// Auto-detect: empty env (default enabled)
	os.Setenv("TERM_PROGRAM", "")
	os.Setenv("TERM", "")
	if !ShouldUsePartialBlocks("") {
		t.Error("ShouldUsePartialBlocks with empty env should default to true")
	}
}

func TestGetGradientStop_Rgb(t *testing.T) {
	config := &BarConfig{
		Gradient: []GradientStop{
			NewRgbStop(0, 255, 0),   // green at 0%
			NewRgbStop(255, 0, 0),   // red at 100%
		},
		Truecolor: true,
		IsRgb:     true,
	}

	// At 0%
	result := GetGradientStop(0, config)
	if result.Fg != "\x1b[38;2;0;255;0m" {
		t.Errorf("GetGradientStop(0) fg = %q, want green truecolor", result.Fg)
	}
	if result.Bg != "\x1b[48;2;0;255;0m" {
		t.Errorf("GetGradientStop(0) bg = %q, want green truecolor bg", result.Bg)
	}

	// At 100%
	result = GetGradientStop(100, config)
	if result.Fg != "\x1b[38;2;255;0;0m" {
		t.Errorf("GetGradientStop(100) fg = %q, want red truecolor", result.Fg)
	}

	// At 50%
	result = GetGradientStop(50, config)
	if result.Fg != "\x1b[38;2;128;128;0m" {
		t.Errorf("GetGradientStop(50) fg = %q, want interpolated midpoint", result.Fg)
	}

	// Clamping: above 100
	result = GetGradientStop(150, config)
	if result.Fg != "\x1b[38;2;255;0;0m" {
		t.Errorf("GetGradientStop(150) should clamp to 100%%, got fg = %q", result.Fg)
	}

	// Clamping: below 0
	result = GetGradientStop(-50, config)
	if result.Fg != "\x1b[38;2;0;255;0m" {
		t.Errorf("GetGradientStop(-50) should clamp to 0%%, got fg = %q", result.Fg)
	}
}

func TestGetGradientStop_256Color(t *testing.T) {
	config := &BarConfig{
		Gradient: []GradientStop{
			NewIndexStop(46),  // green
			NewIndexStop(196), // red
		},
		Truecolor: false,
		IsRgb:     false,
	}

	// At 0%: should snap to first stop
	result := GetGradientStop(0, config)
	if result.Fg != "\x1b[38;5;46m" {
		t.Errorf("GetGradientStop(0) 256-color fg = %q, want index 46", result.Fg)
	}

	// At 100%: should snap to last stop
	result = GetGradientStop(100, config)
	if result.Fg != "\x1b[38;5;196m" {
		t.Errorf("GetGradientStop(100) 256-color fg = %q, want index 196", result.Fg)
	}

	// At 50%: rounds to nearest, with 2 stops rounds to index 1
	result = GetGradientStop(50, config)
	if result.Fg != "\x1b[38;5;196m" {
		t.Errorf("GetGradientStop(50) 256-color fg = %q, want nearest stop", result.Fg)
	}
}

func TestGetGradientStop_RgbOn256Terminal(t *testing.T) {
	config := &BarConfig{
		Gradient: []GradientStop{
			NewRgbStop(0, 255, 0),
			NewRgbStop(255, 0, 0),
		},
		Truecolor: false, // 256-color terminal
		IsRgb:     true,  // but gradient is RGB
	}

	// Should convert interpolated RGB to 256-color index
	result := GetGradientStop(0, config)
	if !strings.HasPrefix(result.Fg, "\x1b[38;5;") {
		t.Errorf("RGB on 256-color terminal should produce 38;5 escape, got %q", result.Fg)
	}
}

func TestGetUrgencyColor(t *testing.T) {
	// At urgency 0 (truecolor, dark): should be gray
	fg := GetUrgencyColor(0, true, true)
	if fg != "\x1b[38;2;120;120;120m" {
		t.Errorf("GetUrgencyColor(0, dark, truecolor) = %q, want gray", fg)
	}

	// At urgency 0.5 (truecolor, dark): should be yellow
	fg = GetUrgencyColor(0.5, true, true)
	if fg != "\x1b[38;2;255;200;0m" {
		t.Errorf("GetUrgencyColor(0.5, dark, truecolor) = %q, want yellow", fg)
	}

	// At urgency 1.0 (truecolor, dark): should be red
	fg = GetUrgencyColor(1.0, true, true)
	if fg != "\x1b[38;2;255;0;0m" {
		t.Errorf("GetUrgencyColor(1.0, dark, truecolor) = %q, want red", fg)
	}

	// At urgency 0 (truecolor, light): should be lighter gray
	fg = GetUrgencyColor(0, false, true)
	if fg != "\x1b[38;2;160;160;160m" {
		t.Errorf("GetUrgencyColor(0, light, truecolor) = %q, want light gray", fg)
	}

	// 256-color mode, dark
	fg = GetUrgencyColor(0, true, false)
	if fg != "\x1b[38;5;245m" {
		t.Errorf("GetUrgencyColor(0, dark, 256) = %q, want 245", fg)
	}
	fg = GetUrgencyColor(0.5, true, false)
	if fg != "\x1b[38;5;226m" {
		t.Errorf("GetUrgencyColor(0.5, dark, 256) = %q, want 226", fg)
	}
	fg = GetUrgencyColor(1.0, true, false)
	if fg != "\x1b[38;5;196m" {
		t.Errorf("GetUrgencyColor(1.0, dark, 256) = %q, want 196", fg)
	}

	// 256-color mode, light
	fg = GetUrgencyColor(0, false, false)
	if fg != "\x1b[38;5;249m" {
		t.Errorf("GetUrgencyColor(0, light, 256) = %q, want 249", fg)
	}

	// Clamping
	fg = GetUrgencyColor(2.0, true, true)
	if fg != "\x1b[38;2;255;0;0m" {
		t.Errorf("GetUrgencyColor(2.0) should clamp to 1.0 (red), got %q", fg)
	}
	fg = GetUrgencyColor(-1.0, true, true)
	if fg != "\x1b[38;2;120;120;120m" {
		t.Errorf("GetUrgencyColor(-1.0) should clamp to 0.0 (gray), got %q", fg)
	}
}

func TestHorizontalBar_0Percent(t *testing.T) {
	config := &BarConfig{
		Width:         10,
		EmptyBg:       "\x1b[48;2;48;48;48m",
		Gradient:      []GradientStop{NewRgbStop(0, 255, 0), NewRgbStop(255, 0, 0)},
		Truecolor:     true,
		IsRgb:         true,
		PartialBlocks: true,
	}

	bar := HorizontalBar(0, config, "")
	// At 0%, all cells should be empty (emptyBg + space)
	if !strings.HasSuffix(bar, Reset) {
		t.Error("HorizontalBar should end with Reset escape")
	}
	// Should not contain any block characters
	for _, ch := range HorizontalChars {
		if strings.ContainsRune(bar, ch) {
			t.Errorf("HorizontalBar(0%%) should not contain partial block %c", ch)
		}
	}
	if strings.ContainsRune(bar, FullBlock) {
		t.Error("HorizontalBar(0%) should not contain full block")
	}
}

func TestHorizontalBar_100Percent(t *testing.T) {
	config := &BarConfig{
		Width:         10,
		EmptyBg:       "\x1b[48;2;48;48;48m",
		Gradient:      []GradientStop{NewRgbStop(0, 255, 0), NewRgbStop(255, 0, 0)},
		Truecolor:     true,
		IsRgb:         true,
		PartialBlocks: true,
	}

	bar := HorizontalBar(100, config, "")
	// At 100%, all cells should be filled (gradient bg + space), no partial blocks
	for _, ch := range HorizontalChars {
		if strings.ContainsRune(bar, ch) {
			t.Errorf("HorizontalBar(100%%) should not contain partial block %c", ch)
		}
	}
	// Should contain the gradient bg escape (red at 100%)
	if !strings.Contains(bar, "\x1b[48;2;255;0;0m") {
		t.Errorf("HorizontalBar(100%%) should contain red bg, got %q", bar)
	}
}

func TestHorizontalBar_50Percent(t *testing.T) {
	config := &BarConfig{
		Width:         10,
		EmptyBg:       "\x1b[48;2;48;48;48m",
		Gradient:      []GradientStop{NewRgbStop(0, 255, 0), NewRgbStop(255, 0, 0)},
		Truecolor:     true,
		IsRgb:         true,
		PartialBlocks: true,
	}

	bar := HorizontalBar(50, config, "")
	// Should contain both filled and empty parts
	if !strings.Contains(bar, config.EmptyBg) {
		t.Error("HorizontalBar(50%) should contain empty bg for unfilled cells")
	}
	// The gradient bg at 50% is the midpoint color
	if !strings.Contains(bar, "\x1b[48;2;128;128;0m") {
		t.Errorf("HorizontalBar(50%%) should contain gradient bg at 50%%")
	}
}

func TestHorizontalBar_NoPartialBlocks(t *testing.T) {
	config := &BarConfig{
		Width:         10,
		EmptyBg:       "\x1b[48;2;48;48;48m",
		Gradient:      []GradientStop{NewRgbStop(0, 255, 0), NewRgbStop(255, 0, 0)},
		Truecolor:     true,
		IsRgb:         true,
		PartialBlocks: false,
	}

	bar := HorizontalBar(55, config, "")
	// With partial blocks disabled, should not contain any fractional chars
	for _, ch := range HorizontalChars {
		if strings.ContainsRune(bar, ch) {
			t.Errorf("HorizontalBar with partialBlocks=false should not contain %c", ch)
		}
	}
}

func TestHorizontalBar_EmptyBgOverride(t *testing.T) {
	config := &BarConfig{
		Width:         5,
		EmptyBg:       "\x1b[48;5;236m",
		Gradient:      []GradientStop{NewIndexStop(46), NewIndexStop(196)},
		Truecolor:     false,
		IsRgb:         false,
		PartialBlocks: true,
	}

	override := "\x1b[48;5;240m"
	bar := HorizontalBar(0, config, override)
	if !strings.Contains(bar, override) {
		t.Error("HorizontalBar should use emptyBgOverride when provided")
	}
	if strings.Contains(bar, config.EmptyBg) {
		t.Error("HorizontalBar should not use config.EmptyBg when override is given")
	}
}

func TestVerticalBarCell_Empty(t *testing.T) {
	config := &BarConfig{
		Width:         1,
		EmptyBg:       "\x1b[48;2;48;48;48m",
		Gradient:      []GradientStop{NewRgbStop(0, 255, 0), NewRgbStop(255, 0, 0)},
		Truecolor:     true,
		IsRgb:         true,
		PartialBlocks: true,
	}

	// 0% - all rows should be empty
	for row := 0; row < 3; row++ {
		cell := VerticalBarCell(0, row, 3, "", config)
		if strings.ContainsRune(cell, FullBlock) {
			t.Errorf("VerticalBarCell(0%%, row %d) should not contain full block", row)
		}
		for _, ch := range VerticalChars {
			if strings.ContainsRune(cell, ch) {
				t.Errorf("VerticalBarCell(0%%, row %d) should not contain partial block", row)
			}
		}
	}
}

func TestVerticalBarCell_Full(t *testing.T) {
	config := &BarConfig{
		Width:         1,
		EmptyBg:       "\x1b[48;2;48;48;48m",
		Gradient:      []GradientStop{NewRgbStop(0, 255, 0), NewRgbStop(255, 0, 0)},
		Truecolor:     true,
		IsRgb:         true,
		PartialBlocks: true,
	}

	// 100% - all rows should be full block
	for row := 0; row < 3; row++ {
		cell := VerticalBarCell(100, row, 3, "", config)
		if !strings.ContainsRune(cell, FullBlock) {
			t.Errorf("VerticalBarCell(100%%, row %d) should contain full block, got %q", row, cell)
		}
	}
}

func TestVerticalBarCell_BottomFillsFirst(t *testing.T) {
	config := &BarConfig{
		Width:         1,
		EmptyBg:       "\x1b[48;2;48;48;48m",
		Gradient:      []GradientStop{NewRgbStop(0, 255, 0), NewRgbStop(255, 0, 0)},
		Truecolor:     true,
		IsRgb:         true,
		PartialBlocks: true,
	}

	// ~33% should fill only bottom row (row 2), leaving rows 0,1 empty
	// 33.33% of 24 levels = 8 levels = exactly bottom row full
	cell2 := VerticalBarCell(33.33, 2, 3, "", config)
	if !strings.ContainsRune(cell2, FullBlock) {
		t.Errorf("VerticalBarCell(33.33%%, row 2) should be full, got %q", cell2)
	}

	cell0 := VerticalBarCell(33.33, 0, 3, "", config)
	if strings.ContainsRune(cell0, FullBlock) {
		t.Error("VerticalBarCell(33.33%, row 0) should be empty")
	}
}

func TestVerticalBarCell_PartialFill(t *testing.T) {
	config := &BarConfig{
		Width:         1,
		EmptyBg:       "\x1b[48;2;48;48;48m",
		Gradient:      []GradientStop{NewRgbStop(0, 255, 0), NewRgbStop(255, 0, 0)},
		Truecolor:     true,
		IsRgb:         true,
		PartialBlocks: true,
	}

	// ~12.5% = 3 levels out of 24, bottom row should have partial fill (level 3)
	cell := VerticalBarCell(12.5, 2, 3, "", config)
	hasPartial := false
	for _, ch := range VerticalChars {
		if strings.ContainsRune(cell, ch) {
			hasPartial = true
			break
		}
	}
	if !hasPartial {
		t.Errorf("VerticalBarCell(12.5%%, row 2) should have partial block, got %q", cell)
	}
}

func TestVerticalBarCell_NoPartialBlocks(t *testing.T) {
	config := &BarConfig{
		Width:         1,
		EmptyBg:       "\x1b[48;2;48;48;48m",
		Gradient:      []GradientStop{NewRgbStop(0, 255, 0), NewRgbStop(255, 0, 0)},
		Truecolor:     true,
		IsRgb:         true,
		PartialBlocks: false,
	}

	// With partialBlocks off, cells are either full or empty (no partial chars)
	cell := VerticalBarCell(12.5, 2, 3, "", config)
	for _, ch := range VerticalChars {
		if strings.ContainsRune(cell, ch) {
			t.Errorf("VerticalBarCell with partialBlocks=false should not contain %c", ch)
		}
	}
}

func TestVerticalBarCell_CustomTotalRows(t *testing.T) {
	config := &BarConfig{
		Width:         1,
		EmptyBg:       "\x1b[48;2;48;48;48m",
		Gradient:      []GradientStop{NewRgbStop(0, 255, 0), NewRgbStop(255, 0, 0)},
		Truecolor:     true,
		IsRgb:         true,
		PartialBlocks: true,
	}

	// 7 rows (used in --test-colors), 100% should fill all rows
	for row := 0; row < 7; row++ {
		cell := VerticalBarCell(100, row, 7, "", config)
		if !strings.ContainsRune(cell, FullBlock) {
			t.Errorf("VerticalBarCell(100%%, row %d, totalRows=7) should be full", row)
		}
	}
}

func TestTimeBarCell(t *testing.T) {
	// Set dark mode for deterministic output
	os.Setenv("HOWMUCHLEFT_DARK", "1")
	ResetDarkModeCache()
	defer func() {
		os.Unsetenv("HOWMUCHLEFT_DARK")
		ResetDarkModeCache()
	}()

	config := &BarConfig{
		Width:         1,
		EmptyBg:       "\x1b[48;2;48;48;48m",
		TimeBarBg:     "\x1b[48;2;38;38;38m",
		Gradient:      []GradientStop{NewRgbStop(0, 255, 0), NewRgbStop(255, 0, 0)},
		Truecolor:     true,
		IsRgb:         true,
		PartialBlocks: true,
	}

	// 100% time elapsed: all rows full, urgency = 0 (gray)
	cell := TimeBarCell(100, 50, 2, 3, config)
	if !strings.ContainsRune(cell, FullBlock) {
		t.Errorf("TimeBarCell(100%% time, row 2) should be full, got %q", cell)
	}

	// 0% time elapsed: all rows empty
	cell = TimeBarCell(0, 50, 0, 3, config)
	if strings.ContainsRune(cell, FullBlock) {
		t.Error("TimeBarCell(0% time) should be empty")
	}

	// Uses TimeBarBg not EmptyBg
	cell = TimeBarCell(0, 0, 0, 3, config)
	if !strings.Contains(cell, config.TimeBarBg) {
		t.Errorf("TimeBarCell should use TimeBarBg, got %q", cell)
	}

	// High urgency (usage >> time) should produce red-ish color
	cell = TimeBarCell(50, 90, 2, 3, config)
	// Should contain some fg escape (urgency-driven)
	if !strings.Contains(cell, "\x1b[38;2;") {
		t.Errorf("TimeBarCell with high urgency should contain truecolor fg, got %q", cell)
	}
}

func TestTimeBarCell_UrgencyZeroWhenTimeFull(t *testing.T) {
	os.Setenv("HOWMUCHLEFT_DARK", "1")
	ResetDarkModeCache()
	defer func() {
		os.Unsetenv("HOWMUCHLEFT_DARK")
		ResetDarkModeCache()
	}()

	config := &BarConfig{
		Width:         1,
		EmptyBg:       "\x1b[48;2;48;48;48m",
		TimeBarBg:     "\x1b[48;2;38;38;38m",
		Gradient:      []GradientStop{NewRgbStop(0, 255, 0), NewRgbStop(255, 0, 0)},
		Truecolor:     true,
		IsRgb:         true,
		PartialBlocks: true,
	}

	// When timePercent >= 100, urgency should be 0 (gray color)
	// Bottom row should be full at 100% time
	cell := TimeBarCell(100, 100, 2, 3, config)
	// Urgency 0 in dark truecolor = gray (120,120,120)
	if !strings.Contains(cell, "\x1b[38;2;120;120;120m") {
		t.Errorf("TimeBarCell(time=100) urgency should be 0 (gray), got %q", cell)
	}
}
