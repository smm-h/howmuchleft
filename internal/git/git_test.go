package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smm-h/stricttest/go/hygiene"
)

func isolate(t *testing.T) {
	t.Helper()
	hygiene.Isolate(t, hygiene.Preserve(hygiene.GoPath, hygiene.GoModCache, hygiene.GoCache))
}

// runGit runs git in dir with a throwaway identity and fails the test when git
// itself fails, so a broken fixture never reads as a passing assertion.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{
		"-c", "user.name=howmuchleft-test",
		"-c", "user.email=howmuchleft-test@example.invalid",
		"-c", "commit.gpgsign=false",
		"-c", "protocol.file.allow=always",
	}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newRepo creates a repository with one commit on branch and returns its path.
func newRepo(t *testing.T, branch string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not installed: %v", err)
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", branch)
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello\n"), 0o600); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}
	runGit(t, dir, "add", "file.txt")
	runGit(t, dir, "commit", "-q", "-m", "initial")
	return dir
}

func TestGetInfo_NormalCheckout(t *testing.T) {
	isolate(t)
	repo := newRepo(t, "main")

	info := GetInfo(repo)
	if !info.HasGit {
		t.Fatal("expected HasGit=true inside a repository")
	}
	if info.Branch != "main" {
		t.Errorf("branch: got %q, want %q", info.Branch, "main")
	}
}

func TestGetInfo_BranchNameWithSlash(t *testing.T) {
	isolate(t)
	repo := newRepo(t, "main")
	runGit(t, repo, "checkout", "-q", "-b", "feature/nested/name")

	info := GetInfo(repo)
	if info.Branch != "feature/nested/name" {
		t.Errorf("branch: got %q, want %q", info.Branch, "feature/nested/name")
	}
}

func TestGetInfo_Subdirectory(t *testing.T) {
	isolate(t)
	repo := newRepo(t, "main")
	sub := filepath.Join(repo, "a", "b", "c")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	info := GetInfo(sub)
	if !info.HasGit {
		t.Fatal("expected HasGit=true in a subdirectory of a repository")
	}
	if info.Branch != "main" {
		t.Errorf("branch: got %q, want %q", info.Branch, "main")
	}
}

func TestGetInfo_DetachedHead(t *testing.T) {
	isolate(t)
	repo := newRepo(t, "main")
	sha := runGit(t, repo, "rev-parse", "HEAD")
	runGit(t, repo, "checkout", "-q", "--detach", sha)

	info := GetInfo(repo)
	if !info.HasGit {
		t.Fatal("expected HasGit=true on a detached HEAD")
	}
	if info.Branch != "(detached)" {
		t.Errorf("branch: got %q, want %q", info.Branch, "(detached)")
	}
}

func TestGetInfo_UnbornBranch(t *testing.T) {
	isolate(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not installed: %v", err)
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "trunk")

	info := GetInfo(dir)
	if !info.HasGit {
		t.Fatal("expected HasGit=true in a repository without commits")
	}
	if info.Branch != "trunk" {
		t.Errorf("branch: got %q, want %q", info.Branch, "trunk")
	}
}

func TestGetInfo_LinkedWorktree(t *testing.T) {
	isolate(t)
	repo := newRepo(t, "main")
	linked := filepath.Join(t.TempDir(), "linked")
	runGit(t, repo, "worktree", "add", "-q", "-b", "side", linked)

	st, err := os.Stat(filepath.Join(linked, ".git"))
	if err != nil {
		t.Fatalf("stat linked .git: %v", err)
	}
	if st.IsDir() {
		t.Fatalf("expected the linked worktree's .git to be a file, got a directory")
	}

	info := GetInfo(linked)
	if !info.HasGit {
		t.Fatal("expected HasGit=true inside a linked worktree")
	}
	if info.Branch != "side" {
		t.Errorf("branch: got %q, want %q", info.Branch, "side")
	}
}

func TestGetInfo_RelativeGitFile(t *testing.T) {
	isolate(t)
	repo := newRepo(t, "main")

	// A submodule's .git file names its gitdir relative to the directory the
	// file sits in, so the relative form has to resolve against that directory.
	nested := filepath.Join(repo, "nested")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, ".git"), []byte("gitdir: ../.git\n"), 0o600); err != nil {
		t.Fatalf("write .git file: %v", err)
	}

	info := GetInfo(nested)
	if !info.HasGit {
		t.Fatal("expected HasGit=true for a .git file with a relative gitdir")
	}
	if info.Branch != "main" {
		t.Errorf("branch: got %q, want %q", info.Branch, "main")
	}
}

func TestGetInfo_NoRepository(t *testing.T) {
	isolate(t)
	dir := t.TempDir()

	info := GetInfo(dir)
	if info.HasGit {
		t.Errorf("expected HasGit=false outside a repository, got branch %q", info.Branch)
	}
}

func TestGetInfo_EmptyCwd(t *testing.T) {
	isolate(t)

	info := GetInfo("")
	if info.HasGit {
		t.Error("expected HasGit=false for an empty working directory")
	}
}

func TestGetInfo_GitDirEnv(t *testing.T) {
	isolate(t)
	repo := newRepo(t, "main")
	elsewhere := t.TempDir()

	t.Setenv("GIT_DIR", filepath.Join(repo, ".git"))
	info := GetInfo(elsewhere)
	if !info.HasGit {
		t.Fatal("expected HasGit=true when GIT_DIR names a repository")
	}
	if info.Branch != "main" {
		t.Errorf("branch: got %q, want %q", info.Branch, "main")
	}
}

func TestGetInfo_UnreadableHead(t *testing.T) {
	isolate(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	info := GetInfo(dir)
	if info.HasGit {
		t.Error("expected HasGit=false when .git holds no HEAD")
	}
}

func TestParseHead(t *testing.T) {
	isolate(t)
	cases := []struct {
		name string
		head string
		want string
	}{
		{"branch", "ref: refs/heads/main\n", "main"},
		{"branch with slashes", "ref: refs/heads/feature/x\n", "feature/x"},
		{"no trailing newline", "ref: refs/heads/main", "main"},
		{"detached", "9fceb02d0ae598e95dc970b74767f19372d61af8\n", "(detached)"},
		{"empty", "", "(detached)"},
		{"ref with no target", "ref:\n", "(detached)"},
		{"symbolic ref elsewhere", "ref: refs/remotes/origin/main\n", "refs/remotes/origin/main"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseHead(tc.head); got != tc.want {
				t.Errorf("parseHead(%q) = %q, want %q", tc.head, got, tc.want)
			}
		})
	}
}
