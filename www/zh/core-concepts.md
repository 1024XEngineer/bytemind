# 核心概念

理解这几个核心概念，可以帮助你更高效地使用 ByteMind。

## Agent 模式

ByteMind 支持两种工作模式，在会话中可随时切换：

### Build 模式（默认）

Build 模式下，Agent 收到任务后会直接读取文件、搜索代码、写入变更并执行验证。适合大多数日常编程任务：

- 修复 Bug
- 添加新功能
- 重构代码
- 更新文档

```bash
bytemind chat          # 默认即 Build 模式
```

### Plan 模式

Plan 模式下，Agent 会先制定逐步执行计划，每个步骤由你审阅后再推进。适合复杂、多步骤的任务：

- 跨多个模块的大规模重构
- 有明确阶段依赖的功能实现
- 需要分阶段验收的迁移任务

在会话中用斜杠命令切换：

```text
/plan    切换到 Plan 模式
/build   切换回 Build 模式
```

## 会话

每次运行 `bytemind chat` 都会创建或恢复一个**会话**。会话会自动持久化完整的对话上下文。

- 会话存储在 `.bytemind/` 目录中
- 中断后重新启动，上下文不会丢失
- 支持多个并行会话，按 ID 切换

常用会话命令：

| 命令            | 说明                         |
| --------------- | ---------------------------- |
| `/session`      | 查看当前会话 ID 与摘要       |
| `/sessions [n]` | 列出最近 n 条会话（默认 10） |
| `/resume <id>`  | 恢复指定 ID 的会话           |
| `/new`          | 在当前工作区开启新会话       |

## 工具

工具是 Agent 执行具体操作的能力单元。ByteMind 内置以下工具：

| 工具              | 作用                      |
| ----------------- | ------------------------- |
| `list_files`      | 列出目录结构              |
| `read_file`       | 读取文件内容              |
| `search_text`     | 全文搜索（支持正则）      |
| `write_file`      | 写入或创建文件            |
| `replace_in_file` | 在文件中替换指定内容      |
| `apply_patch`     | 应用 unified diff 补丁    |
| `run_shell`       | 执行 Shell 命令           |
| `update_plan`     | 更新任务计划（Plan 模式） |
| `web_fetch`       | 抓取网页内容              |
| `web_search`      | 联网搜索                  |

高风险工具（`write_file`、`replace_in_file`、`apply_patch`、`run_shell`）在执行前会触发审批流程。

## 审批策略

审批策略决定 Agent 在执行高风险操作时如何处理：

- **`on-request`（默认）**：每次高风险操作前等待你的明确确认
- **Away 模式**：无人值守场景下，根据 `away_policy` 自动拒绝或终止

详见[工具与审批](/zh/usage/tools-and-approval)。

## 迭代预算

`max_iterations` 限制 Agent 在单次任务中可执行的**工具调用轮次**上限，防止无限循环消耗 token：

- 默认值：`32`
- 到达上限后 Agent 输出阶段性总结并停止，不直接报错
- 对于复杂任务，可在配置或命令行中调高：

```bash
bytemind chat -max-iterations 64
```

## 技能

技能是可激活的**专项工作指引**，通过斜杠命令注入额外的系统提示词，引导 Agent 按特定流程完成任务。

ByteMind 内置技能：

| 技能              | 命令                 | 适用场景       |
| ----------------- | -------------------- | -------------- |
| Bug Investigation | `/bug-investigation` | 结构化排查 Bug |
| Code Review       | `/review`            | 代码审查       |
| GitHub PR         | `/github-pr`         | PR diff 分析   |
| Repo Onboarding   | `/repo-onboarding`   | 快速熟悉新仓库 |
| Write RFC         | `/write-rfc`         | 撰写技术提案   |

技能还支持项目级和用户级自定义，详见[技能](/zh/usage/skills)。

## 上下文预算

ByteMind 会追踪每次会话消耗的 token，在接近模型上下文窗口限制时自动压缩或提示：

- `warning_ratio`（默认 0.85）：达到 85% 时输出警告
- `critical_ratio`（默认 0.95）：达到 95% 时触发压缩或中止

这些阈值可在配置文件的 `context_budget` 字段中调整。
