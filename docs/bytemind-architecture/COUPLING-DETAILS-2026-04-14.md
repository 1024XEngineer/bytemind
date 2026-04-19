# ByteMind 模块耦合明细（汇报版，2026-04-14）

## 1. 汇报一句话结论
当前不是“功能做不出来”，而是“关键职责耦合在同一层”，导致改动扩散、回归成本升高、长期迭代速度下降。

## 2. 具体耦合清单（按严重度）

| 编号 | 耦合簇 | 具体耦合模块 | 代码证据 | 影响 | 建议分界 |
| --- | --- | --- | --- | --- | --- |
| C1 | 主编排层超载 | `agent` 同时耦合 `context/policy/tools/session/history/tokenusage/skills` | `internal/agent/runner.go` 行 299、307、316、346、471、543；`internal/agent/compaction.go` 行 47；`internal/agent/prompt.go` 行 80 | 一个需求会跨多职责改动，回归范围大 | 保留 `agent` 作为编排层；将 `context/policy/runtime/storage` 抽为独立模块 |
| C2 | 权限策略分散 | `policy` 逻辑分散在 `tools/run_shell` + `tools/executor` + `tools/registry` + `agent` | `internal/tools/run_shell.go` 行 118、130、281、405；`internal/tools/executor.go` 行 117；`internal/tools/registry.go` 行 169、196；`internal/agent/runner.go` 行 307 | 策略一致性难保证，规则变更易漏改 | 新建 `internal/policy`；工具层只消费决策结果，不再内置规则 |
| C3 | 会话语义与持久化耦合 | `session` 同时承担语义状态和落盘细节（快照、原子写） | `internal/session/store.go` 行 113、351、369 | `session` 改动会牵扯文件格式与IO细节，测试边界不清 | `session` 保留语义；持久化抽到 `storage` |
| C4 | 存储能力分散 | `session/history/tokenusage` 各自维护存储路径与写入策略 | `internal/session/store.go` 行 351；`internal/history/store.go` 行 30、60；`internal/tokenusage/storage_file.go` 行 49、67 | 统一回放/审计链路难建立，可靠性策略不一致 | 以 `storage` 统一会话/任务/审计/统计读写契约 |
| C5 | UI 与业务编排耦合 | `tui` 直接改会话模式/计划、切 provider、接管审批回调 | `internal/tui/model.go` 行 371、1016、1471、2782、2802、2807、3360 | UI层改动影响业务行为，替换前端形态成本高 | UI 只做展示与交互，业务动作通过应用层服务调用 |
| C6 | 入口与装配耦合 | `cmd/bytemind` 直接承担配置、Store、Provider、Runner 装配 | `cmd/bytemind/main.go` 行 137、143、174、193、198；`cmd/bytemind/tui.go` 行 44、55 | CLI/TUI入口重复装配逻辑，扩展新入口成本高 | 新建 `internal/app` 统一装配与生命周期 |
| C7 | 计划能力跨层耦合 | `update_plan` 直接改 session 计划，agent/tui再各自解释与驱动 | `internal/tools/update_plan.go` 行 127、148；`internal/agent/runner.go` 行 511；`internal/tui/model.go` 行 1008、5283 | 计划状态机职责分散，语义漂移风险高 | 计划状态迁移归口到统一状态服务（后续 runtime/app） |
| C8 | 协议模型新旧双轨耦合 | `llm.Message` 同时维护 part-based 与 legacy 字段 | `internal/llm/types.go` 行 41、154、201、251 | 协议转换路径复杂，缺陷排查成本高 | 先保兼容，逐步清理 legacy 字段依赖 |

## 3. 给管理层的解释口径

### 3.1 为什么要动
不是为了“代码好看”，而是为了解决三个可量化问题：
1. 需求改动面越来越大（改一处牵多层）。
2. 回归风险越来越高（策略和状态机分散）。
3. 交付节奏变慢（排查链路变长）。

### 3.2 为什么不是全量重写
1. 主闭环能力已可用（不该推倒）。
2. 高风险集中在边界不清，而不是单点不可用。
3. “重构主干 + 局部重写模块”能控制风险和周期。

### 3.3 怎么控风险
1. 先建新模块契约（`core/context/policy`），再迁移调用方。
2. 每迁一层就跑对应回归（agent/tools/session/provider）。
3. 保留兼容适配期，不做一次性切换。

## 4. 与前一份盘点的对应关系
- 总体差距与分期：`docs/bytemind-architecture/GAP-ASSESSMENT-2026-04-14.md`
- 本文档：只解决“具体哪里耦合、为什么会拖慢交付、怎么拆”。
