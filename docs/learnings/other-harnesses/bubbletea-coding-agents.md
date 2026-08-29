# Bubble Tea coding-agent research and Loopy plan

Research completed 2026-08-29. This note uses the repository name **whip**
when naming code paths; it is the Loopy codebase in this workspace.

## Decision

Keep Bubble Tea. Do **not** begin by migrating from Bubble Tea v1 to v2 or by
adopting Crush's Ultraviolet renderer. Whip already has the important
low-level protections—block render caching, 40 ms stream coalescing,
follow-mode bookkeeping, mouse selection, and tool rows that update in place.
The next bottleneck is more specific: every finalized streamed line rebuilds
the complete viewport string, and every width change re-renders every block.

Build a small virtual transcript layer first. It unlocks bounded stream and
resize work while retaining the existing rendering, event model, and visual
language. Add observability and two focused UX features around it. Reconsider a
v2 upgrade only after that architecture has a compatibility seam and measured
evidence that Bubble Tea v1 itself is the limiting factor.

## Who uses Bubble Tea

Bubble Tea is a production Go TUI framework based on the Elm architecture. Its
own project calls out a cell-based renderer, color downsampling, declarative
views, and keyboard/mouse/clipboard support. Its public gallery includes
chezmoi, gh-dash, Superfile, Trufflehog, MinIO's `mc`, Glow, and Mods; this is
not a niche framework chosen only by coding agents.

The most relevant agent/harness projects found were:

| Project | Bubble Tea role | Useful lesson for Loopy |
| --- | --- | --- |
| [Crush](https://github.com/charmbracelet/crush/tree/7944b8e52225d8805e31eacbf7ef24856b0dfb7a) | Full coding-agent TUI; now Bubble Tea v2, Bubbles v2, and Ultraviolet | Treat transcript rendering and resize behavior as performance-critical subsystems; cache each message and render only visible work. |
| [EVVA](https://github.com/johnny1110/evva/tree/7ae8d7a9c769f9b4db97cbdad71bd8117bfab983) | Swappable Bubble Tea UI and plain-text sink for a coding agent | Keep the UI an event consumer; serialize agent events through one queue so producer goroutines cannot block the TUI. |
| [core-tui / core-agent](https://github.com/go-steer/core-tui/tree/b6382b50ab0166c51fab428aba400417a1ea9498) | Reusable Bubble Tea v2 agent surface with a reference coding-agent host | A virtual transcript, incremental Markdown, coalesced repainting, and staged resize reflow solve the same long-session problems without requiring a new renderer. |
| [atlas.llm](https://github.com/fezcode/atlas.llm) | Small Bubble Tea chat UI for a local coding companion | Confirms the stack also fits a small single-binary, local-inference agent; it is not a performance reference. |

Crush is the closest mature comparison. EVVA and core-tui are useful secondary
references because their source makes queueing, transcript caching, and UI/runtime
boundaries explicit. The market is otherwise heterogeneous: prominent agents such
as Claude Code, Codex, Goose, and OpenCode use other UI stacks, so this is not a
reason to imitate their renderers.

## What the comparable agents do well

### Crush: optimize the hot rendering path, not the framework first

Crush uses a single top-level Bubble Tea model. Components are stateful helpers;
the root `Update` owns routing, focus, layout, and dialogs. Its chat list renders
visible items lazily, message items cache their rendered output, and expensive
syntax/Markdown/diff work is memoized. During a resize it hides exact scrollbar
geometry, reflows visible content, then warms the remaining message cache in
small batches after a 120 ms settle window. Its screen-buffer path also avoids
re-decoding an unchanged ANSI chat string into cells every frame.

Features worth borrowing selectively:

- A command palette, model/session/command dialogs, inline and overlay
  questions, file attachments/completions, LSP and MCP state, and a full diff
  viewer.
- Focus-aware finish/permission notifications with native, OSC, bell, and
  disabled modes.
- Cached background workspace probes; no filesystem or expensive work occurs in
  `Update` or `View`.

Do not copy the Ultraviolet dependency or the whole screen-buffer architecture.
Crush has a sidebar, image support, animation, and a substantially larger UI;
that rewrite would obscure whether Loopy's actual issue is its transcript.

### EVVA: a bounded ingress queue is a concurrency feature

EVVA's runtime emits typed events to a UI-agnostic sink. The Bubble Tea UI puts
all event delivery behind an `EmitQueue` pump because `Program.Send` can block
while the update loop is busy. This prevents a producer that holds a runtime lock
from mutually waiting with the TUI. It also exposes a plain-text UI, so the agent
does not depend on Bubble Tea.

Whip already follows most of this rule: streamed text is coalesced at 40 ms and
lossy tool-progress sends are detached so a parked program queue does not block a
tool goroutine. Preserve that discipline when adding transcript work. A general
queue is useful only if future producers need reliable, high-rate delivery; it is
not a prerequisite for the virtual transcript.

### core-tui: retain the current UI, make its transcript windowed

core-tui's most relevant ideas are implementation-shaped rather than cosmetic:

- History is item-addressed and rendered only for the visible window. Its
  refresh path rebuilds the live tail, not the full transcript string.
- Streaming Markdown caches a safe, completed prefix and re-renders only the
  trailing unfinished fragment; it does one complete render on finalization.
- Refreshes are coalesced through an explicit dirty/pending message pair.
- Resize work is staged: visible rows refresh after a short pause, off-screen
  rows warm in bounded slices, and stale ticks carry a generation token.
- It has focused tests and benchmarks for resize-drag latency, lazy rendering,
  event coalescing, frame-height limits, mouse paths, and Unicode width.

It is a strong design reference, not a drop-in dependency. Its v2 API and
multi-host surface differ from Whip's single-binary design.

## Loopy today: parity and the actual gap

Whip's Bubble Tea stack is currently v1.3.10 with Bubbles v1.0.0. It is already
more capable than a basic chat TUI:

- `internal/tui/tui.go` caches each transcript block by width, so unchanged
  Markdown does not go back through Glamour on every append.
- The turn bridge batches streamed text/reasoning deltas into at most one UI
  update roughly every 40 ms. The current partial line stays outside the
  transcript, so it remains cheap while it is incomplete.
- Tool calls appear before execution, running output is a bounded tail, finished
  output/diffs are compact and expandable, and blocks preserve a follow mode.
- The TUI supports palette-driven model/effort/theme controls, mouse selection,
  task/subagent docks, rewind, permissions, configuration watching, and terminal
  theme detection.

There are two scale seams:

1. `refreshVP` walks every block, joins the whole transcript, updates every
   block's Y range, and calls `viewport.SetContent` for each final assistant
   line, tool update, and other transcript mutation. Per-block Markdown work is
   cached, but string construction and viewport reset are still proportional to
   transcript size.
2. Every `tea.WindowSizeMsg` with a new width calls `refreshVP`. Each cached
   block becomes cold at that width, so dragging a terminal pane repeatedly
   re-renders the full transcript.

The existing `BenchmarkSeedTranscript` and `BenchmarkAppendStream` are the
right starting point, but the latter has a growing transcript inside one
benchmark iteration and does not separately report fixed 200/1,000-turn
refresh, streaming, and resize paths. This sandbox lacks the repository's Go
1.27 toolchain, so no local benchmark baseline is recorded in this note.

## Prioritized plan

### 0. Establish a measured contract (small, first)

**Goal:** make a rendering regression visible before changing architecture.

1. Split the current benchmark into fixed-size sub-benchmarks for 200 and
   1,000 turns: initial render, a single stream append, a completed tool row,
   steady-state `View`, and a width change.
2. Add a repeatable resize-drag benchmark or `tea.Program` probe that reports
   p50/p95 enqueue-to-`Update` and enqueue-to-frame latency. Keep a fixture with
   Markdown, a large diff, and ANSI links—plain text alone hides the expensive
   path.
3. Record the initial baseline and set budgets from it rather than inventing
   numbers. The acceptance target is flat per-stream-update allocation/time as
   history grows and no user-visible resize queueing under the chosen fixture.

**Files:** `internal/tui/*_bench_test.go`, possibly a small test-only frame
probe. **Risk:** none to product behavior.

### 1. Make the transcript a window, not one giant viewport string (highest value)

**Goal:** rendering a streaming update is bounded by visible content, not total
session history.

1. Extract the existing block layout information into a `transcript` helper:
   stable block IDs, cached rendered lines, prefix line offsets, and invalidation
   on width/theme/expand/content changes.
2. Replace the `viewport.SetContent(fullTranscript)` dependency with a small
   transcript viewport adapter that renders only intersecting block lines. Keep
   the current scroll, follow, click-to-expand, drag-selection, and bottom-pad
   contracts exactly as they are.
3. Use binary search over prefix offsets to find the first visible block; update
   only the appended/mutated block's geometry rather than rewriting every `y0` /
   `y1` range.
4. Retain current block-level render caching. This is a layout/assembly change,
   not a Markdown or style rewrite.

**Tests:** existing selection, mouse, gap, resize, tool-expand, and transcript
tests must remain green. Add long-history tests proving that the visible-window
output, click mapping, and follow behavior match the old path.

**Why before any library upgrade:** it attacks the proven O(history) assembly
path and creates the seam needed to defer off-screen resize work safely.

### 2. Stream one assistant message with incremental Markdown (high value)

**Goal:** preserve Markdown semantics and avoid re-rendering settled content.

1. Represent the in-flight assistant reply as one live transcript block instead
   of finalizing each newline as an independent Markdown block.
2. Cache the safely completed Markdown prefix (a paragraph boundary outside an
   open fence) and render only the mutable suffix per coalesced stream tick.
3. Render the complete raw message once when the turn/tool boundary finalizes;
   this is the correctness pass for tables, lists, and code fences.
4. Keep the existing 40 ms producer coalescing and the current "thinking before
   answer" ordering. Include a bounded fallback for malformed/very long
   unfinished fences so an adversarial stream cannot grow an unbounded render.

**Tests:** streamed list/table/fenced-code snapshots, resize mid-stream,
interrupt, steering, tool-start flush, and one benchmark with long prose plus a
code fence.

### 3. Stage resize work after the virtual transcript lands (high value, contained)

**Goal:** pane dragging remains responsive on large sessions.

1. Stamp width changes with a resize generation. During an active drag, lay out
   input/chrome immediately but keep cached transcript rows; clip safely at the
   draw site rather than re-rendering the backlog per column.
2. After a short quiet window, re-render the visible window at the settled
   width. Then warm off-screen blocks in bounded `tea.Tick` batches guarded by
   the generation token.
3. Avoid a repaint after every warm batch—only repaint when visible output or
   scroll geometry changes. Let stream chunks share the already-coalesced
   refresh.
4. Use the benchmark from phase 0 to tune the quiet window and batch size.

**Tests:** superseded resize ticks, terminal-width bounds, selection row mapping,
follow-mode preservation, and an input event arriving during warming.

### 4. Add two targeted user-facing features (medium value)

**Focus-aware notifications and transient toasts**

- Add a single toast state with success/error/warning/info variants and expiry;
  route command outcomes to it instead of appending routine confirmations into
  the permanent transcript.
- Offer `auto | native | osc | bell | disabled` notifications for turn finished,
  permission needed, and error. In `auto`, notify only when terminal focus
  reporting says the terminal is unfocused; otherwise do nothing. Degrade to no
  notification when focus/transport capability is unknown.
- Store the setting with the planned session preference mechanism or a narrow
  config field; do not introduce a general settings subsystem solely for it.

**Reusable picker/modal primitive**

- Extract the duplicated model/session/rewind picker mechanics into one
  focused, fuzzy-filtered list primitive before adding new dialogs. Its explicit
  push/pop and key-routing contract should make nested permission/question flows
  predictable.
- This completes two already-triaged roadmap items and follows the root-owned
  focus/modal routing used by Crush and core-tui. It should not be combined with
  the transcript performance change.

### 5. Re-evaluate Bubble Tea v2 only with evidence (defer)

Bubble Tea v2 offers a cell-based renderer and current Charm components, and
Crush/EVVA/core-tui use it. However it changes module paths and the Bubble Tea,
Bubbles, and Lip Gloss APIs. Migration adds compatibility risk across mouse,
clipboard, terminal probing, viewport, and all screenshot-like UI tests.

Revisit only if phases 1–3 meet the transcript budgets yet profiler data shows
that Bubble Tea v1's renderer dominates frame time, or a required feature is
v2-only. Put the transcript adapter behind tests first; then an isolated
v1-to-v2 compatibility branch can compare benchmark results without mixing UI
behavior changes into the migration.

## Deliberately not planned

- A Crush-style permanent sidebar, screen-buffer/Ultraviolet rewrite, image
  rendering, animated logo, or a full split diff viewer. They are expensive
  surface area without evidence they solve a Loopy pain point.
- An unconditional event pump replacement. Current stream and tool-progress
  paths already avoid blocking the runtime; add EVVA's general queue only when
  a reliable high-rate producer makes that boundary necessary.
- Copying another agent's UI wholesale. Loopy already has several features that
  those projects advertise—parallel/background subagents, MCP, LSP diagnostics,
  skills, tool permissions, compaction, session rewind, a command palette, and
  machine-readable one-shot output.

## Source notes

Primary sources were read at the pinned revisions below; external projects are
design references, not code to copy.

- [Bubble Tea README](https://github.com/charmbracelet/bubbletea) — framework
  capabilities and its public project gallery.
- [Crush README](https://github.com/charmbracelet/crush/blob/7944b8e52225d8805e31eacbf7ef24856b0dfb7a/README.md),
  [UI guide](https://github.com/charmbracelet/crush/blob/7944b8e52225d8805e31eacbf7ef24856b0dfb7a/internal/ui/AGENTS.md),
  and [chat cache/resize implementation](https://github.com/charmbracelet/crush/blob/7944b8e52225d8805e31eacbf7ef24856b0dfb7a/internal/ui/model/chat.go).
- [EVVA README](https://github.com/johnny1110/evva/blob/7ae8d7a9c769f9b4db97cbdad71bd8117bfab983/README.md),
  [Bubble Tea UI ingress](https://github.com/johnny1110/evva/blob/7ae8d7a9c769f9b4db97cbdad71bd8117bfab983/pkg/ui/bubbletea/ui.go),
  and [transcript cache](https://github.com/johnny1110/evva/blob/7ae8d7a9c769f9b4db97cbdad71bd8117bfab983/pkg/ui/bubbletea/components/transcript/cache.go).
- [core-tui README](https://github.com/go-steer/core-tui/blob/b6382b50ab0166c51fab428aba400417a1ea9498/README.md),
  [lazy refresh/incremental Markdown](https://github.com/go-steer/core-tui/blob/b6382b50ab0166c51fab428aba400417a1ea9498/tui/view.go),
  and [staged resize design](https://github.com/go-steer/core-tui/blob/b6382b50ab0166c51fab428aba400417a1ea9498/tui/resize.go).
- [atlas.llm README](https://github.com/fezcode/atlas.llm/blob/main/README.md).
