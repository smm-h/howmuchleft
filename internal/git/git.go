package git

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Info holds parsed git status information for a working directory.
type Info struct {
	Branch  string
	Ahead   int
	Behind  int
	Changes int
	HasGit  bool
}

// GetInfo runs git status in cwd and returns parsed branch/change info.
// Returns HasGit=false on any error (not a repo, git missing, timeout).
func GetInfo(cwd string) *Info {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git",
		"--no-optional-locks", "status", "--porcelain=v2", "--branch", "-unormal", "--no-renames",
	)
	cmd.Dir = cwd

	out, err := cmd.Output()
	if err != nil {
		return &Info{HasGit: false}
	}

	return parseStatus(string(out))
}

// parseStatus parses git status --porcelain=v2 --branch output.
func parseStatus(output string) *Info {
	info := &Info{HasGit: true}

	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "# branch.head ") {
			info.Branch = line[len("# branch.head "):]
		} else if strings.HasPrefix(line, "# branch.ab ") {
			parts := strings.Fields(line[len("# branch.ab "):])
			if len(parts) >= 1 {
				if v, err := strconv.Atoi(parts[0]); err == nil {
					info.Ahead = v
				}
			}
			if len(parts) >= 2 {
				if v, err := strconv.Atoi(parts[1]); err == nil {
					info.Behind = -v // porcelain reports behind as negative
				}
			}
		} else if len(line) >= 2 {
			prefix := line[:2]
			if prefix == "1 " || prefix == "2 " || prefix == "u " || prefix == "? " {
				info.Changes++
			}
		}
	}

	// Default to "(detached)" if no branch header was found
	if info.Branch == "" {
		info.Branch = "(detached)"
	}

	return info
}
