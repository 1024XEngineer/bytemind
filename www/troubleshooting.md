# Troubleshooting

## `bytemind: command not found`

The binary is installed but not on your `PATH`.

**Fix:** Add the install directory to your `PATH`:

```bash
export PATH="$HOME/bin:$PATH"
```

Add this to `~/.bashrc`, `~/.zshrc`, or your shell profile to make it permanent. On Windows, use:

```powershell
$target = "$env:USERPROFILE\bin"
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (-not (($userPath -split ";") -contains $target)) {
  [Environment]::SetEnvironmentVariable("Path", ($target + ";" + $userPath), "User")
}
$env:Path = $target + ";" + $env:Path
```

## Windows Still Shows the Old Version After Updating

Symptom: the install script downloads the latest version, but `bytemind --version` still prints an older version.

**Fix:** Check which binary PowerShell is resolving:

```powershell
Get-Command bytemind -All | Select-Object Source
& "$env:USERPROFILE\bin\bytemind.exe" --version
```

If the second command prints the new version but the first `Get-Command` result is not `$env:USERPROFILE\bin\bytemind.exe`, move the new install directory to the front of your user `PATH`:

```powershell
$target = "$env:USERPROFILE\bin"
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
$parts = $userPath -split ";" | Where-Object { $_ -and ($_ -ine $target) }
[Environment]::SetEnvironmentVariable("Path", ($target + ";" + ($parts -join ";")), "User")
$env:Path = $target + ";" + $env:Path
bytemind --version
```

## Windows WSL Error When Running the Bash Install Command

Symptom: after running `curl ... install.sh | bash` in PowerShell or CMD, you see an `ext4.vhdx`, `HCS`, `Bash/Service/CreateInstance`, or WSL mount error.

**Fix:** This is a WSL environment error, not a ByteMind package error. In a Windows terminal, use the PowerShell install script:

```powershell
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

Only use `install.sh | bash` after you are inside a working WSL/Linux shell. WSL `~/bin/bytemind` and Windows `%USERPROFILE%\bin\bytemind.exe` are different files.

## Windows Uninstall Says the Path Does Not Exist

Symptom: in PowerShell, `rm ~/bin/bytemind` reports that `C:\Users\<you>\bin\bytemind` does not exist.

**Fix:** The Windows binary is named `bytemind.exe`; remove the file with the `.exe` suffix:

```powershell
Remove-Item "$env:USERPROFILE\bin\bytemind.exe"
```

If the command is running from another directory, check the actual path before deleting:

```powershell
Get-Command bytemind -All | Select-Object Source
Remove-Item "<path to bytemind.exe from the previous command>"
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
bytemind -max-iterations 64
```

Or set it permanently in your config:

```json
{ "max_iterations": 64 }
```

## Config File Not Loaded

Symptom: ByteMind behaves as if no config exists (uses defaults).

**Check the config load order:**

1. `~/.bytemind/config.json` in the home directory
2. `.bytemind/config.json` in the current workspace (optional project overrides)

New users should put common settings in the user config, not in `~/bin` or `%USERPROFILE%\bin`. Run `bytemind -v` to see which config file was loaded.

## Workspace Is Too Large or Too Broad

Symptom: when started from your home directory, a drive root, Downloads, Desktop, or a very large folder, ByteMind reports that the current directory is too broad or feels slow.

**Fix:** Change into a specific code repository or project subdirectory before starting:

```powershell
Set-Location D:\code\my-project
bytemind
```

You can also specify the workspace explicitly from anywhere:

```powershell
bytemind -workspace D:\code\my-project
```

Avoid using a large folder with many unrelated files as the workspace. The install directory `%USERPROFILE%\bin` / `~/bin` only stores the binary and is not a workspace.

## Session Not Found After Resume

Symptom: `/resume <id>` reports session not found.

**Check:**

- You are in the same working directory where the session was created
- The session exists in ByteMind's home directory
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
