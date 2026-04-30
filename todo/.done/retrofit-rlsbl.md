# Retrofit howmuchleft to use rlsbl

## Context

howmuchleft currently uses a custom `scripts/release.sh` for release orchestration and a `publish.yml` workflow that uses NPM_TOKEN (which relies on a granular access token). The new `rlsbl` CLI tool (published on npm) replaces release.sh, and npm's OIDC Trusted Publishing replaces token-based auth.

## Problem

The release infrastructure is bespoke and needs manual token rotation (90-day max expiry on granular tokens). Migrating to rlsbl + OIDC standardizes the pipeline and eliminates token management.

## Steps

1. Install rlsbl globally: `npm i -g rlsbl`
2. Update `.github/workflows/publish.yml` to remove `NODE_AUTH_TOKEN` / `NPM_TOKEN` references and rely on OIDC (just `permissions: id-token: write` and plain `npm publish --provenance --access public`)
3. Configure Trusted Publishing on npmjs.com for howmuchleft (package settings > Trusted Publishers > add smm-h/howmuchleft + publish.yml)
4. Delete `scripts/release.sh` (rlsbl replaces it)
5. Update CLAUDE.md to reference `rlsbl release` instead of `scripts/release.sh`
6. Test with `rlsbl release --dry-run` in the howmuchleft directory
7. Verify CI publishes correctly on next release

## Files that change

- `.github/workflows/publish.yml`
- `scripts/release.sh` (deleted)
- `CLAUDE.md`

## Effort

Low -- mostly config changes and deletions.
