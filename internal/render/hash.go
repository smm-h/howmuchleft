package render

import (
	"fmt"
	"math"
)

// HashToHue converts a string to a hue value 0-359 using djb2 hash.
func HashToHue(s string) int {
	hash := int32(5381)
	for i := 0; i < len(s); i++ {
		// hash * 33 + byte, using int32 to match JS (hash << 5) + hash | 0
		hash = hash*33 + int32(s[i])
	}
	// Ensure positive modulo
	hue := int(hash % 360)
	if hue < 0 {
		hue += 360
	}
	return hue
}

// HueToAnsi converts a hue (0-359) to an ANSI foreground escape sequence.
// Uses HSL->RGB with saturation 0.7 and lightness adapted to terminal background.
func HueToAnsi(hue int, isDark bool) string {
	s := 0.7
	l := 0.35
	if isDark {
		l = 0.75
	}

	r, g, b := hslToRgb(float64(hue), s, l)

	if IsTruecolorSupported() {
		return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
	}
	idx := RgbTo256(r, g, b)
	return fmt.Sprintf("\x1b[38;5;%dm", idx)
}

// hslToRgb converts HSL to RGB. Hue is 0-359, s and l are 0-1.
func hslToRgb(h, s, l float64) (uint8, uint8, uint8) {
	c := (1 - math.Abs(2*l-1)) * s
	hPrime := math.Mod(h/60.0, 6.0)
	x := c * (1 - math.Abs(math.Mod(hPrime, 2)-1))
	m := l - c/2

	var r1, g1, b1 float64
	switch {
	case h < 60:
		r1, g1, b1 = c, x, 0
	case h < 120:
		r1, g1, b1 = x, c, 0
	case h < 180:
		r1, g1, b1 = 0, c, x
	case h < 240:
		r1, g1, b1 = 0, x, c
	case h < 300:
		r1, g1, b1 = x, 0, c
	default:
		r1, g1, b1 = c, 0, x
	}

	r := uint8(math.Round((r1 + m) * 255))
	g := uint8(math.Round((g1 + m) * 255))
	b := uint8(math.Round((b1 + m) * 255))
	return r, g, b
}
