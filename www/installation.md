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

After the script finishes it prints the install path. If `bytemind` is not found, see [Configuring PATH](#configuring-path) below.

## Install a Specific Version

Pin a version in production environments to avoid unexpected behavior changes from updates.

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | BYTEMIND_VERSION=v0.3.0 bash
```

### Windows (PowerShell)

```powershell
$env:BYTEMIND_VERSION = 'v0.3.0'
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

:::tip Browse available versions
All releases and their changelogs are listed on the [GitHub Releases](https://github.com/1024XEngineer/bytemind/releases) page.
:::

## Configuring PATH

The install script places the binary at:

- **Linux / macOS**: `~/.local/bin/bytemind`
- **Windows**: `%USERPROFILE%\.local\bin\bytemind.exe`

If `bytemind --version` reports command not found, add the directory to your `PATH`:

```bash
# bash / zsh — add to ~/.bashrc or ~/.zshrc
export PATH="$HOME/.local/bin:$PATH"
```

```powershell
# PowerShell — permanent for current user
[Environment]::SetEnvironmentVariable("Path", "$env:USERPROFILE\.local\bin;" + $env:Path, "User")
```

Use `BYTEMIND_INSTALL_DIR` to install to a custom location:

```bash
BYTEMIND_INSTALL_DIR=/usr/local/bin curl -fsSL .../install.sh | bash
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
ByteMind v0.4.0 (go1.24.0 darwin/arm64)
```

## Updating

Re-run the install script to upgrade to the latest release:

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash
```

To suppress the update-check prompt, set in your config:

```json
{
  "update_check": { "enabled": false }
}
```

## Uninstalling

Remove the binary to uninstall:

```bash
rm ~/.local/bin/bytemind
```

Session data and config files live in `.bytemind/` and can be removed separately if desired.
