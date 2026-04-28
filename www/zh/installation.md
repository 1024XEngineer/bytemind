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

安装完成后，脚本会输出安装路径。若终端提示找不到 `bytemind` 命令，请参考下方 [PATH 配置](#配置-path) 一节。

## 安装指定版本

生产环境建议固定版本，避免自动更新带来的行为变化。

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | BYTEMIND_VERSION=v0.3.0 bash
```

### Windows（PowerShell）

```powershell
$env:BYTEMIND_VERSION = 'v0.3.0'
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

:::tip 查看可用版本
在 [GitHub Releases](https://github.com/1024XEngineer/bytemind/releases) 页面可以浏览所有发布版本及 CHANGELOG。
:::

## 配置 PATH

安装脚本默认将二进制写入：

- **Linux / macOS**：`~/.local/bin/bytemind`
- **Windows**：`%USERPROFILE%\.local\bin\bytemind.exe`

如果 `bytemind --version` 提示找不到命令，将对应目录加入 `PATH`：

```bash
# bash / zsh（写入 ~/.bashrc 或 ~/.zshrc）
export PATH="$HOME/.local/bin:$PATH"
```

```powershell
# PowerShell（永久生效）
[Environment]::SetEnvironmentVariable("Path", "$env:USERPROFILE\.local\bin;" + $env:Path, "User")
```

也可通过 `BYTEMIND_INSTALL_DIR` 环境变量自定义安装目录：

```bash
BYTEMIND_INSTALL_DIR=/usr/local/bin curl -fsSL .../install.sh | bash
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
go run ./cmd/bytemind chat
```

## 验证安装

```bash
bytemind --version
```

输出示例：

```
ByteMind v0.4.0 (go1.24.0 darwin/arm64)
```

## 更新

重新执行安装脚本即可覆盖更新到最新版本：

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash
```

如需禁用更新检查提示，在配置文件中设置：

```json
{
  "update_check": { "enabled": false }
}
```

## 卸载

删除对应的二进制文件即可完成卸载：

```bash
rm ~/.local/bin/bytemind
```

会话记录和配置保存在 `.bytemind/` 目录中，需要时可一并删除。
