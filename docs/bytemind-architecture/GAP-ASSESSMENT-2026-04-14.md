# ByteMind 架构差距盘点（2026-04-14）

## 1. 盘点范围
- 目标基线：`docs/bytemind-architecture/README.md` 与各模块 `interface.go`。
- 现状代码：`cmd/bytemind` 与 `internal/*`。
- 输出目标：
1. 架构差距矩阵（目标 vs 现状）。
2. 模块“重构 / 重写”分界清单。

## 2. 结论摘要
- 结论：应采用 **“重构为主，局部重写为辅”** 的演进路线。
- 当前整体判断：
1. 主闭环能力已具备（agent + tools + provider + session + tui）。
2. 模块边界尚未按目标架构拆分，`context/runtime/policy/storage` 仍有较多“内嵌实现”。
3. 直接全量重写风险高，应以“新增目标模块 + 旧实现适配迁移”替代“推倒重来”。

## 3. 架构差距矩阵

| 目标能力（文档） | 现状落点 | 差距级别 | 证据 |
| --- | --- | --- | --- |
| `app` 仅做装配与生命周期 | 入口已做装配，但 `cmd/bytemind` 仍承载较多 bootstrap 细节 | 中 | `cmd/bytemind/main.go`、`cmd/bytemind/tui.go` |
| `core` 统一共享类型 | 无 `internal/core`，共享类型分散在 `llm/plan/session` | 高 | `internal/llm/types.go`、`internal/plan/types.go`、`internal/session/store.go` |
| `context` 独立预算/压缩/不变量 | 上下文组装与压缩集中在 `agent` | 高 | `internal/agent/runner.go`、`internal/agent/compaction.go`、`internal/agent/prompt.go` |
| `policy` 独立决策中心 | 权限策略分散在 `tools` 与 `agent` | 高 | `internal/tools/run_shell.go`、`internal/tools/executor.go`、`internal/agent/runner.go` |
| `runtime` 任务状态机/子代理 | 当前为回合循环，无独立任务状态机与子代理调度 | 高 | `internal/agent/runner.go` |
| `storage` 统一会话/任务/审计 | 已有会话与历史存储，但无统一 `storage` 抽象和审计链路 | 高 | `internal/session/store.go`、`internal/history/store.go` |
| `extensions`（MCP/Skills）统一桥接 | 当前仅 `skills` 子集，未见 MCP/插件桥接层 | 中-高 | `internal/skills/manager.go` |
| `provider` 路由/注册/降级 | 当前工厂可用，未形成 registry/router/health 体系 | 中 | `internal/provider/factory.go` |
| 审计最小闭环（permission/tool/task） | 未见独立审计事件流与回放 | 高 | `internal/*` 未见 `audit` 流实现 |
| Prompt 组装顺序基线 | 已对齐：default -> mode -> runtime block -> active skill -> AGENTS.md | 低（已对齐） | `internal/agent/prompt.go` |

## 4. 模块重构 / 重写分界清单

### 4.1 建议“重构”的模块（保留主体，抽边界）

| 模块 | 建议 | 分界说明 |
| --- | --- | --- |
| `internal/agent` | 重构（优先级 P0） | 保留主闭环；抽离 `context`、`policy`、`runtime` 访问接口，收敛为编排层。 |
| `internal/tools` | 重构（P0） | 保留工具契约/执行器；权限判断迁移到独立 `policy`，工具只消费决策结果。 |
| `internal/session` | 重构（P1） | 保留会话语义模型；持久化细节下沉到 `storage` 适配层。 |
| `internal/provider` + `internal/llm` | 重构（P1） | 保留 provider 适配实现；补 registry/router/health 抽象。 |
| `cmd/bytemind` | 重构（P1） | 将 bootstrap 迁移到 `internal/app`，入口仅保留参数与启动。 |
| `internal/tui` | 重构（P2） | 按“状态管理/渲染/命令处理/启动引导”拆分，避免单文件持续膨胀。 |
| `internal/skills` | 重构（P2） | 作为 `extensions` 首个实现接入，不改技能语义，补统一扩展接口。 |
| `internal/tokenusage` | 重构（P2） | 保留统计能力，接入统一存储与事件元信息。 |

### 4.2 建议“重写（新建）”的模块（绿色字段，增量接管）

| 模块 | 建议 | 分界说明 |
| --- | --- | --- |
| `internal/core` | 重写（新建，P0） | 抽取跨模块类型与错误契约；旧类型通过适配层逐步迁移。 |
| `internal/context` | 重写（新建，P0） | 从 `agent` 抽出预算/压缩/不变量，先以接口方式被 `agent` 调用。 |
| `internal/policy` | 重写（新建，P0） | 固化 allow/deny/ask 与优先级链路；先覆盖 `run_shell` 与写工具。 |
| `internal/storage` | 重写（新建，P1） | 统一 session/task/audit 读写与回放，旧 store 先桥接。 |
| `internal/runtime` | 重写（新建，P1） | 建任务状态机与任务日志流；先接管长任务与子代理能力。 |
| `internal/extensions` | 重写（新建，P2） | 建扩展管理器与工具桥接；先纳入 skills，再接 MCP。 |
| `internal/app` | 重写（新建，P1） | 收拢依赖装配与生命周期，替换 `cmd` 中散落 bootstrap。 |

## 5. 优先级路线（建议）

### Phase 1（2-3 周）
1. 新建 `core/context/policy` 空壳接口与最小实现。
2. `agent` 改为依赖接口，不直接依赖具体实现细节。
3. 保持行为不变，先跑通回归测试。

### Phase 2（3-4 周）
1. 新建 `storage/app/runtime`，逐步接管原 `session/history/bootstrap` 路径。
2. 补审计事件最小闭环（permission/tool/task）。
3. 为 `runtime` 增加状态机与日志读取契约测试。

### Phase 3（2-3 周）
1. 新建 `extensions`，将 `skills` 接成统一扩展入口。
2. 预留 MCP 接口与健康检查。
3. 完成模块边界回归与文档收敛。

## 6. 风险与控制
- 风险：边拆边改导致行为漂移。  
  控制：先接口化，再替换实现；保持金丝雀测试。
- 风险：`tui/model.go` 体量过大导致改动连锁。  
  控制：先拆文件不改行为，后做结构优化。
- 风险：`policy` 从工具层抽离后权限行为变化。  
  控制：保留兼容模式，先灰度到 `run_shell` + 写工具。

## 7. 当前验证结果（本次盘点）
- 已执行：`go test ./...`（使用仓库内 `GOCACHE/GOTMPDIR`）。
- 结果：除 `cmd/bytemind` 3 个用例因缺少 API key 失败外，其余包通过。
- 失败用例：
1. `TestBootstrapCreatesSessionInWorkspace`
2. `TestRunOneShotAcceptsTrailingPromptText`
3. `TestRunOneShotCompletesToolLoopSmoke`
