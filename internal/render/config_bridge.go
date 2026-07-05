package render

import (
	"github.com/smm-h/howmuchleft/internal/config"
)

// ConfigColorToRenderColor converts a config.ColorEntry to a render.ColorEntry.
// Returns nil if the entry has no valid gradient.
func ConfigColorToRenderColor(ce config.ColorEntry) *ColorEntry {
	entry := &ColorEntry{
		DarkMode:  ce.DarkMode,
		TrueColor: ce.TrueColor,
	}

	entry.Gradient = ParseGradientStops(ce.Gradient)
	if len(entry.Gradient) == 0 {
		return nil
	}

	entry.Bg = ParseBgValue(ce.Bg)
	return entry
}

// ParseGradientStops converts the generic gradient interface to typed stops.
func ParseGradientStops(g interface{}) []GradientStop {
	arr, ok := g.([]interface{})
	if !ok {
		return nil
	}

	var stops []GradientStop
	for _, item := range arr {
		switch v := item.(type) {
		case []interface{}:
			// RGB triplet [R, G, B]
			if len(v) == 3 {
				r, g, b := toUint8(v[0]), toUint8(v[1]), toUint8(v[2])
				stops = append(stops, NewRgbStop(r, g, b))
			}
		case float64:
			// 256-color index
			stops = append(stops, NewIndexStop(int(v)))
		case int64:
			// 256-color index (TOML integers are int64)
			stops = append(stops, NewIndexStop(int(v)))
		case int:
			// 256-color index
			stops = append(stops, NewIndexStop(v))
		}
	}
	return stops
}

// ParseBgValue converts the generic bg interface to a BgValue.
func ParseBgValue(bg interface{}) BgValue {
	switch v := bg.(type) {
	case []interface{}:
		if len(v) == 3 {
			return NewBgRgb(toUint8(v[0]), toUint8(v[1]), toUint8(v[2]))
		}
	case float64:
		return NewBgIndex(int(v))
	case int64:
		return NewBgIndex(int(v))
	case int:
		return NewBgIndex(v)
	}
	return BgValue{}
}

func toUint8(v interface{}) uint8 {
	switch n := v.(type) {
	case float64:
		if n < 0 {
			return 0
		}
		if n > 255 {
			return 255
		}
		return uint8(n)
	case int64:
		if n < 0 {
			return 0
		}
		if n > 255 {
			return 255
		}
		return uint8(n)
	case int:
		if n < 0 {
			return 0
		}
		if n > 255 {
			return 255
		}
		return uint8(n)
	}
	return 0
}

// BuildBarConfig creates a BarConfig from the user's config. It resolves color
// mode, selects user or builtin gradients, computes time bar background, and
// sets orientation. This is the single authoritative source for bar
// configuration used by the statusline, demo, dashboard, and colors commands.
func BuildBarConfig(cfg *config.Config) *BarConfig {
	isDark := IsDarkMode()

	// Determine truecolor mode
	truecolor := false
	switch cfg.ColorMode {
	case "truecolor":
		truecolor = true
	case "256":
		truecolor = false
	default: // "auto"
		truecolor = IsTruecolorSupported()
	}

	// Resolve color entry: user config colors first, then builtins
	var userEntries []ColorEntry
	for _, ce := range cfg.Colors {
		entry := ConfigColorToRenderColor(ce)
		if entry != nil {
			userEntries = append(userEntries, *entry)
		}
	}

	userMatch := FindColorMatch(userEntries, isDark, truecolor)
	builtinMatch := FindColorMatch(BuiltinColors, isDark, truecolor)

	var gradient []GradientStop
	var isRgb bool
	var bgValue BgValue

	if userMatch != nil {
		gradient = userMatch.Gradient
		isRgb = len(gradient) > 0 && gradient[0].IsRgb
		bgValue = userMatch.Bg
	} else if builtinMatch != nil {
		gradient = builtinMatch.Gradient
		isRgb = len(gradient) > 0 && gradient[0].IsRgb
		bgValue = builtinMatch.Bg
	} else {
		// Hardcoded fallback
		if isDark {
			bgValue = NewBgIndex(236)
		} else {
			bgValue = NewBgIndex(252)
		}
	}

	emptyBg := FormatBgFromValue(bgValue, truecolor)

	// Compute time bar bg
	showTimeBars := cfg.ShowTimeBars != nil && *cfg.ShowTimeBars
	timeBarDim := 0.25
	if cfg.TimeBarDim != nil {
		timeBarDim = *cfg.TimeBarDim
	}

	var timeBarBg string
	if showTimeBars {
		timeBarBg = ComputeTimeBarBg(bgValue, isDark, truecolor, timeBarDim)
	}

	return &BarConfig{
		Width:         cfg.ProgressLength,
		EmptyBg:       emptyBg,
		Gradient:      gradient,
		Truecolor:     truecolor,
		IsRgb:         isRgb,
		PartialBlocks: ShouldUsePartialBlocks(cfg.PartialBlocks),
		TimeBarBg:     timeBarBg,
		Orientation:   cfg.ProgressBarOrientation,
		IsDark:        isDark,
	}
}
