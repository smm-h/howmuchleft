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
3. Three lines are composed and written to stdout

### Color system

Two color depths: truecolor (RGB) and 256-color (palette indices). Detection is via `COLORTERM` env var, overridable with `colorMode` config.

`colors` array entries have optional `dark-mode` and `true-color` conditions. `findColorMatch()` returns the first entry whose conditions match. Builtins cover all 4 combos (dark/light x truecolor/256).

- Truecolor gradients: `interpolateRgb()` does linear interpolation between `[R,G,B]` stops
- 256-color gradients: snaps to nearest stop index
- `bg` field: empty bar background, same format as gradient stops. Formatted to ANSI by `formatBgEscape()`

### Progress bars (progressBar function)

Uses Unicode left fractional block characters (U+258F through U+2589) for sub-cell precision. Each cell is either fully filled (bg-colored space), fractional (fg-colored block char on empty bg), or empty (empty bg space).

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

### Demo mode (lib/demo.js)

Single continuous animation, default 60s. Three sawtooth waves:
- Weekly: 1 cycle (0→100%)
- 5-hour: 8 cycles
- Context: 15 cycles

Final frame pins all bars to 100% (`isLast` flag) because sawtooth math `(t * N) % 1` never reaches exactly 1.0.

## Patterns and conventions

- All state is computed fresh per invocation (no daemon, no IPC)
- Atomic file writes via tmpfile + `rename(2)` for cache and credentials
- `--no-optional-locks` on git commands to avoid blocking concurrent git operations
- No test framework — CI runs smoke tests: `--version`, `--help`, `--test-colors`, stdin pipe
- npm package includes only `bin/`, `lib/`, `config.example.json`

## CI/CD

- `.github/workflows/ci.yml`: smoke tests on Node 18/20/22, triggered on push to main and PRs
- `.github/workflows/publish.yml`: `npm publish --provenance` triggered on GitHub Release, requires `NPM_TOKEN` repo secret
- **Never run `npm publish` manually** — always publish by creating a GitHub Release (`gh release create vX.Y.Z --title "vX.Y.Z" --generate-notes`), which triggers the publish workflow
