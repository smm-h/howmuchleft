# Changelog

## 0.8.2

- Migrate release pipeline to share-it-on (replace scripts/release.sh)
- Add pre-release hook, pre-push changelog enforcement, scaffolding freshness marker
- Add share-it-on scripts (pre-release.sh, pre-push-hook.sh)
- Merge security-sensitive patterns into .gitignore
- Update CLAUDE.md for share-it-on workflow

## 0.8.1

- Time-elapsed bars in profile dashboard (`howmuchleft profile list`)
- More dramatic 5hr urgency in demo animation

## 0.8.0

- Profile dashboard: `howmuchleft profile list` shows all Claude profiles with usage bars side-by-side; `--live` mode refreshes every 30s; discovers profiles from config or `~/.claude-*` dirs
- Time-elapsed bars: two new vertical bar columns alongside 5hr and weekly usage bars showing how much of each time window has passed; urgency coloring (gray -> yellow -> red) based on burnrate ratio; configurable via `showTimeBars` (default true) and `timeBarDim` (default 0.25)
- Demo animation includes time bars
- Fix: renamed invalid `ProjectInit` hook to `SessionStart`

## 0.7.0

- Show extra usage (pay-as-you-go) bar when weekly quota is exhausted
- 3rd bar contextually swaps from weekly to extra usage when weekly >= 100% and extra usage is enabled
- Warm amber background on extra usage bar and highlighted percentage for visual distinction
- Weekly reset countdown preserved on the extra usage line
- Demo animation shows the weekly-to-extra-usage transition

## 0.6.1

- Configurable cwd display: `cwdMaxLength` (default 50) and `cwdDepth` (default 3)
- `scripts/release.sh` now uses CHANGELOG.md content as GitHub Release notes
- Updated project guide (CLAUDE.md)

## 0.6.0

- Show active profile name in statusline (line 1, before subscription tier)
- Profile label color derived from config directory path hash (unique per profile, adapts to dark/light mode)
- Fix `progressBarOrientation: "horizontal"` being ignored in config (#1)
- Fix `--test-colors` using vertical block characters in horizontal bar preview
- Fix stale comment in config.example.json (default orientation is vertical)
- `--test-colors` overhaul: wider horizontal bars (13 cells), vertical column preview spanning all 7 rows (2 chars wide), aligned percentage labels
- `verticalBarCell()` now accepts optional `totalRows` parameter (defaults to 3)

## 0.5.0

- Show session elapsed time (e.g. `1h34m`) on line 1, between context percentage and subscription tier
- Reads `startedAt` from Claude Code's PID session file by walking the process tree via `/proc`

## 0.4.0

- Default progress bar orientation changed to vertical
- Fix usage API endpoint: migrated from api.anthropic.com to platform.claude.com (old endpoint returns 429 unconditionally)
- Fix error cache preserving stale resetAt timestamps, which bypassed error TTL and caused per-render API hammering
- Exponential backoff on error cache TTL (5m/10m/20m/40m/60m cap) to reduce API load during prolonged failures
- Fix stale file credentials shadowing fresh Keychain tokens on macOS (expired token with no refresh token no longer blocks Keychain lookup)
- Keychain fallback in getValidToken() after refresh failure
- Credential mtime detection: force-refresh when Claude Code writes fresh credentials mid-session

## 0.3.0

- `progressBarOrientation` config option (`"horizontal"`/`"vertical"`): vertical mode renders 3 bar columns filling bottom-to-top across all 3 lines (8 states per cell = 24 levels)
- Extract shared `renderLines()` function for line composition, used by both main output and demo mode

## 0.2.2

- `partialBlocks` config option (`true`/`false`/`"auto"`): disable fractional block characters on terminals with broken rendering
- Auto-detection disables partial blocks on Apple Terminal and the Linux console

## 0.2.1

- Read OAuth credentials from macOS Keychain when `.credentials.json` lacks them
- Support both current hashed (`Claude Code-credentials-<hash>`) and legacy service names
- Merge Keychain OAuth data with file data (preserves mcpOAuth etc.)
- Per-process credentials cache to avoid repeated Keychain reads

## 0.2.0

- `bg` field in `colors` entries for per-condition empty bar background color (replaces `emptyBgDark`/`emptyBgLight`)
- Truecolor support with auto-detection via `COLORTERM` env var
- Smooth RGB interpolation between gradient stops
- New `colors` array config: conditional entries with optional `dark-mode` and `true-color` matching
- Separate built-in gradients for dark/light mode and truecolor/256-color
- `colorMode` config option: `"auto"`, `"truecolor"`, or `"256"`
- JSONC config file support (comments and trailing commas)
- `--demo` flag: continuous time-lapse animation with configurable duration
- TTY detection: helpful message instead of hanging when run directly
- Configurable 256-color gradient (replaces hardcoded 4-bucket colors)
- `--test-colors` flag: preview gradient swatch at seven sample levels
- Graceful handling of empty/malformed JSON on stdin

## 0.1.0 (2026-03-15)

- Initial release
- Three-line statusline: context window, 5-hour usage, 7-day usage
- Sub-cell precision progress bars using Unicode fractional block characters
- Green-to-red color shift as usage increases
- OAuth usage API integration with 60s caching and stale-data fallback
- Automatic OAuth token refresh
- Git branch and change count display
- Lines added/removed tracking
- Dark/light mode detection (macOS and Linux/GNOME)
- `--install` / `--uninstall` for Claude Code settings.json
- `--config` to view and manage settings
- Configurable bar width and empty bar background color
