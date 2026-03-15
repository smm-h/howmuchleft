# Publish HowMuchLeft as a Claude Code Plugin

## Context

Claude Code has a full plugin system (since ~v2.0.13, Sept 2025). Users can install plugins via `/plugin install github:user/repo`. Plugins can bundle hooks, skills, slash commands, subagents, MCP servers, and LSP servers.

Publishing howmuchleft as a plugin would give users the most frictionless installation: `/plugin install github:USER/howmuchleft` from within Claude Code itself.

## Problem

The `statusLine` setting in `settings.json` is a top-level config field, not a hook or command. It's unclear whether the plugin system can:
1. Set `statusLine` config automatically on plugin install
2. Bundle a script that gets invoked as the statusline command
3. Or if it can only provide hooks/skills/commands (not statusLine config)

If plugins can't set `statusLine`, we'd need a workaround like a `/howmuchleft-install` slash command that patches settings.json when the user runs it.

## Solutions

### Option A: Native plugin with auto-config

If the plugin system supports setting `statusLine` on install:

- `.claude-plugin/plugin.json` declares the statusline
- Plugin install automatically configures everything
- Zero friction

| Pros | Cons |
|---|---|
| One-command install from within Claude Code | May not be supported by plugin system |
| Auto-discovery via /plugin Discover tab | Coupled to plugin system changes |
| No manual settings.json editing | |

### Option B: Plugin with `/howmuchleft-install` command

If plugins can't set `statusLine` directly:

- Plugin bundles a slash command that patches settings.json
- User runs `/howmuchleft-install` after plugin install

| Pros | Cons |
|---|---|
| Works within current plugin constraints | Two-step process (install plugin, then run command) |
| Still discoverable via /plugin | Modifying settings.json from a command feels hacky |

### Option C: Submit to official marketplace

Apply to get howmuchleft listed in `anthropics/claude-plugins-official`.

| Pros | Cons |
|---|---|
| Maximum discoverability | Gated by Anthropic's review |
| Trust signal from official listing | May take time to get accepted |
| Auto-available in /plugin Discover tab | Must conform to their standards |

## Recommended approach

1. Investigate: test whether `.claude-plugin/plugin.json` can declare a `statusLine` config
2. If yes: implement Option A, then apply for Option C
3. If no: implement Option B as a stopgap, file a feature request with Anthropic for plugin-level statusLine support, then apply for Option C

## Files that would change

- New: `.claude-plugin/plugin.json` -- plugin metadata
- New: `.claude-plugin/commands/install.md` -- slash command (if Option B)
- `package.json` -- add plugin-related metadata if needed

## Effort

Low (1-2 hours) once the plugin system's capabilities are confirmed. The main work is writing the plugin.json manifest and testing it.
