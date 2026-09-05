# Show the context window size after the context percentage

## Context

The statusline shows how full the context window is as a percentage
(rendered in `internal/render/compose.go`, the cyan `fmt.Sprintf("%d%%", ...)`
segment fed by `Context float64`). The percentage comes from the statusline
stdin JSON: `context_window.used_percentage`, extracted by
`extractContextPercent` in `internal/cli/statusline.go`.

## Problem

A percentage alone does not say how big the window actually is. 45% of a
200k window and 45% of a 1m window are very different situations. The user
wants the window size displayed right after the percentage, in compact
form, e.g. `45% 1m` or `45% 200k`.

## Requested behavior

After the context percentage, append the total size of the context window,
abbreviated: `1m` for 1,000,000 tokens, `200k` for 200,000 tokens, etc.

## Solutions

1. **Read the size from the statusline stdin payload (preferred if present).**
   Inspect the real `context_window` object Claude Code sends — it may carry
   a total/max token count alongside `used_percentage` (the exact field name
   must be verified against a live payload; the test fixture in
   `internal/cli/statusline_test.go` only exercises `used_percentage`).
   - Pros: authoritative, no maintenance, correct for any model or plan.
   - Cons: only works if the field exists; must be verified first.
2. **Derive it from used tokens + percentage** if the payload carries a
   used-token count but no total: `total = used / (percentage/100)`, rounded
   to a known bucket (200k, 500k, 1m).
   - Pros: works without an explicit total field.
   - Cons: undefined at 0%; rounding is a heuristic.
3. **Map the model id to a known window size** (the payload already carries
   the model id, extracted near the top of `statusline.go`).
   - Pros: always available.
   - Cons: a hand-maintained table that goes stale as models change, and it
     cannot see per-session variations (e.g. extended-context betas).

If option 1's field exists, use it alone; only reach for 2 or 3 if it does
not. Whatever the source, when the size cannot be determined, show only the
percentage as today — never a guessed size.

Formatting: lowercase compact suffix, no space inside the number (`1m`,
`200k`, `800k`); exact values that are not round get abbreviated to the
nearest sensible unit. Decide whether the size is always shown or behind a
config option (the config surface is in `internal/config/config.go`).

## Affected files

- `internal/cli/statusline.go` — extract the size (new extraction alongside
  `extractContextPercent`), thread it into the render data.
- `internal/render/compose.go` — render data struct (`Context` field's
  neighborhood) and the percentage segment.
- `internal/cli/statusline_test.go` — fixtures gain the new field; new cases
  for present/absent size.
- `internal/demo/demo.go` — demo data should show the new segment.
- `internal/config/config.go` — only if a toggle is added.
- `README.md` / docs — document the new segment.

## Effort estimate

Small: about an hour including verifying the live payload shape, tests, and
docs.
