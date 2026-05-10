package render

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/smm-h/howmuchleft/internal/config"
)

// modelRegex matches Claude model name patterns for shortening.
var modelRegex = regexp.MustCompile(`^claude-(opus|sonnet|haiku)-(\d+)-(\d+)(?:-(\d{1,2}))?(?:-\d{8})?$`)

// ShortenModelName abbreviates a Claude model name to a compact form.
// e.g. "claude-sonnet-4-5-20250514" -> "S4.5"
// Unknown models pass through unchanged.
func ShortenModelName(model string) string {
	m := modelRegex.FindStringSubmatch(model)
	if m == nil {
		return model
	}
	initial := strings.ToUpper(m[1][:1])
	patch := ""
	if m[4] != "" && m[4] != "0" {
		patch = "." + m[4]
	}
	return initial + m[2] + "." + m[3] + patch
}

// FormatPercent formats a usage percentage with ANSI colors.
// nil -> gray "?%", stale -> dim "~{rounded}%", normal -> cyan "{rounded}%".
func FormatPercent(percent *float64, stale bool) string {
	if percent == nil {
		return Gray + "?%" + Reset
	}
	if stale {
		return Dim + fmt.Sprintf("~%d%%", int(math.Round(*percent))) + Reset
	}
	return Cyan + fmt.Sprintf("%d%%", int(math.Round(*percent))) + Reset
}

// FormatExtraPercent formats extra usage percentage with a warm amber background.
func FormatExtraPercent(percent float64, stale bool, isDark bool, truecolor bool) string {
	var warmBg string
	if truecolor {
		if isDark {
			warmBg = "\x1b[48;2;100;60;0m"
		} else {
			warmBg = "\x1b[48;2;255;210;140m"
		}
	} else {
		if isDark {
			warmBg = "\x1b[48;5;94m"
		} else {
			warmBg = "\x1b[48;5;223m"
		}
	}
	var val string
	if stale {
		val = fmt.Sprintf("~%d%%", int(math.Round(percent)))
	} else {
		val = fmt.Sprintf("%d%%", int(math.Round(percent)))
	}
	return warmBg + White + val + Reset
}

// FormatTimeRemaining formats a duration in milliseconds as a human-readable string.
// Negative -> empty string, <60s -> "Ns", <1h -> "Nm", <1d -> "NhNm", else "NdNh".
func FormatTimeRemaining(ms int64) string {
	if ms < 0 {
		return ""
	}
	if ms < 60000 {
		return fmt.Sprintf("%ds", ms/1000)
	}
	if ms < 3600000 {
		return fmt.Sprintf("%dm", ms/60000)
	}
	if ms < 86400000 {
		h := ms / 3600000
		m := (ms % 3600000) / 60000
		return fmt.Sprintf("%dh%dm", h, m)
	}
	d := ms / 86400000
	h := (ms % 86400000) / 3600000
	return fmt.Sprintf("%dd%dh", d, h)
}

// FormatAge formats a duration in milliseconds as a short age string.
// <60s -> "Ns", <1h -> "Nm", else "Nh".
func FormatAge(ms int64) string {
	if ms < 60000 {
		return fmt.Sprintf("%ds", ms/1000)
	}
	if ms < 3600000 {
		return fmt.Sprintf("%dm", ms/60000)
	}
	return fmt.Sprintf("%dh", ms/3600000)
}

// ShortenPath shortens a filesystem path for display.
// Replaces home directory prefix with ~, then truncates to maxLen keeping
// the last depth segments (prepended with .../).
func ShortenPath(p string, maxLen int, depth int) string {
	if p == "" {
		return "~"
	}
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(p, home) {
		p = "~" + p[len(home):]
	}
	if len(p) <= maxLen {
		return p
	}
	parts := strings.Split(p, string(filepath.Separator))
	if len(parts) <= 2 {
		if len(p) > maxLen-3 {
			return "..." + p[len(p)-maxLen+3:]
		}
		return p
	}
	// Keep first segment + "..." + last depth segments
	tail := parts[len(parts)-depth:]
	if depth >= len(parts) {
		tail = parts[1:]
	}
	return parts[0] + "/.../" + strings.Join(tail, "/")
}

// BuildLineText calls each function from the element map in order, filters out
// empty strings, and joins with a single space.
func BuildLineText(elements map[string]func() string, order []string) string {
	var parts []string
	for _, name := range order {
		fn, ok := elements[name]
		if !ok || fn == nil {
			continue
		}
		val := fn()
		if val != "" {
			parts = append(parts, val)
		}
	}
	return strings.Join(parts, " ")
}

// UsageData holds percentage and reset time for a usage window.
type UsageData struct {
	Percent *float64
	ResetIn int64 // milliseconds until reset
}

// ExtraUsageData holds extra usage information.
type ExtraUsageData struct {
	Percent float64
	Enabled bool
}

// GitInfo holds git repository state.
type GitInfo struct {
	Branch  string
	Ahead   int
	Behind  int
	Changes int
	HasGit  bool
}

// LineChangeInfo holds line addition/removal counts.
type LineChangeInfo struct {
	Added   *int
	Removed *int
}

// RenderData contains all data needed to render the 3-line statusline output.
type RenderData struct {
	Context      float64
	Model        string
	Tier         string
	Elapsed      *int64 // milliseconds, nil if unknown
	Profile      string
	ProfileColor string
	GhUser       string
	FiveHour     UsageData
	Weekly       UsageData
	ExtraUsage   *ExtraUsageData
	Stale        bool
	LastSuccessTs int64
	Git          GitInfo
	LineChanges  LineChangeInfo
	Cwd          string
	FiveHourTimePercent *float64
	WeeklyTimePercent   *float64
	CcVersion    string
}

// RenderLines composes the 3-line statusline output from RenderData.
// If lineElements is nil, returns "\n\n" (3 empty lines).
func RenderLines(data *RenderData, barCfg *BarConfig, lineElements *config.LinesConfig) string {
	if lineElements == nil {
		return "\n\n"
	}

	isDark := IsDarkMode()
	truecolor := barCfg.Truecolor

	// Pre-compute shared label values
	elapsedStr := ""
	if data.Elapsed != nil {
		tr := FormatTimeRemaining(*data.Elapsed)
		if tr != "" {
			elapsedStr = Dim + tr + Reset
		}
	}

	profileStr := ""
	if data.Profile != "" {
		color := data.ProfileColor
		if color == "" {
			color = Dim
		}
		profileStr = color + data.Profile + Reset
	}

	ghUserStr := ""
	if data.GhUser != "" {
		color := HueToAnsi(HashToHue(data.GhUser), isDark)
		ghUserStr = color + "@" + data.GhUser + Reset
	}

	ccVersionStr := ""
	if data.CcVersion != "" {
		ccVersionStr = Dim + data.CcVersion + Reset
	}

	ageSuffix := ""
	if data.Stale && data.LastSuccessTs > 0 {
		// Compute age from lastSuccessTs to now
		// Caller should provide the age; we format relative to the stored timestamp.
		// For now, just show the stale indicator since we don't have a clock here.
		ageSuffix = Dim + "(stale)" + Reset
	}

	fiveHourPercent := FormatPercent(data.FiveHour.Percent, data.Stale)
	fiveHourReset := FormatTimeRemaining(data.FiveHour.ResetIn)

	// Git string
	var gitStr string
	if data.Git.HasGit {
		gitStr = Cyan + data.Git.Branch + Reset
		if data.Git.Ahead > 0 {
			gitStr += " " + Magenta + fmt.Sprintf("↑%d", data.Git.Ahead) + Reset
		}
		if data.Git.Behind > 0 {
			gitStr += " " + Magenta + fmt.Sprintf("↓%d", data.Git.Behind) + Reset
		}
		if data.Git.Changes > 0 {
			gitStr += " " + Yellow + fmt.Sprintf("+%d", data.Git.Changes) + Reset
		}
	} else {
		gitStr = Gray + "no .git" + Reset
	}

	// Line changes
	linesStr := ""
	if data.LineChanges.Added != nil || data.LineChanges.Removed != nil {
		added := 0
		removed := 0
		if data.LineChanges.Added != nil {
			added = *data.LineChanges.Added
		}
		if data.LineChanges.Removed != nil {
			removed = *data.LineChanges.Removed
		}
		linesStr = Green + fmt.Sprintf("+%d", added) + Reset + "/" + Red + fmt.Sprintf("-%d", removed) + Reset
	}

	weeklyPercent := FormatPercent(data.Weekly.Percent, data.Stale)
	weeklyReset := FormatTimeRemaining(data.Weekly.ResetIn)

	// Determine if 3rd bar should show extra usage
	showExtraUsage := data.Weekly.Percent != nil && *data.Weekly.Percent >= 100 &&
		data.ExtraUsage != nil && data.ExtraUsage.Enabled

	// Build element maps for each line
	line1Elements := map[string]func() string{
		"context": func() string {
			return Cyan + fmt.Sprintf("%d%%", int(math.Round(data.Context))) + Reset
		},
		"elapsed": func() string { return elapsedStr },
		"profile": func() string { return profileStr },
		"tier":    func() string { return Magenta + data.Tier + Reset },
		"model":   func() string { return White + ShortenModelName(data.Model) + Reset },
		"version": func() string { return ccVersionStr },
	}

	line2Elements := map[string]func() string{
		"usage5h":   func() string { return fiveHourPercent },
		"staleness": func() string { return ageSuffix },
		"age":       func() string { return Dim + fiveHourReset + Reset },
		"ghUser":    func() string { return ghUserStr },
		"branch": func() string {
			if linesStr != "" {
				return gitStr + " " + linesStr
			}
			return gitStr
		},
	}

	line3Elements := map[string]func() string{
		"usageWeekly": func() string {
			if showExtraUsage {
				return FormatExtraPercent(data.ExtraUsage.Percent, data.Stale, isDark, truecolor)
			}
			return weeklyPercent
		},
		"staleness": func() string { return ageSuffix },
		"age":       func() string { return Dim + weeklyReset + Reset },
		"cwd":       func() string { return White + data.Cwd + Reset },
	}

	// Determine warm bg for extra usage bar
	var warmBg string
	if showExtraUsage {
		if truecolor {
			if isDark {
				warmBg = "\x1b[48;2;80;50;0m"
			} else {
				warmBg = "\x1b[48;2;255;220;160m"
			}
		} else {
			if isDark {
				warmBg = "\x1b[48;5;94m"
			} else {
				warmBg = "\x1b[48;5;223m"
			}
		}
	}

	// Determine third bar percent
	var thirdPercent float64
	if showExtraUsage {
		thirdPercent = data.ExtraUsage.Percent
	} else if data.Weekly.Percent != nil {
		thirdPercent = *data.Weekly.Percent
	}

	// Show time bars when enabled and time data is available
	showTimeBars := barCfg.TimeBarBg != "" && data.FiveHourTimePercent != nil

	var lines [3]string

	orientation := "vertical"
	// Check orientation from the bar config width - if EmptyBg is set we use vertical by default
	// The orientation is determined by the caller; for now we check barCfg fields

	if barCfg.Width <= 0 {
		// No bars, just text
		lines[0] = BuildLineText(line1Elements, lineElements.Line1)
		lines[1] = BuildLineText(line2Elements, lineElements.Line2)
		lines[2] = BuildLineText(line3Elements, lineElements.Line3)
		return strings.Join(lines[:], "\n")
	}

	// Determine orientation from BarConfig
	// BarConfig doesn't carry orientation, so we use a heuristic:
	// If orientation field exists we'd use it. Since BarConfig has no orientation field,
	// we default to vertical. The caller can set Width=0 to get horizontal behavior,
	// or we extend BarConfig. For now, use the Orientation field we'll add.
	_ = orientation

	// Vertical orientation: 3 bars (context, 5hr, weekly/extra) as columns spanning 3 rows
	fiveHourPct := float64(0)
	if data.FiveHour.Percent != nil {
		fiveHourPct = *data.FiveHour.Percent
	}
	percents := [3]float64{data.Context, fiveHourPct, thirdPercent}

	for row := 0; row < 3; row++ {
		var barStr strings.Builder
		for i := 0; i < 3; i++ {
			if i > 0 {
				barStr.WriteString(Reset + " ")
			}
			bgOverride := ""
			if i == 2 && warmBg != "" {
				bgOverride = warmBg
			}
			barStr.WriteString(VerticalBarCell(percents[i], row, 3, bgOverride, barCfg))

			// Time bars after 5hr (i=1) and weekly (i=2)
			if showTimeBars && (i == 1 || i == 2) {
				var timePct float64
				var usagePct float64
				if i == 1 && data.FiveHourTimePercent != nil {
					timePct = *data.FiveHourTimePercent
					usagePct = fiveHourPct
				} else if i == 2 && data.WeeklyTimePercent != nil {
					timePct = *data.WeeklyTimePercent
					if showExtraUsage {
						usagePct = data.ExtraUsage.Percent
					} else {
						usagePct = thirdPercent
					}
				}
				barStr.WriteString(TimeBarCell(timePct, usagePct, row, 3, barCfg))
			}
		}
		barStr.WriteString(Reset)

		var text string
		switch row {
		case 0:
			text = BuildLineText(line1Elements, lineElements.Line1)
		case 1:
			text = BuildLineText(line2Elements, lineElements.Line2)
		case 2:
			text = BuildLineText(line3Elements, lineElements.Line3)
		}

		if text != "" {
			lines[row] = barStr.String() + " " + text
		} else {
			lines[row] = barStr.String()
		}
	}

	return strings.Join(lines[:], "\n")
}
