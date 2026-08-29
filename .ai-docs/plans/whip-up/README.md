# Shell `whip up` command

Branch: dublin

## What this does

Adds `whip up <prompt>` to the executable's shell command surface. Every
argument after `up` is joined into one prompt and sent through the existing
headless `whip run` path; for example, `whip up add retry coverage` sends
`add retry coverage` to the model.

## Goal

Make starting a one-shot whip task from Bash quick, while preserving the
existing configuration, streaming, session, and error behavior of `whip run`.

## Non-goals

- A TUI slash command.
- New run flags or another agent/session implementation.
- Shell parsing beyond normal quoting done by the user's shell.

## Design

- `cmd/whip/main.go` dispatches `whip up ...` before starting the TUI.
- `cmd/whip/up.go` joins the command arguments and calls `runCLI` with a flag
  terminator, so prompt text such as `--format json` cannot become a run flag.
- `cmd/whip/up_test.go` exercises the request sent to the fake provider,
  flag-like prompt content, and missing-prompt validation.
- README, feature map, and roadmap document the shell command.

This is intentionally a thin adapter: `whip run` remains the owner of stdin
handling, model routing, streaming, session persistence, and cancellation.

## Prior art

- `whip run` is the shipped non-interactive one-turn command and already
  supports the behavior `whip up` needs.

## Test plan

- Confirm `whip up write the release notes` sends exactly that joined prompt.
- Confirm `whip up --format json` sends those words as prompt text, rather
  than parsing them as run flags.
- Confirm an empty invocation reports `usage: whip up <prompt>`.
- Run focused package tests, repository checks, and the CI-equivalent suite.

## Task breakdown

1. [x] Correct the requested surface from the TUI to the shell executable.
2. [x] Add the thin `whip up` dispatcher and prompt adapter.
3. [x] Add focused regression tests and update user documentation.
4. [x] Run formatting and checks, review the diff, commit, and push.
5. [ ] Open the requested pull request. Blocked: `context-dev/whip` is not a
   resolvable GitHub repository; no alternate target was substituted.

## Adversarial review

- Without `--` before the delegated prompt, a prompt beginning with `-` would
  be parsed as a `whip run` option. The adapter always inserts the terminator.
- An empty command is rejected before configuration or provider work begins.
