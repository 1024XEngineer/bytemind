# ByteMind Prompt Architecture

## Goal

Keep prompt assembly simple, testable, and close to OpenCode's runtime layering model:

1. main system prompt text
2. mode prompt (`build` or `plan`)
3. runtime context block (environment + skills + tools)
4. optional active skill block
5. repository instruction block (`AGENTS.md`)

## Prompt Files

Prompt assets live under `internal/agent/prompts/`:

- `system_prompt.md`
- `mode/build.md`
- `mode/plan.md`
- `block-active-skill.md`

## Assembly Order

`internal/agent/prompt.go` assembles the final system prompt in fixed order:

1. `system_prompt.md`
2. `mode/{build|plan}.md`
3. `renderSystemBlock(...)`
4. `renderActiveSkillPrompt(...)` (only when a session skill is active)
5. `renderInstructionBlock(...)`

Only non-empty blocks are included.

## Runtime Inputs

`PromptInput` carries:

- `workspace`, `approval_policy`, `model`, `mode`, `platform`, `now`
- `skills` list
- `tools` list
- `active_skill` (name/description/args/tool-policy/instructions)
- `instruction` text loaded from workspace root `AGENTS.md`

## Mode Selection

`internal/agent/runner.go` resolves mode in this order:

1. explicit run mode argument
2. existing `session.mode`
3. default `build`

## Why This Replaced the Old Block Layout

The previous prompt design had additional blocks (`repo rules`, `skills summary`, `output contract`, and injected plan state) that were mostly unwired at runtime. That created complexity without behavior gain.

The current architecture keeps blocks that are always meaningful in runtime, while still supporting session-level active skill guidance.

## OpenCode Alignment Notes

This architecture keeps the same high-level layering intent as OpenCode:

- stable base prompt text
- runtime mode influence
- generated environment/tool context
- optional focused instruction block (active skill)
- instruction file injection

ByteMind intentionally remains provider-agnostic and does not use provider-specific prompt forks.
