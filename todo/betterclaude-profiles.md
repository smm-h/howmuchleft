# Support betterclaude profiles for multi-config install/uninstall

## Context

ClaudeOverview (aka "betterclaude") manages multiple Claude configurations via profiles. Each profile points to a separate Claude config directory (e.g., `~/.claude-personal`, `~/.claude-work`), enabling users to maintain distinct OAuth credentials, settings, and statusline caches per profile.

Currently, `howmuchleft --install [config-dir]` targets a single Claude config directory. Users with multiple betterclaude profiles must run `--install` once per profile manually.

## Betterclaude file structure

- `~/.betterclaude/config.json` -- top-level config with a `default_profile` field
- `~/.betterclaude/profiles/*.json` -- one file per profile

### Profile JSON schema

```json
{
  "name": "personal",
  "claude_config_dir": "/home/user/.claude-personal",
  "gh_user": "username"
}
```

Key field for howmuchleft: `claude_config_dir` -- the directory where `settings.json` lives and where the statusline gets installed.

## Required changes

1. **Profile discovery**: read `~/.betterclaude/profiles/*.json` to collect all registered `claude_config_dir` paths.

2. **`--install --all-profiles`**: loop over discovered profiles and run the existing install logic for each `claude_config_dir`. The `userCommandLine` written into each profile's `settings.json` must reference that profile's config dir (for credential/cache lookup).

3. **`--uninstall --all-profiles`**: same loop, removing the statusline entry from each profile's `settings.json`.

4. **Fallback**: if `~/.betterclaude/profiles/` does not exist or is empty, `--all-profiles` should warn and exit cleanly.

## Effort

Small. The install/uninstall logic in `bin/cli.js` already accepts a config-dir argument and writes to `settings.json` within it. The only new work is reading the profile directory, iterating over entries, and adding the `--all-profiles` flag.
