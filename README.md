# HowMuchLeft

Pixel-perfect progress bars for your Claude Code statusline.

![Dark mode demo](./assets/demo-dark.gif)

![Light mode demo](./assets/demo-light.gif)

Three progress bars with sub-cell precision that shift from green to red as you approach your limits:

| Bar | What it tracks |
|---|---|
| **Context window** | How full your conversation is, plus subscription tier and model |
| **5-hour usage** | Rolling rate limit, time until reset, git branch and diff stats |
| **Weekly usage** | Rolling 7-day rate limit, time until reset, current directory |

Works with Pro, Max 5x, Max 20x, and Team subscriptions. API key users see context bar only.

## Install

```bash
go install github.com/smm-h/howmuchleft@latest
```

Pre-built binaries for all platforms are available on [GitHub Releases](https://github.com/smm-h/howmuchleft/releases).

## Setup

```bash
howmuchleft profile install
```

This registers the binary with Claude Code's settings.json.

## Uninstall

```bash
howmuchleft profile uninstall
```

## Config

Config lives at `~/.config/howmuchleft/config.toml`, auto-created on first run.

## Commands

| Command | Purpose |
|---------|---------|
| `howmuchleft profile install` | Register with Claude Code |
| `howmuchleft profile uninstall` | Remove from Claude Code |
| `howmuchleft profile list [--live]` | Multi-profile dashboard |
| `howmuchleft demo` | Animated demo of all bars |
| `howmuchleft colors` | Preview current gradient |
| `howmuchleft config` | Show config path and values |
| `howmuchleft version` | Print version |

## License

MIT
