# Chat Mode

The default interactive mode (`bytemind`) is the primary way to use ByteMind. It supports multi-turn conversations, persistent context, and dynamic task adjustment. `bytemind chat` still works as a compatibility alias.

```bash
bytemind
```

## How It Works

When you start chat mode, ByteMind:

1. Resolves the current directory as the workspace
2. Loads the global user config and merges any optional `.bytemind/config.json` from the current workspace
3. Initializes or resumes an existing session
4. Enters interactive mode and waits for your input

:::warning Do not open very large folders directly
Start ByteMind inside a specific code repository or project subdirectory. Your home directory, a drive root, Downloads, Desktop, or a large folder with many unrelated files is not a good default workspace.
:::

After you describe a task, the agent calls tools (read files, search code, run commands) to complete it. High-risk tool calls pause and wait for your approval.

## Startup Options

```bash
bytemind                         # use the default interactive mode
bytemind -max-iterations 64      # raise the iteration limit
bytemind -config ./my.json       # use a custom config file
bytemind -workspace ./my-project # choose a workspace
```

## Best Practices

**State your goal and constraints upfront**

Tell the agent what outcome you want and what it should not touch:

```text
Add email format validation to UserService. Only change the service layer — do not modify the interface or tests.
```

**Work in small verifiable steps**

For large tasks, break work into checkpoints and confirm each one before proceeding:

```text
First just read the relevant files and analyze the current implementation. Do not make any changes yet.
```

**Activate skills to accelerate specific workflows**

Built-in skills significantly improve output quality for targeted scenarios:

```text
/bug-investigation symptom="order creation endpoint occasionally returns 500"
/review base_ref=main
/repo-onboarding
```

**Switch modes for complex tasks**

When a task needs phased execution, switch to Plan mode:

```text
/plan
Split the HTTP handler layer into a dedicated controller package. Show me the plan in stages.
```

## Session Command Reference

| Command         | Description                         |
| --------------- | ----------------------------------- |
| `/help`         | Show all available commands         |
| `/session`      | Show current session ID and summary |
| `/sessions [n]` | List recent n sessions              |
| `/resume <id>`  | Resume a session by ID or prefix    |
| `/new`          | Start a new session                 |
| `/plan`         | Switch to Plan mode                 |
| `/build`        | Switch back to Build mode           |
| `/commit <message>` | Stage all current changes and create a local Git commit |
| `/undo-commit` | Undo the last local commit created by `/commit` in this session |
| `/quit`         | Exit safely                         |

For `/commit`, choose the command from the slash palette or type it directly, then provide the commit message yourself:

```text
/commit fix(/commit): improve commit feedback
```

ByteMind stages the current workspace changes with `git add -A` before committing, then reports the commit hash, message, and file count.

Use `/undo-commit` only for the previous commit created by `/commit` in the same session. It is blocked when that commit has already reached the upstream branch, when you are in a different session, or when newer working tree changes would be mixed into the undo.

## Interrupting and Resuming

Press `Ctrl+C` or type `/quit` at any time — the session context is automatically saved.

To resume later:

```bash
bytemind
/sessions          # find the session ID
/resume abc123     # resume by ID
```

## See Also

- [Session Management](/usage/session-management)
- [Tools and Approval](/usage/tools-and-approval)
- [Plan Mode](/usage/plan-mode)
- [Subagents](/usage/subagents)
- [Skills](/usage/skills)
