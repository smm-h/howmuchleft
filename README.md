# HowMuchLeft

[![npm version](https://img.shields.io/npm/v/howmuchleft)](https://www.npmjs.com/package/howmuchleft)
[![npm downloads](https://img.shields.io/npm/dm/howmuchleft)](https://www.npmjs.com/package/howmuchleft)
[![license](https://img.shields.io/npm/l/howmuchleft)](https://github.com/smm-h/howmuchleft/blob/main/LICENSE)
[![node](https://img.shields.io/node/v/howmuchleft)](https://nodejs.org)

Pixel-perfect progress bars showing how much context and usage you have left, right in your Claude Code statusline.

Three bars with sub-cell precision (using Unicode fractional block characters):

- **Context window** -- how full your conversation context is, plus subscription tier and model name
- **5-hour usage** -- your rolling 5-hour rate limit utilization, time until reset, git branch/changes, lines added/removed
- **Weekly usage** -- your rolling 7-day rate limit utilization, time until reset, current directory

Bars shift from green to yellow to orange to red as usage increases. Stale data is prefixed with `~`. Works with Pro, Max 5x, Max 20x, and Team subscriptions. API key users see an "API" label with context bar only.

## Install

```bash
npm install -g howmuchleft
howmuchleft --install
```

For multiple Claude Code subscriptions:

```bash
howmuchleft --install ~/.claude-work
howmuchleft --install ~/.claude-personal
```

## Configuration

Config file: `~/.config/howmuchleft.json`

The config file supports JSONC (comments and trailing commas).

```jsonc
{
  "progressLength": 12,
  "colorMode": "auto",
  // First matching entry wins. Omit dark-mode/true-color to match both.
  "colors": [
    { "dark-mode": true, "true-color": true, "bg": [48, 48, 48], "gradient": [[0,215,0], [255,255,0], [255,0,0]] },
    { "dark-mode": true, "true-color": false, "bg": 236, "gradient": [46, 226, 196] }
  ]
}
```

| Field | Default | Description |
|---|---|---|
| `progressLength` | `12` | Bar width in characters (3-40) |
| `colorMode` | `"auto"` | Color depth: `"auto"` (detect via `COLORTERM`), `"truecolor"`, or `"256"` |
| `colors` | — | Array of color entries (see below) |

Each entry in the `colors` array has these fields:

| Field | Required | Description |
|---|---|---|
| `gradient` | Yes | Array of color stops: `[R,G,B]` arrays for truecolor, or integers (0-255) for 256-color |
| `bg` | No | Empty bar background: `[R,G,B]` for truecolor or integer (0-255) for 256-color |
| `dark-mode` | No | If set, only matches dark (`true`) or light (`false`) terminals |
| `true-color` | No | If set, only matches truecolor (`true`) or 256-color (`false`) terminals |

First matching entry wins. Omitted conditions match both modes. Built-in defaults are used when no entry matches.
Truecolor gradients are smoothly interpolated between stops — 3 stops (green, yellow, red) is enough for a smooth bar.

Check current config:

```bash
howmuchleft --config
```

## How it works

Claude Code invokes `howmuchleft` as a child process on each statusline render, piping a JSON object to stdin with model info, context window usage, cwd, and cost data. The script:

1. Reads the JSON from stdin
2. Fetches usage data from Anthropic's OAuth API (cached for 60s, with stale-data fallback)
3. Auto-refreshes expired OAuth tokens
4. Runs `git status` for branch/change info (in parallel with the API call)
5. Renders three lines of progress bars to stdout

## CLI

```
howmuchleft [config-dir]              Run the statusline (called by Claude Code)
howmuchleft --install [config-dir]    Add to Claude Code settings
howmuchleft --uninstall [config-dir]  Remove from Claude Code settings
howmuchleft --config                  Show config path and current settings
howmuchleft --demo [seconds]           Run a time-lapse demo (default 60s)
howmuchleft --test-colors             Preview gradient colors for your terminal
howmuchleft --version                 Show version
howmuchleft --help                    Show help
```

## Uninstall

```bash
howmuchleft --uninstall
npm uninstall -g howmuchleft
```

## Requirements

- Node.js >= 18
- Claude Code with OAuth login (Pro/Max/Team subscription)
- `git` (optional, for branch/change display)
- `gsettings` (Linux/GNOME) or `defaults` (macOS) for dark/light mode detection
