# 配置

ByteMind 默认会先读取用户目录的全局配置，再读取当前工作区的项目配置；项目配置中的同名字段会覆盖全局配置。新用户推荐先创建全局配置：`~/.bytemind/config.json`。

自动加载顺序：

1. 用户目录的 `~/.bytemind/config.json`
2. 当前工作区的 `.bytemind/config.json`（可选，覆盖全局配置）

如果传入 `-config <path>`，则使用该文件作为本次运行配置。

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
  "max_iterations": 64,
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
bytemind
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
  "max_iterations": 64,
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

## 多 Provider 配置（模型切换）

ByteMind 支持配置多个模型 Provider 并在运行时切换，无需重启。使用 `provider_runtime` 替代旧版 `provider` 字段：

```json
{
  "provider_runtime": {
    "current_provider": "deepseek",
    "default_provider": "deepseek",
    "default_model": "deepseek-v4-flash",
    "allow_fallback": false,
    "providers": {
      "deepseek": {
        "type": "openai-compatible",
        "base_url": "https://api.deepseek.com",
        "api_key_env": "DEEPSEEK_API_KEY",
        "model": "deepseek-v4-flash",
        "models": ["deepseek-v4-flash", "deepseek-v4-pro"]
      },
      "openai": {
        "type": "openai-compatible",
        "base_url": "https://api.openai.com/v1",
        "api_key_env": "OPENAI_API_KEY",
        "model": "gpt-5.4-mini",
        "models": ["gpt-5.4-mini", "gpt-5.4"]
      }
    }
  }
}
```

### 关键点

- **`providers.<id>.models`** 必须填写 — 这是 `/model` 切换时可选的模型列表。
- **`providers.<id>.model`** 是该 Provider 当前使用的模型，切换后会自动更新。
- **`api_key_env`** 优于 `api_key` — 避免密钥明文写入配置文件。

### 切换模型

| 命令 | 作用 |
| ---- | ---- |
| `/model` | 打开交互式选择器，浏览所有已配置的 Provider 和模型 |
| `/model deepseek/deepseek-v4-pro` | 直接切换到 deepseek-v4-pro |
| `/model openai/gpt-5.4` | 直接切换到 GPT-5.4 |
| `/models` | 显示所有已发现的模型和当前使用的模型 |

切换后配置文件会自动更新 — 无需手动编辑。

### 添加新 Provider

编辑 `config.json`，在 `provider_runtime.providers` 下新增条目：

```json
"providers": {
  "deepseek": { ... },
  "openai": { ... },
  "anthropic": {
    "type": "anthropic",
    "base_url": "https://api.anthropic.com",
    "api_key_env": "ANTHROPIC_API_KEY",
    "model": "claude-sonnet-4-20250514",
    "models": ["claude-sonnet-4-20250514", "claude-opus-4-20250514"]
  }
}
```

重启 ByteMind 或使用 `/model` 即可在切换器中看到新增的 Provider。

### 为已有 Provider 添加新模型

编辑对应 Provider 的 `models` 数组：

```json
"deepseek": {
  "type": "openai-compatible",
  "base_url": "https://api.deepseek.com",
  "api_key_env": "DEEPSEEK_API_KEY",
  "model": "deepseek-v4-flash",
  "models": ["deepseek-v4-flash", "deepseek-v4-pro", "deepseek-v4-flash-2"]
}
```

保存后使用 `/model` 即可在切换器中看到新增的模型。

:::tip 从旧版 `provider` 迁移
如果配置文件中只有 `provider`（单 Provider），ByteMind 启动时会自动将其转换为 `provider_runtime`。使用 `/model` 切换后，选择结果会保存为 `provider_runtime` 格式。你也可以手动将配置改为上面的多 Provider 格式。
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
| `full_access`         | 全部权限模式，审批请求自动通过且不中断任务 |

兼容说明：为避免静默提权，`approval_mode: away` 默认被阻止。仅在迁移旧配置时，显式设置 `BYTEMIND_ALLOW_AWAY_FULL_ACCESS=true` 才会临时映射到 `full_access`。

`away_policy`（已弃用，仅兼容保留）：

| 值                           | 行为                         |
| ---------------------------- | ---------------------------- |
| `auto_deny_continue`（默认） | 仅用于兼容旧配置，不再影响运行时行为 |
| `fail_fast`                  | 仅用于兼容旧配置，不再影响运行时行为 |

:::warning Full Access 使用注意事项
`full_access` 会自动同意审批请求，不会弹窗打断。请确保沙箱与权限边界配置正确；危险命令硬拦截、sandbox/network/lease 约束仍会继续生效。
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
BYTEMIND_SANDBOX_ENABLED=true BYTEMIND_WRITABLE_ROOTS=./src bytemind
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
  "max_iterations": 64,
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
