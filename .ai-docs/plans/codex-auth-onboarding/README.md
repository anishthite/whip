# Codex auth onboarding

Branch: `port/codex-subscription`

## Goal

Make Codex subscription sign-in follow Whip's provider-onboarding pattern:
after successful device-code OAuth, the Codex provider and its usable model
route are saved immediately, so `/model` can select it without a manual JSON
edit or a restart.

## Scope

- Add `whip auth codex` alongside `whip auth openrouter`; keep
  `whip login codex` as a compatible alias.
- Upsert the fixed Codex provider and `gpt-5.4` route after OAuth succeeds.
- Add `/auth codex` so an active TUI session can complete the same flow and
  show the route in its picker straight away.
- Preserve existing model routes and defaults. A user must choose Codex rather
  than having their current default silently switched.

## Non-goals

- A Codex model catalog: the subscription endpoint has no compatible
  `/models` endpoint, so its documented context and output limits remain the
  configuration source of truth.
- Logout or account-management UI.

## Design

`config.UpsertCodex` owns the provider/model shape and is idempotent. It adds
the `codex` route to `gpt-5.4` without replacing existing providers or explicit
limits. The OAuth credential write still happens first; only a successful login
configures Whip.

The CLI and in-session command share that config operation. The in-session
device flow marks the UI busy while it is active so Esc/Ctrl-C cancels the
same context that is polling OpenAI.

This supersedes the earlier device-login plan's non-goal of avoiding automatic
provider configuration. The user's request on 2026-08-26 is the sign-off for
this focused change.

## Test plan

1. Unit-test the idempotent config upsert and existing-route preservation.
2. Exercise device login against `httptest`, then assert the persisted route.
3. Assert `/auth codex` makes `gpt-5.4 @ codex` immediately pickable.
4. Run focused tests, `go vet ./...`, and the full test suite.

## Tasks

- [x] Add shared Codex config upsert and CLI aliases.
- [x] Add immediate in-session `/auth codex` onboarding.
- [x] Update user documentation and feature map.
- [x] Run checks and review the final diff.
