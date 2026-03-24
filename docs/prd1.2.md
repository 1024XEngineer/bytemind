# ForgeCLI PRD v1.2

## 文档信息

| 字段       | 内容              |
| ---------- | ----------------- |
| 文档类型   | 立项 / 评审版 PRD |
| 产品工作名 | ForgeCLI          |
| 文档版本   | v1.2              |
| 修订日期   | 2026-03-24        |
| 文档状态   | 建议进入立项评审  |

文中沿用三类标记：

- `核查结论`：已依据公开官方资料校验
- `推导补充`：原文信息不足，但基于产品方法作出的合理补充
- `待核实`：当前未获得稳定公开依据，不做强判断

## 1. 产品概述

### 1.1 产品名称

ForgeCLI，一款面向开发团队的终端原生 coding agent 。

### 1.2 产品背景

AI 编码工具正在从“编辑器里的建议与补全”升级为“可执行软件任务的 agent runtime”。当前头部产品已普遍覆盖仓库理解、文件编辑、命令执行、审批、安全边界、恢复机制以及多入口协同。

`核查结论`：Claude Code、OpenAI Codex、Gemini CLI、Qwen Code、Cursor、Warp、通义灵码、Sourcegraph Amp、OpenCode 等公开产品均已提供其中至少数项核心能力，说明赛道已进入“工程系统竞争”阶段，而不是单一模型能力竞争阶段。

### 1.3 问题定义

目标用户不是“不会写代码的人”，而是已经在终端、Git、CI、代码审查流程中工作的开发者与平台团队。当前痛点主要集中在以下四类：

1. 任务闭环割裂：需求理解、仓库检索、文件修改、命令执行、验证结果仍分散在 IDE、终端、浏览器和聊天窗口中。
2. 自动化不可控：现有工具在审批、回滚、审计、敏感路径保护上的深度不一，企业使用门槛高。
3. 环境适配不足：Windows、企业代理、内网、私有模型、受控工具接入场景下，现有产品往往存在额外配置成本。
4. 团队落地困难：很多产品适合个人提效，但不适合纳入团队规范、权限体系与审计链路。

### 1.4 产品目标

ForgeCLI 的阶段性目标不是做“又一个通用 AI IDE”，而是做一款：

- 终端原生、单二进制、低安装摩擦的 coding agent
- 可审批、可审计、可恢复的任务执行系统
- 对 Windows 与企业内网环境友好的工程工具
- 能兼容主流规则文件与 MCP 生态的可扩展 runtime

### 1.5 产品愿景

成为团队级开发流程中的“可信执行层”：让 AI 不只会回答问题，而是在受控边界内可靠地完成真实开发任务。

## 2. 市场与机会分析

### 2.1 行业背景

终端型 coding agent 已从早期实验工具演进为明确产品形态。当前市场更准确的划分方式如下：

- 终端原生 / CLI agent：Claude Code、Codex CLI、Gemini CLI、Qwen Code、Aider、OpenCode
- IDE / 云代理：Cursor、Trae、通义灵码
- Terminal workspace / agent workbench：Warp
- 平台化 / 可扩展：Amp、OpenHands、Goose

`核查结论`：这些分类并非互斥，很多产品正在向多入口融合。因此，“是否能做一个 agent”已不是机会点，真正的机会点在于产品取舍、落地场景和工程质量。

### 2.2 目标市场

本产品建议采用分层市场策略：

- 核心切入市场：中大型研发组织中的后端 / 全栈工程师、开发平台团队、DevOps / SRE 团队
- 次级市场：AI Native 独立开发者、开源维护者
- 非首发市场：零编程基础用户、以视觉设计为主的创意工具用户、纯 IDE 内工作流用户

`推导补充`：首发不建议面向“所有开发者”。如果目标用户边界过宽，产品会同时被 Claude Code、Codex、Cursor、Warp、通义灵码、Aider 等多线夹击。

### 2.3 用户痛点

| 痛点           | 现状表现                             | 业务影响                   |
| -------------- | ------------------------------------ | -------------------------- |
| 上下文切换重   | 需求、代码、命令、日志、PR 往返切换  | 单任务耗时高、认知负担大   |
| 多文件修改难控 | 复杂改动跨多个目录与工具             | 易漏改、难验收、回滚困难   |
| 自动执行风险高 | 命令执行与文件写入缺少统一策略       | 误操作成本高，团队信任不足 |
| 企业接入成本高 | 私有模型、内网工具、审计流程接入复杂 | PoC 能跑，规模化难落地     |

### 2.4 产品机会

本项目成立的前提，不是“市场上还没有 coding agent”，而是以下更窄的机会假设：

1. 市场仍存在一类机会：把“受控执行 + 审计治理 + Windows / 内网适配”做成更稳定、低摩擦、可治理的终端原生产物。
2. 一部分团队需要的不是更强自治，而是更高可信度、更低部署摩擦和更强流程兼容。
3. 终端原生工具若能兼容企业工具链与规则体系，仍有机会切入团队级生产流程。

`核查结论`：原始研究把“Windows 支持”“私有化支持”“MCP 兼容”描述为较明显空白位，表述偏强。Claude Code、Codex、Warp、通义灵码等均已公开提供 Windows、企业部署或模型接入能力。因此，差异化不能建立在“有没有”，而应建立在“做得是否足够稳定、可治理、低摩擦”。

### 2.5 竞品概览

| 产品         | 主入口 / 形态               | 已公开优势                                                             | 对本项目的启示                                    |
| ------------ | --------------------------- | ---------------------------------------------------------------------- | ------------------------------------------------- |
| Claude Code  | CLI / IDE / Desktop / Web   | 统一 runtime、subagents、hooks、审批、MCP、`CLAUDE.md`、GitHub Actions | 产品标杆，但其价值不只在模型，而在 runtime 一体化 |
| OpenAI Codex | CLI / IDE / App / Web       | 本地优先、`AGENTS.md`、MCP、审批、安全、Subagents、Windows             | 本地 CLI 结构成熟，原研究低估了其多 agent 能力    |
| Gemini CLI   | CLI / Headless              | 开源、Checkpointing、Trusted Folders、`GEMINI.md`、Headless、MCP       | 自动化与恢复机制值得直接参考                      |
| Qwen Code    | CLI / IDE / Actions         | 中文文档、子智能体、技能、沙箱、审批、GitHub Actions                   | 对中文开发者与多入口整合友好                      |
| Cursor       | IDE / Cloud Agent           | 背景 Agent、代码库检索、云端任务执行                                   | 证明“代码库理解”与“长任务后台化”很关键            |
| Warp         | Terminal Workspace / Agents | Full Terminal Use、Agents、MCP、企业模型接入                           | 终端工作台方向成立，但产品形态较重                |
| 通义灵码     | IDE / Lingma IDE / Agent    | 智能体模式、终端命令自动执行、MCP、企业专属版 / VPC                    | 中国企业市场竞争强，不能低估其私有化能力          |
| Aider        | CLI                         | Repo map、Git-centric、轻量高频可用                                    | 轻量与顺手本身就是竞争力                          |
| OpenCode     | CLI / TUI                   | agents、permissions、sessions、models、终端工作流                      | 开源终端 agent 正在补齐完整能力带                 |

`核查结论`：OpenAI Codex 官方文档已明确提供 Subagents、`AGENTS.md`、MCP、审批与 Windows 文档，原研究中“多 agent 未见官方明确”的判断已过时。

`核查结论`：规则文件并不存在单一统一标准。Claude Code 使用 `CLAUDE.md`，Codex / Amp 强调 `AGENTS.md`，Gemini CLI 使用 `GEMINI.md`。因此本项目应采用兼容策略，而非假设行业已有唯一事实标准。

### 2.6 差异化方向

ForgeCLI 应聚焦以下四个方向，而不是泛化为“通用全家桶”：

1. 可信执行：审批、回滚、审计、路径沙箱、敏感文件保护必须做成产品主价值。
2. 企业友好：优先支持私有模型或模型网关、企业代理、内网工具接入、统一配置、策略审计与组织级分发。
3. 终端优先但不过度自治：先做单 Agent 闭环，把复杂多智能体排到后续。

## 3. 用户与场景

### 3.1 目标用户画像

| 用户角色                   | 核心诉求                           | 是否首发目标 |
| -------------------------- | ---------------------------------- | ------------ |
| 后端 / 全栈工程师          | 多文件开发、调试、测试修复效率提升 | 是           |
| 开发平台 / 效率工程师      | 私有工具接入、规则治理、团队推广   | 是           |
| DevOps / SRE               | 受控执行脚本、排障自动化           | 是           |
| Tech Lead / Staff Engineer | 仓库理解、重构规划、评审辅助       | 是           |
| AI Native 独立开发者       | 单人推进端到端任务                 | 次优先       |
| 零基础用户                 | 让 AI 代替编程                     | 否           |

`推导补充`：从实际高频使用看，首发最主要的日常操作者仍以后端 / 全栈 / Tech Lead 为主；平台 / 效率工程师更多是组织内推广者和治理角色。这一点在后续商业化和增长设计中应单独考虑。

### 3.2 核心使用场景

1. 新仓库上手：解释目录结构、入口文件、依赖关系、构建测试方式。
2. Bug 修复：读取报错 / 测试失败，定位改动点，修改文件并验证。
3. 小到中等规模功能开发：围绕单需求跨多个文件改动并补齐测试。
4. 规则化改造：依赖升级、API 迁移、lint / 格式问题清理。
5. 受控运维与自动化：在审批边界内运行脚本、收集结果、输出下一步建议。

### 3.3 高频场景

- 读取仓库并提出计划
- 搜索并定位改动文件
- 生成结构化 diff 并请求审批
- 执行测试 / 构建命令并据结果继续修正
- 恢复上一次中断会话

### 3.4 非目标用户或非优先场景

- 追求“完全无人值守”的全自动智能体
- 以浏览器自动化、设计稿联动、长链路产品协同为主的工作流
- 纯 IDE 内对话式问答
- 面向非技术用户的自然语言编程

## 4. 产品定位

### 4.1 一句话定位

面向开发团队的终端原生、可审批、可审计、支持私有化部署扩展的 coding agent runtime。

`推导补充`：相较 v1.0，“可私有化”在这里应理解为产品最终支持私有模型和私有部署扩展能力，而不是首发阶段已经完整具备全部企业部署能力。

### 4.2 核心价值主张

1. 让 AI 在终端内完成“检索 -> 编辑 -> 执行 -> 验证”的闭环。
2. 让自动化始终处于可视、可控、可回滚的边界内。
3. 让产品适配真实企业环境，而不是只适配理想化个人工作流。

### 4.3 与竞品的关键区别

| 维度       | ForgeCLI 定位                     | 不做的方向                  |
| ---------- | --------------------------------- | --------------------------- |
| 主入口     | CLI 优先，后续再扩 IDE / Web / CI | 首发即全入口覆盖            |
| 核心卖点   | 可信执行、治理、企业适配          | 最强模型、最花哨 UI         |
| 自动化策略 | 默认谨慎，逐步放权                | 默认全自动                  |
| 目标客户   | 团队级真实开发流程                | 泛消费级“人人可编程”        |
| 工程语言   | Go 作为交付和部署优势             | 把“Go 实现”当作用户价值本身 |

`核查结论`：Go-native 不是足够的产品壁垒。公开项目中已存在 Go 实现方向，如 Crush、Plandex。Go 更适合作为“单二进制、跨平台、易私有化”的工程手段，而不是定位本身。

## 5. 功能规划

### 5.1 MVP 功能

| 模块                 | 说明                                                                | 优先级 |
| -------------------- | ------------------------------------------------------------------- | ------ |
| CLI 会话系统         | 启动、任务输入、流式输出、会话编号                                  | P0     |
| 仓库读取与检索       | `list/read/glob/rg`、`.gitignore` 过滤、基础摘要                    | P0     |
| 计划与 Todo          | 让用户看到 agent 当前计划、阶段与阻塞项                             | P0     |
| 结构化文件编辑       | 读取片段、写前 hash 校验、原子写入、diff 预览                       | P0     |
| 命令执行             | 超时、退出码、输出截断、审批、日志记录                              | P0     |
| 权限与安全策略       | trusted workspace、路径沙箱、敏感文件保护、危险命令规则             | P0     |
| 会话持久化           | SQLite event log、resume、usage 记录                                | P0     |
| 模型接入层           | 统一 adapter；V0 打通 1 个主模型入口，并预留兼容 / 本地入口扩展能力 | P0     |
| 规则文件兼容         | 兼容加载 `AGENTS.md` / `CLAUDE.md` / `GEMINI.md`                    | P0     |
| Headless / JSON 输出 | 面向 CI 与脚本化                                                    | P1     |
| BM25 / 索引缓存      | 提升大仓库定位效率                                                  | P1     |
| Checkpoint / Undo    | 更强恢复能力                                                        | P1     |

`推导补充`：v1.0 里“规则文件兼容”被放在 P1，但 V0 范围又写入“基础规则文件兼容”，前后矛盾。本版统一调整为 P0。

### 5.2 核心功能模块

1. Workspace：目录访问、路径校验、规则文件扫描。
2. Search：路径过滤、关键词搜索、仓库摘要。
3. Edit：结构化写入、diff 生成、乐观并发校验。
4. Exec：安全执行、审批流、输出解析。
5. Session：会话状态、event log、resume。
6. Policy：审批策略、敏感路径、秘密脱敏、命令等级。
7. Adapter：统一模型调用与工具调用接口。
8. MCP Bridge：接入外部工具与企业连接器。

### 5.3 非本期功能

- 多智能体自动编排
- 全屏 TUI
- 浏览器自动化
- 容器级沙箱
- IDE 插件和桌面端
- 向量数据库与复杂长期记忆
- 插件市场

### 5.4 功能优先级判断

`推导补充`：对 MVP 而言，“审批 + diff + 命令验证 + 恢复”优先级高于“多 agent + TUI + 向量检索”。因为用户购买的是可信闭环，不是功能清单长度。

### 5.5 用户流程

1. 用户指定仓库与任务目标。
2. 系统加载工作区配置、规则文件与基础仓库上下文。
3. Agent 输出计划，开始检索文件与上下文。
4. Agent 生成修改建议与 diff，等待用户审批。
5. 用户批准后写入文件。
6. Agent 请求执行命令，读取验证结果。
7. 系统输出结果总结、未完成项与可恢复会话快照。

## 6. 产品方案

### 6.1 关键交互流程

流程 A：新任务执行

1. 输入：任务描述 + 仓库路径 + 审批模式。
2. 系统动作：扫描规则文件、列目录、构建轻量上下文。
3. Agent 输出：计划、待办、首批工具调用。
4. 用户动作：确认继续或手动补充上下文。

流程 B：文件修改

1. Agent 先读取文件或片段。
2. Agent 生成结构化编辑操作。
3. 系统做 hash 校验并生成 diff。
4. 用户审批后写入；写入失败则提示冲突并要求重读。

流程 C：命令执行与验证

1. Agent 说明执行目的、命令、工作目录、风险等级。
2. 用户审批后执行。
3. 系统返回退出码、stdout / stderr 摘要、超时 / 截断标记。
4. Agent 基于结果决定继续修正、终止或请求进一步授权。

### 6.2 主要输入输出

| 类型     | 内容                                                               |
| -------- | ------------------------------------------------------------------ |
| 输入     | 自然语言任务、仓库路径、模型配置、审批模式、规则文件、可选文件指针 |
| 中间产物 | 计划、todo、工具调用记录、diff、命令日志、会话摘要                 |
| 输出     | 最终变更摘要、验证结果、未完成风险、可恢复 session                 |

### 6.3 风险控制点

- 默认不直接高风险写入和执行
- 文件写入前必须经过真实读取与 hash 校验
- 命令执行默认 `direct exec`；`shell mode` 需显式开启
- 超出 trusted workspace 的路径默认拒绝；只有在显式授权或策略放行时才可访问
- `.env`、证书、密钥等敏感文件默认受保护
- 所有关键动作写入审计日志

`推导补充`：v1.0 中“超出 trusted workspace 的路径一律拒绝”表述过绝对。对 CLI 安全默认值来说可以默认拒绝，但对企业连接器、外部工具或显式授权场景，应保留可控放行能力。

### 6.4 异常场景处理

| 异常                      | 处理策略                             |
| ------------------------- | ------------------------------------ |
| 文件被外部修改            | 拒绝写入，提示重读并重新生成 diff    |
| 命令超时                  | 返回超时状态并建议缩小范围或提升超时 |
| 路径越界 / 软链接逃逸     | 直接阻断并记录审计事件               |
| 模型引用不存在文件 / 命令 | 返回硬错误，不做猜测性修复           |
| 输出过长                  | 摘要化返回，并提供定位原始日志入口   |
| 网络不可用 / 模型失败     | 支持重试、降级或切换 provider        |

## 7. 商业与增长假设

### 7.1 可能的商业模式

1. 团队版订阅：统一策略、共享配置、会话审计、组织级指标。
2. 企业版部署：私有模型 / VPC / 内网连接器 / SSO / RBAC / 审计存档。
3. 服务与实施：模型网关、连接器开发、内网部署支持。

`推导补充`：不建议一开始把商业化建立在个人订阅上。此赛道成熟产品已较多，ForgeCLI 更适合用“团队治理 + 企业适配”建立付费理由。

### 7.2 初期增长路径

1. 先做内部试点或 3-5 家设计合作客户 PoC。
2. 通过单二进制安装降低试用门槛。
3. 以真实 benchmark 任务展示闭环效果，而不是以 demo 展示“看起来很聪明”。
4. 从平台团队切入，再向后端 / 全栈团队扩散。

### 7.3 验证指标

- 任务闭环成功率：完成修改并通过验证命令的任务占比
- 用户接受 diff 比例：生成变更被用户批准的比例
- 首次有效改动时间：从发起任务到出现首个可接受 diff 的中位时长
- 人工接管率：任务中途需要完全转人工的比例
- 回滚率：被接受的改动后续被撤销的比例
- 每成功任务成本：模型调用与工具执行成本

### 7.4 北极星指标及辅助指标

- 北极星指标：周成功闭环任务数
- 辅助指标：
- 周活跃开发者数
- 审批通过率
- 会话恢复使用率
- Headless / CI 调用次数
- 敏感操作拦截次数

## 8. 技术与实施约束

### 8.1 核心技术前提

| 维度     | 约束 / 原则                                                                        |
| -------- | ---------------------------------------------------------------------------------- |
| Runtime  | Go 单二进制优先，便于分发与企业部署                                                |
| 存储     | 本地 SQLite event log，支持查询与恢复                                              |
| 检索     | 先做路径过滤 + 关键词 / BM25；向量检索后置                                         |
| 执行     | `os/exec` 为基线，PTY 后置                                                         |
| 模型接入 | provider-agnostic adapter，支持主 provider 并兼容 OpenAI-compatible / 本地模型扩展 |
| 扩展     | 先兼容 MCP，后评估自有插件机制                                                     |

### 8.2 平台限制

- Windows / macOS / Linux 的 shell quoting、路径、编码、CRLF 差异必须前置处理
- 企业代理、无公网、内网仓库环境需要兼容
- 不应假设目标环境天然具备 Unix 工具链

### 8.3 实现难点

1. 大仓库上下文裁剪与命中率
2. 文件编辑的可靠落地与冲突处理
3. 命令执行的安全边界与审批体验平衡
4. 会话恢复与长任务可视化
5. 多模型接入的一致性

### 8.4 安全与合规注意事项

- 默认审计所有写入与执行动作
- 敏感信息脱敏与环境变量白名单
- 明确 trusted workspace 概念
- 不把私有仓库内容上传到未授权模型端点
- 企业版需要支持组织级审批策略与日志留存

## 9. 风险分析

| 风险类型       | 风险表现                                             | 缓解建议                                                           |
| -------------- | ---------------------------------------------------- | ------------------------------------------------------------------ |
| 产品风险       | 最终范围写得过宽，导致早期版本像“想一次做完所有能力” | 冻结最终方向，但把 MVP / V1 / V2 边界写清，不把首发做成全家桶      |
| 技术风险       | 编辑失败、误改文件、错误命令执行                     | 结构化 edit、hash 校验、审批、回滚、沙箱                           |
| 用户接受度风险 | 自动化过强导致不信任，过弱则价值不明显               | 默认谨慎，逐步放权，强化 plan / diff / 审计可见性                  |
| 市场风险       | 头部产品迭代太快，功能差距被迅速抹平                 | 竞争焦点转向治理、部署、环境适配和组织落地                         |
| 执行风险       | MVP 范围过大，6-8 周难以闭环                         | 严格砍掉多 agent、全屏 TUI、浏览器、向量库、插件市场等首发非必要项 |
| 采购风险       | 企业客户没有足够付费意愿                             | 先验证平台团队场景和审计 / 私有化需求是否形成预算                  |

`推导补充`：v1.0 中“只做企业友好的终端受控执行，不做全家桶”容易被读成最终产品范围收缩；本版改为“最终方向不变，但首发不做全家桶”，以消除与 V2 平台化增强版之间的矛盾。

## 10. 版本规划

### 10.1 V0 / MVP

目标：完成单用户、单工作区的可信闭环。

范围：

- CLI 会话
- 仓库读取 / 搜索
- 结构化编辑与 diff
- 命令审批与执行
- SQLite 持久化
- trusted workspace
- 基础规则文件兼容
- 基础模型接入层

验收重点：

- 能稳定完成内部 benchmark 中的大多数中小任务
- 不出现未审批的高风险写入或执行
- 会话可恢复、日志可追溯

`推导补充`：若从当前仓库基础起步，6-8 周偏紧；在不改变范围的前提下，本版改为 8-12 周更符合工程现实。

### 10.2 V1（团队试点版）

目标：进入 3-5 个真实团队试点。

范围：

- Headless / JSON 输出
- BM25 / 索引缓存
- Checkpoint / Undo
- MCP client
- 统一策略模板
- 简单组织级配置与审计导出

验收重点：

- 可纳入团队代码流程与安全规范
- 平台团队可配置基础审批策略
- 试点用户能形成周活跃使用

### 10.3 V2（平台化增强版）

目标：从 CLI 内核扩展为团队级 agent 平台。

范围：

- IDE 入口
- 远端 / 隔离执行
- 语义检索或混合检索
- 多智能体编排
- 企业连接器包

验收重点：

- 组织级采用率提升
- 与内网系统、CI、代码托管平台形成稳定集成
- 商业化能力可独立成立

## 11. 结论与建议

### 11.1 是否值得做

值得做，但前提不是收缩最终产品范围，而是校准事实判断并收敛阶段目标。

### 11.2 为什么

1. 赛道已被验证，需求真实存在。
2. 通用型产品竞争激烈，但“企业友好的终端受控执行层”仍有切口。
3. 原研究提出的 Windows、私有化、审计、恢复、MCP 兼容方向是有价值的，但必须从“差异化重点”而不是“市场空白”来理解。

### 11.3 建议优先验证什么

1. 平台团队和后端团队是否真的把“审批 / 审计 / 回滚”视为购买理由。
2. Windows / 内网 / 私有模型适配是否能显著降低试点阻力。
3. 单 Agent 闭环在真实仓库任务上的成功率是否足够高。

### 11.4 建议先后置什么

- 多智能体自动编排
- 全屏 TUI
- 浏览器自动化
- 插件市场
- 向量数据库
- 桌面端和 IDE 插件

`推导补充`：这里应理解为“先后置到 V1 / V2 或后续阶段”，而不是从产品最终范围中永久删除。

## 12. 附录

### 12.1 重点审校结论

1. OpenAI Codex 已公开支持 Subagents、`AGENTS.md`、MCP、审批与 Windows，原研究相关判断需更新。
2. 规则文件尚未形成唯一标准，不应把 `AGENTS.md` / `CLAUDE.md` 写成统一事实标准。
3. Windows 与私有化不是空白市场，而是已有竞争、但体验和治理深度仍可分化的市场。
4. 通义灵码不应被简单归类为“IDE 插件”；其官方文档已公开智能体模式、终端命令自动执行、MCP 与企业专属版能力。
5. Warp 更适合归类为 terminal workspace / agent workbench，而不是传统 IDE。
6. Go 实现是交付优势，不是产品定位本身；公开市场已有 Go 方向项目。

### 12.2 待核实事项

- Trae 在公开官方资料中的“多智能体并行”边界与能力范围
- 某些竞品企业版的具体权限模型、价格与部署深度
- 各竞品在中国内网 / 代理环境下的真实交付体验

### 12.3 核查来源

- Claude Code overview: [https://code.claude.com/docs/en/overview](https://code.claude.com/docs/en/overview)
- Claude Code sub-agents: [https://code.claude.com/docs/en/sub-agents](https://code.claude.com/docs/en/sub-agents)
- Claude Code hooks: [https://code.claude.com/docs/en/hooks](https://code.claude.com/docs/en/hooks)
- Claude Code memory: [https://code.claude.com/docs/en/memory](https://code.claude.com/docs/en/memory)
- Claude Code GitHub Actions: [https://code.claude.com/docs/en/github-actions](https://code.claude.com/docs/en/github-actions)
- Claude Code MCP: [https://code.claude.com/docs/en/mcp](https://code.claude.com/docs/en/mcp)
- OpenAI Codex CLI features: [https://developers.openai.com/codex/cli/features](https://developers.openai.com/codex/cli/features)
- OpenAI Codex AGENTS.md: [https://developers.openai.com/codex/guides/agents-md](https://developers.openai.com/codex/guides/agents-md)
- OpenAI Codex Subagents: [https://developers.openai.com/codex/subagents](https://developers.openai.com/codex/subagents)
- OpenAI Codex approvals & security: [https://developers.openai.com/codex/agent-approvals-security](https://developers.openai.com/codex/agent-approvals-security)
- OpenAI Codex Windows: [https://developers.openai.com/codex/windows](https://developers.openai.com/codex/windows)
- Gemini CLI repo: [https://github.com/google-gemini/gemini-cli](https://github.com/google-gemini/gemini-cli)
- Gemini CLI docs: [https://google-gemini.github.io/gemini-cli/](https://google-gemini.github.io/gemini-cli/)
- Qwen Code overview: [https://qwenlm.github.io/qwen-code-docs/zh/users/overview/](https://qwenlm.github.io/qwen-code-docs/zh/users/overview/)
- Qwen Code approval mode: [https://qwenlm.github.io/qwen-code-docs/zh/users/features/approval-mode/](https://qwenlm.github.io/qwen-code-docs/zh/users/features/approval-mode/)
- Qwen Code sandbox: [https://qwenlm.github.io/qwen-code-docs/en/users/features/sandbox/](https://qwenlm.github.io/qwen-code-docs/en/users/features/sandbox/)
- Qwen Code GitHub Action: [https://qwenlm.github.io/qwen-code-docs/en/users/integration-github-action/](https://qwenlm.github.io/qwen-code-docs/en/users/integration-github-action/)
- Cursor background agents: [https://docs.cursor.com/en/background-agents](https://docs.cursor.com/en/background-agents)
- Cursor codebase indexing: [https://docs.cursor.com/context/codebase-indexing](https://docs.cursor.com/context/codebase-indexing)
- Warp agents overview: [https://docs.warp.dev/agents/overview](https://docs.warp.dev/agents/overview)
- Warp local agents: [https://docs.warp.dev/agent-platform/local-agents/overview](https://docs.warp.dev/agent-platform/local-agents/overview)
- Warp full terminal use: [https://docs.warp.dev/agents/full-terminal-use](https://docs.warp.dev/agents/full-terminal-use)
- Warp MCP: [https://docs.warp.dev/knowledge-and-collaboration/mcp](https://docs.warp.dev/knowledge-and-collaboration/mcp)
- 通义灵码产品总览: [https://help.aliyun.com/zh/lingma/](https://help.aliyun.com/zh/lingma/)
- 通义灵码智能体: [https://help.aliyun.com/zh/lingma/user-guide/agent](https://help.aliyun.com/zh/lingma/user-guide/agent)
- 通义灵码自动执行终端命令: [https://help.aliyun.com/zh/lingma/user-guide/auto-execute-terminal-commands](https://help.aliyun.com/zh/lingma/user-guide/auto-execute-terminal-commands)
- Sourcegraph Amp Manual: [https://ampcode.com/manual](https://ampcode.com/manual)
- OpenCode docs: [https://opencode.ai/docs/](https://opencode.ai/docs/)
- OpenCode agents: [https://opencode.ai/docs/agents/](https://opencode.ai/docs/agents/)
- OpenCode permissions: [https://opencode.ai/docs/permissions/](https://opencode.ai/docs/permissions/)
- OpenCode models: [https://opencode.ai/docs/models/](https://opencode.ai/docs/models/)
- Aider docs: [https://aider.chat/docs/](https://aider.chat/docs/)
- Aider repomap: [https://aider.chat/docs/repomap.html](https://aider.chat/docs/repomap.html)
- Crush repository: [https://github.com/charmbracelet/crush](https://github.com/charmbracelet/crush)
- Plandex repository: [https://github.com/plandex-ai/plandex](https://github.com/plandex-ai/plandex)
