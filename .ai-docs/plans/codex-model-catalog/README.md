# Codex subscription request compatibility and model catalog

## Goal

Make a successful Codex subscription login as complete as OpenRouter onboarding:

- Send only the request fields accepted by ChatGPT's Codex backend, fixing the
  `unsupported parameter: max_output_tokens` failure.
- Fetch the authenticated account's `/codex/models` catalog and cache it so
  `/model` immediately lists every model the account can actually select.
- Preserve the `gpt-5.4 @ codex` fallback route and never change a user's
  selected default model.

## Non-goals

- Do not publish a guessed, static list of every public OpenAI API model. The
  subscription backend is authoritative for plan and rollout availability.
- Do not add API-key OpenAI onboarding, model pricing, or non-agent modalities
  (audio, image generation, realtime, embeddings).
- Do not alter the existing device-code OAuth storage or credential-refresh
  behavior.

## Prior art

- Codex's own `ResponsesApiRequest` omits `max_output_tokens` while retaining
  `model`, `instructions`, `input`, tools, reasoning, store, and stream:
  `codex-rs/codex-api/src/common.rs:273-298`.
- Codex's authenticated model client fetches the account-scoped `models`
  endpoint with the bearer token and `ChatGPT-Account-ID`, then maps returned
  model metadata into its picker: `codex-rs/codex-api/src/endpoint/models.rs`.
- Whip already uses the same model-catalog cache and zero-config picker path
  for OpenRouter: `cmd/whip/auth.go:93-131`, `internal/config/config.go:412-457`,
  and `internal/tui/modelpicker.go:128-180`.

## Design

### Codex wire client

`internal/llm/codex.go` will remove `max_output_tokens` from the subscription
Responses request body. The normal public API may accept the field, but the
subscription endpoint rejects it.

`Codex.Models` will make an authenticated `GET /codex/models` request. It will
translate the backend's model records into Whip `ModelInfo` values, keeping the
fields Whip needs: id, context window, supported reasoning efforts, and input
modalities. It will ignore backend-only fields. Failed catalog refreshes return
an error and leave an existing cached catalog intact.

### Onboarding and refresh

After device login, CLI and TUI paths fetch that catalog and write it through
the existing `~/.whip/models.json` cache. The fixed `gpt-5.4` route remains as
the compatibility fallback; all discovered models appear through catalog
resolution and can be selected without individual config entries.

The normal 24-hour catalog refresh also fetches Codex rather than deleting its
cache. This lets accounts gain or lose model access without re-authentication.

## Surfaces

| Surface | Files |
| --- | --- |
| Subscription HTTP compatibility + model discovery | `internal/llm/codex.go`, `internal/llm/codex_test.go` |
| CLI login prefetch | `cmd/whip/login.go`, `cmd/whip/login_test.go` |
| In-session login prefetch | `internal/tui/auth_cmd.go`, `internal/tui/auth_cmd_test.go` |
| Background refresh | `internal/tui/tui.go`, focused TUI tests |
| User documentation | `README.md`, `docs/models-providers.md`, `docs/features.md`, `docs/roadmap.md` |

## Test plan

- An httptest Codex Responses server receives no `max_output_tokens` for both
  stream and non-stream requests.
- An authenticated httptest `/codex/models` response maps account-scoped
  models, capabilities, and request headers correctly; non-2xx responses are
  typed HTTP errors.
- CLI and TUI device-login success cache the discovered catalog, and the model
  picker exposes a discovered model immediately.
- Catalog refresh retains the previous catalog after a failed Codex fetch.
- Run formatting, `go vet ./...`, focused tests, full `go test ./...`, and
  relevant race tests.

## Documentation plan

Replace the old fixed-route/no-catalog claim with account-scoped live model
discovery. Document that the backend's catalog—not a hard-coded list—defines
which Codex subscription models appear for each account.

## User signoff

The request to “make it all work and add in all of the supported OpenAI models
to the model provider list” authorizes this account-scoped catalog approach.
