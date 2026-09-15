// Package git reports the current branch by reading the repository's HEAD file
// directly. It starts at the working directory, walks up until it finds a .git
// directory or a .git file pointing at a worktree or submodule gitdir, and
// reads the branch name out of HEAD. No git process is started, so a render
// costs two file reads instead of a fork, an exec and a status scan.
//
// Only the branch name is reported. Ahead/behind counts need the commit graph
// and a working-tree change count needs the index, the ignore rules and an
// lstat per tracked path -- work that cannot be done without either a git
// process or a reimplementation of git, so the statusline does not show them.
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
}

// detachedBranch is what Branch reads when HEAD names a commit, not a branch.
// It is spelled the way git status --porcelain=v2 spells it.
const detachedBranch = "(detached)"

// GetInfo resolves the repository containing cwd and reads its current branch.
// Returns HasGit=false when cwd is not inside a repository or HEAD is missing
// or unreadable.
func GetInfo(cwd string) *Info {
	gitDir := findGitDir(cwd)
	if gitDir == "" {
		return &Info{HasGit: false}
	}

	head, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return &Info{HasGit: false}
	}

	return &Info{HasGit: true, Branch: parseHead(string(head))}
}

// findGitDir returns the repository's git directory for cwd, or "" when cwd is
// not inside a repository. GIT_DIR wins when it is set, the way git itself
// treats it; otherwise the search walks up from cwd.
func findGitDir(cwd string) string {
	if env := os.Getenv("GIT_DIR"); env != "" {
		return env
	}
	if cwd == "" {
		return ""
	}

	dir := cwd
	for {
		candidate := filepath.Join(dir, ".git")
		st, err := os.Stat(candidate)
		switch {
		case err != nil:
			// keep walking up
		case st.IsDir():
			return candidate
		default:
			// A .git file: a linked worktree or a submodule. Its single
			// "gitdir: <path>" line points at the real git directory, and a
			// relative path there is relative to the directory holding the file.
			if target := parseGitFile(candidate); target != "" {
				if !filepath.IsAbs(target) {
					target = filepath.Join(dir, target)
				}
				return target
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
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
