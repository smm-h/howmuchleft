package render

import (
	"os"
	"strings"
	"testing"
)

func TestRgbTo256(t *testing.T) {
	tests := []struct {
		r, g, b uint8
		want    int
	}{
		{0, 0, 0, 16},         // black -> cube origin
		{255, 255, 255, 231},  // white -> cube max
		{255, 0, 0, 196},      // pure red
		{0, 255, 0, 46},       // pure green
		{0, 0, 255, 21},       // pure blue
		{255, 215, 0, 220},    // gold-ish
		{128, 128, 128, 145},  // mid-gray -> rounds to 3,3,3 in 6x6x6 cube
	}

	for _, tt := range tests {
		got := RgbTo256(tt.r, tt.g, tt.b)
		if got != tt.want {
			t.Errorf("RgbTo256(%d, %d, %d) = %d, want %d", tt.r, tt.g, tt.b, got, tt.want)
		}
	}
}

func TestInterpolateRgb(t *testing.T) {
	stops := [][3]uint8{{0, 0, 0}, {255, 255, 255}}

	// At 0%
	got := InterpolateRgb(stops, 0.0)
	if got != [3]uint8{0, 0, 0} {
		t.Errorf("InterpolateRgb at 0%% = %v, want [0,0,0]", got)
	}

	// At 50%
	got = InterpolateRgb(stops, 0.5)
	if got != [3]uint8{128, 128, 128} {
		t.Errorf("InterpolateRgb at 50%% = %v, want [128,128,128]", got)
	}

	// At 100%
	got = InterpolateRgb(stops, 1.0)
	if got != [3]uint8{255, 255, 255} {
		t.Errorf("InterpolateRgb at 100%% = %v, want [255,255,255]", got)
	}

	// Multi-stop: 3 stops
	stops3 := [][3]uint8{{255, 0, 0}, {0, 255, 0}, {0, 0, 255}}
	got = InterpolateRgb(stops3, 0.0)
	if got != [3]uint8{255, 0, 0} {
		t.Errorf("InterpolateRgb 3-stop at 0%% = %v, want [255,0,0]", got)
	}
	got = InterpolateRgb(stops3, 0.5)
	if got != [3]uint8{0, 255, 0} {
		t.Errorf("InterpolateRgb 3-stop at 50%% = %v, want [0,255,0]", got)
	}
	got = InterpolateRgb(stops3, 1.0)
	if got != [3]uint8{0, 0, 255} {
		t.Errorf("InterpolateRgb 3-stop at 100%% = %v, want [0,0,255]", got)
	}
	got = InterpolateRgb(stops3, 0.25)
	if got != [3]uint8{128, 128, 0} {
		t.Errorf("InterpolateRgb 3-stop at 25%% = %v, want [128,128,0]", got)
	}
}

func TestHashToHue(t *testing.T) {
	// Consistency: same input always produces same output
	h1 := HashToHue("hello")
	h2 := HashToHue("hello")
	if h1 != h2 {
		t.Errorf("HashToHue not consistent: %d != %d", h1, h2)
	}

	// Different inputs produce different hues (probabilistically)
	h3 := HashToHue("world")
	if h1 == h3 {
		t.Logf("Warning: 'hello' and 'world' hash to same hue %d (unlikely but possible)", h1)
	}

	// Result is in range 0-359
	inputs := []string{"", "a", "test", "foo/bar/baz", "/home/user/.claude"}
	for _, s := range inputs {
		h := HashToHue(s)
		if h < 0 || h > 359 {
			t.Errorf("HashToHue(%q) = %d, out of range [0,359]", s, h)
		}
	}

	// Verify specific known value (computed manually):
	// djb2("a") = (5381*33 + 97) = 177670 -> 177670 % 360 = 190
	ha := HashToHue("a")
	if ha != 190 {
		t.Errorf("HashToHue(\"a\") = %d, want 190", ha)
	}
}

func TestHueToAnsi(t *testing.T) {
	// Set truecolor mode for deterministic output
	os.Setenv("COLORTERM", "truecolor")
	ResetTruecolorCache()
	defer func() {
		os.Unsetenv("COLORTERM")
		ResetTruecolorCache()
	}()

	// Dark mode
	result := HueToAnsi(0, true)
	if !strings.HasPrefix(result, "\x1b[38;2;") || !strings.HasSuffix(result, "m") {
		t.Errorf("HueToAnsi(0, true) = %q, not a valid truecolor escape", result)
	}

	// Light mode
	result = HueToAnsi(120, false)
	if !strings.HasPrefix(result, "\x1b[38;2;") || !strings.HasSuffix(result, "m") {
		t.Errorf("HueToAnsi(120, false) = %q, not a valid truecolor escape", result)
	}

	// 256-color fallback
	os.Setenv("COLORTERM", "")
	ResetTruecolorCache()
	result = HueToAnsi(240, true)
	if !strings.HasPrefix(result, "\x1b[38;5;") || !strings.HasSuffix(result, "m") {
		t.Errorf("HueToAnsi(240, true) 256-color = %q, not a valid 256-color escape", result)
	}
}

func TestIsTruecolorSupported(t *testing.T) {
	// Test with "truecolor"
	os.Setenv("COLORTERM", "truecolor")
	ResetTruecolorCache()
	if !IsTruecolorSupported() {
		t.Error("IsTruecolorSupported() = false with COLORTERM=truecolor")
	}

	// Test with "24bit"
	os.Setenv("COLORTERM", "24bit")
	ResetTruecolorCache()
	if !IsTruecolorSupported() {
		t.Error("IsTruecolorSupported() = false with COLORTERM=24bit")
	}

	// Test with empty/other value
	os.Setenv("COLORTERM", "256color")
	ResetTruecolorCache()
	if IsTruecolorSupported() {
		t.Error("IsTruecolorSupported() = true with COLORTERM=256color")
	}

	// Test with unset
	os.Unsetenv("COLORTERM")
	ResetTruecolorCache()
	if IsTruecolorSupported() {
		t.Error("IsTruecolorSupported() = true with COLORTERM unset")
	}
}

func TestFindColorMatch(t *testing.T) {
	// Should find dark+truecolor entry
	entry := FindColorMatch(BuiltinColors, true, true)
	if entry == nil {
		t.Fatal("FindColorMatch(dark, truecolor) returned nil")
	}
	if entry.DarkMode == nil || !*entry.DarkMode {
		t.Error("Expected dark mode entry")
	}
	if entry.TrueColor == nil || !*entry.TrueColor {
		t.Error("Expected truecolor entry")
	}

	// Should find light+256color entry
	entry = FindColorMatch(BuiltinColors, false, false)
	if entry == nil {
		t.Fatal("FindColorMatch(light, 256color) returned nil")
	}
	if entry.DarkMode == nil || *entry.DarkMode {
		t.Error("Expected light mode entry")
	}
	if entry.TrueColor == nil || *entry.TrueColor {
		t.Error("Expected 256-color entry")
	}

	// Wildcard: nil conditions should match anything
	wildcard := []ColorEntry{{
		DarkMode:  nil,
		TrueColor: nil,
		Bg:        NewBgIndex(0),
		Gradient:  []GradientStop{NewIndexStop(196)},
	}}
	entry = FindColorMatch(wildcard, true, true)
	if entry == nil {
		t.Error("Wildcard entry should match any conditions")
	}
	entry = FindColorMatch(wildcard, false, false)
	if entry == nil {
		t.Error("Wildcard entry should match any conditions")
	}

	// Empty gradient should be skipped
	emptyGrad := []ColorEntry{{
		DarkMode:  nil,
		TrueColor: nil,
		Bg:        NewBgIndex(0),
		Gradient:  nil,
	}}
	entry = FindColorMatch(emptyGrad, true, true)
	if entry != nil {
		t.Error("Entry with empty gradient should be skipped")
	}

	// No match
	onlyDark := []ColorEntry{{
		DarkMode:  boolPtr(true),
		TrueColor: boolPtr(true),
		Bg:        NewBgRgb(48, 48, 48),
		Gradient:  []GradientStop{NewRgbStop(255, 0, 0)},
	}}
	entry = FindColorMatch(onlyDark, false, true)
	if entry != nil {
		t.Error("Should not match light mode request against dark-only entry")
	}
}

func TestFormatFg(t *testing.T) {
	tc := FormatFg(255, 128, 0, true)
	if tc != "\x1b[38;2;255;128;0m" {
		t.Errorf("FormatFg truecolor = %q, want \\x1b[38;2;255;128;0m", tc)
	}

	fc := FormatFg(255, 128, 0, false)
	if !strings.HasPrefix(fc, "\x1b[38;5;") {
		t.Errorf("FormatFg 256-color = %q, should start with \\x1b[38;5;", fc)
	}
}

func TestFormatBg(t *testing.T) {
	tc := FormatBg(48, 48, 48, true)
	if tc != "\x1b[48;2;48;48;48m" {
		t.Errorf("FormatBg truecolor = %q, want \\x1b[48;2;48;48;48m", tc)
	}

	fc := FormatBg(48, 48, 48, false)
	if !strings.HasPrefix(fc, "\x1b[48;5;") {
		t.Errorf("FormatBg 256-color = %q, should start with \\x1b[48;5;", fc)
	}
}

func TestFormatBgFromValue(t *testing.T) {
	// RGB background
	rgb := NewBgRgb(208, 208, 208)
	got := FormatBgFromValue(rgb, true)
	if got != "\x1b[48;2;208;208;208m" {
		t.Errorf("FormatBgFromValue RGB truecolor = %q", got)
	}

	// Index background
	idx := NewBgIndex(236)
	got = FormatBgFromValue(idx, true)
	if got != "\x1b[48;5;236m" {
		t.Errorf("FormatBgFromValue index = %q, want \\x1b[48;5;236m", got)
	}

	// RGB background in 256-color mode
	got = FormatBgFromValue(rgb, false)
	if !strings.HasPrefix(got, "\x1b[48;5;") {
		t.Errorf("FormatBgFromValue RGB 256-color = %q, should use 48;5 format", got)
	}
}
