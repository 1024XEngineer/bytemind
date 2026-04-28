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
| `update_plan`     | 计划     | 更新任务执行计划（Plan 模式） |
| `web_fetch`       | 网络     | 抓取网页内容                  |
| `web_search`      | 网络     | 联网搜索                      |

读类工具默默执行。**写入和执行类工具**在运行前会弹出审批提示。

## 审批流程

Agent 调用高风险工具时，会展示：

- 工具名称和将要使用的具体参数
- 操作摘要说明
- 等待你确认（`y`）、拒绝（`n`）或说明理由

默认的 `approval_policy: on-request` 对每次高风险工具调用都开启此流程。

## 推荐工作流程

1. **先分析**—请 Agent 先读取并解释，不做任何修改
2. **审阅计划**—确认哪些文件会被动及原因
3. **逐步审批**—确认每次写入操作前先审阅内容
4. **复杂任务使用 Plan 模式**，在执行前看到完整范围

```text
先读取相关文件并告诉我你的修改建议，不要写任何内容。
```

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

## Away 模式

无人値守场景（CI、流水线）下，配置 `approval_mode: away` 让 Agent 不需阻塞等待输入：

```json
{
  "approval_mode": "away",
  "away_policy": "auto_deny_continue"
}
```

完整审批配置见[配置详解](/zh/configuration)。

## 相关页面

- [配置](/zh/configuration) — 审批策略、Away 模式、沙筱
- [单次执行模式](/zh/usage/run-mode) — 自动化非交互执行
- [核心概念](/zh/core-concepts) — 工具概述
