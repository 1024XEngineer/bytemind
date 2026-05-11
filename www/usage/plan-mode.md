# Plan Mode

In Plan mode the agent first produces a structured execution plan (with step lists, risk annotations, and verification strategy) via the `update_plan` tool for you to review and approve before any code changes are made. This gives you full visibility and control over large or risky tasks.

## Activating Plan Mode

Two ways to switch:

| Method | Action |
| ------ | ------ |
| Slash command | Enter `/plan` to switch; `/build` to return |
| Keyboard shortcut | Press `Tab` to toggle between Build ↔ Plan |

:::tip
For simple, self-contained tasks, Build mode is faster. Switch to Plan mode when you want to **see and approve the approach** before execution begins.
:::

## How It Works

Plan mode progresses through several phases:

1. **Explore** — The agent reads relevant code and understands the current state
2. **Clarify** — If there are ambiguous decisions, the agent asks questions
3. **Draft** — The agent produces a structured plan via `update_plan` (steps, risks, verification)
4. **Converge** — You review, suggest changes, and the agent revises
5. **Approved** — You confirm the plan; the agent switches to Build mode and executes step by step

## When to Use Plan Mode

| Scenario                                            | Why Plan mode helps                               |
| --------------------------------------------------- | ------------------------------------------------- |
| Large-scale refactors spanning many packages        | See the full impact before any file is touched    |
| Feature implementation with sequential dependencies | Prevent misordering of changes                    |
| Database migrations or schema changes               | Validate the migration sequence before running it |
| Onboarding a complex unfamiliar codebase            | Understand scope before committing to changes     |

## Example Session

```text
User:
/plan
Extract token validation logic from the auth module into a dedicated tokenval
package, update all callers, and add tests for the new package.

Agent:
(Enters Explore phase, reads files under internal/auth/)

I've reviewed the auth module structure. Token validation is currently spread
across auth/middleware.go and auth/handler.go, with 4 call sites. I'll draft
a plan now.

Agent calls update_plan:
{
  "goal": "Extract token validation into internal/tokenval/",
  "steps": [
    { "title": "Create tokenval package and migrate validation logic",
      "files": ["internal/tokenval/validator.go"] },
    { "title": "Update auth package callers",
      "files": ["internal/auth/middleware.go", "internal/auth/handler.go"] },
    { "title": "Update API layer callers",
      "files": ["internal/api/router.go", "internal/api/middleware.go"] },
    { "title": "Write unit tests for tokenval",
      "files": ["internal/tokenval/validator_test.go"] },
    { "title": "Run full test suite to verify no regressions" }
  ],
  "risks": ["Incompatible interface signature may break compilation at call sites"],
  "verification": ["go test ./... passes", "Cover expired, invalid, and missing token scenarios"]
}

User:
Can steps 2 and 3 be merged? Auth and API callers can be updated together.

Agent calls update_plan to revise:
(Merges steps 2 and 3 into "Update all callers (auth + api)", now 4 steps total)

User:
Looks good, start executing.

Agent:
(Phase switches to Approved, executes each step, runs tests)
```

## Controlling Execution

During Plan mode execution you can:

- **Ask the agent to revise** a specific step before it runs
- **Pause** after any step to inspect results
- **Switch back to Build mode** (`/build` or `Tab`) to let the agent continue freely
- **Raise `max_iterations`** if the plan is long and the budget may be exceeded

## See Also

- [Interactive Mode (Build)](/usage/chat-mode) — the interactive mode Plan mode runs inside
- [Core Concepts](/core-concepts) — Build vs Plan modes explained
- [Tools and Approval](/usage/tools-and-approval) — how approvals work during execution
