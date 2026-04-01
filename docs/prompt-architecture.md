# ByteMind Prompt Architecture

## Goal

ByteMind's first prompt architecture is designed to be:

- rigorous enough for real coding work
- short enough to avoid wasting context on simple tasks
- composable enough to grow into rules, skills, plan, and session features later

This version intentionally does not try to encode the whole product inside one giant system prompt.

## Design Principles

1. Keep the stable behavioral rules short and explicit.
2. Inject only the runtime state that is already available in the current implementation.
3. Treat optional context as optional blocks, not mandatory boilerplate.
4. Prefer block composition over a single monolithic prompt file.
5. Do not split blocks unless the split creates a real behavioral difference.
6. Make plan mode a real execution-planning mode, not a weak wording variant.

## File Layout

The prompt files live under `internal/agent/prompts/`.

- `core.md`
  - Stable ByteMind identity, execution rules, tool discipline, response discipline, and safety bar.
- `mode-build.md`
  - Implementation-first mode instructions.
- `mode-plan.md`
  - Planning-first mode instructions with a required final answer structure.
- `block-environment.md`
  - Runtime context such as workspace, provider, model, date, approval policy, and iteration budget.
- `block-session.md`
  - Optional compressed continuity summary for resumed or ongoing sessions.
- `block-plan.md`
  - Current structured plan, rendered from session state when present.
- `block-repo-rules.md`
  - Optional project rule summary block.
- `block-skills-summary.md`
  - Optional skill summary block.
- `block-output-contract.md`
  - Optional structured output constraint block.
- `provider-openai.md`
  - Lightweight overlay for GPT/Codex and OpenAI-compatible model families.
- `provider-anthropic.md`
  - Lightweight overlay for Claude-family models.
- `provider-gemini.md`
  - Lightweight overlay for Gemini-family models.

## Assembly Model

`internal/agent/prompt.go` assembles the final system prompt from a `PromptInput` value.

Current assembly order:

1. `core.md`
2. mode block
3. environment block
4. optional provider overlay block
5. optional session block
6. optional plan block
7. optional repo rules block
8. optional skills summary block
9. optional output contract block

The final prompt is produced by concatenating only the non-empty blocks with blank lines between them.

## Current Runtime Wiring

The current runner passes these fields into `PromptInput`:

- workspace
- approval policy
- provider type
- model
- max iterations
- mode
- session summary
- session plan

This means the following blocks are fully wired today:

- `core.md`
- `mode-build.md`
- `mode-plan.md`
- `block-environment.md`
- `block-session.md` when prior session messages exist
- `block-plan.md` when `session.Plan` is non-empty
- provider overlay block when the provider or model family matches OpenAI, Anthropic, or Gemini heuristics

The following blocks are implemented in the prompt assembler but are currently optional and unused unless future runtime code supplies data:

- `block-repo-rules.md`
- `block-skills-summary.md`
- `block-output-contract.md`

## Why This Is Better Than A Single Prompt File

The old structure was:

- one `system.md`
- two template variables

That approach was simple, but it tightly coupled stable behavior rules with runtime-specific information.

The new structure separates:

- stable agent behavior
- mode-specific behavior
- runtime state
- optional future capability summaries

This keeps the prompt stricter and easier to evolve without turning it into a large always-on wall of text.

## Extension Points

This architecture is designed to support the next ByteMind features without another full rewrite.

Planned natural extensions:

- repo rule discovery
  - fill `RepoRulesSummary` from AGENTS-like files or config instructions
- session summary
  - improve the current heuristic summary or replace it with compaction output
- skills registry
  - fill `Skills` with name, description, and enabled state
- structured output
  - fill `OutputContract` only when the user explicitly asks for a schema or contract
- plan mode entry
  - switch `Mode` from `build` to `plan`

## Constraints

This first version deliberately does not yet implement:

- path-scoped lazy rules
- full skill loading
- MCP summary injection
- session memory compaction
- provider-specific deep prompt forks beyond lightweight overlays

Those should be added only when the corresponding runtime systems exist.

## Provider Handling

This version still avoids large provider-specific prompt forks, but it now supports lightweight provider/model overlays.

Reason:

- the shared execution model still belongs in `core.md`
- model families do show small but real behavioral differences in tool discipline and planning style
- a thin overlay preserves those differences without copying the whole prompt three or four times

For now, provider information is exposed in the runtime context block:

- `provider_type`
- `model`

The prompt assembler then selects at most one lightweight overlay:

- `provider-openai.md`
- `provider-anthropic.md`
- `provider-gemini.md`

The selection is heuristic and based on `provider_type` plus model-family detection.

If a larger provider-specific behavior gap appears later, it should become a real new block or execution-path difference, not a cosmetic full-prompt fork.

## Plan Handling

There are two separate plan-related concepts:

- `mode-plan.md`
  - defines how the agent must behave when planning is the primary job
- `block-plan.md`
  - injects the current execution plan state when a plan already exists

This separation is intentional.

`mode-plan.md` is a behavioral contract.
It tells the agent:

- planning is the main objective
- writes and mutating commands are not allowed
- `update_plan` is the authoritative planning tool
- the final answer must use a fixed structure: `Plan`, `Risks`, `Verification`, `Next Action`

`block-plan.md` is state.
It tells the agent:

- what the current execution plan is
- which step is active
- when the plan should be updated

This makes plan mode stricter than build mode instead of merely sounding different.
