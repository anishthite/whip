# SDK: a stable programmatic boundary for whip

Branch: `tripoli`
Status: RESEARCHED — protocol-first plan recommended; awaiting sign-off before
implementation.

## What this does

Defines how applications should drive whip without copying its agent loop or
depending on packages under `internal/`. The recommended first SDK is a stable,
versioned JSON Lines protocol over the existing `whip run` subprocess, followed
by one thin client library chosen for the first real consumer.

This document is research and planning only. It does not establish a public API
or commit the project to supporting every option described below.

## Goal

Let a trusted local application:

- start one foreground whip turn in an explicit workspace;
- select a model/provider and optionally resume a session;
- consume typed streaming events in order;
- cancel or time-limit the run;
- receive the final text, usage, session id, and structured failure;
- detect protocol/binary incompatibility before starting work.

The first implementation should reuse the installed whip binary and its existing
config, provider authentication, agent loop, tools, and SQLite session store.

## Non-goals

- A remote or multi-tenant agent service.
- A background daemon or `whip serve` command.
- A general plugin runtime; MCP remains the extension boundary.
- Exposing `internal/agent.Agent`, `internal/llm.Message`, or the SQLite schema as
  public compatibility surfaces.
- Host-language custom tool callbacks in v1. They require bidirectional IPC, not
  the one-way event stream proposed here.
- TUI feature parity. The SDK v1 capability profile is explicit and may be
  smaller than an interactive session.
- A sandbox. A subprocess boundary isolates Go globals, not filesystem or shell
  access; callers remain responsible for workspace isolation.
- Long-lived background subagents. A one-shot process cannot promise that a
  `task` with `background:true` survives the foreground turn.
- Publishing TypeScript and Python clients simultaneously before there is a
  known consumer for both.

## Working definition of “SDK”

There are four distinct requests hiding behind that word:

1. **Drive whip** — submit turns, stream events, resume, and cancel.
2. **Embed whip** — run the agent loop inside another Go process.
3. **Host whip** — expose persistent sessions to local or remote clients.
4. **Extend whip** — add tools and capabilities.

The recommendation in this plan addresses (1). A native Go API addresses (2),
an HTTP service addresses (3), and MCP already addresses most of (4). Treating
them as separate products keeps the first SDK small and avoids designing a
daemon or plugin system accidentally.

## Current state

Whip already has most of the internal seams needed for a driver SDK:

| Surface | What exists | SDK implication |
| --- | --- | --- |
| Agent loop | `internal/agent.Agent.Turn` is headless and synchronous; concurrency stays inside the turn. | A client can model one run as one operation with a stream of events. |
| Events | `agent.Events` exposes text, thinking, tool start/end, steering, compaction, usage, and retry callbacks. | The protocol can adapt existing events instead of inventing a second event source. |
| Headless CLI | `cmd/whip/run.go` supports text/NDJSON output, piped input, model/provider selection, resume, timeouts, turn caps, and optional persistence. | The subprocess transport already exists. |
| Persistence | `internal/session.Store` persists and resumes conversations in SQLite. | v1 can return an opaque session id; the database is not part of the SDK contract. |
| Providers | `runClientForProvider` supports OpenAI-compatible and Codex-subscription routes through existing config/auth. | No API-key or provider configuration needs to be duplicated in a client library. |
| Extensions | MCP supports stdio/HTTP tools, and `whip mcp serve` exposes whip tools to other harnesses. | New tool ecosystems should use MCP rather than a whip-only plugin API. |

The existing `--format json` output is a useful prototype, not yet an SDK
contract:

- it emits anonymous maps with only `text`, `tool_start`, `tool_end`, `done`,
  and `error`;
- it drops tool-call ids and encodes tool arguments as a JSON string;
- it does not emit thinking, usage, retries, compaction, run/session ids, or
  capability information;
- the session id is a human note on stderr rather than protocol data;
- it has no protocol version or compatibility rules;
- parallel tools invoke event callbacks from multiple goroutines, while the
  current `json.Encoder` has no serialized writer around it;
- headless mode is explicitly trusted automation: no permission prompt, and a
  nil tool gate means allow;
- `runCLI` does not share the TUI's MCP, skills, LSP, browser, or permission
  startup wiring, while `Agent.New` installs several tool definitions before
  headless flags are applied. An SDK must advertise and construct an explicit
  capability set rather than imply TUI parity.

Native embedding has additional blockers. `internal/tools` holds process-global
hooks for permissions, LSP, browser, computer-use, screenshots, interactive
bash, and suggestions. Working-directory behavior also relies on `os.Getwd`
and `os.Chdir`. Two embedded clients could therefore affect each other even if
the public package merely wrapped `Agent`.

## Prior art already in the repository

### opencode

`docs/learnings/other-harnesses/opencode/opencode-ux.md` documents two relevant
surfaces:

- `run --format json` emits a raw event stream for machine-readable runs and
  combines piped stdin with the positional message, matching whip's existing
  headless path.
- opencode also has a local HTTP server, generated SDK, attach mode, and service
  management. That architecture enables rich clients, but it is substantially
  larger than whip's current single-process design.

The useful part to port now is the machine-readable run boundary. The server
and daemon are an upgrade path, not a prerequisite.

### exo adapters

`docs/learnings/other-harnesses/exo.md` describes supervised sidecars speaking
a small JSONL event protocol over stdio. The transferable lesson is to keep the
process contract explicit, typed, bounded, and observable. Exo's durable
outbox, adapter fleet, and process supervision solve an always-on agent problem
that whip does not currently have.

### whip's browser research

`docs/learnings/browser-use-integration.md` found that a CLI plus JSON-lines
IPC can be an effective compatibility boundary, while a persistent daemon adds
identity checks, self-healing, process cleanup, locks, logs, and idle reaping.
That cost supports starting with a foreground subprocess whose lifecycle the
caller already owns.

## Options

### Option A — versioned CLI protocol plus a thin client (recommended)

Add a `json-v1` output mode to `whip run`, then wrap `os/exec`/`subprocess` in
one client library. Each run gets process isolation, while whip continues to
own providers, credentials, tools, sessions, and updates.

Advantages:

- smallest change over a shipped and tested path;
- language-neutral wire format;
- isolates current package globals and process CWD;
- no daemon installation, port selection, authentication, or shutdown logic;
- does not freeze internal Go types under semantic-version compatibility;
- no new Go dependency.

Costs and ceilings:

- one process per foreground turn;
- multi-turn conversations resume through an opaque session id rather than a
  live in-memory object;
- no host-language tool callbacks or interactive permission questions;
- no safe background work after the foreground process exits;
- concurrent writers to the same session must be rejected or documented as
  unsupported.

### Option B — native Go SDK

Expose a narrow package from the module root, for example `whip.New(...)`, with
concrete `Engine`, `Session`, `Event`, and `Result` types and small consumer-side
interfaces for providers, tools, storage, and permissions.

This is the right product if the first consumer is a Go application that needs
custom in-process providers or tools. It is not a package-visibility change:
the implementation must first move all tool hooks and workspace state behind
per-engine dependencies, make capabilities opt-in, close resources explicitly,
and define whether multiple turns or sessions may run concurrently.

Do not export the current `Agent` directly. Its mutable public fields, internal
message model, automatic tool installation, and global collaborators are an
implementation surface rather than a durable SDK contract.

### Option C — local service plus generated clients

Add `whip serve` with HTTP endpoints for sessions/turns/cancellation and SSE for
events. Generate or hand-write TypeScript, Python, and Go clients from an API
description.

This is justified by an IDE, browser app, remote attach flow, or several
simultaneous clients. It also changes the product: even loopback-only service
mode needs bearer authentication, origin checks, workspace ownership, resource
limits, crash recovery, port discovery, process supervision, and an explicit
shutdown story. Remote or multi-user operation additionally requires real
sandboxing.

### Option D — MCP extension kit

If the actual need is “developers should add tools to whip,” publish examples,
templates, naming conventions, and a test workflow for MCP servers. This reuses
the existing standard and avoids a whip-specific plugin SDK. It does not allow
an application to create or drive whip sessions.

## Decision

Start with Option A. It stops at the first ponytail rung that holds: the
headless command and event source already exist, and the subprocess boundary
contains the globals that make native embedding unsafe.

Re-evaluate only when a concrete consumer requires one of these capabilities:

- **Native Go API:** in-process custom tools/providers or process-start latency
  is materially harmful.
- **Bidirectional stdio:** a non-Go client must answer permission requests or
  execute host-language tools.
- **HTTP service:** several clients need a persistent shared process or remote
  attach.
- **Additional client language:** a real adopter cannot use the first client.

## Recommended v1 protocol

### Transport

- Invocation: `whip run --format json-v1 ...`.
- stdout: one complete JSON object per line, protocol data only.
- stderr: human diagnostics only; the SDK starts with quiet diagnostics and
  may expose stderr through a logging callback.
- cancellation: SIGINT first, SIGKILL only after a bounded grace period owned
  by the client.
- process exit: zero only after `run.completed`; non-zero after `run.failed` or
  if no terminal protocol event could be written.
- backpressure: event callbacks feed one bounded channel drained by one writer
  goroutine. The writer assigns sequence numbers and owns `json.Encoder`.
  A slow reader may slow the run; events are never dropped or concurrently
  encoded. A writer failure cancels the turn and closes a done signal that
  producer sends select on, so a full channel cannot strand tool goroutines.

### Envelope

```json
{
  "version": 1,
  "sequence": 7,
  "type": "tool.completed",
  "run_id": "run-...",
  "session_id": "session-...",
  "data": {}
}
```

Rules:

- `version`, `sequence`, `type`, and `run_id` are present on every event.
- `session_id` is omitted for `--no-session`, otherwise present from
  `run.started` onward.
- event-specific fields live in `data` so envelope evolution stays separate.
- unknown event types and unknown optional fields must be ignored by clients.
- fields are additive within v1; removal, renaming, or semantic changes require
  a new protocol version.
- tool input is raw JSON, not a JSON-encoded string.
- tool output remains bounded by whip's existing output cap.
- provider credentials and resolved secret values are never protocol fields.

### Event set

| Event | Required data |
| --- | --- |
| `run.started` | binary version, protocol version, model, provider, workspace, enabled capabilities |
| `message.delta` | text delta |
| `reasoning.delta` | reasoning delta |
| `tool.started` | tool-call id, name, JSON input |
| `tool.completed` | tool-call id, name, output, duration/status when known |
| `steer.received` | injected text or a redacted/typed attachment summary |
| `context.compacted` | messages removed/kept and summary cutoff; summary text optional |
| `request.retrying` | attempt, maximum attempts, delay, structured error |
| `usage.updated` | request usage and cumulative run usage |
| `run.completed` | final text, final usage, outstanding-task count (v1 should be zero) |
| `run.failed` | stable code, message, retryable flag, partial usage |

Initial failure codes should stay deliberately small: `invalid_request`,
`configuration`, `provider`, `timeout`, `cancelled`, `max_turns`,
`persistence`, `protocol_write`, and `internal`. New codes may be added within
v1; clients must handle unknown codes. Messages remain human-readable details,
not classification keys.

### Compatibility handshake

An accepted run begins with `run.started`. Argument, configuration, or provider
initialization failures may instead emit one terminal `run.failed`; every event
carries the protocol version, so either first event is a valid compatibility
handshake. A client validates the version before accepting event data. Keep
legacy `--format json` unchanged during v1 introduction; scripts may already
depend on its current shapes. A future release may alias `json` to the
versioned format only through an explicit deprecation cycle.

## Client shape

The client should remain a process wrapper, not a second implementation of
whip configuration or session semantics:

```text
client = Whip(binary, workspace, logger)
run = client.run(prompt, model?, provider?, session?, timeout?, capabilities?)
for event in run.events:
    ...
result = run.wait()
run.cancel()
```

Required behavior:

- locate a caller-supplied binary or `whip` on `PATH`; do not auto-download in
  v1;
- set the subprocess working directory explicitly;
- expose an async event iterator plus a final result;
- preserve the opaque session id for resume;
- turn malformed/truncated NDJSON, incompatible versions, signals, and non-zero
  exits into distinct client errors;
- drain stderr concurrently with stdout so neither pipe can deadlock;
- make cancellation and timeout idempotent;
- enforce one in-flight run per session object in the client;
- require an explicit trusted/capability choice before enabling mutating tools.

Choose the first client language from the first integration. Do not add Node or
Python tooling to this Go repository speculatively. The protocol and its
conformance fixtures live here; a language package can live here or in its own
repository once its release and ownership model are known.

## Capability and safety model

The current headless command is for trusted automation. SDK naming and defaults
must not imply sandboxing or interactive approval that does not exist.

For v1:

- the start event reports the exact enabled tool/capability names;
- browser and computer-use are disabled unless explicitly requested and wired;
- background subagents are disabled or reject `background:true` with tool
  output explaining the one-shot lifecycle;
- clients must opt into mutating tool execution with an unmistakable setting;
- the workspace is explicit, but absolute reads and shell behavior remain host
  capabilities unless a separate sandbox is supplied;
- secrets stay in whip's config/provider layer and never enter events or client
  configuration by default;
- when persistence is requested, open/create/load/save failures become
  `persistence` run failures. `run.completed` with a session id guarantees that
  the completed turn was saved; the legacy format may retain best-effort saves.

A read-only profile may be added, but it must be described as a tool allowlist,
not a filesystem sandbox: even reading can escape the workspace unless path
enforcement is separately implemented.

## Surfaces and likely implementation files

### Phase 1 — stable protocol in the binary

- `internal/protocol/` (new): versioned envelope, event payload types, stable
  error codes, and the single-writer stream.
- `cmd/whip/run.go`: add `json-v1`, generate run/session ids, adapt every
  `agent.Events` callback, persist before the terminal success event, and emit
  exactly one terminal event.
- `cmd/whip/run_test.go`: protocol, ordering, resume, timeout, failure, and
  concurrent-tool stream tests.
- `internal/agent/task.go`: explicitly reject or disable background mode for
  the one-shot capability profile.
- `docs/features.md`: document the versioned machine interface after it ships.
- `docs/roadmap.md`: check the SDK item only when the protocol and first client
  are both usable.

### Phase 2 — first thin client

- client package/repository chosen by the first consumer;
- async stream parser, cancellation, timeout, binary discovery, session resume,
  and compatibility validation;
- conformance tests consuming protocol fixtures produced by Phase 1;
- one runnable example against a fake provider or isolated test configuration.

### Phase 3 — only if demanded

- bidirectional stdio for permission responses and host-language tools;
- native Go facade after per-instance dependency refactoring; or
- local HTTP/SSE service after lifecycle and threat-model design.

## Test plan

### Protocol unit tests

- every event marshals to the documented envelope;
- raw tool input remains valid nested JSON;
- unknown optional payload fields can be ignored by a reference decoder;
- stable error codes do not depend on provider error text;
- the stream emits strictly increasing sequence numbers;
- concurrent callback senders produce valid, whole NDJSON records under
  `go test -race`;
- closing the stream drains accepted events and rejects later sends without a
  panic or goroutine leak.

### Headless integration tests

- fake provider text stream → `started`, deltas, usage, `completed`;
- parallel tool calls → paired ids and valid serialized records;
- tool failure stays `tool.completed` data and does not become infrastructure
  `run.failed`;
- provider retry and compaction callbacks reach the protocol;
- timeout/SIGINT → one structured failure and non-zero exit;
- new and resumed sessions expose the correct opaque id;
- `--no-session` omits it;
- stdout contains protocol lines only, even when diagnostics are produced;
- legacy `--format json` output remains unchanged;
- background task mode is unavailable with an actionable tool result.

### Client conformance tests

- accepts supported v1 fixtures and ignores added optional fields/events;
- rejects unsupported protocol versions before yielding work events;
- distinguishes protocol corruption, binary exit, run failure, and timeout;
- drains a large stderr stream without deadlock;
- cancellation is safe before start, during streaming, and after completion;
- refuses concurrent turns on the same session object.

### Gates

- `task check` for each implementation phase;
- `go test -race ./cmd/whip ./internal/agent ./internal/protocol` for the
  protocol phase;
- the client language's formatter, type checker, unit tests, and one
  binary-level smoke test for the client phase.

## Documentation plan

When implementation ships:

- add the machine-readable run contract, compatibility policy, and safety
  warning to `docs/features.md` and the CLI/config reference;
- add one minimal streaming example for the first client;
- document binary-version compatibility and how upgrades are handled;
- document that the default transport is local subprocess IPC, not a sandbox or
  remote service;
- check the SDK roadmap item only after both the protocol and one client are
  released.

README changes wait until users can install a client package; this research PR
does not advertise an unshipped SDK.

## Ordered task breakdown

1. [ ] Confirm the first client language and consuming application.
2. [ ] Confirm v1's enabled tool profile and mutating-tool opt-in name.
3. [ ] Add protocol types, stable error codes, and the bounded single-writer
       event stream.
4. [ ] Add `--format json-v1` and adapt the full `agent.Events` surface.
5. [ ] Make one-shot background-task behavior explicit and test it.
6. [ ] Add protocol compatibility, ordering, concurrency, resume, cancellation,
       and legacy-format tests.
7. [ ] Run `task check` and the focused race suite.
8. [ ] Build the first thin client against conformance fixtures.
9. [ ] Add a binary smoke test and runnable client example.
10. [ ] Update shipped-feature docs and check the roadmap item.
11. [ ] Re-evaluate native Go, bidirectional stdio, and HTTP service only from
        concrete unmet requirements reported by the first consumer.

## Open questions for sign-off

1. Which application will consume the SDK first, and in what language?
2. Should SDK v1 expose the current trusted coding-tool profile, or require a
   smaller allowlist unless the client explicitly opts into mutations?
3. Does the first consumer need MCP/skills/LSP parity with the TUI, or is the
   current `whip run` capability set the intended product?
4. Is session resume through the shared SQLite store sufficient, or does the
   consumer require an in-memory multi-turn process?
