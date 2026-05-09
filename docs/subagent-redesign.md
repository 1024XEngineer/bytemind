# SubAgent 模块整改设计方案

## 概述

基于 Claude Code SubAgent 架构对比分析，对 ByteMind 的 SubAgent 模块进行整改。核心改动：

1. 用户交互入口从直接 dispatch 改为 `@` mention + LLM 决策
2. 执行引擎增加流式传递、异步工具白名单、自动后台化
3. Agent 定义扩展，增强 LLM 引导
4. 工具描述动态化

---

## 一、`@` mention 系统扩展

### 1.1 数据结构变更

**文件：`internal/mention/index.go`**

`Candidate` 结构体新增两个字段：

```go
type Candidate struct {
    Path        string
    BaseName    string
    TypeTag     string // 保留兼容，渲染不再使用
    Kind        string // 新增："file" | "agent"
    Description string // 新增：agent 的描述，file 为空
}
```

### 1.2 Agent 候选源接口

**新建文件：`internal/mention/agent_source.go`**

```go
package mention

type AgentSource interface {
    ListAgents() []AgentEntry
}

type AgentEntry struct {
    Name        string
    Description string
    Scope       string // "builtin" | "user" | "project"
}
```

`mention` 包通过此接口获取 agent 列表，不直接依赖 `subagents.Manager`，保持解耦。

### 1.3 搜索合并逻辑

**文件：`tui/component_palette_runtime.go`**

`syncMentionPalette()` 现有逻辑：

```go
results := m.mentionIndex.SearchWithRecency(token.Query, mentionPageSize*3, m.mentionRecent)
```

改为：

```go
results := m.mentionIndex.SearchWithRecency(token.Query, mentionPageSize*3, m.mentionRecent)

// 合并 agent 候选（空 @ 不混入，打字后才搜索 agent）
if m.agentSource != nil && token.Query != "" {
    for _, a := range m.agentSource.ListAgents() {
        if matchesQuery(a.Name, token.Query) {
            results = append(results, mention.Candidate{
                Path:        a.Name,
                BaseName:    a.Name,
                Kind:        "agent",
                Description: a.Description,
            })
        }
    }
}

// cap 到 mentionPageSize (15)
if len(results) > mentionPageSize {
    results = results[:mentionPageSize]
}
```

空 `@` 只展示当前目录第一层文件（现有行为），不混入 agent。用户输入字符后才走全量模糊搜索并合并 agent。

**`model` 新增字段**：

```go
agentSource mention.AgentSource
```

在 `Bootstrap()` 时注入，实现 `subagents.Manager` → `AgentSource` 的适配。

### 1.4 渲染符号

**文件：`tui/component_palettes.go`**

`renderMentionPalette()` 中，当前渲染：

```go
nameText := "[" + item.TypeTag + "] " + item.BaseName
```

改为：

```go
var nameText string
switch item.Kind {
case "agent":
    nameText = "* " + item.BaseName
    if desc := strings.TrimSpace(item.Description); desc != "" {
        nameText += " - " + desc
    }
default: // "file" 或空
    nameText = "+ " + item.BaseName
}
```

渲染效果：

```
* explorer - Read-only explorer agent for broad codebase discovery
+ main.go
+ README.md
```

### 1.5 选中行为

**文件：`tui/component_palette_runtime.go`**

`handleMentionPaletteKey()` 中：

- 选中 file：调用 `mention.InsertIntoInput()` 插入文件路径到输入框（现有行为不变）
- 选中 agent：把 `@agentName` 插入输入框（替换 `@query` 部分）

用户看到的输入框保留原文：`@explorer find auth code`。

---

## 二、Agent Mention → System-Reminder 注入

### 2.1 设计原则

**不修改 `llm.Message` 结构**。`llm.Message` 是全链路公共结构，给它加 `IsMeta` 会影响会话统计、压缩、TUI 渲染等多个模块。

**利用现有的 `RunPromptInput` 覆写机制**。`conversationMessagesForTurn()` 会在发给 API 前替换 `persistedUserMessageIndex` 处的消息。这意味着：

- Session 里存用户原文（不污染）
- 发给 API 时替换成增强文本（带 `<system-reminder>`）
- TUI、统计、压缩都只看到原文

### 2.2 流程总览

```
用户输入: @explorer find auth code
  → @ 补全弹出，显示 "* explorer - Read-only explorer..."
  → 用户选择 explorer 或继续打字
  → 输入框保留: "@explorer find auth code"（原文不动）
  → 用户按 Enter

  → Session 存储: [user: "@explorer find auth code"]（原文，不改）

  → prepareRunPrompt() 阶段：
    1. 检测到 @explorer mention
    2. 构建 RunPromptInput.UserMessage = 原文 + <system-reminder>
    3. 设置 PersistDisplayTextAsUserMessage = true

  → buildTurnMessages() 阶段：
    conversationMessagesForTurn() 用增强消息替换 session 中的原文

  → 发给 API 的 user message:
    "@explorer find auth code
     <system-reminder>
     The user has expressed a desire to invoke the following agent(s):
     - explorer: Read-only explorer agent for broad codebase discovery.
     Use the delegate_subagent tool if appropriate, passing in the required context to it.
     </system-reminder>"

  → 父 Agent LLM turn 开始
  → LLM 看到增强后的消息
  → LLM 决策：
     a) 调用 delegate_subagent(agent: "explorer", task: "find auth code...")
     b) 自己直接处理（如果任务简单或不适合 delegate）
     c) 选不同的 agent（如果 LLM 认为更合适）
  → 正常的 delegate_subagent 执行流程
```

### 2.3 Mention 提取

**新建文件：`internal/agent/agent_mentions.go`**

```go
package agent

import "regexp"

var agentMentionPattern = regexp.MustCompile(`@(\w[\w-]*)`)

type AgentMention struct {
    Name string
}

func extractAgentMentions(input string, knownAgents map[string]struct{}) []AgentMention {
    matches := agentMentionPattern.FindAllStringSubmatch(input, -1)
    var mentions []AgentMention
    seen := make(map[string]struct{})
    for _, match := range matches {
        if len(match) < 2 {
            continue
        }
        name := match[1]
        if _, ok := knownAgents[name]; ok {
            if _, already := seen[name]; !already {
                mentions = append(mentions, AgentMention{Name: name})
                seen[name] = struct{}{}
            }
        }
    }
    return mentions
}
```

只匹配已注册的 agent name，避免误匹配 `@someone` 等非 agent mention。

### 2.4 System-Reminder 构建

**同文件：`internal/agent/agent_mentions.go`**

```go
func buildAgentMentionReminder(mentions []AgentMention, agentDescs map[string]string) string {
    if len(mentions) == 0 {
        return ""
    }
    var b strings.Builder
    b.WriteString("The user has expressed a desire to invoke the following agent(s):\n")
    for _, m := range mentions {
        desc := agentDescs[m.Name]
        if desc != "" {
            fmt.Fprintf(&b, "- %s: %s\n", m.Name, desc)
        } else {
            fmt.Fprintf(&b, "- %s\n", m.Name)
        }
    }
    b.WriteString("Use the delegate_subagent tool if appropriate, passing in the required context to it.")
    return b.String()
}
```

### 2.5 增强消息构建

**同文件：`internal/agent/agent_mentions.go`**

```go
func enhanceUserMessageWithAgentMentions(original string, knownAgents map[string]struct{}, agentDescs map[string]string) string {
    mentions := extractAgentMentions(original, knownAgents)
    if len(mentions) == 0 {
        return original
    }
    reminder := buildAgentMentionReminder(mentions, agentDescs)
    return original + "\n<system-reminder>\n" + reminder + "\n</system-reminder>"
}
```

### 2.6 覆写注入点

**文件：`internal/agent/engine_run_setup.go`**

`prepareRunPrompt()` 中，在构建 `RunPromptInput` 后，检测 agent mention 并增强 user message：

```go
func (e *defaultEngine) prepareRunPrompt(sess *session.Session, input RunPromptInput, mode string) (runPromptSetup, error) {
    // ... existing logic ...

    // 检测 agent mention，增强发给 API 的 user message
    userInput := input.DisplayText
    enhanced := enhanceUserMessageWithAgentMentions(userInput, knownAgents, agentDescs)
    if enhanced != userInput {
        input.UserMessage = llm.NewUserTextMessage(enhanced)
        input.PersistDisplayTextAsUserMessage = true
    }

    // ... rest of existing logic ...
}
```

Session 中存的是 `input.DisplayText`（用户原文），但 `input.UserMessage`（发给 API 的）被增强为带 `<system-reminder>` 的版本。

`conversationMessagesForTurn()` 在 `buildTurnMessages()` 时用增强消息替换 session 中的原文，实现"session 存原文、API 看增强"。

### 2.7 不需要改的地方

| 模块 | 为什么不需要改 |
|------|---------------|
| `llm.Message` | 不加 `IsMeta`，结构不变 |
| 会话统计/摘要 | `lastUserMessage()`、`CountMessageMetrics()` 只看到用户原文 |
| 压缩(compaction) | `firstUserGoal()`、`isHumanUserMessage()` 只看到用户原文 |
| TUI 渲染 | `rebuildSessionTimeline()` 只渲染用户原文，无 meta 消息 |
| 消息组装层 | `BuildMessages()` 不需要"合并 user message"逻辑，覆写机制已处理 |

---

## 三、删除直接 dispatch 路径

### 3.1 删除的代码

**文件：`tui/component_subagent_commands.go`**

删除函数：
- `runBuiltinSubAgentCommand()`
- `submitBuiltinSubAgentPreference()`
- `normalizeBuiltinSubAgentCommandInput()`
- `buildSubAgentThinkingBody()`
- `extractSubAgentTaskInput()`

**文件：`tui/component_slash_entry.go`**

删除 case 分支：
- `"/explorer"`
- `"/exploer"`
- `"/review"`

**文件：`tui/model.go`**

删除 `commandItems` 中：
- `{Name: "/review", ...}`
- `{Name: "/explorer", ...}`

删除 model 字段：
- `subAgentPending bool`
- `subAgentName string`
- `subAgentTask string`
- `subAgentStreamItems []chatEntry`
- `subAgentExpanded bool`

### 3.2 保留的代码

`DispatchSubAgent()` 方法保留在 `internal/agent/subagent_management.go`——供 `/agents` 命令和未来 programmatic API 使用，但不再从 slash 命令直接调用。

---

## 四、`/agents` 命令简化

### 4.1 删除带参数形式

**文件：`tui/component_subagent_commands.go`**

删除 `renderSubAgentDetail()` 函数。

**文件：`tui/component_slash_entry.go`**

`/agents` case 中，删除 `fields[1]` 分支，只保留无参数的列表展示：

```go
case "/agents":
    return m.runAgentsCommand(input) // 不再传 fields
```

### 4.2 保留列表展示

`runAgentsCommand()` 只保留 `runner.ListSubAgents()` + `renderSubAgentsView()`。不做 Picker Modal。

---

## 五、`delegate_subagent` 工具描述动态化

### 5.1 工具结构变更

**文件：`internal/tools/delegate_subagent.go`**

```go
type AgentInfo struct {
    Name        string
    Description string
}

type DelegateSubAgentTool struct {
    agents []AgentInfo
}

func NewDelegateSubAgentTool(agents []AgentInfo) DelegateSubAgentTool {
    return DelegateSubAgentTool{agents: agents}
}
```

### 5.2 动态 description

```go
func (t DelegateSubAgentTool) Definition() llm.ToolDefinition {
    desc := "Delegate a task to a specialized SubAgent."
    if len(t.agents) > 0 {
        desc += " Available agents:\n"
        for _, a := range t.agents {
            desc += fmt.Sprintf("- %s: %s\n", a.Name, a.Description)
        }
    }
    desc += "\nUse this tool when the user explicitly requests an agent (via @mention) or when a task matches an agent's specialization."
    desc += "\nWrite a detailed, self-contained task description including context, constraints, and expected output format."

    return llm.ToolDefinition{
        Type: "function",
        Function: llm.FunctionDefinition{
            Name:        "delegate_subagent",
            Description: desc,
            Parameters: map[string]any{
                // ... 参数定义不变 ...
            },
        },
    }
}
```

### 5.3 构造链路变更

**文件：`internal/tools/registry.go`**

```go
func DefaultRegistry(agentInfos []AgentInfo) *DefaultRegistryImpl {
    // ...
    reg.Register(NewDelegateSubAgentTool(agentInfos))
    // ...
}
```

**文件：`internal/app/bootstrap.go`**

```go
agentInfos := make([]tools.AgentInfo, 0)
for _, a := range subAgentManager.List() {
    agentInfos = append(agentInfos, tools.AgentInfo{
        Name:        a.Name,
        Description: a.Description,
    })
}
registry := tools.DefaultRegistry(agentInfos)
```

---

## 六、系统 prompt 增强

### 6.1 Agent 定义新增 `WhenToUse` 字段

**文件：`internal/subagents/types.go`**

```go
type Agent struct {
    // ... existing fields ...
    WhenToUse string // 新增：描述何时使用此 agent
}
```

**文件：`internal/subagents/frontmatter.go`**

解析 `when_to_use` 字段。

### 6.2 Agent 定义文件更新

**文件：`internal/subagents/explorer.md`**

```yaml
---
name: explorer
description: Read-only explorer agent for broad codebase discovery and file targeting.
when_to_use: Use when the user asks to find files, understand code structure, explore the codebase, or locate specific code patterns.
aliases: [explore]
tools: [read_file, list_files, search_files, search_text]
disallowed_tools: [delegate_subagent, run_shell, write_file, edit_file, delete_file]
mode: build
output: findings
isolation: none
---
```

**文件：`internal/subagents/review.md`**

```yaml
---
name: review
description: Read-only reviewer agent focused on defects, regressions, and test gaps.
when_to_use: Use when the user asks to review code, check for bugs, assess code quality, or identify missing tests.
tools: [read_file, list_files, search_files, search_text]
disallowed_tools: [delegate_subagent, run_shell, write_file, edit_file, delete_file]
mode: build
output: findings
isolation: none
---
```

### 6.3 PromptSubAgent 扩展

**文件：`internal/agent/prompt_subagents.go`**

```go
type PromptSubAgent struct {
    Name        string
    Description string
    WhenToUse   string // 新增
    Mode        string
}
```

`promptSubAgents()` 中提取 `WhenToUse` 字段。

### 6.4 `[Available SubAgents]` 渲染增强

**文件：`internal/agent/prompt.go`**

`formatSubAgents()` 改为：

```go
func formatSubAgents(agents []PromptSubAgent) string {
    // ...
    for _, agent := range agents {
        line := fmt.Sprintf("- %s: %s", name, description)
        if whenToUse := strings.TrimSpace(agent.WhenToUse); whenToUse != "" {
            line += "\n  " + trimPromptText(whenToUse, maxPromptSubAgentDescRune)
        }
        lines = append(lines, line)
    }
    // ...
}
```

### 6.5 `[Available SubAgents]` 头部引导增强

当前：

```
- SubAgents are optional delegated workers available in this session.
```

改为：

```
- SubAgents are optional delegated workers. Use the delegate_subagent tool to invoke them.
- When the user mentions an agent via @mention, delegate their task to that agent with a detailed, context-rich prompt.
```

### 6.6 渲染效果

```
[Available SubAgents]
- SubAgents are optional delegated workers. Use the delegate_subagent tool to invoke them.
- When the user mentions an agent via @mention, delegate their task to that agent with a detailed, context-rich prompt.
- explorer: Read-only explorer agent for broad codebase discovery and file targeting.
  Use when the user asks to find files, understand code structure, or explore the codebase.
- review: Read-only reviewer agent focused on defects, regressions, and test gaps.
  Use when the user asks to review code, check for bugs, or identify missing tests.
```

---

## 七、执行引擎增强

### 7.1 流式消息传递（子任务私有流）

#### 7.1.1 为什么不能复用全局 observer

当前链路有三个串流点，直接走全局 observer 会导致子 agent 事件混入父会话 TUI：

| 串流点 | 位置 | 问题 |
|--------|------|------|
| 子 runner 继承父 observer | `subagent_delegate.go:349` `Observer: r.observer` | 子 agent 的 `emit()` 直接调用父 observer |
| assistant_delta 不带 SessionID | `completion_runtime.go:35` `emit(Event{Type: EventAssistantDelta, Content: delta})` | `Event.SessionID` 字段存在但从不填充 |
| TUI 不做 session 过滤 | `component_run_flow.go:156` `handleAgentEvent()` | 直接 `appendAssistantDelta()`，无 session 比对 |

结论：流式机制必须用"子任务私有流"——子 agent 的事件通过闭包回调传给调用方，不经过全局 observer。

#### 7.1.2 接口设计

**文件：`internal/agent/subagent_executor.go`**

```go
type SubAgentStreamEvent struct {
    Type    string // "assistant_delta" | "tool_call_started" | "tool_call_completed"
    Content string
    Tool    string
}

type SubAgentExecutor interface {
    Execute(ctx context.Context, input SubAgentExecutionInput) (tools.DelegateSubAgentResult, error)
    ExecuteStreaming(ctx context.Context, input SubAgentExecutionInput, onEvent func(SubAgentStreamEvent)) (tools.DelegateSubAgentResult, error)
}
```

`ExecuteStreaming` 是唯一实现体，`Execute` 是纯便利包装：

```go
func (e *defaultSubAgentExecutor) Execute(ctx context.Context, input SubAgentExecutionInput) (tools.DelegateSubAgentResult, error) {
    return e.ExecuteStreaming(ctx, input, nil)
}

func (e *defaultSubAgentExecutor) ExecuteStreaming(
    ctx context.Context,
    input SubAgentExecutionInput,
    onEvent func(SubAgentStreamEvent),
) (tools.DelegateSubAgentResult, error) {
    childRunner := e.newSubAgentChildRunner(...)

    // 关键：挂载私有 observer，切断全局事件链
    if onEvent != nil {
        childRunner.SetObserver(ObserverFunc(func(ev Event) {
            onEvent(SubAgentStreamEvent{
                Type:    string(ev.Type),
                Content: ev.Content,
                Tool:    ev.ToolName,
            })
        }))
    }
    // onEvent == nil 时 childRunner.observer 保持 nil，子 agent 事件静默丢弃

    // ... 后续执行逻辑（prepareRunPrompt、runPromptTurns 等）同现有 Execute ...
}
```

#### 7.1.3 子 runner 不继承父 observer

**文件：`internal/agent/subagent_delegate.go`**

`newSubAgentChildRunner()` 不再传 `r.observer`：

```go
func (r *Runner) newSubAgentChildRunner(workspace string, maxTurns int) *Runner {
    cfg := r.config
    cfg.MaxIterations = resolveSubAgentMaxIterations(cfg.MaxIterations, maxTurns)
    return NewRunner(Options{
        // ... 其他字段同前 ...
        Observer: nil,  // 不传 observer，由 ExecuteStreaming 按需挂载
        // ...
    })
}
```

#### 7.1.4 调用方消费私有流

**文件：`internal/agent/subagent_delegate.go`**

`delegateSubAgent()` 前台路径走 `ExecuteStreaming()`：

```go
if !request.RunInBackground {
    result, err := executor.ExecuteStreaming(ctx, input, func(ev SubAgentStreamEvent) {
        // 调用方决定如何消费：
        // - 更新 TUI 的 subagent 进度面板
        // - 累积 assistant text 用于最终 summary
        // - 但不往父 observer 推事件，不污染主会话流
    })
}
```

后台路径走 `Execute(ctx, input)`（即 `ExecuteStreaming(ctx, input, nil)`），子 agent 事件静默丢弃。

#### 7.1.5 事件流对比

| | 当前链路 | 私有流方案 |
|---|---|---|
| 事件通道 | 共享 `r.observer`（全局） | 闭包 `onEvent`（调用方持有） |
| 传播方向 | 子 → 父 observer → TUI | 子 → `onEvent` → `delegateSubAgent` 内部消费 |
| TUI 是否感知 | 是，直接渲染子 agent 碎片 | 否，TUI 只看到最终 `DelegateSubAgentResult` |
| 后台执行 | 同上，仍然串流 | `onEvent=nil`，静默丢弃 |

#### 7.1.6 Runtime 层进度回调（可选）

**文件：`internal/runtime/manager.go`**

`TaskSpec` 新增：

```go
type TaskSpec struct {
    // ... existing ...
    OnProgress func(event []byte) // 新增：流式进度回调
}
```

此回调用于 Runtime 层面的任务进度上报（如长时间运行的子 agent 心跳），与 agent 层的 `onEvent` 是不同层级，互不干扰。

#### 7.1.7 子 Agent UI 渲染

子 agent 的执行过程和最终结果需要在 TUI 中正确展示。这依赖 TUI 工具渲染管道的整改：

- `delegate_subagent` 作为 `ToolRenderer` 实现接入统一渲染管道
- 执行中：`ProgressText()` 将子 agent 的工具调用序列压缩为摘要（如 "Searched 2 times, read 2 files"）
- 完成后：`ResultSummary()` 将 `DelegateSubAgentResult` 渲染为结构化的 findings/references
- `onEvent` 回调通过 `EventToolProgress` 事件将子 agent 的工具调用传递给渲染器

详细设计见 `docs/tui-tool-renderer-redesign.md`。

### 7.2 异步 Agent 工具白名单

**文件：`internal/subagents/gateway.go`**

`PreflightRequest` 新增：

```go
type PreflightRequest struct {
    // ... existing ...
    IsAsync bool // 新增
}
```

新增常量和过滤逻辑：

```go
var asyncAgentAllowedTools = map[string]struct{}{
    "read_file":   {},
    "list_files":  {},
    "search_text": {},
    "task_output": {},
    "task_stop":   {},
}

func (g *Gateway) Preflight(request PreflightRequest) (PreflightResult, error) {
    // ... existing logic ...

    // 异步 Agent 工具白名单过滤
    if request.IsAsync {
        filtered := make(map[string]struct{})
        for name := range workingSet {
            if _, allowed := asyncAgentAllowedTools[name]; allowed {
                filtered[name] = struct{}{}
            }
        }
        workingSet = filtered
    }

    // ... rest of logic ...
}
```

**文件：`internal/agent/subagent_delegate.go`**

调用 `Preflight()` 时传入 `IsAsync`：

```go
preflight, err := gateway.Preflight(subagentspkg.PreflightRequest{
    // ... existing ...
    IsAsync: request.RunInBackground,
})
```

### 7.3 自动后台化

**文件：`internal/runtime/manager.go`**

新增接口方法：

```go
type TaskManager interface {
    // ... existing ...
    DetachToBackground(id TaskID) (TaskID, error)
}
```

`InMemoryTaskManager` 实现：将同步等待的 task 标记为后台运行，返回新的 task ID。

**文件：`internal/agent/subagent_delegate.go`**

前台 `RunSync()` 增加超时监控：

```go
const autoBackgroundTimeout = 120 * time.Second

if !request.RunInBackground {
    done := make(chan struct{})
    var execution RuntimeTaskExecution
    var runErr error

    go func() {
        execution, runErr = r.runtime.RunSync(ctx, runtimeRequest)
        close(done)
    }()

    select {
    case <-done:
        // 正常完成，处理结果
    case <-time.After(autoBackgroundTimeout):
        // 自动后台化
        newID, detachErr := r.taskManager.DetachToBackground(execution.TaskID)
        if detachErr == nil {
            result.OK = true
            result.Status = subAgentResultStatusAccepted
            result.TaskID = string(newID)
            result.Summary = "SubAgent task automatically moved to background."
            return result, nil
        }
    }
}
```

---

## 八、Agent 定义扩展

### 8.1 新增字段

**文件：`internal/subagents/types.go`**

```go
type Agent struct {
    // ... existing fields ...

    WhenToUse      string // 何时使用此 agent（用于 prompt 引导）
    PermissionMode string // "auto" | "bubble" | "plan"
    Background     bool   // 定义级，总是后台运行
    OmitContext    bool   // 省略 AGENTS.md，只读 agent 节省 token
}
```

### 8.2 Frontmatter 解析

**文件：`internal/subagents/frontmatter.go`**

新增字段映射：

```go
agent.WhenToUse = trimOuterQuotes(fields["when_to_use"])
agent.PermissionMode = trimOuterQuotes(fields["permission_mode"])
agent.Background = strings.EqualFold(trimOuterQuotes(fields["background"]), "true")
agent.OmitContext = strings.EqualFold(trimOuterQuotes(fields["omit_context"]), "true")
```

### 8.3 `omitContext` 生效

**文件：`internal/agent/prompt.go`**

`prepareRunPrompt()` 中，检查 agent 定义的 `OmitContext`：

```go
if !input.OmitContext {
    instruction := loadAGENTSInstruction(input.Workspace)
    // ... 注入 AGENTS.md
}
```

### 8.4 子 Agent 禁用 thinking

**文件：`internal/agent/subagent_executor.go`**

`newSubAgentChildRunner()` 中：

```go
cfg.ThinkingEnabled = false // 子 Agent 默认禁用 thinking
```

---

## 九、用户自定义 SubAgent 执行链路

### 9.1 存储位置

三个 scope，按优先级从低到高：

| Scope | 路径 | 用途 |
|-------|------|------|
| `user` | `~/.bytemind/agents/*.md` | 用户全局自定义 agent，所有项目共享 |
| `project` | `<workspace>/.bytemind/agents/*.md` | 项目级 agent，跟随项目仓库 |
| `project` | `<workspace>/.agents/agents/*.md` | 项目级 agent（备选路径，兼容 `.agents` 目录约定） |

同名 agent 按加载顺序覆盖：`user` → `.bytemind/agents` → `.agents/agents`，后者覆盖前者。

### 9.2 Agent 定义文件格式

用户创建一个 `.md` 文件，使用 YAML frontmatter 定义 agent 元数据：

```markdown
---
name: my-test-writer
description: Write unit tests for Go packages based on existing code.
when_to_use: Use when the user asks to write tests, improve test coverage, or add missing test cases.
tools: [read_file, list_files, search_text, write_file, edit_file]
disallowed_tools: [delegate_subagent, run_shell]
mode: build
output: findings
max_turns: 10
---

You are a test writer specialized in Go.

Your job is to:
1. Read the target package and understand its public API
2. Write comprehensive table-driven tests
3. Cover edge cases, error paths, and boundary conditions
4. Follow existing test patterns in the codebase

Return a summary of files created and test coverage expectations.
```

### 9.3 Frontmatter 字段说明

| 字段 | 必填 | 说明 |
|------|------|------|
| `name` | 否 | agent 名称，省略时取文件名（不含 `.md`）。必须匹配 `^[A-Za-z0-9][A-Za-z0-9._:-]*$` |
| `description` | 否 | 一句话描述，用于 `@` 补全和 `[Available SubAgents]` 展示 |
| `when_to_use` | 否 | 描述何时使用此 agent，注入系统 prompt 引导 LLM 决策 |
| `tools` | 否 | 允许使用的工具列表，空则继承默认工具集 |
| `disallowed_tools` | 否 | 禁止使用的工具列表 |
| `mode` | 否 | `build`（默认）或 `plan` |
| `output` | 否 | 输出类型，如 `findings` |
| `max_turns` | 否 | 最大执行轮数，默认使用全局配置 |
| `model` | 否 | 指定使用的模型，空则继承父 agent |
| `timeout` | 否 | 执行超时 |
| `isolation` | 否 | 隔离模式 |
| `aliases` | 否 | 别名列表，用于 `@` mention 匹配 |
| `permission_mode` | 否 | `auto` / `bubble` / `plan`，控制工具调用审批策略 |
| `background` | 否 | `true` 则始终后台运行 |
| `omit_context` | 否 | `true` 则省略 AGENTS.md 注入，节省 token |

frontmatter 之后的 Markdown 正文作为 agent 的 `Instruction`，注入子 agent 的系统 prompt。

### 9.4 发现与加载流程

```
应用启动 / Reload()
  │
  ├── 扫描用户目录: ~/.bytemind/agents/*.md
  │     └── scope = "user"
  │
  ├── 扫描项目目录 A: <workspace>/.bytemind/agents/*.md
  │     └── scope = "project"
  │
  └── 扫描项目目录 B: <workspace>/.agents/agents/*.md
        └── scope = "project"
```

每个目录独立扫描，只读取 `.md` 文件，跳过子目录。加载后合并到统一 Catalog，同名覆盖。

### 9.5 端到端执行流程

```
1. 用户创建文件
   ~/.bytemind/agents/my-test-writer.md

2. 应用启动，Manager.Reload() 扫描并加载
   → Catalog.Agents 中包含 my-test-writer
   → Scope = "user", SourcePath = "~/.bytemind/agents/my-test-writer.md"

3. 用户输入框输入: @my-test-writer
   → mention 补全弹出: "* my-test-writer - Write unit tests for Go packages..."
   → 用户选择或继续打字

4. 用户按 Enter 提交: "@my-test-writer write tests for auth package"

5. Session 存储原文（不修改）

6. prepareRunPrompt() 阶段:
   → extractAgentMentions() 检测到 @my-test-writer
   → enhanceUserMessageWithAgentMentions() 构建增强消息
   → UserMessage = 原文 + <system-reminder>（提示 LLM 使用 delegate_subagent）

7. buildTurnMessages() 阶段:
   → conversationMessagesForTurn() 用增强消息替换 session 中的原文

8. 父 Agent LLM turn:
   → LLM 看到增强后的消息
   → 决策: 调用 delegate_subagent(agent: "my-test-writer", task: "write tests for auth package")

9. delegate_subagent 执行:
   → Gateway.Preflight() 校验工具权限
   → SubAgentExecutor.ExecuteStreaming() 创建子 runner
   → 子 runner 加载 my-test-writer 的 Instruction + 工具配置
   → 子 agent 执行任务，返回 DelegateSubAgentResult

10. 父 Agent 收到结果，继续后续处理
```

### 9.6 覆盖规则

当多个 scope 存在同名 agent 时：

| 场景 | 行为 |
|------|------|
| `user/` 有 `review.md`，`builtin` 有 `review` | 用户版本覆盖内置版本 |
| `.bytemind/agents/review.md` 和 `.agents/agents/review.md` 同时存在 | 后扫描的 `.agents/agents/` 覆盖 `.bytemind/agents/` |
| `user/` 和 `.bytemind/agents/` 都有 `my-agent.md` | 项目级覆盖用户级 |

`/agents` 命令输出中会显示覆盖信息（`Override` 记录），提示用户哪个 scope 的定义生效。

### 9.7 与内置 Agent 的差异

| | 内置 Agent | 用户自定义 Agent |
|---|---|---|
| 存储 | `internal/subagents/*.md` + hardcoded | `~/.bytemind/agents/*.md` 等 |
| Scope | `builtin` | `user` / `project` |
| 覆盖 | 可被 user/project 覆盖 | 可覆盖 builtin，互相覆盖 |
| `@` 补全 | 始终可用 | 始终可用 |
| 持久化 | 随代码发布 | 用户自行管理 |
| 热更新 | 需重新编译 | `Reload()` 即时生效 |

### 9.8 实现变更

**文件：`internal/subagents/manager.go`**

`NewManager()` 中更新目录路径：

```go
func NewManager(workspace string) *Manager {
    userDir := ""
    // ... home dir resolution ...

    userDir = filepath.Join(home, ".bytemind", "agents") // 改为 agents

    manager := NewManagerWithDirs(
        workspace,
        filepath.Join(workspace, "internal", "subagents"),
        userDir,
        "", // projectDir 拆分为两个
    )
    // ...
}
```

新增项目级双目录扫描：

```go
func (m *Manager) Reload() Catalog {
    scopes := []struct {
        scope Scope
        dir   string
    }{
        {scope: ScopeBuiltin, dir: m.builtinDir},
        {scope: ScopeUser, dir: m.userDir},
        {scope: ScopeProject, dir: filepath.Join(m.workspace, ".bytemind", "agents")},
        {scope: ScopeProject, dir: filepath.Join(m.workspace, ".agents", "agents")},
    }
    // ... 后续加载逻辑不变 ...
}
```

注意：`.agents/agents/` 路径仅作为项目级的补充扫描目录，不需要在 `Manager` 结构体中新增字段——直接在 `Reload()` 中拼接即可。

---

## 十、改动总览

### 新建文件

| 文件 | 用途 |
|------|------|
| `internal/mention/agent_source.go` | Agent 候选源接口 |
| `internal/agent/agent_mentions.go` | Mention 提取 + system-reminder 构建 + 增强消息构建 |
| TUI 渲染管道（5 个文件） | 见 `docs/tui-tool-renderer-redesign.md`：ToolRenderer 接口、内置渲染器、delegate_subagent 渲染器、跨调用压缩器 |

### 修改文件

| 文件 | 改动 |
|------|------|
| `internal/mention/index.go` | `Candidate` 新增 `Kind`、`Description` |
| `internal/subagents/types.go` | `Agent` 新增 `WhenToUse`、`PermissionMode`、`Background`、`OmitContext` |
| `internal/subagents/frontmatter.go` | 解析新字段 |
| `internal/subagents/manager.go` | 目录路径变更为 `~/.bytemind/agents/`、`.bytemind/agents/`、`.agents/agents/`；`Reload()` 新增 `.agents/agents` 双项目目录扫描 |
| `internal/subagents/gateway.go` | `PreflightRequest` 新增 `IsAsync` + 异步工具白名单过滤 |
| `internal/subagents/explorer.md` | 新增 `when_to_use` |
| `internal/subagents/review.md` | 新增 `when_to_use` |
| `internal/tools/delegate_subagent.go` | `DelegateSubAgentTool` 动态 description |
| `internal/tools/registry.go` | `DefaultRegistry()` 接受 agent 列表 |
| `internal/app/bootstrap.go` | 传入 agent 列表给 registry |
| `internal/agent/prompt.go` | `[Available SubAgents]` 头部引导增强 + `formatSubAgents()` 渲染 `WhenToUse` + `omitContext` 生效 |
| `internal/agent/prompt_subagents.go` | `PromptSubAgent` 新增 `WhenToUse` |
| `internal/agent/subagent_executor.go` | `ExecuteStreaming()` 为唯一实现体，`Execute()` 为便利包装；子 runner observer 设为 nil，由 `ExecuteStreaming` 按需挂载私有 observer |
| `internal/agent/subagent_delegate.go` | `newSubAgentChildRunner()` 不传 `r.observer`；前台调用走 `ExecuteStreaming()` + 私有 `onEvent` 回调；异步 `IsAsync` 传参 + 自动后台化 |
| `internal/runtime/manager.go` | `TaskSpec.OnProgress` + `TaskManager.DetachToBackground()` |
| `tui/model.go` | 新增 `agentSource` 字段；删除 `commandItems` 中 `/review`、`/explorer`；删除 `subAgentPending`、`subAgentName`、`subAgentTask`、`subAgentStreamItems`、`subAgentExpanded` |
| `tui/component_palette_runtime.go` | `syncMentionPalette()` 合并 agent 候选；选中 agent 插入 `@name` |
| `tui/component_palettes.go` | `renderMentionPalette()` 符号渲染 `*` / `+` |
| `tui/component_slash_entry.go` | 删除 `/explorer`、`/exploer`、`/review` case；`/agents` 移除带参数分支 |
| `tui/component_subagent_commands.go` | 删除 `runBuiltinSubAgentCommand()`、`submitBuiltinSubAgentPreference()`、`normalizeBuiltinSubAgentCommandInput()`、`buildSubAgentThinkingBody()`、`extractSubAgentTaskInput()`、`renderSubAgentDetail()` |
| `tui/component_run_flow.go` | `handleAgentEvent()` 改为通过 `GetToolRenderer()` 分发；新增 `EventToolProgress` 处理（见 `docs/tui-tool-renderer-redesign.md`） |
| `internal/agent/events.go` | Event 新增 `ToolCallID`；新增 `EventToolProgress` 事件类型 |
| `tui/ports.go` | 同步新增 `ToolCallID`、`EventToolProgress` |

### 删除的代码

| 文件 | 删除内容 |
|------|---------|
| `tui/component_subagent_commands.go` | `runBuiltinSubAgentCommand()`、`submitBuiltinSubAgentPreference()`、`normalizeBuiltinSubAgentCommandInput()`、`buildSubAgentThinkingBody()`、`extractSubAgentTaskInput()`、`renderSubAgentDetail()` |
| `tui/component_slash_entry.go` | `/explorer`、`/exploer`、`/review` case 分支 |
| `tui/model.go` | `commandItems` 中 `/review`、`/explorer` 条目；model 字段 `subAgentPending`、`subAgentName`、`subAgentTask`、`subAgentStreamItems`、`subAgentExpanded`；`summarizeTool()` 函数（迁移到各工具 ToolRenderer 后删除） |

### 优先级

| 优先级 | 编号 | 改动 |
|--------|------|------|
| P0 | 1.1-1.5 | `@` mention 系统扩展（agent 候选 + 符号渲染） |
| P0 | 2.1-2.7 | Agent mention → system-reminder 注入（RunPromptInput 覆写） |
| P0 | 3.1-3.2 | 删除直接 dispatch 路径 |
| P0 | 4.1-4.2 | `/agents` 简化 |
| P0 | 5.1-5.3 | `delegate_subagent` 工具描述动态化 |
| P0 | 6.1-6.6 | 系统 prompt 增强（WhenToUse + 引导） |
| P0 | 9.1-9.8 | 用户自定义 SubAgent 存储路径 + 执行链路（路径变更 + 双项目目录扫描 + 覆盖规则） |
| P1 | 7.1 | 流式消息传递（子任务私有流：ExecuteStreaming 唯一实现 + observer 断开 + onEvent 回调） |
| P1 | 7.1.7 | 子 Agent UI 渲染（依赖 TUI 工具渲染管道整改，见 `docs/tui-tool-renderer-redesign.md`） |
| P1 | 7.2 | 异步 Agent 工具白名单 |
| P1 | 8.1-8.4 | Agent 定义扩展 + omitContext + 禁用 thinking |
| P2 | 7.3 | 自动后台化 |
