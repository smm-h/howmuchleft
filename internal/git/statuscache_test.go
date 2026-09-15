package git

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// The status cache is refreshed by a detached child process, so the tests need
// a way to see that a refresh was started without actually starting one. Every
// test runs with a runner that does nothing; the tests that care about the
// spawn install their own.
func TestMain(m *testing.M) {
	buildTestBinary()
	refreshRunner = func(string) error { return nil }
	code := m.Run()
	if testBinaryDir != "" {
		os.RemoveAll(testBinaryDir)
	}
	os.Exit(code)
}

var (
	testBinaryDir  string
	testBinaryPath string
	testBinaryErr  error
	testBinaryOnce sync.Once
)

// buildTestBinary builds howmuchleft once, before any test poisons the
// environment, so the end-to-end spawn test can run the real binary. A failure
// is remembered and skips the test that needs it instead of failing the package.
func buildTestBinary() {
	testBinaryOnce.Do(func() {
		if _, err := exec.LookPath("go"); err != nil {
			testBinaryErr = err
			return
		}
		dir, err := os.MkdirTemp("", "howmuchleft-refresh-*")
		if err != nil {
			testBinaryErr = err
			return
		}
		testBinaryDir = dir
		out := filepath.Join(dir, "howmuchleft")
		cmd := exec.Command("go", "build", "-o", out, ".")
		cmd.Dir = "../.."
		if combined, err := cmd.CombinedOutput(); err != nil {
			testBinaryErr = err
			_ = combined
			return
		}
		testBinaryPath = out
	})
}

func requireTestBinary(t *testing.T) string {
	t.Helper()
	if testBinaryPath == "" {
		t.Skipf("howmuchleft could not be built for the refresh test: %v", testBinaryErr)
	}
	return testBinaryPath
}

// cacheFixture isolates the environment and points the Claude configuration
// directory at a throwaway directory, so every cache file a test writes or
// reads lives there.
func cacheFixture(t *testing.T) {
	t.Helper()
	isolate(t)
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
}

// recordRefreshes installs a runner that records the roots it was asked to
// refresh, and returns a function reading the recorded roots.
func recordRefreshes(t *testing.T) func() []string {
	t.Helper()
	var mu sync.Mutex
	var roots []string
	previous := refreshRunner
	refreshRunner = func(root string) error {
		mu.Lock()
		defer mu.Unlock()
		roots = append(roots, root)
		return nil
	}
	t.Cleanup(func() { refreshRunner = previous })
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), roots...)
	}
}

// writeEntry writes a cache entry for root, aged ageMs milliseconds.
func writeEntry(t *testing.T, entry statusEntry, ageMs int64) {
	t.Helper()
	entry.Ts = time.Now().UnixMilli() - ageMs
	path := cachePathFor(entry.Root)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir cache directory: %v", err)
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal cache entry: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write cache entry: %v", err)
	}
}

// readEntry reads the cache file written for root.
func readEntry(t *testing.T, root string) statusEntry {
	t.Helper()
	data, err := os.ReadFile(cachePathFor(root))
	if err != nil {
		t.Fatalf("read cache entry for %s: %v", root, err)
	}
	var entry statusEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("parse cache entry: %v", err)
	}
	return entry
}

// newUpstreamClone builds a clone that is one commit ahead of and one commit
// behind its upstream, and returns the clone's path.
func newUpstreamClone(t *testing.T) string {
	t.Helper()
	origin := newRepo(t, "main")
	parent := t.TempDir()
	clone := filepath.Join(parent, "clone")
	runGit(t, parent, "clone", "-q", origin, clone)

	if err := os.WriteFile(filepath.Join(clone, "local.txt"), []byte("local\n"), 0o600); err != nil {
		t.Fatalf("write local file: %v", err)
	}
	runGit(t, clone, "add", "local.txt")
	runGit(t, clone, "commit", "-q", "-m", "local commit")

	if err := os.WriteFile(filepath.Join(origin, "upstream.txt"), []byte("upstream\n"), 0o600); err != nil {
		t.Fatalf("write upstream file: %v", err)
	}
	runGit(t, origin, "add", "upstream.txt")
	runGit(t, origin, "commit", "-q", "-m", "upstream commit")
	runGit(t, clone, "fetch", "-q", "origin")

	return clone
}

// --- what the refresh subcommand writes -------------------------------------

func TestRefreshCache_AheadAndBehind(t *testing.T) {
	cacheFixture(t)
	clone := newUpstreamClone(t)

	before := time.Now().UnixMilli()
	if err := RefreshCache(clone); err != nil {
		t.Fatalf("RefreshCache: %v", err)
	}

	entry := readEntry(t, clone)
	if entry.Root != clone {
		t.Errorf("root: got %q, want %q", entry.Root, clone)
	}
	if entry.Branch != "main" {
		t.Errorf("branch: got %q, want %q", entry.Branch, "main")
	}
	if entry.Ahead != 1 {
		t.Errorf("ahead: got %d, want 1", entry.Ahead)
	}
	if entry.Behind != 1 {
		t.Errorf("behind: got %d, want 1", entry.Behind)
	}
	if entry.Changed != 0 {
		t.Errorf("changed: got %d, want 0 on a clean tree", entry.Changed)
	}
	if entry.Ts < before {
		t.Errorf("timestamp %d predates the refresh at %d", entry.Ts, before)
	}
}

func TestRefreshCache_DirtyTree(t *testing.T) {
	cacheFixture(t)
	repo := newRepo(t, "main")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("changed\n"), 0o600); err != nil {
		t.Fatalf("modify tracked file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("new\n"), 0o600); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}

	if err := RefreshCache(repo); err != nil {
		t.Fatalf("RefreshCache: %v", err)
	}

	if entry := readEntry(t, repo); entry.Changed != 2 {
		t.Errorf("changed: got %d, want 2 (one modified, one untracked)", entry.Changed)
	}
}

func TestRefreshCache_CleanTree(t *testing.T) {
	cacheFixture(t)
	repo := newRepo(t, "main")

	if err := RefreshCache(repo); err != nil {
		t.Fatalf("RefreshCache: %v", err)
	}

	entry := readEntry(t, repo)
	if entry.Changed != 0 {
		t.Errorf("changed: got %d, want 0", entry.Changed)
	}
	if entry.Branch != "main" {
		t.Errorf("branch: got %q, want %q", entry.Branch, "main")
	}
}

func TestRefreshCache_NoUpstream(t *testing.T) {
	cacheFixture(t)
	repo := newRepo(t, "main")

	if err := RefreshCache(repo); err != nil {
		t.Fatalf("RefreshCache: %v", err)
	}

	entry := readEntry(t, repo)
	if entry.Ahead != 0 || entry.Behind != 0 {
		t.Errorf("a branch with no upstream reported ahead=%d behind=%d, want 0 and 0",
			entry.Ahead, entry.Behind)
	}
}

func TestRefreshCache_NoRepository(t *testing.T) {
	cacheFixture(t)
	dir := t.TempDir()

	if err := RefreshCache(dir); err == nil {
		t.Error("expected an error for a directory outside a repository")
	}
	if _, err := os.Stat(cachePathFor(dir)); !os.IsNotExist(err) {
		t.Errorf("a cache file was written for a directory outside a repository: %v", err)
	}
}

func TestRefreshCache_ReleasesTheLock(t *testing.T) {
	cacheFixture(t)
	repo := newRepo(t, "main")
	lock := lockPathFor(repo)
	if err := os.MkdirAll(filepath.Dir(lock), 0o700); err != nil {
		t.Fatalf("mkdir cache directory: %v", err)
	}
	if err := os.WriteFile(lock, nil, 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	if err := RefreshCache(repo); err != nil {
		t.Fatalf("RefreshCache: %v", err)
	}

	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Errorf("the refresh left its lock behind: %v", err)
	}
}

func TestRefreshCache_ReleasesTheLockAfterAFailure(t *testing.T) {
	cacheFixture(t)
	dir := t.TempDir()
	lock := lockPathFor(dir)
	if err := os.MkdirAll(filepath.Dir(lock), 0o700); err != nil {
		t.Fatalf("mkdir cache directory: %v", err)
	}
	if err := os.WriteFile(lock, nil, 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	if err := RefreshCache(dir); err == nil {
		t.Fatal("expected an error for a directory outside a repository")
	}

	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Errorf("a failed refresh left its lock behind: %v", err)
	}
}

// --- what the render does with the cache ------------------------------------

func TestGetInfo_NoCacheRendersTheBranchAloneAndStartsARefresh(t *testing.T) {
	cacheFixture(t)
	repo := newUpstreamClone(t)
	refreshed := recordRefreshes(t)

	info := GetInfo(repo)

	if info.Branch != "main" || !info.HasGit {
		t.Fatalf("got branch %q HasGit=%v, want main and true", info.Branch, info.HasGit)
	}
	if info.HasCounts {
		t.Errorf("counts reported with no cache: ahead=%d behind=%d changed=%d",
			info.Ahead, info.Behind, info.Changed)
	}
	if got := refreshed(); len(got) != 1 || got[0] != repo {
		t.Errorf("refreshes started: %v, want exactly one for %s", got, repo)
	}
}

func TestGetInfo_FreshCacheIsUsedAndStartsNothing(t *testing.T) {
	cacheFixture(t)
	repo := newRepo(t, "main")
	writeEntry(t, statusEntry{Root: repo, Branch: "main", Ahead: 3, Behind: 2, Changed: 7}, 0)
	refreshed := recordRefreshes(t)

	info := GetInfo(repo)

	if !info.HasCounts {
		t.Fatal("a fresh cache was not used")
	}
	if info.Ahead != 3 || info.Behind != 2 || info.Changed != 7 {
		t.Errorf("counts: got ahead=%d behind=%d changed=%d, want 3, 2 and 7",
			info.Ahead, info.Behind, info.Changed)
	}
	if got := refreshed(); len(got) != 0 {
		t.Errorf("a fresh cache still started a refresh: %v", got)
	}
}

func TestGetInfo_StaleCacheStillRendersAndStartsARefresh(t *testing.T) {
	cacheFixture(t)
	repo := newRepo(t, "main")
	writeEntry(t, statusEntry{Root: repo, Branch: "main", Ahead: 1, Changed: 4}, statusCacheTTLMs+500)
	refreshed := recordRefreshes(t)

	info := GetInfo(repo)

	if !info.HasCounts || info.Ahead != 1 || info.Changed != 4 {
		t.Errorf("a stale cache must still be rendered: got HasCounts=%v ahead=%d changed=%d",
			info.HasCounts, info.Ahead, info.Changed)
	}
	if got := refreshed(); len(got) != 1 {
		t.Errorf("refreshes started: %v, want exactly one", got)
	}
}

func TestGetInfo_CacheFromTheFutureIsRefreshed(t *testing.T) {
	cacheFixture(t)
	repo := newRepo(t, "main")
	writeEntry(t, statusEntry{Root: repo, Branch: "main", Ahead: 1}, -60*1000)
	refreshed := recordRefreshes(t)

	GetInfo(repo)

	if got := refreshed(); len(got) != 1 {
		t.Errorf("a future-dated cache has an unknowable age and must be refreshed, started: %v", got)
	}
}

func TestGetInfo_CacheForAnotherBranchIsIgnored(t *testing.T) {
	cacheFixture(t)
	repo := newRepo(t, "main")
	writeEntry(t, statusEntry{Root: repo, Branch: "other", Ahead: 5, Changed: 9}, 0)
	refreshed := recordRefreshes(t)

	info := GetInfo(repo)

	if info.HasCounts {
		t.Errorf("counts for branch %q were rendered on branch %q", "other", info.Branch)
	}
	if got := refreshed(); len(got) != 1 {
		t.Errorf("a cache for another branch must start a refresh, started: %v", got)
	}
}

func TestGetInfo_CacheForAnotherRootIsIgnored(t *testing.T) {
	cacheFixture(t)
	repo := newRepo(t, "main")
	// Same cache file, a different repository root inside it: the entry does
	// not describe this repository and must not be rendered.
	entry := statusEntry{Root: repo, Branch: "main", Ahead: 5, Changed: 9}
	writeEntry(t, entry, 0)
	foreign := entry
	foreign.Root = filepath.Join(repo, "..", "elsewhere")
	data, err := json.Marshal(foreign)
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}
	if err := os.WriteFile(cachePathFor(repo), data, 0o600); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	refreshed := recordRefreshes(t)

	info := GetInfo(repo)

	if info.HasCounts {
		t.Error("an entry naming another repository root was rendered")
	}
	if got := refreshed(); len(got) != 1 {
		t.Errorf("an entry for another root must start a refresh, started: %v", got)
	}
}

func TestGetInfo_CorruptCacheIsIgnored(t *testing.T) {
	cacheFixture(t)
	repo := newRepo(t, "main")
	path := cachePathFor(repo)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir cache directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("write cache file: %v", err)
	}
	refreshed := recordRefreshes(t)

	info := GetInfo(repo)

	if info.HasCounts {
		t.Error("a corrupt cache file was rendered as counts")
	}
	if info.Branch != "main" {
		t.Errorf("branch: got %q, want main", info.Branch)
	}
	if got := refreshed(); len(got) != 1 {
		t.Errorf("a corrupt cache must start a refresh, started: %v", got)
	}
}

func TestGetInfo_OneRefreshAtATime(t *testing.T) {
	cacheFixture(t)
	repo := newRepo(t, "main")
	refreshed := recordRefreshes(t)

	GetInfo(repo)
	GetInfo(repo)
	GetInfo(repo)

	if got := refreshed(); len(got) != 1 {
		t.Errorf("refreshes started: %v, want exactly one while the first is in flight", got)
	}
}

func TestGetInfo_AbandonedLockIsReclaimed(t *testing.T) {
	cacheFixture(t)
	repo := newRepo(t, "main")
	lock := lockPathFor(repo)
	if err := os.MkdirAll(filepath.Dir(lock), 0o700); err != nil {
		t.Fatalf("mkdir cache directory: %v", err)
	}
	if err := os.WriteFile(lock, nil, 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	old := time.Now().Add(-refreshLockStaleness - time.Minute)
	if err := os.Chtimes(lock, old, old); err != nil {
		t.Fatalf("age the lock: %v", err)
	}
	refreshed := recordRefreshes(t)

	GetInfo(repo)

	if got := refreshed(); len(got) != 1 {
		t.Errorf("an abandoned lock must be reclaimed, refreshes started: %v", got)
	}
}

func TestGetInfo_OutsideARepositoryStartsNothing(t *testing.T) {
	cacheFixture(t)
	dir := t.TempDir()
	refreshed := recordRefreshes(t)

	info := GetInfo(dir)

	if info.HasGit {
		t.Error("expected HasGit=false outside a repository")
	}
	if got := refreshed(); len(got) != 0 {
		t.Errorf("a directory outside a repository started a refresh: %v", got)
	}
}

// The whole point of the cache: a checkout that has never been rendered shows
// its branch alone, the refresh it starts runs the real binary, and the next
// render shows the counts that refresh wrote.
func TestGetInfo_SecondRenderShowsTheCountsTheRefreshWrote(t *testing.T) {
	cacheFixture(t)
	binary := requireTestBinary(t)
	clone := newUpstreamClone(t)

	previousRunner := refreshRunner
	previousExecutable := refreshExecutable
	refreshRunner = startDetachedRefresh
	refreshExecutable = func() (string, error) { return binary, nil }
	t.Cleanup(func() {
		refreshRunner = previousRunner
		refreshExecutable = previousExecutable
	})

	first := GetInfo(clone)
	if first.HasCounts {
		t.Fatalf("the first render must show the branch alone, got ahead=%d behind=%d changed=%d",
			first.Ahead, first.Behind, first.Changed)
	}

	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(cachePathFor(clone)); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the detached refresh never wrote a cache file")
		}
		time.Sleep(20 * time.Millisecond)
	}

	second := GetInfo(clone)
	if !second.HasCounts {
		t.Fatal("the second render did not pick up the refreshed counts")
	}
	if second.Ahead != 1 || second.Behind != 1 {
		t.Errorf("counts: got ahead=%d behind=%d, want 1 and 1", second.Ahead, second.Behind)
	}
}
