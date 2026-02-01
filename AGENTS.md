# OpenClaude

OpenClaude is a drop-in CLI replacement for **Claude Code** that routes all model calls to a user-controlled **OpenAI-compatible** gateway instead of Anthropic.

This file defines repo-wide rules for contributors and coding agents.

## Project Structure

Prefer standard Go layout:

- `cmd/claude/`: the `claude` CLI entrypoint.
- `internal/`: non-public packages (`agent`, `tools`, `session`, `config`, `llm`).
- `docs/`: design notes, ADRs, compatibility notes.
- `scripts/`: developer utilities (release/build helpers).

Keep packages small and cohesive; avoid circular dependencies.

## Development Workflow

Add a single, documented entrypoint for common tasks (prefer `Makefile`):

- `make build`: build the CLI binary.
- `make test`: run tests (`go test ./...`).
- `make lint`: run `gofmt` + `golangci-lint` (once configured).

When adding commands, ensure they do not rewrite repo-tracked files unexpectedly.

## Compatibility Principles

- Prefer parsing and implementing Claude Code flags/commands even if the behavior is stubbed initially.
- For unsupported features (e.g., Anthropic login/update/install), fail loudly with:
  - clear stderr message
  - stable non-zero exit code
  - guidance on alternatives
- Never silently ignore user intent.
- Treat `cli.js` in this repo as the **canonical reference** for stream-json event shapes and ordering; match its behavior whenever implementing or modifying stream-json output.

## Security Rules (Non-Negotiable)

- Never commit secrets (API keys, auth headers, session tokens).
- Provider config lives at `~/.openclaude/config.json` and must be recommended/validated as `0600`.
- Debug logs must redact secrets (e.g., `Authorization`, `api_key`, cookies).
- Tests must not make real network calls; use `httptest` servers.

## Testing Guidelines

- Prefer table-driven unit tests for config merge/model resolution.
- For streaming output, test using deterministic fake servers and golden JSONL fixtures.
- Keep tests hermetic: no dependency on `$HOME` unless explicitly sandboxed in the test.

## Docs Expectations

- `README.md` must stay accurate: clearly mark “planned” vs “implemented”.
- Any new flag/behavior must be documented (even briefly) in `README.md` or `docs/compat.md`.

## Commit Hygiene (Mandatory)

- Split changes into **small, logical commits** whenever possible.
- Every commit message must document **why** the change exists, not just what changed.
- Prefer multiple focused commits over one large commit.

## Code Commentary (Mandatory)

- All Go code must be **heavily commented**, including non-exported functions and complex blocks.
- Comments must explain intent, invariants, and edge cases—not only restate the code.
- All comments must end with a period.

## Production-Ready Bar

- Code must handle errors explicitly and return actionable context.
- Avoid panics in normal control flow.
- Keep behavior deterministic and testable; add tests for critical logic.
