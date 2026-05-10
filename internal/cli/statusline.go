package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sync"
	"time"

	"github.com/smm-h/howmuchleft/internal/cache"
	"github.com/smm-h/howmuchleft/internal/config"
	"github.com/smm-h/howmuchleft/internal/git"
	"github.com/smm-h/howmuchleft/internal/oauth"
	"github.com/smm-h/howmuchleft/internal/platform"
	"github.com/smm-h/howmuchleft/internal/render"
)

// Time window durations in milliseconds.
const (
	fiveHourMs = 5 * 60 * 60 * 1000
	sevenDayMs = 7 * 24 * 60 * 60 * 1000
)

// stdinData represents the JSON structure piped from Claude Code.
type stdinData struct {
	Model         interface{}            `json:"model"`
	ContextWindow map[string]interface{} `json:"context_window"`
	Cwd           string                 `json:"cwd"`
	Cost          map[string]interface{} `json:"cost"`
	RateLimits    map[string]interface{} `json:"rate_limits"`
	Workspace     map[string]interface{} `json:"workspace"`
}

// parseStdin reads all of stdin and returns parsed stdinData.
// Falls back to empty struct on any parse error.
func parseStdin(r io.Reader) stdinData {
	raw, err := io.ReadAll(r)
	if err != nil || len(raw) == 0 {
		return stdinData{}
	}

	var data stdinData
	if err := json.Unmarshal(raw, &data); err != nil {
		fmt.Fprintf(os.Stderr, "howmuchleft: warning: failed to parse stdin JSON: %v\n", err)
		return stdinData{}
	}
	return data
}

// extractModel resolves model from stdin data.
// Priority: string > display_name > id > "?"
func extractModel(model interface{}) string {
	if model == nil {
		return "?"
	}

	// Direct string
	if s, ok := model.(string); ok && s != "" {
		return s
	}

	// Object with display_name or id
	if m, ok := model.(map[string]interface{}); ok {
		if dn, ok := m["display_name"].(string); ok && dn != "" {
			return dn
		}
		if id, ok := m["id"].(string); ok && id != "" {
			return id
		}
	}

	return "?"
}

// extractContextPercent gets context_window.used_percentage from stdin data.
func extractContextPercent(cw map[string]interface{}) float64 {
	if cw == nil {
		return 0
	}
	if p, ok := cw["used_percentage"].(float64); ok {
		return p
	}
	return 0
}

// extractCwd resolves cwd from stdin data.
func extractCwd(data stdinData) string {
	if data.Cwd != "" {
		return data.Cwd
	}
	if data.Workspace != nil {
		if dir, ok := data.Workspace["current_dir"].(string); ok && dir != "" {
			return dir
		}
	}
	cwd, _ := os.Getwd()
	return cwd
}

// hasStdinUsage checks if rate_limits has five_hour.used_percentage.
func hasStdinUsage(rateLimits map[string]interface{}) bool {
	if rateLimits == nil {
		return false
	}
	fh, ok := rateLimits["five_hour"].(map[string]interface{})
	if !ok {
		return false
	}
	_, ok = fh["used_percentage"].(float64)
	return ok
}

// usageFromStdinRateLimits builds a UsageResult directly from stdin rate_limits.
func usageFromStdinRateLimits(rateLimits map[string]interface{}) *cache.UsageResult {
	now := time.Now().UnixMilli()
	result := &cache.UsageResult{
		Stale:         false,
		LastSuccessTs: now,
	}

	if fh, ok := rateLimits["five_hour"].(map[string]interface{}); ok {
		wr := &cache.WindowResult{}
		if p, ok := fh["used_percentage"].(float64); ok {
			wr.Percent = p
		}
		if ra, ok := fh["resets_at"].(float64); ok && ra > 0 {
			resetAtMs := int64(ra) * 1000
			resetIn := resetAtMs - now
			if resetIn < 0 {
				resetIn = 0
			}
			wr.ResetIn = resetIn
		}
		result.FiveHour = wr
	}

	if sd, ok := rateLimits["seven_day"].(map[string]interface{}); ok {
		wr := &cache.WindowResult{}
		if p, ok := sd["used_percentage"].(float64); ok {
			wr.Percent = p
		}
		if ra, ok := sd["resets_at"].(float64); ok && ra > 0 {
			resetAtMs := int64(ra) * 1000
			resetIn := resetAtMs - now
			if resetIn < 0 {
				resetIn = 0
			}
			wr.ResetIn = resetIn
		}
		result.Weekly = wr
	}

	if eu, ok := rateLimits["extra_usage"].(map[string]interface{}); ok {
		er := &cache.ExtraResult{}
		if enabled, ok := eu["is_enabled"].(bool); ok {
			er.Enabled = enabled
		}
		if p, ok := eu["utilization"].(float64); ok {
			er.Percent = p
		}
		result.Extra = er
	}

	return result
}

// buildBarConfig creates a render.BarConfig from the loaded config.
func buildBarConfig(cfg *config.Config) *render.BarConfig {
	isDark := render.IsDarkMode()

	// Determine truecolor mode
	truecolor := false
	switch cfg.ColorMode {
	case "truecolor":
		truecolor = true
	case "256":
		truecolor = false
	default: // "auto"
		truecolor = render.IsTruecolorSupported()
	}

	// Resolve color entry: user config colors first, then builtins
	var userEntries []render.ColorEntry
	for _, ce := range cfg.Colors {
		entry := configColorToRenderColor(ce)
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
		// Hardcoded fallback
		if isDark {
			bgValue = render.NewBgIndex(236)
		} else {
			bgValue = render.NewBgIndex(252)
		}
	}

	emptyBg := render.FormatBgFromValue(bgValue, truecolor)

	// Compute time bar bg
	showTimeBars := cfg.ShowTimeBars != nil && *cfg.ShowTimeBars
	timeBarDim := 0.25
	if cfg.TimeBarDim != nil {
		timeBarDim = *cfg.TimeBarDim
	}

	var timeBarBg string
	if showTimeBars {
		timeBarBg = computeTimeBarBg(bgValue, isDark, truecolor, timeBarDim)
	}

	return &render.BarConfig{
		Width:         cfg.ProgressLength,
		EmptyBg:       emptyBg,
		Gradient:      gradient,
		Truecolor:     truecolor,
		IsRgb:         isRgb,
		PartialBlocks: render.ShouldUsePartialBlocks(cfg.PartialBlocks),
		TimeBarBg:     timeBarBg,
	}
}

// computeTimeBarBg blends the bar background toward the terminal default.
// Dark terminals default to black (0,0,0), light to white (255,255,255).
// blend: 0 = same as bar bg, 1 = fully terminal default.
func computeTimeBarBg(bg render.BgValue, isDark bool, truecolor bool, blend float64) string {
	termDefault := float64(0)
	if !isDark {
		termDefault = 255
	}

	if bg.IsRgb {
		r := uint8(math.Round(float64(bg.Rgb[0]) + (termDefault-float64(bg.Rgb[0]))*blend))
		g := uint8(math.Round(float64(bg.Rgb[1]) + (termDefault-float64(bg.Rgb[1]))*blend))
		b := uint8(math.Round(float64(bg.Rgb[2]) + (termDefault-float64(bg.Rgb[2]))*blend))
		return render.FormatBg(r, g, b, truecolor)
	}

	// 256-color: grayscale shift toward terminal default end
	base := float64(bg.Index)
	target := float64(232)
	if !isDark {
		target = 255
	}
	mid := int(math.Round(base + (target-base)*blend))
	return fmt.Sprintf("\x1b[48;5;%dm", mid)
}

// configColorToRenderColor converts a config.ColorEntry to a render.ColorEntry.
// Returns nil if the entry has no valid gradient.
func configColorToRenderColor(ce config.ColorEntry) *render.ColorEntry {
	entry := &render.ColorEntry{
		DarkMode:  ce.DarkMode,
		TrueColor: ce.TrueColor,
	}

	// Parse gradient
	entry.Gradient = parseGradientStops(ce.Gradient)
	if len(entry.Gradient) == 0 {
		return nil
	}

	// Parse bg
	entry.Bg = parseBgValue(ce.Bg)

	return entry
}

// parseGradientStops converts the generic gradient interface to typed stops.
func parseGradientStops(g interface{}) []render.GradientStop {
	arr, ok := g.([]interface{})
	if !ok {
		return nil
	}

	var stops []render.GradientStop
	for _, item := range arr {
		switch v := item.(type) {
		case []interface{}:
			// RGB triplet [R, G, B]
			if len(v) == 3 {
				r, g, b := toUint8(v[0]), toUint8(v[1]), toUint8(v[2])
				stops = append(stops, render.NewRgbStop(r, g, b))
			}
		case float64:
			// 256-color index
			stops = append(stops, render.NewIndexStop(int(v)))
		}
	}
	return stops
}

// parseBgValue converts the generic bg interface to a BgValue.
func parseBgValue(bg interface{}) render.BgValue {
	switch v := bg.(type) {
	case []interface{}:
		if len(v) == 3 {
			return render.NewBgRgb(toUint8(v[0]), toUint8(v[1]), toUint8(v[2]))
		}
	case float64:
		return render.NewBgIndex(int(v))
	}
	return render.BgValue{}
}

func toUint8(v interface{}) uint8 {
	if f, ok := v.(float64); ok {
		if f < 0 {
			return 0
		}
		if f > 255 {
			return 255
		}
		return uint8(f)
	}
	return 0
}

// runStatusline is the main statusline pipeline.
func runStatusline() error {
	data := parseStdin(os.Stdin)

	model := extractModel(data.Model)
	contextPercent := extractContextPercent(data.ContextWindow)
	cwd := extractCwd(data)

	claudeDir := platform.GetClaudeDir()

	// If rate_limits present from stdin, write to cache for other sessions
	hasStdin := hasStdinUsage(data.RateLimits)
	if hasStdin {
		// Best-effort cache write
		_ = cache.WriteUsageFromStdin(claudeDir, data.RateLimits)
	}

	// Parallel data fetching
	var usage *cache.UsageResult
	var gitInfo *git.Info
	var ghUser string

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		if hasStdin {
			usage = usageFromStdinRateLimits(data.RateLimits)
		} else {
			cf := oauth.ReadCredentialsFile(claudeDir)
			var oauthData *oauth.OAuthData
			if cf != nil {
				oauthData = cf.ClaudeAiOauth
			}
			ai := oauth.GetAuthInfo(oauthData)
			forceRefresh := contextPercent == 0 // session start
			if ai.IsOAuth {
				usage = cache.GetUsageData(claudeDir, forceRefresh)
			} else {
				usage = &cache.UsageResult{}
			}
		}
	}()

	go func() {
		defer wg.Done()
		gitInfo = git.GetInfo(cwd)
	}()

	go func() {
		defer wg.Done()
		ghUser = platform.GetActiveGhUser(claudeDir)
	}()

	wg.Wait()

	// Sequential fast operations
	elapsed := platform.GetSessionElapsed(claudeDir)
	profile := platform.GetProfileName(claudeDir)
	profileColor := ""
	if profile != "" {
		profileColor = render.HueToAnsi(render.HashToHue(claudeDir), render.IsDarkMode())
	}
	ccVersion := platform.GetCCVersion()

	// Get auth info for tier display
	credFile := oauth.ReadCredentialsFile(claudeDir)
	var oauthData *oauth.OAuthData
	if credFile != nil {
		oauthData = credFile.ClaudeAiOauth
	}
	authInfo := oauth.GetAuthInfo(oauthData)

	// Time percentages
	var fiveHourTimePercent *float64
	var weeklyTimePercent *float64

	if usage.FiveHour != nil && usage.FiveHour.ResetIn > 0 {
		pct := math.Max(0, math.Min(100, (1.0-float64(usage.FiveHour.ResetIn)/float64(fiveHourMs))*100))
		fiveHourTimePercent = &pct
	}
	if usage.Weekly != nil && usage.Weekly.ResetIn > 0 {
		pct := math.Max(0, math.Min(100, (1.0-float64(usage.Weekly.ResetIn)/float64(sevenDayMs))*100))
		weeklyTimePercent = &pct
	}

	// Load config
	cfg := config.Get()

	// Ensure line elements exist
	lineElements := cfg.Lines
	if lineElements == nil {
		lineElements = &config.LinesConfig{
			Line1: []string{"context", "elapsed", "profile", "tier", "model", "version"},
			Line2: []string{"usage5h", "staleness", "age", "ghUser", "branch"},
			Line3: []string{"usageWeekly", "staleness", "age", "cwd"},
		}
	}

	// Build bar config
	barCfg := buildBarConfig(cfg)

	// Build usage data for render
	var fiveHourData render.UsageData
	if usage.FiveHour != nil {
		pct := usage.FiveHour.Percent
		fiveHourData = render.UsageData{Percent: &pct, ResetIn: usage.FiveHour.ResetIn}
	}

	var weeklyData render.UsageData
	if usage.Weekly != nil {
		pct := usage.Weekly.Percent
		weeklyData = render.UsageData{Percent: &pct, ResetIn: usage.Weekly.ResetIn}
	}

	var extraUsage *render.ExtraUsageData
	if usage.Extra != nil {
		extraUsage = &render.ExtraUsageData{
			Percent: usage.Extra.Percent,
			Enabled: usage.Extra.Enabled,
		}
	}

	// Build git info for render
	gitRender := render.GitInfo{
		Branch:  gitInfo.Branch,
		Ahead:   gitInfo.Ahead,
		Behind:  gitInfo.Behind,
		Changes: gitInfo.Changes,
		HasGit:  gitInfo.HasGit,
	}

	// Build line changes from cost
	var lineChanges render.LineChangeInfo
	if data.Cost != nil {
		if added, ok := data.Cost["total_lines_added"].(float64); ok {
			a := int(added)
			lineChanges.Added = &a
		}
		if removed, ok := data.Cost["total_lines_removed"].(float64); ok {
			r := int(removed)
			lineChanges.Removed = &r
		}
	}

	renderData := &render.RenderData{
		Context:             contextPercent,
		Model:               model,
		Tier:                authInfo.SubscriptionName,
		Elapsed:             elapsed,
		Profile:             profile,
		ProfileColor:        profileColor,
		GhUser:              ghUser,
		FiveHour:            fiveHourData,
		Weekly:              weeklyData,
		ExtraUsage:          extraUsage,
		Stale:               usage.Stale,
		LastSuccessTs:       usage.LastSuccessTs,
		Git:                 gitRender,
		LineChanges:         lineChanges,
		Cwd:                 render.ShortenPath(cwd, cfg.CwdMaxLength, cfg.CwdDepth),
		FiveHourTimePercent: fiveHourTimePercent,
		WeeklyTimePercent:   weeklyTimePercent,
		CcVersion:           ccVersion,
	}

	output := render.RenderLines(renderData, barCfg, lineElements)
	fmt.Println(output)
	return nil
}
