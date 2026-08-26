# Claude Code subscription provider

Branch: `feat/claude-code-subscription`

## What this does

Adds Claude Pro/Max OAuth as a second subscription-backed provider alongside
Codex. `whip auth claude`, `whip login claude`, and `/auth claude` complete a
browser PKCE login, save a Whip-owned OAuth credential, and make a Claude
Sonnet fallback route immediately selectable.

## Goal

Use Claude subscription OAuth through the Anthropic Messages API in the same
shape as Pi Coding Agent: browser OAuth with a local callback, refreshable
credentials, and Claude Code compatibility identity on each OAuth request.

## Non-goals

- Do not read or write Claude Code's own credential store or OS keychain.
- Do not add an Anthropic API-key setup flow, account/catalog discovery, or
  pricing data. Anthropic has no subscription account-model catalog endpoint.
- Do not add a dependency: OAuth, HTTP, SSE, and JSON use the Go standard
  library.

## Prior art

- Pi's Anthropic OAuth flow uses authorization-code PKCE at
  `packages/ai/src/auth/oauth/anthropic.ts`; its OAuth provider exposes
  Claude Pro/Max as a subscription-backed `anthropic-messages` provider.
- Pi's Messages client adds the `claude-code-20250219` and
  `oauth-2025-04-20` betas, `claude-cli/<version>` user agent, `x-app: cli`,
  canonical Claude Code tool names, and a Claude Code identity system block:
  `packages/ai/src/api/anthropic-messages.ts:76-107,924-945,976-1030`.
- Whip's Codex onboarding establishes the local convention: one source owns
  token discovery/refresh, config owns an idempotent route upsert, and CLI/TUI
  reuse it. See `internal/codexauth/auth.go`, `internal/config/codex.go`, and
  `internal/tui/auth_cmd.go`.

## Design

### OAuth state and login

`internal/claudeauth/auth.go` owns `Credentials` and `Source`. It loads
Whip's `~/.whip/claude.json` first and Pi's `~/.pi/agent/auth.json` Anthropic
OAuth entry as a compatible fallback, refreshing credentials near expiry.
Successful `whip auth claude` writes only Whip's atomic, mode-0600 credential
file so it never modifies a Claude Code install or keychain.

The browser authorization-code PKCE flow starts a short-lived localhost
callback server. Its owner is the caller's context: cancellation closes the
server and unblocks the login; the listener is closed on every completion
path. CLI prints the authorization URL, while the TUI marks itself busy,
prints the URL in the transcript, and uses its existing cancel path.

### Anthropic Messages transport

`internal/llm/claude.go` maps Whip's provider-neutral conversation, tool
definitions/results, images, stream deltas, thinking, and usage to/from
`POST /v1/messages`. OAuth requests use bearer credentials and Pi's Claude
Code headers/identity. The existing agent tool loop remains the owner of tool
execution; only matching Whip built-ins are renamed to Claude Code's canonical
casing on the wire and mapped back in responses.

The provider has no account-scoped model list. `config.UpsertClaude` registers
the fixed `claude-sonnet-4-6` fallback with its known context/output limits;
users can still add explicit model routes in config.

### Surfaces

| Surface | Files |
| --- | --- |
| OAuth discovery, refresh, browser callback, persistence | `internal/claudeauth/auth.go`, `internal/claudeauth/auth_test.go` |
| Anthropic Messages client | `internal/llm/claude.go`, `internal/llm/claude_test.go` |
| Provider route | `internal/config/claude.go`, `internal/config/claude_test.go` |
| CLI onboarding + alias | `cmd/whip/auth.go`, `cmd/whip/login.go`, tests |
| In-session onboarding and routing | `internal/tui/auth_cmd.go`, `internal/tui/tui.go`, tests |
| User docs | `README.md`, `docs/features.md`, `docs/models-providers.md`, `docs/roadmap.md` |

## Test plan

1. `httptest` verifies OAuth token exchange/refresh, expiry refresh, atomic
   Whip state, Pi fallback, and cancellation of a waiting callback.
2. `httptest` verifies Messages headers, OAuth identity, conversion of text,
   tools, tool results, images, stream deltas, and usage.
3. Config/CLI/TUI tests prove the provider route is idempotent, login writes
   the route without changing defaults, and `/auth claude` is cancellable.
4. Run focused packages under `-race`, then `task check`.

## Tasks

- [x] Implement OAuth source and tests.
- [x] Implement the Messages client and tests.
- [x] Add provider config and CLI/TUI routing.
- [x] Update docs and roadmap.
- [x] Run checks, then a Ponytail least-code and correctness review. Focused
  and project-wide `go test` plus `go vet` pass with a temporary Go 1.27
  toolchain. Race builds are unavailable because this runner lacks CGO and a
  C compiler.
- [x] Commit, push, and open a draft PR against `anishthite/main`.
