# Installation

## One-line Install

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash
```

### Windows (PowerShell)

```powershell
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

## Install a Specific Version

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | BYTEMIND_VERSION=v0.3.0 bash
```

### Windows (PowerShell)

```powershell
$env:BYTEMIND_VERSION='v0.3.0'
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

## Build from Source

```bash
git clone https://github.com/1024XEngineer/bytemind.git
cd bytemind
go run ./cmd/bytemind chat
```

## Verify Installation

```bash
bytemind --version
```
