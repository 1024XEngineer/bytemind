# Chat Mode

Chat mode (`bytemind chat`) is the primary way to use ByteMind. It supports multi-turn conversations, persistent context, and dynamic task adjustment.

```bash
bytemind chat
```

## How It Works

When you start chat mode, ByteMind:

1. Reads `.bytemind/config.json` from the current directory
2. Initializes or resumes an existing session
3. Enters interactive mode and waits for your input

After you describe a task, the agent calls tools (read files, search code, run commands) to complete it. High-risk tool calls pause and wait for your approval.

## Startup Options

```bash
bytemind chat                         # use default config
bytemind chat -max-iterations 64      # raise the iteration limit
bytemind chat -config ./my.json       # use a custom config file
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
| `/quit`         | Exit safely                         |

## Interrupting and Resuming

Press `Ctrl+C` or type `/quit` at any time — the session context is automatically saved.

To resume later:

```bash
bytemind chat
/sessions          # find the session ID
/resume abc123     # resume by ID
```

## See Also

- [Session Management](/usage/session-management)
- [Tools and Approval](/usage/tools-and-approval)
- [Plan Mode](/usage/plan-mode)
- [Skills](/usage/skills)
