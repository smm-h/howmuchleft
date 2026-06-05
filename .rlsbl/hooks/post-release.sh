#!/usr/bin/env bash
# Post-release hook. Runs after a successful release (non-fatal).
# Environment: RLSBL_VERSION is set to the released version.

set -euo pipefail

echo "Post-release: v$RLSBL_VERSION"
