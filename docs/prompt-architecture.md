# ByteMind Prompt Architecture

## Goal

Keep prompt wiring simple and predictable:

1. One main system prompt.
2. One explicit mode prompt (`build` or `plan`).
3. One runtime context block (environment, skills, tools).
4. One instruction block loaded from `AGENTS.md`.
5. One runtime reminder for step budget (`max-steps.txt`).

## Prompt Layout

Prompt assets live in `internal/agent/prompts/`:

- `main.md`
  - Primary system prompt (based on OpenCode `gpt.txt` text).
- `mode/build.md`
  - Build mode behavior.
- `mode/plan.md`
  - Plan mode behavior.
- `max-steps.txt`
  - Synthetic reminder injected when the run reaches the step budget.
- `task/explore.txt`
- `task/compaction.txt`
- `task/summary.txt`
- `task/title.txt`
  - Specialized task prompts (not part of the main system prompt assembly).

## Assembly Order

`internal/agent/prompt.go` assembles system prompt in fixed order:

1. `main.md`
2. `mode/{build|plan}.md`
3. runtime system block (`renderSystemBlock`)
4. instructions block (`renderInstructionBlock`, loaded from workspace `AGENTS.md`)

Only non-empty parts are included.

## Runtime Behavior

`internal/agent/runner.go` handles runtime injection:

1. Reads mode from input or `session.mode` (defaults to `build`).
2. Builds the normal prompt with available tools for that mode.
3. When `step >= maxIterations`:
   - injects synthetic system reminder from `max-steps.txt`
   - sends `Tools=nil` and `ToolChoice=none`
   - expects text-only completion

## Scope Decisions

Intentionally removed from the main prompt assembly:

- provider-specific prompt overlays
- ad-hoc extra system prompt channels
- old optional prompt blocks for repo rules / output contract

Current rule source is a single workspace file: `AGENTS.md`.

