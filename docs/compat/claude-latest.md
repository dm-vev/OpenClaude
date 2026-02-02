# Claude Code Reference (Latest)

This document records the canonical Claude Code reference we are matching for
CLI and stream-json compatibility.

## Reference Version

- Version: `2.1.29`.
- Build time: `2026-01-31T20:12:07Z`.
- Reference date: `2026-02-01`.

## Source Artifacts

We use public artifacts for behavioral reference only. The upstream source is
not vendored in this repository due to licensing.

Primary references:

- NPM package: `@anthropic-ai/claude-code@2.1.29` (contains `cli.js`).
- Installer script:

```text
https://claude.ai/install.sh
```

- Public distribution bucket (versioned manifests and binaries):

```text
https://storage.googleapis.com/claude-code-dist-86c565f3-f756-42ad-8dfa-d59b1c096819/claude-code-releases
```

## Captured Behavior Notes

- `claude --help` and subcommand help output are used to mirror flag/command
  shapes and descriptions.
- `--version` prints `2.1.29 (Claude Code)`.
- Streaming JSON output on invalid API key emits:
  1) `system` init event.
  2) `assistant` error message event.
  3) terminal `result` event with `is_error: true`.

A sanitized JSONL fixture is checked into
`internal/streamjson/testdata/claude_latest/stream-json.invalid-api.jsonl` for
regression comparison.
