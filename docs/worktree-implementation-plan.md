# WorktreeManager 实现方案

## 目标

实现通用的 git worktree 隔离基础设施，为同步可写 subagent 提供可选的沙箱执行环境。

```
本次做：
  P0: WorktreeManager 实现（通用基础设施）
  P1: 同步可写 agent 可选 isolation=worktree，写操作落入独立副本

暂不做：
  后台可写（保持硬限制）
  Gateway 自动升级 isolation（isolation 完全由 agent 定义或调用参数决定）
  BumpMtime / FindBySession / Reconcile（留到 resume/reconcile 迭代）
```

## 修改清单

| 文件 | 变更 | 说明 |
|------|------|------|
| `internal/runtime/worktree.go` | **新增** | WorktreeManager（Prepare / Cleanup / HasChanges） |
| `internal/runtime/worktree_test.go` | **新增** | 单元测试 |
| `internal/agent/subagent_executor.go` | 修改 | `SubAgentExecutionInput` 增加 `Overrides` 字段；`newSubAgentChildRunner` 应用覆盖 |
| `internal/agent/subagent_delegate.go` | 修改 | 同步路径：isolation=worktree 时 Prepare → Overrides → Execute → HasChanges → Cleanup/保留 |
| `internal/agent/subagent_notifier.go` | 修改 | `SubAgentCompletionNotification` 增加 worktree 字段 |
| `internal/tools/delegate_subagent.go` | 修改 | `DelegateSubAgentResult` 增加 `Worktree` 字段 |

### 不修改的文件

| 文件 | 原因 |
|------|------|
| `internal/subagents/gateway.go` | 不做自动升级，isolation 由 agent 定义声明 |
| `internal/agent/subagent_isolation.go` | `nonInteractiveApproval()` 保持不变 |

---

## 阶段 1：WorktreeManager 实现

### 接口定义

```go
// internal/runtime/worktree.go

type WorktreeHandle struct {
    ID     string
    Path   string
    Branch string
    Commit string  // 创建时的 HEAD commit，用于 HasChanges 对比
}

type WorktreeRequest struct {
    InvocationID  string
    WorkspaceRoot string
}

type WorktreeManager struct {
    workspaceRoot string
    worktreesRoot string  // <workspace>/.bytemind/worktrees
    ownerDir      string
}

// NewWorktreeManager 创建管理器。
// 若 workspace 不是 git 仓库则返回 nil, error。
func NewWorktreeManager(workspaceRoot string) (*WorktreeManager, error)

// Prepare 创建临时 worktree，写入 owner 元数据。
func (m *WorktreeManager) Prepare(ctx context.Context, req WorktreeRequest) (*WorktreeHandle, error)

// Cleanup 删除 worktree、分支和 owner 元数据，各自幂等。
func (m *WorktreeManager) Cleanup(ctx context.Context, h *WorktreeHandle) error

// HasChanges 检测 worktree 是否有未提交改动。
func (m *WorktreeManager) HasChanges(ctx context.Context, h *WorktreeHandle) (bool, error)
```

### 实现细节

**NewWorktreeManager**：
1. `git -C <workspace> rev-parse --git-dir` → 失败则返回 `nil, error`
2. 设置 `worktreesRoot` 和 `ownerDir`，若目录不存在则创建
3. 返回 manager

**Prepare**：
1. `git -C <workspace> rev-parse HEAD` → 记录为 Commit
2. 生成 worktree 路径：`<worktreesRoot>/subagent-<invocation_id>`
3. `git worktree add -b agent-<invocation_id> <path> HEAD`
4. 创建 owner 元数据文件 `<ownerDir>/<invocation_id>.json`：

```json
{
  "worktree_id": "<invocation_id>",
  "path": "<worktree_path>",
  "branch": "agent-<invocation_id>",
  "commit": "<HEAD commit>",
  "created_at": "<ISO 8601>",
  "state": "active"
}
```

**Cleanup**（每步独立，忽略不存在的错误）：
1. `git worktree remove <path> --force`
2. `git branch -D agent-<invocation_id>`
3. 删除 owner 元数据文件

**HasChanges**：
1. `git -C <path> status --porcelain` — 有输出 ⇒ true

### 错误处理

| 场景 | 行为 |
|------|------|
| workspace 不是 git 仓库 | `NewWorktreeManager` 返回 nil + error |
| `git worktree add` 失败 | Prepare 返回 error |
| `git worktree remove` 失败 | Cleanup 吞掉错误，owner 元数据标记 `state: cleanup_failed`，后续手动或 Reconcile 清理 |
| `git status --porcelain` 失败 | HasChanges 返回 error（调用方保守保留 worktree） |

---

## 阶段 2：配置覆盖数据流

### SubAgentExecutionInput 扩展

```go
// internal/agent/subagent_executor.go

type SubAgentConfigOverrides struct {
    Workspace     string
    WritableRoots []string
}

type SubAgentExecutionInput struct {
    // ... 现有字段 ...
    Overrides *SubAgentConfigOverrides
}
```

### newSubAgentChildRunner 应用覆盖

在深拷贝父 config 之后：

```go
if overrides := input.Overrides; overrides != nil {
    if overrides.Workspace != "" {
        workspace = overrides.Workspace
    }
    if overrides.WritableRoots != nil {
        cfg.WritableRoots = overrides.WritableRoots
    }
}
```

不覆盖 ExecAllowlist / NetworkAllowlist / Sandbox——这些继续从父配置继承。

### 数据流

```
delegateSubAgent()
  │
  ├─ preflight.Isolation == "worktree" ？
  │   ├─ r.worktreeManager == nil ？
  │   │   └─ 返回 subagent_isolation_required 错误（用户显式请求 worktree，不静默降级）
  │   ├─ worktreeManager.Prepare(ctx, req)
  │   │   └─ 失败 → 返回 subagent_isolation_required 错误
  │   ├─ input.Overrides = &SubAgentConfigOverrides{
  │   │     Workspace:     handle.Path,
  │   │     WritableRoots: []string{handle.Path},
  │   │   }
  │   └─ metadata["worktree_path"] = handle.Path
  │
  ├─ r.subAgentExecutor.Execute(ctx, input)
  │
  ├─ 执行完毕 → worktreeManager.HasChanges(handle)
  │   ├─ 无改动 → Cleanup
  │   ├─ 有改动 → 保留 + result.Worktree = {Path, Branch, State: "changed"}
  │   └─ 检测失败 → 保留 + result.Worktree = {Path, Branch, State: "unknown"}
  │
  └─ return result
```

---

## 阶段 3：delegate 层改动

### 同步路径 worktree integration

在 `delegateSubAgent()` 同步路径中，`RunSync` 调用之前插入：

```go
var worktreeHandle *runtimepkg.WorktreeHandle
if preflight.Isolation == "worktree" {
    if r.worktreeManager == nil {
        result.Error = &tools.DelegateSubAgentError{
            Code:      "subagent_isolation_required",
            Message:   "worktree isolation requested but workspace is not a git repository",
            Retryable: false,
        }
        return result, nil
    }
    handle, err := r.worktreeManager.Prepare(ctx, runtimepkg.WorktreeRequest{
        InvocationID:  result.InvocationID,
        WorkspaceRoot: r.workspace,
    })
    if err != nil {
        result.Error = &tools.DelegateSubAgentError{
            Code:      "subagent_isolation_required",
            Message:   fmt.Sprintf("failed to create worktree: %v", err),
            Retryable: false,
        }
        return result, nil
    }
    worktreeHandle = handle
    metadata["worktree_path"] = handle.Path

    input.Overrides = &SubAgentConfigOverrides{
        Workspace:     handle.Path,
        WritableRoots: []string{handle.Path},
    }
}
```

`RunSync` 调用之后插入 worktree 结果检测：

```go
if worktreeHandle != nil {
    changed, err := r.worktreeManager.HasChanges(ctx, worktreeHandle)
    if err != nil || changed {
        state := "changed"
        if err != nil {
            state = "unknown"
        }
        result.Worktree = &tools.WorktreeInfo{
            Path:   worktreeHandle.Path,
            Branch: worktreeHandle.Branch,
            State:  state,
        }
    } else {
        _ = r.worktreeManager.Cleanup(ctx, worktreeHandle)
    }
}
```

### 后台路径不变

```go
if request.RunInBackground {
    // ... 现有检查全部保留 ...
    if !r.isReadOnlySubAgentToolset(preflight.EffectiveTools) {
        // 后台 + 写工具 = 拒绝（本次不放开）
    }
}
```

### Runner 注入

`Runner` struct 增加字段，在 `NewRunner` 时通过 `NewWorktreeManager` 初始化：

```go
type Runner struct {
    // ... 现有字段 ...
    worktreeManager *runtimepkg.WorktreeManager
}
```

若 workspace 不是 git 仓库，`worktreeManager` 为 nil。后续 `isolation=none` 的正常路径不受影响，只有显式指定 `isolation=worktree` 时才会检查并拒绝。

---

## 阶段 4：协议变更与兼容说明

### DelegateSubAgentResult 增加字段

```go
// internal/tools/delegate_subagent.go

type WorktreeInfo struct {
    Path   string `json:"path,omitempty"`
    Branch string `json:"branch,omitempty"`
    State  string `json:"state,omitempty"` // "changed" | "unknown"
}

type DelegateSubAgentResult struct {
    // ... 现有字段不变 ...
    Worktree *WorktreeInfo `json:"worktree,omitempty"`
}
```

**兼容性**：新增 `omitempty` 字段。已有 consumer（TUI 渲染、父 agent 提示词）忽略未知字段，不受影响。若 `Worktree == nil`，行为与当前完全一致。

### SubAgentCompletionNotification 增加字段

```go
// internal/agent/subagent_notifier.go

type SubAgentCompletionNotification struct {
    // ... 现有字段不变 ...
    WorktreePath   string `json:"worktree_path,omitempty"`
    WorktreeBranch string `json:"worktree_branch,omitempty"`
    WorktreeState  string `json:"worktree_state,omitempty"`
}
```

**兼容性**：同上，`omitempty` 字段。通知消息渲染器（`buildNotificationMessage`）在 worktree 字段非空时追加 worktree 信息行；为空时不追加，渲染结果与当前一致。

### 消费方改造顺序

1. TUI 结果卡片 → 已有 `DelegateSubAgentResult` 渲染逻辑，应在 worktree 非 nil 时显示 worktree 状态
2. 父 agent 系统提示词 → 可在 `formatSubAgentRuntime` 中追加 worktree 提示，告知父 agent 可查看 worktree 路径

---

## 安全边界

```
同步可写 + isolation=worktree
  │
  ├─ 前置检查：worktreeManager == nil ？
  │   └─ 用户显式请求 worktree → 拒绝，不静默降级
  ├─ WorktreeManager.Prepare → git worktree add
  ├─ WritableRoots = [worktreePath]  ← 防止绝对路径写回父 workspace
  ├─ workspace = worktreePath         ← 全部工具工作目录落在 worktree
  ├─ nonInteractiveApproval 自动批准  ← 写操作在隔离副本内安全
  ├─ 执行完毕 → HasChanges
  │   ├─ 无改动 → Cleanup
  │   └─ 有改动 → 保留 worktree + result.Worktree 通知用户
  └─ 父 workspace 完全不受影响
```

WritableRoots 收窄是防止绝对路径逃逸的关键：

```
子 agent write_file("/parent-workspace/secret.txt")
  → 工具执行器检查：/parent-workspace/secret.txt 不在 WritableRoots 中
  → 拒绝
```

---

## 测试策略

| 场景 | 类型 | 覆盖点 |
|------|------|--------|
| NewWorktreeManager：git 仓库 → 成功 | 单元 | manager != nil |
| NewWorktreeManager：非 git 仓库 → error | 单元 | 明确错误信息 |
| Prepare：创建 worktree + owner 元数据 | 单元 | 目录存在、分支名正确、元数据完整 |
| Cleanup：删除 worktree + 分支 + 元数据 | 单元 | 三步执行、各自不因缺失报错 |
| Cleanup：重复调用不报错 | 单元 | 幂等 |
| HasChanges：无改动 → false | 单元 | status --porcelain 为空 |
| HasChanges：有文件修改 → true | 单元 | status --porcelain 有输出 |
| 同步可写 + isolation=worktree + git 仓库 | 集成 | Prepare → Overrides → Execute → HasChanges → Cleanup/保留 |
| 同步可写 + isolation=worktree + 非 git 仓库 | 单元 | 返回 `subagent_isolation_required`，不执行 |
| WritableRoots 覆盖 → write_file 绝对路径被拒绝 | 集成 | 写入父 workspace 路径失败 |
| 同步可写 + isolation=none | 回归 | general agent 照常运行 |
| 后台 + 写工具 | 回归 | 现有限制不变，返回拒绝 |
