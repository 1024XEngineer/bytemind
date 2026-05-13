# Config Reference

Full reference for all fields in `~/.bytemind/config.json` and project-level `.bytemind/config.json`.

For a working example see [`config.example.json`](https://github.com/1024XEngineer/bytemind/blob/main/config.example.json).

## `provider` (single-provider, legacy)

Single model provider configuration. For configuring multiple providers and switching between them at runtime, prefer `provider_runtime` below.

| Field               | Type   | Description                                 | Default                     |
| ------------------- | ------ | ------------------------------------------- | --------------------------- |
| `type`              | string | `openai-compatible`, `anthropic`, or `gemini` | `openai-compatible`       |
| `base_url`          | string | API endpoint URL                            | `https://api.openai.com/v1` |
| `model`             | string | Model ID to use                             | `gpt-5.4-mini`              |
| `api_key`           | string | API key in plain text — convenient but stores secrets in file | -                           |
| `api_key_env`       | string | Env var name to read the key from. **When both `api_key` and `api_key_env` are set, `api_key` takes priority.** | `BYTEMIND_API_KEY`          |
| `anthropic_version` | string | Anthropic API version header                | `2023-06-01`                |
| `auth_header`       | string | Custom auth header name                     | `Authorization`             |
| `auth_scheme`       | string | Auth scheme prefix (e.g. `Bearer`)          | `Bearer`                    |
| `auto_detect_type`  | bool   | Infer provider type from `base_url`         | `false`                     |
| `family`            | string | Provider family label (for display)         | -                           |
| `api_path`          | string | Custom API path override                    | -                           |
| `models`            | array  | Available model IDs for this provider       | -                           |
| `extra_headers`     | object | Additional HTTP headers                     | -                           |

## `provider_runtime` (multi-provider)

Configure multiple model providers and switch between them at runtime with `/model`. When `provider_runtime` is present, it takes precedence over the legacy `provider` field.

### Top-level fields

| Field              | Type    | Description                                           | Default                  |
| ------------------ | ------- | ----------------------------------------------------- | ------------------------ |
| `current_provider` | string  | The currently active provider ID (e.g. `"deepseek"`)  | (first provider in map)  |
| `default_provider` | string  | Fallback provider ID                                  | same as `current_provider` |
| `default_model`    | string  | Fallback model ID when a provider has no `model` set  | -                        |
| `allow_fallback`   | bool    | Allow automatic failover to another provider          | `false`                  |
| `providers`        | object  | Map of provider ID → provider config (see below)      | (required)               |
| `health`           | object  | Health-check settings for provider failover           | see below                |

### `providers.<id>` fields

Each provider entry supports all fields from the legacy `provider` section above, plus:

| Field      | Type   | Description                                                  |
| ---------- | ------ | ------------------------------------------------------------ |
| `type`     | string | `openai-compatible`, `anthropic`, or `gemini`               |
| `base_url` | string | API endpoint URL                                             |
| `model`    | string | Currently selected model for this provider (updated by `/model`) |
| `models`   | array  | List of model IDs available for switching. **Required** for `/model` picker to show options. |
| `api_key_env` | string | Env var name to read the key from                        |
| `api_key`  | string | API key in plain text (prefer `api_key_env`)                 |

### `health` fields

| Field                      | Type | Default | Description                                |
| -------------------------- | ---- | ------- | ------------------------------------------ |
| `fail_threshold`           | int  | `3`     | Consecutive failures before marking unhealthy |
| `recover_probe_sec`        | int  | `30`    | Seconds between recovery probes            |
| `recover_success_threshold` | int  | `2`    | Consecutive successes to mark healthy      |
| `window_size`              | int  | `60`    | Rolling window size in seconds for health checks |

### How model switching works

1. Define multiple providers under `provider_runtime.providers`, each with a `models` list.
2. Start ByteMind — it uses `current_provider` and that provider's `model`.
3. Type `/model` to open the interactive picker, or `/model <provider>/<model>` to switch directly.
4. The config file is updated automatically: `current_provider` and the provider's `model` field are rewritten.

### Multi-provider example

```json
{
  "provider_runtime": {
    "current_provider": "deepseek",
    "default_provider": "deepseek",
    "default_model": "deepseek-v4-flash",
    "allow_fallback": false,
    "providers": {
      "deepseek": {
        "type": "openai-compatible",
        "base_url": "https://api.deepseek.com",
        "api_key_env": "DEEPSEEK_API_KEY",
        "model": "deepseek-v4-flash",
        "models": ["deepseek-v4-flash", "deepseek-v4-pro"]
      },
      "openai": {
        "type": "openai-compatible",
        "base_url": "https://api.openai.com/v1",
        "api_key_env": "OPENAI_API_KEY",
        "model": "gpt-5.4-mini",
        "models": ["gpt-5.4-mini", "gpt-5.4"]
      }
    },
    "health": {
      "fail_threshold": 3,
      "recover_probe_sec": 30,
      "recover_success_threshold": 2,
      "window_size": 60
    }
  }
}
```

### Adding a new provider

Edit `config.json` and add a new entry under `provider_runtime.providers`:

```json
"providers": {
  "deepseek": { ... },
  "openai": { ... },
  "my-new-provider": {
    "type": "openai-compatible",
    "base_url": "https://api.my-provider.com/v1",
    "api_key_env": "MY_PROVIDER_API_KEY",
    "model": "my-model",
    "models": ["my-model", "my-other-model"]
  }
}
```

Then restart ByteMind or use `/model` to see the new provider and its models in the picker.

:::tip Migrating from legacy `provider`
If your config only has the legacy `provider` field, ByteMind auto-converts it into `provider_runtime` on startup. Switching models with `/model` will persist the selection back to `provider_runtime`. You can also manually restructure your config to the multi-provider format above.
:::

## Setting API Key via Environment Variables

Using `api_key_env` is the recommended approach — it keeps secrets out of your config file. However, `export` only sets the variable for the current terminal session and is lost when you close the window.

### Permanent setup

**Windows (PowerShell)** — write to user-level registry, survives reboots:
```powershell
[Environment]::SetEnvironmentVariable("DEEPSEEK_API_KEY", "sk-...", "User")
```
Restart your terminal after running this command.

**Linux** — add to your shell profile:
```bash
echo 'export DEEPSEEK_API_KEY="sk-..."' >> ~/.bashrc
```

**macOS** — add to your shell profile (zsh is the default):
```bash
echo 'export DEEPSEEK_API_KEY="sk-..."' >> ~/.zshrc
```

### Temporary setup (current terminal only)

```bash
# Linux / macOS
export DEEPSEEK_API_KEY="sk-..."

# Windows PowerShell
$env:DEEPSEEK_API_KEY = "sk-..."
```

### Priority when both `api_key` and `api_key_env` are set

`api_key` (plain text in config) always takes priority over `api_key_env`. The resolution order is:

1. `api_key` — if non-empty, use it directly
2. `api_key_env` — if set, read from that environment variable
3. `BYTEMIND_API_KEY` — final fallback environment variable

If you have `api_key` in your config and also set `api_key_env`, the environment variable is ignored. Remove `api_key` from the config to use the environment variable instead.

## `approval_policy`

| Value                  | Behavior                                              |
| ---------------------- | ----------------------------------------------------- |
| `on-request` (default) | Wait for confirmation before each high-risk tool call |

## `approval_mode`

| Value                   | Behavior                                             |
| ----------------------- | ---------------------------------------------------- |
| `interactive` (default) | Prompt for approval on each operation                |
| `full_access`           | Auto-approve approval-required actions with no prompt |

## `away_policy`

Deprecated compatibility field. Legacy `approval_mode: away` is blocked by default to prevent silent privilege escalation. Temporarily set `BYTEMIND_ALLOW_AWAY_FULL_ACCESS=true` only when migrating old configs; `away_policy` remains compatibility-only.

| Value                          | Behavior                                             |
| ------------------------------ | ---------------------------------------------------- |
| `auto_deny_continue` (default) | Accepted for compatibility; no runtime behavior change |
| `fail_fast`                    | Accepted for compatibility; no runtime behavior change |

## `notifications.desktop`

Desktop notification preferences.

| Field                  | Type | Default | Description |
| ---------------------- | ---- | ------- | ----------- |
| `enabled`              | bool | `true`  | Master switch for desktop notifications. |
| `on_approval_required` | bool | `true`  | Notify when an approval prompt is raised. |
| `on_run_completed`     | bool | `true`  | Notify when a run completes successfully. |
| `on_run_failed`        | bool | `true`  | Notify when a run fails. |
| `on_run_canceled`      | bool | `false` | Notify when a run is canceled. |
| `cooldown_seconds`     | int  | `3`     | Cooldown window for duplicate notification keys. `0` disables cooldown dedupe. |

## `max_iterations`

| Type    | Default |
| ------- | ------- |
| integer | `64`    |

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

### Multi-provider (recommended)

```json
{
  "provider_runtime": {
    "current_provider": "deepseek",
    "default_provider": "deepseek",
    "default_model": "deepseek-v4-flash",
    "allow_fallback": false,
    "providers": {
      "deepseek": {
        "type": "openai-compatible",
        "base_url": "https://api.deepseek.com",
        "api_key_env": "DEEPSEEK_API_KEY",
        "model": "deepseek-v4-flash",
        "models": ["deepseek-v4-flash", "deepseek-v4-pro"]
      },
      "openai": {
        "type": "openai-compatible",
        "base_url": "https://api.openai.com/v1",
        "api_key_env": "OPENAI_API_KEY",
        "model": "gpt-5.4-mini",
        "models": ["gpt-5.4-mini", "gpt-5.4"]
      }
    },
    "health": {
      "fail_threshold": 3,
      "recover_probe_sec": 30,
      "recover_success_threshold": 2,
      "window_size": 60
    }
  },
  "approval_policy": "on-request",
  "approval_mode": "interactive",
  "notifications": {
    "desktop": {
      "enabled": true,
      "on_approval_required": true,
      "on_run_completed": true,
      "on_run_failed": true,
      "on_run_canceled": false,
      "cooldown_seconds": 3
    }
  },
  "max_iterations": 64,
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

### Single provider (legacy)

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
  "max_iterations": 64,
  "stream": true,
  "sandbox_enabled": false,
  "context_budget": {
    "warning_ratio": 0.85,
    "critical_ratio": 0.95,
    "max_reactive_retry": 1
  }
}
```
