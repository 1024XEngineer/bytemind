# tools 模块设计

## 1. 模块定位
`tools` 是统一工具执行平面，负责工具契约、参数校验、执行调度和标准事件流输出。

## 2. 职责边界
做什么：
- 注册与发现工具。
- 执行 schema 强校验。
- 处理超时、取消、重试、并发控制。
- 输出统一事件流：`start/chunk/result/error`。

不做什么：
- 不做权限决策（由 `policy` 负责）。
- 不做主闭环编排（由 `agent` 负责）。

## 3. 对外接口
- `Tool`：单工具统一执行契约。
- `ToolMetadataProvider`：暴露工具副作用与幂等元信息。
- `Registry`：工具注册、查询、枚举。
- `Validator`：参数与 schema 校验。
- `Executor`：工具执行调度与事件流封装。

## 4. 三层体系
- 原子层：ReadFile/EditFile/WriteFile/Glob/Grep/Bash。
- 组合层：TestRunner/GitWorkflow/TaskOutputReader。
- 协作层：AgentTool/MCPTool/SkillTool/TeamTool。

## 5. 测试策略
- Contract Test：schema 与事件流一致性。
- 执行测试：超时/取消/重试语义。
- 并发测试：配额限制与事件顺序。
