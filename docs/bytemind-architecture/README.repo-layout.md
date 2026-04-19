# bytemind 架构设计文档（目录结构对齐版）

> 说明：本文件是新增文档，不替换原有 `README.md`。  
> 目标是“保留原架构设计”，但按当前仓库目录结构组织说明。

## 1. 文档目标

- 保持原架构设计原则与模块边界不变。
- 采用当前仓库目录作为主视角，降低阅读和落地成本。
- 让“设计模块”与“代码目录”建立一一映射关系。

## 2. 设计不变项（继承原架构）

- 单入口交互（CLI/TUI）。
- 主闭环：理解任务 -> 调工具 -> 写代码/执行验证 -> 返回结果。
- 安全优先：`allow/deny/ask`、高风险确认、路径与命令约束。
- 可恢复与可追踪：会话/任务/审计事件可回放。
- 工具统一契约与事件流输出。

## 3. 目录分层视图（按仓库）

```text
bytemind/
  cmd/
    bytemind/                # 入口与装配
  internal/
    tui/                     # 交互层
    agent/                   # 主闭环编排层
    plan/                    # 计划模型
    llm/                     # 统一 LLM 协议层
    provider/                # 模型供应商适配层
    tools/                   # 工具执行平面
    session/                 # 会话状态层
    skills/                  # 技能能力层（扩展子集）
    config/                  # 配置与目录管理
    mention/                 # 输入补全与索引
    history/                 # Prompt 历史
    assets/                  # 多模态资产存储（图片等）
    tokenusage/              # Token 统计与存储
```

## 4. 设计模块与目录映射

| 设计模块 | 当前目录落点 | 说明 |
| --- | --- | --- |
| `app` | `cmd/bytemind` + `internal/config` | 启动、装配、生命周期管理 |
| `agent` | `internal/agent` | 主闭环编排、工具调用协调 |
| `session` | `internal/session` | 会话语义状态、快照与恢复 |
| `context` | `internal/agent`（prompt 组装相关） | 当前以内嵌实现为主 |
| `provider` | `internal/provider` + `internal/llm` | 供应商适配与统一消息协议 |
| `tools` | `internal/tools` | 工具注册、校验、执行与事件流 |
| `extensions` | `internal/skills`（当前子集） | 现阶段主要是 Skills，MCP/插件可归入同层 |
| `storage` | `internal/session` + `internal/history` + `internal/assets` + `internal/tokenusage` | 当前为多存储组件协作 |
| `runtime` | `internal/agent`（执行循环中） | 任务调度能力当前主要由编排层承载 |
| `policy` | `internal/tools` + `internal/agent`（审批/约束逻辑） | 当前为分布式实现 |

## 5. 按目录说明职责与边界

### 5.1 `cmd/bytemind`（入口与装配）

做什么：

- 解析命令与参数。
- 初始化配置、会话、工具、模型客户端与 Runner。
- 启动 CLI 或 TUI。

不做什么：

- 不承载业务闭环逻辑。
- 不实现具体工具或 Provider 协议转换。

### 5.2 `internal/tui`（交互层）

做什么：

- 维护 UI 状态与用户交互流程。
- 消费 `agent` 事件并渲染。
- 处理输入、图片粘贴、历史检索、会话切换。

不做什么：

- 不直接承担模型调用编排。
- 不替代工具执行层。

### 5.3 `internal/agent`（主闭环编排层）

做什么：

- 读取会话状态并组装 Prompt。
- 发起模型调用并处理流式事件。
- 协调工具调用结果回填与迭代终止条件。

不做什么：

- 不实现工具底层动作。
- 不绑定具体 Provider 请求格式。

### 5.4 `internal/provider` + `internal/llm`（模型协议层）

做什么：

- 屏蔽供应商差异，统一消息与工具调用协议。
- 提供流式响应语义。

不做什么：

- 不负责会话持久化。
- 不直接做业务级权限判定。

### 5.5 `internal/tools`（工具执行平面）

做什么：

- 维护统一 `Tool` 契约与注册表。
- 参数校验与执行调度。
- 标准事件流输出（`start/chunk/result/error`）。

不做什么：

- 不做对话主循环编排。
- 不承担 UI 状态管理。

### 5.6 `internal/session`（会话状态层）

做什么：

- 会话创建、恢复、消息追加、模式切换。
- 会话持久化和加载。

不做什么：

- 不做模型协议转换。
- 不执行工具动作。

### 5.7 `internal/skills`（扩展能力层）

做什么：

- 技能发现、解析、索引与选择。
- 向编排层提供技能上下文与约束。

不做什么：

- 不直接替代工具执行层。
- 不接管主闭环控制。

补充：

- 在本目录结构视角下，`skills` 可视作 `extensions` 的当前实现子集。
- 后续接入 MCP/插件时，建议继续归入同一“扩展能力层”。

### 5.8 `internal/history` / `assets` / `tokenusage`（支撑存储层）

做什么：

- `history`：Prompt 历史。
- `assets`：多模态资产落盘（图片等）。
- `tokenusage`：Token 使用统计与存储。

不做什么：

- 不做业务编排。
- 不替代 `session` 的语义状态管理。

## 6. 核心链路（目录视角）

1. `cmd/bytemind` 初始化运行上下文。  
2. `internal/tui` 或 CLI 收集用户输入。  
3. `internal/agent` 读取 `internal/session` 状态并组装上下文。  
4. `internal/agent` 通过 `internal/provider`/`internal/llm` 调用模型。  
5. 模型触发工具调用时进入 `internal/tools`。  
6. 结果回写 `internal/session`，并同步支撑层（`history/assets/tokenusage`）。  
7. `internal/tui` 按事件流更新展示。  

## 7. Skill 与 Extension（目录对齐结论）

- 设计上：技能属于扩展能力域（`extensions`）。
- 目录上：当前由 `internal/skills` 实现并被 `agent` 消费。
- 文档表达：保留“extensions 设计概念”，同时明确“当前落点在 `internal/skills`”。

## 8. 边界约束（目录版）

- `cmd/bytemind` 仅做装配，不承载闭环业务逻辑。
- `internal/agent` 通过接口消费 `session/provider/tools/skills` 能力，不下沉具体实现细节。
- `internal/tools` 保持统一契约，避免分散执行入口。
- `internal/provider` 不承载会话和工具编排职责。
- `internal/session` 保持会话语义状态 owner。

## 9. 测试关注点（目录版）

- `internal/agent`：主闭环、工具迭代、模式分支。
- `internal/tools`：schema 校验、超时/取消/重试语义。
- `internal/provider` + `internal/llm`：协议一致性与流式边界。
- `internal/session`：持久化、回放、恢复一致性。
- `internal/tui`：事件驱动状态更新与关键交互行为。

## 10. 结论

这份目录结构对齐版文档保持了原架构设计的目标与边界，但把表达方式切换为“仓库目录优先”。  
这样既不牺牲设计约束，也能直接指导当前代码结构下的开发与评审。
