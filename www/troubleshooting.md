# Troubleshooting

## `bytemind: command not found`

The binary is installed but not on your `PATH`.

**Fix:** Add the install directory to your `PATH`:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Add this to `~/.bashrc`, `~/.zshrc`, or your shell profile to make it permanent. On Windows, use:

```powershell
[Environment]::SetEnvironmentVariable("Path", "$env:USERPROFILE\.local\bin;" + $env:Path, "User")
```

## Provider Authentication Failed

Symptom: `401 Unauthorized` or `authentication failed` in the output.

**Check:**

1. `provider.api_key` or the env var named in `provider.api_key_env` is set and correct
2. `provider.base_url` points to the right endpoint (no trailing slash, correct version path)
3. `provider.model` exists on your provider plan

```bash
# Quick test
curl -s -H "Authorization: Bearer $OPENAI_API_KEY" \
  https://api.openai.com/v1/models | head -c 200
```

If the curl returns models, your key is valid. If ByteMind still fails, verify the `base_url` in config exactly matches the working curl URL.

## Agent Stops Too Early

Symptom: The agent outputs a partial result and says it hit the iteration limit.

**Fix:** Raise `max_iterations`:

```bash
bytemind chat -max-iterations 64
```

Or set it permanently in your config:

```json
{ "max_iterations": 64 }
```

## Config File Not Loaded

Symptom: ByteMind behaves as if no config exists (uses defaults).

**Check the config search path:**

1. `.bytemind/config.json` in the current directory
2. `config.json` in the current directory
3. `~/.bytemind/config.json` in the home directory

Run `bytemind chat -v` to see which config file was loaded.

## Session Not Found After Resume

Symptom: `/resume <id>` reports session not found.

**Check:**

- You are in the same working directory where the session was created
- Session files exist in `.bytemind/sessions/`
- `BYTEMIND_HOME` env var is not pointing to a different directory

## Sandbox Blocks Writes

Symptom: The agent fails to write a file with a permission error even though the path looks valid.

**Fix:** Add the path to `writable_roots` in your config:

```json
{
  "sandbox_enabled": true,
  "writable_roots": ["./src", "./docs"]
}
```

Or disable sandbox for local development:

```json
{ "sandbox_enabled": false }
```

## Streaming Output is Garbled

Symptom: Output looks corrupted or shows raw escape codes.

**Fix:** Disable streaming:

```json
{ "stream": false }
```

This is more common in non-TTY environments (e.g. piped output, certain CI runners).

## Context Window Exceeded

Symptom: The agent warns about context usage and stops mid-task.

**Options:**

- Start a fresh session (`/new`) for long conversations
- Break the task into smaller pieces
- Adjust `context_budget.warning_ratio` and `context_budget.critical_ratio` thresholds

## See Also

- [FAQ](/faq) — common questions and answers
- [Configuration](/configuration) — config options for tuning behavior
- [Installation](/installation) — PATH and version pinning
