# ByteMind 可写 SubAgent 设计方案（修订版）

## 1. 现状分析

### 1.1 当前能力

ByteMind 当前有 2 个内置只读 SubAgent（`explorer`、`review`）和 1 个同步可写 SubAgent（`general`）。架构层面已具备可写 SubAgent 的基础设施：

| 已具备 | 状态 | 位置 |
|--------|------|------|
| Agent 定义类型（含 Tools/DisallowedTools/Isolation 字段） | 已完成 | `internal/subagents/types.go:13-31` |
| 多层工具过滤（交集+差集，永不扩权） | 已完成 | `internal/subagents/gateway.go:143-160` |
| ToolSpec 分级（ReadOnly/Destructive/SafetyClass） | 已完成 | `internal/tools/spec.go:20-33` |
| 会话/Observer/Approval 隔离 | 已完成 | `internal/agent/subagent_isolation.go` |
| 同步可写 SubAgent（`general`，isolation=none） | 已完成 | `internal/subagents/general.md` |
| 同步/异步执行框架 | 已完成 | `internal/agent/subagent_delegate.go` |
| 子 agent 通知机制 | 已完成 | `internal/agent/subagent_notifier.go` |
| 系统提示词分级（只读警告 vs 写操作指南） | 已完成 | `internal/agent/subagent_executor.go:246` |
| Session 恢复/续接 | 已完成 | `internal/agent/subagent_executor.go:80-91` |
| Quota 并发控制 | 已完成 | `internal/runtime/quota.go` |

### 1.2 当前硬限制

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

同步可写 SubAgent（`general`）可以正常工作——它走的是 `RunSync` 路径，不经过这个检查。但后台模式下，即使配置了 `isolation: worktree`，也会被这行硬限制拒绝。

### 1.3 缺失的关键组件

| 缺失组件 | 影响 |
|----------|------|
| WorktreeManager（创建/清理/补偿/幂等） | 后台写操作无法隔离，是放开后台可写的阻塞项 |
| Gateway 后台隔离自动升级（background+write → worktree） | 需在 Preflight 中实现 |
| 审批策略差异化 | 当前 `nonInteractiveApproval()` 对所有子 agent 一视同仁，worktree 场景下这是正确的 |

### 1.4 明确不做的事

经过安全性分析，以下设计被评估为**过度设计**，不在本方案中：

- **权限快照（SubAgentApprovalSnapshot）**：任务元数据（invocation_id、effective_tools、effective_toolset_hash、isolation）已完整记录启动时的权限状态。快照要校验的"运行时扩权"威胁场景在 ByteMind 中不存在——子 agent 工具集由 `applySubAgentPreflightSetup()` 静态注入，运行期无路径动态获取新工具。新增独立的快照文件只会增加 I/O 失败点、清理逻辑和测试负担，收益为零。
- **bubble 权限模式**：bubble 用于"用户在线审批子 agent 工具调用"，与后台 fire-and-forget 语义矛盾。后台 + worktree 隔离 + nonInteractiveApproval 自动批准是更合理的组合。

---

## 2. 安全模型

### 2.1 核心原则

**隔离维度的正确判断标准不是工具集（读 vs 写），而是执行模式（同步 vs 后台）。**

```
同步可写  → isolation 尊重用户声明，默认 none
            用户在回路中，可看见/中断 → 不需要 worktree

后台可写  → isolation 自动升级为 worktree
            用户不在审批回路中 → 必须沙箱隔离

后台只读  → 保持现状，不需要 worktree
```

### 2.2 五层安全边界

子 agent 的权限在所有维度上都 ≤ 父 agent：

| 维度 | 父 Agent | 子 Agent（同步/后台只读） | 子 Agent（后台可写） |
|------|---------|--------------------------|---------------------|
| 可用工具 | 全部注册工具 | Preflight 交集收窄 | Preflight 交集收窄 |
| 可写路径 | WritableRoots | 深拷贝自父（子 = 父） | 覆盖为 worktree 路径（子 ⊆ 父） |
| 可执行命令 | ExecAllowlist | 深拷贝自父 | 深拷贝自父 |
| 可访问网络 | NetworkAllowlist | 深拷贝自父 | 深拷贝自父 |
| 沙箱 | Sandbox | 深拷贝自父 | 强制 enabled + required |

工具执行经过两道审批闸门：

```
第一道：PolicyGateway.DecideTool()
  → 允许清单命中 → DecisionAllow → 工具开始执行
  → 拒绝清单命中 → DecisionDeny  → 直接拒绝

第二道：execCtx.Approval() — 工具执行期间内部调用
  → executor.go  → 通用执行器审批
  → run_shell.go → shell 命令审批（sandbox 兜底）
  → worker.go    → worker 进程审批
```

第一道闸门由 Gateway.Preflight 的工具过滤保证。第二道闸门由 `nonInteractiveApproval()`（自动批准）保证。两道闸门互补，不能相互替代。

### 2.3 worktree 的作用与局限

worktree 不是替代工具过滤的，而是替代"缺失的用户审批回路"的。

- **同步执行**：用户在回路中 → 不需要 worktree
- **后台执行**：用户离线 → worktree 提供沙箱，使 `nonInteractiveApproval` 自动批准的文件写入不会污染主工作区

**worktree 的隔离范围仅限于文件工具**（`write_file`、`replace_in_file`、`apply_patch`）。这些工具通过 `WritableRoots` + workspace 路径限制写入边界。`run_shell` 不受 worktree 约束——`cmd.Dir` 落在 worktree 目录内，但 shell 命令本身可执行任意系统操作（`rm -rf /`、网络请求等）。

因此后台可写 subagent 需要额外强制 `sandbox=required`，由系统沙箱约束 shell 的文件系统访问和网络访问。worktree + sandbox 组合才能覆盖完整的"后台无人审批"安全需求。

---

## 3. 实施方案

### 阶段 1：WorktreeManager 实现（P0 — 阻塞项）

**新增文件**：`internal/runtime/worktree.go`

```go
type WorktreeManager struct {
    workspaceRoot string
    worktreesRoot string  // <workspace>/.bytemind/worktrees
    ownerDir      string  // ${BYTEMIND_HOME}/runtime/subagents/worktrees
}

// Prepare 创建临时 worktree
func (m *WorktreeManager) Prepare(ctx context.Context, req WorktreeRequest) (*WorktreeHandle, error)

// Cleanup 清理 worktree（幂等）
func (m *WorktreeManager) Cleanup(ctx context.Context, handle WorktreeHandle) error

// HasChanges 检测 worktree 是否有改动
func (m *WorktreeManager) HasChanges(ctx context.Context, handle WorktreeHandle) (bool, error)

// Reconcile 启动期补偿清理陈旧/异常 worktree
func (m *WorktreeManager) Reconcile(ctx context.Context) []ReconcileDiagnostic
```

**关键行为**：
- worktree root 固定为 `<workspace>/.bytemind/worktrees`
- worktree path 格式：`<workspace>/.bytemind/worktrees/subagent-<invocation_id>`
- 所有操作必须幂等
- Cleanup 失败时标记状态等待补偿
- Runner 启动时执行一次 Reconcile 扫描补偿

**BumpMtime**：resume 时更新 worktree 目录的 mtime，防止 stale cleanup 误删正在恢复使用的 worktree。

**owner 元数据字段**：

```json
{
  "worktree_id": "...",
  "invocation_id": "...",
  "task_id": "...",
  "workspace_root": "/path/to/repo",
  "path": "/path/to/.bytemind/worktrees/subagent-xxx",
  "transcript_session_id": "...",
  "created_at": "2026-05-08T10:00:00Z",
  "last_resumed_at": "...",
  "state": "active"
}
```

`transcript_session_id` 建立 session 与 worktree 的双向绑定，resume 时通过该字段验证 worktree 是否仍存在。worktree 不存在时降级为只读模式（fallback 到父 workspace），不阻塞恢复。

**Reconcile 四重守卫**（任何一条失败就跳过）：

1. 名称前缀匹配 → 只清理 `subagent-*` 前缀的 worktree，永不触碰用户命名的工作树
2. 跳过活跃 session → 当前 session 的 worktree 跳过
3. Mtime cutoff → 只清理超过截止时间（默认 30 天）的陈旧 worktree
4. Git 状态检查 → `git status --porcelain` 有未提交变更 → 跳过；有未推送提交 → 跳过

### 阶段 2：Gateway 后台隔离自动升级（P0 — 阻塞项）

**修改文件**：`internal/subagents/gateway.go`

在 Preflight 末尾增加 `resolveEffectiveIsolation`。判定用的写工具集合在 `gateway.go` 内独立定义（与 `subagent_executor.go:236` 的 `writeToolNames` 语义一致，但属于不同包，不跨包引用）：

```go
// gatewayWriteTools 定义哪些工具能修改文件系统，后台执行时需隔离
// 与 subagent_executor.go 的 writeToolNames 保持同步
var gatewayWriteTools = map[string]bool{
    "write_file":      true,
    "replace_in_file": true,
    "apply_patch":     true,
    "run_shell":       true,
}

func resolveEffectiveIsolation(
    requestedIsolation string,
    definitionIsolation string,
    effectiveTools []string,
    runInBackground bool,
) string {
    isolation := firstNonEmpty(requestedIsolation, definitionIsolation)

    // 后台 + 写工具（含 run_shell）→ 自动升级为 worktree
    // 同步模式不受影响（用户在回路中）
    if runInBackground && hasWriteTool(effectiveTools, gatewayWriteTools) {
        if isolation != "worktree" {
            isolation = "worktree"
        }
    }

    if isolation == "" {
        isolation = "none"
    }
    return isolation
}
```

**判定依据**：`gatewayWriteTools` 在 `gateway.go` 内独立定义，包含 `write_file`、`replace_in_file`、`apply_patch`、`run_shell`。`run_shell` 必须包含在内——shell 命令可写入文件系统，不触发 worktree 会导致后台 shell 在父 workspace 执行。`gatewayWriteTools` 与 `subagent_executor.go:236` 的 `writeToolNames` 语义一致，但由于两个包不互相引用，各自独立维护。

**无需 ToolSpec 查询**：用显式工具名集合判定。`!ReadOnly` 会误伤 `task_stop`（ReadOnly=false 但非文件写工具），显式集合避免了这个问题。

### 新增错误码

本方案在 `subagent_delegate.go` 新增以下错误码，遵循现有的 `subagent_<category>_<reason>` 命名约定：

| 错误码 | 字符串 | 语义 | 可重试 |
|--------|--------|------|--------|
| `subagent_isolation_required` | `"subagent_isolation_required"` | worktree 创建失败，无法隔离后台写操作 | false |
| `subagent_sandbox_unavailable` | `"subagent_sandbox_unavailable"` | 后台可写要求 sandbox=required 但系统沙箱不可用 | false |

现有错误码 `subagent_background_write_not_allowed` 保留（防御性），语义收窄为"后台写 tool 无 worktree"（理论上不应到达，Gateway 已自动升级）。

### 阶段 3：放开后台可写 + 绑定 worktree 到执行环境（P0 — 阻塞项）

**修改文件**：`internal/agent/subagent_delegate.go`

这是 P0-2 修复的关键：将 Preflight 产出的 `Isolation` 字符串转为具体的 worktree 句柄，并注入到子执行环境。

**数据流**：

```
Preflight → Isolation="worktree"
  → WorktreeManager.Prepare()           // 创建 worktree
  → childWorkspace = handle.Path        // 覆盖子 runner 工作目录
  → metadata["worktree_path"] = path    // 持久化绑定（供 resume 使用）
  → cfg.SandboxEnabled = true           // 后台可写强制沙箱
  → cfg.SystemSandboxMode = "required"  // 约束 shell 文件系统和网络访问
  → Execute
  → Cleanup / 保留通知
```

**具体修改**：

1. 放开只读硬限制，改为条件性检查：

```go
// 移除 isReadOnlySubAgentToolset 调用，改为：
if request.RunInBackground && hasFileWriteTools && preflight.Isolation != "worktree" {
    result.Error = &tools.DelegateSubAgentError{
        Code:      subAgentErrorCodeBackgroundWriteDenied,
        Message:   "background subagents with write tools require isolation=worktree",
        Retryable: false,
    }
    return result, nil
}
```

2. 在 `runtimeRequest.Execute` 闭包之前，创建 worktree 并切换 workspace：

```go
var worktreeHandle *WorktreeHandle
if preflight.Isolation == "worktree" {
    handle, err := worktreeManager.Prepare(ctx, WorktreeRequest{...})
    if err != nil {
        // worktree 创建失败 → 启动前拒绝
        result.Error = &tools.DelegateSubAgentError{
            Code:      subAgentErrorCodeIsolationRequired,
            Message:   fmt.Sprintf("failed to create worktree: %v", err),
            Retryable: false,
        }
        return result, nil
    }
    worktreeHandle = handle
    // 将子执行环境切换到 worktree 路径
    workspace = handle.Path
    // 限制 writable_roots 为仅 worktree，防止绝对路径写入父 workspace
    cfg.WritableRoots = []string{handle.Path}
    // 持久化绑定
    metadata["worktree_path"] = handle.Path
    // 后台可写强制 sandbox=required
    cfg.SandboxEnabled = true
    cfg.SystemSandboxMode = "required"
}
```

`WritableRoots` 覆盖是关键——子 runner 原样继承父级的 WritableRoots（`subagent_executor.go:163`），其中包含父 workspace 路径。若不覆盖，`write_file("/parent/workspace/secret.txt")` 可绕过 worktree 写入父工作区。覆盖为 `[worktreePath]` 后，文件工具的写入边界收缩到 worktree 内。

**配置覆盖如何传入子 runner**：当前子 runner 配置在 `newSubAgentChildRunner`（`subagent_executor.go:158`）内从父 runner 深拷贝。为支持 delegate 层的覆盖，在 `SubAgentExecutionInput` 增加 `Overrides` 字段：

```go
// SubAgentConfigOverrides 由 delegate 层设置，覆盖子 runner 的继承配置
type SubAgentConfigOverrides struct {
    Workspace         string
    WritableRoots     []string
    SandboxEnabled    *bool    // nil = 不覆盖
    SystemSandboxMode string   // "" = 不覆盖
}

type SubAgentExecutionInput struct {
    // ... 现有字段 ...
    Overrides *SubAgentConfigOverrides  // 可选，后台可写时由 delegate 设置
}
```

`newSubAgentChildRunner` 在深拷贝父 config 之后应用 `Overrides`（非零值覆盖）。数据流：

```
delegateSubAgent()                        // subagent_delegate.go
  ├─ 创建 worktree → workspace = handle.Path
  ├─ 设置 input.Overrides = {workspace, writableRoots, sandbox}
  └─ r.subAgentExecutor.Execute(input)    // subagent_executor.go
       └─ newSubAgentChildRunner(overrides)
            ├─ cfg = deepCopy(parentConfig)       // 现有逻辑
            ├─ if overrides.Workspace != "" → workspace = overrides.Workspace
            ├─ if overrides.WritableRoots != nil → cfg.WritableRoots = overrides.WritableRoots
            └─ if overrides.SandboxEnabled != nil → cfg.SandboxEnabled = *ov.SandboxEnabled
```

3. Resume 时 worktree 缺失的降级处理：

```go
// 在 subagent_executor.go resume 路径中：
if resumeID != "" && preflight.Isolation == "worktree" {
    // 通过 transcript_session_id 查找 worktree
    exists, handle, err := worktreeManager.FindBySession(ctx, resumeID)
    if err != nil || !exists {
        // worktree 不存在 → 降级为只读
        // 从 effectiveTools 中移除文件写工具和 run_shell
        preflight.EffectiveTools = removeWriteTools(preflight.EffectiveTools)
        preflight.Isolation = "none"
        workspace = parentWorkspace
        // 重新构建 allowedTools
        preflight.AllowedTools = buildAllowedToolSet(preflight.EffectiveTools)
    } else {
        // worktree 存在 → BumpMtime + 恢复
        worktreeManager.BumpMtime(ctx, handle)
        workspace = handle.Path
    }
}
```

降级不是靠修改审批策略（`nonInteractiveApproval` 仍自动批准），而是**直接收窄工具集**——移除文件写工具和 `run_shell` 后，子 agent 没有可用的写入口。比改审批策略更可靠（第一道闸门拦截，而非依赖第二道）。

4. Sandbox 可用性预检查（后台可写的前置条件）：

```go
// 在强制 sandbox=required 之前检查
if sandboxStatus, err := resolveAgentSystemSandboxRuntimeStatus(true, "required"); err != nil {
    // sandbox required 不可用（如 Windows 无 bwrap、Linux 无 unshare）
    result.Error = &tools.DelegateSubAgentError{
        Code:      subAgentErrorCodeSandboxUnavailable,
        Message:   fmt.Sprintf("background writable subagent requires system sandbox: %v", err),
        Retryable: false,
    }
    return result, nil
}
```

不做静默降级——sandbox 不可用时直接拒绝，给出明确错误信息。避免用户以为有沙箱保护实际没有。

5. 新增错误码常量（在 `subagent_delegate.go` 头部的常量块中）：

```go
subAgentErrorCodeIsolationRequired  = "subagent_isolation_required"
subAgentErrorCodeSandboxUnavailable = "subagent_sandbox_unavailable"
```

6. 后台可写需要 `hasWriteTool` 检查，复用阶段 2 相同的 `gatewayWriteTools` 集合。

逻辑：
- 后台 + 只读工具 → 放行（不变）
- 后台 + 文件写工具 + worktree → 创建 worktree + 切换 workspace + 强制 sandbox → 放行（新路径）
- 后台 + 文件写工具 + 无 worktree → 拒绝（防御性保留）
- worktree 创建失败 → 启动前拒绝（错误码 `subagent_isolation_required`）

### 阶段 4：Worktree 结果通知增强（P1）

**修改文件**：`internal/agent/subagent_delegate.go` 和 `internal/agent/subagent_notifier.go`

子 agent 完成后，检测 worktree 状态并注入到结果和通知中：

```go
// 检测 worktree 改动
if preflight.Isolation == "worktree" && worktreeHandle != nil {
    changed, err := worktreeManager.HasChanges(ctx, worktreeHandle)
    if err != nil {
        result.Worktree = &WorktreeInfo{
            Path:   worktreeHandle.Path,
            Branch: worktreeHandle.Branch,
            State:  "unknown",
        }
    } else if changed {
        result.Worktree = &WorktreeInfo{
            Path:   worktreeHandle.Path,
            Branch: worktreeHandle.Branch,
            State:  "changed",
        }
    } else {
        // 无改动 → 自动清理
        worktreeManager.Cleanup(ctx, worktreeHandle)
    }
}
```

- 无改动 → 自动清理 worktree
- 有改动 → 保留 worktree，通过 `SubAgentCompletionNotification` 通知父 agent（含 path、branch、state）
- 检测失败 → 保守保留，state 标记为 "unknown"

---

## 4. 修改清单汇总

| 文件 | 变更类型 | 说明 |
|------|----------|------|
| `internal/runtime/worktree.go` | **新增** | WorktreeManager（Prepare/Cleanup/HasChanges/BumpMtime/Reconcile） |
| `internal/runtime/worktree_test.go` | **新增** | WorktreeManager 测试 |
| `internal/subagents/gateway.go` | 修改 | 后台+写工具自动升级 isolation；复用 writeToolNames 判定 |
| `internal/subagents/gateway_test.go` | 修改 | 自动升级 + 不误升级 + 不误伤非文件写工具 测试 |
| `internal/agent/subagent_delegate.go` | 修改 | 放开后台可写；创建 worktree + 切换 workspace + 覆盖 WritableRoots；sandbox 可用性预检查 + 强制 required；注入 worktree_path |
| `internal/agent/subagent_executor.go` | 修改 | 新增 `SubAgentConfigOverrides` 类型；`SubAgentExecutionInput` 增加 `Overrides` 字段；`newSubAgentChildRunner` 应用覆盖；resume 降级逻辑 |
| `internal/agent/subagent_notifier.go` | 修改 | 通知增加 worktree 字段 |

### 不需要修改的代码文件

| 文件 | 原因 |
|------|------|
| `internal/agent/subagent_isolation.go` | `nonInteractiveApproval()` 保持不变——worktree 隔离下自动批准是正确的行为 |
| `internal/agent/subagent_approval_snapshot.go` | **不创建**——评估为过度设计，现有任务元数据已足够 |

### 需要同步的文档

| 文件 | 说明 |
|------|------|
| `docs/subagent-architecture.md` | 更新"MVP 后台仅只读 + 预审批快照"为"后台可写 + worktree 自动升级"，同步错误码说明 |

---

## 5. 安全边界总览

```
后台可写 SubAgent 启动
  │
  ├─ 工具声明
  │   └─ 交集收窄：definition.Tools ∩ parent_visible_tools
  │      └─ 差集移除：- definition.DisallowedTools - {delegate_subagent}
  │         └─ 最终工具集 ⊆ 父会话工具集（永不扩权）
  │
  ├─ 隔离策略
  │   └─ 后台 + 文件写工具 → 自动升级为 worktree
  │      └─ worktree 创建 → 切换 runner workspace 为 worktree 路径
  │      └─ worktree_path 写入 task metadata（供 resume 绑定）
  │      └─ worktree 创建失败 → 启动前拒绝
  │      └─ 同步可写 → 不受影响（isolation 尊重用户声明）
  │
  ├─ Shell 安全
  │   └─ 后台可写 → 强制 SandboxEnabled=true + SystemSandboxMode="required"
  │      └─ 约束 shell 的文件系统访问和网络访问
  │      └─ worktree 仅隔离文件工具，shell 由 sandbox 兜底
  │
  ├─ 审批策略
  │   └─ worktree 内写操作 → nonInteractiveApproval 自动批准（沙箱内安全）
  │      └─ 非 worktree 后台写 → 不应到达（Gateway 已自动升级，防御性拒绝）
  │
  ├─ 文件系统
  │   └─ worktree 模式 → 写入独立副本 → 父工作区不受影响
  │      └─ 无改动 → 自动清理
  │      └─ 有改动 → 保留 worktree + 通知父 agent
  │
  ├─ 续接
  │   └─ resume 时通过 transcript_session_id 查找 worktree
  │      └─ worktree 存在 → BumpMtime + 恢复
  │      └─ worktree 不存在 → 从 effectiveTools 移除写工具+run_shell（收窄工具集而非改审批策略）
  │
  └─ 配置继承
      └─ WritableRoots / ExecAllowlist / NetworkAllowlist
         └─ 深拷贝自父配置 → 所有维度 ≤ 父 agent
```

## 6. 测试策略

| 测试场景 | 类型 | 覆盖点 |
|----------|------|--------|
| WorktreeManager.Prepare 创建成功 | 单元 | 目录结构、分支名、owner 元数据 |
| WorktreeManager.Cleanup 删除成功 | 单元 | worktree + 元数据同步删除 |
| WorktreeManager.Cleanup 幂等 | 单元 | 重复调用不报错 |
| WorktreeManager.HasChanges 无改动 | 单元 | 返回 false |
| WorktreeManager.HasChanges 有改动 | 单元 | 返回 true |
| WorktreeManager.BumpMtime 更新 mtime | 单元 | resume 时防止 stale cleanup 误删 |
| WorktreeManager.Reconcile 四重守卫 | 单元 | 名称模式 + session + mtime + git 状态 |
| WorktreeManager.Reconcile 有未提交变更 → 跳过 | 单元 | git status --porcelain 检查 |
| Preflight 后台+文件写工具 → isolation=worktree | 单元 | 自动升级逻辑 |
| Preflight 同步+文件写工具 → 保持用户声明的 isolation | 单元 | 不误升级 |
| Preflight 后台+只读工具 → 保持 none | 单元 | 不误升级 |
| Preflight 后台+task_stop 不会触发升级 | 单元 | writeToolNames 集合不会误伤非文件写工具 |
| Preflight 后台+run_shell 触发 worktree 升级 | 单元 | run_shell 在 writeToolNames 中 |
| 后台可写：worktree 创建成功 → 切换 workspace + 强制 sandbox + 覆盖 WritableRoots | 集成 | 完整绑定链路 |
| 后台可写：WritableRoots 被限制为 worktree 路径 | 单元 | 绝对路径无法写入父 workspace |
| 后台可写：worktree 有改动 → 保留 + 通知含 worktree | 集成 | 完整链路 + 通知含 worktree |
| 后台可写：worktree 无改动 → 自动清理 | 集成 | 自动清理 + 通知无 worktree |
| 后台可写：worktree 创建失败 → 启动前拒绝 | 集成 | 错误码 `subagent_isolation_required` |
| 后台可写 SubAgent 无 worktree → 拒绝 | 单元 | 防御性拒绝 |
| resume：worktree 存在 → BumpMtime + 恢复 | 集成 | resume 时不触发 stale cleanup |
| resume：worktree 不存在 → 收窄工具集（移除写工具+run_shell） | 集成 | 降级靠工具集收窄，不崩溃 |
| resume：worktree 不存在 → 仍有写工具但限制写入路径的场景不存在 | 单元 | 降级后的工具集不包含文件写工具 |
| Sandbox 不可用 + 后台可写 → 启动前拒绝 | 单元 | 明确错误码，不做静默降级 |
| Sandbox 强制启用：后台可写 → SandboxEnabled=true, mode=required | 单元 | delegate 中配置检查 |
| 同步可写 SubAgent（general）正常运行 | 回归 | 现有行为不受影响 |

## 7. 与 Claude Code 设计对比

| 维度 | Claude Code | ByteMind 当前 | ByteMind 目标 |
|------|-------------|--------------|--------------|
| 工具过滤 | `disallowedTools` 列表 | `Gateway.Preflight` 交集+差集 | 保持现有（更严格的数学集合模型） |
| 隔离级别 | worktree / 无隔离 | none / worktree（部分实现） | 实现 worktree + 后台写自动升级 |
| 审批策略 | 继承/bubble/auto-deny | 统一 nonInteractiveApproval 自动批准 | 保持现有（worktree 隔离下合理） |
| 系统提示词 | READ-ONLY 警告 | 已有（buildToolSafetyGuardrails） | 保持现有 |
| 结果通知 | `<task-notification>` XML | `SubAgentCompletionNotification` | 增加 worktree 字段 |
| Shell 沙箱 | OS sandbox (bwrap) | SandboxEnabled 继承父级 | 后台可写强制 required |
| Worktree 数据流 | AsyncLocalStorage CWD 劫持 | 未实现 | Preflight → Prepare → 切换 workspace |
| 写工具判定 | 精确枚举白名单 | `!ReadOnly` 误伤 task_stop 等 | 显式 fileWriteToolNames 集合 |
| 续接补偿 | metadata 绑定 + mtime bump + 降级 | session resume 已有 | worktree 绑定 + BumpMtime + 降级 |
| 内置可写 Agent | general-purpose | general（同步已可用） | 扩展为后台也可用 |
| 审批快照 | 无 | 无（设计但评估为过度设计） | 不做 |

## 8. MVP 预留（为后续迭代预留的接口/字段）

当前只实现最小路径（P0: WorktreeManager + Gateway 自动升级 + 放开后台可写），但以下预留以零成本避免将来的 breaking change：

### 已预留

**`Agent.PermissionMode` 字段**（`internal/subagents/types.go:28`）

```go
PermissionMode string // reserved: inherit, bubble, acceptEdits, plan
```

MVP 中该字段为空，`nonInteractiveApproval()` 统一处理所有审批。将来启用后，用户可在 agent 定义中声明 `permission_mode: bubble` 将审批请求冒泡到父终端。

### 无需预留（现有机制已覆盖）

**异步工具白名单**：`Gateway.Preflight` 的交集+差集模型 + `Agent.DisallowedTools` 已能精确控制后台可用工具集，不需要额外的 `ASYNC_DISALLOWED_TOOLS` 常量。用户定义后台可写 agent 时，在 `disallowed_tools` 中排除交互类工具即可。

**Transcript 持久化**：`subagent_executor.go:147-153` 已通过 `SessionStore.Save()` 持久化子会话，`TranscriptSessionID` 回写到结果中。恢复路径已验证。

**自动后台化**：`DelegateSubAgentResult.Status` 当前值为 `"completed"` / `"accepted"`，将来可加 `"auto_backgrounded"`，不破坏现有协议。

### 将来扩展点（非 MVP 范围，但设计了扩展方式）

**中间进度事件**：`SubAgentNotifier` 当前有 `NotifyCompletion` / `DrainPending` 两个方法。进度追踪可通过**可选接口**扩展，不破坏现有实现者：

```go
// 将来定义，非 MVP
type SubAgentProgressNotifier interface {
    NotifyProgress(SubAgentProgressNotification)
}
```

调用方做类型断言：`if pn, ok := notifier.(SubAgentProgressNotifier); ok { pn.NotifyProgress(...) }`。这是 Go 的标准扩展模式。

**安全分类器**：在 `SubAgentCompletionNotification` 中预留 `SecurityWarning string` 字段（当前为空），将来分类器检测到问题时填入。无需新接口。
