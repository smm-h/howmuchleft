package render

import (
	"fmt"
	"strings"
)

// TestColors returns a multi-line string previewing the gradient colors.
// Shows sample bars at 7 percentages, vertical bar columns, and a gradient strip.
func TestColors(barCfg *BarConfig) string {
	var out strings.Builder

	isDark := IsDarkMode()
	tc := barCfg.Truecolor
	partialStr := "off"
	if barCfg.PartialBlocks {
		partialStr = "on"
	}

	out.WriteString(fmt.Sprintf(
		"Mode: %s, Color: %s, Width: %d, Partial blocks: %s\n\n",
		modeLabel(isDark), colorLabel(tc), barCfg.Width, partialStr,
	))

	// Sample bars at 7 percentages with vertical columns spanning 7 rows
	pcts := []float64{5, 20, 35, 50, 65, 80, 95}
	totalRows := len(pcts)

	for rowIdx, pct := range pcts {
		// Horizontal bar at this percentage
		bar := HorizontalBar(pct, barCfg, "")
		label := fmt.Sprintf("%2.0f%%", pct)

		line := bar + " " + label + "  "

		// Vertical bar columns: one pair per percentage
		for _, vp := range pcts {
			cell := VerticalBarCell(vp, rowIdx, totalRows, "", barCfg)
			line += cell + cell
		}
		line += Reset

		out.WriteString(line + "\n")
	}

	out.WriteString("\n")

	// Continuous gradient strip (40 cells, each colored by position)
	stripWidth := 40
	var strip strings.Builder
	for i := 0; i < stripWidth; i++ {
		pct := float64(i) / float64(stripWidth-1) * 100
		gr := GetGradientStop(pct, barCfg)
		strip.WriteString(gr.Bg + " ")
	}
	strip.WriteString(Reset)
	out.WriteString(strip.String() + "\n")

	return out.String()
}

func modeLabel(isDark bool) string {
	if isDark {
		return "dark"
	}
	return "light"
}

func colorLabel(truecolor bool) string {
	if truecolor {
		return "truecolor"
	}
	return "256-color"
}
