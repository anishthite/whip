# SDK: a stable programmatic boundary for whip

Branch: `tripoli`
Status: RESEARCHED — consumer-first path selection recommended; awaiting
sign-off before implementation.

## What this does

Defines how applications should drive whip without copying its agent loop or
depending on packages under `internal/`. There are two credible first products:
a narrow native Go facade for an embedded Go consumer, or a stable, versioned
JSON Lines protocol over `whip run` for process and non-Go consumers. The first
committed integration should select between them; do not build both
speculatively.

This document is research and planning only. It does not establish a public API
or commit the project to supporting every option described below.

## Goal

Let a trusted local application:

- start one foreground whip turn in an explicit workspace;
- select a model/provider and optionally resume a session;
- consume typed streaming events in order;
- cancel or time-limit the run;
- receive the final text, usage, session id, and structured failure;
- use a documented compatibility boundary rather than internal Go types.

The first implementation should reuse whip's existing config, provider
authentication, agent loop, tools, and SQLite session store. A process client
does that through the installed binary; an embedded Go client does it through a
small public facade and shared internal construction code.

## Non-goals

- A remote or multi-tenant agent service.
- A background daemon or `whip serve` command.
- A general plugin runtime; MCP remains the extension boundary.
- Exposing `internal/agent.Agent`, `internal/llm.Message`, or the SQLite schema as
  public compatibility surfaces.
- Host-language custom tool callbacks in a subprocess v1. They require
  bidirectional IPC; a native Go facade can support them after it has a public
  tool adapter.
- TUI feature parity. The SDK v1 capability profile is explicit and may be
  smaller than an interactive session.
- A sandbox. A subprocess boundary isolates Go globals, not filesystem or shell
  access; callers remain responsible for workspace isolation.
- Long-lived background subagents in a one-shot process. It cannot promise that
  a `task` with `background:true` survives the foreground turn.
- Publishing TypeScript and Python clients simultaneously before there is a
  known consumer for both.
- Migrating the TUI onto the SDK merely to prove the facade. The TUI is allowed
  to remain a first-party client of internal capabilities.
- Treating message replacement as rewind/fork. Those TUI operations also own
  redo state, persistence, transcripts, and workspace snapshots.

## Working definition of “SDK”

There are four distinct requests hiding behind that word:

1. **Drive whip** — submit turns, stream events, resume, and cancel.
2. **Embed whip** — run the agent loop inside another Go process.
3. **Host whip** — expose persistent sessions to local or remote clients.
4. **Extend whip** — add tools and capabilities.

This plan evaluates both (1) and (2), but recommends implementing only the one
required by the first consumer. An HTTP service addresses (3), and MCP already
addresses most of (4). Treating them as separate products keeps the first SDK
small and avoids designing a daemon or plugin system accidentally.

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

## Two internal surfaces are currently welded together

A useful distinction is between the turn engine and the session editor. A scan
of the current TUI found direct `m.agent` references on 127 non-test source
lines. That count is not itself a reason to migrate the TUI; it identifies
which behavior should not accidentally become the SDK contract.

| Surface | Current shape | Initial SDK treatment |
| --- | --- | --- |
| Turn engine | `Turn`, `Steer`, usage snapshots, and event callbacks form a coherent headless operation. Tool callbacks may arrive concurrently from parallel tool goroutines. | Wrap this. Give it public input/result/event types and one serialized event stream. |
| Session editing | The TUI directly changes messages, model, effort, context limits, compaction settings, and provenance, while coordinating database rows, redo state, transcripts, and workspace snapshots. | Keep internal. Add narrow verbs only when a real embedded consumer needs them. Do not expose mutable fields or raw internal messages. |

This permits a smaller native SDK than a full `Agent` cleanup. The TUI can keep
using the richer internal object while the facade initially supports only a
turn-oriented contract. However, the facade still needs more than a one-file
wrapper: public adapters cannot leak `internal` types, construction logic is
currently split across CLI/TUI code, event delivery must serialize concurrent
callbacks, and global tool/workspace state needs an explicit lifecycle policy.

For an initial single embedded consumer, the lifecycle policy can be a clearly
documented ceiling rather than an immediate architectural rewrite: one engine
per process, one active turn per session, and host-owned process working
directory. If the consumer requires multiple independent engines or workspaces
in one process, per-engine tool dependencies and workspace roots become
prerequisites rather than follow-up cleanup.

Illustrative wrapper signatures must also be checked against the current tree:
`Agent.Client` is an `llm.Client` interface, tool start/end callbacks include a
call id, `Events` includes compaction and retry callbacks, and tool callbacks
can run concurrently. The current `whip run --format json` already provides the
subprocess option in prototype form. The remaining work is to stabilize and
complete that boundary, not create it from zero.

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

### Option A — versioned CLI protocol plus a thin client

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
concrete `Client`, `Run`, `Event`, and `Result` types. Start with turn-engine
operations rather than trying to make the mutable internal `Agent` public.

This is the right product if the first consumer is a Go application that needs
in-process control or custom providers/tools. The smallest safe version may
document one client per process and host-owned CWD instead of first moving every
global behind per-engine dependencies. It must still make capabilities opt-in,
close owned resources explicitly, adapt internal values into public types, and
reject unsupported concurrency. Multiple independent engines or workspace
roots require the larger per-instance dependency refactor before release.

Add session-editing verbs only from concrete requirements. Plausible examples
are `SetEffort`, `SetModel`, `History`, and `Compact`; a raw `ReplaceHistory`
method is not a substitute for the TUI's durable rewind/fork behavior. Prefer
verbs that preserve invariants over exposing fields or slices.

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

Do not select a transport until the first consuming application is named. Then
choose exactly one initial path:

| First consumer requirement | Start with | Why |
| --- | --- | --- |
| CI, scripts, another language, or strong process isolation | Option A | `whip run` already provides the lifecycle boundary and contains package globals/CWD. |
| A Go application needing in-process control or host callbacks | Option B | A subprocess wrapper would omit the capability that motivated the SDK. |
| Several persistent or remote clients | Option C, after a threat/lifecycle design | Neither a one-shot process nor a single-engine facade meets the requirement. |
| Only adding tools to whip | Option D | MCP is already the extension boundary. |

Absent a committed embedded-Go consumer, Option A remains the lower-risk
default: the headless command and event source already exist, and the process
boundary contains the globals that make native embedding unsafe. Conversely,
if the first consumer is embedded Go, building JSONL first would create an
intermediate product it may never use.

Re-evaluate only when a concrete consumer requires one of these capabilities:

- **Native Go API:** in-process custom tools/providers or process-start latency
  is materially harmful.
- **Bidirectional stdio:** a non-Go client must answer permission requests or
  execute host-language tools.
- **HTTP service:** several clients need a persistent shared process or remote
  attach.
- **Additional client language:** a real adopter cannot use the first client.

## Option A candidate: versioned JSONL protocol

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

### Option A client shape

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

## Option B candidate: narrow native Go facade

Prefer the module-root package `github.com/context-labs/whip` over a generic
`sdk` subpackage. The facade should expose public concrete values and translate
to internal agent/config/message types rather than exporting them indirectly.
The first API sketch is intentionally turn-shaped:

```go
client, err := whip.New(whip.WithConfigFile(path))
if err != nil { /* ... */ }
defer client.Close()

run, err := client.Turn(ctx, whip.TurnRequest{Prompt: prompt})
if err != nil { /* ... */ }

for event := range run.Events() {
    // Event is a public tagged value; tool callbacks are serialized here.
}
result, err := run.Wait()
```

Exact names wait for the first integration, but the contract should preserve
these properties:

- construction reuses one shared internal builder also used by the binary;
- `Turn` returns a run handle with a bounded, single-writer event stream;
- cancellation is context-based and `Close` is idempotent;
- public results, usage, errors, events, and tool descriptors contain no
  `internal/...` types;
- one active turn per client/session is enforced until concurrency semantics
  are intentionally designed;
- a second client in the same process fails clearly while process-global tool
  hooks remain; it must not silently overwrite the first client's callbacks;
- the host owns CWD in the constrained version. Do not call `os.Chdir` from a
  library. A per-client workspace option requires refactoring tool path
  resolution around an explicit root first;
- custom tools/providers are added only if required by the first consumer, via
  small public interfaces and adapters rather than exposing registries;
- session-editing methods are verbs with defined persistence semantics. A
  memory-only `History`/`Compact` surface may be valid; `Rewind` or `Fork` must
  account for durable TUI state before claiming those names.

This facade is narrower than migrating the TUI. It can be useful with a
documented single-engine ceiling, but it is not merely a roughly 50-line
wrapper: extracting shared construction, adapting types, guarding globals,
serializing events, and testing lifecycle behavior are part of the minimum
product.

## Capability and safety model

The current headless command is for trusted automation. SDK naming and defaults
must not imply sandboxing or interactive approval that does not exist.

For either initial path:

- the start event reports the exact enabled tool/capability names;
- browser and computer-use are disabled unless explicitly requested and wired;
- clients must opt into mutating tool execution with an unmistakable setting;
- the workspace is explicit, but absolute reads and shell behavior remain host
  capabilities unless a separate sandbox is supplied;
- secrets stay in whip's config/provider layer and never enter events or client
  configuration by default.

For Option A specifically:

- background subagents are disabled or reject `background:true` with tool
  output explaining the one-shot lifecycle;
- when persistence is requested, open/create/load/save failures become
  `persistence` run failures. `run.completed` with a session id guarantees that
  the completed turn was saved; the legacy format may retain best-effort saves.

For Option B specifically:

- the single-engine/process and host-owned-CWD constraints are checked and
  documented until the underlying globals are removed;
- `Close` releases every resource and global lease acquired by the client;
- embedded callers receive the same explicit capability profile as process
  callers rather than inheriting TUI defaults accidentally.

A read-only profile may be added, but it must be described as a tool allowlist,
not a filesystem sandbox: even reading can escape the workspace unless path
enforcement is separately implemented.

## Surfaces and likely implementation files

### Selection gate — shared by both paths

- Name the consuming application and language.
- Write down its minimum capability set: session resume, custom tools/providers,
  MCP/skills/LSP/browser parity, permission callbacks, and process-isolation
  needs.
- Decide whether it can accept one engine/process, one turn/session, and
  host-owned CWD.
- Choose Option A or Option B. Do not start implementation while the answer is
  “a generic SDK for future consumers.”

### Path A — stable protocol and first thin client

- `internal/protocol/` (new): versioned envelope, event payload types, stable
  error codes, and the single-writer stream.
- `cmd/whip/run.go`: add `json-v1`, generate run/session ids, adapt every
  `agent.Events` callback, persist before the terminal success event, and emit
  exactly one terminal event.
- `cmd/whip/run_test.go`: protocol, ordering, resume, timeout, failure, and
  concurrent-tool stream tests.
- `internal/agent/task.go`: explicitly reject or disable background mode for
  the one-shot capability profile.
- Client package/repository chosen by the first consumer: async parsing,
  cancellation, binary discovery, session resume, compatibility validation,
  conformance fixtures, and one runnable example.

### Path B — constrained native Go facade

- module-root Go files (new package `whip`): options, `Client`, turn request,
  run handle, public event/result/error types, lifecycle guard, and adapters;
- a new focused internal construction package: share config/provider/agent/tool
  setup that is currently split between `cmd/whip/run.go` and
  `internal/tui.buildAgent` without importing either UI package;
- `internal/agent`: add only the invariant-preserving verbs required by the
  first consumer; do not expose mutable messages or configuration fields;
- event adapter: copy callback data into public values and serialize callbacks
  from parallel tool goroutines through a bounded channel;
- lifecycle: enforce one client per process while global hooks exist, reject a
  second constructor, and release the lease/resources in `Close`;
- workspace: use the host's stable CWD for the constrained release, or first
  refactor tools to accept an explicit root if per-client workspaces are a hard
  requirement;
- `example_test.go` or `examples/`: one runnable consumer flow using a fake or
  isolated provider.

### Later — only if demanded

- bidirectional stdio for permission responses and host-language tools;
- removal of the native single-engine/CWD ceiling through per-instance tool and
  workspace dependencies; or
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

### Native facade tests

- no exported signature contains a type from `internal/...`;
- constructor failure releases partially acquired resources and the global
  engine lease;
- a second simultaneous client fails deterministically while globals remain;
- `Close` is idempotent and permits a later client;
- parallel tool callbacks yield ordered, race-free public events;
- cancellation closes events and completes `Wait` without a goroutine leak;
- unsupported concurrent turns return a stable error;
- public history/results are defensive copies, not aliases of agent slices;
- the library never changes process CWD;
- any added session-editing verb preserves its documented persistence behavior.

### Gates

- `task check` for each implementation phase;
- `go test -race ./cmd/whip ./internal/agent ./internal/protocol` for the
  protocol path;
- the client language's formatter, type checker, unit tests, and one
  binary-level smoke test for the process-client path;
- `go test -race ./...` plus a small external-package compile test for the
  native facade path.

## Documentation plan

When implementation ships:

- document the selected boundary, compatibility policy, capability profile,
  concurrency ceiling, and safety warning in `docs/features.md`;
- add one minimal streaming example for the first client;
- for Option A, document binary/protocol compatibility and that local
  subprocess IPC is not a sandbox or remote service;
- for Option B, document source compatibility, the single-engine/CWD limits,
  ownership, and `Close` behavior;
- check the SDK roadmap item only after the selected boundary and its first
  usable consumer package are released.

README changes wait until users can install a client package; this research PR
does not advertise an unshipped SDK.

## Ordered task breakdown

1. [ ] Confirm the consuming application, language, and minimum capability set.
2. [ ] Choose Path A (process protocol) or Path B (native Go); record why the
       other path does not meet the first consumer as directly.
3. [ ] Confirm the enabled tool profile and mutating-tool opt-in name.
4. [ ] If Path A: add the versioned single-writer protocol and full event
       adapter, make one-shot background behavior explicit, and preserve the
       legacy format.
5. [ ] If Path A: add ordering/concurrency/resume/cancellation compatibility
       tests, then build the first thin client from conformance fixtures.
6. [ ] If Path B: extract shared construction and add the module-root facade,
       public type adapters, bounded event stream, and global lifecycle guard.
7. [ ] If Path B: add only consumer-required session/tool/provider verbs and
       tests for races, ownership, concurrency rejection, CWD, and cleanup.
8. [ ] Run `task check` and the selected path's race/integration suite.
9. [ ] Add a runnable example and shipped-feature documentation.
10. [ ] Check the roadmap item after the selected SDK is usable by its first
        consumer.
11. [ ] Re-evaluate the unselected path, bidirectional stdio, or HTTP only from
        concrete unmet requirements.

## Open questions for sign-off

1. Which application will consume the SDK first, and in what language?
2. Does it require in-process custom tools/providers, or is a managed `whip`
   subprocess acceptable?
3. If it is embedded Go, can it accept one engine/process, one turn/session,
   and host-owned CWD for the first release?
4. Should SDK v1 expose the current trusted coding-tool profile, or require a
   smaller allowlist unless the client explicitly opts into mutations?
5. Does the first consumer need MCP/skills/LSP parity with the TUI, or is the
   current `whip run` capability set the intended product?
6. Is session resume through the shared SQLite store sufficient, or does the
   consumer require an in-memory multi-turn process?
