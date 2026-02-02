# Night Agent Plan — OpenClaude Compatibility

## Autonomy Mode (No-Ping)
You must proceed without waiting for user input. Use safe defaults, document decisions, and keep moving.
- Default to implementing the strictest, most compatible behavior from `cli.js` unless it conflicts with repo constraints.
- If something is unclear, pick a conservative option, record the assumption in `night.log`, and continue.
- If blocked by missing info, stub loudly with explicit errors and guidance (no silent skips).
- Do not stop for confirmation; only stop after steps 1–3 are complete or time is exhausted.

## Goal
Bring OpenClaude as close as possible to 1:1 Claude Code compatibility so the binary can be swapped without surprises. Implement everything feasible; for unsupported pieces, provide explicit stubs with clear errors and guidance. Prioritize deterministic, testable behavior.

## Priority Order (strict)
1) Stream‑JSON compatibility
2) Tools & stubs correctness
3) UI behavior (interactive + print)
4) Extra polish & hardening (only if 1–3 are complete)

---

## Step 1 — Stream‑JSON Compatibility (Highest Priority)

### 1.1 Audit current stream‑json output
- Read `cmd/claude/stream_json_*` and compare to `cli.js` and `docs/compat/claude-latest.md`.
- Identify missing fields in init/result and ordering gaps.

### 1.2 Implement missing/incorrect fields
Target fields from `cli.js`:
- **Init (`system`, `subtype=init`)** must include:
  `cwd`, `session_id`, `tools`, `mcp_servers`, `model`, `permissionMode`, `slash_commands`, `apiKeySource`, `claude_code_version`, `output_style`, `agents`, `skills`, `plugins`, `uuid`.
- **Result** must include:
  `type`, `subtype`, `is_error`, `duration_ms`, `duration_api_ms`, `num_turns`, `result`, `session_id`, `total_cost_usd`, `usage`, `modelUsage`, `permission_denials`, `uuid`.

Implementation targets:
- `cmd/claude/stream_json_control.go` (init control response)
- `cmd/claude/stream_json_system.go` (default lists + stable ordering)
- `cmd/claude/stream_json_replay.go` (ordering for replay)
- Any emitter responsible for final `result` payloads.

### 1.3 Error‑path ordering
- Ensure invalid API key path emits:
  1) `system` init
  2) `assistant` with error
  3) terminal `result` with `is_error: true`
- Match ordering in `docs/compat/claude-latest.md`.

### 1.4 Tests & fixtures
- Add/extend tests in `cmd/claude/stream_json_*_test.go`.
- Add/update golden JSONL in `internal/streamjson/testdata/claude_latest/` (or existing location).
- Ensure tests cover:
  - init fields present
  - event order
  - invalid‑API error sequence

Acceptance criteria:
- All required init/result fields present and ordered per `cli.js`.
- Error paths emit init → assistant error → result (is_error true).
- Golden fixtures updated intentionally, tests green.

---

## Step 2 — Tools & Stubs (Second Priority)

### 2.1 Tool list integrity
- Verify ordering in `internal/tools/tools.go` matches Claude Code order (from README + `cli.js` intent).
- Ensure unsupported tools are included as stubs.

### 2.2 Unsupported tool behavior
- Ensure every stub tool returns:
  - clear error message
  - stable error shape
  - guidance on alternatives
- Update `internal/tools/unsupported.go` if necessary.

### 2.3 Tool auth/permission flow
- Check `internal/agent/agent.go` and tool authorizer flow for permission mode parity in stream‑json.
- Validate `permissionMode` reflects CLI option state.

### 2.4 Tests
- Add table‑driven tests for unsupported tool errors.
- Add tests for tool list ordering if not present.

Acceptance criteria:
- Tool list order matches Claude Code expectations.
- All unsupported tools return stable error shapes with guidance.
- Tests cover ordering and error messages.

---

## Step 3 — UI (Interactive + Print)

### 3.1 Interactive mode (default)
- Confirm `cmd/claude/main.go` starts interactive session when no `-p/--print`.
- Ensure interactive output does not break stream‑json expectations when `--output-format=stream-json` is in print mode only.

### 3.2 Print mode
- Validate:
  - `--output-format text|json|stream-json` gatekeeping
  - `--input-format text|stream-json`
  - `--output-format=stream-json` requires `--verbose` (per README)
- Ensure any missing flags are implemented or stubbed loudly.

### 3.3 Docs
- Update `README.md` and/or `docs/compat.md` for any new behaviors/flags.
- Clearly mark “planned vs implemented.”

### 3.4 Tests
- Add CLI flag parsing tests in `cmd/claude` (if not present).
- Add stream‑json print‑mode path tests.

Acceptance criteria:
- Interactive mode default behavior matches README.
- Print mode flags validated and enforced.
- Docs updated for any behavior changes.

---

## Step 4 — Extra Polish & Hardening (Only If 1–3 Done)

### 4.1 Error messages & guidance
- Make error text consistent and actionable across CLI, stream‑json, and tool stubs.
- Ensure errors include next‑step hints (e.g., config path, flag usage).

### 4.2 Edge cases
- Verify missing/invalid config file handling in `internal/config`.
- Ensure session replay works with empty/malformed JSONL (clear errors).
- Confirm output formatting doesn’t crash on empty responses.

### 4.3 Docs & examples
- Update README to reflect any new exact behaviors.
- Add a small “compat status” section for what’s newly working.

### 4.4 Quality pass
- Run `gofmt` on changed Go files.
- Re‑scan for TODOs or unused code introduced by changes.

---

## Test Plan (Run Overnight)
1) `make build`
2) `make test` (or `go test ./...` if Makefile lacks target)
3) Ensure golden fixtures are updated only when behavior intentionally changes.

---

## Delivery Rules
- If time is short, complete Step 1 first, then Step 2, then Step 3.
- Always leave a brief progress log in `night.log` with:
  - completed steps
  - open items
  - assumptions made
- Do not request user input mid‑run; finish and report.

---

## Assumptions & Defaults
- Use Claude Code reference `2.1.29` per `docs/compat/claude-latest.md`.
- No real network in tests; use existing deterministic fixtures.
- “UI” includes **both** interactive and print mode.
- Prioritization: **stream‑json → tools/stubs → UI**.
- Extra scope = **polish + hardening** only after core is complete.
- Heavy Go comments required for all new/changed Go code (intent/invariants/edge cases, ending with periods).
