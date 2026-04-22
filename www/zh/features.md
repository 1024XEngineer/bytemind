# 功能特性

## 对话与会话管理

### 多轮对话

ByteMind 在会话中保留完整的对话历史，支持需要前序上下文的多步骤任务。

### 持久化会话

每个会话自动保存。在 TUI 中列出并恢复任意会话：

```bash
# 在 TUI 中输入：
/sessions          # 列出所有会话
/session           # 显示当前会话信息
/quit              # 正常退出
```

### 会话恢复

终端意外关闭？ByteMind 在下次启动时自动恢复上次会话。

---

## Provider 支持

ByteMind 支持两种 Provider 模式：

### OpenAI 兼容接口

兼容任何遵循 OpenAI chat completions 协议的 API，包括各类托管服务和本地模型服务器。

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4-mini",
    "api_key": "你的-api-key"
  }
}
```

### Anthropic

通过原生 Anthropic API 直接使用 Claude 系列模型。

```json
{
  "provider": {
    "type": "anthropic",
    "base_url": "https://api.anthropic.com",
    "model": "claude-sonnet-4-20250514",
    "api_key": "你的-api-key",
    "anthropic_version": "2023-06-01"
  }
}
```

---

## 内置工具

ByteMind 为模型配备了一套用于操作代码库的工具：

| 工具 | 说明 |
|------|------|
| `list_files` | 列出文件和目录，支持 glob 过滤 |
| `read_file` | 读取工作区中任意文件的内容 |
| `search_text` | 在仓库中执行全文搜索 |
| `write_file` | 创建或追加写入文件 |
| `replace_in_file` | 在文件中执行精确字符串替换 |
| `apply_patch` | 应用 unified diff 补丁 |
| `run_shell` | 执行 Shell 命令（需要审批） |
| `web_search` | 搜索网络获取最新信息 |
| `web_fetch` | 获取并解析 URL 页面内容 |
| `update_plan` | 在 Plan 模式中管理结构化计划 |

---

## 安全与控制

### 审批机制

默认模式（`approval_policy: "on-request"`）下，任何修改文件或执行 Shell 命令的工具调用都会在执行前提示你审批。你可以批准、拒绝，或拒绝后继续。

### 执行预算控制

`max_iterations` 设置限制了每个任务最多可执行的工具调用轮次。达到预算上限时，ByteMind 返回阶段性总结，而不是无限运行。

```bash
bytemind chat -max-iterations 64        # 为复杂任务提高预算
bytemind run -prompt "重构 X" -max-iterations 128
```

默认值为 `32`，大型任务推荐设置 `64–128`。

### 重复调用检测

ByteMind 能检测到模型在重复相同的工具调用序列并自动中断循环。

---

## 工作模式

### Build 模式（默认）

面向实际实现任务的默认模式。Agent 直接读取、修改并验证文件。适合：编写代码、重构、调试、应用补丁。

### Plan 模式

适合执行前需要结构化规划的复杂任务。Agent 使用 `update_plan` 工具维护分步计划，并逐步完善。

在 TUI 中使用 `/plan` 和 `/build` 命令切换模式。

---

## 流式输出

ByteMind 在终端中逐 Token 流式输出模型响应。无需等待完整答复 — 你可以实时看到 Agent 的推理过程。

如果你的 API 端点不支持流式，可以禁用：

```json
{ "stream": false }
```

---

## Token 用量追踪

ByteMind 追踪每个会话以及跨会话的 Token 用量：

- TUI 状态栏实时显示用量。
- 用量数据默认持久化至 `.bytemind/token_usage.json`。
- 可配置告警阈值（默认：1,000,000 tokens）。
- 自动数据保留（默认：30 天）。

---

## 配置参考

所有配置均存放在 `.bytemind/config.json`（或工作区根目录的 `config.json`）中。

```json
{
  "provider": { ... },
  "approval_policy": "on-request",
  "max_iterations": 32,
  "stream": true,
  "update_check": {
    "enabled": true
  }
}
```

完整示例：[`config.example.json`](https://github.com/1024XEngineer/bytemind/blob/main/config.example.json)
