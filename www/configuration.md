# Configuration

ByteMind loads config from the first path it finds, in this order:

1. Path passed via `-config <path>` flag
2. `.bytemind/config.json` in the current working directory
3. `config.json` in the current working directory
4. `~/.bytemind/config.json` in your home directory

## OpenAI-Compatible Providers

Works with OpenAI, DeepSeek, Azure OpenAI, and any service that implements the `/v1/chat/completions` interface:

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-4o",
    "api_key": "YOUR_API_KEY"
  },
  "approval_policy": "on-request",
  "max_iterations": 32,
  "stream": true
}
```

Pass the API key via environment variable (recommended — keeps secrets out of files):

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
bytemind chat
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
  "max_iterations": 32,
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

## Approval Policy

`approval_policy` controls when high-risk tools (file writes, shell commands) pause for confirmation:

| Value                  | Behavior                                                     |
| ---------------------- | ------------------------------------------------------------ |
| `on-request` (default) | Agent waits for confirmation before each high-risk operation |

`approval_mode` sets the overall interaction style:

| Value                   | Behavior                                             |
| ----------------------- | ---------------------------------------------------- |
| `interactive` (default) | Prompt for approval on each operation                |
| `away`                  | Unattended — handled automatically via `away_policy` |

`away_policy` (applies only when `approval_mode` is `away`):

| Value                          | Behavior                                             |
| ------------------------------ | ---------------------------------------------------- |
| `auto_deny_continue` (default) | Automatically deny high-risk operations and continue |
| `fail_fast`                    | Abort the task immediately when approval is required |

:::warning Away mode caution
In away mode, all approval-required operations are automatically denied. Make sure your task prompt does not depend on shell execution or file writes, or explicitly allow the needed commands via `exec_allowlist`.
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
BYTEMIND_SANDBOX_ENABLED=true BYTEMIND_WRITABLE_ROOTS=./src bytemind chat
```

## Iteration Budget

`max_iterations` caps the number of tool-call rounds per task, preventing runaway loops:

```json
{
  "max_iterations": 64
}
```

When the limit is reached, the agent produces a progress summary and stops gracefully — it does not error out. Raise this value for complex refactors or large migrations.

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
  "max_iterations": 32,
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
