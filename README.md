# HowMuchLeft

howmuchleft is the fastest Claude Code statusline: context window, 5-hour, and weekly limit usage as three customizable gradient bars, rendering in about 6 ms. It is for Pro, Max and Team subscribers who want to see how much of every limit is left without leaving the terminal. Usage comes from the credentials Claude Code has already stored, so there is no API key to supply and no separate login.

![Dark mode demo](./assets/demo-dark.gif)

![Light mode demo](./assets/demo-light.gif)

What each bar tracks:

| Bar | What it tracks |
|---|---|
| **Context window** | How full your conversation is, plus subscription tier and model |
| **5-hour usage** | Rolling rate limit, time until reset, git branch and the session's line counts |
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

Claude Code spawns the statusline on every render, so start-up cost is paid every time. Medians of 100 warm runs per tool on one machine, every tool fed the same status object:

| Tool | Language | Median ms |
|---|---|---|
| **howmuchleft** | Go | 5.5 |
| cship | Rust | 7.8 |
| CCometixLine (ccline) | Rust | 15.1 |
| best-claude-hud | Rust | 15.4 |
| claude-code-statusline-pro | Rust | 18.9 |
| claude-powerline | TypeScript on Node | 201.4 |

Spawning `/bin/true` on the same machine medians 2.7 ms, so a good part of every compiled tool's number is the cost of starting a process at all. The full table, what each tool shows, the caveats and the harness that produced the numbers are in [Comparison with other statuslines](https://smmh.dev/howmuchleft/comparison/).

## License

MIT
