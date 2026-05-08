# ByteMind 可写 SubAgent 设计方案

## 1. 现状分析

### 1.1 当前能力

ByteMind 当前有 2 个内置 SubAgent（`explorer`、`review`），均为只读型。架构层面已具备可写 SubAgent 的基础设施：

| 已具备 | 状态 | 位置 |
|--------|------|------|
| Agent 定义类型（含 Tools/DisallowedTools/Isolation 字段） | 已完成 | `internal/subagents/types.go:13-31` |
| 多层工具过滤（交集+差集） | 已完成 | `internal/subagents/gateway.go:143-160` |
| ToolSpec 分级（ReadOnly/Destructive/SafetyClass） | 已完成 | `internal/tools/spec.go:20-33` |
| 写工具强行 worktree 隔离的规则 | 已设计未实现 | `docs/subagent-architecture.md:845` |
| 会话/Observer/Approval 隔离 | 已完成 | `internal/agent/subagent_isolation.go` |
| 同步/异步执行 | 已完成 | `internal/agent/subagent_delegate.go` |

### 1.2 缺失的关键组件

| 缺失组件 | 影响 |
|----------|------|
| WorktreeManager（创建/清理/补偿） | 写操作无法隔离，直接污染父工作区 |
| 权限快照（SubAgentApprovalSnapshot） | 无法在启动前预审批写操作 |
| 系统提示词分级（只读警告 vs 写操作指南） | 只读/可写 agent 行为约束无软防护 |
| 上下文优化（omitContext / omitGitStatus） | 不必要的 token 消耗 |
| 可写 SubAgent 示例定义 | 用户无参考模板 |
| 审批策略差异化（nonInteractiveApproval 过于宽松） | 写操作无用户确认环节 |
| 结果通知增强（worktree 信息注入） | 父 agent 无法得知 worktree 位置和变更状态 |

### 1.3 当前硬限制

`internal/agent/subagent_delegate.go:201` 处有强制限制：

```go
// 后台 SubAgent 仅允许只读工具集
if !r.isReadOnlySubAgentToolset(preflight.EffectiveTools) {
    result.Error = &tools.DelegateSubAgentError{
        Code:      subAgentErrorCodeBackgroundWriteDenied,
        Message:   "run_in_background currently supports read-only subagents only",
    }
}
```

这意味着即使定义了可写 SubAgent，后台模式下也会被拒绝。

---

## 2. Claude Code 可参考的设计精华

### 2.1 声明式差异而非两套代码路径

Claude Code 的只读型和可写型 SubAgent 共用同一套 `runAgent()` 引擎，差异完全由 `AgentDefinition` 的字段声明决定：

```
可写型: tools: ['*'], 无 disallowedTools
只读型: tools: undefined, disallowedTools: [Edit, Write, NotebookEdit]
```

ByteMind 已经具备这种声明式架构基础（`Agent.Tools` / `Agent.DisallowedTools`），只需补全其余层次。

### 2.2 五层纵深防御

```
Layer 1: 工具过滤      → disallowedTools 阻止写工具（硬限制）
Layer 2: 系统提示词    → READ-ONLY 警告（软防护，纵深防御）
Layer 3: 权限模式      → permissionMode 控制是否需要用户确认
Layer 4: 隔离机制      → worktree 隔离文件系统（可选但推荐）
Layer 5: 安全分类/审计 → 完成后安全检查 + cleanup 防僵尸进程
```

### 2.3 Worktree 自动清理策略

Claude Code 的关键设计决策：**子 agent 完成后检测是否有改动。无改动自动删除 worktree，有改动保留并通知父 agent。**

```typescript
// AgentTool.tsx — 伪代码表达
if (headCommit) {
  const changed = await hasWorktreeChanges(worktreePath, headCommit);
  if (!changed) {
    await removeAgentWorktree(...);  // 无改动 → 自动删除
    return {};
  }
}
// 有改动 → 保留 worktree，通知父 agent
return { worktreePath, worktreeBranch };
```

### 2.4 上下文优化

只读 agent 跳过 CLAUDE.md 和 gitStatus，节省 token：

| 优化项 | 只读型 (Explore/Plan) | 可写型 (general-purpose) |
|--------|----------------------|--------------------------|
| CLAUDE.md | 省略 (omitClaudeMd: true) | 包含 |
| gitStatus | 省略 | 包含 |
| 系统提示词 | 显式 READ-ONLY 警告 | 通用任务指令 + 编辑指南 |

### 2.5 权限模式差异化

- **同步执行**：继承父级权限模式
- **异步执行**：默认 `shouldAvoidPermissionPrompts: true`（自动拒绝权限请求）
- **Fork agent**：`permissionMode: 'bubble'`（权限请求冒泡到用户终端）

---

## 3. ByteMind 可写 SubAgent 设计方案

### 3.1 总体原则

1. **不新增代码路径**：可写和只读共用 `Runner` / `Engine` / `Gateway.Preflight`，差异仅在于 Agent 定义的配置字段
2. **纵深防御**：工具过滤 → 系统提示词 → 权限快照 → worktree 隔离 → 审计日志
3. **最小实现**：MVP 只打通前台同步可写 SubAgent，后台可写留待后续
4. **安全优先**：写工具默认强制 worktree 隔离，无法隔离则拒绝启动
5. **对齐 Claude Code**：worktree 回传用通知机制而非自动 merge

### 3.2 实现计划

#### 阶段 1：WorktreeManager 实现（基础设施）

这是可写 SubAgent 的前置依赖，当前完全未实现。

**新增文件**：`internal/runtime/worktree.go`

```go
// WorktreeManager 管理临时 git worktree 的创建、清理和补偿
type WorktreeManager struct {
    workspaceRoot string
    worktreesRoot string  // <workspace>/.bytemind/worktrees
    ownerDir      string  // ${BYTEMIND_HOME}/runtime/subagents/worktrees
}

// Prepare 创建临时 worktree
func (m *WorktreeManager) Prepare(ctx context.Context, req WorktreeRequest) (*WorktreeHandle, error)

// Cleanup 清理 worktree（幂等）
func (m *WorktreeManager) Cleanup(ctx context.Context, handle WorktreeHandle) error

// Reconcile 启动期补偿清理陈旧/异常 worktree
func (m *WorktreeManager) Reconcile(ctx context.Context) []ReconcileDiagnostic
```

**关键行为**：

- worktree_root 固定为 `<workspace>/.bytemind/worktrees`
- worktree_path 格式：`<workspace>/.bytemind/worktrees/subagent-<invocation_id>`
- 创建时落 owner 元数据到 `${BYTEMIND_HOME}/runtime/subagents/worktrees/<id>.json`
- Cleanup 成功时同步删除 worktree 和 owner 元数据
- Cleanup 失败时标记 `state=cleanup_failed` 等待补偿
- Runner 启动时执行一次 Reconcile 扫描补偿
- 所有操作必须幂等

**owner 元数据最小字段**：

```json
{
  "worktree_id": "...",
  "invocation_id": "...",
  "task_id": "...",
  "workspace_root": "/path/to/repo",
  "path": "/path/to/.bytemind/worktrees/subagent-xxx",
  "created_at": "2026-05-08T10:00:00Z",
  "state": "active"
}
```

#### 阶段 2：自动隔离升级（安全核心）

在 `Gateway.Preflight()` 中实现"包含写工具则强制 worktree"的规则（当前架构文档已规定但未实现）。

**修改文件**：`internal/subagents/gateway.go`

在 Preflight 函数末尾，计算 `effectiveIsolation` 的逻辑改为：

```go
// 当前实现（gateway.go:172-178）：
isolation := strings.TrimSpace(request.RequestedIsolation)
if isolation == "" {
    isolation = strings.TrimSpace(definition.Isolation)
}
if isolation == "" {
    isolation = isolationNone
}

// 改为：
effectiveIsolation := resolveEffectiveIsolation(
    request.RequestedIsolation,
    definition.Isolation,
    effectiveTools,  // 传入最终工具集
    toolSpecLookup,   // 传入 ToolSpec 查询接口
)

func resolveEffectiveIsolation(
    requestedIsolation string,
    definitionIsolation string,
    effectiveTools []string,
    lookup ToolSpecLookup,
) string {
    // 1. 优先级：请求参数 > 定义默认 > 空
    isolation := firstNonEmpty(requestedIsolation, definitionIsolation)
    
    // 2. 如果工具集包含写工具，强制升级为 worktree
    hasWrite := hasDestructiveTools(effectiveTools, lookup)
    if hasWrite && isolation != isolationWorktree {
        isolation = isolationWorktree
    }
    
    if isolation == "" {
        isolation = isolationNone
    }
    return isolation
}
```

**新增辅助函数**：

```go
func hasDestructiveTools(toolNames []string, lookup ToolSpecLookup) bool {
    for _, name := range toolNames {
        spec, ok := lookup.Spec(name)
        if !ok {
            continue
        }
        if !spec.ReadOnly || spec.Destructive {
            return true
        }
    }
    return false
}
```

**注意**：Gateway 当前没有 ToolSpec 查询能力。需要注入或通过接口扩展。建议在 `PreflightRequest` 中增加可选的 `ToolSpecLookup` 字段，或在 Gateway 构造时注入。

#### 阶段 3：权限快照机制

对于可写 SubAgent，需要在启动前生成权限快照，用于审计和运行期校验。

**新增文件**：`internal/agent/subagent_approval_snapshot.go`

```go
type SubAgentApprovalSnapshot struct {
    SnapshotID           string    `json:"snapshot_id"`
    InvocationID         string    `json:"invocation_id"`
    TaskID               string    `json:"task_id"`
    ApprovalPolicy       string    `json:"approval_policy"`
    WritableRoots        []string  `json:"writable_roots"`
    AllowedTools         []string  `json:"allowed_tools"`
    EffectiveToolSetHash string    `json:"effective_toolset_hash"`
    Isolation            string    `json:"isolation"`
    WorktreePath         string    `json:"worktree_path,omitempty"`
    CreatedAt            time.Time `json:"created_at"`
}
```

**写入时机**：Preflight 通过后、子任务启动前

**修改文件**：`internal/agent/subagent_delegate.go`

在 `runtimeRequest.Execute` 闭包启动前，生成并持久化快照：

```go
// 在 delegateSubAgent() 中，遍历 preflight.EffectiveTools 后：
if !isReadOnly {
    snapshot := buildApprovalSnapshot(...)
    if err := persistSnapshot(snapshot); err != nil {
        // 快照写入失败 → 启动前失败
        result.Error = &tools.DelegateSubAgentError{
            Code:    "subagent_snapshot_failed",
            Message: "failed to persist approval snapshot",
        }
        return result, nil
    }
}
```

#### 阶段 4：审批策略差异化

当前 `nonInteractiveApproval()` 对所有子 agent 一视同仁地自动批准。对于可写 SubAgent 需要更精细的控制。

**修改文件**：`internal/agent/subagent_isolation.go`

```go
// 当前：所有子 agent 自动批准
func nonInteractiveApproval() tools.ApprovalHandler {
    return func(req tools.ApprovalRequest) (tools.ApprovalDecision, error) {
        return tools.ApprovalDecision{Disposition: tools.ApprovalApproveOnce}, nil
    }
}

// 改为：根据工具集和 isolation 模式决定审批策略
func subAgentApproval(isolation string, tools []string, lookup ToolSpecLookup) tools.ApprovalHandler {
    hasWrite := hasDestructiveTools(tools, lookup)
    
    return func(req tools.ApprovalRequest) (tools.ApprovalDecision, error) {
        switch {
        case hasWrite && isolation == "worktree":
            // worktree 隔离下，写操作在独立副本中进行 → 自动批准
            return tools.ApprovalDecision{Disposition: tools.ApprovalApproveOnce}, nil
        case hasWrite && isolation != "worktree":
            // 理论上不应到达这里（Preflight 强制 worktree），防御性拒绝
            return tools.ApprovalDecision{
                Disposition: tools.ApprovalDeny,
                Reason:      "write operations require worktree isolation",
            }, nil
        default:
            // 只读工具集 → 自动批准
            return tools.ApprovalDecision{Disposition: tools.ApprovalApproveOnce}, nil
        }
    }
}
```

#### 阶段 5：系统提示词分级（软防护）

为只读和可写 SubAgent 注入不同的行为约束提示词。

**修改文件**：`internal/agent/subagent_executor.go` 或 `internal/agent/prompt.go`

在子会话 prompt 的 `[SubAgent Runtime]` 块中增加行为约束：

```go
func renderSubAgentGuardrails(definition subagentspkg.Agent, effectiveTools []string, toolSpecs ToolSpecLookup) string {
    isReadOnly := !hasDestructiveTools(effectiveTools, toolSpecs)
    
    if isReadOnly {
        return `=== CRITICAL: READ-ONLY MODE ===
You are STRICTLY PROHIBITED from:
- Creating or modifying files
- Running shell commands that change system state
- Deleting any files

Your role is EXCLUSIVELY to read, search, and analyze existing code.`
    }
    
    return `=== WRITE MODE - FILE MODIFICATIONS ALLOWED ===
You have access to file editing tools. Follow these rules:
- Prefer editing existing files to creating new ones
- Respect the project conventions described in AGENTS.md
- Only modify files within the workspace boundary
- All changes are made in an isolated worktree — they will NOT affect the main workspace until merged`
}
```

#### 阶段 6：上下文优化

只读 agent 跳过不必要的上下文以节省 token。

**修改文件**：`internal/agent/engine_run_setup.go` 或 `internal/agent/subagent_executor.go`

在子会话的 `prepareRunPrompt` 中增加判断：

```go
// 在 prepareRunPrompt 或等效位置：
isReadOnly := !hasDestructiveTools(preflight.EffectiveTools, toolSpecs)

if isReadOnly {
    // 只读 agent 不需要项目约定和 git 状态
    promptInput.OmitClaudeMd = true
    promptInput.OmitGitStatus = true
}
```

这需要在 `PromptInput` 或等效结构中增加 `OmitClaudeMd` 和 `OmitGitStatus` 字段。

#### 阶段 7：结果通知增强（Worktree 回传）

当 worktree 中有改动时，将 worktree 信息注入到结果通知中。

**修改文件**：`internal/agent/subagent_delegate.go` 和 `internal/agent/subagent_notifier.go`

在子 agent 完成后、cleanup 之前：

```go
// 检测 worktree 改动
if preflight.Isolation == isolationWorktree && worktreeHandle != nil {
    changed, err := worktreeManager.HasChanges(ctx, worktreeHandle)
    if err != nil {
        // 检测失败：保守保留 worktree
        result.Worktree = &WorktreeInfo{
            Path:   worktreeHandle.Path,
            Branch: worktreeHandle.Branch,
            State:  "unknown",
        }
    } else if changed {
        // 有改动：保留 worktree，通知父 agent
        result.Worktree = &WorktreeInfo{
            Path:   worktreeHandle.Path,
            Branch: worktreeHandle.Branch,
            State:  "changed",
        }
    } else {
        // 无改动：自动清理
        worktreeManager.Cleanup(ctx, worktreeHandle)
    }
}
```

同时更新 `SubAgentCompletionNotification` 以包含 worktree 信息：

```go
type SubAgentCompletionNotification struct {
    // ... 现有字段 ...
    WorktreePath   string `json:"worktree_path,omitempty"`
    WorktreeBranch string `json:"worktree_branch,omitempty"`
    WorktreeState  string `json:"worktree_state,omitempty"`  // "changed" | "clean" | "unknown"
}
```

#### 阶段 8：内置可写 SubAgent 模板

提供 `general` 作为可写 SubAgent 的参考定义。

**新增文件**：`internal/subagents/general.md`

```markdown
---
name: general
description: General-purpose agent for complex multi-step tasks including file modifications. Use when the task requires both reading and writing code.
tools:
  - read_file
  - list_files
  - search_text
  - run_shell
  - write_file
  - replace_in_file
  - apply_patch
disallowed_tools:
  - delegate_subagent
max_turns: 15
timeout: 5m
isolation: worktree
---

You are a general-purpose coding agent. You can read, search, and modify code.
Prefer editing existing files to creating new ones. Only modify files within
the task scope. Return a structured summary with findings and references.
```

### 3.3 修改清单汇总

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `internal/runtime/worktree.go` | **新增** | WorktreeManager 实现 |
| `internal/runtime/worktree_test.go` | **新增** | WorktreeManager 测试 |
| `internal/subagents/gateway.go` | 修改 | 自动升级 isolation 为 worktree |
| `internal/subagents/gateway_test.go` | 修改 | 增加写工具强制隔离测试 |
| `internal/agent/subagent_isolation.go` | 修改 | 审批策略差异化 |
| `internal/agent/subagent_approval_snapshot.go` | **新增** | 权限快照机制 |
| `internal/agent/subagent_delegate.go` | 修改 | 集成 worktree + 快照 + 结果增强 |
| `internal/agent/subagent_executor.go` | 修改 | 系统提示词分级、上下文优化 |
| `internal/agent/subagent_notifier.go` | 修改 | 通知增加 worktree 字段 |
| `internal/subagents/general.md` | **新增** | 可写 SubAgent 模板定义 |
| `internal/agent/prompt_subagents.go` | 修改 | 渲染行为约束提示词 |
| `docs/subagent-architecture.md` | 修改 | 更新实现状态，标记可写 SubAgent 为已实现 |

### 3.4 安全边界再确认

```
用户定义可写 SubAgent
  │
  ├─ 工具声明
  │   └─ 交集收窄：definition.Tools ∩ parent_visible_tools
  │      └─ 差集移除：- definition.DisallowedTools - {delegate_subagent}
  │         └─ 最终工具集 ⊆ 父会话工具集（永不扩权）
  │
  ├─ 隔离策略
  │   └─ 包含写工具 → 自动升级为 worktree
  │      └─ worktree 创建失败 → 启动前拒绝
  │
  ├─ 权限快照
  │   └─ 启动前生成并持久化
  │      └─ 运行期校验：不允许快照外的新权限请求
  │
  ├─ 审批策略
  │   └─ worktree 内写操作 → 自动批准（在隔离副本中执行）
  │      └─ 非 worktree 写操作 → 拒绝（不应到达，防御性编码）
  │
  ├─ 文件系统
  │   └─ worktree 模式 → 写入独立副本 → 父工作区不受影响
  │      └─ 无改动 → 自动清理
  │      └─ 有改动 → 保留 worktree + 通知父 agent
  │
  └─ 审计
      └─ task event + approval snapshot + worktree state 全链路可追溯
```

### 3.5 MVP 不做的事

- 后台可写 SubAgent（当前后台强制只读，这个限制在 MVP 阶段保持）
- worktree 改动自动 merge 回父分支
- 跨会话 worktree 复用
- 自定义 `worktree_root` 路径配置
- 写操作的用户交互式审批（worktree 内自动批准）
- 多个可写 SubAgent 的并行执行

### 3.6 与 Claude Code 设计对比

| 维度 | Claude Code | ByteMind 当前 | ByteMind 目标 |
|------|-------------|--------------|--------------|
| 工具过滤 | `disallowedTools` 列表 | `Gateway.Preflight` 交集+差集 | 保持现有（更严格的数学集合模型） |
| 隔离级别 | `worktree` / 无隔离 / Fork | `none` / `worktree`（设计） | 实现 worktree + 自动升级规则 |
| 审批策略 | 继承/bubble/自动拒绝 | 统一自动批准 | 按 isolation+工具集差异化 |
| 系统提示词 | READ-ONLY 警告 | 无区分 | 增加行为约束提示词 |
| 上下文优化 | omitClaudeMd + omitGitStatus | 无 | 按只读/可写区分 |
| Worktree 清理 | 无改动自动删除 | 设计但未实现 | 实现完整清理+补偿+幂等 |
| 结果通知 | `<task-notification>` XML | `SubAgentCompletionNotification` | 增加 worktree 字段 |
| 安全分类器 | `classifyHandoffIfNeeded` | 无 | MVP 不做（后续扩展） |
| 内置可写 Agent | `general-purpose` | 无 | `general` 模板 |
| 一键只读 | Explore/Plan agent | explorer/review | 保持现有 |

---

## 4. 实施优先级

### P0（阻塞可写 SubAgent 上线）
1. WorktreeManager 实现（创建/清理/补偿/幂等）
2. Gateway 自动隔离升级（写工具 → worktree）
3. 审批策略差异化

### P1（质量与安全增强）
4. 权限快照机制
5. 结果通知中的 worktree 回传
6. 可写 SubAgent 模板定义 (`general`)

### P2（体验优化）
7. 系统提示词分级（软防护）
8. 上下文优化（省略 CLAUDE.md/gitStatus）

---

## 5. 测试策略

| 测试场景 | 类型 | 覆盖点 |
|----------|------|--------|
| WorktreeManager.Prepare 创建成功 | 单元 | 目录结构、分支名、owner 元数据 |
| WorktreeManager.Cleanup 删除成功 | 单元 | worktree + 元数据同步删除 |
| WorktreeManager.Cleanup 幂等 | 单元 | 重复调用不报错 |
| WorktreeManager.Reconcile 补偿清理 | 单元 | 陈旧/异常 worktree 回收 |
| Preflight 包含写工具 → isolation=worktree | 单元 | 自动升级逻辑 |
| Preflight 只读工具 + 显式 isolation=none → 保持 none | 单元 | 不误升级 |
| 可写 SubAgent 同步执行成功 + worktree 有改动 | 集成 | 完整链路 + 通知含 worktree |
| 可写 SubAgent 同步执行成功 + worktree 无改动 | 集成 | 自动清理 + 通知无 worktree |
| 可写 SubAgent worktree 创建失败 → 启动前拒绝 | 集成 | 错误码 `subagent_isolation_required` |
| 后台可写 SubAgent → 拒绝 | 单元 | 错误码 `subagent_background_write_not_allowed` |
| 快照持久化成功 + 运行期校验通过 | 单元 | 完整的快照生命周期 |
