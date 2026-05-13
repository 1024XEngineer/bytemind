# Configuration

ByteMind normally loads the global user config first, then an optional project config for the current workspace. Matching fields in the project config override the global config. New users should start with the global config: `~/.bytemind/config.json`.

Automatic load order:

1. `~/.bytemind/config.json` in your home directory
2. `.bytemind/config.json` in the current workspace (optional project overrides)

If you pass `-config <path>`, ByteMind uses that file for this run.

## OpenAI-Compatible Providers

Works with OpenAI, DeepSeek, Tongyi Qianwen, Azure OpenAI, and other compatible services:

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-4o",
    "api_key": "YOUR_API_KEY"
  },
  "approval_policy": "on-request",
  "max_iterations": 64,
  "stream": true
}
```

Pass the API key via environment variable (recommended, to avoid writing secrets to files):

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-4o",
    "api_key_env": "OPENAI_API_KEY"
  }
}
```

```bash
export OPENAI_API_KEY="sk-..."
bytemind
```

## Anthropic

```json
{
  "provider": {
    "type": "anthropic",
    "base_url": "https://api.anthropic.com",
    "model": "claude-sonnet-4-20250514",
    "api_key": "YOUR_API_KEY",
    "anthropic_version": "2023-06-01"
  },
  "approval_policy": "on-request",
  "max_iterations": 64,
  "stream": true
}
```

## Local / Custom Models

Any endpoint that speaks the OpenAI chat completions format works:

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "http://localhost:11434/v1",
    "model": "qwen2.5-coder:7b",
    "api_key": "ollama"
  }
}
```

:::tip Auto-detect provider type
Set `"auto_detect_type": true` to let ByteMind infer the provider type from `base_url` automatically.
:::

## Multi-Provider Setup (Model Switching)

ByteMind supports configuring multiple model providers and switching between them at runtime without restarting. Use `provider_runtime` instead of the legacy `provider` field:

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
    }
  }
}
```

### Key points

- **`providers.<id>.models`** is required — this is the list of models you can switch between with `/model`.
- **`providers.<id>.model`** is the currently active model for that provider. It gets updated automatically when you switch.
- **`api_key_env`** is preferred over `api_key` — keeps secrets out of your config file.

### Switching models

| Command | What it does |
| ------- | ------------ |
| `/model` | Open interactive picker to browse all configured providers and models |
| `/model deepseek/deepseek-v4-pro` | Switch directly to deepseek-v4-pro |
| `/model openai/gpt-5.4` | Switch directly to GPT-5.4 |
| `/models` | Show all discovered models and current active model |

The config file is updated automatically after switching — no need to edit it manually.

### Adding a new provider

Edit your `config.json` and add a new entry under `provider_runtime.providers`:

```json
"providers": {
  "deepseek": { ... },
  "openai": { ... },
  "anthropic": {
    "type": "anthropic",
    "base_url": "https://api.anthropic.com",
    "api_key_env": "ANTHROPIC_API_KEY",
    "model": "claude-sonnet-4-20250514",
    "models": ["claude-sonnet-4-20250514", "claude-opus-4-20250514"]
  }
}
```

Restart ByteMind or use `/model` to see the new provider in the picker.

### Adding a new model to an existing provider

Edit the `models` array for that provider:

```json
"deepseek": {
  "type": "openai-compatible",
  "base_url": "https://api.deepseek.com",
  "api_key_env": "DEEPSEEK_API_KEY",
  "model": "deepseek-v4-flash",
  "models": ["deepseek-v4-flash", "deepseek-v4-pro", "deepseek-v4-flash-2"]
}
```

Save and use `/model` to see the new model in the picker.

:::tip Migrating from the legacy `provider` field
If your config has `provider` (single), ByteMind auto-converts it to `provider_runtime` on startup. After using `/model` to switch, the config is persisted in `provider_runtime` format. You can also manually restructure your config following the multi-provider example above.
:::

## Approval Policy

`approval_policy` controls when high-risk tools (file writes, shell commands) pause for confirmation:

| Value                  | Behavior                                                     |
| ---------------------- | ------------------------------------------------------------ |
| `on-request` (default) | Agent waits for confirmation before each high-risk operation |

`approval_mode` sets the overall interaction style:

| Value                   | Behavior                                             |
| ----------------------- | ---------------------------------------------------- |
| `interactive` (default) | Prompt for approval on each operation                |
| `full_access`           | Auto-approve approval-required actions with no prompt |

Legacy compatibility is now gated: `approval_mode: away` is blocked by default to prevent silent privilege escalation. Set `BYTEMIND_ALLOW_AWAY_FULL_ACCESS=true` only as a temporary migration escape hatch.

`away_policy` (deprecated compatibility field):

| Value                          | Behavior                                             |
| ------------------------------ | ---------------------------------------------------- |
| `auto_deny_continue` (default) | Accepted for compatibility; no runtime behavior change |
| `fail_fast`                    | Accepted for compatibility; no runtime behavior change |

:::warning Full access caution
In `full_access` mode, approval-required operations are auto-approved with no prompt. Keep sandbox/allowlist settings strict when running unattended.
:::

## Sandbox

When sandbox is enabled, file and shell tools are restricted to the declared writable roots:

```json
{
  "sandbox_enabled": true,
  "writable_roots": ["./src", "./tests"]
}
```

You can also enable it via environment variables:

```bash
BYTEMIND_SANDBOX_ENABLED=true BYTEMIND_WRITABLE_ROOTS=./src bytemind
```

## Iteration Budget

`max_iterations` caps the number of tool-call rounds per task, preventing runaway loops:

```json
{
  "max_iterations": 64
}
```

When the limit is reached, the agent produces a progress summary and stops instead of exiting with an error. Raise this value for complex refactors or large migrations.

## Token Quota

`token_quota` sets the warning threshold for token consumption per task (default: 300,000):

```json
{
  "token_quota": 500000
}
```

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
  "max_iterations": 64,
  "stream": true,
  "update_check": { "enabled": true },
  "context_budget": {
    "warning_ratio": 0.85,
    "critical_ratio": 0.95,
    "max_reactive_retry": 1
  }
}
```

See [Config Reference](/reference/config-reference) for the full field list.
