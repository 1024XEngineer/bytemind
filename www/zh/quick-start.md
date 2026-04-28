# 快速开始

本指南帮助你在约 5 分钟内完成安装并执行第一个 AI 编程任务。

## 前置条件

ByteMind 以预编译二进制分发，**无需预先安装 Go**。

| 项目     | 要求                             |
| -------- | -------------------------------- |
| 操作系统 | macOS、Linux 或 Windows          |
| API Key  | 任意 OpenAI 兼容服务或 Anthropic |
| 网络     | 能访问你的 LLM Provider 端点     |

## 第一步：安装

**macOS / Linux**

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh | bash
```

**Windows（PowerShell）**

```powershell
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

安装完成后验证：

```bash
bytemind --version
```

:::tip 安装目录
默认安装到 `~/.local/bin`（Linux/macOS）或 `%USERPROFILE%\.local\bin`（Windows）。如果提示找不到命令，请确认该目录已加入 `PATH`。
:::

## 第二步：创建配置

在你的项目根目录下创建 `.bytemind/config.json`：

```bash
mkdir -p .bytemind
```

以 OpenAI 兼容接口为例，写入以下内容：

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-4o",
    "api_key": "YOUR_API_KEY"
  },
  "approval_policy": "on-request",
  "max_iterations": 32,
  "stream": true
}
```

:::warning 安全提示
不要把含有真实 `api_key` 的配置文件提交到 Git。建议把 `.bytemind/` 加入 `.gitignore`，或者改用 `api_key_env` 字段读取环境变量。
:::

字段说明：

| 字段                   | 说明                                              | 默认值                      |
| ---------------------- | ------------------------------------------------- | --------------------------- |
| `provider.type`        | Provider 类型：`openai-compatible` 或 `anthropic` | `openai-compatible`         |
| `provider.base_url`    | API 端点                                          | `https://api.openai.com/v1` |
| `provider.model`       | 模型 ID                                           | `gpt-5.4-mini`              |
| `provider.api_key`     | API 密钥（明文）                                  | —                           |
| `provider.api_key_env` | 从环境变量读取 API 密钥的变量名                   | `BYTEMIND_API_KEY`          |
| `approval_policy`      | 工具执行审批策略                                  | `on-request`                |
| `max_iterations`       | 单任务最大工具调用轮次                            | `32`                        |
| `stream`               | 流式输出                                          | `true`                      |

完整配置字段见[配置参考](/zh/reference/config-reference)。

## 第三步：启动聊天模式

进入你的项目目录后运行：

```bash
bytemind chat
```

ByteMind 会读取当前目录的 `.bytemind/config.json`，初始化会话，然后进入交互模式。

:::info 会话自动保存
每次对话都会被持久化。下次运行 `bytemind chat` 时，可以用 `/sessions` 列出历史会话，用 `/resume <id>` 恢复。
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

:::tip 使用技能
以 `/` 开头的斜杠命令可以激活内置技能，为 Agent 注入专项工作指引。例如 `/bug-investigation` 会引导 Agent 按结构化流程排查 Bug。输入 `/help` 可查看所有可用命令。
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
- [配置详解](/zh/configuration) — Anthropic、自定义端点、沙箱
- [核心概念](/zh/core-concepts) — 模式、会话、审批策略
- [聊天模式](/zh/usage/chat-mode) — 最佳实践与工作流
