#!/usr/bin/env bash
#
# Benchmark howmuchleft against other Claude Code statuslines.
#
# Every tool is fed the same synthetic status object on stdin, from the same
# working directory, with HOME, XDG_CONFIG_HOME and CLAUDE_CONFIG_DIR pointed
# into a throwaway scratch directory, so no tool can reach the real Claude Code
# state and none of them can write outside the scratch directory.
#
#   scripts/compare-statuslines.sh --dry-run          # print the plan, touch nothing
#   scripts/compare-statuslines.sh --run              # install, measure, write results
#   scripts/compare-statuslines.sh --run --runs 100   # more timed runs per tool
#
# The results go to docs/statusline-comparison.toml, which the selfdoc
# directive `statusline-comparison` renders into docs/comparison.md.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RESULTS_FILE="$REPO_ROOT/docs/statusline-comparison.toml"
SELF_NAME="$(basename "${BASH_SOURCE[0]}")"

# Pinned competitor versions. Changing a version here is the only place a
# version needs editing: the download URLs and the results file both read it.
CSHIP_VERSION="1.8.3"
CCLINE_VERSION="1.1.2"
BCH_VERSION="0.1.11"
CCSP_VERSION="4.1.1"
CCUSAGE_VERSION="20.0.20"
CCSTATUSLINE_VERSION="2.2.29"
POWERLINE_VERSION="1.31.0"

CSHIP_REPO="stephenleo/cship"
CCLINE_REPO="Haleclipse/CCometixLine"
BCH_REPO="GaoSSR/best-claude-hud"
CCSP_REPO="Wangnov/claude-code-statusline-pro"

CSHIP_ASSET="cship-x86_64-unknown-linux-musl"
CCLINE_ASSET="ccline-linux-x64-static.tar.gz"
BCH_ASSET="best-claude-hud-linux-x64-musl.tar.gz"
CCSP_ASSET="claude-code-statusline-pro-x86_64-unknown-linux-musl.tar.xz"

POWERLINE_PKG="@owloops/claude-powerline"

RUNS=50
WARMUP=5
RSS_RUNS=3
MODE=""

usage() {
  cat <<EOF
usage: $SELF_NAME (--dry-run | --run) [--runs N]

  --dry-run   print what would be installed and measured, change nothing
  --run       install the tools into a scratch directory, measure, and write
              $RESULTS_FILE
  --runs N    timed runs per tool (default $RUNS)
EOF
}

die() {
  echo "$SELF_NAME: $*" >&2
  exit 1
}

while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) MODE="dry" ;;
    --run) MODE="run" ;;
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
  die "choose --dry-run or --run"
}
case "$RUNS" in
  '' | *[!0-9]*) die "--runs needs a positive integer, got: $RUNS" ;;
esac
[ "$RUNS" -gt 0 ] || die "--runs needs a positive integer, got: $RUNS"

# Refuse before doing anything when a required program is missing, naming it.
for prog in go npm gh /usr/bin/time tar xz git; do
  command -v "$prog" >/dev/null 2>&1 || die "required program not found: $prog"
done

HOWMUCHLEFT_VERSION="$(cat "$REPO_ROOT/VERSION")"
HOWMUCHLEFT_COMMIT="$(git -C "$REPO_ROOT" rev-parse --short HEAD)"

# One record per benchmarked tool, in declaration order:
#   id|display name|version|language|source|5-hour|weekly|reads transcripts
# The three capability fields are declared, not measured: "yes" and "no" say
# whether the tool renders that limit at all, "api" means it does not take the
# number from the status object on stdin but fetches it from the Anthropic API.
TOOLS=(
  "howmuchleft|howmuchleft|$HOWMUCHLEFT_VERSION|Go|built from this checkout|yes|yes|no"
  "cship|cship|$CSHIP_VERSION|Rust|GitHub release $CSHIP_REPO, $CSHIP_ASSET|yes|yes|no"
  "ccline|CCometixLine (ccline)|$CCLINE_VERSION|Rust|GitHub release $CCLINE_REPO, $CCLINE_ASSET|api|api|yes"
  "best-claude-hud|best-claude-hud|$BCH_VERSION|Rust|GitHub release $BCH_REPO, $BCH_ASSET|yes|no|no"
  "claude-code-statusline-pro|claude-code-statusline-pro|$CCSP_VERSION|Rust|GitHub release $CCSP_REPO, $CCSP_ASSET|yes|yes|yes"
  "ccusage|ccusage statusline|$CCUSAGE_VERSION|Rust behind an npm shim|npm package ccusage|no|no|yes"
  "ccstatusline|ccstatusline|$CCSTATUSLINE_VERSION|TypeScript on Bun|npm package ccstatusline|yes|yes|no"
  "claude-powerline|claude-powerline|$POWERLINE_VERSION|TypeScript on Node|npm package $POWERLINE_PKG|yes|yes|yes"
)

field() { echo "$1" | cut -d'|' -f"$2"; }

if [ "$MODE" = "dry" ]; then
  echo "$SELF_NAME --run would:"
  echo
  echo "  1. create a scratch directory with mktemp -d, and put every download,"
  echo "     every config file and every tool's HOME inside it"
  echo "  2. build howmuchleft $HOWMUCHLEFT_VERSION (commit $HOWMUCHLEFT_COMMIT) from $REPO_ROOT"
  echo "  3. download these release assets with gh:"
  echo "       $CSHIP_REPO v$CSHIP_VERSION  $CSHIP_ASSET"
  echo "       $CCLINE_REPO v$CCLINE_VERSION  $CCLINE_ASSET"
  echo "       $BCH_REPO v$BCH_VERSION  $BCH_ASSET"
  echo "       $CCSP_REPO v$CCSP_VERSION  $CCSP_ASSET"
  echo "  4. install these npm packages under <scratch>/npm (never globally):"
  echo "       ccusage@$CCUSAGE_VERSION"
  echo "       ccstatusline@$CCSTATUSLINE_VERSION"
  echo "       $POWERLINE_PKG@$POWERLINE_VERSION"
  echo "  5. write the synthetic status object, a 200-line synthetic transcript,"
  echo "     and the config files cship, ccline and best-claude-hud need"
  echo "  6. run each tool $WARMUP times to warm the caches, then time $RUNS runs of:"
  for record in "${TOOLS[@]}"; do
    echo "       $(field "$record" 2)"
  done
  echo "     and measure the /bin/true spawn floor the same way"
  echo "  7. measure peak RSS over $RSS_RUNS more runs per tool with /usr/bin/time -f '%M'"
  echo "  8. write $RESULTS_FILE"
  echo
  echo "Nothing outside the scratch directory and $RESULTS_FILE is written."
  exit 0
fi

SCRATCH="$(mktemp -d -t howmuchleft-compare.XXXXXXXX)"
BIN="$SCRATCH/bin"
FAKEHOME="$SCRATCH/home"
WORK="$SCRATCH/work"
TIMES="$SCRATCH/times"
mkdir -p "$BIN" "$FAKEHOME/.config" "$FAKEHOME/.claude" "$WORK" "$TIMES"

echo "$SELF_NAME: scratch directory $SCRATCH"

# --- the working directory every tool runs in -------------------------------
# A throwaway git repository, so the tools that render a branch and a diff do
# that work during the measurement instead of bailing out early.
git -C "$WORK" init -q
git -C "$WORK" -c user.name=compare -c user.email=compare@example.invalid \
  commit -q --allow-empty -m "statusline comparison fixture"
printf 'fixture\n' >"$WORK/fixture.txt"

SESSION_ID="abc123de-4567-89ab-cdef-0123456789ab"
TRANSCRIPT="$SCRATCH/transcript.jsonl"
PAYLOAD="$SCRATCH/payload.json"

# --- the synthetic transcript -----------------------------------------------
: >"$TRANSCRIPT"
for i in $(seq 0 199); do
  printf '{"type":"assistant","timestamp":"2026-09-14T22:%02d:00.000Z","sessionId":"%s","requestId":"req_%d","message":{"id":"msg_%d","type":"message","role":"assistant","model":"claude-opus-4-5-20251101","usage":{"input_tokens":%d,"output_tokens":50,"cache_creation_input_tokens":1200,"cache_read_input_tokens":60000}}}\n' \
    "$((i % 60))" "$SESSION_ID" "$i" "$i" "$((100 + i))" >>"$TRANSCRIPT"
done
# Tools that discover transcripts themselves look under the Claude projects
# directory, keyed by the working directory with slashes turned into dashes.
PROJECT_KEY="$(echo "$WORK" | tr '/' '-')"
mkdir -p "$FAKEHOME/.claude/projects/$PROJECT_KEY"
cp "$TRANSCRIPT" "$FAKEHOME/.claude/projects/$PROJECT_KEY/$SESSION_ID.jsonl"

# --- the synthetic status object --------------------------------------------
cat >"$PAYLOAD" <<EOF
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

# --- install ----------------------------------------------------------------
echo "$SELF_NAME: building howmuchleft $HOWMUCHLEFT_VERSION from $REPO_ROOT"
(cd "$REPO_ROOT" && go build -ldflags "-X main.Version=$HOWMUCHLEFT_VERSION" -o "$BIN/howmuchleft" .)

fetch_release() {
  local repo="$1" tag="$2" asset="$3" dest="$4"
  mkdir -p "$dest"
  echo "$SELF_NAME: downloading $repo $tag $asset"
  gh release download "$tag" --repo "$repo" --pattern "$asset" --dir "$dest" --clobber
}

fetch_release "$CSHIP_REPO" "v$CSHIP_VERSION" "$CSHIP_ASSET" "$SCRATCH/cship"
chmod +x "$SCRATCH/cship/$CSHIP_ASSET"

fetch_release "$CCLINE_REPO" "v$CCLINE_VERSION" "$CCLINE_ASSET" "$SCRATCH/ccline"
tar -xzf "$SCRATCH/ccline/$CCLINE_ASSET" -C "$SCRATCH/ccline"

fetch_release "$BCH_REPO" "v$BCH_VERSION" "$BCH_ASSET" "$SCRATCH/best-claude-hud"
tar -xzf "$SCRATCH/best-claude-hud/$BCH_ASSET" -C "$SCRATCH/best-claude-hud"

fetch_release "$CCSP_REPO" "v$CCSP_VERSION" "$CCSP_ASSET" "$SCRATCH/ccsp"
tar -xJf "$SCRATCH/ccsp/$CCSP_ASSET" -C "$SCRATCH/ccsp"

echo "$SELF_NAME: installing npm packages under $SCRATCH/npm"
mkdir -p "$SCRATCH/npm"
npm_config_cache="$SCRATCH/npm-cache" npm install --prefix "$SCRATCH/npm" \
  --no-audit --no-fund --loglevel=error \
  "ccusage@$CCUSAGE_VERSION" "ccstatusline@$CCSTATUSLINE_VERSION" \
  "$POWERLINE_PKG@$POWERLINE_VERSION"

# Binaries land at different depths depending on how each project packages its
# archive, so resolve each one by name instead of assuming a layout.
find_binary() {
  local dir="$1" name="$2" found
  found="$(find "$dir" -type f -name "$name" -perm -u+x -print -quit)"
  [ -n "$found" ] || die "no executable named $name under $dir"
  echo "$found"
}

CSHIP_BIN="$SCRATCH/cship/$CSHIP_ASSET"
CCLINE_BIN="$(find_binary "$SCRATCH/ccline" ccline)"
BCH_BIN="$(find_binary "$SCRATCH/best-claude-hud" best-claude-hud)"
CCSP_BIN="$(find_binary "$SCRATCH/ccsp" claude-code-statusline-pro)"
NPM_BIN="$SCRATCH/npm/node_modules/.bin"

# --- per-tool config --------------------------------------------------------
# cship prints nothing at all without a config, and renders no usage limits
# unless its line declares them. Only native $cship.* modules are asked for:
# cship's directory and git segments are Starship passthrough modules, and this
# harness does not install Starship, so asking for them would silently render
# empty instead of doing the work.
cat >"$FAKEHOME/.config/cship.toml" <<'EOF'
[cship]
lines = ["$cship.model | $cship.context_bar | $cship.cost | $cship.usage_limits"]
EOF

# ccline and best-claude-hud share a config format. Both get every segment this
# comparison is about, in plain (non-nerd-font) mode.
write_segment_config() {
  cat >"$1" <<'EOF'
theme = "default"

[style]
mode = "plain"
separator = " | "

[[segments]]
id = "model"
enabled = true
[segments.icon]
plain = "M"
nerd_font = "M"
[segments.colors.icon]
c16 = 14
[segments.colors.text]
c16 = 14
[segments.styles]
text_bold = false
[segments.options]

[[segments]]
id = "directory"
enabled = true
[segments.icon]
plain = "D"
nerd_font = "D"
[segments.colors.icon]
c16 = 11
[segments.colors.text]
c16 = 10
[segments.styles]
text_bold = false
[segments.options]

[[segments]]
id = "git"
enabled = true
[segments.icon]
plain = "G"
nerd_font = "G"
[segments.colors.icon]
c16 = 12
[segments.colors.text]
c16 = 12
[segments.styles]
text_bold = false
[segments.options]
show_sha = false

[[segments]]
id = "context_window"
enabled = true
[segments.icon]
plain = "C"
nerd_font = "C"
[segments.colors.icon]
c16 = 13
[segments.colors.text]
c16 = 13
[segments.styles]
text_bold = false
[segments.options]

[[segments]]
id = "usage"
enabled = true
[segments.icon]
plain = "U"
nerd_font = "U"
[segments.colors.icon]
c16 = 14
[segments.colors.text]
c16 = 14
[segments.styles]
text_bold = false
[segments.options]
api_base_url = "https://api.anthropic.com"
timeout = 2
cache_duration = 180

[[segments]]
id = "cost"
enabled = true
[segments.icon]
plain = "$"
nerd_font = "$"
[segments.colors.icon]
c16 = 3
[segments.colors.text]
c16 = 3
[segments.styles]
text_bold = false
[segments.options]
EOF
}

mkdir -p "$FAKEHOME/.claude/ccline" "$FAKEHOME/.claude/best-claude-hud"
write_segment_config "$FAKEHOME/.claude/ccline/config.toml"
write_segment_config "$FAKEHOME/.claude/best-claude-hud/config.toml"

# --- running ----------------------------------------------------------------
# Every tool sees the same environment: a scratch HOME, a scratch Claude config
# directory, and no inherited Claude Code variables.
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

# Fills CMD with the argv for a tool id.
CMD=()
command_for() {
  case "$1" in
    howmuchleft) CMD=("$BIN/howmuchleft") ;;
    cship) CMD=("$CSHIP_BIN") ;;
    ccline) CMD=("$CCLINE_BIN") ;;
    best-claude-hud) CMD=("$BCH_BIN") ;;
    claude-code-statusline-pro) CMD=("$CCSP_BIN") ;;
    ccusage) CMD=("$NPM_BIN/ccusage" "statusline") ;;
    ccstatusline) CMD=("$NPM_BIN/ccstatusline") ;;
    claude-powerline) CMD=("$NPM_BIN/claude-powerline") ;;
    floor) CMD=("/bin/true") ;;
    *) die "no command for tool id: $1" ;;
  esac
}

# Times $RUNS runs of one tool and leaves the per-run milliseconds in
# $TIMES/<id>.txt and the peak RSS in kilobytes in $TIMES/<id>.rss.
bench() {
  local id="$1" i start end out
  command_for "$id"
  cd "$WORK"

  out="$("${TOOL_ENV[@]}" "${CMD[@]}" <"$PAYLOAD" 2>/dev/null)" ||
    die "$id exited non-zero on the status object"
  if [ "$id" != "floor" ] && [ -z "$out" ]; then
    die "$id printed nothing on the status object"
  fi

  for ((i = 1; i < WARMUP; i++)); do
    "${TOOL_ENV[@]}" "${CMD[@]}" <"$PAYLOAD" >/dev/null 2>&1 || true
  done

  : >"$TIMES/$id.txt"
  for ((i = 0; i < RUNS; i++)); do
    start="$(date +%s%N)"
    "${TOOL_ENV[@]}" "${CMD[@]}" <"$PAYLOAD" >/dev/null 2>&1 || true
    end="$(date +%s%N)"
    awk -v ns="$((end - start))" 'BEGIN { printf "%.3f\n", ns / 1000000 }' >>"$TIMES/$id.txt"
  done

  : >"$TIMES/$id.rss"
  for ((i = 0; i < RSS_RUNS; i++)); do
    /usr/bin/time -f '%M' -a -o "$TIMES/$id.rss" \
      "${TOOL_ENV[@]}" "${CMD[@]}" <"$PAYLOAD" >/dev/null 2>/dev/null || true
  done

  cd "$REPO_ROOT"
}

for record in "${TOOLS[@]}"; do
  id="$(field "$record" 1)"
  echo "$SELF_NAME: timing $(field "$record" 2) ($RUNS runs)"
  bench "$id"
done
echo "$SELF_NAME: timing the /bin/true spawn floor ($RUNS runs)"
bench floor

# --- results ----------------------------------------------------------------
stat_of() { # stat_of <file> <median|mean|min|max>
  sort -n "$1" | awk -v want="$2" '
    { v[NR] = $1; sum += $1 }
    END {
      median = (NR % 2) ? v[(NR + 1) / 2] : (v[NR / 2] + v[NR / 2 + 1]) / 2
      if (want == "median") printf "%.1f", median
      else if (want == "mean") printf "%.1f", sum / NR
      else if (want == "min") printf "%.1f", v[1]
      else printf "%.1f", v[NR]
    }'
}

peak_mb_of() {
  awk 'BEGIN { max = 0 } $1 > max { max = $1 } END { printf "%.1f", max / 1024 }' "$1"
}

CPU_MODEL="$(awk -F': ' '/^model name/ { print $2; exit }' /proc/cpuinfo)"
[ -n "$CPU_MODEL" ] || CPU_MODEL="unknown"

{
  echo "# Generated by scripts/$SELF_NAME -- do not edit by hand."
  echo "# Rendered into docs/comparison.md by the statusline-comparison directive."
  echo
  echo "[run]"
  echo "date = \"$(date -u +%Y-%m-%d)\""
  echo "machine = \"$(uname -m)\""
  echo "cpu = \"$CPU_MODEL\""
  echo "floor_ms = $(stat_of "$TIMES/floor.txt" median)"
  echo "howmuchleft_commit = \"$HOWMUCHLEFT_COMMIT\""
  for record in "${TOOLS[@]}"; do
    id="$(field "$record" 1)"
    version="$(field "$record" 3)"
    echo
    echo "[[tool]]"
    echo "name = \"$(field "$record" 2)\""
    echo "version = \"$version\""
    echo "language = \"$(field "$record" 4)\""
    echo "source = \"$(field "$record" 5)\""
    echo "shows_five_hour = \"$(field "$record" 6)\""
    echo "shows_weekly = \"$(field "$record" 7)\""
    echo "reads_transcripts = \"$(field "$record" 8)\""
    echo "median_ms = $(stat_of "$TIMES/$id.txt" median)"
    echo "mean_ms = $(stat_of "$TIMES/$id.txt" mean)"
    echo "min_ms = $(stat_of "$TIMES/$id.txt" min)"
    echo "max_ms = $(stat_of "$TIMES/$id.txt" max)"
    echo "peak_mb = $(peak_mb_of "$TIMES/$id.rss")"
    echo "runs = $RUNS"
  done
} >"$RESULTS_FILE"

echo "$SELF_NAME: wrote $RESULTS_FILE"
echo "$SELF_NAME: scratch directory kept at $SCRATCH"
