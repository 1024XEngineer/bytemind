# Installation

ByteMind can be installed without a local Go toolchain.

## One-line Install

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash
```

### Windows PowerShell

```powershell
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

## Install a Specific Version

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | BYTEMIND_VERSION=v0.3.0 bash
```

### Windows PowerShell

```powershell
$env:BYTEMIND_VERSION='v0.3.0'; iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

## Optional Environment Variables

- `BYTEMIND_REPO`: GitHub repository in the format `owner/repo` (default: `1024XEngineer/bytemind`).
- `BYTEMIND_VERSION`: Release tag to install (for example `v0.3.0`).
- `BYTEMIND_INSTALL_DIR`: Target install directory (default: `~/.bytemind/bin`).

## Manual Installation from Release Assets

1. Download the matching archive for your OS/architecture from the GitHub release page.
2. Verify `checksums.txt`.
3. Extract the archive.
4. Run:

```bash
./bytemind install
```

On Windows:

```powershell
.\bytemind.exe install
```
