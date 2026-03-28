# MVP 测试计划与当前结果

## 1. 文档用途

- 说明当前主 MVP 的测试范围、用户故事、验收标准和测试分层
- 记录当前已经落地到什么程度、有哪些结果、还差哪些收口动作

## 2. 范围

当前 MVP 关注点：

- 从 CLI 启动 `chat` 和 `run`
- 正确加载 config、provider、session store 和 runner
- 跑通 `prompt -> tool loop -> answer` 主流程
- 约束工作区边界和 shell 安全边界
- 在常见失败场景下给出可恢复反馈

## 3. 当前总体结论

基于本轮自动化测试补强和手工验收，当前 MVP 已形成基本工程闭环：

- 用户故事已梳理
- 自动化测试已覆盖主要主路径和关键异常路径
- 手工验收已发现并推动修复真实体验问题
- CI workflow 已加入仓库配置

## 4. 状态定义

| 状态 | 含义 |
| --- | --- |
| 已完成 | 已有代码、测试或文档支持，且本轮已验证 |
| 部分完成 | 已覆盖主要路径，但仍有边角场景或收口动作未完成 |
| 待完成 | 已识别需要做，但本轮还未落地 |
| 不处理 | 当前判断不属于通用问题，暂不进入修改范围 |

## 5. 用户故事与当前状态

| ID | 用户故事 | 当前状态 | 说明 |
| --- | --- | --- | --- |
| US-1 | 作为用户，我希望启动 `bytemind chat` 并进入一段会话。 | 已完成 | CLI 启动、slash command、session 展示和基础交互已覆盖。 |
| US-2 | 作为用户，我希望执行 `bytemind run -prompt "..."` 完成一次单次任务。 | 已完成 | 单次任务入口已覆盖，手工执行也验证通过。 |
| US-3 | 作为用户，我希望系统能够保存、列出和恢复 session。 | 已完成 | 持久化、列出、恢复规则已覆盖，基础恢复体验已完成验证。 |
| US-4 | 作为用户，我希望工具执行始终处于安全边界内，并遵守审批策略。 | 已完成 | shell 审批、危险命令阻断、错误反馈、展示文案已覆盖并做过修复。 |
| US-5 | 作为用户，我希望在配置错误、预算耗尽或助手循环时，系统给出清晰且可恢复的反馈。 | 已完成 | 预算耗尽、循环停止、工具失败封装、配置错误主要路径都已覆盖。 |

## 6. 用户故事、验收标准与证据

| 用户故事 | 验收标准 | 当前状态 | 证据 |
| --- | --- | --- | --- |
| US-1 | 启动 `chat` 时打印 session id、workspace 和帮助提示。 | 已完成 | `cmd/bytemind/main_test.go` 已覆盖启动 banner。 |
| US-1 | `/help`、`/session`、`/sessions`、`/resume`、`/new`、`/quit` 行为正确。 | 已完成 | CLI 命令相关测试已补强，手工也验证了 `/sessions` 等命令。 |
| US-1 | 未知 slash command 不会导致崩溃，而是给出建议。 | 已完成 | 已新增未知命令建议测试。 |
| US-2 | `run` 能接受 `-prompt` 或命令尾部 prompt 文本，并返回最终回答或暂停总结。 | 已完成 | `runOneShot` 入口测试和手工执行已覆盖。 |
| US-2 | 缺少 prompt 时给出清晰错误。 | 已完成 | 已新增缺失 prompt 测试。 |
| US-2 | CLI bootstrap 正确加载 config、session store、provider client 和 runner 参数。 | 部分完成 | 主路径和关键错误路径已覆盖；如果后续继续扩展 CLI 功能，可再补更多装配边界测试。 |
| US-3 | prompt 执行后 session 会被保存，并按最近更新时间倒序列出。 | 已完成 | `internal/session/store_test.go` 已覆盖；`/sessions` 手工验证通过。 |
| US-3 | 同一 workspace 内可通过唯一前缀恢复 session。 | 已完成 | 已有成功恢复测试。 |
| US-3 | 跨 workspace 恢复会被拒绝，并给出清晰错误。 | 已完成 | 已有跨 workspace 拒绝测试。 |
| US-4 | 在 `on-request` 策略下，只读 shell 命令不触发审批。 | 已完成 | `internal/tools/run_shell_test.go` 已覆盖。 |
| US-4 | 风险 shell 命令在 `on-request` 和 `always` 策略下触发审批。 | 已完成 | `internal/tools/run_shell_test.go` 已覆盖。 |
| US-4 | 危险 shell 命令无论何种审批策略都必须被阻断。 | 已完成 | `internal/tools/run_shell_test.go` 已覆盖。 |
| US-4 | 工具执行结果会被写回 session history。 | 已完成 | `internal/agent/runner_test.go` 已覆盖最小 tool loop。 |
| US-5 | 达到迭代预算时返回暂停总结，而不是硬失败。 | 已完成 | `internal/agent/runner_test.go` 已覆盖。 |
| US-5 | 重复相同 tool sequence 时返回暂停总结。 | 已完成 | `internal/agent/runner_test.go` 已覆盖。 |
| US-5 | 工具执行失败时不会直接把整个流程打崩，而是返回可继续的错误反馈。 | 已完成 | `internal/agent/runner_test.go` 已覆盖未知工具场景下，错误会被编码进 tool message，且流程可继续。 |
| US-5 | provider 配置非法或 API key 缺失时快速失败并给出清晰错误。 | 已完成 | 已覆盖非法 provider type、缺失 API key、非法 `approval_policy`、非法 `-stream`、配置文件损坏、显式坏 config 路径等场景。 |

## 7. 自动化测试落地情况

| ID | 测试项 | 类型 | 当前状态 | 当前情况 |
| --- | --- | --- | --- | --- |
| T1 | `chat` 启动成功 | 自动化 | 已完成 | 已覆盖启动 banner 和退出路径。 |
| T2 | Slash command 行为 | 自动化 | 已完成 | 已覆盖补全、恢复、新建、未知命令建议等。 |
| T3 | `run` 接受合法 prompt | 自动化 | 已完成 | 已覆盖 `-prompt` 和尾部 prompt 文本。 |
| T4 | `run` 拒绝缺失 prompt | 自动化 | 已完成 | 已补测试。 |
| T5 | Bootstrap 校验 config 和 flags | 自动化 | 已完成 | 已覆盖缺 API key、非法 `-stream`、负数 `-max-iterations`、显式坏 config 路径等。 |
| T6 | Config 加载优先级与规范化 | 自动化 | 已完成 | 已覆盖 env override、workspace config、非法 provider、非法 `approval_policy`、相对 `session_dir`、配置损坏等场景。 |
| T7 | Provider 工厂构造正确 client | 自动化 | 已完成 | `internal/provider/factory_test.go` 已新增。 |
| T8 | Session 持久化和列表 | 自动化 | 已完成 | `internal/session/store_test.go` 已覆盖，并补了 UTF-8 截断保护。 |
| T9 | 同 workspace 恢复规则 | 自动化 | 已完成 | 成功恢复与跨 workspace 拒绝均已覆盖。 |
| T10 | 预算耗尽返回暂停总结 | 自动化 | 已完成 | `internal/agent/runner_test.go` 已覆盖。 |
| T11 | 重复 tool sequence 停止 | 自动化 | 已完成 | `internal/agent/runner_test.go` 已覆盖。 |
| T12 | 最小 tool loop 主链路 | 自动化 | 已完成 | 已覆盖 `prompt -> tool -> tool result -> answer`。 |
| T13 | 未知工具错误被编码为 tool message | 自动化 | 已完成 | 已覆盖 `runner` 在遇到未注册工具名时，会把错误编码进 tool message，并继续后续流程。 |
| T14 | Shell 审批边界 | 自动化 | 已完成 | 已覆盖 quoted operator、timeout、审批拒绝文案等。 |
| T15 | 真实 `chat` 关键交互可用性 | 手工 | 已完成 | 已手工验证 `/sessions` 展示、session 恢复和基础交互。 |
| T16 | 真实 `run` 可用性 | 手工 | 已完成 | 已手工执行 `inspect this repo`，结果通过基本验收。 |
| T17 | 真实审批流体验 | 手工 | 已完成 | 已手工验证审批拒绝路径，并据此修正文案。 |

## 8. 本轮已修复的问题

本轮手工验收中发现并修复的问题：

| 问题 | 处理结果 |
| --- | --- |
| `/sessions` 中 session ID 被截断，不利于识别和恢复 | 已修复，改为完整显示 session ID |
| 审批拒绝后的工具层反馈文案不够清楚 | 已修复，改为更明确的“命令未执行，因为审批被拒绝” |
| `/sessions` 中文摘要预览出现乱码 | 已修复，字符串截断改为按 rune 处理，避免中文被截坏 |

## 9. 手工验收结果

| 手工检查项 | 当前状态 | 结果说明 |
| --- | --- | --- |
| 启动 `chat` | 已完成 | 可正常启动并进入会话。 |
| `/sessions` 展示 | 已完成 | 已修复 session ID 截断和中文预览乱码。 |
| 单次任务 `run -prompt "inspect this repo"` | 已完成 | 功能通过基本验收，输出可用。 |
| 审批拒绝路径 | 已完成 | 已验证并修正文案。 |
| session 恢复体验 | 已完成 | 已验证恢复后可继续沿用既有上下文。 |
