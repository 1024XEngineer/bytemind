## 0. 文档信息

- 产品：ByteMind
- 模块：Provider（`internal/provider`）
- 文档日期：2026-04-15
- 适用代码基线：`main`
- 关联架构基线：PR架构设计 #162
- 目标读者：产品、架构、研发、测试、运维

## 1. 背景与目标

当前 Provider 已支持多供应商接入，但实现仍偏“适配器集合”，上层仍需感知 provider 差异，导致路由、流式处理、错误处理与治理策略分散。

本模块目标是构建统一模型供应商抽象层，提供稳定的：

1. 注册中心与模型目录聚合
2. 路由与 fallback 决策
3. 流式事件规范化
4. 错误语义统一（含 retryable）
5. 健康治理与状态驱动
6. usage/cost 元信息上报

最终使上层编排与具体 provider 解耦。

## 2. 架构边界

### 2.1 In Scope

1. provider 客户端注册与查询
2. 模型目录聚合与去重
3. provider/model 路由与降级候选输出
4. 统一流式事件语义与时序约束
5. 统一错误语义与 retryable 判定
6. 健康探测、状态机与隔离/恢复
7. usage/cost 标准字段透传与估算标注

### 2.2 Out of Scope

1. 会话持久化
2. 权限决策与风险规则
3. 工具编排
4. 审计落盘
5. 上下文预算策略（warning/critical/reactive）

### 2.3 预算边界（强约束）

1. 上下文预算计算与压缩归 `context` 模块
2. provider 仅做模型硬上限校验
3. provider 返回 usage/cost，不做预算决策

## 3. 当前基线与主要缺口

### 3.1 可复用基线

1. `internal/provider/factory.go`：OpenAI/OpenAI-compatible/Anthropic 装配
2. 现有流式调用链路
3. `internal/llm` 统一消息抽象
4. preflight 检查机制

### 3.2 缺口

1. Registry/Router 契约不够稳定，路由上下文弱类型
2. 流式事件在不同 provider 的顺序与字段一致性不足
3. 错误码闭环不完整，retryable 规则不够刚性
4. 健康治理缺少完整状态机（含 half_open）
5. 模型目录去重键不够严格，聚合失败语义不完善

## 4. 核心契约

```go
type Client interface {
    ProviderID() ProviderID
    ListModels(ctx context.Context) ([]ModelInfo, error)
    Stream(ctx context.Context, req Request) (<-chan Event, error)
}

type Registry interface {
    Register(ctx context.Context, client Client) error
    Get(ctx context.Context, id ProviderID) (Client, bool)
    List(ctx context.Context) ([]ProviderID, error)
}

type RouteContext struct {
    Scenario      string
    Region        string
    PreferLatency bool
    PreferLowCost bool
    AllowFallback bool
    Tags          map[string]string
}

type Router interface {
    Route(ctx context.Context, requestedModel ModelID, rc RouteContext) (RouteResult, error)
}

type HealthChecker interface {
    Check(ctx context.Context, id ProviderID) error
}
```

## 5. 统一错误码与 retryable 规则

```go
type ErrorCode string

const (
    ErrCodeUnauthorized      ErrorCode = "unauthorized"
    ErrCodeRateLimited       ErrorCode = "rate_limited"
    ErrCodeTimeout           ErrorCode = "timeout"
    ErrCodeUnavailable       ErrorCode = "unavailable"
    ErrCodeBadRequest        ErrorCode = "bad_request"
    ErrCodeProviderNotFound  ErrorCode = "provider_not_found"
    ErrCodeDuplicateProvider ErrorCode = "duplicate_provider"
)
```

retryable 固定规则：

1. `unauthorized=false`
2. `bad_request=false`
3. `duplicate_provider=false`
4. `provider_not_found=false`
5. `rate_limited=true`
6. `timeout=true`
7. `unavailable=true`

未知错误默认映射 `unavailable`，并保留原始错误上下文。

## 6. 流式事件契约（含顺序不变量）

```go
type EventType string

const (
    EventStart    EventType = "start"
    EventDelta    EventType = "delta"
    EventToolCall EventType = "tool_call"
    EventUsage    EventType = "usage"
    EventResult   EventType = "result"
    EventError    EventType = "error"
)
```

强约束：

1. 每个 stream 必须且仅有 1 个 `start`，且首事件必须是 `start`
2. `result` 与 `error` 互斥，二选一作为终止事件
3. 终止事件后不得再出现任何事件
4. `usage` 允许出现 `0..N` 次；若 provider 仅支持最终统计，结束前至少发送 1 次
5. 所有事件必须包含 `event_id`、`trace_id`、`provider_id`、`model_id`
6. 非法 chunk 必须转为 `EventError` 并终止流

## 7. 模型目录与去重规则

1. 内部唯一键必须是 `(provider_id, model_id)`，禁止仅按 `model_id` 去重
2. 可对外暴露 `display_alias`（可选），仅用于展示，不参与唯一性
3. `ListModels` 聚合支持部分成功：
   - 返回成功模型结果
   - 同时返回 `warnings`（失败 provider 列表与原因）

## 8. 路由与 fallback 决策顺序

路由流程固定为：

1. 用户指定模型直连命中
2. 过滤不可用候选（健康状态 + 模型上下限）
3. 按策略排序（场景优先级、延迟偏好、成本偏好、区域约束）
4. 输出 `primary + ordered fallbacks`

若无候选，返回 `unavailable`。

## 9. 健康治理状态机

状态集：`healthy`、`degraded`、`unavailable`、`half_open`。

迁移规则：

1. 连续失败达到 `fail_threshold`：`healthy/degraded -> unavailable`
2. 到达恢复探测窗口：`unavailable -> half_open`
3. `half_open` 连续成功达到 `recover_success_threshold`：`-> healthy`
4. `half_open` 任一失败：`-> unavailable`

治理要求：

1. 使用滑动窗口统计，控制短时抖动导致误熔断
2. 健康状态必须可稳定驱动 Router 的候选过滤与排序

## 10. usage/cost 契约

```go
type Usage struct {
    InputTokens  int64
    OutputTokens int64
    TotalTokens  int64
    Cost         float64
    Currency     string // e.g. USD
    IsEstimated  bool   // true=本地估算, false=provider返回
}
```

约束：

1. token 与货币单位必须明确
2. provider 未返回 usage/cost 时允许 `IsEstimated=true`，并记录估算来源
3. provider 仅负责上报，不做预算决策

## 11. 与 Context/Agent 边界

1. provider 仅执行模型硬上限校验（以 provider 原生 tokenizer 计数为准）
2. provider 不实现预算策略（warning/critical/reactive）
3. provider 不直接调用 storage、审计持久化接口
4. 预算策略字段误入 provider 时，需由编译或契约测试阻断

## 12. 功能需求（FR）

- `FR-PROV-001`：提供统一 `Client/Registry/Router/HealthChecker` 契约
- `FR-PROV-002`：支持 provider/model 路由与 fallback 输出
- `FR-PROV-003`：统一流式事件字段、终态约束与顺序不变量
- `FR-PROV-004`：统一错误码并输出稳定 `retryable`
- `FR-PROV-005`：支持健康检查、状态查询与状态机迁移
- `FR-PROV-006`：返回 usage/cost 标准元信息（含估算标记）

## 13. 实施步骤与验收口径

### Step 1：统一接口层

1. 落地 `interfaces.go`、`compat.go` 与 provider 适配
2. Router 入参统一为强类型 `RouteContext`
3. 验收：上层仅依赖统一接口，不依赖 provider-specific 类型

### Step 2：Registry 与模型目录

1. 落地 `registry.go`、`models.go`
2. 并发安全注册与查询
3. `ListModels` 支持部分成功与 warnings
4. 验收：重复注册返回 `duplicate_provider`，聚合结果可诊断

### Step 3：Router 与 fallback

1. 落地 `router.go`、`router_policy.go`
2. 严格执行候选过滤与排序流程
3. 输出稳定有序 fallback 列表
4. 验收：无候选统一返回 `unavailable`

### Step 4：流式事件规范化

1. 落地 `normalize.go`，改造 `openai_stream.go`、`anthropic_stream.go`
2. 强制注入 `event_id/trace_id/provider_id/model_id`
3. 保证 start/终态/后续事件约束
4. 验收：上层无 provider-specific 事件分支

### Step 5：错误映射与 retryable

1. 落地 `errors.go`、`error_map_openai.go`、`error_map_anthropic.go`
2. 覆盖 401/429/timeout/unavailable 映射
3. 未知错误统一到 `unavailable` 并保留上下文
4. 验收：上层可仅凭错误码与 retryable 完成处理

### Step 6：健康检查与状态机

1. 落地 `health.go`、`health_store.go`、`health_scheduler.go`
2. 实现 fail_threshold、half_open、recover_success_threshold
3. 引入滑动窗口与隔离恢复
4. 验收：健康状态可稳定驱动候选选择

### Step 7：Context/Agent 联调

1. 对齐 `request.go` 与 `agent/runner.go` 字段
2. 固化硬上限校验与 usage 透传
3. 通过边界契约测试阻断职责回流
4. 验收：provider 内无预算策略、无持久化逻辑

### Step 8：契约测试与故障注入门禁

1. 落地 `contract_test.go`、`e2e_test.go`
2. 必测故障：401/429/timeout/disconnect
3. 将契约一致性纳入 CI 阻断
4. 验收：契约破坏第一时间被 CI 拦截

## 14. 目标指标

1. 首次调用成功率：>= 97%
2. 不可用场景降级成功率：>= 85%
3. 流式事件契约一致率：100%
4. 健康状态误判率：<= 1%
5. 上层 provider-specific 分支数量下降：>= 70%

## 15. 配置模型（示例）

```yaml
provider:
  default_provider: openai
  default_model: gpt-5.4
  providers:
    openai:
      base_url: https://api.openai.com/v1
      api_key_ref: secret://workspace/openai_api_key
    anthropic:
      base_url: https://api.anthropic.com
      api_key_ref: secret://workspace/anthropic_api_key
  routing:
    fallback:
      gpt-5.4:
        - claude-sonnet-4.5
  health:
    check_interval_sec: 30
    fail_threshold: 3
    recover_probe_sec: 20
    recover_success_threshold: 2
```

说明：`api_key_ref` 由 `app/config` 解析，provider 仅消费解析结果。

## 16. 测试与验收方案

1. 单元测试：registry/router/health/normalizer/error mapping
2. 集成测试：OpenAI/Anthropic 统一事件语义与终态约束
3. 故障注入：401/429/timeout/disconnect/model unavailable
4. 压力测试：并发流式请求 TTFT 与吞吐
5. 合同测试：错误码闭环、retryable、事件顺序不变量、边界约束

## 17. 风险与回滚

1. 风险：错误映射不完整导致上层误判
   - 应对：保留原始错误上下文并持续补齐映射样本
2. 风险：健康探测过敏导致误熔断
   - 应对：阈值可配置 + 滑动窗口 + half_open 恢复
3. 风险：多 provider 事件差异导致回归
   - 应对：合同测试 + 回放测试 + CI 门禁

回滚策略：

1. 保留 legacy provider 直连开关
2. router/normalize/health 可独立关闭并回退最小路径
3. 紧急情况下回退到单 provider 固定模型运行

## 18. 最终验收标准（DoD）

1. 注册、路由、流式、错误语义、健康治理全部可用
2. 上层无 provider-specific 事件分支
3. 错误码闭环，retryable 可被上层直接消费
4. 健康状态可稳定驱动候选选择
5. provider 中无预算策略、无权限策略、无持久化逻辑
6. 契约破坏可被 CI 合同测试第一时间阻断

## 附录 A：与当前代码映射（首批改造点）

1. `internal/provider/factory.go`：并入统一 registry 装配
2. `internal/provider/openai*.go`：补齐标准事件与错误映射
3. `internal/provider/anthropic*.go`：补齐标准事件与错误映射
4. `internal/agent/runner.go`：只消费统一 provider 接口与事件
5. `internal/context/*`：继续负责预算计算与压缩策略
