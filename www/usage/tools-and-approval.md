# Tools and Approval

Tools are the actions ByteMind's agent can take. Understanding which tools exist and how the approval system works lets you keep full control while staying efficient.

## Available Tools

| Tool              | Category    | What it does                             |
| ----------------- | ----------- | ---------------------------------------- |
| `list_files`      | Read        | List files in a directory tree           |
| `read_file`       | Read        | Read the contents of a file              |
| `search_text`     | Read        | Search files for text or regex patterns  |
| `write_file`      | **Write**   | Create or overwrite a file               |
| `replace_in_file` | **Write**   | Replace specific content inside a file   |
| `apply_patch`     | **Write**   | Apply a unified diff patch to a file     |
| `run_shell`       | **Execute** | Run a shell command                      |
| `update_plan`     | Plan        | Update the current task plan (Plan mode) |
| `web_fetch`       | Web         | Fetch and read a URL                     |
| `web_search`      | Web         | Search the web for information           |

Read tools run silently. **Write and Execute tools** pause and wait for your approval before proceeding.

## Approval Flow

When the agent wants to call a high-risk tool, it displays:

- The tool name and the exact arguments it will use
- A summary of the operation
- A prompt to approve (`y`), deny (`n`), or explain why it should not run

The default `approval_policy: on-request` enables this for every high-risk tool call.

## Recommended Workflow

1. **Analyze first** — ask the agent to read and explain before making any changes
2. **Review the plan** — check what files will be touched and why
3. **Approve incrementally** — approve each write operation only after you're satisfied
4. **Use Plan mode** for large tasks so you see the full scope before any execution

```text
First read the relevant files and tell me what changes you'd propose, without writing anything.
```

## Exec Allowlist

For trusted commands you don't want to approve repeatedly, define an `exec_allowlist` in your config:

```json
{
  "exec_allowlist": [
    { "command": "go", "args_pattern": ["test", "./..."] },
    { "command": "make", "args_pattern": ["build"] }
  ]
}
```

Allowlisted commands skip the approval prompt.

## Away Mode

For unattended runs (CI pipelines, scripts), set `approval_mode: away` so the agent doesn't block waiting for input:

```json
{
  "approval_mode": "away",
  "away_policy": "auto_deny_continue"
}
```

See [Configuration](/configuration) for all approval-related settings.

## See Also

- [Configuration](/configuration) — approval policy, away mode, sandbox
- [Run Mode](/usage/run-mode) — automated non-interactive execution
- [Core Concepts](/core-concepts) — tools overview
