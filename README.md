# HowMuchLeft

howmuchleft is a Claude Code statusline that shows context window, 5-hour and weekly limit usage as three progress bars with sub-cell precision, shading each from green to red as it fills. It is for Pro, Max and Team subscribers who want to see how much of every limit is left without leaving the terminal. Usage comes from the credentials Claude Code has already stored, so there is no API key to supply and no separate login.

![Dark mode demo](./assets/demo-dark.gif)

![Light mode demo](./assets/demo-light.gif)

What each bar tracks:

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

## Performance

The Go rewrite is 6x faster than the previous Node.js version: 17ms average per invocation vs 101ms. Measured over 50 iterations of the real workload (stdin JSON parsing, git subprocess, config read, ANSI rendering).

## License

MIT
