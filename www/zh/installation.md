# 安装

ByteMind 无需本地 Go 工具链即可安装。

## 一键安装

最快速的入门方式。安装脚本会为你的平台下载预编译二进制文件并放置到 `~/.bytemind/bin`。

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash
```

### Windows（PowerShell）

```powershell
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

安装完成后，如尚未添加，请将 `~/.bytemind/bin` 加入你的 `PATH`。

## 安装指定版本

### macOS / Linux

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | BYTEMIND_VERSION=v0.3.0 bash
```

### Windows（PowerShell）

```powershell
$env:BYTEMIND_VERSION='v0.3.0'
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

## 手动安装

1. 从 [GitHub Releases 页面](https://github.com/1024XEngineer/bytemind/releases) 下载适合你操作系统和架构的压缩包。
2. 对照 `checksums.txt` 验证校验和。
3. 解压压缩包。
4. 运行安装程序：

```bash
./bytemind install
```

Windows：

```powershell
.\bytemind.exe install
```

## 从源码构建

需要 Go 1.24 或更高版本。

```bash
git clone https://github.com/1024XEngineer/bytemind.git
cd bytemind
go run ./cmd/bytemind chat
```

## 环境变量

安装脚本支持以下环境变量：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `BYTEMIND_VERSION` | 最新版本 | 要安装的 Release 标签（如 `v0.3.0`） |
| `BYTEMIND_INSTALL_DIR` | `~/.bytemind/bin` | 目标安装目录 |
| `BYTEMIND_REPO` | `1024XEngineer/bytemind` | GitHub 仓库地址 |

## 首次运行

安装完成后，复制示例配置并填入你的 API Key：

```bash
mkdir -p .bytemind
cp config.example.json .bytemind/config.json
# 编辑 .bytemind/config.json，填入你的 api_key
```

然后启动一个会话：

```bash
bytemind chat
```

查看[功能特性](./features)了解所有可用命令和选项。
