# Concurrent session detection

## Context

Multiple Claude Code sessions share the same rate limits (5-hour and weekly quotas). When two or more sessions run simultaneously, each session's usage projections are wrong because they don't account for the other session's consumption. Users have no way to know this from the statusline alone.

## Problem

A user with two active Claude Code sessions sees, say, 30% usage in each. But the actual combined usage is higher, and either session could unexpectedly hit the limit. There's no visual warning that quota is being split across sessions.

## Current state

`getSessionElapsed()` in `lib/statusline.js` already reads PID-based session files to calculate elapsed time, but only walks the process tree to find one session — it does not count concurrent sessions.

## Solution options

### Option A: PID file scanning

Scan Claude Code's session PID files in the profile directory. Count how many correspond to running processes.

- Pros: uses existing session file infrastructure, no external process calls
- Cons: depends on Claude Code's PID file format and location (could change), stale PID files from crashed sessions could cause false positives

### Option B: Process enumeration via pgrep

Use `pgrep -f "claude"` or similar to count running Claude Code processes.

- Pros: always reflects actual running processes, no stale file issues
- Cons: spawns a child process on every render (~3ms), pattern matching could false-positive on unrelated processes (e.g., this statusline tool itself), platform-dependent behavior

### Option C: Lock file with PID registry

Maintain a shared lock file (e.g., `~/.claude/.statusline-sessions.json`) where each invocation registers its parent PID and timestamp. Read the file to count active sessions, pruning entries whose PIDs are no longer running.

- Pros: explicit, no pattern matching, self-cleaning
- Cons: adds file I/O on every invocation, needs atomic read-modify-write, race conditions between concurrent writers

## Rendering

- Where: line 1 of the statusline, as a dim/yellow warning indicator
- Format: e.g., `2 sessions` or `+1 session` in yellow, only shown when count > 1
- Silent when only one session is active (the common case)

## Files likely affected

- `lib/statusline.js` — new detection function, rendering logic in `renderLines()`

## Effort

Low-Medium. The detection logic is straightforward regardless of approach. The main complexity is avoiding false positives and keeping the per-render cost low (the statusline is invoked on every render cycle).
