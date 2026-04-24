# Example: Refactor Code

This example shows how to use Plan mode to safely execute a multi-file refactor with full visibility into every step before any code changes are made.

## Scenario

The service layer has duplicated validation logic spread across several files. You want to extract it into a shared helper, update all call sites, and keep behavior identical.

## Step 1: Use Plan Mode for Visibility

Switch to Plan mode so the agent produces a step-by-step plan before touching any files:

```text
/plan
Refactor the service layer to remove duplicated validation logic.
Extract all shared validation into a single helper function.
Keep the behavior unchanged and ensure existing tests still pass.
```

The agent will produce a plan like:

```
Plan:
1. Read all service files to identify duplicated validation patterns
2. Choose the best location for the shared helper (e.g. internal/validation/)
3. Create the helper with the extracted logic
4. Update each call site to use the new helper
5. Run the test suite to verify no behavior changes
```

## Step 2: Review the Plan

Read through each step. You can ask the agent to clarify or adjust:

```text
Before step 3, also check if there are any edge cases in the validation logic that differ between call sites.
```

## Step 3: Approve and Execute

Once you're satisfied, tell the agent to proceed:

```text
Looks good. Go ahead.
```

The agent will execute each step, pausing at high-risk write operations for your approval.

## Step 4: Review the Diff

After all writes, ask the agent to summarize:

```text
Summarize what files were changed and what the new validation helper does.
```

## Step 5: Run Tests

```text
Run the full test suite.
```

Approve the shell command when prompted. If tests fail, use the bug investigation approach to diagnose.

## Example Prompt (Single Turn)

For simpler refactors where you trust the scope:

```text
Refactor the service layer to remove duplicated validation logic while keeping behavior unchanged.
Extract the shared logic into a helper, update all callers, and run the tests to verify.
```

## Expected Outcome

- Duplicated logic removed across all service files
- New shared helper with clear responsibility
- No changes to public API contracts
- All existing tests pass

## Tips

:::tip Keep refactor PRs tight
Ask the agent explicitly: "Do not fix unrelated issues you encounter. Log them but skip them." This keeps the PR reviewable and reduces risk.
:::

## See Also

- [Plan Mode](/usage/plan-mode) — review the full scope before execution
- [Tools and Approval](/usage/tools-and-approval) — approving each write step
- [Example: Fix a Bug](/examples/fix-bug) — targeted single-issue fixes
