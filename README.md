# HowMuchLeft

[![npm version](https://img.shields.io/npm/v/howmuchleft)](https://www.npmjs.com/package/howmuchleft)
[![npm downloads](https://img.shields.io/npm/dm/howmuchleft)](https://www.npmjs.com/package/howmuchleft)
[![license](https://img.shields.io/npm/l/howmuchleft)](https://github.com/smm-h/howmuchleft/blob/main/LICENSE)
[![node](https://img.shields.io/node/v/howmuchleft)](https://nodejs.org)

Pixel-perfect progress bars for your Claude Code statusline. See how much context and usage you have left at a glance.

![Demo](./assets/demo.gif)

## What you get

Three bars with sub-cell precision using Unicode fractional block characters:

| Bar | Shows | Extra info |
|---|---|---|
| Context window | How full your conversation is | Subscription tier, model name |
| 5-hour usage | Rolling rate limit utilization | Time until reset, git branch/changes, lines added/removed |
| Weekly usage | Rolling 7-day rate limit utilization | Time until reset, current directory |

Bars shift green → yellow → orange → red as usage increases. Stale data (API unreachable) is prefixed with `~`. Works with Pro, Max 5x, Max 20x, and Team subscriptions. API key users see an "API" label with context bar only.

## Install

```bash
npm install -g howmuchleft
howmuchleft --install
```

For multiple Claude Code config directories:

```bash
howmuchleft --install ~/.claude-work
howmuchleft --install ~/.claude-personal
```

## Configuration

Config file: `~/.config/howmuchleft.json` (JSONC — comments and trailing commas are allowed).

```jsonc
{
  "progressLength": 12,
  "colorMode": "auto",
  "colors": [
    // Truecolor dark: RGB gradient + RGB background
    { "dark-mode": true, "true-color": true, "bg": [48, 48, 48], "gradient": [[0,215,0], [255,255,0], [255,0,0]] },
    // 256-color dark: index gradient + index background
    { "dark-mode": true, "true-color": false, "bg": 236, "gradient": [46, 226, 196] }
  ]
}
```

### Top-level settings

| Field | Default | Description |
|---|---|---|
| `progressLength` | `12` | Bar width in characters (3–40) |
| `colorMode` | `"auto"` | `"auto"` (detect via `COLORTERM`), `"truecolor"`, or `"256"` |
| `colors` | built-in | Array of color entries (see below) |

### Color entries

Each entry in the `colors` array is matched against the current terminal. First match wins. Omit a condition to match both modes.

| Field | Required | Description |
|---|---|---|
| `gradient` | Yes | Color stops: `[R,G,B]` arrays for truecolor, or integers (0–255) for 256-color |
| `bg` | No | Empty bar background: `[R,G,B]` for truecolor, or integer (0–255) for 256-color |
| `dark-mode` | No | Match dark (`true`) or light (`false`) terminals only |
| `true-color` | No | Match truecolor (`true`) or 256-color (`false`) terminals only |

Truecolor gradients are smoothly interpolated between stops — 3 stops (green, yellow, red) is enough for a smooth bar. 256-color gradients snap to the nearest stop.

To preview your current gradient: `howmuchleft --test-colors`

## CLI

```
howmuchleft [config-dir]              Run the statusline (called by Claude Code)
howmuchleft --install [config-dir]    Add to Claude Code settings.json
howmuchleft --uninstall [config-dir]  Remove from Claude Code settings.json
howmuchleft --config                  Show config path and current settings
howmuchleft --demo [seconds]          Time-lapse animation (default 60s)
howmuchleft --test-colors             Preview gradient at seven sample levels
howmuchleft --version                 Show version
howmuchleft --help                    Show help
```

## How it works

Claude Code invokes `howmuchleft` as a child process on each statusline render, piping a JSON object to stdin with model info, context window usage, cwd, and cost data. The script:

1. Parses the JSON from stdin
2. Fetches usage data from Anthropic's OAuth API (cached 60s, stale-data fallback on failure)
3. Auto-refreshes expired OAuth tokens
4. Runs `git status --porcelain=v2` in parallel for branch/change info
5. Renders three lines of ANSI-colored progress bars to stdout

## Uninstall

```bash
howmuchleft --uninstall
npm uninstall -g howmuchleft
```

## Requirements

- Node.js >= 18
- Claude Code with OAuth login (Pro, Max, or Team subscription)
- `git` (optional, for branch/change display)
- `gsettings` (Linux/GNOME) or `defaults` (macOS) for dark/light mode detection
