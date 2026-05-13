# core 模块设计（共享内核）

## 1. 模块定位
`core` 提供跨模块稳定共享的领域类型与通用错误契约，统一所有模块的基础语义。

## 2. 职责边界
做什么：
- 定义跨模块复用类型：`SessionID`、`TaskID`、`EventID`、`TraceID`、`Role`、`SessionMode`、`TaskStatus`、`Decision`、`RiskLevel`。
- 定义多模态消息载荷：`MessagePart`（text/image/file/audio）。
- 定义统一事件元信息：`EventMeta`。
- 提供通用错误语义契约：`SemanticError`。

不做什么：
- 不定义模块专属业务请求/响应结构。
- 不定义业务流程编排接口。
- 不依赖任何业务模块实现。

## 3. 对外接口（契约）
- `EventMeta`：所有流式事件共享元字段（`event_id/trace_id/session_id/task_id/timestamp`）。
- `SemanticError`：错误可判定、可测试的最小接口（`Code()`、`Retryable()`）。

## 4. 架构对齐点
- `SessionMode` 在 `core` 定义，避免 `session/policy/app` 重复声明。
- `TaskStatus` 统一定义在 `core`，但迁移权仅属于 `runtime`。
- 任何被两个及以上模块复用的基础类型必须下沉到 `core`。

## 5. 测试策略
- 类型稳定性检查：常量值变更需要显式评审。
- 错误语义检查：`SemanticError` 的 `Code/Retryable` 在上层可稳定断言。
