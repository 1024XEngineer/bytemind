# 快速开始

本指南帮助你在约 5 分钟内完成安装、配置，并执行第一个 AI 编程任务。

## 前置条件

ByteMind 以预编译二进制分发，**无需预先安装 Go**。

| 项目     | 要求                             |
| -------- | -------------------------------- |
| 操作系统 | Windows、Linux 或 MacOS          |
| API Key  | 任意 OpenAI 兼容服务或 Anthropic；不会获取可先看[获取 API Key](/zh/api-key) |
| 网络     | 能访问你的 LLM Provider 端点     |

## 第一步：安装

先选择当前系统，再复制对应命令。

<Tabs default-tab="PowerShell">
<Tab title="PowerShell">

```powershell
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

默认安装到 `%USERPROFILE%\bin\bytemind.exe`。

:::warning Windows 用户请复制 PowerShell 命令
不要在 Windows PowerShell 或 CMD 中运行 `curl ... install.sh | bash`。那条命令会启动 WSL；如果 WSL 本身损坏，会出现 `ext4.vhdx` 或 `HCS` 错误。
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

安装完成后验证：

```bash
bytemind --version
```

如果提示找不到命令，或更新后仍显示旧版本，请确认安装目录已加入 `PATH` 且排在旧副本之前。Windows 中可用 `Get-Command bytemind -All` 查看实际命中的二进制。

## 第二步：创建全局配置API

推荐先在用户目录创建全局配置：`~/.bytemind/config.json`。这样只需配置一次，之后在任意项目目录运行 ByteMind 都会读取同一份配置。`~/bin` 或 `%USERPROFILE%\bin` 只是安装目录，不是项目目录，也不需要把配置放在那里。

<Tabs default-tab="PowerShell">
<Tab title="PowerShell">

```powershell
New-Item -ItemType Directory -Force "$env:USERPROFILE\.bytemind" | Out-Null
@'
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-4o",
    "api_key": "YOUR_API_KEY"
  }
}
'@ | Set-Content -Encoding utf8 "$env:USERPROFILE\.bytemind\config.json"
```

</Tab>

<Tab title="Linux">

```bash
mkdir -p ~/.bytemind
cat > ~/.bytemind/config.json <<'JSON'
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-4o",
    "api_key": "YOUR_API_KEY"
  }
}
JSON
```

</Tab>

<Tab title="MacOS">

```bash
mkdir -p ~/.bytemind
cat > ~/.bytemind/config.json <<'JSON'
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-4o",
    "api_key": "YOUR_API_KEY"
  }
}
JSON
```

</Tab>
</Tabs>

将其中的 `YOUR_API_KEY` 替换为你的真实 API Key。

字段说明：

| 字段                   | 说明                                              | 用例                      |
| ---------------------- | ------------------------------------------------- | --------------------------- |
| `provider.type`        | Provider 类型：`openai-compatible` 或 `anthropic` | `openai-compatible` `anthropic`        |
| `provider.base_url`    | API 端点                                          | `https://api.openai.com/v1` |
| `provider.model`       | 模型 ID                                           | `gpt-5.4-mini`              |
| `provider.api_key`     | API 密钥（明文）                                  | `sk-xxxxxxxxxxxxxxxxxx`   |

如果某个项目需要不同模型或沙箱设置，可以在该项目目录下额外创建 `.bytemind/config.json`；项目配置会覆盖全局配置中的同名字段。完整配置字段见[配置参考](/zh/reference/config-reference)。

## 第三步：进入项目目录并启动 ByteMind

进入你要处理的具体代码项目目录后运行：

<Tabs default-tab="PowerShell">
<Tab title="PowerShell">

```powershell
Set-Location D:\code\your-project
bytemind
```

</Tab>

<Tab title="Linux">

```bash
cd /path/to/your-project
bytemind
```

</Tab>

<Tab title="MacOS">

```bash
cd /path/to/your-project
bytemind
```

</Tab>
</Tabs>

`bytemind` 会启动默认交互界面。`bytemind chat` 仍可作为兼容别名使用，但日常使用直接运行 `bytemind` 即可。

:::warning 选择具体项目目录
ByteMind 会把当前目录作为工作区。不要直接在用户主目录、磁盘根目录、Downloads、Desktop 或包含大量无关文件的大文件夹中启动；请先进入具体代码仓库或项目子目录。安装目录 `%USERPROFILE%\bin` / `~/bin` 只放二进制文件，也不是工作区。
:::

启动后，ByteMind 会读取全局配置和当前项目的可选覆盖配置，初始化会话，然后进入交互模式。

:::info 会话自动保存
每次对话都会被持久化。下次运行 `bytemind` 时，可以用 `/sessions` 列出历史会话，用 `/resume <id>` 恢复。
:::

## 第四步：执行第一个任务

试试这几个入门提示词：

**修复失败测试**

```text
定位所有失败的单元测试，分析根因，以最小改动完成修复。
```

**理解代码库结构**

```text
帮我梳理这个项目的目录结构和主要入口，输出一份概览说明。
```

**修复一个 Bug**

```text
/bug-investigation symptom="登录接口返回 500"
```

:::tip 斜杠命令与技能
`/` 是会话中的斜杠命令入口；其中一部分命令来自可用技能的自动识别和暴露。例如 `/bug-investigation` 会引导 Agent 按结构化流程排查 Bug。输入 `/help` 可查看所有可用命令。
:::

## 常用会话命令

| 命令           | 说明             |
| -------------- | ---------------- |
| `/help`        | 查看所有可用命令 |
| `/session`     | 查看当前会话详情 |
| `/sessions`    | 列出最近会话     |
| `/resume <id>` | 恢复指定会话     |
| `/new`         | 开启新会话       |
| `/quit`        | 退出             |

## 下一步

- [安装详解](/zh/installation) — 版本固定、从源码构建
- [获取 API Key](/zh/api-key) — 以 DeepSeek 为例完成模型服务配置
- [配置详解](/zh/configuration) — Anthropic、自定义端点、沙箱
- [核心概念](/zh/core-concepts) — 模式、会话、审批策略
- [聊天模式](/zh/usage/chat-mode) — 最佳实践与工作流
