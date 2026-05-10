package platform

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGhTokenHash(t *testing.T) {
	// SHA-256 of "ghp_testtoken123" first 8 hex chars
	hash := GhTokenHash("ghp_testtoken123")
	if len(hash) != 8 {
		t.Errorf("hash length = %d, want 8", len(hash))
	}

	// Deterministic: same input produces same output
	hash2 := GhTokenHash("ghp_testtoken123")
	if hash != hash2 {
		t.Errorf("hash not deterministic: %q != %q", hash, hash2)
	}

	// Different tokens produce different hashes
	hash3 := GhTokenHash("ghp_differenttoken")
	if hash == hash3 {
		t.Errorf("different tokens produced same hash: %q", hash)
	}
}

func TestReadWriteGhUserDiskCache(t *testing.T) {
	dir := t.TempDir()

	// Initially empty
	cache := readGhUserDiskCache(dir)
	if len(cache) != 0 {
		t.Errorf("expected empty cache, got %d entries", len(cache))
	}

	// Write a cache entry
	cache["abc12345"] = ghUserCacheEntry{User: "testuser", Ts: 1700000000000}
	writeGhUserDiskCache(dir, cache)

	// Read it back
	cache2 := readGhUserDiskCache(dir)
	if len(cache2) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(cache2))
	}
	entry := cache2["abc12345"]
	if entry.User != "testuser" {
		t.Errorf("user = %q, want %q", entry.User, "testuser")
	}
	if entry.Ts != 1700000000000 {
		t.Errorf("ts = %d, want %d", entry.Ts, 1700000000000)
	}

	// Verify file permissions
	cachePath := filepath.Join(dir, ".gh-users-cache.json")
	info, err := os.Stat(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("file perm = %o, want 0600", perm)
	}
}

func TestCacheFormat(t *testing.T) {
	dir := t.TempDir()

	// Write raw JSON in the expected format and verify it parses
	raw := `{"abcd1234":{"user":"octocat","ts":1700000000000},"efgh5678":{"ts":1700000001000,"negative":true}}`
	cachePath := filepath.Join(dir, ".gh-users-cache.json")
	if err := os.WriteFile(cachePath, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}

	cache := readGhUserDiskCache(dir)
	if len(cache) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(cache))
	}

	positive := cache["abcd1234"]
	if positive.User != "octocat" || positive.Negative {
		t.Errorf("positive entry: user=%q negative=%v", positive.User, positive.Negative)
	}

	negative := cache["efgh5678"]
	if negative.User != "" || !negative.Negative {
		t.Errorf("negative entry: user=%q negative=%v", negative.User, negative.Negative)
	}
}

func TestNegativeCacheTTLExpiry(t *testing.T) {
	// A negative entry with ts in the past should be considered expired
	now := time.Now().UnixMilli()

	// Entry from 6 minutes ago (past the 5-min TTL)
	expiredTs := now - 6*60*1000
	entry := ghUserCacheEntry{Negative: true, Ts: expiredTs}

	elapsed := now - entry.Ts
	if elapsed < ghNegativeTTLMs {
		t.Error("expected entry to be expired (elapsed >= TTL)")
	}

	// Entry from 2 minutes ago (within TTL)
	freshTs := now - 2*60*1000
	freshEntry := ghUserCacheEntry{Negative: true, Ts: freshTs}
	freshElapsed := now - freshEntry.Ts
	if freshElapsed >= ghNegativeTTLMs {
		t.Error("expected entry to be fresh (elapsed < TTL)")
	}
}

func TestWriteGhUserDiskCache_AtomicCreatesDir(t *testing.T) {
	// The cache write should handle a non-existent parent directory gracefully
	// (it creates the directory via MkdirAll)
	dir := filepath.Join(t.TempDir(), "nested", "dir")

	cache := map[string]ghUserCacheEntry{
		"test1234": {User: "user1", Ts: 1700000000000},
	}
	writeGhUserDiskCache(dir, cache)

	// Verify it was written
	cachePath := filepath.Join(dir, ".gh-users-cache.json")
	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("failed to read cache after write: %v", err)
	}

	var readBack map[string]ghUserCacheEntry
	if err := json.Unmarshal(data, &readBack); err != nil {
		t.Fatalf("failed to parse written cache: %v", err)
	}
	if readBack["test1234"].User != "user1" {
		t.Errorf("read back user = %q, want %q", readBack["test1234"].User, "user1")
	}
}

func TestGetActiveGhUser_NoToken(t *testing.T) {
	ResetGhUserCache()
	t.Setenv("GH_TOKEN", "")

	dir := t.TempDir()
	user := GetActiveGhUser(dir)
	if user != "" {
		t.Errorf("expected empty user with no GH_TOKEN, got %q", user)
	}
}

func TestGetActiveGhUser_CachedPositive(t *testing.T) {
	ResetGhUserCache()
	dir := t.TempDir()

	// Pre-populate cache with a positive entry for a known token hash
	token := "ghp_cachedtoken"
	hash := GhTokenHash(token)
	cache := map[string]ghUserCacheEntry{
		hash: {User: "cacheduser", Ts: time.Now().UnixMilli()},
	}
	writeGhUserDiskCache(dir, cache)

	t.Setenv("GH_TOKEN", token)
	user := GetActiveGhUser(dir)
	if user != "cacheduser" {
		t.Errorf("expected %q, got %q", "cacheduser", user)
	}
}
