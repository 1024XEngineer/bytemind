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

:::warning Copy the command for your current terminal
Windows PowerShell users should copy the PowerShell block, not the macOS / Linux `install.sh | bash` command. The bash command starts WSL; if WSL itself is broken, you may see `ext4.vhdx` or `HCS` errors.
:::

Verify the installation:

```powershell
bytemind --version
```

:::tip Install Location
Defaults to `~/bin` (Linux/macOS) or `%USERPROFILE%\bin` (Windows). If the command is not found, or an update still shows an older version, make sure that directory is on your `PATH` before older copies. In PowerShell, `Get-Command bytemind -All` shows the binary being resolved.
:::

## Step 2: Create a Global Config

Start with a global config at `~/.bytemind/config.json`. You only need to configure it once, and ByteMind can read it from any project directory. `~/bin` or `%USERPROFILE%\bin` is only the install directory; it is not your project directory, and the config does not belong there.

**macOS / Linux**

```bash
mkdir -p ~/.bytemind
cat > ~/.bytemind/config.json <<'JSON'
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
JSON
```

**Windows (PowerShell)**

```powershell
New-Item -ItemType Directory -Force "$env:USERPROFILE\.bytemind" | Out-Null
@'
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
'@ | Set-Content -Encoding utf8 "$env:USERPROFILE\.bytemind\config.json"
```

Replace `YOUR_API_KEY` with your real key.

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

If a specific project needs a different model or sandbox setting, create an additional `.bytemind/config.json` inside that project; project config overrides matching fields from the global config. See [Config Reference](/reference/config-reference) for the full field list.

## Step 3: Open a Project and Start ByteMind

Change into the specific code project you want ByteMind to work on, then run:

**macOS / Linux**

```bash
cd /path/to/your-project
bytemind
```

**Windows (PowerShell)**

```powershell
Set-Location D:\code\your-project
bytemind
```

`bytemind` opens the default interactive UI. `bytemind chat` still works as a compatibility alias, but daily use can start with `bytemind`.

:::warning Choose a specific project directory
ByteMind treats the current directory as the workspace. Do not start it directly from your home directory, a drive root, Downloads, Desktop, or a large folder with many unrelated files; enter a specific code repository or project subdirectory first. The install directory `%USERPROFILE%\bin` / `~/bin` only stores the binary and is not a workspace.
:::

ByteMind loads your global config plus any optional project override config, initializes a session, and enters interactive mode.

:::info Sessions are auto-saved
Every conversation is persisted. Next time you run `bytemind`, use `/sessions` to list previous sessions and `/resume <id>` to continue one.
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
