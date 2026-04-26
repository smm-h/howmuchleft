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

Show which GitHub account is currently authenticated. Relevant when switching between personal and work accounts -- the wrong account can cause silent permission failures on private repos. The user already injects `GH_TOKEN` per profile via shell aliases (e.g. `cw`, `cwp`, `c`), so the env var is the cheap, reliable signal -- no need for `gh auth status --active` on the hot path.

- Where: line 1, near the profile name (both are identity signals)
- Trigger: only when `GH_TOKEN` is set in `process.env`; render nothing otherwise
- Resolution strategy:
  - Compute `tokenHash = sha256(GH_TOKEN).slice(0, 8)` (one-way, 32 bits stored)
  - Look up `cache.ghUsers[tokenHash]` in `.statusline-cache.json`
  - On cache miss, fork `gh api user --jq .login` once (inherits `GH_TOKEN` from env, so the token never enters argv); persist `{tokenHash: username}` and return
  - Cache is effectively permanent: tokens are stable and bound to one user
- Security:
  - Never store the raw token, only the truncated hash
  - Never use `curl -H "Authorization: token $GH_TOKEN"`-style invocation -- argv is visible in `ps`
  - Cache file should be mode `0600` (verify existing cache write path enforces this)
  - Never log the token or full API response on error paths
- Fallback: if `gh` is not installed or the API call fails, cache a sentinel (e.g. `null` with a short TTL) and render nothing
- Files likely affected: `lib/statusline.js` (rendering, cache schema, resolver)
- Effort: Low
