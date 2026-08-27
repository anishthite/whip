# PR #7 feature integration notes

This branch reimplements the semantic additions from PR #7 on current `main`, without adopting its stale copies of the Codex subscription implementation.

## Experimental hashline editing

Set `"experimental": {"hashlineEdit": true}` in `~/.whip/config.json` to replace the main agent's `read` and `edit` tools with `hashline_read` and `hashline_edit`. Reads annotate every line as `LINE#HASH`; edits use those tags as staleness checks and validate a batch before writing. Subagents and MCP keep the standard tools.

## User-authored JSON themes

Theme files in `~/.whip/themes/*.json` (or `$WHIP_HOME/themes`) provide the six transcript colors and a `dark`, `light`, or `auto` background mode. `/theme` lists built-ins and loaded names, persists the selection, and reloads theme files when config syncs. `go run ./cmd/themes` is the standalone authoring playground.

## Live TPS gauge

While a completion streams, the status line estimates completion tokens per second from text deltas. `tpsGauge` selects `tach` (the default), `bar`, `spark`, `lights`, or `off` in `~/.whip/config.json`. It is live-only and resets at turn boundaries. `go run ./cmd/tps-demo -snap` renders deterministic example frames.

## Trust prompt suppression

The startup trust prompt has a `Never show again` option. It records `noTrustPrompt: true`; future untrusted folders decline startup without displaying an interactive prompt. Set it to `false` in config to restore the prompt.

## Terminal titles

Whip sets the terminal title to `whip <cwd>` after Bubble Tea starts and updates it after a successful `/cd`.
