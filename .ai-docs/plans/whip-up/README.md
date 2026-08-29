# Whip-up prompt command

Branch: dublin

## What this does

Adds `/whip up <prompt>` to the interactive TUI. It strips the command
prefix and starts the normal authored agent turn with `<prompt>`; e.g.,
`/whip up add retry coverage` sends `add retry coverage` to the model.

## Goal

Give users an explicit, discoverable TUI command for launching whip with a
prompt, without creating a second turn path or changing session semantics.

## Non-goals

- A new CLI subcommand (`whip up ...`). The requested surface is the TUI.
- Custom command templates, argument interpolation, or alternate models.
- Treating bare chat text beginning with `whip up` as a command; ordinary
  prompts retain their literal text.

## Design

The change stays in the TUI surface and reuses `(*model).submit`:

- `internal/tui/registry.go`: register `/whip` with its usage hint. The
  existing registry supplies `/help` and tab completion.
- `internal/tui/tui.go`: dispatch `/whip up <prompt>`. An empty/malformed
  invocation renders a usage error; a valid invocation extracts the original
  remainder after `up` and calls `submit`, so the model and transcript receive
  only the user prompt. While busy, the stripped prompt enters the existing
  turn queue, so it is still the next normal authored turn.
- `internal/tui/whip_cmd_test.go`: use the fake streaming provider to prove the
  provider receives the stripped prompt, and pin malformed-command behavior.
- `docs/features.md` and `README.md`: document the user-facing command.

No new state, goroutines, dependencies, or persistence format is needed. The
existing turn path owns cancellation, transcript blocks, input history,
session persistence, and queued messages.

## Prior art

- The shipped `/goal <text>` command dispatches to `submit` in
  `internal/tui/tui.go`; `/whip up` uses the same established path rather
  than a parallel agent invocation.
- `docs/learnings/other-harnesses/opencode/opencode-ux.md` §3 records the
  single-registry pattern for slash commands. Whip already implements that
  pattern in `internal/tui/registry.go`.

## Test plan

- Valid `/whip up <prompt>` starts one authored turn and the fake provider
  receives precisely `<prompt>`.
- `/whip`, `/whip up`, and `/whip down ...` report the usage error and do not
  start a turn.
- Existing registry tests ensure the command remains discoverable through
  help and tab completion.
- Run the focused TUI tests, `task check`, and the repository CI workflow
  against the requested `anishthite/main` base.

## Docs plan

- Add the shipped command to the TUI feature map and first-try README list.
- No roadmap item exists for this small alias, so no unrelated roadmap entry
  is added.

## Task breakdown

1. [x] Inspect the TUI dispatcher, registry, test fixtures, features map, and
   relevant command prior art.
2. [x] Add the registry and command dispatch.
3. [x] Add focused tests for prompt extraction and usage errors.
4. [x] Document the command.
5. [x] Run formatting, focused tests, `task check`, and CI-equivalent checks
   against `anishthite/main`.
6. [ ] Review the diff, commit, push, and create the PR to `context-dev/whip`.

## Adversarial review

- Queuing the command verbatim while busy would have leaked `/whip up` into
  the later model turn. The busy-input route now queues the extracted prompt,
  with a regression test.
- Invalid invocations never start a turn; they yield the single usage message.
- The implementation delegates to the established `submit` path, so it does
  not introduce separate cancellation, persistence, or transcript behavior.
