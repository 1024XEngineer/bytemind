# 聊天模式

默认交互模式（`bytemind`）是 ByteMind 的主要使用方式，支持多轮对话、上下文持久化和动态任务调整。`bytemind chat` 仍可作为兼容别名使用。

```bash
bytemind
```

## 工作原理

启动后，ByteMind 会：

1. 将当前目录解析为工作区
2. 读取用户目录的全局配置，并合并当前工作区可选的 `.bytemind/config.json`
3. 初始化或恢复已有会话
4. 进入交互模式，等待你的输入

:::warning 不要直接打开大文件夹
请在具体代码仓库或项目子目录中启动 ByteMind。用户主目录、磁盘根目录、Downloads、Desktop 或包含大量无关文件的大文件夹不适合作为默认工作区。
:::

你输入任务描述后，Agent 会自动调用工具（读取文件、搜索代码、执行命令等）完成任务，高风险操作前会弹出审批提示。

## 启动选项

```bash
bytemind                         # 使用默认交互模式
bytemind -max-iterations 64      # 提高迭代上限
bytemind -config ./my.json       # 使用自定义配置文件
bytemind -workspace ./my-project # 指定工作区
```

## 最佳实践

**明确目标和约束**

告诉 Agent 你期望的结果和不希望改动的范围：

```text
为 UserService 添加邮箱格式校验，只改 service 层，不修改接口和测试。
```

**先做小步验证**

对于大任务，拆成若干可验证的小步骤，每步完成后确认结果再继续：

```text
先只读取相关文件，分析现有实现，不要做任何修改。
```

**利用技能加速工作流**

激活内置技能可以显著提高特定场景下的输出质量：

```text
/bug-investigation symptom="订单创建接口偶发 500"
/review base_ref=main
/repo-onboarding
```

**切换模式应对复杂任务**

遇到需要分步推进的复杂任务时，切换到 Plan 模式：

```text
/plan
把 HTTP handler 层拆成独立的 controller 包，分阶段给我看计划。
```

## 会话命令参考

| 命令            | 说明                   |
| --------------- | ---------------------- |
| `/help`         | 查看所有可用命令       |
| `/session`      | 查看当前会话 ID 与摘要 |
| `/sessions [n]` | 列出最近 n 条会话      |
| `/resume <id>`  | 恢复指定会话           |
| `/new`          | 开启新会话             |
| `/plan`         | 切换到 Plan 模式       |
| `/build`        | 切换回 Build 模式      |
| `/commit <message>` | 暂存当前全部改动并创建本地 Git commit |
| `/undo-commit` | 回退当前会话里由 `/commit` 创建的最后一个本地 commit |
| `/quit`         | 安全退出               |

使用 `/commit` 时，可以从 Slash 命令面板选择，也可以直接输入，但需要自己填写 commit message：

```text
/commit fix(/commit): 调整 /commit 的反馈形式
```

ByteMind 会先用 `git add -A` 暂存当前工作区改动，再创建 commit，并反馈 commit hash、message 和文件数量。

`/undo-commit` 只用于回退同一会话里刚由 `/commit` 创建的上一个 commit。如果该 commit 已经进入 upstream 分支、当前在另一个会话里、或工作区已有更新改动会和回退结果混在一起，ByteMind 会阻止执行。

## 中途中断与恢复

随时可以按 `Ctrl+C` 或输入 `/quit` 退出。会话上下文已自动保存。

下次恢复：

```bash
bytemind
# 启动后执行
/sessions          # 找到上次的会话 ID
/resume abc123     # 按 ID 恢复
```

## 相关页面

- [会话管理](/zh/usage/session-management)
- [工具与审批](/zh/usage/tools-and-approval)
- [Plan 模式](/zh/usage/plan-mode)
- [技能](/zh/usage/skills)
