# Provider Setup

ByteMind supports any model provider that exposes an OpenAI-compatible API, plus Anthropic and Gemini native APIs.

## Multi-Provider Setup (Model Switching)

Configure multiple providers at once and switch between them at runtime with `/model`:

```json
{
  "provider_runtime": {
    "current_provider": "deepseek",
    "default_provider": "deepseek",
    "default_model": "deepseek-v4-flash",
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
      },
      "anthropic": {
        "type": "anthropic",
        "base_url": "https://api.anthropic.com",
        "api_key_env": "ANTHROPIC_API_KEY",
        "model": "claude-sonnet-4-20250514",
        "models": ["claude-sonnet-4-20250514", "claude-opus-4-20250514"]
      }
    }
  }
}
```

| Command | Action |
| ------- | ------ |
| `/model` | Interactive picker with all configured models |
| `/model openai/gpt-5.4` | Switch to GPT-5.4 |
| `/models` | Show current active model and all discovered models |

The config file is updated automatically after switching. See [Config Reference](/reference/config-reference#provider-runtime-multi-provider) for every field.

## Single Provider Examples (Legacy)

## OpenAI

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

## Anthropic

```json
{
  "provider": {
    "type": "anthropic",
    "base_url": "https://api.anthropic.com",
    "model": "claude-sonnet-4-20250514",
    "api_key_env": "ANTHROPIC_API_KEY",
    "anthropic_version": "2023-06-01"
  }
}
```

## Gemini

```json
{
  "provider": {
    "type": "gemini",
    "base_url": "https://generativelanguage.googleapis.com/v1beta",
    "model": "gemini-2.5-flash",
    "api_key_env": "GEMINI_API_KEY"
  }
}
```

## DeepSeek

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.deepseek.com",
    "model": "deepseek-v4-flash",
    "api_key_env": "DEEPSEEK_API_KEY"
  }
}
```

## Local Models (Ollama)

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

:::tip Any OpenAI-compatible endpoint works
If a service accepts `POST /v1/chat/completions` with standard OpenAI request/response format, it works with ByteMind. This includes Azure OpenAI, Groq, Together AI, and most self-hosted inference servers.
:::

## Using Environment Variables for API Keys

Always prefer `api_key_env` over a literal `api_key` in config files. This keeps secrets out of your source tree:

```json
{ "provider": { "api_key_env": "MY_API_KEY_VAR" } }
```

Set the variable **before** starting ByteMind:

<Tabs default-tab="PowerShell">
<Tab title="PowerShell">

```powershell
# Temporary (current window only):
$env:MY_API_KEY_VAR = "sk-..."

# Permanent (survives reboots):
[Environment]::SetEnvironmentVariable("MY_API_KEY_VAR", "sk-...", "User")
# Restart terminal after this command.
```

</Tab>

<Tab title="Linux">

```bash
# Temporary (current window only):
export MY_API_KEY_VAR="sk-..."

# Permanent:
echo 'export MY_API_KEY_VAR="sk-..."' >> ~/.bashrc
```

</Tab>

<Tab title="macOS">

```bash
# Temporary (current window only):
export MY_API_KEY_VAR="sk-..."

# Permanent:
echo 'export MY_API_KEY_VAR="sk-..."' >> ~/.zshrc
```

</Tab>
</Tabs>

```bash
bytemind
```

:::warning `api_key` overrides `api_key_env`
If both `api_key` and `api_key_env` are set, `api_key` (plain text) takes priority. Remove `api_key` from your config to use the environment variable.
:::

## Custom Auth Headers

For providers that require non-standard authentication:

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://my-internal-gateway/v1",
    "model": "gpt-4o",
    "auth_header": "X-API-Token",
    "auth_scheme": "",
    "api_key_env": "GATEWAY_TOKEN"
  }
}
```

## Verifying the Setup

After creating your config, run:

```bash
bytemind
```

Type a simple task like `say hello` and verify the model responds. If it fails, check:

- `base_url` is reachable from your machine
- `api_key` or the env var is set and valid
- `model` ID matches what the provider offers

See [Troubleshooting](/troubleshooting) for common auth error solutions.

## See Also

- [Configuration](/configuration) — full config reference
- [Environment Variables](/reference/env-vars) — runtime overrides
- [Troubleshooting](/troubleshooting) — auth failures and connectivity issues
