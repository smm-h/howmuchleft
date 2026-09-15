# HowMuchLeft — Agent Guide

Claude Code statusline tool. Static Go binary, zero runtime dependencies.

## File structure

```
main.go              Entry point: version detection, strictcli app
internal/
  cli/               strictcli command definitions, statusline runner, profile install/uninstall
  config/            TOML config loading, validation, clamping, JSON-to-TOML conversion
  render/            Progress bars, gradients, color system, ANSI output composition
  oauth/             OAuth token refresh, usage API client
  cache/             Atomic file cache with TTL, stale-data fallback
  git/               Branch name from .git/HEAD, counts from an async status cache
  platform/          Claude dir resolution, GitHub user lookup
  demo/              Animated sawtooth-wave demo
  dashboard/         Multi-profile live dashboard
  migrate/           Config default seeding (creates/completes config.toml)
assets/              demo-dark.gif and demo-light.gif (recorded via VHS)
```

## How the statusline protocol works

Claude Code spawns `howmuchleft` as a child process on every render. It pipes a JSON object to stdin containing `model`, `context_window`, `cwd`, and `cost`. The binary writes 3 lines of ANSI-escaped text to stdout and exits. There is no persistent process.

## Architecture

### CLI (internal/cli)

Uses go-strictcli. `NewApp()` builds a `strictcli.App` with subcommands: `version`, `profile {install,uninstall,list}`, `demo`, `colors`, `config`. Two invocations are handled in `main.go` before the app is built and are therefore not commands in it: piped stdin, dispatched by `RunStatuslineDirect()`, and `--refresh-git-cache <repository-root>`, dispatched by `RunGitCacheRefresh()`, which is what a render starts on the binary itself to refresh the git status cache. Each command calls `runMigrations()` (sync.Once-wrapped) for JSON-to-TOML conversion and config default seeding.

### Config (internal/config)

TOML file at `~/.config/howmuchleft/config.toml`. Parsed via go-toml-edit. Per-process cache via `sync.Once`. `Config` struct covers: color_mode, progress_length, colors array, partial_blocks, progress_bar_orientation, cwd settings, show_time_bars, time_bar_dim, lines config, profiles list. `validate()` clamps ranges and fills defaults.

`convert.go` handles one-time JSON-to-TOML migration from the old `~/.config/howmuchleft.json`.

### Render (internal/render)

- `compose.go`: `RenderLines()` takes usage data and produces 3-line ANSI output. Configurable line elements via `[lines]` table.
- `bar.go`: `ProgressBar()` with horizontal (fractional left blocks U+258F-U+2589) and vertical (lower blocks U+2581-U+2587) orientations.
- `gradient.go`: truecolor RGB interpolation and 256-color palette snapping.
- `colors.go`: builtin gradients for 4 combos (dark/light x truecolor/256). Condition matching via `FindColorMatch()`. `IsDarkMode()` detects the desktop theme -- macOS (`defaults read -g AppleInterfaceStyle`), Linux (`gsettings` color-scheme query) -- and keeps the answer in `.dark-mode-cache.json` in the Claude directory for `darkModeCacheTTLMs`, so renders following one another closely read a file instead of starting a process.
- `hash.go`: djb2 hash to hue for profile label coloring.
- `config_bridge.go`: converts `config.Config` to render-internal `BarConfig`.

### OAuth (internal/oauth)

Credentials from `<claude-dir>/.credentials.json`, macOS Keychain fallback. Token refresh via `console.anthropic.com/v1/oauth/token`. Usage data from the platform API.

### Cache (internal/cache)

File-based at `<claude-dir>/.statusline-cache.json`. Atomic writes (tmpfile + rename). 60s TTL for success, 5min for errors. Force-refresh when reset time passes. Stale-data fallback shows last-known-good with `~` prefix.

### Git (internal/git)

Reads `.git/HEAD` directly, walking up from the working directory and following a `.git` file to a worktree or submodule gitdir. Returns the branch name, or `(detached)` when HEAD names a commit.

The ahead/behind counts and the changed-path count need a git process, so they come from a cache file instead: `<claude-dir>/.git-status-cache/<hash-of-repository-root>.json` holds the branch, the counts and when they were measured. A render shows what that file holds and never waits for git. When the entry is missing, older than `statusCacheTTLMs`, unparseable, or names another branch or another root, the render takes a lock file beside the cache and starts `howmuchleft --refresh-git-cache <root>` detached (`cmd.Start()`, its own session, stdio on /dev/null, never waited for). That child runs `git status --porcelain=v2 --branch`, writes the cache atomically and releases the lock, so the counts a render shows lag the working tree by up to the TTL. The lock keeps one refresh in flight at a time; a lock left by a killed refresher is reclaimed once it is `refreshLockStaleness` old.

### Platform (internal/platform)

- `GetClaudeDir()`: resolves Claude Code config directory
- `ghuser.go`: GitHub username from `gh` CLI auth status

### Demo (internal/demo)

Sawtooth waves: weekly 1 cycle, 5-hour 8 cycles, context 15 cycles. Default 60s duration. Final frame pins all bars to 100%.

### Dashboard (internal/dashboard)

`profile list` renders all discovered profiles side-by-side. `--live` refreshes every 30s.

### Migrate (internal/migrate)

`EnsureDefaults(configDir)` creates `config.toml` when missing and fills in any key a newer version introduced, leaving existing values, comments and unknown keys untouched. Defaults come from `config.Default()` and `config.DefaultLines()`, so those are the single source of truth.

## Dependencies

- `github.com/smm-h/strictcli/go` -- CLI framework
- `github.com/smm-h/go-toml-edit` -- TOML parsing/editing (preserves comments and formatting)

## Local development

```bash
go build -o howmuchleft .
./howmuchleft version
echo '{"model":"claude-sonnet-4-20250514","context_window":200000}' | ./howmuchleft
```

The version is injected via `-ldflags "-X main.Version=..."` at build time. Without ldflags, it falls back to `debug.ReadBuildInfo()` or `"dev"`. The symbol name must match `.goreleaser.yml` exactly -- the linker silently ignores an `-X` naming a symbol that does not exist.

## Patterns and conventions

- All state is computed fresh per invocation (no daemon, no IPC)
- Atomic file writes via tmpfile + rename for cache
- Keep processes off the render path: the branch comes from reading `.git/HEAD`, the git counts come from a cache a detached child refreshes, and the desktop theme and the GitHub user are detected once and cached on disk
- Tests use `go test ./... -race`
- Model aliases: O4.6 (opus), S4.6 (sonnet), H4.5 (haiku)

## CI/CD

- `.github/workflows/ci.yml`: `go test ./... -race` on Go latest, triggered on push to main and PRs
- `.github/workflows/publish.yml`: goreleaser triggered on GitHub Release (no secrets needed for Go binaries)
- **Never publish manually** -- always use `rlsbl release`, which bumps the version, validates CHANGELOG.md, creates a GitHub Release, and triggers goreleaser
- **Always update `CHANGELOG.md`** when bumping a version
- Use `rlsbl release --dry-run` to preview a release without making changes
