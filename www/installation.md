# Installation

ByteMind can be installed without a local Go toolchain.

## One-Line Install

The fastest way to get started. The install script downloads a pre-built binary for your platform and places it in `~/.bytemind/bin`.

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash
```

### Windows (PowerShell)

```powershell
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

After installation, add `~/.bytemind/bin` to your `PATH` if it is not already there.

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

## Manual Installation

1. Download the archive for your OS and architecture from the [GitHub Releases page](https://github.com/1024XEngineer/bytemind/releases).
2. Verify the checksum against `checksums.txt`.
3. Extract the archive.
4. Run the installer:

```bash
./bytemind install
```

On Windows:

```powershell
.\bytemind.exe install
```

## Build from Source

Requires Go 1.24 or later.

```bash
git clone https://github.com/1024XEngineer/bytemind.git
cd bytemind
go run ./cmd/bytemind chat
```

## Environment Variables

The install scripts respect the following environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `BYTEMIND_VERSION` | latest | Release tag to install (e.g. `v0.3.0`) |
| `BYTEMIND_INSTALL_DIR` | `~/.bytemind/bin` | Target installation directory |
| `BYTEMIND_REPO` | `1024XEngineer/bytemind` | GitHub repository |

## First Run

Once installed, copy the example configuration and add your API key:

```bash
mkdir -p .bytemind
cp config.example.json .bytemind/config.json
# edit .bytemind/config.json and set your api_key
```

Then start a session:

```bash
bytemind chat
```

See [Features](./features) for all available commands and options.
