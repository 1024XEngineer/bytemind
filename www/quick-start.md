# Quick Start

This guide gets ByteMind installed and running your first AI coding task in about 5 minutes.

## Prerequisites

ByteMind ships as a pre-compiled binary — **no Go installation required**.

| Requirement | Details                                    |
| ----------- | ------------------------------------------ |
| OS          | macOS, Linux, or Windows                   |
| API Key     | Any OpenAI-compatible service or Anthropic |
| Network     | Access to your LLM provider endpoint       |

## Step 1: Install

**macOS / Linux**

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash
```

**Windows (PowerShell)**

```powershell
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

Verify the installation:

```bash
bytemind --version
```

:::tip Install Location
Defaults to `~/.local/bin` (Linux/macOS) or `%USERPROFILE%\.local\bin` (Windows). If the command is not found, make sure that directory is on your `PATH`.
:::

## Step 2: Create a Config

Create a `.bytemind/` directory inside your project:

```bash
mkdir -p .bytemind
```

Create `.bytemind/config.json` with your provider settings:

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

:::warning Keep secrets out of Git
Never commit a config file with a real `api_key`. Add `.bytemind/` to your `.gitignore`, or use the `api_key_env` field to read the key from an environment variable instead.
:::

Key fields at a glance:

| Field                  | Description                        | Default                     |
| ---------------------- | ---------------------------------- | --------------------------- |
| `provider.type`        | `openai-compatible` or `anthropic` | `openai-compatible`         |
| `provider.base_url`    | API endpoint URL                   | `https://api.openai.com/v1` |
| `provider.model`       | Model ID to use                    | `gpt-5.4-mini`              |
| `provider.api_key`     | API key (plain text)               | —                           |
| `provider.api_key_env` | Env var name to read the key from  | `BYTEMIND_API_KEY`          |
| `approval_policy`      | When to prompt for approval        | `on-request`                |
| `max_iterations`       | Max tool-call rounds per task      | `32`                        |
| `stream`               | Enable streaming output            | `true`                      |

See [Config Reference](/reference/config-reference) for the full field list.

## Step 3: Start Chat Mode

From your project directory, run:

```bash
bytemind chat
```

ByteMind reads `.bytemind/config.json` from the current directory, initializes a session, and enters interactive mode.

:::info Sessions are auto-saved
Every conversation is persisted. Next time you run `bytemind chat`, use `/sessions` to list previous sessions and `/resume <id>` to continue one.
:::

## Step 4: Run Your First Task

Try one of these starter prompts:

**Fix failing tests**

```text
Find all failing unit tests, analyze the root cause, and fix them with minimal changes.
```

**Understand the codebase**

```text
Walk me through this project's directory structure and main entry points. Produce a summary.
```

**Investigate a bug with a skill**

```text
/bug-investigation symptom="login endpoint returns 500"
```

:::tip Using Skills
Slash commands starting with `/` activate built-in skills that inject domain-specific instructions into the agent. For example, `/bug-investigation` guides the agent through a structured diagnosis workflow. Type `/help` to see all available commands.
:::

## Session Commands

| Command        | Description                      |
| -------------- | -------------------------------- |
| `/help`        | Show all available commands      |
| `/session`     | Show current session details     |
| `/sessions`    | List recent sessions             |
| `/resume <id>` | Resume a session by ID or prefix |
| `/new`         | Start a new session              |
| `/quit`        | Exit                             |

## Next Steps

- [Installation](/installation) — version pinning, build from source
- [Configuration](/configuration) — Anthropic, custom endpoints, sandbox
- [Core Concepts](/core-concepts) — modes, sessions, approval policy
- [Chat Mode](/usage/chat-mode) — best practices and workflows
