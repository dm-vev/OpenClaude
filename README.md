# OpenClaude

OpenClaude is a drop-in replacement for the **Claude Code** `claude` CLI that lets you run agents against *your own* LLM toolchain (an **OpenAI-compatible** HTTP gateway) instead of Anthropic.

## Status

This repository is currently in early scaffolding. The primary goal is to reach CLI+config compatibility first; anything not implemented yet should fail loudly and clearly.

## Goals

- Drop-in CLI compatibility with `claude` (interactive and `--print` modes).
- Linux-first single binary (no Node runtime required).
- Route all LLM calls to an OpenAI-compatible endpoint you control.
- File-only provider configuration (no required environment variables).
- Deterministic, hermetic tests (no real network).

## Non-goals

- Anthropic subscription login (`setup-token`) or any Anthropic API calls.
- VS Code extension integration and “Claude in Chrome” integration.
- Full parity with the Claude plugin ecosystem from day 1.

## Configuration

OpenClaude reads provider settings from:

- `~/.openclaude/config.json`

Example (schema may evolve, but these are the intended core fields):

```json
{
  "api_base_url": "http://localhost:12345/v1",
  "api_key": "replace-me",
  "timeout_ms": 600000,
  "default_model": "gpt-5.2-chat",
  "model_aliases": {
    "opus": "gpt-5.2-chat",
    "sonnet": "gpt-5.2-chat"
  }
}
```

Security note: keep this file `chmod 0600 ~/.openclaude/config.json`.

## Quickstart

```bash
make build
./bin/claude doctor
./bin/claude -p "что за проект в этом репозитории"
```

## Usage

Interactive session:

```bash
./bin/claude
```

Print mode (one-shot):

```bash
./bin/claude -p "hello"
```

Stream JSON (Claude Code-compatible):

```bash
./bin/claude -p "hello" --output-format=stream-json --verbose --include-partial-messages
```

Note: `--output-format=stream-json` requires `--verbose` in print mode.
Note: `--include-partial-messages` enables `stream_event` lines for streaming deltas.

## Intended CLI Compatibility

The target shape matches Claude Code:

```bash
claude [options] [command] [prompt]
```

Key behaviors to support early:

- Interactive by default (no `-p`): start a terminal session.
- `-p/--print`: non-interactive, print and exit.
- `--output-format`: `text|json|stream-json` (print-mode).
- `--input-format`: `text|stream-json` (print-mode).
- Session flags: `--continue`, `--resume`, `--session-id`.
- Tool gating: `--tools`, `--allowedTools`, `--disallowedTools`, `--permission-mode`.

## Roadmap (high level)

1. CLI parser + compatibility surface (flags/commands) with clear stubs for unsupported pieces.
2. OpenAI-compatible chat client (streaming + non-streaming).
3. Agent loop (multi-turn) + built-in tools (read/edit/bash/search).
4. Session persistence (`--continue`, `--resume`) and replayable `stream-json`.
5. Tests + “doctor” command for config/perms/health checks.
