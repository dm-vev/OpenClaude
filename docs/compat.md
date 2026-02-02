# OpenClaude Compatibility Notes

OpenClaude aims to match Claude Code's CLI and stream-json behavior. The
canonical stream-json reference is `cli.js` at the repository root.

Current reference target:

- Claude Code `2.1.29` (see `docs/compat/claude-latest.md`).

Compatibility expectations:

- CLI flags and subcommands are parsed with Claude Code-compatible names.
- Unsupported features fail loudly with actionable guidance.
- Stream-json output ordering and event shapes should follow `cli.js`.

Compatibility status (snapshot):

- Stream-json init/result field order, auth error ordering, and permission_denials coverage are enforced by tests.
- `--disable-slash-commands` removes slash commands and skills from `system:init`.
- Tool list ordering matches Claude Code; most tools are implemented with clear fallbacks.
- Interactive mode uses a full-screen TUI (chat + tool panes, status bar), streams responses, shows tool progress with animated indicators, prompts for tool permissions, renders markdown, supports bash mode (`!`), slash-command typeahead, large paste placeholders, and a message selector (`Esc`) for forking; slash commands are stubbed with guidance.
- Task executes inline by default; async payload flags (`async`, `background`, `detached`, `run_in_background`) run in the background, with TaskOutput returning latest output when `output` is omitted and TaskStop attempting cancellation.
- AskUserQuestion requires a TTY or `OPENCLOUDE_ASK_RESPONSE`.
- Skill loads local skill files from `.openclaude/skills` or `skills` under the project root.

Known gaps are tracked in issues and in the end-of-work report for each
compatibility iteration.
