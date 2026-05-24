# `.claude/hooks/` — project-specific hook extension points

This directory is **empty by design**. User-level harness-engineering plugin
(`~/.claude/plugins/harness-engineering`) already provides:

- **SessionStart**: the 5-step startup checklist (pwd → progress.json → features.json → test baseline → in-progress feature check) that fires at the start of every Claude Code session in this repo.
- **PostToolUse**: telemetry append to `docs/agent-telemetry.jsonl` + auto-commit of `docs/claude-progress.json` when modified (we observed this fire on PR 82cc2ad — separate `chore: update agent progress` commit appeared automatically).

**Do not duplicate those hooks at the repo level** — they'd double-fire.

## When to add a project-specific hook

Add one here only if it's behavior that belongs to **this repo specifically**, not every project the user touches. Examples that would qualify:

- **PreToolUse** for `git commit` that blocks commits touching `backend/migrations/` AND `backend/ch-migrations/` in the same change (forces splitting PG vs CH schema work).
- **PreToolUse** for `Bash` that warns if a command runs `go test` without `-count=1` (the cache occasionally masks dockertest flakes — we hit this once in SLICE-0, recorded in lessons-learned).
- **Stop** that runs a fast `gofmt -l backend/` + `npx prettier --check frontend/src` and reports drift before the session ends, so we don't ship unformatted files.

## How to add one

1. Write the hook script in this directory (e.g., `.claude/hooks/pre-commit-migrations.sh`). Make it executable: `chmod +x`.
2. Register it in `.claude/settings.json` under `hooks.PreToolUse` (or whatever event), with `"matcher"` scoped tightly (e.g., `"Bash(git commit:*)"`) — broad matchers slow every tool call.
3. Document what the hook does in a comment block at the top of the script.
4. Add a one-line entry in this README under "Active project hooks" below.

## Active project hooks

_(none yet — user-level plugin is sufficient as of 2026-05-24)_
