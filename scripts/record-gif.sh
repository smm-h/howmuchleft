#!/usr/bin/env bash
# Record demo GIFs for both dark and light modes using VHS.
#
# Usage: ./scripts/record-gif.sh [duration_seconds]
# Output: assets/demo-dark.gif, assets/demo-light.gif
#
# Prerequisites:
#   - vhs (go install github.com/charmbracelet/vhs@latest)
#   - ffmpeg
#   - ttyd (https://github.com/tsl0922/ttyd/releases)

set -euo pipefail

DURATION="${1:-15}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
ASSETS_DIR="$PROJECT_DIR/assets"

mkdir -p "$ASSETS_DIR"

record_gif() {
  local mode="$1"
  local dark_val theme

  if [ "$mode" = "dark" ]; then
    dark_val=1
    theme="zenwritten_dark"
  else
    dark_val=0
    theme="zenwritten_light"
  fi

  local gif_file="$ASSETS_DIR/demo-${mode}.gif"
  local tape_file="$ASSETS_DIR/.demo-${mode}.tape"

  # Generate tape file for VHS
  cat > "$tape_file" <<TAPE
Output "${gif_file}"
Set FontSize 36
Set FontFamily "Adwaita Mono"
Set Height 250
Set Width 1400
Set Padding 20
Set Framerate 30
Set Theme "${theme}"
Env COLORTERM "truecolor"
Env HOWMUCHLEFT_DARK "${dark_val}"
Hide
Type "node ${PROJECT_DIR}/bin/cli.js --demo ${DURATION}"
Enter
Sleep 500ms
Show
Sleep ${DURATION}s
TAPE

  echo "Recording $mode mode demo (${DURATION}s)..."
  vhs "$tape_file"

  # Clean up tape file
  rm -f "$tape_file"

  local size
  size=$(du -h "$gif_file" | cut -f1)
  echo "  -> $gif_file ($size)"
}

record_gif dark
record_gif light

echo "Done."
