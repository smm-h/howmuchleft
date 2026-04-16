# Profile registry and multi-profile dashboard

## Context

howmuchleft currently operates on a single config dir per invocation (passed as a CLI arg or read from `CLAUDE_CONFIG_DIR`). There's no central record of which profiles exist, and no way to see all profiles' usage at a glance.

## Profile registry

### Where to store it

Add a `"profiles"` array to the existing config file at `~/.config/howmuchleft.json`. Each entry is an absolute path to a Claude config dir.

```json
{
  "progressLength": 12,
  "profiles": [
    "/home/user/.claude",
    "/home/user/.claude-work",
    "/home/user/.claude-personal"
  ]
}
```

### When to populate it

`--install <dir>` should append the dir to `profiles` (deduplicated) when registering the statusline command. `--uninstall <dir>` should remove it.

### Fallback discovery

If `profiles` is empty or absent, scan for dirs containing `.credentials.json`:
- `~/.claude/.credentials.json`
- `~/.claude-*/.credentials.json`

Always include `~/.claude` in the scan (it doesn't match the `~/.claude-*` glob).

## Multi-profile dashboard

A new flag (`--all` or `--dashboard`) that renders all registered profiles' usage in a single live terminal view.

### What it shows

For each profile:
- Profile name (derived from dir basename, color-hashed as today)
- Subscription tier
- 5-hour usage bar + percentage + reset time
- Weekly usage bar + percentage + reset time
- Extra usage bar (if weekly >= 100%)

### How it works

1. Read `profiles` from config (or fall back to discovery)
2. For each profile dir: read credentials, fetch usage (parallel), read cache
3. Render stacked output -- one section per profile
4. Live loop: clear and re-render on an interval (e.g. every 30s)
5. Exit on ctrl+c

### Building blocks already available

- `getProfileName()` -- extracts name from dir basename
- `hashToHue()` -- stable color per profile
- `getUsageData()` -- OAuth + usage API + caching
- `renderLines()` -- progress bar rendering
- All credential handling (token refresh, keychain, cache TTL)

## Effort

Medium. The registry (config file + install/uninstall hooks) is small. The dashboard needs a terminal loop, parallel fetching across profiles, and a combined layout -- but all rendering and data-fetching primitives exist.
