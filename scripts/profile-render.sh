#!/usr/bin/env bash
#
# Profile a single howmuchleft render against the same fixture the statusline
# comparison harness uses: a throwaway git working directory, a hermetic HOME,
# and the synthetic status object on stdin.
#
#   scripts/profile-render.sh --setup                     # build the fixture, print its path
#   scripts/profile-render.sh --fixture DIR --bench BIN    # median/min/max over --runs runs
#   scripts/profile-render.sh --fixture DIR --strace BIN   # syscall and subprocess summary
#   scripts/profile-render.sh --fixture DIR --once BIN     # one run, output to stdout
#
# The fixture lives outside the repository (mktemp -d), because a nested git
# repository inside the checkout would be picked up by repo-integrity walks.
# Delete it when done; nothing else is written outside it.

set -euo pipefail

SELF_NAME="$(basename "${BASH_SOURCE[0]}")"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

MODE=""
BINARY=""
FIXTURE=""
RUNS=200

usage() {
  cat <<EOF
usage: $SELF_NAME --setup
       $SELF_NAME --fixture DIR (--bench BIN | --strace BIN | --once BIN) [--runs N]

  --setup        create the fixture directory and print its path
  --fixture DIR  a fixture directory created by --setup
  --bench BIN    time N runs of BIN and print median, min, max and mean
  --strace BIN   run BIN once under strace -f -c and print the summary
  --once BIN     run BIN once and print its output
  --runs N       timed runs for --bench (default $RUNS)
EOF
}

die() {
  echo "$SELF_NAME: $*" >&2
  exit 1
}

while [ $# -gt 0 ]; do
  case "$1" in
    --setup) MODE="setup" ;;
    --bench | --strace | --once)
      MODE="${1#--}"
      shift
      [ $# -gt 0 ] || die "--$MODE needs a path to a binary"
      BINARY="$1"
      ;;
    --fixture)
      shift
      [ $# -gt 0 ] || die "--fixture needs a directory"
      FIXTURE="$1"
      ;;
    --runs)
      shift
      [ $# -gt 0 ] || die "--runs needs a number"
      RUNS="$1"
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      die "unknown argument: $1"
      ;;
  esac
  shift
done

[ -n "$MODE" ] || {
  usage >&2
  die "choose --setup, --bench, --strace or --once"
}

if [ "$MODE" = "setup" ]; then
  [ -z "$FIXTURE" ] || die "--setup creates its own directory, drop --fixture"

  SCRATCH="$(mktemp -d -t howmuchleft-profile.XXXXXXXX)"
  FAKEHOME="$SCRATCH/home"
  WORK="$SCRATCH/work"
  mkdir -p "$FAKEHOME/.config" "$FAKEHOME/.claude" "$WORK"

  git -C "$WORK" init -q
  git -C "$WORK" -c user.name=profile -c user.email=profile@example.invalid \
    commit -q --allow-empty -m "profile fixture"
  printf 'fixture\n' >"$WORK/fixture.txt"

  SESSION_ID="abc123de-4567-89ab-cdef-0123456789ab"
  TRANSCRIPT="$SCRATCH/transcript.jsonl"
  : >"$TRANSCRIPT"
  for i in $(seq 0 199); do
    printf '{"type":"assistant","timestamp":"2026-09-14T22:%02d:00.000Z","sessionId":"%s","requestId":"req_%d","message":{"id":"msg_%d","type":"message","role":"assistant","model":"claude-opus-4-5-20251101","usage":{"input_tokens":%d,"output_tokens":50,"cache_creation_input_tokens":1200,"cache_read_input_tokens":60000}}}\n' \
      "$((i % 60))" "$SESSION_ID" "$i" "$i" "$((100 + i))" >>"$TRANSCRIPT"
  done
  PROJECT_KEY="$(echo "$WORK" | tr '/' '-')"
  mkdir -p "$FAKEHOME/.claude/projects/$PROJECT_KEY"
  cp "$TRANSCRIPT" "$FAKEHOME/.claude/projects/$PROJECT_KEY/$SESSION_ID.jsonl"

  cat >"$SCRATCH/payload.json" <<EOF
{
  "hook_event_name": "Status",
  "session_id": "$SESSION_ID",
  "transcript_path": "$TRANSCRIPT",
  "cwd": "$WORK",
  "model": { "id": "claude-opus-4-5-20251101", "display_name": "Opus 4.5" },
  "workspace": { "current_dir": "$WORK", "project_dir": "$WORK" },
  "version": "2.0.30",
  "output_style": { "name": "default" },
  "context_window": {
    "used_tokens": 84213,
    "max_tokens": 200000,
    "used_percentage": 42.1,
    "total_input_tokens": 84213,
    "total_output_tokens": 5120,
    "total_cache_creation_input_tokens": 12000,
    "total_cache_read_input_tokens": 60000,
    "input_tokens": 84213,
    "output_tokens": 5120,
    "cache_creation_input_tokens": 12000,
    "cache_read_input_tokens": 60000,
    "context_window_size": 200000
  },
  "cost": {
    "total_cost_usd": 1.2345,
    "total_duration_ms": 512000,
    "total_api_duration_ms": 91000,
    "total_lines_added": 173,
    "total_lines_removed": 42,
    "total_input_tokens": 84213,
    "total_output_tokens": 5120,
    "total_cache_creation_input_tokens": 12000,
    "total_cache_read_input_tokens": 60000
  },
  "rate_limits": {
    "five_hour": { "used_percentage": 37.4, "resets_at": 1789436219 },
    "seven_day": { "used_percentage": 61.2, "resets_at": 1789807219 },
    "seven_day_overage_included": { "used_percentage": 12.0, "resets_at": 1789807219 },
    "extra_usage": { "is_enabled": false, "utilization": 0 }
  },
  "context_window_size": 200000
}
EOF

  echo "$SCRATCH"
  exit 0
fi

[ -n "$FIXTURE" ] || die "--$MODE needs --fixture DIR (make one with --setup)"
[ -d "$FIXTURE/work" ] || die "not a fixture directory: $FIXTURE"
[ -x "$BINARY" ] || die "not an executable: $BINARY"
BINARY="$(cd "$(dirname "$BINARY")" && pwd)/$(basename "$BINARY")"

FAKEHOME="$FIXTURE/home"
WORK="$FIXTURE/work"
PAYLOAD="$FIXTURE/payload.json"

TOOL_ENV=(
  env -i
  "PATH=$PATH"
  "HOME=$FAKEHOME"
  "XDG_CONFIG_HOME=$FAKEHOME/.config"
  "XDG_CACHE_HOME=$FAKEHOME/.cache"
  "XDG_DATA_HOME=$FAKEHOME/.local/share"
  "CLAUDE_CONFIG_DIR=$FAKEHOME/.claude"
  "TERM=xterm-256color"
)

cd "$WORK"

case "$MODE" in
  once)
    "${TOOL_ENV[@]}" "$BINARY" <"$PAYLOAD"
    ;;
  strace)
    command -v strace >/dev/null 2>&1 || die "strace not found"
    "${TOOL_ENV[@]}" "$BINARY" <"$PAYLOAD" >/dev/null 2>&1 || true
    strace -f -c -o "$FIXTURE/strace.txt" \
      "${TOOL_ENV[@]}" "$BINARY" <"$PAYLOAD" >/dev/null 2>/dev/null || true
    cat "$FIXTURE/strace.txt"
    ;;
  bench)
    case "$RUNS" in
      '' | *[!0-9]*) die "--runs needs a positive integer, got: $RUNS" ;;
    esac
    [ "$RUNS" -gt 0 ] || die "--runs needs a positive integer, got: $RUNS"

    for _ in 1 2 3 4 5; do
      "${TOOL_ENV[@]}" "$BINARY" <"$PAYLOAD" >/dev/null 2>&1 || true
    done

    TIMES="$FIXTURE/times.txt"
    : >"$TIMES"
    for ((i = 0; i < RUNS; i++)); do
      start="$(date +%s%N)"
      "${TOOL_ENV[@]}" "$BINARY" <"$PAYLOAD" >/dev/null 2>&1 || true
      end="$(date +%s%N)"
      awk -v ns="$((end - start))" 'BEGIN { printf "%.3f\n", ns / 1000000 }' >>"$TIMES"
    done

    sort -n "$TIMES" | awk -v bin="$BINARY" '
      { v[NR] = $1; sum += $1 }
      END {
        median = (NR % 2) ? v[(NR + 1) / 2] : (v[NR / 2] + v[NR / 2 + 1]) / 2
        printf "%s\n  runs %d  median %.2f ms  mean %.2f ms  min %.2f ms  max %.2f ms\n",
          bin, NR, median, sum / NR, v[1], v[NR]
      }'
    ;;
esac

cd "$REPO_ROOT"
