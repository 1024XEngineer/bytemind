# 同轮多工具调用并行执行 — 并发设计文档 v2

## 1. 设计目标

将 `processTurn()` 中的工具调用执行从 **严格串行** 改为 **无依赖并行 + 有依赖串行**，使 LLM 在同一轮 assistant message 中发出的多个 `delegate_subagent`（或其他无依赖工具调用）可以并发执行，将端到端时延从 `ΣT` 降为 `max(T)`。

### 适用场景

- 父 LLM 同时委派 `@explorer` 查前端 + `@explorer` 查后端 + `@review` 审查
- 父 LLM 同时发出多个只读工具调用（`read_file` + `search_text`）

### 非目标（本期不做）

- 跨轮并行：仅限同一轮内的多个 tool_call
- 跨会话共享：不改变子代理间隔离模型
- 参数级依赖分析：不分析文件路径重叠，仅依赖工具类型判定

---

## 2. 现状分析

### 2.1 当前执行路径

```
runPromptTurns()                           # engine_run_loop.go:21
  for step := 0; step < max; step++ {
    processTurn()                          # turn_processing.go:38
      reply = LLM.Chat(messages, tools)    # 单次 API 调用，可返回多个 ToolCall
      for _, call := range reply.ToolCalls {  # turn_processing.go:150 ← 串行关键路径
        executeToolCall(call)              # tool_execution.go:61
          → runner.delegateSubAgent()      # 进入子代理执行
          → sess.Messages = append(...)    # 追加工具结果消息
          → runner.store.Save(sess)        # 持久化
      }
    }
  }
```

### 2.2 关键约束与陷阱

| 约束 | 详情 | 影响 |
|------|------|------|
| 协议已支持多 tool_call | LLM 单轮可返回多个 `ToolCall` | 无需协议层改动 |
| 串行瓶颈 | `turn_processing.go:150` 的 `for range` 循环逐个执行 | 改动集中点 |
| 会话非并发安全 | `session.Session.Messages` 无锁，直接追加 | 需提取副作用 |
| `ConcurrencySafe` 默认值为 `true` | `spec.go:74`，未显式声明的工具（含扩展工具）均默认可并行 | **并行判定不能用此字段做主判据** |
| `run_shell` 未显式设 `ConcurrencySafe=false` | `spec.go:86-87`，保留默认 `true` | **两个 run_shell 会被误判为可并行** |
| AgentID 不是唯一键 | `events.go:54` 用 agent 名称；两个 `@explorer` 有相同 AgentID | **TUI 事件路由会错配** |

### 2.3 现有的并发基础设施

- `runtime.TaskManager`：goroutine-safe，支持 `Submit`/`Wait`/`Cancel`
- `runtime.RunSync()`：已封装 submit + wait 模式
- `sync.Map`（`subagentToolCallsStore`）：侧通道存储，key 为 `invocationID`
- `SubAgentNotifier`：带 `sync.Mutex` 的通知队列，支持跨 goroutine 推送
- `InvocationID`：`DelegateSubAgentResult` 中已有全局唯一 ID（`subagent_delegate.go:55` 格式为 `subagent-{unix-nano}-{counter}`）
- **Observer 通道架构（关键发现）**：当前 TUI observer 已通过 `async` channel 投递事件：
  ```go
  // model.go:521
  async := make(chan tea.Msg, 128)
  // model.go:554-555
  opts.Runner.SetObserver(func(event Event) {
      async <- agentEventMsg{Event: event}  // chan 支持多 goroutine 并发写
  })
  // model.go:792-793 — bubbletea 保证 Update 单线程
  case agentEventMsg:
      m.handleAgentEvent(msg.Event)
  ```
  **这意味着并行 goroutine 可以直接 emit 事件，不需要延迟到 `wg.Wait()` 之后。** Go channel 原生支持并发发送，bubbletea 的 `Update()` 单线程消费。整个链路已经是 Plan C。

---

## 3. 核心设计

### 3.1 并行执行模型

```
processTurn()
  reply = LLM.Chat(messages, tools)
  // Phase A: 分析依赖，划分并行组
  groups = partitionForParallelExecution(reply.ToolCalls)
  // Phase B: 同步发出所有 EventToolCallStarted（TUI 立即显示每个子代理的 running 条目）
  for each call in reply.ToolCalls {
      emit(EventToolCallStarted{InvocationID: ..., ...})
  }
  // Phase C: 组间串行，组内并行
  for each group in groups {
      if len(group) == 1  → 串行执行
      else                → executeParallel(group)
            ↓ 并行 goroutine 直接在完成时 emit(EventToolCallCompleted)
            ↓ 事件通过 async channel → bubbletea Update() → TUI 实时更新
  }
  // Phase D: 批量持久化（一次 Save）
  runner.store.Save(sess)
```

**关键设计点：**
- `EventToolCallStarted` 在启动 goroutine 前同步发出 → TUI entry 先创建，后续事件可精确定位
- 并行 goroutine 直接通过 `runner.emit()` 发送 `EventToolCallCompleted` → 经过 `async` channel → bubbletea `Update()` 安全消费。**事件实时到达，TUI 能看到子代理内部工具调用的逐步推进**
- `sess.Messages` 追加和 `store.Save` 在 `wg.Wait()` 后由主 goroutine 执行（唯一需要串行的部分）

### 3.2 独立性判断 — 显式可并行白名单

**问题诊断：** `ConcurrencySafe` 默认值为 `true`（`spec.go:74`），扩展工具和 `run_shell` 均会被误判。不能用它做主判据。

**修正方案：** 不依赖 `ConcurrencySafe` 字段，改用**显式白名单**：

```go
// 可并行的工具名称集合（编译期确定）。
// v1 只放行 delegate_subagent + 纯本地只读工具。
// web_search/web_fetch 暂不列入（依赖外部 API，并行可能触发 API rate limit）。
var toolsParallelizable = map[string]bool{
    "delegate_subagent": true,  // 独立 session，天然隔离
    "list_files":        true,  // 只读，纯本地
    "read_file":         true,  // 只读，纯本地
    "search_text":       true,  // 只读，纯本地
}
// 未列入白名单的工具（write_file, replace_in_file, apply_patch, run_shell,
// web_search, web_fetch 及所有扩展工具）→ 默认串行，保守策略。
```

**规则总结：**

| 工具 | 同组并行 | 依据 |
|------|---------|------|
| `delegate_subagent` | ✅ | 独立 session，天然隔离 |
| `list_files`, `read_file`, `search_text` | ✅ | 只读，纯本地，无副作用 |
| `web_search`, `web_fetch` | ❌ v1 不并行 | 依赖外部 API，并行可能触发 API rate limit |
| `write_file`, `replace_in_file`, `apply_patch` | ❌ | 可能修改同一文件 |
| `run_shell` | ❌ | 可能有文件/进程副作用 |
| 任何未知扩展工具 | ❌ | 保守：不知道行为 → 不并行 |

**分区算法：**

```go
func partitionForParallelExecution(toolCalls []llm.ToolCall) [][]indexedToolCall {
    groups := make([][]indexedToolCall, 0)
    current := make([]indexedToolCall, 0)

    for i, call := range toolCalls {
        ic := indexedToolCall{Index: i, ToolCall: call}
        name := call.Function.Name

        if toolsParallelizable[name] {
            current = append(current, ic)
            // 并行组达到上限：截断当前组，剩余单独成组
            if len(current) >= maxParallelToolCalls {
                groups = append(groups, current)
                current = nil
            }
        } else {
            // 不可并行工具：结束当前并行组（如果非空），单独成组
            if len(current) > 0 {
                groups = append(groups, current)
                current = nil
            }
            groups = append(groups, []indexedToolCall{ic})
        }
    }
    if len(current) > 0 {
        groups = append(groups, current)
    }
    return groups
}
```

**关键行为：**
- 白名单工具聚合成组，组大小上限 `maxParallelToolCalls`（4），超出截断为新组
- 非白名单工具结束当前并行组后单独成组（组间串行）
- 示例：`[read, read, write, read]` → `[[read, read], [write], [read]]` — 写操作隔断了前后读操作的并行
- 示例：`[read, read, read, read, read]` → `[[read×4], [read×1]]` — 截断非全降级

### 3.3 并发上限

```go
const maxParallelToolCalls = 4
```

每个并行组最多 4 个工具。超出部分**截断为新的并行组**而非全部降级串行。理由：
- `delegate_subagent` 每个创建独立 LLM 会话 → N 路并发 API 调用，4 路是合理上限
- 截断策略避免 4→5 时的行为断层：4 个并行快，5 个变成前面 4 个并行 + 后面 1 个接续，而非 5 倍慢
- 示例：同轮 6 个白名单工具 → `[[4 个并行], [2 个并行]]`，两个并行组之间串行

### 3.4 会话状态并发安全

**问题：** `executeToolCall()` 当前操作 `sess.Messages`（`tool_execution.go:334`）和 `runner.store.Save(sess)`（`:336`），在并行 goroutine 中不安全。

**方案：副作用分类处理**

`executeToolCall()` 内的副作用实际只分两类：

| 分类 | 副作用 | goroutine 安全？ | 处理方式 |
|------|--------|:---:|------|
| **可并发** | `runner.emit(...)` — observer 事件 | ✅ Go channel 原生支持并发 send | **不动**，保留在 `executeToolCall` 内 |
| **可并发** | `runner.appendAudit(...)` — 审计日志 | ✅ audit store 内部有锁 | **不动** |
| **可并发** | `io.WriteString(out, ...)` — stdout | ✅ 单行输出，可接受短期交织 | **不动** |
| **必须串行** | `sess.Messages = append(...)` | ❌ slice 无锁 | 提取到主 goroutine，`wg.Wait()` 后按序追加 |
| **必须串行** | `runner.store.Save(sess)` | ❌ 依赖 session 完整状态 | 提取到主 goroutine，每轮一次批量保存 |

**关键洞察：** observer 事件不需要提取。当前 TUI 的 observer 是一个闭包 `func(event Event) { async <- agentEventMsg{Event: event} }`（`model.go:554-555`），将事件写入 buffered channel。Go channel 支持多 goroutine 并发 send，bubbletea 的 `Update()` 单线程消费。并行 goroutine 在完成时立即 emit 事件是安全的，TUI 能看到实时进度。

提取到主 goroutine 的只有两项：`sess.Messages` 追加和 `store.Save`。

```go
type toolCallResult struct {
    Index        int
    ToolCallID   string
    ToolName     string
    ToolMessage  llm.Message      // 包含 LLM payload 和 meta
    Error        error            // 框架级错误 — nil 表示工具执行完成
}
```

`executeToolCall()` 返回 `(toolCallResult, error)`，observer/audit/stdout 副作用已在函数内完成。

### 3.5 错误处理 — 区分框架错误与业务错误

**两层错误模型：**

```
executeToolCall() 返回 (result, error):
  error != nil  → 框架级错误（runtime 不可用、policy gateway 故障、store 写入失败）
                  → processTurn 立即返回 error，终止整个 turn
  error == nil  → 工具执行完成（result.ToolMessage 中可能包含业务错误，如文件未找到）
                  → 正常追加到 sess.Messages，LLM 自行判断
```

这保留了当前 `turn_processing.go:163` 的语义：基础设施级错误会终止 turn。

```go
func (e *defaultEngine) executeToolCallsParallel(ctx context.Context, ...) ([]toolCallResult, error) {
    n := len(calls)
    results := make([]toolCallResult, n)
    var firstFatal error
    var mu sync.Mutex
    var wg sync.WaitGroup
    wg.Add(n)

    for i, call := range calls {
        go func(idx int, c indexedToolCall) {
            defer wg.Done()
            result, err := e.executeToolCall(ctx, sess, runMode, c.ToolCall, ...)
            result.Index = c.Index
            results[idx] = result

            if err != nil {
                mu.Lock()
                if firstFatal == nil {
                    firstFatal = err  // 记录第一个框架错误
                }
                mu.Unlock()
            }
        }(i, call)
    }

    wg.Wait()

    if firstFatal != nil {
        return results, firstFatal  // 框架错误 → 终止 turn
    }
    return results, nil
}
```

| 场景 | 行为 |
|------|------|
| 所有工具成功 | 所有结果追加到 sess.Messages |
| 一个子代理找不到文件 | 该结果含 `ok:false` → 继续，所有结果返回 LLM |
| 一个子代理 runtime 不可用 | `firstFatal` 被设置 → `processTurn` 返回 error |
| 用户按 Escape 中止 | ctx 取消 → goroutine 退出 → `wg.Wait()` 返回 → error 传播 |

### 3.6 TUI 事件路由 — 用 InvocationID 替代 AgentID

**问题诊断：** AgentID = agent 名称（`"explorer"`），两个同类型子代理并行时 AgentID 碰撞，`findActiveSubAgentEntryByID` 返回错误的 entry。

**已有基础：** 当前 TUI 的事件通道架构（2.3 节所述）已经是 Plan C — 所有事件通过 `async` channel 进入 bubbletea `Update()`，并发安全已保证。唯一需要补充的是路由键的唯一性。

**修正方案：** 用 `InvocationID`（全局唯一）做路由键。

**Step 1 — Event 增加 InvocationID：**

`internal/agent/events.go` 和 `tui/ports.go`：

```go
type Event struct {
    // ... 现有字段 ...
    AgentID       string // 保留：用于 TUI 分组渲染
    InvocationID  string // 新增：精确事件路由键（全局唯一）
}
```

**Step 2 — delegate_subagent 注入 InvocationID 到 Observer：**

```go
// subagent_delegate.go delegateSubAgent() 中：
result.InvocationID = newSubAgentInvocationID()  // 已有，无需新增

// tool_execution.go:181 — 传入：
observer := SubAgentObserver(runner.observer, req.Agent, result.InvocationID)
```

**Step 3 — TUI 用 InvocationID 精匹配：**

```go
// events.go — SubAgentObserver 注入 InvocationID：
func SubAgentObserver(inner Observer, agentID, invocationID string) Observer {
    return ObserverFunc(func(event Event) {
        event.AgentID = agentID
        event.InvocationID = invocationID
        inner.HandleEvent(event)
    })
}

// component_run_flow.go — 创建 entry 时记录 InvocationID：
newEntry.InvocationID = event.InvocationID

// component_run_flow.go — handleSubAgentEvent 优先用 InvocationID 匹配：
entry := m.findActiveSubAgentEntryByInvocation(event.InvocationID)  // 精匹配
if entry == nil {
    entry = m.findActiveSubAgentEntryByID(event.AgentID)  // fallback
}
```

AgentID 保留用于 TUI 的**分组聚合**（`isSubAgentGroup` 按 AgentID 聚合同类型），**事件路由**用 InvocationID 精匹配。

---

## 4. 实现方案

### Phase 1：扩展 Event 结构体 + SubAgentObserver

**文件：** `internal/agent/events.go`, `tui/ports.go`, `internal/agent/subagent_delegate.go`

- Event 增加 `InvocationID string` 字段
- `SubAgentObserver` 签名增加 `invocationID` 参数
- `delegateSubAgent()` 和 `tool_execution.go:181` 传 InvocationID

**变更量：** ~15 行修改

### Phase 2：添加分组函数和并发执行函数

**文件：** `internal/agent/turn_processing.go`

新增 `partitionForParallelExecution()`（~45 行）和 `executeToolCallsParallel()`（~55 行）。

### Phase 3：重构 `processTurn()` 工具执行循环

**文件：** `internal/agent/turn_processing.go`

将 `:150-175` 的串行循环替换为分组并行逻辑（~70 行），同时：
- `EventToolCallStarted` 在启动 goroutine 前同步发出，携带 `InvocationID`（确保 TUI entry 先创建，后续事件可精确定位）
- 并行 goroutine 中 `executeToolCall` 在完成时通过 channel observer 实时 emit `EventToolCallCompleted`（TUI 看到逐步推进）
- `wg.Wait()` 后主 goroutine 按原始顺序追加 `sess.Messages` + 调用一次 `store.Save(sess)`
- 框架级 error 检查（`firstFatal` 传播）

### Phase 4：重构 `executeToolCall()` 提取会话状态副作用

**文件：** `internal/agent/tool_execution.go`

- 函数签名改为返回 `(toolCallResult, error)`
- `sess.Messages = append(...)` → 移除，改为返回 `toolCallResult.ToolMessage`（~5 行删除）
- `store.Save(sess)` → 移除，由 `processTurn` 在 `wg.Wait()` 后统一调用（~3 行删除）
- Observer emit、audit append、stdout 输出保留在函数内（channel 架构保证并发安全）
- 返回 `toolCallResult` 结构体

**变更量：** ~20 行修改，~10 行删除

### Phase 5：TUI 路由适配

**文件：** `tui/component_run_flow.go`, `tui/model.go`

- `chatEntry` 增加 `InvocationID string` 字段
- `handleAgentEvent` 解析 `event.InvocationID` 并写入 `newEntry`
- 新增 `findActiveSubAgentEntryByInvocation()`
- `handleSubAgentEvent` 优先用 InvocationID 匹配

**变更量：** ~30 行新增 + 10 行修改

### Phase 6：提示词更新

**文件：** `internal/agent/prompt.go`

在 `[Available SubAgents]` 块中增加并行指导：

```
- When investigating multiple independent areas, delegate to multiple
  subagents in a single turn. Independent subagents run concurrently.
```

### Phase 7：并发上限

**文件：** `internal/agent/turn_processing.go`

```go
const maxParallelToolCalls = 4
```

在 `partitionForParallelExecution` 中截断，每组最多 4 个，超出部分进入下一并行组。

---

## 5. 变更文件清单

| 文件 | 变更类型 | 描述 | 估计行数 |
|------|---------|------|---------|
| `internal/agent/turn_processing.go` | 重写循环 + 新增函数 | 并行执行循环、分区函数、并发上限 | +170, -20 |
| `internal/agent/tool_execution.go` | 重构 | 返回 `toolCallResult`，移除 session 副作用 | +20, -10 |
| `internal/agent/events.go` | 修改 | Event 增加 InvocationID、SubAgentObserver 增加参数 | +8, -3 |
| `internal/agent/subagent_delegate.go` | 修改 | 传 InvocationID 给 SubAgentObserver | +2, -1 |
| `internal/agent/prompt.go` | 修改 | 增加并行使用指导 | +8 |
| `tui/ports.go` | 修改 | Event 增加 InvocationID | +2 |
| `tui/component_run_flow.go` | 修改 + 新增 | InvocationID 路由、entry 创建 | +25, -2 |
| `tui/model.go` | 修改 | chatEntry 增加 InvocationID | +2 |
| `internal/agent/turn_processing_test.go` | 新增 | 分区逻辑 + 并行执行 + 并发上限测试 | +120 |
| `tui/component_chat_stream_test.go` | 新增 | InvocationID 路由测试（同类型并行） | +30 |

**估计总变更量：** ~390 行新增，~50 行修改，~40 行删除。

---

## 6. 风险与缓解

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|---------|
| **并发上限触发部分串行** | 低 — LLM 极少同轮发 5+ tool_call | 低 | 截断策略：前 4 个并行，剩余串行。非全降级 |
| **Context 取消传播** | 低 | 中 — 子代理不响应取消 | 所有并行 goroutine 共享父 ctx，`runtime.TaskManager` 已支持 ctx 取消传播 |
| **令牌用量估算偏差** | 低 | 低 | `wg.Wait()` 后统一记录 |
| **扩展工具被误判为不可并行** | 低 — 保守策略安全但保守 | 低 | 未来通过注册时声明属性来放开 |
| **Provider rate limit** | 中 | 中 | `maxParallelToolCalls = 4` 限制并发 LLM 会话数 |
| **TUI 事件乱序** | 极低 — `async` channel 已序列化 | 低 | 无需额外缓解，现有通道架构已保证 |

---

## 7. 测试策略

### 7.1 单元测试

| 测试用例 | 内容 |
|---------|------|
| `TestPartitionForParallelExecution_AllParallelizable` | 全部白名单工具 → 单个并行组 |
| `TestPartitionForParallelExecution_Mixed` | 并行 + 串行混合 → 多个组 |
| `TestPartitionForParallelExecution_OnlySequential` | 全部写/Shell工具 → 每组一个 |
| `TestPartitionForParallelExecution_UnknownTool` | 未知扩展工具 → 单独串行组 |
| `TestPartitionForParallelExecution_ExceedsCap` | 5 个白名单工具 → `[[4 并行], [1 串行]]`，截断非全降级 |
| `TestExecuteParallel_AllSuccess` | 所有并行工具成功，结果按序返回 |
| `TestExecuteParallel_PartialToolFailure` | 一个子代理业务失败 → 其他继续，全部结果返回 |
| `TestExecuteParallel_FatalError` | 一个子代理 runtime 不可用 → firstFatal 传播，终止 turn |
| `TestExecuteParallel_ContextCanceled` | ctx 取消 → goroutine 退出 |
| `TestInvocationIDRouting_SameTypeAgents` | 两个 explorer 并行 → 事件各自路由到正确 entry |

### 7.2 集成测试

- LLM 模拟返回同轮 `[delegate_subagent(explorer), delegate_subagent(review)]`，验证并行执行
- 验证并行执行后 `sess.Messages` 中的工具结果顺序与 `reply.ToolCalls` 一致
- 验证 InvocationID 在 Event → TUI entry 链路中正确传递

### 7.3 手动验证

1. `@explorer 查前端代码 @explorer 查后端代码` → 两个子代理同时启动，TUI 各自独立显示
2. 一个子代理出错（文件未找到）→ 另一个正常完成，LLM 看到两个结果
3. 按 Escape 中止 → 所有并行子代理被取消
4. 同轮 `delegate_subagent` + `write_file` → 串行执行，写操作等子代理完成

---

## 8. 向后兼容性

- **API 兼容：** `delegate_subagent` 工具签名不变
- **会话格式兼容：** `sess.Messages` 的消息顺序语义不变
- **TUI 兼容：** AgentID 保留用于聚合渲染；InvocationID 仅用于事件路由
- **配置兼容：** `maxParallelToolCalls` 为编译期常量，无用户侧配置项
