# 配置参考

`.bytemind/config.json` 所有字段的完整说明。

可用示例参考 [`config.example.json`](https://github.com/1024XEngineer/bytemind/blob/main/config.example.json)。

## `provider`

模型 Provider 配置。

| 字段                | 类型   | 说明                                     | 默认值                      |
| ------------------- | ------ | ---------------------------------------- | --------------------------- |
| `type`              | string | `openai-compatible` 或 `anthropic`       | `openai-compatible`         |
| `base_url`          | string | API 端点 URL                             | `https://api.openai.com/v1` |
| `model`             | string | 使用的模型 ID                            | `gpt-5.4-mini`              |
| `api_key`           | string | API 密钥（明文，建议改用 `api_key_env`） | —                           |
| `api_key_env`       | string | 从该环境变量读取 API 密钥                | `BYTEMIND_API_KEY`          |
| `anthropic_version` | string | Anthropic API 版本头                     | `2023-06-01`                |
| `auth_header`       | string | 自定义鉴权头名称                         | `Authorization`             |
| `auth_scheme`       | string | 鉴权前缀（如 `Bearer`）                  | `Bearer`                    |
| `auto_detect_type`  | bool   | 根据 `base_url` 自动推断 Provider 类型   | `false`                     |

## `approval_policy`

| 值                   | 行为                         |
| -------------------- | ---------------------------- |
| `on-request`（默认） | 每次高风险工具调用前等待确认 |

## `approval_mode`

| 值                    | 行为                                  |
| --------------------- | ------------------------------------- |
| `interactive`（默认） | 交互式审批，每次操作弹出确认          |
| `away`                | 无人值守模式，根据 `away_policy` 处理 |

## `away_policy`

仅在 `approval_mode` 为 `away` 时生效。

| 值                           | 行为                         |
| ---------------------------- | ---------------------------- |
| `auto_deny_continue`（默认） | 自动拒绝高风险操作并继续执行 |
| `fail_fast`                  | 遇到需审批的操作立即终止任务 |

## `max_iterations`

| 类型    | 默认值 |
| ------- | ------ |
| integer | `32`   |

单任务最大工具调用轮次。到达上限后 Agent 输出阶段性总结并停止。

## `stream`

| 类型 | 默认值 |
| ---- | ------ |
| bool | `true` |

开启流式输出。非 TTY 环境（如 CI 管道）建议设为 `false`。

## `sandbox_enabled`

| 类型 | 默认值  |
| ---- | ------- |
| bool | `false` |

设为 `true` 后，文件和 Shell 工具的写入操作将被限制在 `writable_roots` 范围内。

## `writable_roots`

| 类型     | 默认值 |
| -------- | ------ |
| string[] | `[]`   |

开启沙箱时允许写入的目录列表。

## `exec_allowlist`

跳过审批提示的 Shell 命令白名单。

```json
{
  "exec_allowlist": [
    { "command": "go", "args_pattern": ["test", "./..."] },
    { "command": "make", "args_pattern": ["build"] }
  ]
}
```

## `token_quota`

| 类型    | 默认值   |
| ------- | -------- |
| integer | `300000` |

单会话 token 消耗预警阈值。

## `update_check`

| 字段      | 类型 | 默认值 | 说明               |
| --------- | ---- | ------ | ------------------ |
| `enabled` | bool | `true` | 启动时是否检查更新 |

## `context_budget`

上下文窗口用量管理。

| 字段                 | 类型  | 默认值 | 说明                           |
| -------------------- | ----- | ------ | ------------------------------ |
| `warning_ratio`      | float | `0.85` | 用量达到此比例时输出警告       |
| `critical_ratio`     | float | `0.95` | 用量达到此比例时触发压缩或停止 |
| `max_reactive_retry` | int   | `1`    | 上下文压缩后最大重试次数       |

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
  "sandbox_enabled": false,
  "writable_roots": [],
  "token_quota": 300000,
  "update_check": { "enabled": true },
  "context_budget": {
    "warning_ratio": 0.85,
    "critical_ratio": 0.95,
    "max_reactive_retry": 1
  }
}
```
