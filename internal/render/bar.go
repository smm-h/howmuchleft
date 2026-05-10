package render

import (
	"fmt"
	"math"
	"os"
	"strings"
)

// Fractional block characters for sub-cell precision.
// Horizontal: left fractional blocks (U+258F to U+2589), fill left-to-right.
var HorizontalChars = []rune{'▏', '▎', '▍', '▌', '▋', '▊', '▉'}

// Vertical: lower fractional blocks (U+2581 to U+2587), fill bottom-to-top.
var VerticalChars = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇'}

// FullBlock is U+2588, used for fully filled cells.
const FullBlock = '█'

// PartialBlocksBlocklist contains terminal identifiers that don't render
// fractional block characters correctly.
var PartialBlocksBlocklist = []string{"Apple_Terminal", "linux"}

// BarConfig packages the rendering configuration needed by bar functions.
type BarConfig struct {
	Width         int
	EmptyBg       string // Pre-formatted ANSI background escape for empty cells
	Gradient      []GradientStop
	Truecolor     bool
	IsRgb         bool
	PartialBlocks bool
	TimeBarBg     string // Pre-formatted ANSI background escape for time bar empty cells
	Orientation   string // "vertical" (default) or "horizontal"
}

// GradientResult holds the foreground and background ANSI escape sequences
// for a given position in the gradient.
type GradientResult struct {
	Fg string
	Bg string
}

// ShouldUsePartialBlocks determines whether fractional block characters should
// be used. configOverride: "true" forces on, "false" forces off, anything else
// uses auto-detection against the blocklist.
func ShouldUsePartialBlocks(configOverride string) bool {
	switch strings.ToLower(configOverride) {
	case "true":
		return true
	case "false":
		return false
	}

	// Auto-detect: check TERM_PROGRAM and TERM against blocklist
	termProgram := os.Getenv("TERM_PROGRAM")
	term := os.Getenv("TERM")
	for _, blocked := range PartialBlocksBlocklist {
		if termProgram == blocked || term == blocked {
			return false
		}
	}
	return true
}

// GetGradientStop returns fg and bg ANSI escapes for a given percentage (0-100)
// within the configured gradient.
func GetGradientStop(percent float64, config *BarConfig) GradientResult {
	t := math.Max(0, math.Min(1, percent/100))

	if config.IsRgb {
		// Extract RGB values from gradient stops
		rgbStops := make([][3]uint8, len(config.Gradient))
		for i, stop := range config.Gradient {
			rgbStops[i] = stop.Rgb
		}
		rgb := InterpolateRgb(rgbStops, t)
		if config.Truecolor {
			return GradientResult{
				Fg: fmt.Sprintf("\x1b[38;2;%d;%d;%dm", rgb[0], rgb[1], rgb[2]),
				Bg: fmt.Sprintf("\x1b[48;2;%d;%d;%dm", rgb[0], rgb[1], rgb[2]),
			}
		}
		// RGB gradient on 256-color terminal: convert to nearest index
		idx := RgbTo256(rgb[0], rgb[1], rgb[2])
		return GradientResult{
			Fg: fmt.Sprintf("\x1b[38;5;%dm", idx),
			Bg: fmt.Sprintf("\x1b[48;5;%dm", idx),
		}
	}

	// 256-color gradient: snap to nearest stop
	idx := int(math.Round(t * float64(len(config.Gradient)-1)))
	colorIdx := config.Gradient[idx].Index
	return GradientResult{
		Fg: fmt.Sprintf("\x1b[38;5;%dm", colorIdx),
		Bg: fmt.Sprintf("\x1b[48;5;%dm", colorIdx),
	}
}

// GetUrgencyColor computes an ANSI foreground escape for the time bar based on
// urgency ratio (0-1). Uses a 3-stop gradient: gray -> yellow -> red.
// Adapts to dark/light mode and truecolor/256-color.
func GetUrgencyColor(urgency float64, isDark bool, truecolor bool) string {
	u := math.Max(0, math.Min(1, urgency))

	if truecolor {
		// Truecolor: smooth linear interpolation across 3 stops
		var gray, yellow, red [3]float64
		if isDark {
			gray = [3]float64{120, 120, 120}
			yellow = [3]float64{255, 200, 0}
			red = [3]float64{255, 0, 0}
		} else {
			gray = [3]float64{160, 160, 160}
			yellow = [3]float64{200, 160, 0}
			red = [3]float64{200, 0, 0}
		}

		var rgb [3]int
		if u <= 0.5 {
			t := u / 0.5
			rgb[0] = int(math.Round(gray[0] + (yellow[0]-gray[0])*t))
			rgb[1] = int(math.Round(gray[1] + (yellow[1]-gray[1])*t))
			rgb[2] = int(math.Round(gray[2] + (yellow[2]-gray[2])*t))
		} else {
			t := (u - 0.5) / 0.5
			rgb[0] = int(math.Round(yellow[0] + (red[0]-yellow[0])*t))
			rgb[1] = int(math.Round(yellow[1] + (red[1]-yellow[1])*t))
			rgb[2] = int(math.Round(yellow[2] + (red[2]-yellow[2])*t))
		}
		return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", rgb[0], rgb[1], rgb[2])
	}

	// 256-color: snap to nearest of 3 stops
	if u < 0.25 {
		if isDark {
			return "\x1b[38;5;245m"
		}
		return "\x1b[38;5;249m"
	} else if u < 0.75 {
		if isDark {
			return "\x1b[38;5;226m"
		}
		return "\x1b[38;5;178m"
	} else {
		if isDark {
			return "\x1b[38;5;196m"
		}
		return "\x1b[38;5;160m"
	}
}

// HorizontalBar renders a horizontal progress bar with sub-cell precision.
// Filled cells use the gradient bg as background + space. The fractional cell
// uses a left block character with gradient fg on empty bg. Empty cells use
// empty bg + space. If emptyBgOverride is non-empty, it overrides config.EmptyBg.
func HorizontalBar(percent float64, config *BarConfig, emptyBgOverride string) string {
	emptyBg := config.EmptyBg
	if emptyBgOverride != "" {
		emptyBg = emptyBgOverride
	}

	width := config.Width
	if width <= 0 {
		return ""
	}

	clamped := math.Max(0, math.Min(100, percent))
	gr := GetGradientStop(clamped, config)
	fillFrac := clamped / 100.0

	var out strings.Builder
	for i := 0; i < width; i++ {
		cellStart := float64(i) / float64(width)
		cellEnd := float64(i+1) / float64(width)

		if cellEnd <= fillFrac {
			// Fully filled cell
			out.WriteString(gr.Bg)
			out.WriteByte(' ')
		} else if cellStart >= fillFrac {
			// Empty cell
			out.WriteString(emptyBg)
			out.WriteByte(' ')
		} else if config.PartialBlocks {
			// Partial cell with fractional block character
			cellFill := (fillFrac - cellStart) / (1.0 / float64(width))
			idx := int(math.Floor(cellFill * float64(len(HorizontalChars))))
			if idx < 0 {
				idx = 0
			}
			if idx >= len(HorizontalChars) {
				idx = len(HorizontalChars) - 1
			}
			out.WriteString(emptyBg)
			out.WriteString(gr.Fg)
			out.WriteRune(HorizontalChars[idx])
		} else {
			// No partial blocks: round to nearest full cell
			cellMid := (cellStart + cellEnd) / 2.0
			if fillFrac >= cellMid {
				out.WriteString(gr.Bg)
				out.WriteByte(' ')
			} else {
				out.WriteString(emptyBg)
				out.WriteByte(' ')
			}
		}
	}
	out.WriteString(Reset)
	return out.String()
}

// VerticalBarCell renders one cell of a vertical bar.
// Each bar spans totalRows rows (top=0, bottom=totalRows-1), 8 fill states per
// row. Fills bottom-to-top: bottom row fills first.
// If emptyBgOverride is non-empty, it overrides config.EmptyBg.
func VerticalBarCell(percent float64, rowIdx int, totalRows int, emptyBgOverride string, config *BarConfig) string {
	emptyBg := config.EmptyBg
	if emptyBgOverride != "" {
		emptyBg = emptyBgOverride
	}

	clamped := math.Max(0, math.Min(100, percent))
	maxLevel := totalRows * 8
	level := int(math.Round(clamped / 100.0 * float64(maxLevel)))
	gr := GetGradientStop(clamped, config)

	// Each row covers 8 levels; bottom row fills first, top row last
	rowBase := (totalRows - 1 - rowIdx) * 8
	rowLevel := level - rowBase

	if rowLevel >= 8 {
		// Full cell: full block with fg on empty bg
		return emptyBg + gr.Fg + string(FullBlock)
	} else if rowLevel > 0 && config.PartialBlocks {
		// Partial cell: fractional vertical block
		return emptyBg + gr.Fg + string(VerticalChars[rowLevel-1])
	}
	// Empty cell
	return emptyBg + " "
}

// TimeBarCell renders one cell of a vertical time-elapsed bar with urgency
// coloring. Same fill logic as VerticalBarCell but the color is derived from
// urgency (how far usage outpaces elapsed time) rather than the standard
// gradient.
func TimeBarCell(timePercent float64, usagePercent float64, rowIdx int, totalRows int, config *BarConfig) string {
	emptyBg := config.TimeBarBg
	if emptyBg == "" {
		emptyBg = config.EmptyBg
	}

	clamped := math.Max(0, math.Min(100, timePercent))
	maxLevel := totalRows * 8
	level := int(math.Round(clamped / 100.0 * float64(maxLevel)))

	// Urgency: how much usage outpaces elapsed time (0 = fine, 1 = critical)
	var urgency float64
	if timePercent >= 100 {
		urgency = 0
	} else if timePercent > 0 {
		urgency = math.Max(0, math.Min(1, (usagePercent-timePercent)/(100-timePercent)))
	}

	isDark := IsDarkMode()
	fg := GetUrgencyColor(urgency, isDark, config.Truecolor)

	// Each row covers 8 levels; bottom row fills first, top row last
	rowBase := (totalRows - 1 - rowIdx) * 8
	rowLevel := level - rowBase

	if rowLevel >= 8 {
		return emptyBg + fg + string(FullBlock)
	} else if rowLevel > 0 && config.PartialBlocks {
		return emptyBg + fg + string(VerticalChars[rowLevel-1])
	}
	return emptyBg + " "
}
