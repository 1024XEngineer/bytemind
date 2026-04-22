# Features

## Chat & Session Management

### Multi-Round Conversations

ByteMind maintains full conversation history within a session, enabling multi-step tasks that require context from earlier turns.

### Persistent Sessions

Every session is saved automatically. List your sessions and resume any of them:

```bash
# Inside the TUI, type:
/sessions          # list all sessions
/session           # show current session info
/quit              # exit cleanly
```

### Session Recovery

If your terminal closes unexpectedly, ByteMind recovers the last session automatically on next launch.

---

## Provider Support

ByteMind supports two provider modes:

### OpenAI-Compatible APIs

Works with any API that follows the OpenAI chat completions protocol — including hosted services and local model servers.

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4-mini",
    "api_key": "your-api-key"
  }
}
```

### Anthropic

Direct support for Anthropic's Claude models via the native Anthropic API.

```json
{
  "provider": {
    "type": "anthropic",
    "base_url": "https://api.anthropic.com",
    "model": "claude-sonnet-4-20250514",
    "api_key": "your-api-key",
    "anthropic_version": "2023-06-01"
  }
}
```

---

## Built-In Tools

ByteMind equips the model with a set of tools for working with your codebase:

| Tool | Description |
|------|-------------|
| `list_files` | List files and directories with optional glob filtering |
| `read_file` | Read the contents of any file in the workspace |
| `search_text` | Full-text search across the repository |
| `write_file` | Create or append to files |
| `replace_in_file` | Targeted string replacement within a file |
| `apply_patch` | Apply a unified diff patch |
| `run_shell` | Execute shell commands (requires approval) |
| `web_search` | Search the web for up-to-date information |
| `web_fetch` | Fetch and parse content from a URL |
| `update_plan` | Manage structured plans in Plan mode |

---

## Safety & Control

### Approval Gates

By default (`approval_policy: "on-request"`), any tool call that modifies files or executes shell commands prompts for your approval before proceeding. You can approve, deny, or deny and continue.

### Execution Budget

The `max_iterations` setting caps how many tool-call rounds the agent can take per task. When the budget is reached, ByteMind returns a partial summary instead of running indefinitely.

```bash
bytemind chat -max-iterations 64        # raise the budget for complex tasks
bytemind run -prompt "refactor X" -max-iterations 128
```

Default is `32`. Recommended range for large tasks: `64–128`.

### Duplicate Call Detection

ByteMind detects when the model is repeating the same tool call sequence and breaks the loop automatically.

---

## Work Modes

### Build Mode (Default)

The default mode for practical implementation tasks. The agent reads, modifies, and verifies files directly. Best for: writing code, refactoring, debugging, applying patches.

### Plan Mode

For complex tasks that benefit from structured planning before execution. The agent uses `update_plan` to maintain a step-by-step plan and revises it incrementally.

```bash
bytemind chat   # start in Build mode (default)
```

Switch modes during a session using the `/plan` and `/build` commands inside the TUI.

---

## Streaming Output

ByteMind streams the model's response token by token directly in the terminal. No waiting for a complete response — you see the agent reasoning in real time.

Disable streaming if your API endpoint does not support it:

```json
{ "stream": false }
```

---

## Token Tracking

ByteMind tracks token usage per session and across sessions:

- Real-time usage displayed in the TUI status bar.
- Usage data persisted to `.bytemind/token_usage.json` by default.
- Configurable alert threshold (default: 1,000,000 tokens).
- Automatic data retention (default: 30 days).

---

## Configuration Reference

All settings live in `.bytemind/config.json` (or `config.json` in your workspace root).

```json
{
  "provider": { ... },
  "approval_policy": "on-request",
  "max_iterations": 32,
  "stream": true,
  "update_check": {
    "enabled": true
  }
}
```

Full example: [`config.example.json`](https://github.com/1024XEngineer/bytemind/blob/main/config.example.json)
