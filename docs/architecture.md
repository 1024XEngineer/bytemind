# ByteMind 架构设计文档

## 1. 项目概述

ByteMind 是一个运行在本地终端中的 AI 编程助手（coding agent）。用户通过自然语言描述任务，系统调用 LLM 理解意图、操作文件、执行命令、搜索信息，在终端内完成"理解→修改→执行→验证→恢复"的工程闭环。

- **语言**: Go 1.25
- **UI**: Bubble Tea TUI (github.com/charmbracelet/bubbletea v1.3)
- **LLM**: 多 Provider 路由（Anthropic/OpenAI/Gemini），统一 OpenAI-compatible 协议

## 2. 完整用户故事走查

以下用一个典型用户故事走查所有模块协作关系。

### 场景：用户让 ByteMind 修复一个登录 Bug

```
用户: "pages/login.tsx 的登录按钮在移动端点击没反应，帮我修一下"
```

---

#### 阶段 1：启动与装配

```
用户执行: $ bytemind
  → cmd/bytemind/main.go
    → app.RunCLI(args, stdin, stdout, stderr)
      → cli_dispatch.go 解析子命令（默认 run）
        → BootstrapEntrypoint (entrypoint.go)
          → ResolveWorkspace: 获取当前目录绝对路径
          → Bootstrap (bootstrap.go):
              │
              ├─[1] LoadRuntimeConfig
              │     → config.Load(workspace)
              │       → Default()          ← 内置默认值（模型、模式、策略）
              │       → mergeConfigFromFile ← ~/.bytemind/config.json（用户级）
              │       → mergeConfigFromFile ← .bytemind/config.json（项目级）
              │       → applyEnv()         ← 环境变量覆盖
              │       → normalize()        ← 校验 + 标准化（Provider 类型、API Key、模式）
              │
              ├─[2] config.EnsureHomeLayout
              │     → mkdir ~/.bytemind/{sessions,logs,cache,auth}
              │
              ├─[3] session.NewStore("~/.bytemind/sessions")
              │
              ├─[4] 新建或恢复 Session
              │     → session.New(workspace) 或 store.Load(sessionID)
              │     → store.Save(sess) 确保持久化
              │
              ├─[5] provider.NewClientFromRuntime
              │     → NewRouterClient(providerRuntimeConfig)
              │       → NewRegistry(cfg)     ← 解析 providers{...} 配置
              │       → NewHealthChecker()   ← 健康检查定时器
              │       → NewRoutedClientWithPolicy(router, health, fallback)
              │         → RoutedClient 包装 Router + 健康路由 + 自动降级
              │
              ├─[6] runtime.NewInMemoryTaskManager
              │     → 内存任务表 + goroutine 执行器 + 事件流
              │
              ├─[7] extensions.NewManager(workspace)
              │     → skills.Manager.Reload()
              │       → 扫描 builtin/user/project 三层目录
              │       → 解析 skill.json + SKILL.md
              │       → 构建查找索引（别名映射）
              │     → extensionsruntime.NewManager(...)
              │       → MCP 服务器管理 + 工具同步
              │
              ├─[8] storage.NewDefaultAuditStore + PromptStore
              │
              ├─[9] tokenusage.NewTokenUsageManager
              │     → 文件/SQLite/memory 三种存储后端
              │
              └─[10] agent.NewRunner(Options{...})
                    → 注入所有依赖：Config, Client, Store, Registry, TaskManager, Extensions...
                    → 返回 Runtime{Config, Runner, Store, Session, TaskManager, Extensions}

          返回 Runtime 包 → 进入 TUI 或 Headless 模式
```

**此阶段涉及模块**: `cmd` → `app` → `config` → `session` → `provider` → `runtime` → `extensions`(+`skills`+`mcpctl`) → `storage` → `tokenusage` → `agent`

---

#### 阶段 2：TUI 启动与渲染

```
app/tui_run.go
  → tui_adapter.go: 将 agent.Runner 适配为 tui.Runner 接口
    → tuiRunnerAdapter 包装所有方法：
        RunPromptWithInput → runner.RunPromptWithInput
        SetObserver        → runner.SetObserver（事件桥接）
        SetApprovalHandler → runner.SetApprovalHandler（审批桥接）
        ListSkills / SubAgentManager / ListModels ...

  → tui.Run(Options{Runner, Store, MCPService, Session, ...})
    → tea.NewProgram(newModel(opts), tea.WithAltScreen())
      → Bubble Tea Elm 循环：
          Model.Init()    → 初始化命令（加载会话列表、启动状态栏）
          Model.Update()  → 响应消息/按键/事件
          Model.View()    → 渲染 TUI 界面
```

**TUI 界面布局**:
```
┌─ 输入区 ───────────────────────────────────────┐
│ > pages/login.tsx 的登录按钮在移动端点击没反应...│
├─ 对话区 ───────────────────────────────────────┤
│  [user]: pages/login.tsx 的登录按钮...          │
│  [assistant]: 正在分析 login.tsx...             │
│  [tool:read_file] login.tsx:45-89              │
│  [tool:search_text] "onClick" → 3 matches      │
│  [assistant]: 发现 onClick 绑定了 handleLogin   │
│              但移动端应使用 onTouchEnd...        │
│  [tool:replace_in_file] login.tsx ✓            │
├─ 状态栏 ───────────────────────────────────────┤
│ 会话 #12 │ model: claude-sonnet │ tokens: 4.2k  │
└────────────────────────────────────────────────┘
```

---

#### 阶段 3：用户提交任务 → Agent 处理

```
用户按 Enter 提交
  → tui/model.go: 构建 RunPromptInput{
        UserMessage: {Role: "user", Content: "pages/login.tsx..."},
        Assets: {},
        DisplayText: "pages/login.tsx 的登录按钮..."
    }

  → component_run_flow.go: 启动 Run 流程
    → runnerAdapter.RunPromptWithInput(ctx, session, input, "build", out)

      → agent/runner.go: Runner.RunPromptWithInput()
        → engine.go: Engine.HandleTurn(ctx, TurnRequest{Session, Input, Mode})
          → goroutine 中执行:

          ┌── emit(TurnEventStart) ──────────────────────┐
          │                                               │
          ├── prepareRunPrompt:                           │
          │   ├─ 加载 session.Messages                    │
          │   ├─ 构建系统 Prompt（prompt.go 固定顺序）:     │
          │   │   ① prompts/default.md                    │
          │   │   ② prompts/mode/build.md                 │
          │   │   ③ renderSystemBlock(环境/技能/工具)       │
          │   │   ④ renderActiveSkillPrompt (如有激活技能)  │
          │   │   ⑤ renderInstructionBlock (AGENTS.md)     │
          │   ├─ context/compaction.go: 上下文预算检查      │
          │   │   → usageRatio = tokens/contextWindow     │
          │   │   → >= 0.95: 触发压缩 (compaction)         │
          │   │   → >= 0.85: 标记 Warning                  │
          │   └─ 构建 llm.InternalRequest{                │
          │        Model, Messages, Tools, Assets, Stream  │
          │      }                                        │
          │                                               │
          ├── runPromptTurns (对话循环):                   │
          │   for step := 0; step < maxIterations; step++ {│
          │                                               │
          │     ├─ LLM.Chat(request) → stream             │
          │     │   → provider/router.go: Route(model)    │
          │     │     → 健康候选筛选 → 负载均衡 → fallback  │
          │     │   → anthropic/openai/gemini.go: Chat()  │
          │     │     → emit: TextDelta (逐 token 流出)    │
          │     │     → emit: ToolUse (工具调用)           │
          │     │     → emit: Done                        │
          │     │                                         │
          │     ├─ 收集文本 → emit(AssistantDelta)         │
          │     │   → TUI Observer → 实时流式渲染           │
          │     │                                         │
          │     ├─ 工具调用执行 (并行或串行):                │
          │     │   ┌─────────────────────────────────┐   │
          │     │   │ LLM 返回:                        │   │
          │     │   │  tool_1: read_file(login.tsx)    │   │
          │     │   │  tool_2: search_text("onClick")  │   │
          │     │   │  → 无依赖 → 并行执行               │   │
          │     │   └─────────────────────────────────┘   │
          │     │                                         │
          │     │   对每个工具调用:                          │
          │     │   ├─ PolicyGateway.DecideTool()         │
          │     │   │   → 权限决策: Allow / Deny / Ask    │
          │     │   │                                      │
          │     │   ├─ [若 Ask] ApprovalHandler()         │
          │     │   │   → TUI 弹出审批对话框               │
          │     │   │   → 用户在终端用键盘选择              │
          │     │   │   → 返回 Decision                   │
          │     │   │                                      │
          │     │   ├─ [若 Deny] 返回错误消息给 LLM         │
          │     │   │                                      │
          │     │   ├─ [若 Allow] Tool.Run(ctx, args)     │
          │     │   │   → 沙箱检查 (system_sandbox)        │
          │     │   │   → 文件系统权限验证                  │
          │     │   │   → 执行工具逻辑                     │
          │     │   │   → 返回结果文本                     │
          │     │   │                                      │
          │     │   ├─ emit(ToolCallStarted)               │
          │     │   ├─ emit(ToolCallCompleted + result)    │
          │     │   └─ sess.Messages += ToolMessage       │
          │     │                                         │
          │     ├─ 工具结果写回 → LLM 下一轮推理            │
          │     └─ 若无工具调用 → 退出循环                  │
          │   }                                           │
          │                                               │
          ├── 最终回复处理 (final_reply.go)                 │
          ├── emit(TurnEventComplete)                     │
          └── Store.Save(sess) 持久化会话                  │
```

**此阶段涉及模块**: `tui` → `agent` → `llm/provider` → `tools` → `policy` → `session` → `context/compaction` → `sandbox`

---

#### 阶段 4：LLM 决定委派子代理（假设场景）

假设 LLM 同时委派 explorer 子代理搜索相关文件：

```
LLM 返回: [
  {tool: "delegate_subagent", args: {agent: "explorer", task: "查找所有引用 handleLogin 的文件"}},
  {tool: "delegate_subagent", args: {agent: "explorer", task: "查找项目中的移动端事件处理模式"}}
]

→ 并行执行（两个子代理无依赖）:

  → DelegateSubAgentTool.Run()
    → agent/subagent_delegate.go: Runner.delegateSubAgent()

      ① 查找子代理定义
        → subagents.Manager.Find("explorer")
          → 查找索引: lookup["explorer"] → "explorer"
          → 返回 Agent{Name, Tools: ["list_files","read_file","search_text"], MaxTurns: 6, ...}

      ② 计算最终工具集
        → 父会话可用工具 ∩ 子代理定义工具
        → 强制移除 delegate_subagent（防止递归）
        → 若含写操作 → isolation 默认提升为 worktree

      ③ 隔离策略判定
        → agent/subagent_isolation.go:
          → isolation=none:  共享工作区，直接执行
          → isolation=worktree:
            → git worktree add .claude/worktrees/subagent-{id}
            → 在隔离目录中执行（写操作不影响主工作区）
            → 执行完毕 → 清理 worktree

      ④ 通过 TaskManager 创建异步任务
        → runtime.TaskManager.Submit(TaskSpec{
            SessionID, Name: "explorer", Kind: "subagent",
            Timeout: 120s, IsolatedWorktree: true, ...
          })
          → InMemoryTaskManager:
            → 注册任务 → Pending → Running
            → goroutine 中执行子代理:
              → 独立的 Runner.HandleTurn()
              → 独立的 session.Messages（仅包含子代理系统提示 + 用户任务）
              → 独立的工具集（只读）
              → 独立的 turn 循环 (≤ maxTurns)
            → 事件流通过 Stream() 推送给父代理

      ⑤ 返回结构化结果
        → DelegateSubAgentResult{
            OK: true, Status: "completed",
            Summary: "找到2个文件引用 handleLogin...",
            Findings: [{title:"...", body:"..."}],
            References: [{path:"...", line:45, note:"..."}],
            ModifiedFiles: []  // explorer 为只读，无修改
          }

      ⑥ 父代理继续推理
        → 子代理结果作为 ToolMessage 添加到父会话
        → LLM 基于子代理发现做出修改决策
        → replace_in_file(login.tsx): onClick → onTouchEnd
```

**此阶段涉及模块**: `tools/delegate_subagent` → `subagents` → `agent/subagent_*.go` → `runtime` → `sandbox`(worktree)

---

#### 阶段 5：持久化与恢复

```
运行完成:
  → session.Store.Save(sess)
    → ~/.bytemind/sessions/{session_id}.json
      → 完整保存: Messages[], Plan, ActiveSkill, TokenUsage, Mode

  → tokenusage.Manager.RecordTurnUsage()
    → 记录本 turn 的 input/output tokens
    → 累计到会话总用量
    → 按配置间隔持久化到文件/SQLite

  → storage.AuditStore
    → 记录工具调用审计: tool_name, args, result, decision

下次启动:
  → bytemind --session {id}
    → store.Load(id) 恢复完整会话
    → TUI 渲染全部历史消息（含工具调用和结果）
    → 用户继续对话
```

---

#### 阶段 6：扩展系统介入（假设用户安装了 MCP 服务器）

```
用户配置了 MCP 服务器 (mcp.json):
  → extensions/mcp/adapter.go: MCPAdapter
    → 启动子进程（stdio transport）
    → MCP 协议握手 + tools/list
    → 将 MCP 工具注册到 tools.Registry
      → Registry.Register(tool, {Source: "extension", ExtensionID: "mcp-..."})
      → 工具定义注入到 LLM 的系统 Prompt 中
      → LLM 可调用 MCP 工具

如果有激活的 Skill:
  → skills/manager.go: 加载项目 .bytemind/skills/{name}/
  → agent/prompt_skills.go: 注入技能提示到系统 Prompt
  → 技能工具策略决定可用工具范围
```

**完整链路涉及的全部模块**:
```
cmd → app → config → tui → agent → provider/llm → tools
                                            → policy → session
                                            → runtime → subagents
                                            → extensions/skills/mcpctl
                                            → sandbox → storage
                                            → context/compaction
                                            → tokenusage
                                            → notify → history
```

---

## 3. 分层架构

```
┌────────────────────────────────────────────────────────────────┐
│  cmd/bytemind         入口，仅做参数解析与装配委托                │
├────────────────────────────────────────────────────────────────┤
│  internal/app         应用装配层：Bootstrap 依赖注入               │
│                        + CLI dispatch + TUI/Worker 模式分发      │
├───────────────────────┬────────────────────────────────────────┤
│  tui/                 │  internal/agent                        │
│  Bubble Tea TUI       │  主闭环编排：Runner→Engine→RunLoop      │
│  Model-Update-View    │  Prompt 组装 / 对话循环 / 工具协调       │
│  Event Observer       │  SubAgent 委派 / Compaction / Skills   │
├───────────────────────┴────────────────────────────────────────┤
│  internal/          核心服务层（按接口隔离）                       │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌──────────┐ ┌────────┐  │
│  │provider │ │ tools   │ │ session │ │ runtime  │ │ config │  │
│  │ + llm   │ │+policy  │ │         │ │+sandbox  │ │        │  │
│  ├─────────┤ ├─────────┤ ├─────────┤ ├──────────┤ ├────────┤  │
│  │skills   │ │extensions│ │subagents│ │ plan     │ │notify  │  │
│  ├─────────┤ ├─────────┤ ├─────────┤ ├──────────┤ ├────────┤  │
│  │storage  │ │tokenusage│ │history  │ │ context  │ │assets  │  │
│  └─────────┘ └─────────┘ └─────────┘ └──────────┘ └────────┘  │
└────────────────────────────────────────────────────────────────┘
```

## 4. 模块职责与边界

### 4.1 入口 + 装配层

| 模块 | 做什么 | 不做什么 |
|------|--------|----------|
| `cmd/bytemind` | 参数解析，调用 `app.RunCLI` | 不承载业务逻辑、不初始化依赖 |
| `internal/app` | Bootstrap 装配全部依赖，CLI/TUI/Worker 模式分发 | 不实现具体工具或 Provider 协议 |

Bootstrap 装配顺序严格固定，所有依赖在 `agent.NewRunner()` 构造时一次性注入，无全局变量：

```
Config → Session → Provider Client → TaskManager → Extensions →
Storage → TokenUsage → agent.NewRunner(全部依赖) → Runtime
```

### 4.2 TUI 交互层 (`tui/`)

基于 Bubble Tea Elm 架构。`tui/ports.go` 定义 TUI 层自有接口，`app/tui_adapter.go` 负责将 `agent.Runner` 适配过来。

| 组件 | 文件 | 职责 |
|------|------|------|
| Model | `model.go` | 全局 UI 状态（输入、对话、会话列表、审批、Scroll） |
| 对话渲染 | `component_conversation.go` + `component_chat_stream.go` | 消息历史 + 流式增量渲染 |
| Markdown | `markdown_renderer.go` + `simple_markdown.go` | Markdown → ANSI 终端格式 |
| 工具渲染 | `tool_renderers_builtin.go` | 工具调用卡片（文件操作/diff/shell/子代理） |
| 输入 | `component_input.go` | 多行输入 + 图片粘贴 + @mention |
| 会话管理 | `component_sessions.go` | 会话列表、切换、删除 |
| Slash 命令 | `component_slash_entry.go` + `component_palettes.go` | `/` 命令面板 + 候选搜索 |
| 运行控制 | `component_run_flow.go` | 运行启动、停止、流式更新 |
| 审批 | `component_overlays.go` | 审批弹窗（Overlay） |

### 4.3 Agent 编排层 (`internal/agent`)

系统核心。`Runner` 是外部唯一入口，`Engine` 封装单轮对话执行。

```
Runner (runner.go + runner_control.go)
  ├── 配置聚合：Config + Client + Store + Registry + TaskManager + Extensions
  ├── 顶层 API：RunPromptWithInput / ListSkills / ActivateSkill / ListModels
  ├── 子代理：DispatchSubAgent / SubAgentManager
  └── 控制：SetObserver / SetApprovalHandler / UpdateProvider

Engine (engine.go → engine_run_loop.go → engine_run_setup.go)
  ├── HandleTurn: 单轮对话 → <-chan TurnEvent
  ├── prepareRunPrompt: Prompt 组装 + Context Budget 检查
  └── runPromptTurns: LLM 调用 → 工具执行 → 循环

对话循环核心流程:
  ┌─ 构建 messages (系统 Prompt + 历史消息 + 新用户消息)
  ├─ LLM.Chat(stream) → 收集 TextDelta + ToolCalls
  ├─ 执行 ToolCalls（工具依赖分析 → 并行/串行）
  │   ├─ PolicyGateway.DecideTool → Allow/Deny/Ask
  │   ├─ ApprovalHandler (如需要)
  │   ├─ Tool.Run() 执行
  │   └─ 工具结果 → ToolMessage 追加到 messages
  └─ 循环直到: 无 ToolCall | 达到 max_iterations | 上下文超限 | 用户中断
```

### 4.4 LLM Provider 层 (`internal/llm` + `internal/provider`)

```
internal/llm/contract.go     ← 统一协议接口（ProviderClient）
internal/llm/types.go        ← Message / ToolCall / StreamEvent / Usage

internal/provider/
  factory.go       ← NewClient / NewClientFromRuntime / NewRouterClient
  registry.go      ← Provider 注册表
  router.go        ← 路由策略（模型筛选 + 健康检查 + Fallback）
  models.go        ← 模型元数据 + 兼容性映射
  anthropic.go     ← Anthropic Messages API 适配
  openai.go        ← OpenAI Chat Completions API 适配
  gemini.go        ← Gemini generateContent API 适配
  health.go        ← 健康检查（定时探测 + 熔断）
```

### 4.5 工具系统 (`internal/tools`)

| 概念 | 说明 |
|------|------|
| `Tool` 接口 | `Definition()` + `Run(ctx, rawArgs, execCtx)` |
| `ToolSpec` | 工具规格：名称、描述、安全等级、允许模式、超时、并发安全性 |
| `Registry` | 线程安全注册表，支持模式过滤、allowlist/denylist、名称冲突解决 |
| `ExecutionContext` | 工具执行所需的全部上下文（工作区、审批、会话、沙箱、权限） |

内置 13 个工具：文件操作 ×5、搜索 ×1、网络 ×2、Shell ×1、子代理委派 ×1、任务管理 ×2、写入文件 ×1（apply_patch 含在内）。

### 4.6 会话系统 (`internal/session`)

```go
Session { ID, Workspace, Title, Messages[], Conversation, Plan, ActiveSkill, Mode, TokenUsage }
```

- `Store`: Save/Load/List/Delete，基于文件系统 JSON 持久化
- 会话保存在 `~/.bytemind/sessions/{id}.json`
- 支持会话列表、跨会话恢复、零消息清理

### 4.7 子代理系统 (`internal/subagents` + `agent/subagent_*.go`)

子代理通过 `.md` 文件定义（YAML frontmatter + Markdown 正文），三层作用域覆盖。

**文件格式**:
```markdown
---
name: explorer
description: Read-only repository exploration agent
tools: [list_files, read_file, search_text]
max_turns: 6
isolation: none
---
You are a focused repository explorer...
```

**执行隔离**:
| isolation | 行为 |
|-----------|------|
| `none` | 共享工作区，直接执行 |
| `worktree` | 创建 git worktree，隔离文件写入。若工具集含写操作，自动提升为此级别 |

### 4.8 运行时系统 (`internal/runtime`)

`InMemoryTaskManager` 实现 `TaskManager` 接口：
- goroutine 异步执行，状态机驱动（Pending→Running→Completed/Failed/Killed）
- 父子任务关联（子代理是父任务的子任务，级联取消）
- 事件流推送（`Stream()` 返回 channel）
- 支持重试（计数 + 最大重试限制）
- 可插拔的 `TaskEventStore`（审计持久化）

### 4.9 上下文管理 (`internal/context` + `agent/compaction.go`)

自动上下文压缩：当 Token 使用率达到 `warning_ratio`(0.85) 时标记警告，达到 `critical_ratio`(0.95) 时触发压缩。压缩保留最近 N 对消息 + 生成历史摘要，用摘要替换旧消息。

### 4.10 其余模块速查

| 模块 | 职责 |
|------|------|
| `internal/config` | 多层配置加载、合并、校验（用户→项目→环境变量） |
| `internal/policy` | 权限决策（Allow/Deny/Ask）+ Shell 命令风险分析 |
| `internal/sandbox` | 沙箱隔离（macOS Seatbelt / Linux Landlock / Windows 进程） |
| `internal/extensions` | 扩展管理（Load/Unload/List）+ MCP 适配 |
| `internal/skills` | Skill 发现与加载，`skill.json` + `SKILL.md` |
| `internal/plan` | Plan 模式状态管理（build/plan） |
| `internal/tokenusage` | Token 用量统计与存储（file/memory/sqlite3） |
| `internal/storage` | 统一存储抽象（AuditStore / TaskStore / PromptHistory / JSONL） |
| `internal/notify` | 平台原生桌面通知 |
| `internal/mcpctl` | MCP 服务器管理（Add/Remove/Enable/Test） |
| `internal/mention` | @mention 自动补全（文件/Agent 候选） |
| `internal/frontmatter` | YAML frontmatter 解析器 |
| `internal/history` | Prompt 历史存储 |
| `internal/assets` | 多模态资产存储（图片等） |
| `internal/context` | 上下文构建器（消息列表 → LLM Request） |
| `internal/core` | 基础类型（SessionID, TaskID, Role, Decision, RiskLevel） |

---

## 5. 关键设计决策

### 决策 1：TUI (Bubble Tea) 而非 Web UI 或 IDE 插件

**选择**: 基于 Go + Bubble Tea 的终端全屏 UI

**备选方案**:

| 方案 | 优势 | 劣势 |
|------|------|------|
| Web UI (Electron/React) | 丰富交互、跨平台一致 | 重依赖、启动慢、偏离"终端工具"定位 |
| IDE 插件 (VS Code) | 深度集成 IDE | 绑定特定 IDE、用户群受限、开发复杂度高 |
| 纯 CLI 行式交互 | 极简、无 UI 依赖 | 无法承载会话列表、审批弹窗、实时流式、命令面板 |
| **TUI (Bubble Tea)** ✓ | 终端原生、轻量、状态管理清晰、可承载复杂 UI | 终端兼容性需要处理（Windows CJK/IME） |

**选择理由**:
1. **用户画像匹配**: 目标用户本身在终端工作，TUI 让工具融入现有工作流，无需切换窗口
2. **Elm 架构优势**: Bubble Tea 的 Model-Update-View 模式天然适合管理复杂 UI 状态（会话切换、审批弹窗、实时流式、@mention 面板），比纯 CLI 回调式代码更可维护
3. **轻量启动**: 单二进制文件，无前端构建链，启动 < 100ms
4. **当前基线**: 仓库已有成熟的 TUI 实现，产品形态与代码实现一致

**代价**: 终端兼容性（Windows CJK/IME 需要特殊处理）、鼠标支持需要终端配合

---

### 决策 2：Go 语言而非 Python/TypeScript

**选择**: Go 1.25

**备选方案**:

| 方案 | 优势 | 劣势 |
|------|------|------|
| Python | LLM 生态丰富、开发快 | 单二进制分发困难、GIL 限制并发、启动慢 |
| TypeScript (Node) | 前端同构、npm 生态 | 单二进制分发依赖打包工具、内存占用高 |
| Rust | 极致性能、零成本抽象 | 开发效率低、LLM 生态不成熟、招聘难 |
| **Go** ✓ | 编译为单二进制、并发原生（goroutine）、启动快、部署简单 | LLM 库需自行适配 |

**选择理由**:
1. **单二进制分发**: Go 编译为静态链接的单一可执行文件，用户无需安装运行时。对终端工具而言这是最重要的分发特性
2. **并发模型**: `goroutine + channel` 天然适合 Agent 的核心模型——流式 LLM 调用、多工具并行执行、事件流推送、子代理异步调度
3. **快速启动**: 冷启动 < 100ms，对 CLI/TUI 工具至关重要
4. **跨平台**: Go 交叉编译支持 macOS/Linux/Windows，一次构建、到处运行

**代价**: 需要自行适配各 LLM Provider 的 HTTP API（OpenAI/Anthropic/Gemini），无现成的官方 Go SDK

---

### 决策 3：事件驱动的 Observer 模式而非直接耦合

**选择**: Agent 层通过 `chan TurnEvent` + `Observer` 回调与 TUI 通信

```
Engine.HandleTurn → <-chan TurnEvent (goroutine 安全)
Observer(Event)   → TUI 异步 channel → Model.Update()
```

**备选方案**:

| 方案 | 优势 | 劣势 |
|------|------|------|
| 直接回调 | 简单、同步、易理解 | Agent 阻塞等 UI 渲染，流式体验卡顿 |
| 共享状态 + 轮询 | 无回调嵌套 | 轮询开销、状态竞态 |
| **事件流 (channel + observer)** ✓ | 解耦、可测试、goroutine 安全 | 需要小心 channel 关闭和 buffer 大小 |

**选择理由**:
1. **并发安全**: Agent 在 goroutine 中运行，TUI 在主 goroutine。Channel 是 Go 最自然的安全通信方式
2. **背压控制**: 128 大小的 buffered channel 确保 UI 渲染不会阻塞 Agent 推理
3. **可测试性**: 测试时可以直接消费 `<-chan TurnEvent` 验证事件序列，无需启动 TUI
4. **扩展性**: 未来可以接多个 Observer（日志、监控、审计）而不修改 Engine

**代价**: 异步错误处理需要额外注意（Agent crash 时 channel 正确关闭）

---

### 决策 4：接口驱动的依赖注入而非全局单例

**选择**: `Bootstrap()` 中显式构造所有依赖，通过接口注入到 `Runner`

```go
type Runner struct {
    store         SessionStore      // 接口
    registry      ToolRegistry      // 接口
    executor      ToolExecutor      // 接口
    policyGateway PolicyGateway     // 接口
    taskManager   TaskManager       // 接口
    extensions    extensions.Manager // 接口
    // ...
}
```

**备选方案**:

| 方案 | 优势 | 劣势 |
|------|------|------|
| 全局单例 | 简单、无参数传递 | 测试难隔离、隐式依赖、并行测试冲突 |
| Service Locator | 比全局变量稍好 | 依赖关系不透明、编译时无法检查 |
| **接口注入** ✓ | 可测试、依赖明确、编译时检查 | 构造函数参数多（Options struct 缓解） |

**选择理由**:
1. **可测试性**: 每个接口都可以用 mock 替换，Runner 的测试覆盖了复杂的策略分支和工具组合
2. **依赖透明**: 一个类型需要什么能力，在它的 Options/构造函数中一目了然
3. **渐进式重构**: 当需要替换实现（如从 InMemoryTaskManager 换为持久化队列），只需实现接口并修改 Bootstrap 一处

**代价**: 接口定义需要维护，Go 的隐式接口实现让寻找"谁实现了这个接口"需要 grep

---

### 决策 5：多 Provider Router 而非单一 Provider 绑定

**选择**: Router + Registry + HealthChecker + Fallback

```
ProviderRuntimeConfig
  → Registry(providers: {openai:..., anthropic:..., gemini:...})
  → Router(defaultProvider, defaultModel)
  → HealthChecker(定时探测)
  → RoutedClient(route → 健康排序 → 调用 → 失败则 fallback)
```

**备选方案**:

| 方案 | 优势 | 劣势 |
|------|------|------|
| 单一 Provider | 简单、无路由逻辑 | 单点故障、无法利用不同模型优势 |
| 简单轮询 | 均衡 | 不考虑健康状态，故障 Provider 仍被选中 |
| **Router + Health + Fallback** ✓ | 高可用、可灵活选择模型 | 实现复杂度较高 |

**选择理由**:
1. **高可用**: 用户配置多个 Provider，一个故障时自动降级到备用。对长时间运行的 Agent 任务至关重要
2. **灵活选择模型**: 不同任务可能需要不同模型（如快速任务用小模型、复杂任务用大模型），Router 让模型切换对 Agent 透明
3. **Provider 无关**: `llm.Client` 统一接口屏蔽了 OpenAI/Anthropic/Gemini 的协议差异，Agent 层不感知 Provider 细节

**代价**: 需要维护三个 Provider 适配器（~1500 行/适配器），协议差异（如 Anthropic 的 tool_use content block vs OpenAI 的 function call）增加了适配复杂度

---

### 决策 6：子代理通过 TaskManager 异步执行 + worktree 隔离

**选择**: 子代理作为 `runtime.Task` 提交到 `TaskManager`，写操作自动升级为 git worktree 隔离

**备选方案**:

| 方案 | 优势 | 劣势 |
|------|------|------|
| 同步调用 | 简单、结果立即可用 | 阻塞父 Agent、无法并行多个子代理、无超时控制 |
| 独立进程 | 强隔离 | 进程启动开销大、通信复杂、资源浪费 |
| **TaskManager + worktree** ✓ | 并发、可控隔离、复用现有基础设施 | worktree 创建/清理有开销 |

**选择理由**:
1. **上下文隔离**: 子代理的核心价值不是多 Agent 编排，而是保护主 Agent 的上下文窗口。搜索、阅读、探索等高噪声过程在独立上下文中完成，主 Agent 只接收压缩结论
2. **并发执行**: LLM 可在单轮发出多个 `delegate_subagent` 调用（如同时查前端+后端），通过 `TaskManager` 并行执行，延迟从 ΣT 降为 max(T)
3. **安全边界**: 子代理工具权限只能收窄（父会话可用 ∩ 子代理定义），写操作自动触发 worktree 隔离。不允许子代理递归委派
4. **复用现有能力**: 子代理复用同一套 Runner/Engine/ToolExecutor/PolicyGateway，不另建平行的执行引擎

**代价**: worktree 的 git 操作（add/remove）有 I/O 开销，对快速只读子代理略显多余

---

### 决策 7：文件系统 JSON 会话存储而非数据库

**选择**: 每个 Session 一个 JSON 文件，存储在 `~/.bytemind/sessions/{id}.json`

**备选方案**:

| 方案 | 优势 | 劣势 |
|------|------|------|
| SQLite | 查询快、事务安全、支持索引 | 增加 CGO 依赖（或纯 Go 实现不够成熟）、多进程写入冲突 |
| 嵌入式 KV (BoltDB) | 高性能、事务 | 额外的依赖、查看/调试不直观 |
| **JSON 文件** ✓ | 零依赖、人类可读、易于调试/迁移/备份 | 大文件读写性能差、无并发安全保证 |

**选择理由**:
1. **零依赖**: Go 标准库 `encoding/json` + `os` 即可，不引入 CGO 或第三方存储引擎
2. **可调试**: 用户可直接 `cat` 查看会话内容，方便问题排查
3. **MVP 阶段足够**: 单用户 TUI 工具不存在并发写入竞争，单个会话文件通常在几百 KB 到几 MB
4. **可迁移**: JSON 文件易于备份、迁移、跨机器同步

**代价**: 会话列表需要遍历目录 + 逐文件读取元数据，大规模会话时性能会下降（当前通过 in-memory index 缓解）

---

### 决策 8：内存任务管理器而非持久化任务队列

**选择**: `InMemoryTaskManager`——所有任务状态在内存中，可选持久化事件存储

**备选方案**:

| 方案 | 优势 | 劣势 |
|------|------|------|
| Redis/消息队列 | 持久化、分布式 | 重量级依赖、对本地 TUI 工具过度设计 |
| SQLite 任务表 | 持久化、可查询 | 增加存储层复杂度 |
| **InMemory** ✓ | 零依赖、低延迟、简单 | 进程退出后任务状态丢失 |

**选择理由**:
1. **本地单用户工具**: 无分布式需求，所有任务与 Agent 进程同生命周期
2. **低延迟**: 子代理调度纯内存操作，无需 I/O
3. **可选持久化**: `TaskEventStore` 接口允许接持久化存储（当前已有文件/SQLite 适配器），事件审计与任务执行解耦
4. **足够的能力**: 状态机、父子任务、重试、超时、流式推送——在内存中全部实现，满足当前所有需求

**代价**: 进程崩溃后无法恢复正在执行的任务（对 TUI 工具而言可接受，用户重新运行即可）

---

### 决策 9：基于 Token 比例的上下文压缩而非固定窗口

**选择**: 监控 `tokens / contextWindow` 比例，达到阈值触发 LLM 驱动的摘要压缩

**备选方案**:

| 方案 | 优势 | 劣势 |
|------|------|------|
| 固定消息窗口 (最近 N 轮) | 简单、可预测 | 长消息可能撑爆上下文、短消息浪费预算 |
| 滑动窗口 (按字符数) | 简单 | 字符数 ≠ Token 数，跨模型不准确 |
| 仅截断旧消息 | 无 LLM 调用开销 | 丢失关键上下文（用户目标、已完成决策） |
| **Token 比例 + LLM 摘要** ✓ | 精确、保留关键信息 | 需要额外 LLM 调用（压缩本身消耗 Token） |

**选择理由**:
1. **精确控制**: 使用 tiktoken-go 按模型 tokenizer 精确计算，避免超出上下文窗口
2. **保留语义**: LLM 驱动的摘要可以保留"用户目标、已做决策、未完成任务、关键文件路径"等结构化信息，而纯截断会丢失
3. **渐进式压缩**: Warning(85%) 只是标记，Critical(95%) 才触发，给 Agent 留出完成当前任务的空间

**代价**: 压缩本身需要一次 LLM 调用的 Token（约 2k-4k），且摘要质量依赖 LLM 能力

---

### 决策 10：Prompt 固定分层组装而非动态模板

**选择**: 固定顺序的 5 层组装（base → mode → runtime context → active skill → AGENTS.md）

**备选方案**:

| 方案 | 优势 | 劣势 |
|------|------|------|
| 动态模板（Jinja/Go template） | 灵活、可配置 | 普通用户不会改、LLM 对 prompt 结构敏感、测试困难 |
| 单一长 prompt | 简单 | 难以按场景增减块 |
| **固定分层** ✓ | 可测试、可预测、对 LLM 行为友好 | 修改 prompt 结构需要改代码 |

**选择理由**:
1. **LLM 稳定性**: 固定结构让 LLM 行为可预测。动态模板带来的 prompt 结构变化可能导致模型行为退化
2. **可测试**: 每个层的渲染函数可以独立单元测试，验证包含/不包含条件
3. **Referential transparency**: 同样的输入产生同样的 prompt，方便调试和回归

**代价**: Prompt 结构调整需要修改 Go 代码并重新编译，但对于 beta 阶段的工具，这比外部模板配置更安全

---

### 决策 11：工具并发执行（无依赖并行 + 有依赖串行）

**选择**: 同轮内多个 `ToolCall` 分析依赖关系后并行/串行执行

```
同一轮 LLM 返回 [read_file(A), read_file(B), replace_in_file(A, patch)]
  → 依赖分析:
      read_file(A)  ──┐
      read_file(B)  ──┤ 无依赖 → 并行
      replace_in_file(A, patch) ─┤ 依赖 read_file(A) 结果 → 等 A 完成后执行
```

**备选方案**:

| 方案 | 优势 | 劣势 |
|------|------|------|
| 严格串行 | 简单、无竞态 | 延迟 ΣT，子代理委派场景浪费严重 |
| 全部并行 | 最大吞吐 | 有依赖的工具调用可能读到过期状态 |
| **依赖分析 + 混合执行** ✓ | 安全 + 性能 | 依赖判定算法需要持续完善 |

**选择理由**:
1. **实际价值大**: LLM 常在同轮发出多个只读工具调用（如同时查多个文件），并行执行将延迟从 ΣT 降为 max(T)
2. **安全性**: 通过工具类型和参数分析依赖，写操作排队等读操作完成，避免竞态
3. **基础设施复用**: `runtime.TaskManager` + goroutine 已经提供了并行执行所需的所有原语

**代价**: 依赖分析目前基于工具类型判定，不分析参数级文件路径重叠（未来可优化）

---

### 决策 12：扩展系统三层作用域覆盖

**选择**: Builtin → User → Project 三层，后加载覆盖先加载

**备选方案**:

| 方案 | 优势 | 劣势 |
|------|------|------|
| 单一全局目录 | 简单 | 无法区分内置/用户/项目、无优先级 |
| 仅项目级 | 与仓库绑定 | 无法跨项目共享常用配置 |
| **三层作用域** ✓ | 内置基础能力 + 用户自定义 + 项目特化 | Override 语义需要清晰文档 |

**选择理由**:
1. **匹配使用场景**: Builtin 提供基础子代理（explorer/review/general），用户可跨项目安装常用技能（~/.bytemind/skills/），项目可定制项目特定的 Agent
2. **Override 优先级明确**: Project > User > Builtin，清晰可预测
3. **与 Claude Code 对齐**: 采用类似的目录结构和文件格式，降低用户迁移成本

**代价**: 三层目录扫描有 I/O 开销，Reload 时需要合并三层并处理覆盖关系

---

## 6. 已知架构问题与演进方向

当前架构存在一些已知的耦合问题（来自 2026-04-14 耦合审计），正在逐步解决：

| 编号 | 问题 | 当前状态 |
|------|------|----------|
| C1 | `agent` 包职责过重（编排 + 策略 + 上下文 + 工具执行） | 已拆分 `internal/policy`、`internal/context`、`turn_*.go` 文件；进一步拆分进行中 |
| C2 | 权限策略分散在 `tools/` 和 `agent/` 两处 | `policy_gateway.go` 已集中，`policy/` 包独立 |
| C3 | `session` 同时承担语义状态和落盘细节 | 语义 + 持久化分拆进行中 |
| C4 | `session/history/tokenusage` 各自维护存储 | `storage/` 统一抽象已建立，迁移进行中 |
| C5 | `tui` 直接改会话模式/计划/Provider | adapter 层已隔离大部分，剩余少量直接调用 |
| C6 | 入口重复装配逻辑 | `internal/app` 已统一 Bootstrap |
| C8 | `llm.Message` 同时维护 part-based 和 legacy 字段 | 正在逐步清理 legacy 字段依赖 |

---

## 7. 接口清单

系统核心接口汇总：

```go
// LLM 协议
type Client interface {
    Chat(ctx, InternalRequest) (<-chan StreamEvent, error)
}

// Agent 引擎
type Engine interface {
    HandleTurn(ctx, TurnRequest) (<-chan TurnEvent, error)
}

// 权限决策
type PolicyGateway interface {
    DecideTool(ctx, ToolDecisionInput) (ToolDecision, error)
}

// 工具执行
type Tool interface {
    Definition() ToolDefinition
    Run(ctx, json.RawMessage, *ExecutionContext) (string, error)
}

// 任务调度
type TaskManager interface {
    Submit(ctx, TaskSpec) (TaskID, error)
    Get(ctx, TaskID) (Task, error)
    Cancel(ctx, TaskID, reason string) error
    Retry(ctx, TaskID) (TaskID, error)
    Stream(ctx, TaskID) (<-chan TaskEvent, error)
    Wait(ctx, TaskID) (TaskResult, error)
}

// TUI ↔ Agent 桥接
type Runner interface {
    RunPromptWithInput(ctx, sess, input, mode, out) (string, error)
    SetObserver(observer Observer)
    SetApprovalHandler(handler ApprovalHandler)
    // + Skills / SubAgents / Models 管理方法
}
```
