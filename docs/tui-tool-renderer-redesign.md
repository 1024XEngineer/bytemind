# TUI 工具渲染管道整改设计

## 概述

当前 ByteMind 的工具渲染采用集中式 switch-case（`summarizeTool()`），存在三个结构性问题：

1. **无跨调用压缩**：连续调用同类工具（如连续读 5 个文件），每条独立渲染，刷屏
2. **新工具必须改集中函数**：每加一个工具就要往 `summarizeTool()` 里塞一个 case
3. **长时间运行的工具无中间进度**：`EventToolCallStarted` 到 `EventToolCallCompleted` 之间 Body 为空

整改目标：引入 `ToolRenderer` 接口 + 跨调用压缩器，让每个工具自描述渲染逻辑，统一管道分发。

---

## 一、当前渲染管道分析

### 1.1 数据结构

**文件：`tui/model.go`**

```go
type chatEntry struct {
    Kind   string // "user" | "assistant" | "tool" | "system"
    Title  string // 工具: "READ | read_file"
    Meta   string
    Body   string // 工具: 摘要 + 详情行
    Status string // "running" | "done" | "error" | "warn"
}

type toolRun struct {
    Name    string
    Summary string
    Lines   []string
    Status  string
}
```

### 1.2 事件处理链

**文件：`tui/component_run_flow.go`**

```
EventToolCallStarted
  → finalizeAssistantTurnForTool()
  → appendChat(chatEntry{Kind: "tool", Title: toolEntryTitle(name), Body: "", Status: "running"})
  → toolRuns = append(toolRuns, toolRun{...})

EventToolCallCompleted
  → summarizeTool(name, payload) → (summary, lines, status)  // 集中式 switch
  → finishLatestToolCall(name, joinSummary(summary, lines), status)
  → 更新 toolRuns
```

### 1.3 渲染链

**文件：`tui/component_conversation.go`**

```
renderChatRow(item)
  → renderChatCard(item, width)
    → renderChatSection(item, width)
      → toolDisplayParts(title) → (label, name)  // "READ", "read_file"
      → renderToolTag(label, "info")              // 头部标签
      → formatChatBody(item, width)               // Body 渲染
```

### 1.4 核心问题

**`summarizeTool()`（model.go:2081-2253）**：

```go
func summarizeTool(name, payload string) (summary string, lines []string, status string) {
    switch name {
    case "list_files":   // 解析 JSON，生成摘要
    case "read_file":    // 解析 JSON，生成摘要
    case "search_text":  // 解析 JSON，生成摘要
    case "web_search":   // ...
    case "web_fetch":    // ...
    case "write_file":   // ...
    case "replace_in_file": // ...
    case "apply_patch":  // ...
    case "update_plan":  // ...
    case "run_shell":    // ...
    }
    return compact(payload, 96), nil, "done"  // fallback
}
```

问题：
- 200 行 switch-case，每加一个工具改一次
- 工具的渲染逻辑分散在 TUI 包里，工具定义和渲染分离
- 无扩展点：delegate_subagent 的压缩面板、subagent 进度都无法自定义

**`toolDisplayLabel()`（model.go:2024-2046）**：

```go
func toolDisplayLabel(name string) string {
    switch name {
    case "list_files":    return "LIST"
    case "read_file":     return "READ"
    case "search_text":   return "SEARCH"
    // ...
    default:              return "TOOL"
    }
}
```

同样集中式，同样需要每个工具改一次。

**`finishLatestToolCall()`（component_chat_stream.go:204-224）**：

```go
func (m *model) finishLatestToolCall(name, body, status string) {
    title := toolEntryTitle(name)
    for i := len(m.chatItems) - 1; i >= 0; i-- {
        if m.chatItems[i].Kind != "tool" { continue }
        if m.chatItems[i].Title != title { continue }  // 按 Title 匹配
        m.chatItems[i].Body = body
        m.chatItems[i].Status = status
        return
    }
}
```

按 Title 反向查找，同类工具并行时可能匹配错误。

---

## 二、ToolRenderer 接口设计

### 2.1 接口定义

**新建文件：`tui/tool_renderer.go`**

```go
package tui

// ToolRenderer 为每个工具提供自描述渲染逻辑。
// 工具实现此接口后，由统一渲染管道分发调用。
type ToolRenderer interface {
    // DisplayLabel 返回工具的显示标签（如 "READ"、"SEARCH"、"AGENT"）
    DisplayLabel() string

    // ProgressText 根据累积的中间事件生成执行中进度文本。
    // 返回空字符串表示不更新进度（保持 running 状态）。
    ProgressText(events []ToolEvent) string

    // ResultSummary 根据工具的最终结果 JSON 生成渲染内容。
    // summary: 头部一行摘要（如 "Read model.go"）
    // lines: 详情行（如 ["range: 1-50", "path: internal/agent/model.go"]）
    // status: "done" | "error" | "warn"
    ResultSummary(payload string) (summary string, lines []string, status string)

    // CompactLine 返回单行紧凑文本，用于树形渲染中的叶子节点。
    // 如 "model.go (1-50)"、"3 matches for auth"。
    // 默认实现取 summary + lines 第一行。
    CompactLine(payload string) string
}

// defaultCompactLine 提供 CompactLine 的默认实现。
// 各渲染器可嵌入此结构体获得默认行为，也可以覆盖。
type defaultCompactLine struct{}

func (d defaultCompactLine) CompactLine(payload string) string {
    // 默认取 ResultSummary 的 summary
    return ""
}

// 渲染器可组合 defaultCompactLine 获得默认 CompactLine，
// 然后只覆盖需要自定义的方法。
// 例如：
//   type readFileRenderer struct { defaultCompactLine }
//   func (r *readFileRenderer) CompactLine(payload string) string { return "model.go (1-50)" }

// ToolEvent 表示工具执行过程中的中间事件。
type ToolEvent struct {
    Type    string // "delta" | "sub_tool_start" | "sub_tool_complete"
    Content string
    Tool    string
}
```

### 2.2 注册机制

**文件：`tui/tool_renderer.go`**

```go
// toolRendererRegistry 持有所有已注册的工具渲染器。
type toolRendererRegistry struct {
    renderers map[string]ToolRenderer
}

var defaultRendererRegistry = &toolRendererRegistry{
    renderers: make(map[string]ToolRenderer),
}

func RegisterToolRenderer(toolName string, renderer ToolRenderer) {
    defaultRendererRegistry.renderers[toolName] = renderer
}

func GetToolRenderer(toolName string) ToolRenderer {
    if r, ok := defaultRendererRegistry.renderers[toolName]; ok {
        return r
    }
    return nil
}
```

### 2.3 初始化

**文件：`tui/tool_renderers_init.go`**

```go
package tui

func init() {
    RegisterToolRenderer("read_file", &readFileRenderer{})
    RegisterToolRenderer("list_files", &listFilesRenderer{})
    RegisterToolRenderer("search_text", &searchTextRenderer{})
    RegisterToolRenderer("run_shell", &runShellRenderer{})
    RegisterToolRenderer("write_file", &writeFileRenderer{})
    RegisterToolRenderer("replace_in_file", &replaceInFileRenderer{})
    RegisterToolRenderer("apply_patch", &applyPatchRenderer{})
    RegisterToolRenderer("update_plan", &updatePlanRenderer{})
    RegisterToolRenderer("web_search", &webSearchRenderer{})
    RegisterToolRenderer("web_fetch", &webFetchRenderer{})
    // delegate_subagent 在 subagent 模块初始化时注册
}
```

---

## 三、各工具渲染器实现

### 3.1 从 summarizeTool() 迁移

每个 case 从 `summarizeTool()` 的 switch 里提取出来，变成独立的 ToolRenderer 实现。

**新建文件：`tui/tool_renderers_builtin.go`**

```go
package tui

import (
    "encoding/json"
    "fmt"
    "path/filepath"
    "strings"
)

// readFileRenderer
type readFileRenderer struct{}

func (r *readFileRenderer) DisplayLabel() string { return "READ" }

func (r *readFileRenderer) ProgressText(events []ToolEvent) string {
    return "" // read_file 无中间进度
}

func (r *readFileRenderer) ResultSummary(payload string) (string, []string, string) {
    var result struct {
        Path      string `json:"path"`
        StartLine int    `json:"start_line"`
        EndLine   int    `json:"end_line"`
    }
    if json.Unmarshal([]byte(payload), &result) != nil {
        return compact(payload, 96), nil, "done"
    }
    summary := "Read " + filepath.Base(result.Path)
    lines := []string{
        fmt.Sprintf("range: %d-%d", result.StartLine, result.EndLine),
        "path: " + compactDisplayPath(result.Path),
    }
    return summary, lines, "done"
}

// CompactLine 返回 "model.go (1-50)" 用于树形渲染
func (r *readFileRenderer) CompactLine(payload string) string {
    var result struct {
        Path      string `json:"path"`
        StartLine int    `json:"start_line"`
        EndLine   int    `json:"end_line"`
    }
    if json.Unmarshal([]byte(payload), &result) != nil {
        return compact(payload, 60)
    }
    return fmt.Sprintf("%s (%d-%d)", filepath.Base(result.Path), result.StartLine, result.EndLine)
}

// listFilesRenderer
type listFilesRenderer struct{}

func (r *listFilesRenderer) DisplayLabel() string { return "LIST" }

func (r *listFilesRenderer) ProgressText(events []ToolEvent) string { return "" }

func (r *listFilesRenderer) ResultSummary(payload string) (string, []string, string) {
    var result struct {
        Items []struct {
            Type string `json:"type"`
        } `json:"items"`
    }
    if json.Unmarshal([]byte(payload), &result) != nil {
        return compact(payload, 96), nil, "done"
    }
    dirs, files := 0, 0
    for _, item := range result.Items {
        if item.Type == "dir" {
            dirs++
        } else {
            files++
        }
    }
    return fmt.Sprintf("Read %d files, listed %d directories", files, dirs), []string{}, "done"
}

func (r *listFilesRenderer) CompactLine(payload string) string {
    var result struct {
        Items []struct {
            Type string `json:"type"`
        } `json:"items"`
    }
    if json.Unmarshal([]byte(payload), &result) != nil {
        return compact(payload, 60)
    }
    dirs, files := 0, 0
    for _, item := range result.Items {
        if item.Type == "dir" {
            dirs++
        } else {
            files++
        }
    }
    return fmt.Sprintf("%d files, %d dirs", files, dirs)
}

// runShellRenderer
type runShellRenderer struct{}

func (r *runShellRenderer) DisplayLabel() string { return "SHELL" }

func (r *runShellRenderer) ProgressText(events []ToolEvent) string {
    // 长时间运行的 shell 命令可以显示最后几行 stdout
    for i := len(events) - 1; i >= 0; i-- {
        if events[i].Type == "delta" && strings.TrimSpace(events[i].Content) != "" {
            return compact(events[i].Content, 80)
        }
    }
    return ""
}

func (r *runShellRenderer) ResultSummary(payload string) (string, []string, string) {
    var result struct {
        OK       bool   `json:"ok"`
        ExitCode int    `json:"exit_code"`
        Stdout   string `json:"stdout"`
        Stderr   string `json:"stderr"`
    }
    if json.Unmarshal([]byte(payload), &result) != nil {
        return compact(payload, 96), nil, "done"
    }
    lines := make([]string, 0, 2)
    if text := strings.TrimSpace(result.Stdout); text != "" {
        lines = append(lines, "stdout: "+compact(strings.Split(text, "\n")[0], 64))
    }
    if text := strings.TrimSpace(result.Stderr); text != "" {
        lines = append(lines, "stderr: "+compact(strings.Split(text, "\n")[0], 64))
    }
    status := "done"
    if !result.OK {
        status = "warn"
    }
    return fmt.Sprintf("Shell exited with code %d", result.ExitCode), lines, status
}

func (r *runShellRenderer) CompactLine(payload string) string {
    var result struct {
        ExitCode int    `json:"exit_code"`
        Stdout   string `json:"stdout"`
    }
    if json.Unmarshal([]byte(payload), &result) != nil {
        return compact(payload, 60)
    }
    firstLine := ""
    if text := strings.TrimSpace(result.Stdout); text != "" {
        firstLine = compact(strings.Split(text, "\n")[0], 48)
    }
    if firstLine != "" {
        return firstLine
    }
    return fmt.Sprintf("exited %d", result.ExitCode)
}
}

// 其他渲染器同理迁移...
```

### 3.2 delegate_subagent 渲染器

**新建文件：`tui/tool_renderer_delegate_subagent.go`**

渲染风格参考 Claude Code 的 Agent 工具：树形结构，`⎿` 连接子操作。

```go
package tui

import (
    "encoding/json"
    "fmt"
    "strings"
)

// delegateSubAgentRenderer 是无状态的。
// 每次调用 ProgressText 时从 events 临时构建结果，
// 不在 renderer 上持有可变状态。
type delegateSubAgentRenderer struct{}

type subToolCallRecord struct {
    Name    string
    Detail  string // 关键信息（文件名、匹配数等）
    Status  string
}

func (r *delegateSubAgentRenderer) DisplayLabel() string { return "AGENT" }

// ProgressText 输出树形格式的进度文本。
// 每次从 events 临时构建，不做状态累积。
func (r *delegateSubAgentRenderer) ProgressText(events []ToolEvent) string {
    var toolCalls []subToolCallRecord
    for _, ev := range events {
        switch ev.Type {
        case "sub_tool_start":
            toolCalls = append(toolCalls, subToolCallRecord{
                Name:   ev.Tool,
                Status: "running",
            })
        case "sub_tool_complete":
            if len(toolCalls) > 0 {
                toolCalls[len(toolCalls)-1].Detail = ev.Content
                toolCalls[len(toolCalls)-1].Status = "done"
            }
        }
    }
    return renderSubToolCallTree(toolCalls)
}

// 渲染子 agent 的工具调用序列（纯文本，不带 ⎿ 前缀，由渲染层统一添加）
func renderSubToolCallTree(toolCalls []subToolCallRecord) string {
    if len(toolCalls) == 0 {
        return ""
    }

    var lines []string
    for _, tc := range toolCalls {
        status := ""
        if tc.Status == "running" {
            status = " · Running..."
        }
        detail := tc.Detail
        if detail == "" {
            detail = tc.Name
        }
        lines = append(lines, detail+status)
    }
    return strings.Join(lines, "\n")
}

// ResultSummary 渲染子 agent 的最终结果。
// 返回的 lines 是树形格式的 findings。
func (r *delegateSubAgentRenderer) ResultSummary(payload string) (string, []string, string) {
    var result struct {
        OK       bool   `json:"ok"`
        Status   string `json:"status"`
        Summary  string `json:"summary"`
        Agent    string `json:"agent"`
        Findings []struct {
            Title string `json:"title"`
            Body  string `json:"body"`
        } `json:"findings"`
        Error *struct {
            Code    string `json:"code"`
            Message string `json:"message"`
        } `json:"error"`
    }
    if json.Unmarshal([]byte(payload), &result) != nil {
        return compact(payload, 96), nil, "done"
    }

    if !result.OK {
        msg := "SubAgent failed"
        if result.Error != nil {
            msg = result.Error.Message
        }
        return msg, nil, "error"
    }

    summary := result.Summary
    if summary == "" {
        summary = fmt.Sprintf("%s completed", result.Agent)
    }

    // findings 渲染为纯文本行（渲染层统一添加 ⎿ 前缀）
    var lines []string
    for _, f := range result.Findings {
        line := f.Title
        if line == "" {
            line = compact(f.Body, 80)
        }
        lines = append(lines, line)
    }

    status := "done"
    if result.Status == "failed" {
        status = "error"
    }
    return summary, lines, status
}

// CompactLine 返回 "explorer · Found auth middleware · 5 tool uses · 12s"
func (r *delegateSubAgentRenderer) CompactLine(payload string) string {
    var result struct {
        OK      bool   `json:"ok"`
        Summary string `json:"summary"`
        Agent   string `json:"agent"`
    }
    if json.Unmarshal([]byte(payload), &result) != nil {
        return compact(payload, 60)
    }
    if !result.OK {
        return fmt.Sprintf("%s · failed", result.Agent)
    }
    summary := result.Summary
    if summary == "" {
        summary = "completed"
    }
    return fmt.Sprintf("%s · %s", result.Agent, summary)
}
```

---

## 四、跨调用压缩（渲染层折叠）

### 4.1 设计原则

**不在 EventRunFinished 后改写 chatItems**。原始明细保留在 chatItems 中，压缩只在渲染时发生。这样：

- 原始数据不丢失，复制/历史恢复不受影响
- 用户可以展开查看每条工具调用的完整内容
- 切换压缩/展开状态不需要重新计算

### 4.2 渲染层折叠模型

**文件：`tui/model.go`**

```go
// chatEntry 新增字段支持树形渲染和精确匹配
type chatEntry struct {
    Kind        string
    Title       string
    Meta        string
    Body        string   // 兼容：summary + "\n" + lines（旧逻辑）
    Status      string
    GroupID     string   // 新增：连续同类工具共享 GroupID，空表示不折叠
    ToolCallID  string   // 新增：精确匹配用
    CompactBody string   // 新增：折叠态显示文本，如 "model.go (1-50)"
    DetailLines []string // 新增：展开态详情行，如 ["range: 1-50", "path: ..."]
}
```

### 4.3 折叠组分配

**文件：`tui/component_run_flow.go`**

`EventToolCallCompleted` 时，检查是否与前一条 tool entry 同类，如果是则分配相同 GroupID：

```go
func (m *model) assignToolGroupID(name string) string {
    // 从后往前找最近的 tool entry
    for i := len(m.chatItems) - 1; i >= 0; i-- {
        if m.chatItems[i].Kind != "tool" {
            continue
        }
        _, prevName := toolDisplayParts(m.chatItems[i].Title)
        if prevName == name && m.chatItems[i].Status == "done" && m.chatItems[i].GroupID != "" {
            return m.chatItems[i].GroupID // 复用前一条的 GroupID
        }
        break
    }
    // 新组
    return fmt.Sprintf("tg-%d", m.nextGroupSeq)
}
```

### 4.4 渲染层折叠

**文件：`tui/component_conversation.go`**

渲染时对连续同 GroupID 的 tool entries 做折叠/展开：

```go
const toolIndent = "  "
const toolTreeChar = "⎿ "
const toolIcon = "⏺ "

func renderChatRows(m model, items []chatEntry, width int) string {
    var result strings.Builder
    i := 0
    for i < len(items) {
        item := items[i]

        // 非 tool 或无 GroupID：正常渲染
        if item.Kind != "tool" || item.GroupID == "" {
            result.WriteString(renderChatRow(item, width))
            i++
            continue
        }

        // 收集连续同 GroupID 的 entries
        group := []chatEntry{item}
        j := i + 1
        for j < len(items) && items[j].GroupID == item.GroupID {
            group = append(group, items[j])
            j++
        }

        if len(group) == 1 {
            // 单条工具
            result.WriteString(renderToolEntry(group[0], width, m.toolDetailExpanded))
        } else if m.toolDetailExpanded {
            // 展开态：摘要行 + 每条 CompactBody
            summary := compressGroupSummary(group)
            result.WriteString(toolIndent + toolIcon + summary + "\n")
            for _, entry := range group {
                detail := entry.CompactBody
                if detail == "" {
                    detail = strings.TrimSpace(strings.Split(entry.Body, "\n")[0])
                }
                result.WriteString(toolIndent + toolIndent + toolTreeChar + detail + "\n")
            }
        } else {
            // 折叠态（默认）：只渲染摘要行
            summary := compressGroupSummary(group)
            result.WriteString(toolIndent + toolIcon + summary + "\n")
        }
        i = j
    }
    return result.String()
}

// 单条工具渲染
func renderToolEntry(item chatEntry, width int, expanded bool) string {
    // 折叠态：用 CompactBody（如 "model.go (1-50)"）
    if !expanded {
        text := item.CompactBody
        if text == "" {
            text = strings.TrimSpace(strings.Split(item.Body, "\n")[0])
        }
        return toolIndent + toolIcon + text + "\n"
    }

    // 展开态：CompactBody 作为标题 + DetailLines 作为详情
    text := item.CompactBody
    if text == "" {
        text = strings.TrimSpace(strings.Split(item.Body, "\n")[0])
    }
    result := toolIndent + toolIcon + text + "\n"
    for _, line := range item.DetailLines {
        result += toolIndent + toolIndent + toolTreeChar + line + "\n"
    }
    return result
}

// chatEntry 的 CompactBody 和 DetailLines 分别存储折叠态和展开态的内容。
// 在 EventToolCallCompleted 时一次性生成，渲染时直接读取。
//
// 折叠态：读取 CompactBody → "model.go (1-50)"
// 展开态：读取 DetailLines → ["range: 1-50", "path: internal/agent/model.go"]
//
// 填充逻辑见 5.1.3 handleAgentEvent 改造。

// 跨工具组压缩摘要（信息只出现一次）
func compressGroupSummary(group []chatEntry) string {
    if len(group) == 0 {
        return ""
    }
    // 取第一条的 Body 作为基础，加上数量
    firstBody := strings.TrimSpace(group[0].Body)
    count := len(group)
    switch {
    case strings.HasPrefix(firstBody, "Read "):
        return fmt.Sprintf("Read %d files", count)
    case strings.Contains(firstBody, "matches for"):
        return fmt.Sprintf("Searched %d times", count)
    case strings.HasPrefix(firstBody, "Shell exited"):
        return fmt.Sprintf("Ran %d commands", count)
    default:
        return fmt.Sprintf("%d operations", count)
    }
}
```

### 4.5 展开/折叠交互

**文件：`tui/model.go`**

```go
// model 新增字段
toolDetailExpanded bool // Ctrl+O 全局 toggle
```

**文件：`tui/model.go` handleKey**

```go
case "ctrl+o":
    m.toolDetailExpanded = !m.toolDetailExpanded
```

Ctrl+O 切换全局展开/折叠状态，所有工具组统一响应。不需要 row-level selection。

### 4.6 折叠策略

| 条件 | 行为 |
|------|------|
| 连续同类工具 + 全部 done + >= 2 条 | 分配同 GroupID，渲染层折叠 |
| 中间有 error/warn | 中断折叠组，error 独立显示 |
| 不同工具交替 | 各自独立 GroupID |
| delegate_subagent | 不参与折叠（它内部有自己的压缩逻辑） |

---

## 五、统一渲染管道改造

### 5.1 handleAgentEvent 改造

#### 5.1.1 Event 新增 ToolCallID

**文件：`internal/agent/events.go`**

```go
type Event struct {
    Type          EventType
    SessionID     corepkg.SessionID
    ToolCallID    string      // 新增：唯一标识一次工具调用
    UserInput     string
    Content       string
    ToolName      string
    ToolArguments string
    ToolResult    string
    Error         string
    Plan          planpkg.State
    Usage         llm.Usage
}
```

agent 层 emit 时填充 `ToolCallID`（单调递增计数器或 UUID）：

```go
runner.emit(Event{
    Type:       EventToolCallStarted,
    ToolCallID: newToolCallID(),  // 每次工具调用唯一
    ToolName:   name,
})
```

**文件：`tui/ports.go`**

同步新增 `ToolCallID` 字段。

#### 5.1.2 chatEntry 新增字段

**文件：`tui/model.go`**

```go
type chatEntry struct {
    Kind        string
    Title       string
    Meta        string
    Body        string    // 兼容：summary + "\n" + lines
    Status      string
    GroupID     string    // 折叠组（见第四章）
    ToolCallID  string    // 精确匹配用
    CompactBody string    // 折叠态显示文本，如 "model.go (1-50)"
    DetailLines []string  // 展开态详情行
}
```

#### 5.1.3 handleAgentEvent 改造

**文件：`tui/component_run_flow.go`**

```go
case EventToolCallStarted:
    renderer := GetToolRenderer(event.ToolName)
    label := "TOOL"
    if renderer != nil {
        label = renderer.DisplayLabel()
    }
    m.appendChat(chatEntry{
        Kind:       "tool",
        Title:      label + " | " + event.ToolName,
        Body:       "",
        Status:     "running",
        ToolCallID: event.ToolCallID,
    })

case EventToolCallCompleted:
    renderer := GetToolRenderer(event.ToolName)
    var summary string
    var lines []string
    var status string
    var compactBody string

    if renderer != nil {
        summary, lines, status = renderer.ResultSummary(event.ToolResult)
        compactBody = renderer.CompactLine(event.ToolResult)
    } else {
        summary = compact(event.ToolResult, 96)
        status = "done"
        compactBody = summary
    }
    if compactBody == "" {
        compactBody = summary
    }

    m.finishToolCall(event.ToolCallID, toolCallResult{
        Body:        joinSummary(summary, lines),
        CompactBody: compactBody,
        DetailLines: lines,
        Status:      status,
    })
```

#### 5.1.4 finishToolCall 改为按 ToolCallID 匹配

**文件：`tui/component_chat_stream.go`**

```go
type toolCallResult struct {
    Body        string
    CompactBody string
    DetailLines []string
    Status      string
}

func (m *model) finishToolCall(toolCallID string, result toolCallResult) {
    for i := len(m.chatItems) - 1; i >= 0; i-- {
        if m.chatItems[i].Kind != "tool" {
            continue
        }
        if m.chatItems[i].ToolCallID == toolCallID {
            m.chatItems[i].Body = result.Body
            m.chatItems[i].CompactBody = result.CompactBody
            m.chatItems[i].DetailLines = result.DetailLines
            m.chatItems[i].Status = result.Status
            return
        }
    }
}
```

旧的 `finishLatestToolCall()` 保留为 fallback（处理没有 ToolCallID 的遗留数据）。

### 5.2 中间进度更新

对于有 `ProgressText` 的工具（如 delegate_subagent、run_shell），需要在事件循环中定期更新。

#### 5.2.1 EventToolProgress 定义

**文件：`internal/agent/events.go`**

```go
const (
    // ... existing ...
    EventToolProgress    EventType = "tool_progress"    // 新增
)
```

**文件：`tui/ports.go`**

同步新增：

```go
const (
    // ... existing ...
    EventToolProgress    EventType = "tool_progress"    // 新增
)
```

两处都需要加，因为 agent 层的 `EventType` 和 TUI 层的 `EventType` 是各自定义的常量（TUI 通过 ports.go 复制 agent 层的事件类型，保持解耦）。

agent 层在工具执行过程中定期 emit：

```go
// internal/agent/completion_runtime.go 或工具执行层
runner.emit(Event{
    Type:     EventToolProgress,
    ToolName: "delegate_subagent",
    Content:  string(progressJSON), // ToolEvent 序列
})
```

TUI 处理：

```go
case EventToolProgress:
    renderer := GetToolRenderer(event.ToolName)
    if renderer != nil {
        var events []ToolEvent
        json.Unmarshal([]byte(event.Content), &events)
        progressText := renderer.ProgressText(events)
        if progressText != "" {
            m.updateLatestToolBody(event.ToolName, progressText)
        }
    }
```

---

## 六、渲染效果

渲染风格参考 Claude Code：树形结构，`⏺` 工具图标，`⎿` 子操作连接符，信息不重复。

### 6.1 单条工具

```
  ⏺ Read model.go
```

Ctrl+O 展开后：

```
  ⏺ Read model.go
    ⎿ range: 1-50
    ⎿ path: internal/agent/model.go
```

### 6.2 连续同类工具（默认折叠）

```
  ⏺ Read 5 files
  ⏺ Searched 2 times
  ⏺ Ran 3 commands
```

Ctrl+O 展开后：

```
  ⏺ Read 5 files
    ⎿ model.go (1-50)
    ⎿ view.go (1-30)
    ⎿ controller.go (1-80)
    ⎿ service.go (1-45)
    ⎿ dao.go (1-60)
  ⏺ Searched 2 times
    ⎿ 3 matches for "auth middleware"
    ⎿ 5 matches for "auth pattern"
  ⏺ Ran 3 commands
    ⎿ npm test — exited 0
    ⎿ npm run lint — exited 0
    ⎿ npm run build — exited 0
```

### 6.3 混合工具（各自独立）

```
  ⏺ Read model.go
  ⏺ 3 matches for "auth middleware"
  ⏺ Shell exited with code 0
  ⏺ Changed 3 lines in model.go
```

### 6.4 delegate_subagent 执行中

```
  ⏺ explorer · Running...
    ⎿ search_text "auth middleware" — 3 matches
    ⎿ read_file · src/auth/jwt.go — 142 lines
    ⎿ read_file · src/middleware/auth.go — 89 lines
    ⎿ search_text "auth patterns" · Running...
```

### 6.5 delegate_subagent 完成

```
  ⏺ explorer · Found auth middleware in 2 locations · 5 tool uses · 12s
    ⎿ src/auth/jwt.go — JWT validation logic
    ⎿ src/middleware/auth.go — request auth check
```

### 6.6 delegate_subagent 失败

```
  ⏺ explorer · SubAgent failed · 3 tool uses · 8s
    ⎿ Code: execution_failed
    ⎿ Message: LLM client returned timeout
```

### 6.7 完整对话流示例

```
You: @explorer find all auth middleware and refactor it

  ⏺ explorer · Found auth middleware in 3 locations · 5 tool uses · 12s
    ⎿ src/auth/jwt.go — JWT validation logic
    ⎿ src/middleware/auth.go — request auth check
    ⎿ src/routes/protected.go — route-level auth guard

Assistant: I found the auth middleware in 3 locations. Let me refactor...

  ⏺ Changed 12 lines in middleware/auth.go
  ⏺ Changed 5 lines in routes/protected.go
  ⏺ Shell exited with code 0

Assistant: Refactoring complete. All 24 tests passed.
```

Ctrl+O 展开后：

```
You: @explorer find all auth middleware and refactor it

  ⏺ explorer · Found auth middleware in 3 locations · 5 tool uses · 12s
    ⎿ src/auth/jwt.go — JWT validation logic
    ⎿ src/middleware/auth.go — request auth check
    ⎿ src/routes/protected.go — route-level auth guard

Assistant: I found the auth middleware in 3 locations. Let me refactor...

  ⏺ Changed 2 files
    ⎿ Changed 12 lines in middleware/auth.go
    ⎿ Changed 5 lines in routes/protected.go
  ⏺ Shell exited with code 0
    ⎿ stdout: All 24 tests passed in 3.2s

Assistant: Refactoring complete. All 24 tests passed.
```

---

## 七、改动总览

### 新建文件

| 文件 | 用途 |
|------|------|
| `tui/tool_renderer.go` | `ToolRenderer` 接口 + 注册机制 |
| `tui/tool_renderers_init.go` | 内置工具渲染器注册 |
| `tui/tool_renderers_builtin.go` | `read_file`、`list_files`、`search_text`、`run_shell`、`write_file`、`replace_in_file`、`apply_patch`、`update_plan`、`web_search`、`web_fetch` 的渲染器实现 |
| `tui/tool_renderer_delegate_subagent.go` | `delegate_subagent` 渲染器（无状态，每次 ProgressText 从 events 临时构建） |

### 修改文件

| 文件 | 改动 |
|------|------|
| `internal/agent/events.go` | Event 新增 `ToolCallID string`；新增 `EventToolProgress` 事件类型 |
| `tui/ports.go` | 同步新增 `ToolCallID`、`EventToolProgress` |
| `tui/model.go` | `chatEntry` 新增 `GroupID`、`ToolCallID`、`CompactBody`、`DetailLines`；新增 `toolDetailExpanded bool` + Ctrl+O 全局 toggle；`toolDisplayLabel()` 保留为 fallback |
| `tui/component_run_flow.go` | `handleAgentEvent()` 改为通过 `GetToolRenderer()` 分发 + `ToolCallID` 精确匹配 + `CompactBody`/`DetailLines` 填充；新增 `EventToolProgress` 处理；`assignToolGroupID()` 分配折叠组 |
| `tui/component_chat_stream.go` | 新增 `finishToolCall(toolCallID, toolCallResult)` 按 ID 精确匹配 + `CompactBody`/`DetailLines` 写入；旧 `finishLatestToolCall()` 保留为 fallback |
| `tui/component_conversation.go` | `renderChatRows()` 支持 GroupID 折叠/展开渲染（树形风格：`⏺` 图标 + `⎿` 连接符；折叠态读 `CompactBody`，展开态读 `CompactBody` + `DetailLines`） |

### 删除的代码

| 文件 | 删除内容 |
|------|---------|
| `tui/model.go` | `summarizeTool()` 函数（200 行 switch-case，逐步迁移到各 ToolRenderer 后删除） |

### 优先级

| 优先级 | 编号 | 改动 |
|--------|------|------|
| P0 | 2.1-2.2 | `ToolRenderer` 接口 + 注册机制 |
| P0 | 3.1 | 内置工具渲染器迁移（从 `summarizeTool()` 提取） |
| P0 | 5.1.1-5.1.4 | Event/chatEntry 新增 ToolCallID + CompactBody/DetailLines + `finishToolCall` 精确匹配 |
| P1 | 4.1-4.6 | 渲染层折叠（GroupID + renderChatRows 折叠 + 展开交互） |
| P1 | 3.2 | `delegate_subagent` 渲染器 |
| P1 | 5.2 | `EventToolProgress` 中间进度更新 |
| P2 | 6.4 | `run_shell` 中间进度 |

### 与 subagent-redesign.md 的关系

本整改是 subagent UI 渲染的前置依赖。`delegate_subagent` 作为 `ToolRenderer` 实现接入统一渲染管道后，subagent 的执行过程（压缩面板）和最终结果（结构化 findings）才能正确展示。

subagent 的 `onEvent` 回调通过 `EventToolProgress` 事件将子 agent 的工具调用序列传递给 `delegateSubAgentRenderer.ProgressText()`，实现压缩渲染。
