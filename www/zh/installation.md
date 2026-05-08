# 安装

本页聚焦安装 ByteMind 本身。安装完成后，如需配置 API Key 和启动第一个任务，请继续阅读[快速开始](/zh/quick-start)。

## 系统要求

| 项目     | 要求                          |
| -------- | ----------------------------- |
| 操作系统 | Windows 10+、Linux、MacOS 12+ |
| 架构     | amd64、arm64                  |
| Linux    | glibc 2.17+                   |
| 磁盘空间 | < 20 MB                       |

安装脚本会自动检测平台并下载对应二进制，**无需预先安装 Go**。

## 一键安装（推荐）

选择当前系统，复制对应命令运行即可。安装完成后，脚本会输出实际安装路径。

<Tabs default-tab="PowerShell">
<Tab title="PowerShell">

```powershell
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

默认安装到 `%USERPROFILE%\bin\bytemind.exe`。

:::warning Windows 用户请使用 PowerShell 命令
不要在 Windows PowerShell 或 CMD 中运行 `curl ... install.sh | bash`。如果误运行后看到 WSL 相关错误，请参考[故障排查](/zh/troubleshooting#windows-运行-bash-安装命令时报-wsl-错误)。
:::

</Tab>

<Tab title="Linux">

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash
```

默认安装到 `~/bin/bytemind`。

</Tab>

<Tab title="MacOS">

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash
```

默认安装到 `~/bin/bytemind`。

</Tab>
</Tabs>

## 验证安装

```bash
bytemind --version
```

输出示例：

```text
vX.Y.Z
```

如果终端提示找不到 `bytemind` 命令，请参考[命令未找到](/zh/troubleshooting#bytemind-未找到命令)。如果 Windows 更新后仍显示旧版本，请参考[Windows 更新后仍显示旧版本](/zh/troubleshooting#windows-更新后仍显示旧版本)。

## 更新

重新执行安装脚本即可覆盖更新到最新版本。

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

如需禁用更新检查提示，请在[配置参考](/zh/reference/config-reference)中查看 `update_check`。

## 卸载

删除对应的二进制文件即可完成卸载。会话记录和配置会保留在 `.bytemind` 目录中，可按需单独删除。

<Tabs default-tab="PowerShell">
<Tab title="PowerShell">

```powershell
Remove-Item "$env:USERPROFILE\bin\bytemind.exe"
```

会话记录和配置默认保存在 `%USERPROFILE%\.bytemind`。

</Tab>

<Tab title="Linux">

```bash
rm ~/bin/bytemind
```

会话记录和配置默认保存在 `~/.bytemind/`。

</Tab>

<Tab title="MacOS">

```bash
rm ~/bin/bytemind
```

会话记录和配置默认保存在 `~/.bytemind/`。

</Tab>
</Tabs>

如果你曾经自定义安装目录，或不确定当前运行的是哪一个二进制，请先参考[故障排查](/zh/troubleshooting#windows-卸载时报路径不存在)确认实际路径。

## 进阶安装

只有在需要固定版本、改变安装目录或从源码构建时，才需要阅读本节。

### 指定版本

生产环境建议固定版本，避免自动更新带来的行为变化。将 `vX.Y.Z` 替换为 [GitHub Releases](https://github.com/1024XEngineer/bytemind/releases) 页面中的发布标签。

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

### 自定义安装目录

通过 `BYTEMIND_INSTALL_DIR` 环境变量指定安装目录。

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

### 源码构建

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
