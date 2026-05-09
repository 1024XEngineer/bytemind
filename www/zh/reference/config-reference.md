# 配置参考

`~/.bytemind/config.json` 和项目级 `.bytemind/config.json` 所有字段的完整说明。

可用示例参考 [`config.example.json`](https://github.com/1024XEngineer/bytemind/blob/main/config.example.json)。

## `provider`（单 Provider，兼容旧配置）

单一模型 Provider 配置。如需配置多个 Provider 并在运行时切换，建议使用下方的 `provider_runtime`。

| 字段                | 类型   | 说明                                     | 默认值                      |
| ------------------- | ------ | ---------------------------------------- | --------------------------- |
| `type`              | string | `openai-compatible`、`anthropic` 或 `gemini` | `openai-compatible`      |
| `base_url`          | string | API 端点 URL                             | `https://api.openai.com/v1` |
| `model`             | string | 使用的模型 ID                            | `gpt-5.4-mini`              |
| `api_key`           | string | API 密钥（明文，建议改用 `api_key_env`） | —                           |
| `api_key_env`       | string | 从该环境变量读取 API 密钥                | `BYTEMIND_API_KEY`          |
| `anthropic_version` | string | Anthropic API 版本头                     | `2023-06-01`                |
| `auth_header`       | string | 自定义鉴权头名称                         | `Authorization`             |
| `auth_scheme`       | string | 鉴权前缀（如 `Bearer`）                  | `Bearer`                    |
| `auto_detect_type`  | bool   | 根据 `base_url` 自动推断 Provider 类型   | `false`                     |
| `family`            | string | Provider 系列标签（用于显示）             | —                           |
| `api_path`          | string | 自定义 API 路径覆盖                      | —                           |
| `models`            | array  | 该 Provider 可用的模型 ID 列表            | —                           |
| `extra_headers`     | object | 额外的 HTTP 请求头                        | —                           |

## `provider_runtime`（多 Provider）

配置多个模型 Provider，并在运行时通过 `/model` 命令切换。当 `provider_runtime` 存在时，优先级高于旧版 `provider` 字段。

### 顶层字段

| 字段              | 类型   | 说明                                                   | 默认值                    |
| ----------------- | ------ | ------------------------------------------------------ | ------------------------- |
| `current_provider` | string | 当前激活的 Provider ID（如 `"deepseek"`）              | （providers map 中的第一个） |
| `default_provider` | string | 兜底 Provider ID                                       | 同 `current_provider`     |
| `default_model`   | string  | 兜底模型 ID，当 Provider 内未设置 `model` 时使用       | —                         |
| `allow_fallback`  | bool    | 是否允许自动故障转移至其他 Provider                     | `false`                   |
| `providers`       | object  | Provider ID → Provider 配置的映射表（见下方）           | （必填）                   |
| `health`          | object  | 故障转移健康检查配置（见下方）                          | 见下方                    |

### `providers.<id>` 字段

每个 Provider 配置支持上述旧版 `provider` 的全部字段，特别注意：

| 字段        | 类型   | 说明                                                        |
| ----------- | ------ | ----------------------------------------------------------- |
| `type`      | string | `openai-compatible`、`anthropic` 或 `gemini`                |
| `base_url`  | string | API 端点 URL                                                 |
| `model`     | string | 该 Provider 当前选用的模型（`/model` 切换时会自动更新）       |
| `models`    | array  | 可切换的模型 ID 列表。**必须填写**，否则 `/model` 选择器不会显示可选项 |
| `api_key_env` | string | 从该环境变量读取 API 密钥                                  |
| `api_key`   | string | API 密钥明文（建议使用 `api_key_env`）                       |

### `health` 字段

| 字段                       | 类型 | 默认值 | 说明                       |
| -------------------------- | ---- | ------ | -------------------------- |
| `fail_threshold`           | int  | `3`    | 连续失败多少次后标记为不健康 |
| `recover_probe_sec`        | int  | `30`   | 恢复探测间隔（秒）          |
| `recover_success_threshold` | int  | `2`   | 连续成功多少次后标记为健康   |
| `window_size`              | int  | `60`   | 滚动窗口大小（秒）           |

### 模型切换流程

1. 在 `provider_runtime.providers` 下配置多个 Provider，每个都写好 `models` 列表。
2. 启动 ByteMind — 默认使用 `current_provider` 及其对应的 `model`。
3. 输入 `/model` 打开交互式选择器，或输入 `/model <provider>/<model>` 直接切换。
4. 配置文件会被自动更新：`current_provider` 和对应 Provider 的 `model` 字段会被重写。

### 多 Provider 示例

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
    },
    "health": {
      "fail_threshold": 3,
      "recover_probe_sec": 30,
      "recover_success_threshold": 2,
      "window_size": 60
    }
  }
}
```

### 添加新 Provider

编辑 `config.json`，在 `provider_runtime.providers` 下新增一个条目：

```json
"providers": {
  "deepseek": { ... },
  "openai": { ... },
  "my-new-provider": {
    "type": "openai-compatible",
    "base_url": "https://api.my-provider.com/v1",
    "api_key_env": "MY_PROVIDER_API_KEY",
    "model": "my-model",
    "models": ["my-model", "my-other-model"]
  }
}
```

重启 ByteMind 或使用 `/model` 即可在切换器中看到新增的 Provider 及其模型。

:::tip 从旧版 `provider` 迁移
如果配置文件中只有旧版的 `provider` 字段，ByteMind 启动时会自动将其转换为 `provider_runtime`。使用 `/model` 切换模型后，选择结果会持久化到 `provider_runtime`。你也可以手动将配置改为上面的多 Provider 格式。
:::

## `approval_policy`

| 值                   | 行为                         |
| -------------------- | ---------------------------- |
| `on-request`（默认） | 每次高风险工具调用前等待确认 |

## `approval_mode`

| 值                    | 行为                                  |
| --------------------- | ------------------------------------- |
| `interactive`（默认） | 交互式审批，每次操作弹出确认          |
| `full_access`         | 全部权限模式，审批请求自动通过且不中断任务 |

兼容说明：为避免静默提权，`approval_mode: away` 默认被阻止。仅在迁移旧配置时，显式设置 `BYTEMIND_ALLOW_AWAY_FULL_ACCESS=true` 才会临时映射到 `full_access`。

## `away_policy`

已弃用兼容字段。保留用于兼容旧配置形状，不再影响运行时行为。

| 值                           | 行为                         |
| ---------------------------- | ---------------------------- |
| `auto_deny_continue`（默认） | 仅用于兼容旧配置，不再影响运行时行为 |
| `fail_fast`                  | 仅用于兼容旧配置，不再影响运行时行为 |

## `notifications.desktop`

桌面通知偏好设置。

| 字段                    | 类型 | 默认值 | 说明 |
| ----------------------- | ---- | ------ | ---- |
| `enabled`               | bool | `true` | 桌面通知总开关。 |
| `on_approval_required`  | bool | `true` | 出现审批请求时发送通知。 |
| `on_run_completed`      | bool | `true` | 任务成功完成时发送通知。 |
| `on_run_failed`         | bool | `true` | 任务失败时发送通知。 |
| `on_run_canceled`       | bool | `false` | 任务取消时发送通知。 |
| `cooldown_seconds`      | int  | `3` | 同一通知 key 的去重窗口。`0` 表示关闭 cooldown 去重。 |

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

### 多 Provider（推荐）

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
    },
    "health": {
      "fail_threshold": 3,
      "recover_probe_sec": 30,
      "recover_success_threshold": 2,
      "window_size": 60
    }
  },
  "approval_policy": "on-request",
  "approval_mode": "interactive",
  "notifications": {
    "desktop": {
      "enabled": true,
      "on_approval_required": true,
      "on_run_completed": true,
      "on_run_failed": true,
      "on_run_canceled": false,
      "cooldown_seconds": 3
    }
  },
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

### 单 Provider（兼容旧版）

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
  "context_budget": {
    "warning_ratio": 0.85,
    "critical_ratio": 0.95,
    "max_reactive_retry": 1
  }
}
```
