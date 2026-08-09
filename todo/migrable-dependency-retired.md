# go.mod depends on retired migrable

migrable is retired (0.7.0 final; module carries `// Deprecated:` + a self-inclusive
retract; repo archived). howmuchleft pins v0.6.0 and builds green (verified 2026-08-08) —
pinned deps survive retraction — but the dependency is frozen upstream forever. Eventually
migrate the config-migration usage to the successor tooling (strictspec migrations) or
vendor the small engine.

Effort: medium; no urgency.
