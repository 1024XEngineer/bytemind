# Sandbox

ByteMind provides a multi-layer sandbox that restricts the agent's file access, command execution, and network requests. The sandbox ensures the agent stays within expected boundaries during task execution, with the approval flow still active.

## Sandbox Layers

ByteMind's sandbox provides protection at three levels:

| Layer      | Controls                    | Configuration        |
| ---------- | --------------------------- | -------------------- |
| Filesystem | Read/write directory scope  | `writable_roots`     |
| Commands   | Executable whitelist         | `exec_allowlist`     |
| Network    | Allowed domains/IPs          | `network_allowlist`  |

### Filesystem Sandbox

When enabled, write operations (`write_file`, `replace_in_file`, `apply_patch`) are restricted to directories specified in `writable_roots`. Read operations are unaffected.

```json
{
  "sandbox_enabled": true,
  "writable_roots": ["./src", "./tests", "./docs"]
}
```

With this configuration, the agent can only write to files under `./src`, `./tests`, and `./docs`. Attempts to write elsewhere (root config, system files) are blocked.

### Command Execution Sandbox

`exec_allowlist` defines commands that skip approval and are automatically allowed. Commands not in the list still require approval or trigger sandbox interception:

```json
{
  "exec_allowlist": [
    { "command": "go", "args_pattern": ["test", "./..."] },
    { "command": "go", "args_pattern": ["build"] },
    { "command": "make", "args_pattern": ["build"] },
    { "command": "npm", "args_pattern": ["test"] },
    { "command": "git", "args_pattern": ["status"] }
  ]
}
```

Each rule has `command` (executable name) and `args_pattern` (argument prefix match). `["test", "./..."]` matches both `go test ./...` and `go test ./... -v`.

### Network Sandbox

`network_allowlist` restricts destinations the agent can reach during network operations (applicable to `web_fetch`, `web_search`, and network operations within `run_shell`):

```json
{
  "network_allowlist": [
    { "host": "api.github.com" },
    { "host": "*.openai.com", "port": 443 }
  ]
}
```

## System Sandbox Mode

`system_sandbox_mode` controls how strictly sandbox constraints are enforced at the system level:

| Value         | Description                                      |
| ------------- | ------------------------------------------------ |
| `off`         | No system-level sandbox (default)                |
| `best_effort` | Apply system sandbox if supported by the platform |
| `required`    | Require system sandbox; fail if unavailable       |

:::warning
`system_sandbox_mode` requires `sandbox_enabled: true`. Setting a non-`off` value without enabling the sandbox will fail validation.
:::

```json
{
  "sandbox_enabled": true,
  "system_sandbox_mode": "best_effort"
}
```

## Sandbox vs. Approval

The sandbox and approval system are independent but complementary:

| Scenario                     | Sandbox Behavior     | Approval Behavior   |
| ---------------------------- | -------------------- | ------------------- |
| Writing within `writable_roots` | Allowed              | Approval required (default) |
| Writing outside `writable_roots` | **Blocked**          | Not triggered       |
| Executing an allowlisted command | Skipped              | No approval prompt  |
| Executing unknown command    | Risk assessment      | Approval prompt     |
| Accessing allowlisted network | Allowed              | Skipped             |
| Accessing unknown network    | Risk assessment      | Approval prompt     |

Even with `full_access` approval mode, filesystem and network sandbox restrictions remain in effect. The sandbox provides hard boundaries; approval provides interactive gating.

## Enabling via Environment Variables

Use the OS path list separator to delimit multiple writable roots (`:` on Linux/macOS, `;` on Windows):

```bash
# Linux / macOS — colon-separated
BYTEMIND_SANDBOX_ENABLED=true BYTEMIND_WRITABLE_ROOTS=./src:./tests bytemind
```

```powershell
# Windows PowerShell — semicolon-separated
$env:BYTEMIND_SANDBOX_ENABLED = "true"
$env:BYTEMIND_WRITABLE_ROOTS = "./src;./tests"
bytemind
```

Enable system sandbox mode:

```bash
BYTEMIND_SANDBOX_ENABLED=true BYTEMIND_SYSTEM_SANDBOX_MODE=best_effort bytemind
```

## Best Practices

1. **Incremental adoption** — Enable first on trusted projects, verify workflows, then expand
2. **Least privilege** — Include only directories that truly need modification in `writable_roots`
3. **Pair with exec_allowlist** — Add common safe commands (`go test`, `npm test`, etc.) to reduce approval noise
4. **CI + full_access** — Use `full_access` in CI pipelines to avoid blocking, with sandbox providing hard protection

## See Also

- [Tools and Approval](/usage/tools-and-approval) — tool approval mechanism
- [Configuration](/configuration) — full sandbox config options
- [Run Mode](/usage/run-mode) — sandbox best practices for CI automation
