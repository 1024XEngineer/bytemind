# Plan 模式

Plan 模式下，Agent 会先通过 `update_plan` 工具生成结构化的执行计划（含步骤列表、风险标注、验证方案），由你审阅并确认后再开始执行。这让你对大型任务或高风险变更有完整的可见性和控制权。

## 如何激活

两种方式：

| 方式 | 操作 |
| ---- | ---- |
| 斜杠命令 | 输入 `/plan` 切换；输入 `/build` 切回 |
| 快捷键 | 按 `Tab` 键在 Build ↔ Plan 之间切换 |

:::tip
简单独立的任务用 Build 模式更快。希望在执行前**看到并确认执行方案**时再切换到 Plan 模式。
:::

## 工作流程

Plan 模式分为几个阶段：

1. **Explore（探索）** — Agent 读取相关代码，理解现状
2. **Clarify（澄清）** — 如有不确定的决策点，Agent 会提问
3. **Draft（起草）** — Agent 通过 `update_plan` 生成结构化计划（步骤、风险、验证方案）
4. **Converge（收敛）** — 你审阅计划，提出修改意见，Agent 修订
5. **Approved（批准）** — 你确认计划，Agent 切换到 Build 模式开始逐步执行

## 适用场景

| 场景                     | Plan 模式的价值                |
| ------------------------ | ------------------------------ |
| 跨多个包的大规模重构     | 动任何文件之前先看到全局影响 |
| 有序依赖的功能实现       | 防止变更顺序错乱               |
| 数据库迁移或 Schema 变更 | 运行前验证迁移顺序             |
| 快速熟悉复杂新仓库       | 明确范围后再提交变更           |

## 示例会话

```text
用户：
/plan
把认证模块里的 token 校验逻辑抽到独立的 tokenval 包，更新所有调用方，给新包写测试。

Agent：
（进入 Explore 阶段，读取 internal/auth/ 下的相关文件）

我已经看过认证模块的结构。当前 token 校验逻辑分布在 auth/middleware.go
和 auth/handler.go 中，共有 4 个调用方。接下来我会起草一份执行计划。

Agent 调用 update_plan：
{
  "goal": "将 token 校验逻辑抽取到 internal/tokenval/ 包",
  "steps": [
    { "title": "创建 tokenval 包并迁移校验逻辑", "files": ["internal/tokenval/validator.go"] },
    { "title": "更新 auth 包调用方", "files": ["internal/auth/middleware.go", "internal/auth/handler.go"] },
    { "title": "更新 API 层调用方", "files": ["internal/api/router.go", "internal/api/middleware.go"] },
    { "title": "为 tokenval 编写单元测试", "files": ["internal/tokenval/validator_test.go"] },
    { "title": "运行全量测试确认无回归" }
  ],
  "risks": ["tokenval 包的接口签名若与旧代码不兼容，调用方可能编译失败"],
  "verification": ["go test ./... 全部通过", "token 过期/无效/缺失三种场景覆盖"]
}

用户：
第二步和第三步可以合并吗？auth 和 api 层一起改就行。

Agent 调用 update_plan 修订：
（将步骤 2 和 3 合并为 "更新所有调用方（auth + api）"，共 4 个步骤）

用户：
没问题，开始执行。

Agent：
（Phase 切换到 Approved，开始逐步执行每个步骤，完成后运行测试）
```

## 执行过程中的控制

Plan 模式执行期间你可以：

- **要求 Agent 修订**某个具体步骤
- **在任意步骤后暂停**检查结果
- **切回 Build 模式**（`/build` 或 `Tab`）让 Agent 自由继续
- 对长计划**提高 `max_iterations`** 防止预算耗尽

## 相关页面

- [交互模式 (Build)](/zh/usage/chat-mode) — Plan 模式运行其中的交互模式
- [核心概念](/zh/core-concepts) — Build 与 Plan 模式详解
- [工具与审批](/zh/usage/tools-and-approval) — 执行期间的审批流程
