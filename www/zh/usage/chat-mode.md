# 聊天模式

聊天模式（`bytemind chat`）是 ByteMind 的主要交互方式，支持多轮对话、上下文持久化和动态任务调整。

```bash
bytemind chat
```

## 工作原理

启动后，ByteMind 会：

1. 读取当前目录的 `.bytemind/config.json`
2. 初始化或恢复已有会话
3. 进入交互模式，等待你的输入

你输入任务描述后，Agent 会自动调用工具（读取文件、搜索代码、执行命令等）完成任务，高风险操作前会弹出审批提示。

## 启动选项

```bash
bytemind chat                         # 使用默认配置
bytemind chat -max-iterations 64      # 提高迭代上限
bytemind chat -config ./my.json       # 使用自定义配置文件
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
| `/quit`         | 安全退出               |

## 中途中断与恢复

随时可以按 `Ctrl+C` 或输入 `/quit` 退出。会话上下文已自动保存。

下次恢复：

```bash
bytemind chat
# 启动后执行
/sessions          # 找到上次的会话 ID
/resume abc123     # 按 ID 恢复
```

## 相关页面

- [会话管理](/zh/usage/session-management)
- [工具与审批](/zh/usage/tools-and-approval)
- [Plan 模式](/zh/usage/plan-mode)
- [技能](/zh/usage/skills)
