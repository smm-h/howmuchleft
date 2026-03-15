# Changelog

## 0.2.0

- Truecolor support with auto-detection via `COLORTERM` env var
- Smooth RGB interpolation between gradient stops
- New `colors` array config: conditional entries with optional `dark-mode` and `true-color` matching
- Separate built-in gradients for dark/light mode and truecolor/256-color
- `colorMode` config option: `"auto"`, `"truecolor"`, or `"256"`
- JSONC config file support (comments and trailing commas)
- `--demo` flag: continuous time-lapse animation with configurable duration
- TTY detection: helpful message instead of hanging when run directly
- Configurable 256-color gradient (replaces hardcoded 4-bucket colors)
- `--test-colors` flag: preview gradient swatch at seven sample levels
- Graceful handling of empty/malformed JSON on stdin

## 0.1.0 (2026-03-15)

- Initial release
- Three-line statusline: context window, 5-hour usage, 7-day usage
- Sub-cell precision progress bars using Unicode fractional block characters
- Green-to-red color shift as usage increases
- OAuth usage API integration with 60s caching and stale-data fallback
- Automatic OAuth token refresh
- Git branch and change count display
- Lines added/removed tracking
- Dark/light mode detection (macOS and Linux/GNOME)
- `--install` / `--uninstall` for Claude Code settings.json
- `--config` to view and manage settings
- Configurable bar width and empty bar background color
