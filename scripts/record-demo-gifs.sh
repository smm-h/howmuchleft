#!/usr/bin/env bash
#
# Record the demo GIFs in assets/ from a freshly built howmuchleft.
#
#   scripts/record-demo-gifs.sh --dry-run             # print the plan, touch nothing
#   scripts/record-demo-gifs.sh --record              # build, record both GIFs
#   scripts/record-demo-gifs.sh --record --duration 20
#
# Both GIFs show the same animation -- howmuchleft demo, whose sawtooth waves
# drive every bar and whose git, profile and directory labels are synthetic --
# recorded once against a dark terminal theme and once against a light one, with
# HOWMUCHLEFT_DARK telling the binary which gradient to pick.
#
# VHS runs the binary inside a headless xterm.js, which draws the fractional
# block characters the bars are made of algorithmically instead of from font
# glyphs, so the bars come out at the same subdivisions the terminal shows.
#
# The build, the HOME the binary runs with and the tape files all live in a
# scratch directory made with mktemp -d. The only files written outside it are
# the two GIFs.

set -euo pipefail

SELF_NAME="$(basename "${BASH_SOURCE[0]}")"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ASSETS_DIR="$REPO_ROOT/assets"

DURATION=15
MODE=""

# The recording geometry. Changing one of these changes both GIFs.
FONT_SIZE=36
FONT_FAMILY="Adwaita Mono"
WIDTH=1400
HEIGHT=250
PADDING=20
FRAMERATE=30

usage() {
  cat <<EOF
usage: $SELF_NAME (--dry-run | --record) [--duration N]

  --dry-run     print what would be built, recorded and written, change nothing
  --record      build howmuchleft and record assets/demo-dark.gif and
                assets/demo-light.gif
  --duration N  seconds of animation per GIF (default $DURATION)
EOF
}

die() {
  echo "$SELF_NAME: $*" >&2
  exit 1
}

while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) MODE="dry" ;;
    --record) MODE="record" ;;
    --duration)
      shift
      [ $# -gt 0 ] || die "--duration needs a number"
      DURATION="$1"
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
  die "choose --dry-run or --record"
}
case "$DURATION" in
  '' | *[!0-9]*) die "--duration needs a positive integer, got: $DURATION" ;;
esac
[ "$DURATION" -gt 0 ] || die "--duration needs a positive integer, got: $DURATION"

# vhs drives ttyd and encodes with ffmpeg, and says nothing useful when either
# is missing, so refuse here and name the one that is not installed.
for prog in go vhs ttyd ffmpeg; do
  command -v "$prog" >/dev/null 2>&1 || die "required program not found: $prog"
done

if [ "$MODE" = "dry" ]; then
  echo "$SELF_NAME --record would:"
  echo
  echo "  1. create a scratch directory with mktemp -d and build howmuchleft"
  echo "     from $REPO_ROOT into it"
  echo "  2. write a tape per theme, each one running: howmuchleft demo $DURATION"
  echo "     with HOME, XDG_CONFIG_HOME and CLAUDE_CONFIG_DIR inside the scratch"
  echo "     directory, COLORTERM=truecolor, and HOWMUCHLEFT_DARK set to 1 for the"
  echo "     dark recording and 0 for the light one"
  echo "  3. record ${WIDTH}x${HEIGHT} at ${FRAMERATE} fps, ${FONT_SIZE}pt $FONT_FAMILY,"
  echo "     theme zenwritten_dark and zenwritten_light, and overwrite:"
  echo "       $ASSETS_DIR/demo-dark.gif"
  echo "       $ASSETS_DIR/demo-light.gif"
  echo
  echo "Nothing outside the scratch directory and those two GIFs is written."
  exit 0
fi

SCRATCH="$(mktemp -d -t howmuchleft-record.XXXXXXXX)"
BIN="$SCRATCH/howmuchleft"
FAKEHOME="$SCRATCH/home"
mkdir -p "$FAKEHOME/.config" "$FAKEHOME/.claude"

echo "$SELF_NAME: scratch directory $SCRATCH"
echo "$SELF_NAME: building howmuchleft from $REPO_ROOT"
(cd "$REPO_ROOT" && go build -o "$BIN" .)

record() {
  local mode="$1" dark theme gif tape
  case "$mode" in
    dark)
      dark=1
      theme="zenwritten_dark"
      ;;
    light)
      dark=0
      theme="zenwritten_light"
      ;;
    *) die "no such recording mode: $mode" ;;
  esac
  gif="$ASSETS_DIR/demo-$mode.gif"
  tape="$SCRATCH/demo-$mode.tape"

  cat >"$tape" <<TAPE
Output "$gif"
Set FontSize $FONT_SIZE
Set FontFamily "$FONT_FAMILY"
Set Width $WIDTH
Set Height $HEIGHT
Set Padding $PADDING
Set Framerate $FRAMERATE
Set Theme "$theme"
Env COLORTERM "truecolor"
Env HOWMUCHLEFT_DARK "$dark"
Env HOME "$FAKEHOME"
Env XDG_CONFIG_HOME "$FAKEHOME/.config"
Env XDG_CACHE_HOME "$FAKEHOME/.cache"
Env CLAUDE_CONFIG_DIR "$FAKEHOME/.claude"
Hide
Type "$BIN demo $DURATION"
Enter
Sleep 500ms
Show
Sleep ${DURATION}s
TAPE

  echo "$SELF_NAME: recording $mode (${DURATION}s)"
  vhs "$tape"
  echo "$SELF_NAME: wrote $gif ($(du -h "$gif" | cut -f1))"
}

mkdir -p "$ASSETS_DIR"
record dark
record light

echo "$SELF_NAME: scratch directory kept at $SCRATCH"
