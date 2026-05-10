package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const ghNegativeTTLMs = 5 * 60 * 1000 // 5 minutes

// ghUserCacheEntry represents a cached GitHub user lookup result.
type ghUserCacheEntry struct {
	User     string `json:"user,omitempty"`
	Ts       int64  `json:"ts"`
	Negative bool   `json:"negative,omitempty"`
}

// Per-process in-memory cache for the resolved GitHub user.
var (
	ghUserOnce   sync.Once
	ghUserResult string
)

// GetActiveGhUser returns the GitHub username for the current GH_TOKEN, or ""
// if no token is set or the lookup fails. Results are cached both on disk and
// in process memory (subsequent calls never re-read or re-run gh).
func GetActiveGhUser(claudeDir string) string {
	ghUserOnce.Do(func() {
		ghUserResult = resolveGhUser(claudeDir)
	})
	return ghUserResult
}

// ResetGhUserCache clears the per-process cache (for testing only).
func ResetGhUserCache() {
	ghUserOnce = sync.Once{}
	ghUserResult = ""
}

func resolveGhUser(claudeDir string) string {
	token := os.Getenv("GH_TOKEN")
	if token == "" {
		return ""
	}

	hash := ghTokenHash(token)
	cache := readGhUserDiskCache(claudeDir)
	entry, ok := cache[hash]

	if ok {
		if entry.User != "" {
			return entry.User
		}
		// Negative entry: check TTL
		if entry.Ts > 0 && nowMs()-entry.Ts < ghNegativeTTLMs {
			return ""
		}
	}

	// Cache miss or stale negative: call gh
	login := runGhUserLookup()

	// Update cache
	if login != "" {
		cache[hash] = ghUserCacheEntry{User: login, Ts: nowMs()}
	} else {
		cache[hash] = ghUserCacheEntry{Negative: true, Ts: nowMs()}
	}
	writeGhUserDiskCache(claudeDir, cache)

	return login
}

func ghTokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])[:8]
}

// GhTokenHash is exported for testing.
func GhTokenHash(token string) string {
	return ghTokenHash(token)
}

func readGhUserDiskCache(claudeDir string) map[string]ghUserCacheEntry {
	cachePath := filepath.Join(claudeDir, ".gh-users-cache.json")
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return make(map[string]ghUserCacheEntry)
	}
	var cache map[string]ghUserCacheEntry
	if err := json.Unmarshal(data, &cache); err != nil {
		return make(map[string]ghUserCacheEntry)
	}
	if cache == nil {
		return make(map[string]ghUserCacheEntry)
	}
	return cache
}

func writeGhUserDiskCache(claudeDir string, cache map[string]ghUserCacheEntry) {
	cachePath := filepath.Join(claudeDir, ".gh-users-cache.json")
	data, err := json.Marshal(cache)
	if err != nil {
		return
	}

	// Atomic write: tmpfile + rename, then chmod 0600
	dir := filepath.Dir(cachePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, ".tmp-gh-cache-*")
	if err != nil {
		return
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return
	}
	if err := os.Rename(tmpPath, cachePath); err != nil {
		os.Remove(tmpPath)
		return
	}
	os.Chmod(cachePath, 0600)
}

func runGhUserLookup() string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "api", "user", "--jq", ".login")
	// GH_TOKEN is already in the environment, inherited by the subprocess.
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "howmuchleft: gh user lookup failed\n")
		return ""
	}
	login := strings.TrimSpace(string(out))
	return login
}

func nowMs() int64 {
	return time.Now().UnixMilli()
}
