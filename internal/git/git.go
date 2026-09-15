// Package git reports the current branch by reading the repository's HEAD file
// directly. It starts at the working directory, walks up until it finds a .git
// directory or a .git file pointing at a worktree or submodule gitdir, and
// reads the branch name out of HEAD. No git process is started on the render
// path, so a render costs file reads instead of a fork, an exec and a status
// scan.
//
// The ahead/behind counts and the number of changed working-tree paths need
// the commit graph, the index and the ignore rules, which cannot be read
// without a git process. They come from a cache file instead: a render shows
// what the last git status measured, and when that measurement is older than
// statusCacheTTLMs the render starts a detached refresher and returns without
// waiting for it. The counts a render shows are therefore up to a couple of
// seconds behind the working tree.
package git

import (
	"os"
	"path/filepath"
	"strings"
)

// Info holds the git state the statusline renders for a working directory.
type Info struct {
	// Branch is the checked-out branch name, or "(detached)" when HEAD points
	// straight at a commit. Empty when HasGit is false.
	Branch string
	// HasGit is true when cwd is inside a git repository whose HEAD was read.
	HasGit bool
	// Ahead and Behind are the commit counts between the branch and its
	// upstream, and Changed is the number of changed working-tree paths, all
	// taken from the status cache the refresh subcommand writes. They are zero
	// when no cached status applies to the checked-out branch.
	Ahead   int
	Behind  int
	Changed int
	// HasCounts is true when Ahead, Behind and Changed come from a cached
	// status for this branch, false when no such status was available.
	HasCounts bool
}

// detachedBranch is what Branch reads when HEAD names a commit, not a branch.
// It is spelled the way git status --porcelain=v2 spells it.
const detachedBranch = "(detached)"

// GetInfo resolves the repository containing cwd, reads its current branch out
// of HEAD and fills in the counts from the status cache, starting a detached
// refresh when that cache no longer describes the branch it is on. Returns
// HasGit=false when cwd is not inside a repository or HEAD is missing or
// unreadable.
func GetInfo(cwd string) *Info {
	root, gitDir := findRepo(cwd)
	if gitDir == "" {
		return &Info{HasGit: false}
	}

	head, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return &Info{HasGit: false}
	}

	info := &Info{HasGit: true, Branch: parseHead(string(head))}
	applyStatusCache(info, root)
	return info
}

// findRepo returns the working-tree root containing cwd and the repository's
// git directory, or two empty strings when cwd is not inside a repository. The
// root is what git status is run in and what the status cache is keyed by; the
// git directory is where HEAD is read. GIT_DIR wins when it is set, the way
// git itself treats it, and GIT_WORK_TREE then names the root; otherwise the
// search walks up from cwd.
func findRepo(cwd string) (root, gitDir string) {
	if env := os.Getenv("GIT_DIR"); env != "" {
		root = os.Getenv("GIT_WORK_TREE")
		if root == "" {
			root = cwd
		}
		return root, env
	}
	if cwd == "" {
		return "", ""
	}

	dir := cwd
	for {
		candidate := filepath.Join(dir, ".git")
		st, err := os.Stat(candidate)
		switch {
		case err != nil:
			// keep walking up
		case st.IsDir():
			return dir, candidate
		default:
			// A .git file: a linked worktree or a submodule. Its single
			// "gitdir: <path>" line points at the real git directory, and a
			// relative path there is relative to the directory holding the file.
			if target := parseGitFile(candidate); target != "" {
				if !filepath.IsAbs(target) {
					target = filepath.Join(dir, target)
				}
				return dir, target
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ""
		}
		dir = parent
	}
}

// parseGitFile reads a .git file and returns the gitdir path it names, or ""
// when the file is unreadable or does not carry a gitdir line.
func parseGitFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "gitdir:"); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// parseHead turns the contents of a HEAD file into a branch name. A symbolic
// ref to refs/heads/<name> yields <name>; a symbolic ref anywhere else yields
// the ref as written; a raw object id yields "(detached)".
func parseHead(head string) string {
	head = strings.TrimSpace(head)

	rest, ok := strings.CutPrefix(head, "ref:")
	if !ok {
		return detachedBranch
	}

	ref := strings.TrimSpace(rest)
	if ref == "" {
		return detachedBranch
	}
	if name, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
		return name
	}
	return ref
}
