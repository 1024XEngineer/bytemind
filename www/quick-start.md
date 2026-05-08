# Quick Start

This guide helps you install ByteMind, configure it, and run your first AI coding task in about 5 minutes.

## Prerequisites

ByteMind ships as a precompiled binary, so **you do not need to install Go first**.

| Requirement | Details |
| ----------- | ------- |
| OS | Windows, Linux, or MacOS |
| API Key | Any OpenAI-compatible service or Anthropic; if you do not have one, start with [Get API Key](/api-key) |
| Network | Access to your LLM provider endpoint |

## Step 1: Install

Select your current system, then copy the corresponding command.

<Tabs default-tab="PowerShell">
<Tab title="PowerShell">

```powershell
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

Defaults to `%USERPROFILE%\bin\bytemind.exe`.

:::warning Windows users: copy the PowerShell command
Do not run `curl ... install.sh | bash` in Windows PowerShell or CMD. That command starts WSL; if WSL itself is broken, you may see `ext4.vhdx` or `HCS` errors.
:::

</Tab>

<Tab title="Linux">

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash
```

Defaults to `~/bin/bytemind`.

</Tab>

<Tab title="MacOS">

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash
```

Defaults to `~/bin/bytemind`.

</Tab>
</Tabs>

Verify after installation:

```bash
bytemind --version
```

If the command is not found, or an update still shows an older version, make sure the install directory is on your `PATH` and appears before older copies. On Windows, use `Get-Command bytemind -All` to see which binary PowerShell resolves.

## Step 2: Create a Global API Config

Start with a global config at `~/.bytemind/config.json`. You only need to configure it once, and ByteMind can read it from any project directory. `~/bin` or `%USERPROFILE%\bin` is only the install directory; it is not your project directory, and the config does not belong there.

<Tabs default-tab="PowerShell">
<Tab title="PowerShell">

```powershell
New-Item -ItemType Directory -Force "$env:USERPROFILE\.bytemind" | Out-Null
@'
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-4o",
    "api_key": "YOUR_API_KEY"
  }
}
'@ | Set-Content -Encoding utf8 "$env:USERPROFILE\.bytemind\config.json"
```

</Tab>

<Tab title="Linux">

```bash
mkdir -p ~/.bytemind
cat > ~/.bytemind/config.json <<'JSON'
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-4o",
    "api_key": "YOUR_API_KEY"
  }
}
JSON
```

</Tab>

<Tab title="MacOS">

```bash
mkdir -p ~/.bytemind
cat > ~/.bytemind/config.json <<'JSON'
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-4o",
    "api_key": "YOUR_API_KEY"
  }
}
JSON
```

</Tab>
</Tabs>

Replace `YOUR_API_KEY` with your real API key.

Field overview:

| Field | Description | Example |
| ----- | ----------- | ------- |
| `provider.type` | Provider type: `openai-compatible` or `anthropic` | `openai-compatible` `anthropic` |
| `provider.base_url` | API endpoint | `https://api.openai.com/v1` |
| `provider.model` | Model ID | `gpt-5.4-mini` |
| `provider.api_key` | API key in plain text | `sk-xxxxxxxxxxxxxxxxxx` |

If a project needs a different model or sandbox setting, create an additional `.bytemind/config.json` inside that project; project config overrides matching fields from the global config. See [Config Reference](/reference/config-reference) for the full field list.

## Step 3: Enter a Project Directory and Start ByteMind

Enter the specific code project directory you want ByteMind to work on, then run:

<Tabs default-tab="PowerShell">
<Tab title="PowerShell">

```powershell
Set-Location D:\code\your-project
bytemind
```

</Tab>

<Tab title="Linux">

```bash
cd /path/to/your-project
bytemind
```

</Tab>

<Tab title="MacOS">

```bash
cd /path/to/your-project
bytemind
```

</Tab>
</Tabs>

`bytemind` starts the default interactive interface. `bytemind chat` still works as a compatibility alias, but daily use can simply run `bytemind`.

:::warning Choose a specific project directory
ByteMind treats the current directory as the workspace. Do not start it directly from your home directory, a drive root, Downloads, Desktop, or a large folder with many unrelated files; enter a specific code repository or project subdirectory first. The install directory `%USERPROFILE%\bin` / `~/bin` only stores the binary and is not a workspace.
:::

After startup, ByteMind loads your global config plus any optional project override config, initializes a session, and enters interactive mode.

:::info Sessions are auto-saved
Every conversation is persisted. Next time you run `bytemind`, use `/sessions` to list previous sessions and `/resume <id>` to continue one.
:::

## Step 4: Run Your First Task

Try these starter prompts:

**Fix failing tests**

```text
Find all failing unit tests, analyze the root cause, and fix them with minimal changes.
```

**Understand the codebase structure**

```text
Walk me through this project's directory structure and main entry points. Produce a summary.
```

**Fix a bug**

```text
/bug-investigation symptom="login endpoint returns 500"
```

:::tip Slash commands and skills
`/` opens slash commands in the session; some commands are exposed by available skills. For example, `/bug-investigation` guides the agent through a structured bug investigation flow. Type `/help` to see all available commands.
:::

## Common Session Commands

| Command | Description |
| ------- | ----------- |
| `/help` | Show all available commands |
| `/session` | Show current session details |
| `/sessions` | List recent sessions |
| `/resume <id>` | Resume a specific session |
| `/new` | Start a new session |
| `/quit` | Exit |

## Next Steps

- [Installation](/installation) — version pinning, build from source
- [Get API Key](/api-key) — configure a model provider using DeepSeek as an example
- [Configuration](/configuration) — Anthropic, custom endpoints, sandbox
- [Core Concepts](/core-concepts) — modes, sessions, approval policy
- [Chat Mode](/usage/chat-mode) — best practices and workflows
