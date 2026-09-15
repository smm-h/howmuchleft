package main

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// The selfdoc directive registered as statusline-comparison renders
// docs/statusline-comparison.toml into the table on docs/comparison.md. selfdoc
// loads the script in a Python process and calls resolve(attrs, config, body);
// this test does the same thing, so a results file missing a field or a script
// that stops producing a table fails here instead of on the docs site.
const directiveDriver = `
import runpy, sys
module = runpy.run_path("scripts/statusline-comparison-directive.py")
sys.stdout.write(module["resolve"]({}, {}, []))
`

func renderComparisonDirective(t *testing.T) string {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed; selfdoc needs it to resolve custom directives")
	}
	cmd := exec.Command(python, "-c", directiveDriver)
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("resolving the statusline-comparison directive failed: %v\n%s", err, out)
	}
	return string(out)
}

func TestComparisonDirectiveRendersATable(t *testing.T) {
	rendered := renderComparisonDirective(t)
	lines := strings.Split(strings.TrimSpace(rendered), "\n")
	if len(lines) < 4 {
		t.Fatalf("expected a table and a run line, got:\n%s", rendered)
	}

	wantHeader := "| Tool | Language | Median ms | Peak MB | 5-hour | Weekly | Transcript-free |"
	if lines[0] != wantHeader {
		t.Errorf("header row = %q, want %q", lines[0], wantHeader)
	}
	if !strings.HasPrefix(lines[1], "| ---") {
		t.Errorf("second row = %q, want a markdown separator row", lines[1])
	}
}

func TestComparisonDirectiveMarksThisProject(t *testing.T) {
	rendered := renderComparisonDirective(t)
	if !strings.Contains(rendered, "**howmuchleft** (this project)") {
		t.Errorf("howmuchleft's row is not marked as this project:\n%s", rendered)
	}
}

func TestComparisonDirectiveSortsByMedian(t *testing.T) {
	rendered := renderComparisonDirective(t)
	previous := -1.0
	rows := 0
	for _, line := range strings.Split(rendered, "\n") {
		if !strings.HasPrefix(line, "| ") || strings.HasPrefix(line, "| ---") ||
			strings.HasPrefix(line, "| Tool") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "| "), " | ")
		if len(cells) != 7 {
			t.Fatalf("row %q has %d cells, want 7", line, len(cells))
		}
		median, err := strconv.ParseFloat(strings.TrimSpace(cells[2]), 64)
		if err != nil {
			t.Fatalf("row %q has an unparseable median: %v", line, err)
		}
		if median < previous {
			t.Errorf("row %q breaks the fastest-first order (previous median %.1f)", line, previous)
		}
		previous = median
		rows++
	}
	if rows < 2 {
		t.Fatalf("expected a row per measured tool, got %d", rows)
	}
}

func TestComparisonDirectiveStatesTheRunConditions(t *testing.T) {
	rendered := renderComparisonDirective(t)
	runLine := strings.TrimSpace(rendered[strings.LastIndex(rendered, "\n\n")+1:])
	for _, want := range []string{"Measured on", "timed runs per tool", "/bin/true", "medians"} {
		if !strings.Contains(runLine, want) {
			t.Errorf("the run line does not mention %q:\n%s", want, runLine)
		}
	}
}
