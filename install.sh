#!/usr/bin/env bash
# HowMuchLeft installer
# Usage: bash -c "$(curl -fsSL https://raw.githubusercontent.com/USER/howmuchleft/main/install.sh)"
set -euo pipefail

INSTALL_DIR="${HOME}/.howmuchleft"
REPO_URL="https://github.com/USER/howmuchleft.git"

info() { printf '\033[36m%s\033[0m\n' "$1"; }
warn() { printf '\033[33m%s\033[0m\n' "$1"; }
err()  { printf '\033[31m%s\033[0m\n' "$1" >&2; }

# Check prerequisites
command -v node >/dev/null 2>&1 || { err "Node.js is required but not found. Install it first."; exit 1; }
command -v git >/dev/null 2>&1 || { err "Git is required but not found. Install it first."; exit 1; }

# Clone or update
if [ -d "$INSTALL_DIR/.git" ]; then
  info "Updating existing installation in ${INSTALL_DIR}..."
  git -C "$INSTALL_DIR" pull --ff-only
else
  if [ -d "$INSTALL_DIR" ]; then
    err "${INSTALL_DIR} exists but is not a git repo. Remove it first."
    exit 1
  fi
  info "Cloning howmuchleft to ${INSTALL_DIR}..."
  git clone --depth=1 "$REPO_URL" "$INSTALL_DIR"
fi

chmod +x "${INSTALL_DIR}/bin/cli.js"

info ""
info "Installed to ${INSTALL_DIR}"
info ""

# Offer to configure Claude Code
DEFAULT_CLAUDE_DIR="${HOME}/.claude"
CLAUDE_DIR="${1:-$DEFAULT_CLAUDE_DIR}"

read -rp "Configure Claude Code settings in ${CLAUDE_DIR}? [Y/n] " answer
answer="${answer:-y}"

if [[ "${answer,,}" == "y" ]]; then
  node "${INSTALL_DIR}/bin/cli.js" --install "$CLAUDE_DIR"
else
  info ""
  info "To install later, run:"
  info "  node ${INSTALL_DIR}/bin/cli.js --install"
  info ""
  info "Or add this to your Claude Code settings.json manually:"
  info "  \"statusLine\": {"
  info "    \"type\": \"command\","
  info "    \"command\": \"node ${INSTALL_DIR}/bin/cli.js ${CLAUDE_DIR}\","
  info "    \"padding\": 0"
  info "  }"
fi

info ""
info "Done. Restart Claude Code to see the statusline."
