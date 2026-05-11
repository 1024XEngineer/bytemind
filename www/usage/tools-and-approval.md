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
| `delegate_subagent`| Agent       | Delegate a sub-task to a subagent        |
| `task_output`      | Task        | Retrieve output from a background task   |
| `task_stop`        | Task        | Stop a background task                   |
| `update_plan`     | Plan        | Update the current task plan (Plan mode) |
| `web_fetch`       | Web         | Fetch and read a URL                     |
| `web_search`      | Web         | Search the web for information           |

Read tools run silently. **Write and Execute tools** pause and wait for your approval before proceeding.

## Approval Flow

When the agent wants to call a high-risk tool, it shows a summary and presents three choices:

| Option | Behavior |
| ------ | -------- |
| **Approve this operation only** | Allow just the current request; the same tool will ask again next time |
| **Approve later requests from this tool** | Auto-approve future calls from this tool for the rest of the TUI session |
| **Disable approvals for this TUI session** | Auto-approve all approval requests for the rest of this session |

You can also **deny** the request (`Esc` or selecting deny).

The default `approval_policy: on-request` enables this for every high-risk tool call.

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

## Full Access Mode

For unattended runs (CI pipelines, scripts), set `approval_mode: full_access` so approval-required operations are auto-approved and the agent doesn't block waiting for input:

```json
{
  "approval_mode": "full_access"
}
```

Legacy `approval_mode: away` is blocked by default to prevent silent privilege escalation. Set `BYTEMIND_ALLOW_AWAY_FULL_ACCESS=true` only for temporary migration.

See [Configuration](/configuration) for all approval-related settings.

## See Also

- [Configuration](/configuration) — approval policy, access modes, sandbox
- [Run Mode](/usage/run-mode) — automated non-interactive execution
- [Subagents](/usage/subagents) — delegating to specialized agents
- [MCP](/usage/mcp) — extending tools via MCP servers
- [Sandbox](/usage/sandbox) — file and command boundaries
- [Core Concepts](/core-concepts) — tools overview
