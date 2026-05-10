package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/smm-h/howmuchleft/internal/oauth"
)

// Cache TTL constants.
const (
	CacheTTL        = 60 * time.Second
	ErrorCacheTTL   = 60 * time.Second
	MaxErrorCacheTTL = 5 * time.Minute
	CombinedTimeout = 7 * time.Second
)

// ErrAuth is returned when the usage API responds with 401 or 403.
var ErrAuth = errors.New("authentication error (401/403)")

// --- Usage API types (3.3) ---

// UsageResponse is the parsed JSON from the usage API.
type UsageResponse struct {
	FiveHour WindowUsage `json:"five_hour"`
	Weekly   WindowUsage `json:"seven_day"`
	Extra    *ExtraUsage `json:"extra_usage"`
}

// WindowUsage represents a single usage window from the API.
type WindowUsage struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    string  `json:"resets_at"` // ISO 8601 timestamp
}

// ExtraUsage represents extra/overage usage from the API.
type ExtraUsage struct {
	Enabled     bool    `json:"is_enabled"`
	Utilization float64 `json:"utilization"`
}

// --- Cache types (3.4) ---

// CacheData is persisted to .statusline-cache.json.
type CacheData struct {
	Status           string       `json:"status"`           // "ok" or "error"
	Ts               int64        `json:"timestamp"`        // unix ms when cached
	ErrorCount       int          `json:"consecutiveErrors"`
	FiveHour         *CachedWindow `json:"fiveHour"`
	Weekly           *CachedWindow `json:"weekly"`
	Extra            *CachedExtra  `json:"extraUsage"`
	LastSuccessTs    *int64       `json:"lastSuccessTs"`
}

// CachedWindow stores a usage window in the cache.
type CachedWindow struct {
	Percent *float64 `json:"percent"`
	ResetAt *int64   `json:"resetAt"` // absolute unix ms
}

// CachedExtra stores extra usage in the cache.
type CachedExtra struct {
	Percent *float64 `json:"percent"`
	Enabled bool     `json:"enabled"`
}

// --- Result types ---

// UsageResult is what GetUsageData returns to callers.
type UsageResult struct {
	FiveHour      *WindowResult
	Weekly        *WindowResult
	Extra         *ExtraResult
	Stale         bool
	LastSuccessTs int64 // 0 means unknown
}

// WindowResult represents a single usage window for display.
type WindowResult struct {
	Percent float64
	ResetIn int64 // ms until reset
}

// ExtraResult represents extra usage for display.
type ExtraResult struct {
	Enabled bool
	Percent float64
}

// --- Usage API (3.3) ---

// FetchUsageFromAPI calls the usage API with the given access token.
// Returns ErrAuth for 401/403, generic error for other failures.
func FetchUsageFromAPI(accessToken string) (*UsageResponse, error) {
	if accessToken == "" {
		return nil, fmt.Errorf("empty access token")
	}

	client := &http.Client{Timeout: 5 * time.Second}

	req, err := http.NewRequest("GET", "https://platform.claude.com/api/oauth/usage", nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "howmuchleft")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, ErrAuth
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	// Check for API-level error response.
	var errCheck struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(body, &errCheck) == nil && errCheck.Type == "error" {
		return nil, fmt.Errorf("API returned error response")
	}

	var result UsageResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return &result, nil
}

// --- Cache layer (3.4) ---

// cacheFileName is the filename within claudeDir for the cache.
const cacheFileName = ".statusline-cache.json"

// ReadCache reads the cache file from claudeDir. Returns nil on any error.
func ReadCache(claudeDir string) *CacheData {
	path := filepath.Join(claudeDir, cacheFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cache CacheData
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil
	}
	return &cache
}

// WriteCache atomically writes cache data to the cache file.
func WriteCache(claudeDir string, data *CacheData) error {
	path := filepath.Join(claudeDir, cacheFileName)
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling cache: %w", err)
	}
	return oauth.WriteFileAtomic(path, raw)
}

// NowMs returns the current time in unix milliseconds. Exported for testing.
var NowMs = func() int64 {
	return time.Now().UnixMilli()
}

// computeErrorTTL calculates the backoff TTL for a given error count.
func computeErrorTTL(errorCount int) time.Duration {
	if errorCount <= 0 {
		errorCount = 1
	}
	ttl := ErrorCacheTTL * time.Duration(math.Pow(2, float64(errorCount-1)))
	if ttl > MaxErrorCacheTTL {
		ttl = MaxErrorCacheTTL
	}
	return ttl
}

// IsCacheValid determines whether the cached data can be used without refresh.
func IsCacheValid(cache *CacheData, now int64, forceRefresh bool) bool {
	if cache == nil || cache.Ts == 0 {
		return false
	}
	if forceRefresh {
		return false
	}

	ageMs := now - cache.Ts

	// Check if a reset time has passed (cached percent is stale).
	if cache.FiveHour != nil && cache.FiveHour.ResetAt != nil && now >= *cache.FiveHour.ResetAt {
		return false
	}
	if cache.Weekly != nil && cache.Weekly.ResetAt != nil && now >= *cache.Weekly.ResetAt {
		return false
	}

	var ttl time.Duration
	if cache.Status == "error" {
		ttl = computeErrorTTL(cache.ErrorCount)
	} else {
		ttl = CacheTTL
	}

	return ageMs < ttl.Milliseconds()
}

// cacheToResult converts cached data to UsageResult.
func cacheToResult(cache *CacheData, now int64, stale bool) *UsageResult {
	result := &UsageResult{
		Stale: stale,
	}

	if cache.LastSuccessTs != nil {
		result.LastSuccessTs = *cache.LastSuccessTs
	}

	if cache.FiveHour != nil && cache.FiveHour.Percent != nil {
		wr := &WindowResult{Percent: *cache.FiveHour.Percent}
		if cache.FiveHour.ResetAt != nil {
			resetIn := *cache.FiveHour.ResetAt - now
			if resetIn < 0 {
				resetIn = 0
			}
			wr.ResetIn = resetIn
		}
		result.FiveHour = wr
	}

	if cache.Weekly != nil && cache.Weekly.Percent != nil {
		wr := &WindowResult{Percent: *cache.Weekly.Percent}
		if cache.Weekly.ResetAt != nil {
			resetIn := *cache.Weekly.ResetAt - now
			if resetIn < 0 {
				resetIn = 0
			}
			wr.ResetIn = resetIn
		}
		result.Weekly = wr
	}

	if cache.Extra != nil {
		result.Extra = &ExtraResult{
			Enabled: cache.Extra.Enabled,
		}
		if cache.Extra.Percent != nil {
			result.Extra.Percent = *cache.Extra.Percent
		}
	}

	return result
}

// hasUsableData checks if the cache has any percent data worth falling back to.
func hasUsableData(cache *CacheData) bool {
	if cache == nil {
		return false
	}
	if cache.FiveHour != nil && cache.FiveHour.Percent != nil {
		return true
	}
	if cache.Weekly != nil && cache.Weekly.Percent != nil {
		return true
	}
	return false
}

// GetUsageData is the main entry point: reads cache, fetches if needed, returns results.
func GetUsageData(claudeDir string, forceRefresh bool) *UsageResult {
	now := NowMs()
	cache := ReadCache(claudeDir)

	// Return cached data if valid.
	if IsCacheValid(cache, now, forceRefresh) {
		return cacheToResult(cache, now, false)
	}

	// Need to fetch fresh data. Combined timeout for token + API.
	type fetchResult struct {
		usage *UsageResponse
		err   error
	}

	ch := make(chan fetchResult, 1)
	go func() {
		token, err := oauth.GetValidToken(claudeDir)
		if err != nil {
			ch <- fetchResult{nil, err}
			return
		}
		resp, err := FetchUsageFromAPI(token)
		ch <- fetchResult{resp, err}
	}()

	var fr fetchResult
	select {
	case fr = <-ch:
	case <-time.After(CombinedTimeout):
		fr = fetchResult{nil, fmt.Errorf("timeout")}
	}

	// Handle fetch failure.
	if fr.err != nil {
		writeErrorCache(claudeDir, cache, now)
		if hasUsableData(cache) {
			return cacheToResult(cache, now, true)
		}
		return emptyResult()
	}

	// Success: parse API response into cache format and write.
	return writeSuccessCache(claudeDir, cache, fr.usage, now)
}

// writeErrorCache writes an error entry to the cache, preserving last-known-good data.
func writeErrorCache(claudeDir string, oldCache *CacheData, now int64) {
	entry := &CacheData{
		Status:     "error",
		Ts:         now,
		ErrorCount: 1,
	}

	if oldCache != nil {
		entry.ErrorCount = oldCache.ErrorCount + 1
		entry.LastSuccessTs = oldCache.LastSuccessTs
		entry.FiveHour = oldCache.FiveHour
		entry.Weekly = oldCache.Weekly
		entry.Extra = oldCache.Extra
	}

	// Best effort write.
	_ = WriteCache(claudeDir, entry)
}

// writeSuccessCache converts the API response into a cache entry, writes it, and returns the result.
func writeSuccessCache(claudeDir string, oldCache *CacheData, resp *UsageResponse, now int64) *UsageResult {
	successTs := now
	entry := &CacheData{
		Status:        "ok",
		Ts:            now,
		ErrorCount:    0,
		LastSuccessTs: &successTs,
	}

	result := &UsageResult{
		Stale:         false,
		LastSuccessTs: successTs,
	}

	// Five-hour window.
	if resp.FiveHour.Utilization != 0 || resp.FiveHour.ResetsAt != "" {
		percent := resp.FiveHour.Utilization
		cw := &CachedWindow{Percent: &percent}
		wr := &WindowResult{Percent: percent}

		if resp.FiveHour.ResetsAt != "" {
			if t, err := time.Parse(time.RFC3339, resp.FiveHour.ResetsAt); err == nil {
				resetAt := t.UnixMilli()
				cw.ResetAt = &resetAt
				resetIn := resetAt - now
				if resetIn < 0 {
					resetIn = 0
				}
				wr.ResetIn = resetIn
			}
		}

		entry.FiveHour = cw
		result.FiveHour = wr
	}

	// Weekly window.
	if resp.Weekly.Utilization != 0 || resp.Weekly.ResetsAt != "" {
		percent := resp.Weekly.Utilization
		cw := &CachedWindow{Percent: &percent}
		wr := &WindowResult{Percent: percent}

		if resp.Weekly.ResetsAt != "" {
			if t, err := time.Parse(time.RFC3339, resp.Weekly.ResetsAt); err == nil {
				resetAt := t.UnixMilli()
				cw.ResetAt = &resetAt
				resetIn := resetAt - now
				if resetIn < 0 {
					resetIn = 0
				}
				wr.ResetIn = resetIn
			}
		}

		entry.Weekly = cw
		result.Weekly = wr
	}

	// Extra usage.
	if resp.Extra != nil {
		ce := &CachedExtra{
			Enabled: resp.Extra.Enabled,
		}
		er := &ExtraResult{
			Enabled: resp.Extra.Enabled,
		}
		if resp.Extra.Enabled {
			p := resp.Extra.Utilization
			ce.Percent = &p
			er.Percent = p
		}
		entry.Extra = ce
		result.Extra = er
	}

	// Write only if we got meaningful data.
	if result.FiveHour != nil || result.Weekly != nil {
		_ = WriteCache(claudeDir, entry)
	}

	return result
}

// emptyResult returns a UsageResult with no data.
func emptyResult() *UsageResult {
	return &UsageResult{
		Stale:         false,
		LastSuccessTs: 0,
	}
}

// WriteUsageFromStdin writes rate limit data from stdin directly to cache,
// bypassing the API. This is the "newer Claude Code" path where rate_limits
// are provided in the stdin JSON.
func WriteUsageFromStdin(claudeDir string, rateLimits map[string]interface{}) error {
	now := NowMs()
	entry := &CacheData{
		Status:        "ok",
		Ts:            now,
		ErrorCount:    0,
		LastSuccessTs: &now,
	}

	// Parse five_hour from rate_limits.
	if fh, ok := rateLimits["five_hour"].(map[string]interface{}); ok {
		cw := &CachedWindow{}
		if p, ok := fh["utilization"].(float64); ok {
			cw.Percent = &p
		}
		if ra, ok := fh["resets_at"].(string); ok && ra != "" {
			if t, err := time.Parse(time.RFC3339, ra); err == nil {
				resetAt := t.UnixMilli()
				cw.ResetAt = &resetAt
			}
		}
		if cw.Percent != nil {
			entry.FiveHour = cw
		}
	}

	// Parse seven_day (weekly) from rate_limits.
	if sd, ok := rateLimits["seven_day"].(map[string]interface{}); ok {
		cw := &CachedWindow{}
		if p, ok := sd["utilization"].(float64); ok {
			cw.Percent = &p
		}
		if ra, ok := sd["resets_at"].(string); ok && ra != "" {
			if t, err := time.Parse(time.RFC3339, ra); err == nil {
				resetAt := t.UnixMilli()
				cw.ResetAt = &resetAt
			}
		}
		if cw.Percent != nil {
			entry.Weekly = cw
		}
	}

	// Parse extra_usage from rate_limits.
	if eu, ok := rateLimits["extra_usage"].(map[string]interface{}); ok {
		ce := &CachedExtra{}
		if enabled, ok := eu["is_enabled"].(bool); ok {
			ce.Enabled = enabled
		}
		if p, ok := eu["utilization"].(float64); ok {
			ce.Percent = &p
		}
		entry.Extra = ce
	}

	return WriteCache(claudeDir, entry)
}
