package dashboard

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/smm-h/howmuchleft/internal/cache"
	"github.com/smm-h/howmuchleft/internal/config"
	"github.com/smm-h/howmuchleft/internal/oauth"
	"github.com/smm-h/howmuchleft/internal/render"
)

// DiscoverProfiles returns deduplicated profile directories from 3 sources:
// 1. Config file profiles array
// 2. Claudewheel/claudelauncher options.json
// 3. ~/.claude-* directories with .credentials.json
// Always includes ~/.claude as default.
func DiscoverProfiles() []string {
	seen := make(map[string]bool)
	var dirs []string

	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	add := func(dir string) {
		if dir == "" || seen[dir] {
			return
		}
		// Verify it's a directory
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			return
		}
		seen[dir] = true
		dirs = append(dirs, dir)
	}

	// Always include default
	add(filepath.Join(home, ".claude"))

	// Source 1: config file profiles array
	cfg := config.Get()
	for _, p := range cfg.Profiles {
		resolved := resolvePath(p, home)
		add(resolved)
	}

	// Source 2: Claudewheel/claudelauncher options.json
	for _, launcher := range []string{".claudewheel", ".claudelauncher"} {
		optPath := filepath.Join(home, launcher, "options.json")
		data, err := os.ReadFile(optPath)
		if err != nil {
			continue
		}
		var opts struct {
			Profiles []struct {
				Dir string `json:"dir"`
			} `json:"profiles"`
		}
		if json.Unmarshal(data, &opts) == nil {
			for _, p := range opts.Profiles {
				if p.Dir != "" {
					add(resolvePath(p.Dir, home))
				}
			}
		}
	}

	// Source 3: scan ~/.claude-* directories with .credentials.json
	entries, err := os.ReadDir(home)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasPrefix(name, ".claude-") {
				continue
			}
			dir := filepath.Join(home, name)
			credPath := filepath.Join(dir, ".credentials.json")
			if _, err := os.Stat(credPath); err == nil {
				add(dir)
			}
		}
	}

	// Sort alphabetically (stable and predictable)
	sort.Strings(dirs)
	return dirs
}

// ProfileResult holds the rendered output for a single profile.
type ProfileResult struct {
	Dir    string
	Output string
	Err    error
}

// FetchAndRender fetches usage for a profile directory and renders 3 lines.
func FetchAndRender(dir string, barCfg *render.BarConfig) (string, error) {
	// Reset credentials cache so we read fresh for this profile
	oauth.ResetCredentialsCache()

	credFile := oauth.ReadCredentialsFile(dir)
	if credFile == nil || credFile.ClaudeAiOauth == nil {
		return "", fmt.Errorf("no credentials")
	}

	oauthData := credFile.ClaudeAiOauth
	authInfo := oauth.GetAuthInfo(oauthData)

	if !authInfo.IsOAuth {
		return "", fmt.Errorf("not OAuth profile")
	}

	// Fetch usage from cache (don't force refresh)
	usage := cache.GetUsageData(dir, false)

	// Compute profile name and color
	profileName := profileDisplayName(dir)
	isDark := render.IsDarkMode()
	profileColor := render.HueToAnsi(render.HashToHue(dir), isDark)

	// Build the 3 rows
	return renderProfileRows(profileName, profileColor, authInfo.SubscriptionName, usage, barCfg), nil
}

// RenderDashboard fetches all profiles in parallel and returns the complete output.
func RenderDashboard(dirs []string) string {
	cfg := config.Get()
	barCfg := buildDashboardBarConfig(cfg)

	results := make([]ProfileResult, len(dirs))
	var wg sync.WaitGroup

	for i, dir := range dirs {
		wg.Add(1)
		go func(idx int, d string) {
			defer wg.Done()
			output, err := FetchAndRender(d, barCfg)
			results[idx] = ProfileResult{Dir: d, Output: output, Err: err}
		}(i, dir)
	}
	wg.Wait()

	var sections []string
	for _, r := range results {
		if r.Err != nil {
			continue
		}
		sections = append(sections, r.Output)
	}

	if len(sections) == 0 {
		return render.Dim + "No profiles with credentials found." + render.Reset
	}

	return strings.Join(sections, "\n\n")
}

// RunLive runs the dashboard in live mode with periodic refresh.
func RunLive(dirs []string) error {
	// Hide cursor
	fmt.Print("\x1b[?25l")

	// Show cursor on exit
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	cleanup := func() {
		fmt.Print("\x1b[?25h") // show cursor
	}

	go func() {
		<-sigCh
		cleanup()
		os.Exit(0)
	}()

	defer cleanup()

	for {
		// Clear screen and move to top
		fmt.Print("\x1b[2J\x1b[H")

		// Header
		now := time.Now().Format("15:04:05")
		fmt.Printf("%showmuchleft dashboard%s  %s%s%s\n\n",
			render.Bold, render.Reset,
			render.Dim, now, render.Reset)

		// Render all profiles
		output := RenderDashboard(dirs)
		fmt.Println(output)

		// Footer
		fmt.Printf("\n%sRefresh: 30s | Ctrl+C to exit%s", render.Dim, render.Reset)

		time.Sleep(30 * time.Second)
	}
}

// RunOnce renders the dashboard once to stdout.
func RunOnce(dirs []string) error {
	output := RenderDashboard(dirs)
	fmt.Println(output)
	return nil
}

// Run is the exported entry point for the dashboard.
func Run(live bool) error {
	dirs := DiscoverProfiles()
	if len(dirs) == 0 {
		fmt.Println("No profiles found.")
		return nil
	}

	if live {
		return RunLive(dirs)
	}
	return RunOnce(dirs)
}

// --- internal helpers ---

func resolvePath(p string, home string) string {
	if strings.HasPrefix(p, "~/") || p == "~" {
		return filepath.Join(home, p[1:])
	}
	if strings.HasPrefix(p, "~") {
		return filepath.Join(home, p[1:])
	}
	if filepath.IsAbs(p) {
		return p
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

func profileDisplayName(dir string) string {
	base := filepath.Base(dir)
	if base == ".claude" {
		return "default"
	}
	if strings.HasPrefix(base, ".claude-") {
		remainder := strings.TrimPrefix(base, ".claude-")
		if remainder != "" {
			return remainder
		}
	}
	return base
}

func renderProfileRows(name, nameColor, tier string, usage *cache.UsageResult, barCfg *render.BarConfig) string {
	isDark := render.IsDarkMode()
	truecolor := barCfg.Truecolor

	// Compute bar percentages
	var fiveHourPct float64
	var weeklyPct float64
	var showExtra bool
	var extraPct float64

	if usage.FiveHour != nil {
		fiveHourPct = usage.FiveHour.Percent
	}
	if usage.Weekly != nil {
		weeklyPct = *&usage.Weekly.Percent
	}
	if usage.Weekly != nil && usage.Weekly.Percent >= 100 &&
		usage.Extra != nil && usage.Extra.Enabled {
		showExtra = true
		extraPct = usage.Extra.Percent
	}

	thirdPct := weeklyPct
	if showExtra {
		thirdPct = extraPct
	}

	// Warm bg for extra usage bar
	var warmBg string
	if showExtra {
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

	// Bar percents: [5hr, weekly/extra]
	percents := [2]float64{fiveHourPct, thirdPct}

	var lines [3]string
	for row := 0; row < 3; row++ {
		var barStr strings.Builder
		for i := 0; i < 2; i++ {
			if i > 0 {
				barStr.WriteString(render.Reset + " ")
			}
			bgOverride := ""
			if i == 1 && warmBg != "" {
				bgOverride = warmBg
			}
			barStr.WriteString(render.VerticalBarCell(percents[i], row, 3, bgOverride, barCfg))
		}
		barStr.WriteString(render.Reset)

		var text string
		switch row {
		case 0:
			// Profile name + tier
			text = nameColor + name + render.Reset + " " + render.Magenta + tier + render.Reset
		case 1:
			// 5hr percent + reset time
			text = formatUsageLine("5h", usage.FiveHour, usage.Stale)
		case 2:
			// Weekly/extra percent + reset time
			if showExtra {
				text = formatExtraLine(extraPct, usage.Stale, isDark, truecolor) +
					" " + formatResetTime(usage.Weekly)
			} else {
				text = formatUsageLine("7d", usage.Weekly, usage.Stale)
			}
		}

		if text != "" {
			lines[row] = barStr.String() + " " + text
		} else {
			lines[row] = barStr.String()
		}
	}

	return strings.Join(lines[:], "\n")
}

func formatUsageLine(label string, window *cache.WindowResult, stale bool) string {
	if window == nil {
		return render.Gray + label + " ?%" + render.Reset
	}
	pctStr := render.Cyan + fmt.Sprintf("%d%%", int(math.Round(window.Percent))) + render.Reset
	if stale {
		pctStr = render.Dim + fmt.Sprintf("~%d%%", int(math.Round(window.Percent))) + render.Reset
	}
	resetStr := render.FormatTimeRemaining(window.ResetIn)
	if resetStr != "" {
		return label + " " + pctStr + " " + render.Dim + resetStr + render.Reset
	}
	return label + " " + pctStr
}

func formatExtraLine(pct float64, stale bool, isDark bool, truecolor bool) string {
	return render.FormatExtraPercent(pct, stale, isDark, truecolor)
}

func formatResetTime(window *cache.WindowResult) string {
	if window == nil {
		return ""
	}
	resetStr := render.FormatTimeRemaining(window.ResetIn)
	if resetStr != "" {
		return render.Dim + resetStr + render.Reset
	}
	return ""
}

// buildDashboardBarConfig creates a BarConfig for dashboard rendering.
// Uses the same logic as the statusline but without time bars.
func buildDashboardBarConfig(cfg *config.Config) *render.BarConfig {
	isDark := render.IsDarkMode()

	truecolor := false
	switch cfg.ColorMode {
	case "truecolor":
		truecolor = true
	case "256":
		truecolor = false
	default:
		truecolor = render.IsTruecolorSupported()
	}

	// Resolve color entry
	var userEntries []render.ColorEntry
	for _, ce := range cfg.Colors {
		entry := render.ConfigColorToRenderColor(ce)
		if entry != nil {
			userEntries = append(userEntries, *entry)
		}
	}

	userMatch := render.FindColorMatch(userEntries, isDark, truecolor)
	builtinMatch := render.FindColorMatch(render.BuiltinColors, isDark, truecolor)

	var gradient []render.GradientStop
	var isRgb bool
	var bgValue render.BgValue

	if userMatch != nil {
		gradient = userMatch.Gradient
		isRgb = len(gradient) > 0 && gradient[0].IsRgb
		bgValue = userMatch.Bg
	} else if builtinMatch != nil {
		gradient = builtinMatch.Gradient
		isRgb = len(gradient) > 0 && gradient[0].IsRgb
		bgValue = builtinMatch.Bg
	} else {
		if isDark {
			bgValue = render.NewBgIndex(236)
		} else {
			bgValue = render.NewBgIndex(252)
		}
	}

	emptyBg := render.FormatBgFromValue(bgValue, truecolor)

	return &render.BarConfig{
		Width:         cfg.ProgressLength,
		EmptyBg:       emptyBg,
		Gradient:      gradient,
		Truecolor:     truecolor,
		IsRgb:         isRgb,
		PartialBlocks: render.ShouldUsePartialBlocks(cfg.PartialBlocks),
	}
}

