#!/usr/bin/env bash
# Create a GitHub release that triggers the npm publish workflow.
#
# Usage: ./scripts/release.sh
#
# Reads the version from package.json, validates preconditions,
# pushes to remote, and creates a GitHub release.
# The publish workflow (.github/workflows/publish.yml) handles npm publish.

set -euo pipefail

version=$(node -e "console.log(require('./package.json').version)")
tag="v${version}"

# Precondition checks
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "error: working tree is not clean" >&2
  exit 1
fi

if [ -n "$(git ls-files --others --exclude-standard)" ]; then
  echo "error: untracked files present" >&2
  exit 1
fi

if ! grep -q "## ${version}" CHANGELOG.md; then
  echo "error: no changelog entry for ${version}" >&2
  exit 1
fi

if git rev-parse "${tag}" >/dev/null 2>&1; then
  echo "error: tag ${tag} already exists" >&2
  exit 1
fi

local_sha=$(git rev-parse HEAD)
remote_sha=$(git ls-remote origin HEAD | cut -f1)
if [ "${local_sha}" != "${remote_sha}" ]; then
  echo "Local HEAD differs from remote. Pushing..."
  git push
fi

echo "Creating release ${tag}..."
gh release create "${tag}" --title "${tag}" --generate-notes
echo "Done. Publish workflow will push ${version} to npm."
