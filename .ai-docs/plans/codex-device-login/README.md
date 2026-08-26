# Codex device-code login

Branch: `port/codex-subscription`

## What this does

Adds `whip login codex`: a native, terminal-friendly ChatGPT subscription
sign-in. It prints the Codex verification URL and one-time code, waits for the
user to approve it in a browser, then stores the resulting OAuth credentials
in `~/.codex/auth.json`. A subsequent `whip -p codex` uses that login without
requiring Pi or the Codex CLI.

## Goal

Make the existing Codex subscription provider self-contained for a user who
has a ChatGPT subscription but has never installed another coding harness.

## Non-goals

- Browser-loopback login, API-key login, logout, or account-management UI.
- A second Whip credential store or a new dependency.
- Changing provider configuration automatically.

## Prior art

Codex's device-code implementation requests
`/api/accounts/deviceauth/usercode`, polls
`/api/accounts/deviceauth/token` for up to 15 minutes on 403/404, then
exchanges the returned authorization code at `/oauth/token` with the supplied
PKCE verifier. See
[`device_code_auth.rs`](https://github.com/openai/codex/blob/main/codex-rs/login/src/device_code_auth.rs#L781-L928)
and
[`server.rs`](https://github.com/openai/codex/blob/main/codex-rs/login/src/server.rs#L1022-L1040).
Whip follows that protocol directly with the Go standard library. The app-server
documentation confirms the device flow is responsible only for showing a URL
and user code before completion.

## Design

- `internal/codexauth/auth.go`
  - Add a small `DeviceLogin(ctx, show)` method on `Source`.
  - POST/poll/exchange against the fixed OpenAI issuer, respecting `ctx` and
    the server-provided interval. No goroutine is needed.
  - Atomically create or update Codex-compatible `~/.codex/auth.json` with
    `0600` permissions, retaining unrelated JSON fields. Prefer that file over
    Pi's fallback so a successful Whip login is the login Whip uses.
- `cmd/whip/login.go`
  - Route `whip login codex` before regular config/TUI startup.
  - Print only the verification URL and transient user code; never print
    OAuth tokens. Ctrl-C cancels polling.
- `internal/codexauth/auth_test.go` and `cmd/whip/login_test.go`
  - Use `httptest` for the three requests, check body/form shapes, persistence,
    404 behavior, cancellation, and CLI instructions.
- `README.md`, `docs/features.md`, `docs/roadmap.md`
  - Make native login the primary setup path and retain Pi/Codex state as a
    fallback.

The user explicitly requested this implementation on 2026-08-26; that is the
sign-off for this focused plan.

## Test plan

1. `go test ./internal/codexauth ./cmd/whip` proves the complete protocol
   without real credentials.
2. `go test -race ./internal/codexauth ./cmd/whip` checks cancellation and
   credential access safety.
3. `task check` runs formatting, vet, and the full suite.

## Tasks

- [x] Implement device-code auth and Codex-file persistence.
- [x] Add the `whip login codex` CLI surface and focused tests.
- [x] Document the command and fallback behavior.
- [x] Run checks and make the least-code review pass (`task check` and focused
  `go test -race`).
