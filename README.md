<p align="center">
  <img alt="ByteMind Banner" src="https://capsule-render.vercel.app/api?type=waving&height=190&color=0:0ea5e9,100:2563eb&text=ByteMind&fontColor=ffffff&fontSize=52&animation=fadeIn" />
</p>

<h1 align="center">ByteMind</h1>

<p align="center">
  <a href="https://github.com/1024XEngineer/bytemind/stargazers"><img alt="GitHub Stars" src="https://img.shields.io/github/stars/1024XEngineer/bytemind?style=for-the-badge&logo=github&color=f59e0b" /></a>
  <a href="https://github.com/1024XEngineer/bytemind/network/members"><img alt="GitHub Forks" src="https://img.shields.io/github/forks/1024XEngineer/bytemind?style=for-the-badge&logo=github&color=06b6d4" /></a>
  <a href="https://github.com/1024XEngineer/bytemind/blob/main/LICENSE"><img alt="License" src="https://img.shields.io/badge/License-MIT-22c55e?style=for-the-badge" /></a>
  <img alt="Go Version" src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=for-the-badge&logo=go&logoColor=white" />
  <a href="https://github.com/1024XEngineer/bytemind/commits/main"><img alt="Last Commit" src="https://img.shields.io/github/last-commit/1024XEngineer/bytemind/main?style=for-the-badge&color=8b5cf6" /></a>
</p>

<p align="center">
  一个用 Go 实现的 AI Coding CLI，目标是提供更接近 OpenCode / ClaudeCode 的工作流能力。
</p>

<p align="center">
  <a href="#core-features">核心能力</a> •
  <a href="#quick-start">快速开始</a> •
  <a href="#configuration">配置文件</a> •
  <a href="#project-structure">目录结构</a>
</p>

> [!NOTE]
> 当前版本已具备多轮会话、流式输出、工具调用循环、Shell 执行审批、执行预算控制与重复调用检测等核心能力。

> [!TIP]
> 长任务建议提高 `-max-iterations`，到达预算后会返回阶段性总结，不会直接失败退出。

<a id="core-features"></a>

## 🎯 核心能力

| 模块 | 说明 | 状态 |
| --- | --- | --- |
| 会话系统 | 多轮会话 + 会话持久化 | ![status](https://img.shields.io/badge/status-ready-22c55e?style=flat-square) |
| 对话交互 | 纯 CLI 聊天 + 流式终端输出 | ![status](https://img.shields.io/badge/status-ready-22c55e?style=flat-square) |
| Provider 适配 | OpenAI-compatible / Anthropic 双适配层 | ![status](https://img.shields.io/badge/status-ready-22c55e?style=flat-square) |
| 工具执行 | 文件读写搜索、补丁替换、命令执行审批 | ![status](https://img.shields.io/badge/status-ready-22c55e?style=flat-square) |
| 运行稳定性 | `max_iterations` 预算控制 + 重复调用检测 | ![status](https://img.shields.io/badge/status-ready-22c55e?style=flat-square) |

<a id="quick-start"></a>

## 🚀 快速开始

### 0) 一键安装（无需 Go）

macOS / Linux：

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash
```

Windows PowerShell：

```powershell
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

安装指定版本（示例 `v0.3.0`）：

```bash
BYTEMIND_VERSION=v0.3.0 curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash
```

```powershell
$env:BYTEMIND_VERSION='v0.3.0'; iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

更多安装方式见：[docs/installation.md](docs/installation.md)

### 1) 准备配置

先复制示例配置，再把 `api_key` 等字段改成你自己的值：

```powershell
New-Item -ItemType Directory -Force .bytemind | Out-Null
Copy-Item config.example.json .bytemind/config.json
```

> 兼容说明：工作区 `config.json` 也会被识别；这里推荐 `.bytemind/config.json` 方便与源码分离。

### 2) 运行 ByteMind

聊天模式：

```powershell
go run ./cmd/bytemind chat
```

单次任务：

```powershell
go run ./cmd/bytemind run -prompt "分析当前项目并生成改进建议"
```

提高执行预算：

```powershell
go run ./cmd/bytemind chat -max-iterations 64
go run ./cmd/bytemind run -prompt "refactor this module" -max-iterations 64
```

<a id="configuration"></a>

## ⚙️ 配置文件

默认配置（OpenAI-compatible）：

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4-mini",
    "api_key": "your-api-key-here"
  },
  "approval_policy": "on-request",
  "max_iterations": 32,
  "stream": true
}
```

<details>
<summary>Anthropic 配置示例</summary>

```json
{
  "provider": {
    "type": "anthropic",
    "base_url": "https://api.anthropic.com",
    "model": "claude-sonnet-4-20250514",
    "api_key": "your-api-key-here",
    "anthropic_version": "2023-06-01"
  }
}
```

</details>

<a id="project-structure"></a>

## 🧱 目录结构

```text
cmd/bytemind            CLI 入口
internal/agent          对话循环、系统提示词模板、流式输出
internal/config         配置加载与环境变量覆盖
internal/llm            通用消息与工具类型
internal/provider       多 provider 适配层
internal/session        会话持久化
internal/tools          文件工具、patch 工具、shell 工具
```

## 🧭 交互命令

- `/help`
- `/session`
- `/sessions`
- `/quit`

## 🧰 已实现工具

- `list_files`
- `read_file`
- `search_text`
- `write_file`
- `replace_in_file`
- `apply_patch`
- `run_shell`

## 📝 系统提示词维护

系统提示词模板在：

- `internal/agent/prompts/system.md`

运行时由 `internal/agent/prompt.go` 通过 `go:embed` 内嵌 Markdown，并替换 `{{WORKSPACE}}`、`{{APPROVAL_POLICY}}` 占位符。

## 🌍 Environment Variables

See [docs/environment-variables.md](docs/environment-variables.md) for runtime TUI env vars:

- `BYTEMIND_ENABLE_MOUSE`
- `BYTEMIND_WINDOWS_INPUT_TTY`
- `BYTEMIND_MOUSE_Y_OFFSET`

## 📄 License

MIT License
