package render

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/smm-h/stricttest/go/hygiene"
)

// darkModeFixture isolates the environment, points the Claude configuration
// directory at a throwaway directory and returns the cache file's path.
func darkModeFixture(t *testing.T) string {
	t.Helper()
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	t.Setenv("HOWMUCHLEFT_DARK", "")
	ResetDarkModeCache()
	t.Cleanup(ResetDarkModeCache)
	return filepath.Join(dir, DarkModeCacheFile)
}

func writeCacheEntry(t *testing.T, path string, dark bool, ageMs int64) {
	t.Helper()
	data, err := json.Marshal(darkModeCacheEntry{Dark: dark, Ts: time.Now().UnixMilli() - ageMs})
	if err != nil {
		t.Fatalf("marshal cache entry: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write cache entry: %v", err)
	}
}

func readCacheEntry(t *testing.T, path string) darkModeCacheEntry {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cache entry: %v", err)
	}
	var entry darkModeCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("parse cache entry: %v", err)
	}
	return entry
}

// A fresh cache decides the answer whichever way it points, which is what
// proves the detector was not consulted: one of the two values must disagree
// with whatever this machine's desktop actually reports.
func TestDetectDarkMode_FreshCacheDecides(t *testing.T) {
	for _, dark := range []bool{true, false} {
		t.Run(map[bool]string{true: "dark", false: "light"}[dark], func(t *testing.T) {
			path := darkModeFixture(t)
			writeCacheEntry(t, path, dark, 0)

			if got := detectDarkMode(); got != dark {
				t.Errorf("detectDarkMode() = %v, want the cached %v", got, dark)
			}
		})
	}
}

func TestDetectDarkMode_StaleCacheIsRedetected(t *testing.T) {
	path := darkModeFixture(t)
	writeCacheEntry(t, path, true, darkModeCacheTTLMs+1000)

	before := time.Now().UnixMilli()
	got := detectDarkMode()

	entry := readCacheEntry(t, path)
	if entry.Ts < before {
		t.Errorf("stale cache was not refreshed: timestamp %d predates the call at %d", entry.Ts, before)
	}
	if entry.Dark != got {
		t.Errorf("cache holds Dark=%v but detectDarkMode() returned %v", entry.Dark, got)
	}
}

func TestDetectDarkMode_CacheEntryFromTheFutureIsIgnored(t *testing.T) {
	path := darkModeFixture(t)
	// A clock change can leave a timestamp ahead of now; such an entry has an
	// unknowable age, so it must not be trusted.
	writeCacheEntry(t, path, true, -60*1000)

	before := time.Now().UnixMilli()
	detectDarkMode()

	if entry := readCacheEntry(t, path); entry.Ts < before {
		t.Errorf("future-dated cache was not refreshed: timestamp %d predates the call at %d", entry.Ts, before)
	}
}

func TestDetectDarkMode_CorruptCacheIsReplaced(t *testing.T) {
	path := darkModeFixture(t)
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("write cache file: %v", err)
	}

	got := detectDarkMode()

	entry := readCacheEntry(t, path)
	if entry.Dark != got {
		t.Errorf("cache holds Dark=%v but detectDarkMode() returned %v", entry.Dark, got)
	}
}

func TestDetectDarkMode_MissingCacheIsWritten(t *testing.T) {
	path := darkModeFixture(t)

	got := detectDarkMode()

	entry := readCacheEntry(t, path)
	if entry.Dark != got {
		t.Errorf("cache holds Dark=%v but detectDarkMode() returned %v", entry.Dark, got)
	}
	if entry.Ts == 0 {
		t.Error("cache entry carries no timestamp")
	}
}

func TestDetectDarkMode_EnvOverrideBeatsCacheAndWritesNothing(t *testing.T) {
	path := darkModeFixture(t)
	writeCacheEntry(t, path, false, 0)
	t.Setenv("HOWMUCHLEFT_DARK", "1")

	if !detectDarkMode() {
		t.Error("HOWMUCHLEFT_DARK=1 must win over a cache saying light")
	}
	if entry := readCacheEntry(t, path); entry.Dark {
		t.Error("the override must not be written into the cache")
	}

	t.Setenv("HOWMUCHLEFT_DARK", "0")
	writeCacheEntry(t, path, true, 0)
	if detectDarkMode() {
		t.Error("HOWMUCHLEFT_DARK=0 must win over a cache saying dark")
	}
}

// The TTL is what decides how soon a theme switch reaches the statusline, so
// it is pinned in time, not against the constant that sets it.
func TestDetectDarkMode_CacheYoungerThanTwoSecondsIsUsed(t *testing.T) {
	for _, dark := range []bool{true, false} {
		t.Run(map[bool]string{true: "dark", false: "light"}[dark], func(t *testing.T) {
			path := darkModeFixture(t)
			writeCacheEntry(t, path, dark, 1500)

			if got := detectDarkMode(); got != dark {
				t.Errorf("detectDarkMode() = %v, want the cached %v", got, dark)
			}
		})
	}
}

func TestDetectDarkMode_CacheOlderThanTwoSecondsIsRedetected(t *testing.T) {
	path := darkModeFixture(t)
	writeCacheEntry(t, path, true, 2500)

	before := time.Now().UnixMilli()
	detectDarkMode()

	if entry := readCacheEntry(t, path); entry.Ts < before {
		t.Errorf("a cache 2.5 seconds old was not refreshed: timestamp %d predates the call at %d",
			entry.Ts, before)
	}
}
