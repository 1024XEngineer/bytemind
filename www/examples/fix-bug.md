# Example: Fix a Bug

This example shows how to use ByteMind to diagnose and fix a failing test or reported bug with minimal, targeted changes.

## Scenario

Your integration tests are failing: the login API returns `500 Internal Server Error` for valid credentials. You want to find the root cause and fix it without disrupting unrelated code.

## Step 1: Start with the Bug Investigation Skill

Activate the structured bug investigation workflow:

```text
/bug-investigation symptom="login API returns 500 for valid credentials in integration tests"
```

The skill guides the agent through:

1. Locating the relevant handler and middleware
2. Tracing the call chain from request to error
3. Reading logs or test output for clues
4. Forming a root cause hypothesis

## Step 2: Read Before You Write

Ask the agent to analyze first, without making any changes:

```text
Read the login handler and authentication middleware. Identify what could cause a 500 for valid credentials. Do not make any changes yet.
```

Review the agent's analysis. Confirm the root cause before proceeding.

## Step 3: Apply a Minimal Fix

```text
Apply the fix for the identified root cause. Keep the change as small as possible. Do not modify unrelated code or tests.
```

Approve each write operation when prompted.

## Step 4: Add a Regression Test

```text
Add a unit test that would have caught this bug. The test should cover the specific condition that triggered the 500.
```

## Step 5: Verify

```text
Run the affected tests to confirm the fix works.
```

The agent will call `run_shell` with the test command — approve it.

## Example Prompt (All in One)

If you prefer a single prompt for a well-understood bug:

```text
Investigate why the login API returns 500 in integration tests for valid credentials.
Apply a minimal fix and add a regression test.
Do not modify any code outside the authentication module.
```

## Expected Outcome

- Root cause clearly identified with evidence
- Change limited to the minimal required fix
- New regression test covering the exact failure path
- All existing tests still pass

## See Also

- [Bug Investigation Skill](/usage/skills#bug-investigation) — structured diagnosis workflow
- [Tools and Approval](/usage/tools-and-approval) — reviewing changes before they apply
- [Example: Refactor Code](/examples/refactor) — larger scope changes with Plan mode
