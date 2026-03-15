# HowMuchLeft

Pixel-perfect progress bars showing how much context and usage you have left, right in your Claude Code statusline.

Three bars with sub-cell precision (using Unicode fractional block characters):

- **Context window** -- how full your conversation context is, plus subscription tier and model name
- **5-hour usage** -- your rolling 5-hour rate limit utilization, time until reset, git branch/changes, lines added/removed
- **Weekly usage** -- your rolling 7-day rate limit utilization, time until reset, current directory

Bars shift from green to yellow to orange to red as usage increases. Stale data is prefixed with `~`. Works with Pro, Max 5x, Max 20x, and Team subscriptions. API key users see an "API" label with context bar only.

## Install

### npm (recommended)

```bash
npm install -g howmuchleft
howmuchleft --install
```

For a specific Claude config directory (e.g., multiple subscriptions):

```bash
howmuchleft --install ~/.claude-work
howmuchleft --install ~/.claude-personal
```

### curl

```bash
bash -c "$(curl -fsSL https://raw.githubusercontent.com/USER/howmuchleft/main/install.sh)"
```

### Manual (git clone)

```bash
git clone https://github.com/USER/howmuchleft ~/.howmuchleft
```

Then add to your Claude Code `settings.json`:

```json
{
  "statusLine": {
    "type": "command",
    "command": "node ~/.howmuchleft/bin/cli.js ~/.claude",
    "padding": 0
  }
}
```

## Configuration

Config file: `~/.config/howmuchleft.json`

```json
{
  "progressLength": 12,
  "emptyBgDark": 236,
  "emptyBgLight": 252
}
```

| Field | Default | Description |
|---|---|---|
| `progressLength` | `12` | Bar width in characters (3-40) |
| `emptyBgDark` | `236` | 256-color index for empty bar background in dark terminals |
| `emptyBgLight` | `252` | 256-color index for empty bar background in light terminals |

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
howmuchleft [config-dir]          Run the statusline (called by Claude Code)
howmuchleft --install [config-dir]    Add to Claude Code settings
howmuchleft --uninstall [config-dir]  Remove from Claude Code settings
howmuchleft --config                  Show config path and current settings
howmuchleft --version                 Show version
howmuchleft --help                    Show help
```

## Uninstall

```bash
howmuchleft --uninstall
npm uninstall -g howmuchleft
```

Or for git clone installs:

```bash
node ~/.howmuchleft/bin/cli.js --uninstall
rm -rf ~/.howmuchleft
```

## Requirements

- Node.js >= 18
- Claude Code with OAuth login (Pro/Max/Team subscription)
- `git` (optional, for branch/change display)
- `gsettings` (Linux/GNOME) or `defaults` (macOS) for dark/light mode detection
