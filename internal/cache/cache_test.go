package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadWriteCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()

	percent := 0.42
	resetAt := int64(1700000000000)
	successTs := int64(1699999000000)
	original := &CacheData{
		Status:        "ok",
		Ts:            1699999000000,
		ErrorCount:    0,
		LastSuccessTs: &successTs,
		FiveHour: &CachedWindow{
			Percent: &percent,
			ResetAt: &resetAt,
		},
		Weekly: &CachedWindow{
			Percent: &percent,
			ResetAt: &resetAt,
		},
		Extra: &CachedExtra{
			Enabled: true,
			Percent: &percent,
		},
	}

	if err := WriteCache(dir, original); err != nil {
		t.Fatalf("WriteCache failed: %v", err)
	}

	// Verify file exists.
	path := filepath.Join(dir, cacheFileName)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cache file not created: %v", err)
	}

	got := ReadCache(dir)
	if got == nil {
		t.Fatal("ReadCache returned nil")
	}

	if got.Status != "ok" {
		t.Errorf("status: got %q, want %q", got.Status, "ok")
	}
	if got.Ts != original.Ts {
		t.Errorf("ts: got %d, want %d", got.Ts, original.Ts)
	}
	if got.ErrorCount != 0 {
		t.Errorf("errorCount: got %d, want 0", got.ErrorCount)
	}
	if got.FiveHour == nil || got.FiveHour.Percent == nil || *got.FiveHour.Percent != 0.42 {
		t.Errorf("fiveHour percent mismatch")
	}
	if got.Weekly == nil || got.Weekly.ResetAt == nil || *got.Weekly.ResetAt != resetAt {
		t.Errorf("weekly resetAt mismatch")
	}
	if got.Extra == nil || !got.Extra.Enabled || got.Extra.Percent == nil || *got.Extra.Percent != 0.42 {
		t.Errorf("extra usage mismatch")
	}
}

func TestReadCacheReturnsNilOnMissing(t *testing.T) {
	dir := t.TempDir()
	got := ReadCache(dir)
	if got != nil {
		t.Errorf("expected nil for missing cache file, got %+v", got)
	}
}

func TestReadCacheReturnsNilOnInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, cacheFileName)
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}
	got := ReadCache(dir)
	if got != nil {
		t.Errorf("expected nil for invalid JSON, got %+v", got)
	}
}

func TestIsCacheValidFreshSuccess(t *testing.T) {
	now := int64(1700000060000)
	cache := &CacheData{
		Status: "ok",
		Ts:     now - 30000, // 30s ago
	}
	if !IsCacheValid(cache, now, false) {
		t.Error("expected fresh success cache (30s old) to be valid")
	}
}

func TestIsCacheValidExpiredSuccess(t *testing.T) {
	now := int64(1700000060000)
	cache := &CacheData{
		Status: "ok",
		Ts:     now - 61000, // 61s ago (>60s TTL)
	}
	if IsCacheValid(cache, now, false) {
		t.Error("expected expired success cache (61s old) to be invalid")
	}
}

func TestIsCacheValidForceRefresh(t *testing.T) {
	now := int64(1700000060000)
	cache := &CacheData{
		Status: "ok",
		Ts:     now - 10000, // 10s ago, fresh
	}
	if IsCacheValid(cache, now, true) {
		t.Error("expected force refresh to invalidate cache")
	}
}

func TestErrorBackoff(t *testing.T) {
	// Error backoff: 1st=60s, 2nd=120s, 3rd=240s, 4th+=300s (capped)
	tests := []struct {
		errorCount int
		wantTTL    time.Duration
	}{
		{1, 60 * time.Second},
		{2, 120 * time.Second},
		{3, 240 * time.Second},
		{4, 300 * time.Second}, // capped
		{5, 300 * time.Second}, // still capped
	}

	for _, tt := range tests {
		got := computeErrorTTL(tt.errorCount)
		if got != tt.wantTTL {
			t.Errorf("computeErrorTTL(%d) = %v, want %v", tt.errorCount, got, tt.wantTTL)
		}
	}
}

func TestIsCacheValidErrorBackoff(t *testing.T) {
	now := int64(1700000100000)

	// 1st error, 59s ago: should be valid (TTL=60s).
	cache1 := &CacheData{
		Status:     "error",
		Ts:         now - 59000,
		ErrorCount: 1,
	}
	if !IsCacheValid(cache1, now, false) {
		t.Error("1st error, 59s old: expected valid")
	}

	// 1st error, 61s ago: should be invalid.
	cache2 := &CacheData{
		Status:     "error",
		Ts:         now - 61000,
		ErrorCount: 1,
	}
	if IsCacheValid(cache2, now, false) {
		t.Error("1st error, 61s old: expected invalid")
	}

	// 2nd error, 100s ago: should be valid (TTL=120s).
	cache3 := &CacheData{
		Status:     "error",
		Ts:         now - 100000,
		ErrorCount: 2,
	}
	if !IsCacheValid(cache3, now, false) {
		t.Error("2nd error, 100s old: expected valid (TTL=120s)")
	}

	// 2nd error, 121s ago: should be invalid.
	cache4 := &CacheData{
		Status:     "error",
		Ts:         now - 121000,
		ErrorCount: 2,
	}
	if IsCacheValid(cache4, now, false) {
		t.Error("2nd error, 121s old: expected invalid (TTL=120s)")
	}

	// 3rd error, 250s ago: should be valid (TTL=240s? no, 240s < 250s).
	cache5 := &CacheData{
		Status:     "error",
		Ts:         now - 250000,
		ErrorCount: 3,
	}
	if IsCacheValid(cache5, now, false) {
		t.Error("3rd error, 250s old: expected invalid (TTL=240s)")
	}

	// 4th error, 290s ago: should be valid (TTL=300s, capped).
	cache6 := &CacheData{
		Status:     "error",
		Ts:         now - 290000,
		ErrorCount: 4,
	}
	if !IsCacheValid(cache6, now, false) {
		t.Error("4th error, 290s old: expected valid (TTL=300s capped)")
	}
}

func TestForceRefreshWhenResetAtPassed(t *testing.T) {
	now := int64(1700000100000)
	resetAt := int64(1700000050000) // 50s ago, already passed

	// Cache is only 10s old (fresh), but resetAt has passed.
	cache := &CacheData{
		Status: "ok",
		Ts:     now - 10000,
		FiveHour: &CachedWindow{
			Percent: ptrFloat(0.5),
			ResetAt: &resetAt,
		},
	}
	if IsCacheValid(cache, now, false) {
		t.Error("expected cache to be invalid when resetAt has passed")
	}
}

func TestForceRefreshWhenWeeklyResetAtPassed(t *testing.T) {
	now := int64(1700000100000)
	resetAt := int64(1700000050000) // already passed

	cache := &CacheData{
		Status: "ok",
		Ts:     now - 10000,
		Weekly: &CachedWindow{
			Percent: ptrFloat(0.3),
			ResetAt: &resetAt,
		},
	}
	if IsCacheValid(cache, now, false) {
		t.Error("expected cache to be invalid when weekly resetAt has passed")
	}
}

func TestStaleFallbackReturnsLastSuccessData(t *testing.T) {
	now := int64(1700000100000)
	resetAt := int64(1700000200000) // still in future
	successTs := int64(1699999000000)

	cache := &CacheData{
		Status:        "error",
		Ts:            now - 61000, // expired error cache
		ErrorCount:    1,
		LastSuccessTs: &successTs,
		FiveHour: &CachedWindow{
			Percent: ptrFloat(0.75),
			ResetAt: &resetAt,
		},
		Weekly: &CachedWindow{
			Percent: ptrFloat(0.25),
			ResetAt: &resetAt,
		},
	}

	result := cacheToResult(cache, now, true)

	if !result.Stale {
		t.Error("expected stale=true")
	}
	if result.LastSuccessTs != successTs {
		t.Errorf("lastSuccessTs: got %d, want %d", result.LastSuccessTs, successTs)
	}
	if result.FiveHour == nil || result.FiveHour.Percent != 0.75 {
		t.Error("expected fiveHour percent 0.75")
	}
	if result.Weekly == nil || result.Weekly.Percent != 0.25 {
		t.Error("expected weekly percent 0.25")
	}
	if result.FiveHour.ResetIn != (resetAt - now) {
		t.Errorf("fiveHour resetIn: got %d, want %d", result.FiveHour.ResetIn, resetAt-now)
	}
}

func TestCacheToResultNoData(t *testing.T) {
	now := int64(1700000100000)
	cache := &CacheData{
		Status:     "error",
		Ts:         now,
		ErrorCount: 1,
	}
	result := cacheToResult(cache, now, false)
	if result.FiveHour != nil {
		t.Error("expected nil fiveHour when no data cached")
	}
	if result.Weekly != nil {
		t.Error("expected nil weekly when no data cached")
	}
}

func TestHasUsableData(t *testing.T) {
	if hasUsableData(nil) {
		t.Error("nil cache should not have usable data")
	}
	if hasUsableData(&CacheData{}) {
		t.Error("empty cache should not have usable data")
	}
	if !hasUsableData(&CacheData{FiveHour: &CachedWindow{Percent: ptrFloat(0.5)}}) {
		t.Error("cache with fiveHour percent should have usable data")
	}
	if !hasUsableData(&CacheData{Weekly: &CachedWindow{Percent: ptrFloat(0.3)}}) {
		t.Error("cache with weekly percent should have usable data")
	}
}

func TestWriteUsageFromStdin(t *testing.T) {
	dir := t.TempDir()

	// Override NowMs for deterministic test.
	origNow := NowMs
	NowMs = func() int64 { return 1700000000000 }
	defer func() { NowMs = origNow }()

	rateLimits := map[string]interface{}{
		"five_hour": map[string]interface{}{
			"utilization": 0.65,
			"resets_at":   "2024-01-15T12:00:00Z",
		},
		"seven_day": map[string]interface{}{
			"utilization": 0.30,
			"resets_at":   "2024-01-20T00:00:00Z",
		},
		"extra_usage": map[string]interface{}{
			"is_enabled":  true,
			"utilization": 0.10,
		},
	}

	if err := WriteUsageFromStdin(dir, rateLimits); err != nil {
		t.Fatalf("WriteUsageFromStdin failed: %v", err)
	}

	// Read it back.
	cache := ReadCache(dir)
	if cache == nil {
		t.Fatal("ReadCache returned nil after WriteUsageFromStdin")
	}
	if cache.Status != "ok" {
		t.Errorf("status: got %q, want %q", cache.Status, "ok")
	}
	if cache.FiveHour == nil || cache.FiveHour.Percent == nil || *cache.FiveHour.Percent != 0.65 {
		t.Error("fiveHour percent mismatch")
	}
	if cache.Weekly == nil || cache.Weekly.Percent == nil || *cache.Weekly.Percent != 0.30 {
		t.Error("weekly percent mismatch")
	}
	if cache.Extra == nil || !cache.Extra.Enabled {
		t.Error("extra usage enabled mismatch")
	}
	if cache.Extra.Percent == nil || *cache.Extra.Percent != 0.10 {
		t.Error("extra usage percent mismatch")
	}
}

func TestWriteUsageFromStdinPartial(t *testing.T) {
	dir := t.TempDir()

	origNow := NowMs
	NowMs = func() int64 { return 1700000000000 }
	defer func() { NowMs = origNow }()

	// Only five_hour present.
	rateLimits := map[string]interface{}{
		"five_hour": map[string]interface{}{
			"utilization": 0.9,
		},
	}

	if err := WriteUsageFromStdin(dir, rateLimits); err != nil {
		t.Fatalf("WriteUsageFromStdin failed: %v", err)
	}

	cache := ReadCache(dir)
	if cache == nil {
		t.Fatal("ReadCache returned nil")
	}
	if cache.FiveHour == nil || cache.FiveHour.Percent == nil || *cache.FiveHour.Percent != 0.9 {
		t.Error("fiveHour percent mismatch")
	}
	if cache.Weekly != nil {
		t.Error("expected nil weekly when not provided")
	}
}

func TestCacheJSONFieldNames(t *testing.T) {
	// Verify JSON field names match the JS implementation for cross-language compat.
	percent := 0.5
	resetAt := int64(1700000000000)
	successTs := int64(1699999000000)
	data := &CacheData{
		Status:        "ok",
		Ts:            1699999900000,
		ErrorCount:    0,
		LastSuccessTs: &successTs,
		FiveHour: &CachedWindow{
			Percent: &percent,
			ResetAt: &resetAt,
		},
		Weekly: &CachedWindow{
			Percent: &percent,
			ResetAt: &resetAt,
		},
		Extra: &CachedExtra{
			Enabled: true,
			Percent: &percent,
		},
	}

	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}

	// Unmarshal into a generic map to check field names.
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}

	// Top-level fields.
	expectedKeys := []string{"status", "timestamp", "consecutiveErrors", "lastSuccessTs", "fiveHour", "weekly", "extraUsage"}
	for _, k := range expectedKeys {
		if _, ok := m[k]; !ok {
			t.Errorf("missing expected JSON key %q", k)
		}
	}

	// Nested fiveHour fields.
	if fh, ok := m["fiveHour"].(map[string]interface{}); ok {
		if _, ok := fh["percent"]; !ok {
			t.Error("fiveHour missing 'percent' key")
		}
		if _, ok := fh["resetAt"]; !ok {
			t.Error("fiveHour missing 'resetAt' key")
		}
	} else {
		t.Error("fiveHour is not a map")
	}
}

// ptrFloat is a helper to create a *float64.
func ptrFloat(f float64) *float64 {
	return &f
}
