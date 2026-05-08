# Installation

This page focuses on installing ByteMind itself. After installation, continue to [Quick Start](/quick-start) to configure an API key and run your first task.

## System Requirements

| Requirement | Details |
| ----------- | ------- |
| OS | Windows 10+, Linux, MacOS 12+ |
| Architecture | amd64, arm64 |
| Linux | glibc 2.17+ |
| Disk space | < 20 MB |

The install script automatically detects your platform and downloads the correct binary, so **you do not need to install Go first**.

## One-Line Install (Recommended)

Select your current system and copy the corresponding command. After the script finishes, it prints the actual install path.

<Tabs default-tab="PowerShell">
<Tab title="PowerShell">

```powershell
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

Defaults to `%USERPROFILE%\bin\bytemind.exe`.

:::warning Windows users: use the PowerShell command
Do not run `curl ... install.sh | bash` in Windows PowerShell or CMD. If you accidentally run it and see WSL-related errors, see [Troubleshooting](/troubleshooting#windows-wsl-error-when-running-the-bash-install-command).
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

## Verify the Installation

```bash
bytemind --version
```

Example output:

```text
vX.Y.Z
```

If the terminal says `bytemind` is not found, see [Command not found](/troubleshooting#bytemind-command-not-found). If Windows still shows the old version after updating, see [Windows still shows the old version after updating](/troubleshooting#windows-still-shows-the-old-version-after-updating).

## Updating

Re-run the install script to overwrite the existing binary with the latest release.

<Tabs default-tab="PowerShell">
<Tab title="PowerShell">

```powershell
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

</Tab>

<Tab title="Linux">

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash
```

</Tab>

<Tab title="MacOS">

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash
```

</Tab>
</Tabs>

To disable update-check prompts, see `update_check` in [Config Reference](/reference/config-reference).

## Uninstalling

Delete the corresponding binary to uninstall. Session data and config remain in the `.bytemind` directory and can be removed separately if desired.

<Tabs default-tab="PowerShell">
<Tab title="PowerShell">

```powershell
Remove-Item "$env:USERPROFILE\bin\bytemind.exe"
```

Session data and config are stored in `%USERPROFILE%\.bytemind` by default.

</Tab>

<Tab title="Linux">

```bash
rm ~/bin/bytemind
```

Session data and config are stored in `~/.bytemind/` by default.

</Tab>

<Tab title="MacOS">

```bash
rm ~/bin/bytemind
```

Session data and config are stored in `~/.bytemind/` by default.

</Tab>
</Tabs>

If you used a custom install directory, or you are not sure which binary is running, see [Troubleshooting](/troubleshooting#windows-uninstall-says-the-path-does-not-exist) to confirm the actual path first.

## Advanced Installation

Read this section only if you need to pin a version, change the install directory, or build from source.

### Specific Version

Pin a version in production environments to avoid unexpected behavior changes from updates. Replace `vX.Y.Z` with a release tag from the [GitHub Releases](https://github.com/1024XEngineer/bytemind/releases) page.

<Tabs default-tab="PowerShell">
<Tab title="PowerShell">

```powershell
$env:BYTEMIND_VERSION = 'vX.Y.Z'
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

</Tab>

<Tab title="Linux">

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | BYTEMIND_VERSION=vX.Y.Z bash
```

</Tab>

<Tab title="MacOS">

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | BYTEMIND_VERSION=vX.Y.Z bash
```

</Tab>
</Tabs>

### Custom Install Directory

Use the `BYTEMIND_INSTALL_DIR` environment variable to choose the install directory.

<Tabs default-tab="PowerShell">
<Tab title="PowerShell">

```powershell
$env:BYTEMIND_INSTALL_DIR = "$env:LOCALAPPDATA\Programs\ByteMind"
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

</Tab>

<Tab title="Linux">

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | BYTEMIND_INSTALL_DIR=/usr/local/bin bash
```

</Tab>

<Tab title="MacOS">

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | BYTEMIND_INSTALL_DIR=/usr/local/bin bash
```

</Tab>
</Tabs>

### Build from Source

Requires Go 1.24 or later.

```bash
git clone https://github.com/1024XEngineer/bytemind.git
cd bytemind
go build -o bytemind ./cmd/bytemind
```

Run without installing:

```bash
go run ./cmd/bytemind
```
