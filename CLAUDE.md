# HowMuchLeft — Agent Guide

Claude Code statusline tool. Zero dependencies, CommonJS, Node >= 18.

## File structure

```
bin/cli.js           CLI entry point, flag routing, --install/--uninstall
lib/statusline.js    Core: stdin parsing, OAuth, usage API, git info, bar rendering
lib/demo.js          --demo animation (sawtooth waves, standalone)
config.example.json  JSONC example with all 4 gradient combos
assets/              demo-dark.gif and demo-light.gif (recorded via VHS)
```

## How the statusline protocol works

Claude Code spawns `howmuchleft` as a child process on every render. It pipes a JSON object to stdin containing `model`, `context_window`, `cwd`, and `cost`. The script writes 3 lines of ANSI-escaped text to stdout and exits. There is no persistent process.

## Architecture

### Rendering pipeline (lib/statusline.js)

`main()` is the entry point:
1. `readStdin()` — parse JSON from stdin, fallback to `{}` on malformed input (logs warning to stderr)
2. `getUsageData()` and `getGitInfo()` run in parallel via `Promise.all`
3. `renderLines()` composes the 3-line output (shared by main and demo mode)

Line 1 shows the active profile name (config directory basename) in a color hashed from the full config directory path, placed between elapsed time and subscription tier. Hidden when the directory is the default `.claude`.

### Color system

Two color depths: truecolor (RGB) and 256-color (palette indices). Detection is via `COLORTERM` env var, overridable with `colorMode` config.

`colors` array entries have optional `dark-mode` and `true-color` conditions. `findColorMatch()` returns the first entry whose conditions match. Builtins cover all 4 combos (dark/light x truecolor/256).

- Truecolor gradients: `interpolateRgb()` does linear interpolation between `[R,G,B]` stops
- 256-color gradients: snaps to nearest stop index
- `bg` field: empty bar background, same format as gradient stops. Formatted to ANSI by `formatBgEscape()`
- Profile label color: `hashToHue()` (djb2 hash to hue 0-359) and `hueToAnsi()` (HSL to ANSI escape, adapts lightness for dark/light mode, truecolor/256-color fallback)

### Progress bars (progressBar function)

Two orientations controlled by `progressBarOrientation` config:

- **Horizontal** (default): each line has its own bar using left fractional blocks (U+258F–U+2589) for sub-cell precision. Bar width set by `progressLength`.
- **Vertical**: 3 bar columns (context, 5hr, weekly) span all 3 output lines, filling bottom-to-top using lower fractional blocks (U+2581–U+2587). Each cell has 8 states (empty + 7 levels), 3 rows = 24 discrete levels per bar.

Fractional blocks can be disabled via `partialBlocks` config (`true`/`false`/`"auto"`). Auto-detection disables them on terminals in `PARTIAL_BLOCKS_BLOCKLIST` (Apple Terminal, Linux console).

`verticalBarCell()` accepts an optional `totalRows` parameter (defaults to 3 for the statusline, 7 in `--test-colors`).

### OAuth and usage API

- Credentials: file at `<claude-dir>/.credentials.json` first, then macOS Keychain fallback
  - Keychain service name: `Claude Code-credentials-<sha256(configDir)[:8]>` (current) or `Claude Code-credentials` (legacy)
  - Keychain data merged with file data (preserves mcpOAuth etc.)
  - Per-process cache avoids repeated Keychain reads
- Token refresh via `console.anthropic.com/v1/oauth/token`
- Usage data from `api.anthropic.com/api/oauth/usage` (beta header required)
- Cache at `<claude-dir>/.statusline-cache.json` with absolute timestamps
- 60s TTL for success, 5min for errors, force-refresh when reset time passes
- Stale-data fallback: last-known-good data shown with `~` prefix

### Dark/light mode detection

`isDarkMode()`: macOS uses `defaults read -g AppleInterfaceStyle`, Linux uses `gsettings get org.gnome.desktop.interface color-scheme`. Cached per-process.

### Config

JSONC file at `~/.config/howmuchleft.json`. Parsed by `stripJsonComments()` + `JSON.parse()`. Config is cached per-process in `_barConfig`.

Notable config options:
- `cwdMaxLength` (default 50, range 10-100): max characters for cwd display
- `cwdDepth` (default 3, range 1-10): trailing path segments to keep when truncated

### Demo mode (lib/demo.js)

Single continuous animation, default 60s. Three sawtooth waves:
- Weekly: 1 cycle (0→100%)
- 5-hour: 8 cycles
- Context: 15 cycles

Final frame pins all bars to 100% (`isLast` flag) because sawtooth math `(t * N) % 1` never reaches exactly 1.0.

## Local development

The package is installed globally via `npm link`, so `/usr/local/bin/howmuchleft` is a symlink to `bin/cli.js` in this repo. Code changes are picked up immediately by Claude Code's statusline — no reinstall needed.

## Patterns and conventions

- All state is computed fresh per invocation (no daemon, no IPC)
- Atomic file writes via tmpfile + `rename(2)` for cache and credentials
- `--no-optional-locks` on git commands to avoid blocking concurrent git operations
- No test framework — CI runs smoke tests: `--version`, `--help`, `--test-colors`, stdin pipe
- npm package includes only `bin/`, `lib/`, `config.example.json`

## CI/CD

- `.github/workflows/ci.yml`: smoke tests on Node 18/20/22, triggered on push to main and PRs
- `.github/workflows/publish.yml`: `npm publish --provenance` triggered on GitHub Release, requires `NPM_TOKEN` repo secret
- **Never run `npm publish` manually** — always publish by creating a GitHub Release via `scripts/release.sh`, which extracts the version's section from CHANGELOG.md as the release body and triggers the publish workflow
- **Always update `CHANGELOG.md`** when bumping a version — add an entry for each new version before publishing
