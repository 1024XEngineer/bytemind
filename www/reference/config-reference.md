# Config Reference

Full reference for all fields in `.bytemind/config.json`.

For a working example see [`config.example.json`](https://github.com/1024XEngineer/bytemind/blob/main/config.example.json).

## `provider`

Model provider configuration.

| Field               | Type   | Description                                 | Default                     |
| ------------------- | ------ | ------------------------------------------- | --------------------------- |
| `type`              | string | `openai-compatible`, `anthropic`, or `gemini` | `openai-compatible`       |
| `base_url`          | string | API endpoint URL                            | `https://api.openai.com/v1` |
| `model`             | string | Model ID to use                             | `gpt-5.4-mini`              |
| `api_key`           | string | API key (plain text — prefer `api_key_env`) | —                           |
| `api_key_env`       | string | Env var name to read the key from           | `BYTEMIND_API_KEY`          |
| `anthropic_version` | string | Anthropic API version header                | `2023-06-01`                |
| `auth_header`       | string | Custom auth header name                     | `Authorization`             |
| `auth_scheme`       | string | Auth scheme prefix (e.g. `Bearer`)          | `Bearer`                    |
| `auto_detect_type`  | bool   | Infer provider type from `base_url`         | `false`                     |

## `approval_policy`

| Value                  | Behavior                                              |
| ---------------------- | ----------------------------------------------------- |
| `on-request` (default) | Wait for confirmation before each high-risk tool call |

## `approval_mode`

| Value                   | Behavior                                             |
| ----------------------- | ---------------------------------------------------- |
| `interactive` (default) | Prompt for approval on each operation                |
| `away`                  | Unattended — use `away_policy` to determine behavior |

## `away_policy`

Only applies when `approval_mode` is `away`.

| Value                          | Behavior                                             |
| ------------------------------ | ---------------------------------------------------- |
| `auto_deny_continue` (default) | Deny high-risk operations automatically and continue |
| `fail_fast`                    | Abort the task on first operation requiring approval |

## `max_iterations`

| Type    | Default |
| ------- | ------- |
| integer | `32`    |

Maximum number of tool-call rounds per task. When reached, the agent summarizes progress and stops.

## `stream`

| Type | Default |
| ---- | ------- |
| bool | `true`  |

Enable streaming output. Set to `false` for non-TTY environments.

## `sandbox_enabled`

| Type | Default |
| ---- | ------- |
| bool | `false` |

When `true`, file and shell tools are restricted to `writable_roots`.

## `writable_roots`

| Type     | Default |
| -------- | ------- |
| string[] | `[]`    |

List of directories the agent is allowed to write to when sandbox is enabled.

## `exec_allowlist`

List of shell commands that skip the approval prompt.

```json
{
  "exec_allowlist": [
    { "command": "go", "args_pattern": ["test", "./..."] },
    { "command": "make", "args_pattern": ["build"] }
  ]
}
```

## `token_quota`

| Type    | Default  |
| ------- | -------- |
| integer | `300000` |

Warning threshold for token consumption per session.

## `update_check`

| Field     | Type | Default | Description                            |
| --------- | ---- | ------- | -------------------------------------- |
| `enabled` | bool | `true`  | Enable/disable update check on startup |

## `context_budget`

Controls context window management.

| Field                | Type  | Default | Description                                    |
| -------------------- | ----- | ------- | ---------------------------------------------- |
| `warning_ratio`      | float | `0.85`  | Emit warning at this fraction of context usage |
| `critical_ratio`     | float | `0.95`  | Trigger compaction/stop at this fraction       |
| `max_reactive_retry` | int   | `1`     | Max retries after context compaction           |

## Full Example

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-4o",
    "api_key_env": "OPENAI_API_KEY"
  },
  "approval_policy": "on-request",
  "approval_mode": "interactive",
  "max_iterations": 32,
  "stream": true,
  "sandbox_enabled": false,
  "writable_roots": [],
  "token_quota": 300000,
  "update_check": { "enabled": true },
  "context_budget": {
    "warning_ratio": 0.85,
    "critical_ratio": 0.95,
    "max_reactive_retry": 1
  }
}
```
