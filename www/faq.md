# FAQ

## Installation

### Do I need Go installed to use ByteMind?

No. ByteMind ships as a pre-compiled binary for macOS, Linux, and Windows. No Go installation is required.

### The `bytemind` command is not found after installing. What do I do?

The install script puts the binary in `~/bin`. Make sure that directory is on your `PATH`:

```bash
export PATH="$HOME/bin:$PATH"
```

Add that line to your `~/.bashrc` or `~/.zshrc` to make it permanent.

### How do I install a specific version?

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh \
  | BYTEMIND_VERSION=vX.Y.Z bash
```

Windows PowerShell:

```powershell
$env:BYTEMIND_VERSION = 'vX.Y.Z'
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

Replace `vX.Y.Z` with the release tag you want to install.

### Why does the old version still run after updating?

PowerShell may be finding an older `bytemind.exe` earlier in `PATH`. Run:

```powershell
Get-Command bytemind -All | Select-Object Source
& "$env:USERPROFILE\bin\bytemind.exe" --version
```

If the second command prints the new version, move `%USERPROFILE%\bin` to the front of your user `PATH`; see [Troubleshooting](/troubleshooting#windows-still-shows-the-old-version-after-updating).

### Why do I get a WSL error when running the bash install command on Windows?

`install.sh | bash` is for macOS, Linux, or a working WSL shell. In Windows PowerShell, use:

```powershell
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

## Providers

### Can I use a local model?

Yes, as long as the local server exposes an OpenAI-compatible API (e.g. Ollama with its `/v1/chat/completions` endpoint). Set `provider.type` to `openai-compatible` and point `base_url` at `http://localhost:11434/v1`.

### Can I use DeepSeek, Groq, or other third-party providers?

Yes. Any provider that implements the OpenAI chat completions interface works. Configure `base_url` to point to the provider's endpoint.

### Do I need to set `provider.type` explicitly?

No. Set `"auto_detect_type": true` and ByteMind will infer the type from `base_url`.

## Privacy and Security

### Is my code uploaded or stored anywhere?

No. ByteMind runs entirely on your local machine. It reads your local files and sends only what you explicitly include in prompts to the LLM API you configure. No data is sent to ByteMind servers.

### How do I keep my API key secure?

Use `api_key_env` instead of `api_key` in your config file. Store the actual key in an environment variable and add `.bytemind/` to your `.gitignore`.

## Usage

### The agent ran out of iterations before finishing. What do I do?

Raise `max_iterations` in your config or use the `-max-iterations` flag:

```bash
bytemind -max-iterations 64
```

### Can I use ByteMind in CI without manual approvals?

Yes. Set `approval_mode` to `full_access`. Approval-required operations are auto-approved without blocking prompts, so CI pipelines can run unattended. Legacy `away` is blocked by default unless you explicitly set `BYTEMIND_ALLOW_AWAY_FULL_ACCESS=true` for migration.

### How do I resume a previous session?

```text
/sessions
/resume <session-id>
```

### Can I create custom skills for my team?

Yes. Place a skill directory with `skill.json` and `SKILL.md` in `.bytemind/skills/` inside your repository. Everyone on the team will have access to it. See [Skills](/usage/skills) for details.

## See Also

- [Troubleshooting](/troubleshooting) — specific error solutions
- [Installation](/installation) — detailed install steps
- [Configuration](/configuration) — full config guide
