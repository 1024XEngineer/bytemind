# 工具与审批

工具是 ByteMind Agent 能够执行的具体操作单元。了解工具分类和审批流程，可以让你在保持效率的同时握控每一步执行。

## 工具列表

| 工具              | 分类     | 功能                          |
| ----------------- | -------- | ----------------------------- |
| `list_files`      | 读       | 列出目录结构                  |
| `read_file`       | 读       | 读取文件内容                  |
| `search_text`     | 读       | 全文搜索（支持正则）          |
| `write_file`      | **写**   | 创建或覆盖写入文件            |
| `replace_in_file` | **写**   | 替换文件中的指定内容          |
| `apply_patch`     | **写**   | 应用 unified diff 补丁        |
| `run_shell`       | **执行** | 执行 Shell 命令               |
| `delegate_subagent`| 代理     | 委派子代理执行子任务          |
| `task_output`      | 任务     | 查询后台任务的输出            |
| `task_stop`        | 任务     | 终止后台任务                  |
| `update_plan`     | 计划     | 更新任务执行计划（Plan 模式） |
| `web_fetch`       | 网络     | 抓取网页内容                  |
| `web_search`      | 网络     | 联网搜索                      |

读类工具默默执行。**写入和执行类工具**在运行前会弹出审批提示。

## 审批流程

Agent 调用高风险工具时，会展示操作摘要并弹出三个选项：

| 选项 | 行为 |
| ---- | ---- |
| **Approve this operation only** | 仅允许当前这一次调用，同一工具下次还会弹出审批 |
| **Approve later requests from this tool** | 当前 TUI 会话内，该工具的后续调用自动通过 |
| **Disable approvals for this TUI session** | 当前会话内所有工具的审批请求全部自动通过 |

此外也可以直接**拒绝**（`Esc` 关闭对话框或选择拒绝）。

默认的 `approval_policy: on-request` 对每次高风险工具调用都开启此流程。

## 执行命令白名单

对于不希望重复确认的可信命令，在配置中定义 `exec_allowlist`：

```json
{
  "exec_allowlist": [
    { "command": "go", "args_pattern": ["test", "./..."] },
    { "command": "make", "args_pattern": ["build"] }
  ]
}
```

在白名单中的命令不会弹出审批提示。

## Full Access 模式

无人值守场景（CI、流水线）下，配置 `approval_mode: full_access`，审批请求会自动通过，任务不再被弹窗阻塞：

```json
{
  "approval_mode": "full_access"
}
```

兼容说明：为避免静默提权，`approval_mode: away` 默认被阻止。仅在迁移旧配置时，显式设置 `BYTEMIND_ALLOW_AWAY_FULL_ACCESS=true` 才会临时映射到 `full_access`。

完整审批配置见[配置详解](/zh/configuration)。

## 相关页面

- [配置](/zh/configuration) — 审批策略、权限模式、沙箱
- [单次执行模式](/zh/usage/run-mode) — 自动化非交互执行
- [子代理](/zh/usage/subagents) — 委派子代理执行
- [MCP 配置与使用](/zh/usage/mcp) — 通过 MCP 扩展工具
- [沙箱](/zh/usage/sandbox) — 文件与命令执行边界
- [核心概念](/zh/core-concepts) — 工具概述
