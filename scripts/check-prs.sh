#!/usr/bin/env bash
# Print open PRs for the current repo, if any.
# Exits 0 regardless — safe to use in hooks/prompts without blocking.

repo_root=$(git rev-parse --show-toplevel 2>/dev/null) || exit 0
cd "$repo_root" || exit 0

count=$(gh pr list --state open --json number --jq length 2>/dev/null) || exit 0
if [ "$count" -gt 0 ]; then
  echo "[$count open PR(s) in $(basename "$repo_root")]"
  gh pr list --state open 2>/dev/null
fi
