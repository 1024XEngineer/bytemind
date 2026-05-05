# 安装

## 系统要求

| 项目     | 要求                                         |
| -------- | -------------------------------------------- |
| 操作系统 | macOS 12+、Linux（glibc 2.17+）、Windows 10+ |
| 架构     | amd64、arm64                                 |
| 磁盘空间 | < 20 MB                                      |

安装脚本会自动检测平台并下载对应二进制，**无需预先安装 Go**。

## 一键安装（推荐）

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash
```

### Windows（PowerShell）

```powershell
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

:::warning Windows 用户请复制 PowerShell 命令
在 Windows PowerShell 或 CMD 中不要运行 `curl ... install.sh | bash`。那条命令只适用于 macOS、Linux 或正常工作的 WSL；在 Windows 终端里运行会启动 WSL。若看到 `ext4.vhdx`、`HCS` 或 `Bash/Service/CreateInstance` 错误，请改用上面的 PowerShell 命令重新安装。
:::

安装完成后，脚本会输出安装路径。若终端提示找不到 `bytemind` 命令，请参考下方 [PATH 配置](#配置-path) 一节。

## 安装指定版本

生产环境建议固定版本，避免自动更新带来的行为变化。

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | BYTEMIND_VERSION=v0.1.5 bash
```

### Windows（PowerShell）

```powershell
$env:BYTEMIND_VERSION = 'v0.1.5'
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

:::tip 查看可用版本
在 [GitHub Releases](https://github.com/1024XEngineer/bytemind/releases) 页面可以浏览所有发布版本及 CHANGELOG。
:::

## 配置 PATH

安装脚本默认将二进制写入：

- **Linux / macOS**：`~/bin/bytemind`
- **Windows**：`%USERPROFILE%\bin\bytemind.exe`

如果 `bytemind --version` 提示找不到命令，将对应目录加入 `PATH`：

```bash
# bash / zsh（写入 ~/.bashrc 或 ~/.zshrc）
export PATH="$HOME/bin:$PATH"
```

```powershell
# PowerShell（当前终端立即生效，并对后续新终端永久生效）
$target = "$env:USERPROFILE\bin"
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (-not (($userPath -split ";") -contains $target)) {
  [Environment]::SetEnvironmentVariable("Path", ($target + ";" + $userPath), "User")
}
$env:Path = $target + ";" + $env:Path
```

如果更新后 `bytemind --version` 仍显示旧版本，先确认 PowerShell 实际命中的二进制：

```powershell
Get-Command bytemind -All
& "$env:USERPROFILE\bin\bytemind.exe" --version
```

若 `Get-Command` 第一条不是 `%USERPROFILE%\bin\bytemind.exe`，说明 PATH 中有更靠前的旧副本；删除旧副本，或把 `%USERPROFILE%\bin` 移到它前面。

也可通过 `BYTEMIND_INSTALL_DIR` 环境变量自定义安装目录：

### macOS / Linux

```bash
BYTEMIND_INSTALL_DIR=/usr/local/bin curl -fsSL .../install.sh | bash
```

### Windows（PowerShell）

```powershell
$env:BYTEMIND_INSTALL_DIR = "$env:LOCALAPPDATA\Programs\ByteMind"
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

## Windows：更新后仍显示旧版本

如果安装脚本显示下载了新版本，但 `bytemind --version` 仍输出旧版本，通常是 PATH 中有旧的 `bytemind.exe` 排在新安装目录前面。直接按下面步骤处理：

```powershell
Get-Command bytemind -All | Select-Object Source
& "$env:USERPROFILE\bin\bytemind.exe" --version
```

如果第二行输出的是新版本，而第一行的第一个路径不是 `$env:USERPROFILE\bin\bytemind.exe`，把新安装目录移动到用户 PATH 最前面：

```powershell
$target = "$env:USERPROFILE\bin"
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
$parts = $userPath -split ";" | Where-Object { $_ -and ($_ -ine $target) }
[Environment]::SetEnvironmentVariable("Path", ($target + ";" + ($parts -join ";")), "User")
$env:Path = $target + ";" + $env:Path
bytemind --version
```

若仍显示旧版本，关闭当前终端并打开一个新的 PowerShell 后再运行：

```powershell
Get-Command bytemind -All | Select-Object Source
bytemind --version
```

## 从源码构建

需要 Go 1.24 或更高版本。

```bash
git clone https://github.com/1024XEngineer/bytemind.git
cd bytemind
go build -o bytemind ./cmd/bytemind
```

直接运行而不安装：

```bash
go run ./cmd/bytemind
```

## 验证安装

```bash
bytemind --version
```

输出示例：

```
v0.1.5
```

## 更新

重新执行安装脚本即可覆盖更新到最新版本：

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash
```

### Windows（PowerShell）

```powershell
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

如果你在 Windows 终端中误运行了 `curl ... install.sh | bash` 并看到 WSL 错误，不需要修复 ByteMind；改运行上面的 PowerShell 命令即可。WSL 与 Windows 是两套环境，在 WSL 中安装的 `~/bin/bytemind` 不会更新 Windows 的 `%USERPROFILE%\bin\bytemind.exe`。

如需禁用更新检查提示，在配置文件中设置：

```json
{
  "update_check": { "enabled": false }
}
```

## 卸载

删除对应的二进制文件即可完成卸载：

### macOS / Linux

```bash
rm ~/bin/bytemind
```

### Windows（PowerShell）

```powershell
Remove-Item "$env:USERPROFILE\bin\bytemind.exe"
```

如果你曾经自定义安装目录，或不确定当前运行的是哪一个二进制，先查看实际路径再删除：

```powershell
Get-Command bytemind -All | Select-Object Source
Remove-Item "<上一步显示的 bytemind.exe 路径>"
```

会话记录和配置保存在 `.bytemind/` 目录中，需要时可一并删除。Windows 默认位置是 `%USERPROFILE%\.bytemind`。
