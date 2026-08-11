# The subscription tier never resolves for token-authenticated profiles

## Problem

`GetAuthInfo` in `internal/oauth/oauth.go` returns early when the credential file carries no
access token:

- `oauth.go:162-164` — if `oauth == nil` or `oauth.AccessToken == ""`, it returns
  `IsOAuth: false, SubscriptionName: "API"` and never looks at anything else.
- `oauth.go:166-169` — the tier lookup and its `"Pro"` fallback sit *after* that return, so they
  are unreachable in that case.

A Claude Code profile authenticated by a long-lived setup token has no access token in
`<claudeDir>/.credentials.json`. The token is supplied through the environment instead. So for
every such profile the early return fires and the statusline reports `API`, regardless of the
real plan.

Verified on this machine: a live profile's `.credentials.json` contains exactly two keys,
`rateLimitTier` and `subscriptionType`, with no `accessToken`. The tier was present and correct
in the file, and was never read.

There is also no environment fallback — nothing in `internal/oauth/` or
`internal/cli/statusline.go` reads an OAuth token from the environment, so there is no second
path that could recover it.

## Consequences

1. **A Max account displays `API`.** Not a generic label with a warning — a confidently wrong
   one, with no signal that resolution failed.
2. **The `"Pro"` fallback at `oauth.go:167-169` is a second wrong answer waiting behind the
   first.** If the early return is ever relaxed without also handling an absent tier, an account
   with an unrecognised tier string will silently display `Pro`.

## Possible directions

- Treat a credential file with a tier but no access token as OAuth rather than API — the presence
  of `rateLimitTier` or `subscriptionType` is itself evidence of an OAuth profile.
- Read the token from the environment when the file lacks one, which is where token-authenticated
  sessions actually carry it.
- Make an unresolvable tier visibly unresolved rather than falling back to a specific plan name,
  so a wrong answer is distinguishable from a real one.

## Related change coming

The tier fields in `.credentials.json` are written there by the profile launcher purely so this
tool can read them. That write is being removed, because it makes the file look like it holds
session credentials when it does not — which breaks an unrelated auth check on the writer's side.
The tier will instead live in a dedicated location the launcher owns, alongside each profile.

Since the early return means this tool has never actually read those fields, that removal changes
nothing observable here. But when this bug is fixed, the tier should be read from the new location
rather than from `.credentials.json`.

## Effort

Small. One function, plus a decision about which of the three directions above is right.
