# Configuration

ByteMind reads config from `.bytemind/config.json` (or `config.json` in project root).

## Minimal OpenAI-Compatible Example

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4-mini",
    "api_key": "YOUR_API_KEY"
  },
  "approval_policy": "on-request",
  "max_iterations": 32,
  "stream": true
}
```

## Minimal Anthropic Example

```json
{
  "provider": {
    "type": "anthropic",
    "base_url": "https://api.anthropic.com",
    "model": "claude-sonnet-4-20250514",
    "api_key": "YOUR_API_KEY",
    "anthropic_version": "2023-06-01"
  }
}
```

## Recommended Defaults

- Keep `approval_policy` as `on-request` for daily use.
- Increase `max_iterations` for complex tasks.
- Keep `stream` enabled for real-time progress.

See [Config Reference](/en/reference/config-reference) for full options.
