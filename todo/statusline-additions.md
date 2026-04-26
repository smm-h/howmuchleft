# Statusline Additions

## Context

The current 3-line statusline covers usage pressure (context, 5hr, weekly), git branch + changes, model, profile, and directory. Four additional data points would round out the picture without adding visual clutter.

## Items

### 1. Cache freshness indicator

Show how old the displayed data is. Currently the `~` prefix signals stale data but doesn't say *how* stale. When the API is flaky, "data is 8s old" vs "data is 2m old" changes whether you trust the numbers.

- Where: next to the usage percentages, or as a dim suffix
- Data source: timestamp of last successful API response vs current time
- Files likely affected: `lib/statusline.js` (rendering), wherever API responses are cached
- Effort: Low

### 2. Concurrent session detection

Warn if multiple Claude Code sessions are running simultaneously. They share the same rate limits, so one session's usage projections are wrong when another is consuming quota in parallel.

- Where: a warning indicator on the status bar (e.g. "2 sessions" in yellow)
- Data source: scan for running CC processes via PID files in the profile directory, or `pgrep`
- Files likely affected: `lib/statusline.js` (rendering), new detection logic
- Effort: Low-Medium

### 3. Git remote status

Currently shows branch name and changed file count. Missing: ahead/behind upstream counts, which tell you whether you have unpushed commits or need to pull.

- Where: next to the existing branch name, e.g. `main +2/-1 [3 ahead, 1 behind]`
- Data source: `git status -b --porcelain=v2` already provides this (the `branch.ab` line)
- Files likely affected: wherever git status is currently parsed
- Effort: Low

### 4. Active GitHub username

Show which GitHub account is currently authenticated. Relevant when switching between personal and work accounts -- the wrong account can cause silent permission failures on private repos.

- Where: on one of the three lines, near the profile name or git branch
- Data source: `gh auth status --active` or `GH_TOKEN` + `gh api user`
- Files likely affected: `lib/statusline.js` (rendering), new GH detection logic
- Effort: Low
