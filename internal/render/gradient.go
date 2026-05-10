package render

// GradientStop represents a single gradient stop: either an RGB triplet
// (for truecolor) or a 256-color palette index.
type GradientStop struct {
	IsRgb bool
	Rgb   [3]uint8
	Index int
}

// NewRgbStop creates an RGB gradient stop.
func NewRgbStop(r, g, b uint8) GradientStop {
	return GradientStop{IsRgb: true, Rgb: [3]uint8{r, g, b}}
}

// NewIndexStop creates a 256-color palette index gradient stop.
func NewIndexStop(idx int) GradientStop {
	return GradientStop{IsRgb: false, Index: idx}
}

// ColorEntry represents a color configuration with optional conditions.
// nil pointers for DarkMode/TrueColor act as wildcards (match any).
type ColorEntry struct {
	DarkMode  *bool
	TrueColor *bool
	Bg        BgValue
	Gradient  []GradientStop
}

// IsRgbStop validates whether a GradientStop is an RGB stop with values 0-255.
// Since uint8 is inherently 0-255, this just checks the IsRgb flag.
func IsRgbStop(stop GradientStop) bool {
	return stop.IsRgb
}

// FindColorMatch returns the first entry whose conditions match the given
// dark mode and truecolor state. nil condition pointers are wildcards.
func FindColorMatch(entries []ColorEntry, isDark bool, isTruecolor bool) *ColorEntry {
	for i := range entries {
		entry := &entries[i]
		if entry.DarkMode != nil && *entry.DarkMode != isDark {
			continue
		}
		if entry.TrueColor != nil && *entry.TrueColor != isTruecolor {
			continue
		}
		if len(entry.Gradient) == 0 {
			continue
		}
		return entry
	}
	return nil
}

// Helper to create *bool for condition fields.
func boolPtr(v bool) *bool {
	return &v
}

// BuiltinColors contains the 4 default color entries covering
// dark/light x truecolor/256-color combinations.
var BuiltinColors = []ColorEntry{
	// Dark mode, truecolor
	{
		DarkMode:  boolPtr(true),
		TrueColor: boolPtr(true),
		Bg:        NewBgRgb(48, 48, 48),
		Gradient: []GradientStop{
			NewRgbStop(0, 215, 0),
			NewRgbStop(95, 215, 0),
			NewRgbStop(175, 215, 0),
			NewRgbStop(255, 255, 0),
			NewRgbStop(255, 215, 0),
			NewRgbStop(255, 175, 0),
			NewRgbStop(255, 135, 0),
			NewRgbStop(255, 95, 0),
			NewRgbStop(255, 55, 0),
			NewRgbStop(255, 0, 0),
		},
	},
	// Light mode, truecolor
	{
		DarkMode:  boolPtr(false),
		TrueColor: boolPtr(true),
		Bg:        NewBgRgb(208, 208, 208),
		Gradient: []GradientStop{
			NewRgbStop(0, 170, 0),
			NewRgbStop(75, 170, 0),
			NewRgbStop(140, 170, 0),
			NewRgbStop(200, 200, 0),
			NewRgbStop(200, 170, 0),
			NewRgbStop(200, 135, 0),
			NewRgbStop(200, 100, 0),
			NewRgbStop(200, 65, 0),
			NewRgbStop(200, 30, 0),
			NewRgbStop(190, 0, 0),
		},
	},
	// Dark mode, 256-color
	{
		DarkMode:  boolPtr(true),
		TrueColor: boolPtr(false),
		Bg:        NewBgIndex(236),
		Gradient: []GradientStop{
			NewIndexStop(46),
			NewIndexStop(82),
			NewIndexStop(118),
			NewIndexStop(154),
			NewIndexStop(190),
			NewIndexStop(226),
			NewIndexStop(220),
			NewIndexStop(214),
			NewIndexStop(208),
			NewIndexStop(202),
			NewIndexStop(196),
		},
	},
	// Light mode, 256-color
	{
		DarkMode:  boolPtr(false),
		TrueColor: boolPtr(false),
		Bg:        NewBgIndex(252),
		Gradient: []GradientStop{
			NewIndexStop(40),
			NewIndexStop(76),
			NewIndexStop(112),
			NewIndexStop(148),
			NewIndexStop(184),
			NewIndexStop(178),
			NewIndexStop(172),
			NewIndexStop(166),
			NewIndexStop(160),
		},
	},
}
