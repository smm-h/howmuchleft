package git

import "testing"

func TestParseStatus_Normal(t *testing.T) {
	output := `# branch.oid abc123def456
# branch.head main
# branch.upstream origin/main
# branch.ab +3 -1
1 .M N... 100644 100644 100644 abc123 def456 lib/statusline.js
? untracked-file.txt
`
	info := parseStatus(output)

	if !info.HasGit {
		t.Fatal("expected HasGit=true")
	}
	if info.Branch != "main" {
		t.Errorf("branch: got %q, want %q", info.Branch, "main")
	}
	if info.Ahead != 3 {
		t.Errorf("ahead: got %d, want 3", info.Ahead)
	}
	if info.Behind != 1 {
		t.Errorf("behind: got %d, want 1", info.Behind)
	}
	if info.Changes != 2 {
		t.Errorf("changes: got %d, want 2", info.Changes)
	}
}

func TestParseStatus_DetachedHead(t *testing.T) {
	output := `# branch.oid abc123def456
# branch.head (detached)
`
	info := parseStatus(output)

	if !info.HasGit {
		t.Fatal("expected HasGit=true")
	}
	if info.Branch != "(detached)" {
		t.Errorf("branch: got %q, want %q", info.Branch, "(detached)")
	}
	if info.Ahead != 0 {
		t.Errorf("ahead: got %d, want 0", info.Ahead)
	}
	if info.Behind != 0 {
		t.Errorf("behind: got %d, want 0", info.Behind)
	}
}

func TestParseStatus_NoUpstream(t *testing.T) {
	output := `# branch.oid abc123def456
# branch.head feature-branch
1 .M N... 100644 100644 100644 abc123 def456 file.go
`
	info := parseStatus(output)

	if !info.HasGit {
		t.Fatal("expected HasGit=true")
	}
	if info.Branch != "feature-branch" {
		t.Errorf("branch: got %q, want %q", info.Branch, "feature-branch")
	}
	if info.Ahead != 0 {
		t.Errorf("ahead: got %d, want 0", info.Ahead)
	}
	if info.Behind != 0 {
		t.Errorf("behind: got %d, want 0", info.Behind)
	}
	if info.Changes != 1 {
		t.Errorf("changes: got %d, want 1", info.Changes)
	}
}

func TestParseStatus_InitialBranch(t *testing.T) {
	output := `# branch.oid (initial)
# branch.head (initial)
? new-file.go
`
	info := parseStatus(output)

	if !info.HasGit {
		t.Fatal("expected HasGit=true")
	}
	if info.Branch != "(initial)" {
		t.Errorf("branch: got %q, want %q", info.Branch, "(initial)")
	}
	if info.Changes != 1 {
		t.Errorf("changes: got %d, want 1", info.Changes)
	}
}

func TestParseStatus_EmptyOutput(t *testing.T) {
	info := parseStatus("")

	if !info.HasGit {
		t.Fatal("expected HasGit=true")
	}
	if info.Branch != "(detached)" {
		t.Errorf("branch: got %q, want %q", info.Branch, "(detached)")
	}
	if info.Changes != 0 {
		t.Errorf("changes: got %d, want 0", info.Changes)
	}
}

func TestParseStatus_AllChangeTypes(t *testing.T) {
	output := `# branch.oid abc123
# branch.head main
# branch.upstream origin/main
# branch.ab +0 -0
1 M. N... 100644 100644 100644 abc123 def456 modified.go
2 R. N... 100644 100644 100644 abc123 def456 R100 new.go	old.go
u UU N... 100644 100644 100644 100644 abc123 def456 789abc conflict.go
? untracked.txt
`
	info := parseStatus(output)

	if info.Changes != 4 {
		t.Errorf("changes: got %d, want 4", info.Changes)
	}
}

func TestGetInfo_NotARepo(t *testing.T) {
	info := GetInfo("/")

	if info.HasGit {
		t.Error("expected HasGit=false for non-repo directory")
	}
}
