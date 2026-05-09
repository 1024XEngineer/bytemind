# ByteMind 用户故事

四个场景覆盖 ByteMind 全部功能点。每个故事末尾标注了该故事覆盖的功能模块。

---

## 故事一：设计阶段 — "为新模块做技术方案"

> **角色**：后端工程师小张，刚接手一个 Go 微服务项目，需要为"消息推送模块"输出一份技术方案。

### 1. 安装与上手

小张在 Windows 上用 PowerShell 一键安装 ByteMind：

```powershell
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

安装完成后进入项目目录，第一次启动看到了 **启动引导页**，提示他复制示例配置并填入 API Key。他编辑 `.bytemind/config.json`，配置了 OpenAI-compatible provider，顺手加了 Anthropic 和 Gemini 作为备用 provider，并开启 `auto_detect_type` 让系统自动识别 provider 类型。他通过环境变量 `BYTEMIND_HOME` 指定了配置目录。

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4-mini",
    "api_key": "sk-xxx"
  },
  "provider_runtime": {
    "providers": [
      { "id": "anthropic", "type": "anthropic", "model": "claude-sonnet-4-20250514", "api_key": "sk-ant-xxx" },
      { "id": "gemini", "type": "gemini", "model": "gemini-2.5-pro", "api_key": "xxx" }
    ]
  },
  "stream": true,
  "max_iterations": 64
}
```

### 2. 启动 Plan 模式，探索代码库

小张启动 TUI 交互模式：

```bash
bytemind chat
```

进入 TUI 后，他先用 `/new` 新建会话，然后通过 `/models` 命令切换到 Claude Sonnet 4 模型——ByteMind 自动从 Anthropic provider 路由过去。他按 `Tab` 键打开子智能体面板，了解到有 `explorer`、`general`、`review` 三个内置子智能体。

他输入 `@explorer 帮我梳理项目中与消息推送相关的所有代码文件和模块依赖`，ByteMind 自动补全子智能体名称，派发 `explorer` 子智能体去搜索代码库。子智能体通过 `list_files`、`search_text`、`read_file` 等工具遍历项目结构，返回了一份完整的模块依赖报告。

接着小张用 `web_search` 调研业界消息推送的最佳实践，用 `web_fetch` 抓取了几篇技术文章的详细内容。

### 3. Plan 模式：从探索到方案

小张输入 `/plan` 进入 Plan 模式，描述需求：

> "我需要为这个项目设计一个消息推送模块，支持 APNs 和 FCM 双通道，请帮我做技术方案。"

ByteMind 进入 Plan 模式的阶段流转：
- **explore**：通过 `search_text` 定位现有通知相关代码，`read_file` 了解当前架构风格
- **clarify**：追问了几个关键问题（推送优先级策略、失败重试机制、是否需要本地消息队列）
- **draft**：生成方案初稿，包含架构图描述、数据流设计、接口定义
- **converge_ready**：方案待小张确认

整个过程展示在 **Plan 面板**中，小张可以看到每个步骤的状态（pending/in_progress/completed/blocked）和风险等级标识（low/medium/high）。TUI 界面同时展示**上下文窗口使用量**，当接近 85% 告警线时触发了 warning 提示。

### 4. 加载 Skill，引入 RFC 模板

小张激活 `write-rfc` Skill：

```
/skill write-rfc
```

Skill 加载后，系统提示词被替换为 RFC 写作模板。他继续对话，ByteMind 按照 RFC 格式输出完整的技术方案文档，小张确认后方案进入 `approved_to_build` 阶段，Plan 模式自动将方案步骤写入执行计划。

### 5. 持久化与收尾

小张退出前用 `/sessions` 查看历史会话列表，确认方案会话已自动持久化。他配置了桌面通知，关闭终端后收到了审批请求的通知提醒。

---

**本故事覆盖功能**：

| 模块 | 功能点 |
|------|--------|
| 运行模式 | `chat`/`tui` 交互模式, `install` 安装 |
| Provider | OpenAI-compatible, Anthropic, Gemini 三适配; 多 Provider 注册路由; 模型动态切换; `auto_detect_type` |
| TUI | Bubble Tea 全功能终端; 启动引导页; `/new` `/models` `/sessions` `/plan` 命令; 子智能体面板; Skill 面板; Plan 面板; 上下文窗口可视化; @mentions 补全; 命令面板 |
| Plan 模式 | explore→clarify→draft→converge_ready→approved_to_build 阶段流转; 步骤状态跟踪; 风险等级; Plan 面板渲染 |
| 工具 | `list_files`, `read_file`, `search_text`, `web_fetch`, `web_search` |
| 子智能体 | `explorer` 代码探索; `delegate_subagent` 委托执行; builtin/user/project 三级管理 |
| Skills | `write-rfc`; 三级 scope (builtin/user/project); Skill 激活/清除 |
| 会话 | JSONL 持久化; 会话列表/恢复; 事件日志 |
| 上下文 | 上下文窗口预算管理; warning/critical 告警 |
| 通知 | 桌面通知 (审批/完成/失败); 通知冷却时间 |
| 配置 | JSON 配置; 环境变量覆盖 (`BYTEMIND_HOME`); `provider_runtime` 多 provider 配置 |

---

## 故事二：开发阶段 — "实现消息推送模块"

> **角色**：小张确认方案后，切换到 Build 模式开始写代码。

### 1. 切换 Build 模式，全自动执行

小张在 TUI 中恢复上次 Plan 会话（`/resume <id>`），然后切换到 Build 模式直接开始实现。他通过 `/models` 切换到 `deepseek-v4-pro` 以降低长任务成本。输入：

> "按照刚才的方案，帮我实现消息推送模块，包括 APNs 和 FCM 两个 provider、消息队列、重试逻辑。"

Build 模式下 ByteMind 直接开始干活，**流式输出**思考过程和工具调用结果，TUI 界面用不同颜色区分 thinking 和 assistant 内容。

### 2. 高强度工具调用

ByteMind 自动编排工具调用序列：
- `write_file` 创建 `push/provider.go`、`push/apns.go`、`push/fcm.go`、`push/queue.go`、`push/retry.go` 等文件
- `replace_in_file` 在现有模块中注入依赖
- `apply_patch` 修复编译错误
- `run_shell` 执行 `go mod tidy`、`go build` 等命令

TUI 的 **Markdown 渲染器**将工具输出格式化展示，diff 内容带**语法高亮**，`run_shell` 的执行结果实时流式显示在终端中。

### 3. 安全审批与沙箱

当 ByteMind 尝试执行 `go build` 时，由于配置了 `approval_policy: "on-request"`，Shell 命令触发了**审批流程**。TUI 弹出审批对话框，显示命令内容和风险评估。小张确认后继续。

小张之前配置了：

```json
{
  "approval_policy": "on-request",
  "sandbox_enabled": true,
  "system_sandbox_mode": "non-blocking",
  "writable_roots": ["/home/user/project"],
  "exec_allowlist": [
    { "command": "go", "args_pattern": ["build", "test", "mod", "vet", "fmt"] }
  ],
  "network_allowlist": [
    { "host": "api.github.com", "port": "443" }
  ]
}
```

- 文件沙箱保证工具只能读写 `writable_roots` 范围内的文件
- 命令白名单限制只能执行 `go build/test/mod/vet/fmt`
- 网络沙箱限制只能访问 `api.github.com`

当 ByteMind 尝试读取 `/etc/passwd` 时，**沙箱**直接 `deny` 并返回 `fs_out_of_scope`；尝试 `curl` 外网地址时被网络沙箱拦截，返回 `network_not_allowed`。

### 4. Provider 故障自动切换

实现过程中，OpenAI provider 突然返回 503 错误。ByteMind 的 **Provider 路由**检测到主 provider 不健康，自动通过**健康检查**切换到备用 Anthropic provider，任务无缝继续。小张在 TUI 的状态栏看到了 provider 切换提示。

### 5. 子智能体并行加速

编译时发现缺少 protobuf 定义，小张手动输入：

> "帮我生成 push.proto 文件，然后用 protoc 编译"

同时他派发 `general` 子智能体去写单元测试：

```
@general 帮我给 push/ 目录下所有文件写单元测试，覆盖正常路径和边界情况
```

子智能体在**后台运行**，通过 `task_output` 查看结果。TUI 底部的状态栏显示后台任务进度，完成后桌面弹出通知。

### 6. 预算控制与上下文压缩

实现过程中已跑了 50+ 轮工具调用，接近 `max_iterations: 64`。ByteMind 触发了**阶段性总结**（stop summary），归纳已完成的工作和剩余待办项。同时**上下文压缩**自动触发，将较早的对话压缩为摘要，释放上下文窗口空间。**重复调用检测**发现了两次相同的 `go build` 调用并及时终止。

### 7. Token 用量监控

TUI 右下角的 **Token 用量实时监控**组件显示了本轮会话的 token 消耗（输入/输出/总计），小张设置了 `alert_threshold: 100000`，当日总 token 接近阈值时弹出了告警。用量数据自动写入 SQLite 数据库（`database_driver: "sqlite"`）。

---

**本故事覆盖功能**：

| 模块 | 功能点 |
|------|--------|
| 运行模式 | Build 模式; `--yolo` 全自动; `/resume` 会话恢复; `run` 单次任务 |
| Provider | 健康检查; 故障自动切换; Provider 路由回退; 模型切换 |
| 对话引擎 | 流式输出; 多轮对话; 工具调用循环; max_iterations 预算控制; stop summary 阶段总结; 重复调用检测; 上下文压缩 |
| 工具 | `write_file`, `replace_in_file`, `apply_patch`, `run_shell`; 工具执行审计 |
| TUI | Markdown 渲染; diff 语法高亮; 审批对话框; 后台任务状态栏; Token 用量实时监控; 桌面通知; 鼠标文本选择; 剪贴板粘贴; 图片输入 |
| 审批安全 | on-request 分级审批; Shell 命令审批; 文件沙箱 (writable_roots); 网络沙箱 (network_allowlist); 命令白名单 (exec_allowlist); 沙箱 escalate/deny/allow 决策 |
| 沙箱 | FS/Exec/Network 三级拦截; 审批通道 |
| 子智能体 | `general` 子智能体; 子智能体并行执行; `task_output` 结果查看 |
| 后台任务 | 后台/前台任务; 任务超时; worktree 隔离执行 |
| Token | 实时监控; SQLite 持久化; 用量告警; 多存储后端 |
| 上下文 | 自动压缩; 窗口预算 |

---

## 故事三：调试阶段 — "排查线上推送失败问题"

> **角色**：小张实现的推送模块上线后出现间歇性推送失败，需要定位根因。

### 1. 快速定位问题代码

小张打开终端启动 TUI，恢复之前开发推送模块的会话继续对话：

```bash
bytemind chat
```

```
/resume push-module
```

> "线上出现间歇性推送失败，错误日志显示 'connection timeout after 30s'，帮我排查根因。"

ByteMind 通过 `search_text` 搜索代码中所有 timeout 相关配置，`read_file` 读取关键文件定位到 `push/apns.go` 中的 HTTP Client 超时设置为硬编码的 30s。

### 2. 深入排查：Shell + Web 联动

ByteMind 用 `run_shell` 执行 `go test -v -run TestAPNsRetry ./push/...` 查看测试覆盖情况，发现重试逻辑的单元测试没有覆盖 timeout 场景。

接着用 `web_search` 搜索 "APNs timeout best practice 2026"，用 `web_fetch` 读取 Apple 官方文档中关于 connection timeout 的建议。

### 3. 激活 Bug Investigation Skill

小张激活内置的 `bug-investigation` Skill：

```
/skill bug-investigation
```

Skill 替换系统提示词为 Bug 调查专用模板，引导 ByteMind 从以下维度系统排查：
- 问题复现条件
- 影响范围（影响多少用户/设备）
- 代码层面根因
- 配置/环境因素
- 修复方案与回归验证

ByteMind 自动排查了：
- `push/retry.go` 的重试策略是否对 timeout 场景生效
- `push/fcm.go` 是否也有同样的硬编码问题
- `config/config.go` 中是否有可配置的超时参数

最终定位到两个问题：HTTP Client 超时硬编码 + 重试逻辑对 context.DeadlineExceeded 未正确捕获。

### 4. 修复与验证

> "把超时改成可配置的，默认值 60s；修复 retry.go 中对 context.DeadlineExceeded 的处理。"

ByteMind 用 `replace_in_file` 修改了相关代码，用 `run_shell` 跑了 `go vet ./push/...`、`go test -race ./push/...` 验证。

小张在 TUI 中通过鼠标**拖拽选择**了一段 diff 输出，`Ctrl+C` 复制后贴到代码审查文档里。diff 的**语法高亮**让改动一目了然。

---

**本故事覆盖功能**：

| 模块 | 功能点 |
|------|--------|
| 运行模式 | `chat` TUI 交互; `/resume` 会话恢复 |
| 工具 | `search_text`, `read_file`, `run_shell`, `replace_in_file`, `web_search`, `web_fetch` |
| Skills | `bug-investigation` Skill 激活/清除; Skill 提示词替换 |
| TUI | diff 语法高亮; 鼠标拖拽选择; 剪贴板复制; 终端流式输出 |
| 对话引擎 | 多轮对话排查; 流式输出 |
| 安全 | `run_shell` 命令白名单审批 |
| 会话 | 历史会话恢复 |

---

## 故事四：代码审查 — "Review 推送模块 PR"

> **角色**：小张的同事小王负责 Review 这次改动，他用 ByteMind 进行深度代码审查。

### 1. 启动审查

小王拉取 PR 分支后在项目目录启动 ByteMind：

```bash
bytemind chat
```

> "帮我 review 当前分支相对于 main 的所有改动，重点关注并发安全、错误处理、资源泄漏。"

### 2. 激活 Review Skill + Review 子智能体

小王先激活 `review` Skill：

```
/skill review
```

Review Skill 提供了结构化的审查框架（安全性、性能、可维护性、测试覆盖等维度）。

同时他派发 `review` 内置子智能体：

```
@review 审查 push/ 目录下所有文件，检查并发安全问题
```

子智能体通过 `read_file`、`search_text` 检查了 mutex 使用、goroutine 泄漏、channel 关闭等问题。返回结果指出了 `push/queue.go` 中一处 channel 未正确关闭可能导致 goroutine 泄漏的问题。

### 3. 逐文件审查与 MCP 集成

小王之前通过 `bytemind mcp add` 接入了团队的代码质量 MCP 服务器：

```bash
bytemind mcp add my-linter -- node ./linter-mcp-server.js
bytemind mcp list
bytemind mcp health my-linter
```

在 TUI 中，MCP 工具自动注册到 ByteMind，审查时额外调用了 MCP 提供的静态分析能力。小王可以在 **MCP 管理面板**中查看所有 MCP 服务器的健康状态。

ByteMind 逐文件审查：
- `push/apns.go` — `read_file` 检查 HTTP Client 连接池配置
- `push/fcm.go` — `search_text` 搜索 error handling 模式
- `push/queue.go` — 重点审查 channel 生命周期
- `push/retry.go` — 检查 backoff 策略和 context 取消传播

### 4. Diff 预览与总结

小王用 diff 预览工具查看改动：

> "展示当前分支所有改动的 diff 摘要。"

ByteMind 用 `diff_preview` 工具生成变更摘要。TUI 的 **diff 渲染器**将增删改分别用绿色/红色/黄色高亮展示。

最终 ByteMind 输出了一份结构化 Review 报告，包含：
- 严重问题（goroutine 泄漏）→ 风险等级 high
- 建议改进（超时配置应加校验）→ 风险等级 medium
- 测试覆盖分析（timeout 场景已覆盖）→ 通过

整个审查会话自动**持久化**为 JSONL，小王用 `/sessions` 可以随时回溯。他想看看这次审查消耗了多少 token，通过 `/session` 查看当前会话的消息统计和 token 消耗。

---

**本故事覆盖功能**：

| 模块 | 功能点 |
|------|--------|
| 运行模式 | `bytemind mcp add/list/health` MCP 管理命令 |
| TUI | MCP 管理面板; diff 渲染器 (绿/红/黄); 会话消息统计 |
| 工具 | `read_file`, `search_text`, `diff_preview` |
| Skills | `review` Skill 结构化审查框架 |
| 子智能体 | `review` 子智能体; builtin 内置定义 |
| MCP | MCP 服务器增删查; 健康检查; MCP 工具自动注册; MCP 面板 |
| 扩展 | Extensions 生命周期管理; MCP adapter |
| 会话 | JSONL 持久化; 会话列表; 消息统计 |
| Token | 会话级 token 消耗展示 |

---

## 功能覆盖率总览

以下按模块列出所有功能点及其在四个故事中的分布：

| 模块 | 功能点 | 故事一(设计) | 故事二(开发) | 故事三(Debug) | 故事四(Review) |
|------|--------|:---:|:---:|:---:|:---:|
| **运行模式** | `chat`/`tui` 交互 | ✅ | ✅ | ✅ | ✅ |
| | `run` 单次任务 | | ✅ | | |
| | `worker` 后台进程 | | ✅ | | |
| | `install` 安装 | ✅ | | | |
| | `mcp` MCP 管理 | | | | ✅ |
| | `version` 版本 | | | | |
| | `--yolo` 全自动 | | ✅ | | |
| **Provider** | OpenAI-compatible 适配 | ✅ | | | |
| | Anthropic 适配 | ✅ | | | |
| | Gemini 适配 | ✅ | | | |
| | 多 Provider 注册与路由 | ✅ | ✅ | | |
| | 健康检查 + 故障切换 | | ✅ | | |
| | 模型列表查询 | ✅ | | | |
| | `auto_detect_type` | ✅ | | | |
| **对话引擎** | 多轮对话 | ✅ | ✅ | ✅ | |
| | 流式输出 | ✅ | ✅ | ✅ | |
| | Build 模式 | | ✅ | | |
| | Plan 模式 | ✅ | | | |
| | 上下文压缩 | | ✅ | | |
| | max_iterations 预算 | | ✅ | | |
| | 重复调用检测 | | ✅ | | |
| | stop summary | | ✅ | | |
| | 子智能体委托 | ✅ | ✅ | | ✅ |
| **工具** | `list_files` | ✅ | | | ✅ |
| | `read_file` | ✅ | | ✅ | ✅ |
| | `search_text` | ✅ | | ✅ | ✅ |
| | `write_file` | | ✅ | | |
| | `replace_in_file` | | ✅ | ✅ | |
| | `apply_patch` | | ✅ | | |
| | `run_shell` | | ✅ | ✅ | |
| | `web_fetch` | ✅ | | ✅ | |
| | `web_search` | ✅ | | ✅ | |
| | `delegate_subagent` | ✅ | ✅ | | |
| | `task_output` / `task_stop` | | ✅ | | |
| | `diff_preview` | | | | ✅ |
| **TUI** | Bubble Tea 终端 UI | ✅ | ✅ | ✅ | ✅ |
| | Markdown 渲染 | | ✅ | | |
| | diff 语法高亮 | | ✅ | ✅ | ✅ |
| | 鼠标支持 (选择/拖拽/滚动) | | ✅ | ✅ | |
| | 剪贴板粘贴 | | ✅ | ✅ | |
| | 图片输入 | | ✅ | | |
| | 会话管理面板 | ✅ | | | ✅ |
| | 模型切换 (`/models`) | ✅ | ✅ | | |
| | 子智能体面板 | ✅ | | | |
| | Skill 面板 | ✅ | | | |
| | MCP 面板 | | | | ✅ |
| | Plan 面板 | ✅ | | | |
| | Token 用量监控 | | ✅ | | |
| | 命令面板/调色板 | ✅ | | | |
| | @mentions 自动补全 | ✅ | | | |
| | `/` 命令补全 | ✅ | | | |
| | 桌面通知 | ✅ | ✅ | | |
| | 启动引导页 | ✅ | | | |
| | 上下文窗口可视化 | ✅ | | | |
| | 审批对话框 | | ✅ | | |
| | 后台任务状态栏 | | ✅ | | |
| | 增强输入框 (多行) | ✅ | | | |
| **审批安全** | on-request / away / full_access | | ✅ | | |
| | Shell 命令审批 | | ✅ | ✅ | |
| | 文件沙箱 (writable_roots) | | ✅ | | |
| | 网络沙箱 (network_allowlist) | | ✅ | | |
| | 命令白名单 (exec_allowlist) | | ✅ | ✅ | |
| | worktree 隔离 | | ✅ | | |
| **扩展系统** | MCP 服务器增删查 | | | | ✅ |
| | MCP 健康检查 | | | | ✅ |
| | Skills (6 个内置) | ✅ | | ✅ | ✅ |
| | Skill 三级 scope | ✅ | | | |
| | 子智能体 (3 个内置) | ✅ | ✅ | | ✅ |
| | 子智能体三级 scope | ✅ | | | |
| **Plan 模式** | 阶段流转 | ✅ | | | |
| | 步骤状态跟踪 | ✅ | | | |
| | 风险等级 | ✅ | | | |
| | Plan 面板渲染 | ✅ | | | |
| **会话** | JSONL 持久化 | ✅ | ✅ | | ✅ |
| | 会话列表/恢复 | ✅ | | ✅ | ✅ |
| | 事件日志 | ✅ | | | |
| | 会话删除/清理 | | | | |
| | 消息统计 | | | | ✅ |
| **上下文** | 窗口预算管理 | ✅ | ✅ | | |
| | warning/critical 告警 | ✅ | | | |
| | 自动压缩 | | ✅ | | |
| **Token** | 用量追踪 | | ✅ | | ✅ |
| | 多后端 (文件/DB/内存) | | ✅ | | |
| | 用量告警 | | ✅ | | |
| | 实时监控 | | ✅ | | |
| **后台任务** | 后台/前台执行 | | ✅ | | |
| | 超时控制 | | ✅ | | |
| | 重试 | | ✅ | | |
| | worktree 隔离 | | ✅ | | |
| **通知** | 桌面通知 | ✅ | ✅ | | |
| | 审批/完成/失败通知 | ✅ | ✅ | | |
| | 冷却时间 | ✅ | | | |
| **配置** | JSON 配置文件 | ✅ | | | |
| | 环境变量覆盖 | ✅ | | | |
| | `provider_runtime` | ✅ | | | |
| | 更新检查 | | | | |
