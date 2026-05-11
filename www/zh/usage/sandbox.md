# 沙箱

ByteMind 提供多层级沙箱机制，限制 Agent 的文件访问、命令执行和网络请求范围。沙箱确保 Agent 在执行任务时不会超出预期边界，同时审批流程仍然生效。

## 沙箱层级

ByteMind 的沙箱在三个层面提供保护：

| 层级       | 控制目标       | 配置方式             |
| ---------- | -------------- | -------------------- |
| 文件系统   | 读写目录范围   | `writable_roots`     |
| 命令执行   | 可执行命令白名单 | `exec_allowlist`   |
| 网络访问   | 允许访问的域名/IP | `network_allowlist` |

### 文件系统沙箱

启用后，写操作（`write_file`、`replace_in_file`、`apply_patch`）被限制在 `writable_roots` 指定的目录内。读操作不受影响。

```json
{
  "sandbox_enabled": true,
  "writable_roots": ["./src", "./tests", "./docs"]
}
```

此配置下，Agent 只能对 `./src`、`./tests`、`./docs` 三个目录下的文件执行写入操作。尝试写入其他位置（如根目录配置、系统文件）将被拦截。

### 命令执行沙箱

`exec_allowlist` 定义可跳过审批直接执行的命令白名单，不在白名单中的命令仍需审批或触发沙箱拦截：

```json
{
  "exec_allowlist": [
    { "command": "go", "args_pattern": ["test", "./..."] },
    { "command": "go", "args_pattern": ["build"] },
    { "command": "make", "args_pattern": ["build"] },
    { "command": "npm", "args_pattern": ["test"] },
    { "command": "git", "args_pattern": ["status"] }
  ]
}
```

每条规则包含 `command`（可执行文件名）和 `args_pattern`（参数匹配模式）。`args_pattern` 为前缀匹配：`["test", "./..."]` 同时匹配 `go test ./...` 和 `go test ./... -v`。

### 网络沙箱

`network_allowlist` 限制 Agent 执行网络请求时可访问的目标（适用于 `web_fetch`、`web_search` 等工具以及 `run_shell` 中的网络操作）：

```json
{
  "network_allowlist": [
    { "host": "api.github.com" },
    { "host": "*.openai.com", "port": 443 }
  ]
}
```

## 系统沙箱模式

`system_sandbox_mode` 决定沙箱的底层实现机制：

| 值         | 说明                         | 适用平台  |
| ---------- | ---------------------------- | --------- |
| `none`     | 不使用系统级沙箱（默认）     | 所有平台  |
| `profile`  | 基于 OS 配置文件限制进程权限 | macOS     |
| `bwrap`    | 基于 Bubblewrap 的文件系统隔离 | Linux   |
| `bind`     | 基于 bind mount 的隔离       | Linux     |

```json
{
  "system_sandbox_mode": "bwrap",
  "sandbox_enabled": true
}
```

## 与审批模式的关系

沙箱和审批系统独立但互补：

| 场景                    | 沙箱行为               | 审批行为           |
| ----------------------- | ---------------------- | ------------------ |
| 写入 `writable_roots` 内文件 | 允许                   | 仍需审批（默认）   |
| 写入 `writable_roots` 外文件 | **拦截**               | 不触发审批         |
| 执行 allowlist 中的命令     | 跳过审批               | 不显示审批提示     |
| 执行未知命令            | 触发风险判定           | 弹出审批提示       |
| 访问 allowlist 中的网络     | 允许                   | 不触发审批         |
| 访问未知网络目标        | 触发风险判定           | 弹出审批提示       |

即使配置了 `full_access` 审批模式，沙箱的文件系统和网络限制仍然生效。沙箱提供的是硬性边界，审批提供的是交互式门控。

## 通过环境变量启用

```bash
# 启用沙箱并指定可写目录
BYTEMIND_SANDBOX_ENABLED=true BYTEMIND_WRITABLE_ROOTS=./src,./tests bytemind

# 启用 Linux 系统沙箱
BYTEMIND_SANDBOX_ENABLED=true BYTEMIND_SYSTEM_SANDBOX_MODE=bwrap bytemind
```

## 最佳实践

1. **渐进启用** — 先在可信项目中开启，确认工作流正常再推广
2. **最小权限** — `writable_roots` 只包含确实需要修改的目录
3. **与 exec_allowlist 配合** — 将常用安全命令（`go test`、`npm test` 等）加入白名单减少审批噪音
4. **CI 环境加 full_access** — CI 流水线中配置 `full_access` 避免阻塞，沙箱提供硬性保护

## 相关页面

- [工具与审批](/zh/usage/tools-and-approval) — 工具审批机制
- [配置](/zh/configuration) — 完整沙箱配置选项
- [单次执行模式](/zh/usage/run-mode) — CI 自动化的沙箱最佳实践
