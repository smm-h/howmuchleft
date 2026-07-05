package cli

import (
	"encoding/json"
	"fmt"
	"io"
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
		percent, resetAtMs := cache.ParseWindowFromMap(fh)
		wr := &cache.WindowResult{}
		if percent != nil {
			wr.Percent = *percent
		}
		if resetAtMs != nil {
			resetIn := *resetAtMs - now
			if resetIn < 0 {
				resetIn = 0
			}
			wr.ResetIn = resetIn
		}
		result.FiveHour = wr
	}

	if sd, ok := rateLimits["seven_day"].(map[string]interface{}); ok {
		percent, resetAtMs := cache.ParseWindowFromMap(sd)
		wr := &cache.WindowResult{}
		if percent != nil {
			wr.Percent = *percent
		}
		if resetAtMs != nil {
			resetIn := *resetAtMs - now
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
	if hasStdin && !authInfo.IsOAuth {
		authInfo = oauth.AuthInfo{IsOAuth: true, SubscriptionName: "OAuth"}
	}

	// Time percentages
	var fiveHourTimePercent *float64
	var weeklyTimePercent *float64

	if usage.FiveHour != nil && usage.FiveHour.ResetIn > 0 {
		pct := render.ComputeTimePercent(usage.FiveHour.ResetIn, fiveHourMs)
		fiveHourTimePercent = &pct
	}
	if usage.Weekly != nil && usage.Weekly.ResetIn > 0 {
		pct := render.ComputeTimePercent(usage.Weekly.ResetIn, sevenDayMs)
		weeklyTimePercent = &pct
	}

	var fableWeeklyTimePercent *float64
	if usage.FableWeekly != nil && usage.FableWeekly.ResetIn > 0 {
		pct := render.ComputeTimePercent(usage.FableWeekly.ResetIn, sevenDayMs)
		fableWeeklyTimePercent = &pct
	}

	// Load config
	cfg := config.Get()

	// Ensure line elements exist
	lineElements := cfg.Lines
	if lineElements == nil {
		lineElements = config.DefaultLines()
	}

	// Build bar config
	barCfg := render.BuildBarConfig(cfg)

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

	var fableWeeklyData render.UsageData
	if usage.FableWeekly != nil {
		pct := usage.FableWeekly.Percent
		fableWeeklyData = render.UsageData{Percent: &pct, ResetIn: usage.FableWeekly.ResetIn}
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
		Context:                contextPercent,
		Model:                  model,
		Tier:                   authInfo.SubscriptionName,
		Elapsed:                elapsed,
		Profile:                profile,
		ProfileColor:           profileColor,
		GhUser:                 ghUser,
		FiveHour:               fiveHourData,
		Weekly:                 weeklyData,
		FableWeekly:            fableWeeklyData,
		ExtraUsage:             extraUsage,
		Stale:                  usage.Stale,
		LastSuccessTs:          usage.LastSuccessTs,
		Git:                    gitRender,
		LineChanges:            lineChanges,
		Cwd:                    render.ShortenPath(cwd, cfg.CwdMaxLength, cfg.CwdDepth),
		FiveHourTimePercent:    fiveHourTimePercent,
		WeeklyTimePercent:      weeklyTimePercent,
		FableWeeklyTimePercent: fableWeeklyTimePercent,
		CcVersion:              ccVersion,
	}

	output := render.RenderLines(renderData, barCfg, lineElements, nil)
	fmt.Println(output)
	return nil
}
