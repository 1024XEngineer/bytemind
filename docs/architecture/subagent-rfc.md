# Subagent 架构设计 RFC

## 背景

当前仓库已经具备一套可用于 subagent 的 runtime-task 基础设施，但还没有形成真正的产品级 subagent 系统。

当前代码中已经可以确认的事实：

- 已有 runtime task 原语：`TaskManager`、父子任务、取消传播、事件流、日志、等待机制以及持久化桥接。
- 已有一个最小化的 `SubAgentCoordinator`，但其职责仅限于 `Spawn/Wait + quota`。
- 主产品链路还没有把 subagent 真正接入到 runner、prompt、UI 或可观测性链路中。

相关代码位置：

- `internal/runtime/subagent.go:10`
- `internal/runtime/manager.go:74`
- `internal/agent/runner.go:49`
- `internal/agent/runtime_gateway.go:17`
- `internal/app/bootstrap.go:132`

## 阶段判断

当前仓库应被定义为：

**Stage 1.5 - Runtime Ready / Product Not Wired**

可以采用如下四阶段模型理解：

- `Stage 0`：没有 runtime，也没有 delegation
- `Stage 1`：已有 task runtime 原语
- `Stage 2`：已有 subagent 编排，但只支持显式调用
- `Stage 3`：已有受控自动委派
- `Stage 4`：已有可配置 agent 生态、可观测性与稳定策略系统

当前仓库处于 `Stage 1.5` 的原因：

- 已经具备 subagent 所需的执行内核。
- 已经具备 quota 和父子任务语义。
- 但还没有把 `SubAgentCoordinator` 接入主执行链路。
- 也没有 `AgentSpec`、delegation contract、结构化结果协议，或用户可见的 subagent 生命周期。

## 目标

- 在不替换现有 runtime 的前提下，构建本地会话内的 subagent 能力。
- 让主 agent 能将探索、评审、局部实现等任务委派给专用子 agent。
- 让子 agent 具备独立上下文窗口、独立工具限制和独立模型配置。
- 保持当前 prompt 架构、mode、skills、tools、runtime 设计整体稳定。
- 优先落地一个最小、可控、端到端闭环的 subagent 方案。

## 非目标

- 第一阶段不实现无限递归 subagent。
- 第一阶段不实现 subagent 之间的直接通信协议。
- 第一阶段不实现远程分布式多 agent 集群。
- 第一阶段不重写现有 runner 或 runtime 内核。
- 不把 `skill` 和 `agent` 混成一个概念。

## 设计原则

- **复用现有 runtime**
  - 所有 subagent 执行继续运行在 `TaskManager` 之上。
- **补齐产品层，而不是重写内核**
  - 在 runtime 上方增加 orchestration 与 agent-definition 两层。
- **先显式，后自动**
  - 先支持显式 delegation，再逐步放开自动 delegation。
- **先只读探索，再可写执行**
  - 优先落地 `explore`，再扩展 `general`。
- **默认强约束**
  - 默认限制深度、并发、工具集和权限升级。
- **优先结构化结果**
  - 子 agent 输出既要便于人读，也要便于系统消费。

## 参考方向与推荐产品形态

最适合当前仓库的方向是：**Claude Code / OpenCode 风格的受控本地 subagent 系统**，并继续复用当前 Go runtime 作为执行底座。

原因如下：

- 当前仓库已经具备本地 CLI/TUI 执行、prompt 组装、tool registry、runtime task 等能力。
- 这些能力天然适合支撑“本地会话内子代理”。
- 如果转向更重的 Codex 式远程任务中心路线，会明显偏离当前架构，并增加系统复杂度。

因此推荐策略为：

- 在**产品形态**上参考 Claude Code / OpenCode。
- 在**执行内核**上继续使用当前 `runtime`。
- 在中间增加清晰的 **agent definition + delegation orchestration** 层。

## 目标架构

推荐采用四层结构：

- **Layer 1: Agent Definition**
  - 定义 agent 是什么，包括 prompt、tools、model、permissions、description。
- **Layer 2: Delegation / Orchestration**
  - 决定何时委派、调用哪个 agent、如何施加限制、如何回收结果。
- **Layer 3: Execution Runtime**
  - 复用 `TaskManager`、事件流、父子任务关系、日志与 quota。
- **Layer 4: UX / Observability**
  - 将子任务生命周期和摘要暴露给 CLI/TUI 和存储层。

## 核心概念模型

建议引入以下概念：

- `Mode`
  - 继续表示主会话行为模式，例如 `build` 和 `plan`。
- `Skill`
  - 继续表示可复用的工作方法或任务模板。
- `AgentSpec`
  - 定义一个 primary agent 或 subagent。
- `SubAgentInvocation`
  - 表示一次委派出来的子 agent 执行。
- `SubAgentResult`
  - 表示结构化的子 agent 输出。
- `DelegationPolicy`
  - 控制谁可以在什么条件下委派给谁。

## 推荐内建 Subagent

建议内建三类 subagent，但按阶段启用：

- `explore`
  - 只读
  - 快速/低成本模型
  - 用于搜索、发现和代码库理解
- `review`
  - 只读
  - 用于代码评审、测试建议和回归风险分析
- `general`
  - 可读写
  - 继承主模型或使用更强模型
  - 用于复杂但可隔离的局部任务

推荐启用顺序：

- `M1` 仅启用 `explore` + `review`（先只读）。
- `M2` 再引入 `general`（受控可写）。

## 主执行流程

推荐高层执行流程如下：

1. 主 agent 接收用户请求并建立 turn 执行上下文。
2. runner 为该 turn 提供 `ParentTaskID`（显式传入或在 turn 入口创建）。
3. 主 agent 判断是否需要委派。
4. 若需要，则调用 `SubAgentService`。
5. `SubAgentService` 解析 `AgentSpec + DelegationPolicy + Quota`，并创建 `InvocationID`。
6. 在现有 runtime 中创建并执行一个子任务；提交成功后建立 `InvocationID <-> TaskID` 的一对一绑定。
7. 子任务使用自己的 prompt、tool filter、model settings 和约束运行。
8. 子任务结束后产出一个 `SubAgentResult`。
9. 主 agent 消费该结构化结果，并决定是：
   - 直接回答用户，
   - 继续执行工具，
   - 还是继续发起其他委派。

## Invocation 与 Task 一致性约束

为避免生命周期分裂，建议在实现层明确以下强约束：

- 一个 `Invocation` 在成功提交后必须绑定且仅绑定一个 `TaskID`。
- `Wait/Stream/Cancel` 先解析 `InvocationID -> TaskID`，再委托给 `TaskManager`。
- terminal 状态以 runtime task 为真相源，`Invocation` 仅做语义补充与展示聚合。
- 持久化恢复时若 `Invocation` 存在但 task 丢失，应进入 `failed` 并携带明确 `error_code`（如 `task_not_found`）。

## 自动委派策略

第一阶段不应该把自动委派完全交给模型自由决定。

推荐策略：

- 只有主 agent 可以委派。
- subagent 默认不允许再 spawn subagent。
- 最大 delegation depth 固定为 `1`。
- 最大并发建议限制在 `2~3`。
- `plan` mode 只能委派给只读 agent。
- `build` mode 在 `M1` 仅委派给只读 agent（`explore`/`review`），从 `M2` 开始可委派 `general`。
- 仅当满足以下条件之一时才允许自动委派：
  - 搜索范围很大；
  - 任务会向主上下文灌入大量日志或文件内容；
  - 工作可被清晰隔离并定义交付物；
  - 专用 reviewer / explorer 明显更有价值。

## 上下文模型

每个 subagent 都应拥有独立上下文窗口。

传给 subagent 的输入上下文应包含：

- 父任务摘要
- 委派目标与成功标准
- 必需文件列表
- 当前 mode 和审批上下文
- 子 agent 自身 system prompt
- 可用工具与权限限制
- 可选的 skill block
- 经过筛选的相关历史摘要，而不是完整父对话

默认不应透传完整父会话。

推荐传递的输入形式：

- `task brief`
- `relevant files`
- `constraints`
- `expected output schema`

## 权限模型

子 agent 的权限绝不能超过父 agent。

推荐权限收敛公式：

- `effective tools = parent tools ∩ agent allowed tools - agent denied tools`
- `effective approval <= parent approval looseness`
- 既有 mode 限制继续生效

具体默认值建议：

- `explore`：禁止文件写入和会产生变更的 shell 命令
- `review`：禁止文件写入和会产生变更的 shell 命令
- `general`：允许受控写入与 shell，但仍受父审批边界约束

## 结果协议

subagent 输出不应继续只是一个不透明的 `[]byte`。

推荐结构化结果字段：

- `summary`
- `status`
- `error_code`
- `artifacts`
- `patch_summary`
- `findings`
- `open_questions`
- `token_usage`
- `duration_ms`

建议最小 schema：

```go
type SubAgentResult struct {
    InvocationID  string
    AgentName     string
    Status        string
    ErrorCode     string
    Summary       string
    OutputText    string
    PatchSummary  string
    Findings      []SubAgentFinding
    Artifacts     []SubAgentArtifact
    OpenQuestions []string
    TokenUsage    *SubAgentTokenUsage
    DurationMS    int64
}
```

## Invocation 状态机

建议对齐 runtime 的任务状态，统一使用以下状态：

- `pending`
- `running`
- `completed`
- `failed`
- `killed`

可选：`queued` 仅作为 runtime 提交前的瞬时编排态，不作为持久化终态。

与 runtime 的映射关系：

- `pending/running/completed/failed/killed` 与 runtime task 状态一一对应
- 超时使用：`status=failed` + `error_code=task_timeout`
- 取消使用：`status=killed` + `error_code=task_cancelled`

## 可观测性

现有 task event stream 可以复用，但需要补强语义层。

建议新增以下 task metadata：

- `agent_name`
- `agent_mode`
- `invocation_kind=subagent`
- `runtime_task_id`
- `parent_turn_id`
- `delegated_by`
- `delegation_depth`

CLI/TUI 至少应展示：

- subagent 开始
- subagent 完成或失败
- subagent 摘要
- error code（若失败）
- token 用量与 duration

## 错误处理

错误建议按三层处理：

- **delegation error**
  - 如 agent 不存在、策略拒绝、深度超限、并发超限
- **runtime error**
  - 如 quota、timeout、cancellation（分别映射到统一 `status + error_code`）
- **task result error**
  - 如子 agent 正常结束，但输出失败结论

主 agent 的处理策略：

- 能降级为 inline 处理时则降级
- 无法降级时，明确向用户报告 blocker

## 与当前仓库的映射关系

当前已经可复用的组件：

- `internal/runtime/manager.go:74` 中的 `TaskManager` 及父子任务支持
- `internal/runtime/subagent.go:26` 中的 quota 思路
- `internal/agent/prompt_test.go:12` 中体现的 prompt 组装顺序与 mode 模型
- `internal/tools/registry.go:69` 中的 tool registry 与 mode filters
- `internal/app/bootstrap.go:34` 中的统一装配点

当前明显缺口：

- 缺少 `AgentSpecRegistry`
- 缺少 `SubAgentService`
- 缺少 `SubAgentResult` schema
- 缺少 runner 层的 subagent 执行上下文组装能力
- 缺少 turn 级 `ParentTaskID` 打通与子任务绑定约束
- 缺少 `InvocationID <-> TaskID` 一致性与恢复策略
- 缺少用户可见的 invocation 事件

## 接口草案

建议新增如下接口：

```go
type AgentSpecRegistry interface {
    Get(ctx context.Context, name string) (AgentSpec, error)
    List(ctx context.Context) ([]AgentSpec, error)
}

type SubAgentService interface {
    Invoke(ctx context.Context, req SubAgentInvokeRequest) (SubAgentInvocationHandle, error)
    Wait(ctx context.Context, invocationID string) (SubAgentResult, error)
    Stream(ctx context.Context, invocationID string) (<-chan SubAgentEvent, error)
    Cancel(ctx context.Context, invocationID string, reason string) error
}

type DelegationPolicy interface {
    CanInvoke(ctx context.Context, req SubAgentInvokeRequest) error
    MaxDepth() int
    MaxParallel(sessionID string) int
}

type AgentExecutorFactory interface {
    NewSubAgentRunner(ctx context.Context, spec AgentSpec, parent ParentExecutionContext) (SubAgentRunner, error)
}

type SubAgentRunner interface {
    Run(ctx context.Context, input SubAgentExecutionInput) (SubAgentResult, error)
}
```

## 数据结构草案

建议的关键类型如下：

```go
type AgentSpec struct {
    Name           string
    Description    string
    Mode           AgentMode
    Model          string
    SystemPrompt   string
    ToolPolicy     ToolPolicy
    AllowedTools   []string
    DeniedTools    []string
    Hidden         bool
    ReadOnly       bool
    MaxSteps       int
    PermissionMode string
    SkillRefs      []string
    Tags           []string
}

type SubAgentInvokeRequest struct {
    SessionID       string
    TraceID         string
    ParentTaskID    string // required: 用于父子任务取消传播与审计关联
    ParentTurnID    string
    AgentName       string
    Goal            string
    SuccessCriteria []string
    RelevantFiles   []string
    InputSummary    string
    Metadata        map[string]string
    Timeout         time.Duration
    Background      bool
}

type SubAgentInvocationHandle struct {
    InvocationID string
    TaskID       string // 与 InvocationID 一对一绑定
    AgentName    string
}

type SubAgentEvent struct {
    InvocationID string
    AgentName    string
    Type         string
    Message      string
    Timestamp    time.Time
    Metadata     map[string]string
}

type SubAgentFinding struct {
    Severity string
    Title    string
    Summary  string
    Evidence []string
}

type SubAgentArtifact struct {
    Kind string
    Path string
    Note string
}

type SubAgentTokenUsage struct {
    InputTokens  int64
    OutputTokens int64
    TotalTokens  int64
}
```

## 目录改造草案

推荐尽量贴合当前仓库布局，以新增为主而不是重排：

```text
internal/
  agent/
    runner.go
    runtime_gateway.go
    prompt.go
    subagent_runner.go
    subagent_prompt.go
    subagent_result.go
  agents/
    registry.go
    loader.go
    types.go
    builtin/
      explore.md
      general.md
      review.md
  orchestration/
    subagent_service.go
    delegation_policy.go
    invocation_store.go
  runtime/
    manager.go
    subagent.go
    invocation.go
  storage/
    task_store.go
    subagent_store.go
  app/
    bootstrap.go
```

建议职责划分如下：

- `internal/agents`
  - 定义 agent 是什么
- `internal/orchestration`
  - 决定何时、如何调用 agent
- `internal/agent`
  - 负责实际运行一个 agent 实例
- `internal/runtime`
  - 负责任务调度与收敛
- `internal/storage`
  - 负责持久化 invocation 与结果视图

## 与现有概念的关系

### Skills

- `skills` 不应被 `agents` 替代
- 推荐关系：一个 `agent` 可以引用一个或多个 `skills`
- agent 是执行身份，skill 是可复用的工作方法指导

### Modes

- `mode` 仍应是主会话的主要控制面
- `build` 和 `plan` 可以继续扮演 primary-agent 形态

### Prompt

- 主 agent 的 system prompt 继续由 `internal/agent/prompt.go` 统一组装：
  1. `prompts/default.md`
  2. `prompts/mode/{build|plan}.md`
  3. runtime context block
  4. optional active skill block
  5. `AGENTS.md` instruction block
- `internal/agents/builtin` 仅承载子 agent 的定义与提示词（如 `explore/general/review`），避免复制 `build/plan` 主提示词形成双源漂移。

### Tools

- tools 仍由现有 registry 统一注册
- agents 只负责过滤和权限收敛

## Bootstrap 装配草案

未来的 bootstrap 装配关系建议为：

```text
Bootstrap
  -> TaskManager
  -> AgentSpecRegistry
  -> DelegationPolicy
  -> SubAgentService
  -> Runner
```

`Runner` 在处理 turn 时可使用 `SubAgentService`。
第一阶段仍建议优先显式 delegation，而不是直接引入自动 delegation。

## 分阶段落地路线

### M1：最小可用版本

- `AgentSpecRegistry`
- `SubAgentService`
- 内建 `explore` 与 `review`（只读）
- 仅支持显式调用
- 引入结构化结果协议
- 打通 turn 级 `ParentTaskID` 透传
- 落地 `InvocationID <-> TaskID` 一致性约束

### M2：受控自动委派

- 在 prompt 和 runner 流程中加入 delegation contract
- 仅允许主 agent 委派
- 深度限制为 `1`
- 并发限制为 `2~3`
- 引入 `general`（受控可写）

### M3：可观测性

- 在 CLI/TUI 中显示 invocation timeline
- 在 task 持久化中记录结构化 subagent metadata
- 可见 token 用量与 duration

### M4：项目级自定义 Agents

- 支持项目级 agent 定义
- 与现有 skill discovery 方式联动

## 验收与测试建议

建议在每个里程碑同步补齐最小测试集：

- runtime 状态映射测试：验证 `pending/running/completed/failed/killed` 与 `error_code`（`task_timeout`、`task_cancelled`）组合语义。
- invocation 一致性测试：验证 `InvocationID <-> TaskID` 一对一绑定、恢复场景与 `task_not_found` 降级行为。
- 取消传播测试：验证 `ParentTaskID` 打通后，父任务取消可正确传播到子任务。
- prompt 组装测试：验证主 prompt 顺序保持 `default -> mode -> runtime context -> active skill -> AGENTS.md`。
- 可观测性测试：验证 task metadata 中 `invocation_kind`、`runtime_task_id`、`delegation_depth` 等字段完整落盘与展示。

## 最终建议

该仓库应演进到：

**一种受 Claude Code / OpenCode 启发、运行在现有 runtime task 系统之上的受控本地 subagent 架构。**

建议战略方向：

- 保持当前 `runtime` 作为执行内核
- 增加 `AgentSpec + Orchestration + Structured Result` 这一缺失的中间层
- 先交付显式 subagent 调用
- 再引入受控自动 delegation
- 优先级顺序为：`explore` 第一、`review` 第二、`general` 第三

## 总结

当前仓库拥有的是：**subagent runtime 原语，而不是 subagent 产品本身。**

最佳下一步方案是：

- **产品形态**：采用 Claude Code / OpenCode 风格的受控 subagent
- **执行模型**：复用现有 task runtime
- **实施策略**：先显式、后自动；先只读、后可写；从第一天开始使用结构化结果协议
