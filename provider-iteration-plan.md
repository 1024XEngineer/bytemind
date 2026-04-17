# Provider 模块迭代方案

## 0. 现状基线（先校正文档锚点）

基于当前仓库真实实现，本轮 provider 迭代必须先对齐以下调用链与职责边界，否则后续设计会偏离代码现状：

- Runner 的请求组装入口在 `internal/agent/turn_processing.go:38`，由 `contextpkg.BuildChatRequest(...)` 构造 `llm.ChatRequest`
- Runner 的流式/非流式调用入口在 `internal/agent/completion_runtime.go:14`
- 当前 `Runner` 直接依赖 `llm.Client`，并通过 `StreamMessage/CreateMessage` 与 provider 交互：`internal/agent/runner.go:37`、`internal/agent/completion_runtime.go:15-19`
- 当前仅存在一次显式 stream -> non-stream 回退，且决策权在 Runner：`internal/agent/completion_runtime.go:40-46`
- `context_too_long` 语义已被上层压缩/重试逻辑依赖：`internal/agent/prompt_too_long.go:22-40`、`internal/llm/errors.go:17-18`
- 当前轻量 provider 可用性探测在 `internal/provider/preflight.go:25-99`
- 当前已有一套流式事件契约底座：`internal/llm/contract.go:13-31`

因此，本轮方案的首要原则不是“按想象中的模块边界重画”，而是基于上述真实锚点做渐进演进。

## 1. 目标说明

基于 `prd-provider.md` 的目标，结合当前 ByteMind 代码基线，Provider 模块本轮迭代的核心不是“再加一个 provider”，而是把现有 `internal/provider` 从“面向 OpenAI / Anthropic 的适配实现”升级为“可治理、可路由、可观测的统一供应商抽象层”，并让 `agent`、`context`、`config` 等上层模块只依赖稳定契约。

当前项目已经具备以下可复用基础：

- 已有 OpenAI-compatible 与 Anthropic 双 provider 接入能力：`internal/provider/factory.go:10`
- 已有统一 LLM 请求/消息结构：`internal/llm/types.go:33`、`internal/llm/types.go:338`
- 已有 Runner 对单一 `llm.Client` 的消费入口：`internal/agent/runner.go:37`
- 已有 context 模块负责请求构建与能力裁剪，真实入口是 `internal/agent/turn_processing.go:38` 调用 `contextpkg.BuildChatRequest(...)`
- 已有基础 provider 错误映射，但语义仍然偏弱：`internal/llm/errors.go:9`

因此，本次方案应尽量沿用现有 `llm.ChatRequest` / `llm.Message` / `llm.Client` 基线，优先通过 Provider 内部新增层次完成演进，避免一次性打穿上层调用链。

---

## 2. 当前项目与 PRD 的差距分析

### 2.1 当前已有能力

#### Provider 工厂与接入

当前 Provider 工厂仍是基于配置类型直接分支实例化单个 client：`internal/provider/factory.go:10`。

这意味着：

- 只能创建一个活动 provider client
- 没有 registry 概念
- 没有多 provider 目录聚合
- 没有 route / fallback 输出

#### OpenAI-compatible 流式能力

OpenAI-compatible 已具备较完整的 SSE 流解析与 tool call 拼装逻辑：`internal/provider/openai.go:74`。

这部分是后续“流式事件规范化”的最佳切入点，因为已经存在：

- chunk 读取
- delta 解析
- tool call 增量聚合
- usage 提取

但当前输出仍然是“最终 `llm.Message` + 可选文本 delta 回调”，尚未统一到结构化事件流消费模型。

#### Anthropic 兼容能力

Anthropic 目前 `StreamMessage` 实际是退化为一次性 `CreateMessage` 后再回调全文：`internal/provider/anthropic.go:147`。

这说明：

- Anthropic 的流式语义尚未真正接入
- 上层虽然使用统一接口，但底层并未统一到一致的 streaming contract
- Provider PRD 中关于事件顺序与终态约束，当前并未落地

#### 请求装配边界

当前真实边界不是 `internal/context/request.go`，而是 Runner 在 `internal/agent/turn_processing.go:38-44` 调用 `contextpkg.BuildChatRequest(...)` 完成请求组装。

这与 PRD 的边界约束基本一致：预算与上下文治理应留在 context/agent，provider 只处理模型调用与硬约束。因此这个边界应保留，不应把预算逻辑回灌到 provider。

#### 现有流式契约底座

仓库已经存在 `internal/llm/contract.go:13-31`：

- `StreamEventType`
- `StreamEvent`
- `ProviderClient.Chat(...)`

虽然这套契约尚未成为当前 Runner 主链消费接口，但它已经是仓库里唯一成型的结构化流式事件模型。后续若要推进事件规范化，应优先复用或正式替换它，而不是在 `internal/provider` 再平行新建一套长期并存的事件协议。

### 2.2 当前主要缺口

结合 PRD，当前代码缺口主要有六类：

1. 缺少独立的 Provider 域模型与统一契约层，当前主调用面仍只暴露 `llm.Client`
2. 缺少多 provider 注册中心，工厂仍是一次只返回一个 client
3. 缺少路由、排序、fallback 决策层
4. 结构化流事件未进入主链，`onDelta func(string)` 仍是当前 Runner 可见接口
5. 缺少完整错误码闭环与 retryable 刚性规则，当前只有 rate limit / context too long 等少数语义：`internal/llm/errors.go:67`
6. 缺少健康检查、状态机、半开恢复与路由联动能力

---

## 3. 设计原则

本轮改造建议遵循以下原则：

### 3.1 渐进兼容，避免一次性替换 `llm.Client`

当前 `agent.Runner` 直接依赖 `llm.Client`：`internal/agent/runner.go:40`。如果直接把所有上层改成依赖 PRD 中全新 `provider.Client` / `Router` / `Registry`，改动面会过大。

建议做法：

- 在 `internal/provider` 新增面向 PRD 的新契约
- 同时提供一个兼容适配层，把“路由结果中的最终 client”适配回现有 `llm.Client`
- 先做到“上层感知最小化”，再逐步把 runner 切到新的 provider facade

### 3.2 通用消息结构继续复用 `internal/llm`，流式事件契约必须单点归属

`internal/llm` 目前已经承载：

- Message
- ChatRequest
- Usage
- ToolDefinition
- 基础错误结构
- 一套已有的流式事件底座：`internal/llm/contract.go:13-31`

建议不要在 provider 内重复造一套消息体系，也不要让事件契约长期双轨并存。更合理的是：

- provider 层补充路由、目录、健康、错误语义
- 数据载体尽量复用 `llm.Message`、`llm.ChatRequest`
- 流式事件 contract 先确定唯一归属层：优先复用 `internal/llm/contract.go`，若要迁移则必须正式替换并清理旧契约，而不是双轨保留

### 3.3 把“可配置策略”与“运行时状态”分开

当前 `config.ProviderConfig` 仍然是单 provider 配置：`internal/config/config.go:34`。

而 PRD 需要：

- providers 列表
- routing fallback 策略
- health 参数

建议配置改造成三层：

1. provider static config：连接信息、model 列表、provider 类型
2. routing policy config：默认 provider / 默认 model / fallback map / 偏好
3. runtime health state：内存态，不落配置

### 3.4 fallback 决策权必须唯一

当前 Runner 已有一次 stream -> non-stream 回退：`internal/agent/completion_runtime.go:40-46`。

因此后续若引入 `RoutedClient`：

- provider 路由 fallback 与 stream/non-stream 回退不能分散在多层做最终决策
- 必须明确只有一层拥有“最终切换下一个 provider”的权力
- 否则会出现双重重试、请求放大、token 统计和 latency 统计口径失真

建议收敛方式：

- `RoutedClient` 拥有 provider 级 fallback 决策权
- Runner 保留仅针对“空流结果”的兼容回退，且不得再次触发 provider 级重试链
- 若实现上难以区分，则进一步把这段 stream -> non-stream 回退也下沉到 `RoutedClient`，Runner 只消费最终结果

### 3.5 `context_too_long` 保持强语义，不允许被泛化吞没

当前上层已依赖 `context_too_long` 触发压缩/重试：`internal/agent/prompt_too_long.go:22-40`、`internal/llm/errors.go:75-80`。

因此错误语义改造时必须保证：

- `context_too_long` 继续作为强语义保留
- 不能因为统一未知错误到 `unavailable` 而吞掉这条可恢复路径
- 相关兼容测试必须与错误码闭环同批落地

---

## 4. 推荐迭代步骤

## Step 1：先补“现状基线 + 单一契约归属”

### 目标

先修正文档与接口基线，避免后续设计建立在错误文件路径和重复契约上。

### 建议动作

- 在方案和实现说明中统一修正真实调用链：`internal/agent/turn_processing.go:38-44` + `internal/agent/completion_runtime.go:14-46`
- 明确 `context` 负责请求构造，provider 不接管预算与会话治理
- 明确结构化流事件的唯一归属层：优先沿用 `internal/llm/contract.go:13-31`
- 若决定未来迁移到 provider 层，则必须在同一迭代中提供替代契约、完成调用方切换，并清理旧契约

### 产出验收

- 文档锚点全部指向真实文件
- 流式事件 contract 不再存在“双轨长期并存”的设计歧义
- fallback 与错误语义边界在方案层先说清楚

---

## Step 2：补齐 Provider 领域契约层

### 目标

先把 PRD 中的核心接口落地为独立文件，但暂不要求所有调用方一次切换完成。

### 建议新增文件

- `internal/provider/interfaces.go`
- `internal/provider/types.go`
- `internal/provider/compat.go`

### 建议落地内容

在 `interfaces.go` 中定义：

- `ProviderID`
- `ModelID`
- `Client`
- `Registry`
- `Router`
- `HealthChecker`
- `RouteContext`
- `RouteResult`

在 `types.go` 中定义：

- `ModelInfo`
- `ProviderWarning`
- `Usage`
- `ProviderError`
- `HealthStatus`

事件结构不要在此步新增第二套长期契约；若确需补充字段，应围绕 `internal/llm/contract.go` 扩展或提供短期适配层。

在 `compat.go` 中提供兼容层：

- 把现有 `OpenAICompatible`、`Anthropic` 包装成新的 `provider.Client`
- 提供将 provider 请求映射为 `llm.ChatRequest` 的桥接逻辑
- 保留现有 `llm.Client` 路径供 runner 继续运行

### 实施细节

1. 不要立即删除 `llm.Client`，先做双接口并存
2. provider 请求优先复用 `llm.ChatRequest`，避免重复字段扩散
3. `ProviderID` 先用字符串别名实现，降低接入复杂度

### 产出验收

- `internal/provider` 内有独立契约层
- OpenAI/Anthropic 可被包装为统一 provider client
- 现有功能不回归

---

## Step 3：将工厂升级为 Registry 装配入口

### 目标

把当前单 client 工厂升级为“多 provider 注册与查询中心”，同时兼容当前默认 provider 运行模式。

### 当前基线

当前工厂仅按 `cfg.Type` 返回一个 `llm.Client`：`internal/provider/factory.go:10`。

### 建议改造

新增：

- `internal/provider/registry.go`
- `internal/provider/registry_test.go`
- `internal/provider/models.go`

保留 `NewClient(cfg)`，同时新增：

- `NewRegistry(cfg config.ProviderRuntimeConfig) (Registry, error)`
- `NewDefaultFacade(cfg ...)` 或 `NewRouterClient(...)`

### Registry 具体行为

1. 并发安全注册 provider client
2. 重复注册时报标准错误 `duplicate_provider`
3. `Get/List` 为纯查询，不携带业务决策
4. `ListModels` 聚合返回 `models + warnings`
5. 唯一键严格使用 `(provider_id, model_id)`

### 配置改造建议

当前 `config.ProviderConfig` 是单 provider 结构，不够承载 PRD：`internal/config/config.go:34`。

建议新增结构，而不是强改旧字段：

- 保留 `ProviderConfig` 作为兼容模式
- 新增 `ProviderRuntimeConfig` 或 `ProvidersConfig`
- `providers.<id>` 必须完整继承现有单 provider 连接字段能力，至少保留：`base_url`、`api_path`、`api_key`、`api_key_env`、`auth_header`、`auth_scheme`、`extra_headers`、`anthropic_version`

建议形态：

```json
{
  "provider": {
    "default_provider": "openai",
    "default_model": "gpt-5.4-mini",
    "providers": {
      "openai": {
        "type": "openai-compatible",
        "base_url": "https://api.openai.com",
        "api_path": "/v1/chat/completions",
        "api_key_env": "OPENAI_API_KEY",
        "auth_header": "Authorization",
        "auth_scheme": "Bearer",
        "extra_headers": {},
        "model": "gpt-5.4-mini"
      },
      "anthropic": {
        "type": "anthropic",
        "base_url": "https://api.anthropic.com",
        "api_key_env": "ANTHROPIC_API_KEY",
        "anthropic_version": "2023-06-01",
        "model": "claude-sonnet-4-20250514"
      }
    }
  }
}
```

### 兼容策略

如果用户仍使用旧配置：

- 自动把旧 `ProviderConfig` 映射为单 provider registry
- 默认 registry 中仅注册一个 provider
- openai-compatible 网关相关字段必须原样保留，避免能力倒退

### 产出验收

- 系统支持一次加载多个 provider
- 旧配置不失效
- openai-compatible 扩展能力不回归
- 模型目录聚合支持部分成功与 warnings

---

## Step 4：引入 Router 与唯一 fallback 决策层

### 目标

在 provider 之上加入明确的路由层，让模型选择、候选过滤、排序、fallback 成为可测试的独立能力，并避免双重重试。

### 建议新增文件

- `internal/provider/router.go`
- `internal/provider/router_policy.go`
- `internal/provider/router_test.go`

### 路由输入建议

路由入参应包含：

- requested model
- route context
- registry 可用模型视图
- health 状态快照

### RouteContext 初版建议

结合当前 ByteMind CLI 现状，初版不需要把所有 PRD 字段都用满，但结构先定下来：

- `Scenario`
- `Region`
- `PreferLatency`
- `PreferLowCost`
- `AllowFallback`
- `Tags`

其中 `Scenario` 可先从 agent mode 或命令类型映射，例如 chat/run；后续再细分为 coding / planning / tool-heavy。

### 路由流程建议

严格按 PRD 顺序执行：

1. 优先命中显式指定模型
2. 过滤健康不可用 provider
3. 过滤模型不满足约束的 provider
4. 按策略排序
5. 输出 primary 和 ordered fallbacks

### 与现有 runner 的衔接

当前 `Runner` 只有一个 `llm.Client` 字段：`internal/agent/runner.go:63`。

建议新增一个 facade，例如：

- `provider.RoutedClient`

它对外仍实现 `llm.Client`，但内部会：

1. 根据 request/model 调用 router
2. 选择 primary provider 执行
3. 根据错误是否 retryable 决定是否切 fallback

### fallback 责任边界

这里必须明确：

- provider 级 fallback 只允许 `RoutedClient` 决策一次
- Runner 不应再做第二层 provider fallback
- `internal/agent/completion_runtime.go:40-46` 的 stream -> non-stream 回退要么保留为“同 provider 同请求形态降级”，要么整体下沉到 `RoutedClient`
- 无论选择哪种做法，都要保证 token/latency 统计只按最终定义的请求链路记账，不出现重复放大

### 产出验收

- 上层仍按 `llm.Client` 调用
- provider 内部已具备 route + fallback 能力
- fallback 决策权唯一
- 无候选时统一返回 `unavailable`

---

## Step 5：重构错误语义与 retryable 规则

### 目标

把当前 `internal/llm/errors.go` 的弱映射，提升为 Provider 域内稳定错误闭环，同时兼容现有 `context_too_long` 路径。

### 当前问题

当前错误码只有：

- `rate_limited`
- `context_too_long`
- `unknown`

且 `MapProviderError` 主要依赖 HTTP 状态码与模糊文本：`internal/llm/errors.go:67`。

与 PRD 相比，缺少：

- unauthorized
- timeout
- unavailable
- bad_request
- provider_not_found
- duplicate_provider
- 刚性的 retryable 规则

### 建议新增文件

- `internal/provider/errors.go`
- `internal/provider/error_map_openai.go`
- `internal/provider/error_map_anthropic.go`
- `internal/provider/errors_test.go`

### 推荐做法

1. Provider 域定义新的错误码枚举
2. 提供 `MapError(err error) *provider.Error`
3. 保留原始 `status/body/raw error` 到 detail 字段，方便诊断
4. `retryable` 不通过调用方判断，而是映射时直接固化
5. `context_too_long` 明确保留为强语义，并提供到现有 `llm.ProviderError` 的兼容转换

### 与 `internal/llm/errors.go` 的关系

建议不要立刻删除旧错误结构，而是：

- 新错误体系在 `internal/provider` 内闭环
- 对外提供转换函数，把 provider error 适配成当前 Runner 能识别的错误
- `context_too_long` 必须继续映射到 `llm.ErrorCodeContextTooLong`
- 后续稳定后再考虑是否把 `llm/errors.go` 收敛为通用底座

### 必测场景

- 401 -> unauthorized, retryable=false
- 400 -> bad_request, retryable=false
- 413 或 context-length 语义 -> context_too_long, retryable=false
- 429 -> rate_limited, retryable=true
- timeout/network timeout -> timeout, retryable=true
- 5xx -> unavailable, retryable=true
- registry miss -> provider_not_found, retryable=false
- duplicate register -> duplicate_provider, retryable=false
- prompt too long 压缩链路继续可触发

### 产出验收

- 上层只看错误码与 retryable 就能做 fallback / 提示
- `context_too_long` 兼容现有压缩/重试路径
- 未知错误可映射 unavailable，但不能吞掉已知强语义错误

---

## Step 6：建立统一流式事件规范化层

### 目标

把当前“字符串 delta 回调”升级为结构化事件流，同时保持现有终端输出链路可用。

### 当前问题

OpenAI 的流逻辑已经较完整，但只暴露 `onDelta func(string)`：`internal/provider/openai.go:74`。
Anthropic 实际没有真流：`internal/provider/anthropic.go:147`。

这意味着当前无法满足 PRD 对以下能力的要求：

- `start/delta/tool_call/usage/result/error` 事件统一
- 终止事件互斥
- 每个事件注入公共字段
- 非法 chunk 转 `EventError`

### 建议新增文件

- `internal/provider/normalize.go`
- `internal/provider/openai_stream.go`
- `internal/provider/anthropic_stream.go`
- `internal/provider/normalize_test.go`

如必须新增事件扩展文件，也应围绕 `internal/llm/contract.go` 做补充，而不是重新定义另一套长期并存的 `Event/EventType` 主契约。

### 推荐实现方式

#### 第一阶段

先复用或扩展现有：

```go
type StreamEvent = llm.StreamEvent
```

或为其提供 provider 侧适配器，再新增统一流接口，例如：

```go
type StreamingClient interface {
    Stream(ctx context.Context, req Request) (<-chan llm.StreamEvent, error)
}
```

同时保留旧接口 `StreamMessage(ctx, req, onDelta)`，通过适配层把 `text_delta` 事件转发给旧回调。

#### 第二阶段

对 OpenAI：

- chunk 解析后先生成标准事件
- 再由 normalizer / assembler 组装最终 `llm.Message`

对 Anthropic：

- 优先补真实 streaming API
- 如果短期无法完整接入，也要通过 normalizer 输出完整事件序列：`start -> delta/usage -> result`
- 即使底层是一次性响应，也不能让上层再看到 provider-specific 差异

### 公共字段建议

每个事件注入：

- `event_id`
- `trace_id`
- `provider_id`
- `model_id`
- `sequence`
- `timestamp` 可选，仅内部观测使用

### 产出验收

- OpenAI/Anthropic 都能输出统一事件流
- Runner 不再需要 provider-specific streaming 判断
- 事件契约只有单一归属层
- 事件顺序不变量有测试覆盖

---

## Step 7：引入健康检查与状态机治理

### 目标

让 provider 不仅能“被调用”，还能“被治理”，并能稳定驱动 router 的候选过滤。

### 建议新增文件

- `internal/provider/health.go`
- `internal/provider/health_store.go`
- `internal/provider/health_scheduler.go`
- `internal/provider/health_test.go`

### 状态机建议

状态集按 PRD：

- `healthy`
- `degraded`
- `unavailable`
- `half_open`

### 推荐实现策略

#### health_store

负责记录：

- 最近失败窗口
- 最近成功窗口
- 当前状态
- 下次 probe 时间

建议先采用内存实现，不急着持久化。

#### health_scheduler

负责后台探测，但必须区分“主动探测失败”和“真实业务失败”：

- 主动探测：低频、轻量 endpoint，优先复用/扩展现有 preflight 思路：`internal/provider/preflight.go:25-99`
- 被动探测：统计真实业务请求失败结果
- 不建议直接把 `ListModels` 当高频健康探测主手段，避免成本和限流风险

#### 被动治理

除了主动探测，还应在真实请求结果上更新状态：

- timeout / unavailable / 5xx 计入失败
- success 计入恢复成功
- unauthorized / bad_request 默认不计入基础可用性失败窗，可按配置决定

### 与 router 的联动

Router 获取候选时必须读取 health snapshot：

- `unavailable` 默认剔除
- `degraded` 可降权但不剔除
- `half_open` 默认只允许小流量试探，初版可先排在 fallback 尾部

### 配置建议

在 `config` 新增 health 配置：

- `check_interval_sec`
- `fail_threshold`
- `recover_probe_sec`
- `recover_success_threshold`
- 可选 `window_size`

### 产出验收

- 健康状态可被观测
- 主动探测与被动失败统计口径分离
- 状态迁移符合半开恢复语义
- router 选择受健康状态驱动

---

## Step 8：改造请求边界与 Runner 接入方式

### 目标

把 Provider 新能力接入 Agent 主调用链，但仍然维持 context/agent/provider 的职责边界稳定。

### 当前边界

- 请求组装实际发生在 `internal/agent/turn_processing.go:38-44`
- 对话循环在 runner：`internal/agent/runner.go:132`
- 流式/非流式 completion 入口在 `internal/agent/completion_runtime.go:14-46`
- provider 只负责模型调用

这个分层基本正确，应继续保持。

### 建议改造点

#### 在 Runner 中引入 provider facade，而不是直接依赖具体 client

当前 `Options.Client llm.Client`：`internal/agent/runner.go:40`。

建议短期不改字段类型，而是让传入的实现从“单 provider client”替换成“路由客户端 RoutedClient”。

这样 Runner 几乎不需要大改。

#### RouteContext 来源独立于 `llm.ChatRequest`

建议不要把 RouteContext 塞进 `llm.ChatRequest`，而是：

- 在 Runner 内部单独构造 route metadata
- 或新增 provider facade 专用请求包装结构

避免让 `llm.ChatRequest` 变成 provider 策略承载体。

#### 硬上限校验位置

PRD 要求 provider 仅做模型硬上限校验。

结合当前结构，推荐：

- context 继续负责软预算与能力裁剪
- provider 在真正发送请求前做 provider/model-specific 最大 token 限制校验
- 若超限，直接返回 `bad_request` 或 `context_too_long` 的 provider 错误

### 产出验收

- Runner 不直接感知 provider 差异
- context 不承接 provider 路由策略
- provider 不回流预算逻辑和持久化逻辑

---

## Step 9：测试矩阵与门禁建设

### 目标

把 Provider 从“能跑”升级到“契约破坏可被及时拦截”。

### 建议新增测试类型

#### 契约测试

新增：

- `internal/provider/contract_test.go`

覆盖：

- 每个 stream 首事件必须是 start
- `result/error` 互斥
- 终态之后无事件
- usage 事件规则
- 公共字段非空
- retryable 规则固定
- `context_too_long` 到 Runner 压缩链路的兼容性

#### Registry / Router 单测

覆盖：

- 重复注册
- 并发注册/查询
- 部分成功模型聚合
- fallback 顺序稳定
- health 状态影响候选选择
- 不发生双重 fallback

#### Provider 适配测试

基于现有测试风格补充：

- `internal/provider/openai_test.go`
- `internal/provider/anthropic_test.go`

重点不是只测 JSON 解析，而是测：

- 事件规范化
- 错误映射
- usage/cost 提取
- 非法 chunk 终止

#### Runner 集成测试

验证：

- RoutedClient 接入后 tool loop 不回归
- fallback 时最终响应仍满足 runner 预期
- 上层无 provider-specific 分支
- 空流回退与 provider fallback 不形成双重重试

### CI 门禁建议

至少纳入：

- `go test ./internal/provider/... ./internal/agent/... ./internal/context/... -v`
- 契约测试必须是阻断项

如果未来 provider 扩展增多，可把 contract test 设计成 provider adapter 必须复用的一套通用测试夹具。

---

## 5. 建议的落地顺序

为了降低回归风险，建议按下面顺序推进，而不是按 PRD 字面并行铺开：

1. 现状基线文档校正 + 单一事件契约归属决策
2. 契约与类型层：`interfaces/types/compat`
3. Registry 与多配置装配
4. Router + RoutedClient 兼容接入，并收敛 fallback 决策到一处
5. 错误映射闭环，与 `context_too_long` 兼容测试同批落地
6. 健康状态与 router 联动
7. 流式事件规范化
8. Anthropic 真流补齐
9. 契约测试、故障注入、CI 门禁

这样安排的原因是：

- 先纠正文档和契约归属，避免设计基线跑偏
- 路由和错误闭环先落地，能先解决多 provider 实用性问题
- 健康治理依赖 router，放在其后更合适
- 流式规范化是改动面最大的一步，放在中后段更稳妥
- Anthropic 真流是收益高但风险也高的工作，适合在统一事件层确定后补齐

---

## 6. 建议的目录调整方案

建议最终把 `internal/provider` 整理为如下结构：

```text
internal/provider/
  interfaces.go
  types.go
  compat.go
  factory.go
  registry.go
  models.go
  router.go
  router_policy.go
  errors.go
  error_map_openai.go
  error_map_anthropic.go
  normalize.go
  health.go
  health_store.go
  health_scheduler.go
  openai.go
  openai_stream.go
  anthropic.go
  anthropic_stream.go
  preflight.go
```

说明：

- `openai.go` / `anthropic.go` 保留请求协议适配职责
- 路由、健康、错误从协议适配代码中抽离
- 事件规范化优先复用 `internal/llm/contract.go`，不建议在 `internal/provider` 再沉淀第二套主事件模型
- `factory.go` 只负责装配，不再承载策略

---

## 7. 风险点与规避建议

### 7.1 最大风险：文档基线与真实代码脱节

如果方案继续引用不存在的 `internal/context/request.go`，后续实施边界会直接跑偏。

建议：所有设计说明统一锚定 `internal/agent/turn_processing.go:38-44`、`internal/agent/completion_runtime.go:14-46`、`internal/llm/contract.go:13-31`。

### 7.2 第二风险：事件模型与现有消息模型重复建设

如果在 provider 新建一整套 message/request/result/event 类型，会导致 `llm` 与 `provider` 双轨并存，维护成本上升。

建议：通用载体复用 `internal/llm`，事件契约也必须单点归属。

### 7.3 第三风险：配置迁移破坏现有用户体验

当前 README 和 `config.example.json` 仍是单 provider 模式：`README.md:53-94`、`config.example.json:1-28`。

建议：

- 保持旧配置可直接运行
- 新配置作为增强模式引入
- 在代码改造前先补“兼容模式 + 新模式”配置说明，降低迁移沟通成本
- 多 provider 配置必须保留 openai-compatible 扩展字段能力

### 7.4 第四风险：fallback 决策权分散

若 Runner 和 RoutedClient 同时做 fallback，会出现双重重试和统计口径污染。

建议：provider 级 fallback 只保留一处，Runner 的兼容回退需要明确约束或下沉。

### 7.5 第五风险：Anthropic 流式实现复杂度被低估

当前 Anthropic 是非真流实现：`internal/provider/anthropic.go:147`。

建议先通过统一事件适配保证上层一致性，再补底层真流，不要把这两件事绑死在一个迭代里。

---

## 8. 最终建议的交付物清单

本轮 provider 迭代最终建议至少交付以下内容：

### 代码层

- Provider 新契约层
- Registry / Models 聚合
- Router / fallback
- RoutedClient 兼容实现
- 错误码闭环与 retryable 规则
- 健康状态机
- 统一流式事件规范化层

### 配置层

- 多 provider 配置结构
- routing 配置
- health 配置
- 旧配置兼容映射
- openai-compatible 扩展字段完整继承

### 测试层

- Registry/Router/Health 单测
- OpenAI/Anthropic 适配测试
- Contract test
- Runner 集成测试
- `context_too_long` 兼容回归测试

### 文档层

- provider 迭代方案文档
- README 配置更新
- 配置示例更新
- provider contract 说明文档（可选）
- 一页“现状基线文档”（真实文件路径、真实调用链、真实接口）

---

## 9. 结论

结合当前项目现状，Provider 模块最合理的迭代路径不是直接重写，而是以 `internal/provider` 为中心，按“先校正基线与契约归属 -> Registry/Router -> fallback 收敛 -> 错误与健康 -> 事件规范化 -> 上层平滑接入”的方式渐进升级。

这样既能满足 PRD 对统一抽象、fallback、错误语义、健康治理、事件规范化的目标，也能最大程度复用当前 ByteMind 已有的 `llm`、`agent`、`context` 基线，降低对现有 CLI 主链路的冲击。
