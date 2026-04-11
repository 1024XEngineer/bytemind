# storage 模块设计

## 1. 模块定位
`storage` 是无数据库场景下的文件持久化与恢复层，提供会话与任务日志可靠读写和回放。

## 2. 职责边界
做什么：
- 写入会话事件：`~/.bytemind/sessions/<session-id>.jsonl`。
- 写入任务日志：`~/.bytemind/tasks/<task-id>.log`。
- 支持增量读取、回放恢复、幂等去重。

不做什么：
- 不做业务决策与调度。
- 不承担权限规则与工具编排。

## 3. 对外接口
- `SessionStore`：会话事件追加与增量读取。
- `TaskStore`：任务日志追加与增量读取。
- `Locker`：会话/任务级锁控制。
- `Deduplicator`：基于 `event_id` 去重。
- `Replayer`：会话/任务事件流回放。

## 4. 一致性策略
- append-only。
- 单记录原子落盘（tmp+rename 或 fsync）。
- 文件锁避免并发乱序。

## 5. 测试策略
- 原子写测试、并发写测试、去重测试、回放一致性测试。
