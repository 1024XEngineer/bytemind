# 配置

ByteMind 从以下路径按优先级加载配置，找到第一个存在的文件后停止：

1. `-config <path>` 命令行参数指定的路径
2. 当前工作目录的 `.bytemind/config.json`
3. 当前工作目录的 `config.json`
4. 用户主目录的 `~/.bytemind/config.json`

## OpenAI 兼容接口

适用于 OpenAI、DeepSeek、通义千问、Azure OpenAI 等兼容接口的服务：

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

通过环境变量传入 API Key（推荐，避免密钥写入文件）：

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-4o",
    "api_key_env": "OPENAI_API_KEY"
  }
}
```

```bash
export OPENAI_API_KEY="sk-..."
bytemind chat
```

## Anthropic

```json
{
  "provider": {
    "type": "anthropic",
    "base_url": "https://api.anthropic.com",
    "model": "claude-sonnet-4-20250514",
    "api_key": "YOUR_API_KEY",
    "anthropic_version": "2023-06-01"
  },
  "approval_policy": "on-request",
  "max_iterations": 32,
  "stream": true
}
```

## 自定义 / 本地模型

只要端点兼容 OpenAI `/v1/chat/completions` 接口，即可直接使用：

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "http://localhost:11434/v1",
    "model": "qwen2.5-coder:7b",
    "api_key": "ollama"
  }
}
```

:::tip 自动检测 Provider 类型
设置 `"auto_detect_type": true` 后，ByteMind 会根据 `base_url` 自动推断 Provider 类型，无需手动指定 `type` 字段。
:::

## 审批策略

`approval_policy` 控制高风险工具（写文件、执行 Shell 命令等）何时请求确认：

| 值                   | 行为                                   |
| -------------------- | -------------------------------------- |
| `on-request`（默认） | Agent 在执行每个高风险操作前等待你确认 |

`approval_mode` 控制整体审批行为：

| 值                    | 行为                                      |
| --------------------- | ----------------------------------------- |
| `interactive`（默认） | 交互式审批，每次操作弹出确认              |
| `away`                | 无人值守模式，根据 `away_policy` 自动处理 |

`away_policy`（仅在 `approval_mode: away` 时生效）：

| 值                           | 行为                         |
| ---------------------------- | ---------------------------- |
| `auto_deny_continue`（默认） | 自动拒绝高风险操作并继续执行 |
| `fail_fast`                  | 遇到需审批的操作立即终止任务 |

:::warning Away 模式注意事项
Away 模式下 Agent 将自动拒绝所有需要审批的操作。确保你的任务提示词不依赖 Shell 执行或文件写入，或在 `exec_allowlist` 中明确允许相关命令。
:::

## 沙箱

启用沙箱后，Shell 工具和文件工具将受到写目录限制：

```json
{
  "sandbox_enabled": true,
  "writable_roots": ["./src", "./tests"]
}
```

也可通过环境变量开启：

```bash
BYTEMIND_SANDBOX_ENABLED=true BYTEMIND_WRITABLE_ROOTS=./src bytemind chat
```

## 迭代预算

`max_iterations` 限制 Agent 在单次任务中可调用工具的最大轮次，防止无限循环：

```json
{
  "max_iterations": 64
}
```

到达上限后，Agent 会输出阶段性总结并停止，而不是直接报错退出。对于复杂重构或大型迁移任务，建议适当调高此值。

## Token 配额

`token_quota` 设置单任务的 token 消耗预警阈值（默认 300,000）：

```json
{
  "token_quota": 500000
}
```

## 完整示例

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-4o",
    "api_key_env": "OPENAI_API_KEY"
  },
  "approval_policy": "on-request",
  "approval_mode": "interactive",
  "max_iterations": 32,
  "stream": true,
  "update_check": { "enabled": true },
  "context_budget": {
    "warning_ratio": 0.85,
    "critical_ratio": 0.95,
    "max_reactive_retry": 1
  }
}
```

完整字段参考见[配置参考](/zh/reference/config-reference)。
