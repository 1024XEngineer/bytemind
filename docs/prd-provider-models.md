# PRD: Provider / Model 管理

## 0. 文档信息

- 产品：ByteMind
- 功能名称：Provider / Model 管理
- 关联 Issue：1024XEngineer/bytemind#323
- 文档日期：2026-05-08
- 文档状态：当前产品口径
- 覆盖范围：模型列表、模型切换、模型新增、模型删除、provider 配置持久化

本文只描述当前应保留的 Provider / Model 管理闭环，不写未来规划。

## 1. 产品定义

ByteMind 的 Provider / Model 管理用于让用户在 TUI 内完成模型目标的查看、切换、新增和删除。

当前只保留三个用户入口：

- `/models`
- `/model add`
- `/model delete`

其中 `/models` 同时承担模型状态查看和模型切换；用户输入 `/models` 后，TUI 展示可用 provider/model 列表，用户直接在该列表中选择目标并切换。系统不再提供单独的 `/model picker` 入口，也不保留 `/add model`、`/delete model` 这种反向命令格式。

## 2. 设计原则

- 入口收敛：provider/model 管理只暴露三条命令，避免命令体系发散。
- 列表即操作：`/models` 展示列表后即可完成切换，不再让用户记住额外切换命令。
- 配置即状态：新增、删除、切换都需要同步更新运行时状态，并在成功后写回配置。
- 当前优先：默认围绕本地 TUI 和当前 workspace 配置工作，不引入远程控制台或批处理管理界面。

## 3. 用户入口

### 3.1 `/models`

`/models` 是模型管理主入口。

输入 `/models` 后，TUI 展示当前模型状态和可选模型列表：

- 当前 active provider/model。
- 当前 default provider/model。
- 已配置的 provider/model 目标。
- 可展示的 discovered model 目标。
- provider 分组。
- `active`、`default`、`family=<value>` 等标签。
- provider 列表失败时的 warning。

用户在 `/models` 展示的列表中直接移动光标并按 Enter 选择目标。选择后系统完成模型切换：

- 校验目标是否存在。
- 更新 `provider_runtime.default_provider`。
- 更新 `provider_runtime.default_model`。
- 更新目标 provider 的 `model`。
- 创建新的 runtime provider client。
- 更新当前 runner。
- 刷新 token budget 与 token usage 状态。
- 将选择写回配置文件。

切换成功后，当前会话后续请求立即使用新 provider/model。

### 3.2 `/model add`

`/model add` 用于新增或修正一个 provider/model 目标。

输入 `/model add` 后，TUI 进入配置引导，依次收集：

1. `provider`：provider 类型。
2. `base_url`：provider endpoint。
3. `model`：模型名。
4. `api_key`：API key。

引导支持用户直接输入带字段名的值：

```text
provider=anthropic
base_url=https://api.anthropic.com
model=claude-sonnet-4-20250514
api_key=sk-...
```

配置校验通过后，系统写回 `provider` 和 `provider_runtime`，并立即更新当前运行时 client。

### 3.3 `/model delete`

`/model delete` 用于删除已配置的 provider/model 目标。

输入 `/model delete` 后，TUI 展示已配置模型目标列表。用户在列表中选择目标并确认删除。

删除规则：

- 只允许删除配置中存在的 provider/model 目标。
- 不允许删除最后一个已配置目标。
- 如果删除的是当前 active/default 目标，系统从剩余目标中选择一个新的默认目标。
- 删除成功后更新当前 runner runtime。
- 删除成功后写回配置文件。

## 4. Slash Command

| 命令 | 行为 |
| --- | --- |
| `/models` | 展示 provider/model 列表，并允许在列表中直接切换 active 模型 |
| `/model add` | 打开 provider/model 配置引导，新增或修正模型目标 |
| `/model delete` | 打开已配置模型目标列表，删除一个模型目标 |

不保留以下入口：

- `/model picker`
- `/add model`
- `/delete model`
- `/models add ...`
- `/models remove ...`
- `/models replace ...`
- `/models use ...`

## 5. 配置模型

### 5.1 `provider`

`provider` 保留为单 provider 兼容配置，也是新增模型时写入的基础配置。

```json
{
  "provider": {
    "type": "openai-compatible",
    "family": "",
    "auto_detect_type": false,
    "base_url": "https://api.openai.com/v1",
    "api_path": "",
    "model": "gpt-5.4-mini",
    "api_key": "",
    "api_key_env": "BYTEMIND_API_KEY",
    "auth_header": "Authorization",
    "auth_scheme": "Bearer",
    "extra_headers": {},
    "anthropic_version": "2023-06-01"
  }
}
```

关键字段：

- `type`：`openai-compatible`、`openai`、`anthropic`、`gemini`。
- `base_url`：provider endpoint。
- `model`：当前 provider 默认模型。
- `api_key`：明文 API key。
- `api_key_env`：读取 API key 的环境变量。
- `auth_header`、`auth_scheme`、`extra_headers`：自定义鉴权与扩展 header。
- `anthropic_version`：Anthropic API version。

### 5.2 `provider_runtime`

`provider_runtime` 是运行时实际使用的 provider/model 注册表。

```json
{
  "provider_runtime": {
    "default_provider": "openai",
    "default_model": "gpt-5.4-mini",
    "allow_fallback": false,
    "providers": {
      "openai": {
        "type": "openai-compatible",
        "base_url": "https://api.openai.com/v1",
        "model": "gpt-5.4-mini",
        "api_key_env": "BYTEMIND_API_KEY"
      }
    },
    "health": {
      "fail_threshold": 3,
      "recover_probe_sec": 20,
      "recover_success_threshold": 2,
      "window_size": 5
    }
  }
}
```

规则：

- 如果没有 `provider_runtime.providers`，系统从 `provider` 自动生成 runtime。
- `default_provider` 指向当前默认 provider。
- `default_model` 指向当前默认 model。
- `providers` 以 provider id 为 key。
- provider id 需要归一化为小写。
- 当前一个 provider entry 对应一个默认 model。
- 切换、新增、删除都必须保持 `provider` 与 `provider_runtime` 同步。

## 6. 配置加载与覆盖

默认配置：

- 默认 provider type：`openai-compatible`
- 默认 base URL：`https://api.openai.com/v1`
- 默认 model：`gpt-5.4-mini`
- 默认 key env：`BYTEMIND_API_KEY`

未显式传入 config path 时，加载顺序为：

1. 内置默认配置。
2. 用户配置：`~/.bytemind/config.json`。
3. 项目配置：`<workspace>/.bytemind/config.json`。
4. 用户配置中存在 provider 凭证时，用户 provider/provider_runtime 凭证覆盖回当前配置。
5. 环境变量覆盖。
6. normalize。

provider 相关环境变量：

| 环境变量 | 覆盖字段 |
| --- | --- |
| `BYTEMIND_PROVIDER_TYPE` | `provider.type` |
| `BYTEMIND_PROVIDER_FAMILY` | `provider.family` |
| `BYTEMIND_PROVIDER_AUTO_DETECT_TYPE` | `provider.auto_detect_type` |
| `BYTEMIND_BASE_URL` | `provider.base_url` |
| `BYTEMIND_MODEL` | `provider.model` |
| `BYTEMIND_API_KEY` | `provider.api_key` |
| `BYTEMIND_API_KEY_ENV` | `provider.api_key_env` |

## 7. 支持的 Provider

| Provider type | 请求实现 | 默认 base URL | 默认 model |
| --- | --- | --- | --- |
| `openai-compatible` | OpenAI Chat Completions 兼容协议 | `https://api.openai.com/v1` | `gpt-5.4-mini` |
| `openai` | OpenAI Chat Completions 兼容协议 | `https://api.openai.com/v1` | `gpt-5.4-mini` |
| `anthropic` | Anthropic Messages API | `https://api.anthropic.com` | 用户填写 |
| `gemini` | Gemini `generateContent` API | `https://generativelanguage.googleapis.com/v1beta` | `gemini-2.5-flash` |

## 8. 运行时行为

模型列表由当前 runtime provider registry 生成：

- 使用 `provider_runtime` 构建 registry。
- 获取 provider/model 目标。
- 合并配置中的模型目标和可发现模型目标。
- 按 provider id 和 model id 排序。
- 返回 warning，不让单个 provider 失败中断整个列表。

模型请求由 provider router 执行：

- runner 使用 `provider_runtime.default_model` 作为请求 model。
- router 按 requested model、default provider、default model、健康状态和 fallback 策略选择 provider。
- provider health 为 unavailable 的目标不会参与路由。
- 切换模型后，当前 runner 立即替换 runtime client。

## 9. 持久化要求

新增、切换、删除模型都需要写回配置文件。

写回规则：

- 使用结构化 JSON 更新。
- 同步更新 `provider` 与 `provider_runtime`。
- 补齐默认配置字段。
- 默认写入 `<workspace>/.bytemind/config.json`。
- 显式 config path 存在时写入显式路径。
- 写入成功后当前会话立即使用新 runtime。

## 10. 验收口径

- `/models` 可以展示当前 active/default provider/model。
- `/models` 展示列表后可以直接选择并切换模型。
- `/model add` 可以完成 provider/base_url/model/api_key 配置引导。
- `/model add` 成功后，新模型目标进入 provider runtime，并立即可用。
- `/model delete` 可以删除一个已配置模型目标。
- `/model delete` 不允许删除最后一个模型目标。
- 切换、新增、删除都能写回配置并更新当前 runner。
