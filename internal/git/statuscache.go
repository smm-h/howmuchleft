package git

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/smm-h/howmuchleft/internal/platform"
)

// StatusCacheDir is the directory, inside the Claude configuration directory,
// that holds the status cache: one file per repository root, named after a
// hash of that root, holding the branch, its ahead/behind counts against its
// upstream, the number of changed working-tree paths and the time they were
// measured.
const StatusCacheDir = ".git-status-cache"

// RefreshFlag is the argument that runs howmuchleft as the detached refresher
// a render starts: howmuchleft --refresh-git-cache <repository-root>.
const RefreshFlag = "--refresh-git-cache"

// statusCacheTTLMs is how long a cached status is rendered before a render
// starts a refresh. The refresh is never waited for, so this is the lag
// between a working-tree change and the counts that describe it, not a delay
// the render pays.
const statusCacheTTLMs = 2 * 1000

// refreshLockStaleness is how old a refresh lock may get before a render
// treats it as abandoned and starts a refresh of its own. It only has to
// exceed how long git status takes on a large repository.
const refreshLockStaleness = 30 * time.Second

// refreshTimeout bounds the git status the refresher runs, so a hung git does
// not hold the lock until it goes stale.
const refreshTimeout = 10 * time.Second

// statusEntry is what a cache file holds. Root is stored so an entry can be
// recognized as belonging to some other repository -- a hash collision, or a
// cache file copied between machines -- instead of being rendered as this one.
type statusEntry struct {
	Root    string `json:"root"`
	Branch  string `json:"branch"`
	Ahead   int    `json:"ahead"`
	Behind  int    `json:"behind"`
	Changed int    `json:"changed"`
	Ts      int64  `json:"ts"`
}

// cacheKey turns a repository root into the file name stem its cache uses.
func cacheKey(root string) string {
	sum := sha256.Sum256([]byte(root))
	return hex.EncodeToString(sum[:8])
}

// cachePathFor returns the cache file path for a repository root.
func cachePathFor(root string) string {
	return filepath.Join(statusCacheDir(), cacheKey(root)+".json")
}

// lockPathFor returns the refresh lock path for a repository root.
func lockPathFor(root string) string {
	return filepath.Join(statusCacheDir(), cacheKey(root)+".lock")
}

func statusCacheDir() string {
	return filepath.Join(platform.GetClaudeDir(), StatusCacheDir)
}

// applyStatusCache fills in the counts for a repository from the status cache
// and starts a refresh when what it holds is missing, stale, unreadable, or
// describes another branch or another repository. Nothing here waits for the
// refresh: this render shows what the last one measured.
func applyStatusCache(info *Info, root string) {
	if root == "" {
		return
	}

	entry, ok := readStatusCache(cachePathFor(root), root)
	if ok && entry.Branch == info.Branch {
		info.Ahead = entry.Ahead
		info.Behind = entry.Behind
		info.Changed = entry.Changed
		info.HasCounts = true

		age := time.Now().UnixMilli() - entry.Ts
		if age >= 0 && age < statusCacheTTLMs {
			return
		}
	}

	startRefresh(root)
}

// readStatusCache reads the cache file at path and reports whether it holds a
// usable entry for root. An unreadable, unparseable or foreign entry is not
// one, and the caller refreshes.
func readStatusCache(path, root string) (*statusEntry, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var entry statusEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, false
	}
	if entry.Root != root {
		return nil, false
	}
	return &entry, true
}

// writeStatusCache stores an entry atomically, so a render never reads a
// half-written file.
func writeStatusCache(path string, entry *statusEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-git-status-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

// startRefresh starts one detached refresh for root, and only one: the render
// that takes the lock spawns the refresher, every other render meanwhile sees
// the lock and renders what the cache already holds. The refresher releases
// the lock when it is done; a lock left behind by a killed refresher is
// reclaimed once it is refreshLockStaleness old.
func startRefresh(root string) {
	lock := lockPathFor(root)
	if !acquireRefreshLock(lock) {
		return
	}
	if err := refreshRunner(root); err != nil {
		// Nothing started, so nothing will release the lock.
		os.Remove(lock)
	}
}

// acquireRefreshLock creates the lock file and reports whether this caller now
// owns it.
func acquireRefreshLock(path string) bool {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false
	}
	if createLockFile(path) {
		return true
	}

	st, err := os.Stat(path)
	if err != nil || time.Since(st.ModTime()) < refreshLockStaleness {
		return false
	}
	os.Remove(path)
	return createLockFile(path)
}

func createLockFile(path string) bool {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// refreshRunner starts a detached refresh for a repository root. Tests replace
// it to watch what a render asks for without starting a process.
var refreshRunner = startDetachedRefresh

// refreshExecutable resolves the binary the detached refresh runs.
var refreshExecutable = os.Executable

// startDetachedRefresh starts howmuchleft --refresh-git-cache <root> and
// returns as soon as the child is running. The child is never waited for and
// gets no terminal: it outlives this render, writes the cache and exits.
func startDetachedRefresh(root string) error {
	exe, err := refreshExecutable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, RefreshFlag, root)
	cmd.Dir = root
	// nil stdio is /dev/null, so the child can neither read from nor write to
	// the terminal Claude Code is drawing the statusline on.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	detachChild(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// RefreshCache runs git status for a repository root and stores the result in
// the status cache, then releases the refresh lock. It is what the detached
// child started by a render runs, and it is the only place a git process is
// started: no render path calls it.
func RefreshCache(root string) error {
	if root == "" {
		return errors.New("refresh: no repository root given")
	}
	defer os.Remove(lockPathFor(root))

	ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git",
		"--no-optional-locks", "status", "--porcelain=v2", "--branch", "-unormal", "--no-renames",
	)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("refresh: git status in %s: %w", root, err)
	}

	entry := parseStatus(string(out))
	entry.Root = root
	entry.Ts = time.Now().UnixMilli()
	return writeStatusCache(cachePathFor(root), entry)
}

// parseStatus reads git status --porcelain=v2 --branch output into an entry.
func parseStatus(output string) *statusEntry {
	entry := &statusEntry{}

	for _, line := range strings.Split(output, "\n") {
		switch {
		case strings.HasPrefix(line, "# branch.head "):
			entry.Branch = strings.TrimPrefix(line, "# branch.head ")
		case strings.HasPrefix(line, "# branch.ab "):
			fields := strings.Fields(strings.TrimPrefix(line, "# branch.ab "))
			if len(fields) >= 1 {
				if v, err := strconv.Atoi(fields[0]); err == nil {
					entry.Ahead = v
				}
			}
			if len(fields) >= 2 {
				if v, err := strconv.Atoi(fields[1]); err == nil {
					entry.Behind = -v // porcelain reports behind as a negative number
				}
			}
		case len(line) >= 2:
			switch line[:2] {
			case "1 ", "2 ", "u ", "? ":
				entry.Changed++
			}
		}
	}

	if entry.Branch == "" {
		entry.Branch = detachedBranch
	}
	return entry
}
