# Installation

## System Requirements

| Requirement  | Details                                     |
| ------------ | ------------------------------------------- |
| OS           | macOS 12+, Linux (glibc 2.17+), Windows 10+ |
| Architecture | amd64, arm64                                |
| Disk space   | < 20 MB                                     |

The install script automatically detects your platform and downloads the correct binary — **no Go installation required**.

## One-Line Install (Recommended)

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash
```

### Windows (PowerShell)

```powershell
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

:::warning Windows users: copy the PowerShell command
Do not run `curl ... install.sh | bash` from Windows PowerShell or CMD. That command is for macOS, Linux, or a working WSL shell; from a Windows terminal it starts WSL. If you see an `ext4.vhdx`, `HCS`, or `Bash/Service/CreateInstance` error, re-run the PowerShell command above instead.
:::

After the script finishes it prints the install path. If `bytemind` is not found, see [Configuring PATH](#configuring-path) below.

## Install a Specific Version

Pin a version in production environments to avoid unexpected behavior changes from updates.

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | BYTEMIND_VERSION=v0.1.5 bash
```

### Windows (PowerShell)

```powershell
$env:BYTEMIND_VERSION = 'v0.1.5'
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

:::tip Browse available versions
All releases and their changelogs are listed on the [GitHub Releases](https://github.com/1024XEngineer/bytemind/releases) page.
:::

## Configuring PATH

The install script places the binary at:

- **Linux / macOS**: `~/bin/bytemind`
- **Windows**: `%USERPROFILE%\bin\bytemind.exe`

If `bytemind --version` reports command not found, add the directory to your `PATH`:

```bash
# bash / zsh — add to ~/.bashrc or ~/.zshrc
export PATH="$HOME/bin:$PATH"
```

```powershell
# PowerShell: update this terminal now and future terminals permanently
$target = "$env:USERPROFILE\bin"
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (-not (($userPath -split ";") -contains $target)) {
  [Environment]::SetEnvironmentVariable("Path", ($target + ";" + $userPath), "User")
}
$env:Path = $target + ";" + $env:Path
```

If `bytemind --version` still prints an older version after updating, check which binary PowerShell is resolving:

```powershell
Get-Command bytemind -All
& "$env:USERPROFILE\bin\bytemind.exe" --version
```

If the first `Get-Command` result is not `%USERPROFILE%\bin\bytemind.exe`, an older copy appears earlier in `PATH`; remove that copy or move `%USERPROFILE%\bin` before it.

Use `BYTEMIND_INSTALL_DIR` to install to a custom location:

### macOS / Linux

```bash
BYTEMIND_INSTALL_DIR=/usr/local/bin curl -fsSL .../install.sh | bash
```

### Windows (PowerShell)

```powershell
$env:BYTEMIND_INSTALL_DIR = "$env:LOCALAPPDATA\Programs\ByteMind"
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

## Windows: Version Still Looks Old After Updating

If the install script downloads a new version but `bytemind --version` still prints the old version, an older `bytemind.exe` is usually earlier in `PATH`. Run:

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

If it still prints the old version, close the terminal, open a new PowerShell window, and run:

```powershell
Get-Command bytemind -All | Select-Object Source
bytemind --version
```

## Build from Source

Requires Go 1.24 or later.

```bash
git clone https://github.com/1024XEngineer/bytemind.git
cd bytemind
go build -o bytemind ./cmd/bytemind
```

Run without installing:

```bash
go run ./cmd/bytemind chat
```

## Verify the Installation

```bash
bytemind --version
```

Example output:

```
v0.1.5
```

## Updating

Re-run the install script to upgrade to the latest release:

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash
```

### Windows (PowerShell)

```powershell
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

If you accidentally ran `curl ... install.sh | bash` in a Windows terminal and got a WSL error, you do not need to fix ByteMind; run the PowerShell command above. WSL and Windows are separate environments, so a WSL `~/bin/bytemind` install does not update Windows `%USERPROFILE%\bin\bytemind.exe`.

To suppress the update-check prompt, set in your config:

```json
{
  "update_check": { "enabled": false }
}
```

## Uninstalling

Remove the binary to uninstall:

### macOS / Linux

```bash
rm ~/bin/bytemind
```

### Windows (PowerShell)

```powershell
Remove-Item "$env:USERPROFILE\bin\bytemind.exe"
```

If you used a custom install directory, or you are not sure which binary is running, check the actual path before deleting:

```powershell
Get-Command bytemind -All | Select-Object Source
Remove-Item "<path to bytemind.exe from the previous command>"
```

Session data and config files live in `.bytemind/` and can be removed separately if desired. On Windows, the default location is `%USERPROFILE%\.bytemind`.
