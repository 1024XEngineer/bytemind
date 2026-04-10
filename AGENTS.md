# AGENTS.md

Repository-level instructions for ByteMind agents working in this workspace.

## Scope

- Applies to the whole repository.
- Keep changes focused on the user's request. Avoid unrelated refactors.

## Engineering Rules

- Prefer existing project patterns and helpers over introducing new abstractions.
- For Go code, use standard library solutions when they are sufficient.
- Keep functions and interfaces stable unless the task explicitly requires API changes.
- Avoid destructive operations (`git reset --hard`, bulk file deletion) unless explicitly requested.

## Validation

- Run the narrowest relevant tests first (for example `go test ./internal/agent -v`).
- If behavior changes in prompt assembly or runner flow, update or add tests in the same change.
- If full test execution is blocked by environment limits, report exactly what was run and what failed.

## Prompt Architecture Expectations

- System prompt should be assembled in this order:
  1. `internal/agent/prompts/default.md`
  2. `internal/agent/prompts/mode/{build|plan}.md`
  3. runtime context block
  4. optional active skill block
  5. this `AGENTS.md` instruction block
- Avoid provider-specific prompt forks unless there is a verified behavior gap that requires one.
