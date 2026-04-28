# Plan Mode

In Plan mode the agent first produces a written step-by-step execution plan for you to review and approve before any code changes are made. This gives you full visibility and control over large or risky tasks.

## Activating Plan Mode

Switch into Plan mode from any active session:

```text
/plan
```

Return to the default Build mode at any time:

```text
/build
```

## How It Works

1. You describe the task
2. The agent produces a plan listing ordered steps with estimated scope
3. You review, ask questions, or request changes to the plan
4. Once you approve, the agent executes step by step
5. Each step can prompt for confirmation before proceeding

## When to Use Plan Mode

| Scenario                                            | Why Plan mode helps                               |
| --------------------------------------------------- | ------------------------------------------------- |
| Large-scale refactors spanning many packages        | See the full impact before any file is touched    |
| Feature implementation with sequential dependencies | Prevent misordering of changes                    |
| Database migrations or schema changes               | Validate the migration sequence before running it |
| Onboarding a complex unfamiliar codebase            | Understand scope before committing to changes     |

:::tip
For simple, self-contained tasks, Build mode is faster. Switch to Plan mode when you want to **see and approve the approach** before execution begins.
:::

## Example Session

```text
/plan
Refactor the authentication module: extract token validation into a separate package,
update all callers, and add unit tests for the new package.
```

The agent will respond with a plan like:

```
Plan:
1. Read current auth module structure (internal/auth/)
2. Identify all token validation call sites
3. Create new internal/tokenval/ package with extracted logic
4. Update callers in internal/auth/ and internal/api/
5. Write unit tests for internal/tokenval/
6. Run existing test suite to verify no regressions
```

You review, then approve to start execution.

## Controlling Execution

During Plan mode execution you can:

- **Ask the agent to revise** a specific step before it runs
- **Pause** after any step to inspect results
- **Switch back to Build mode** (`/build`) to let the agent continue freely
- **Raise `max_iterations`** if the plan is long and the budget may be hit

## See Also

- [Chat Mode](/usage/chat-mode) — the interactive mode Plan mode runs inside
- [Core Concepts](/core-concepts) — Build vs Plan modes explained
- [Tools and Approval](/usage/tools-and-approval) — how approvals work during execution
