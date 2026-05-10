package render

import "github.com/smm-h/howmuchleft/internal/config"

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
