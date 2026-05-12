# ByteMind 用户故事

下面的故事展示 ByteMind 当前已实现能力的常见组合用法，并标出每个流程背后的架构链路。它们不是功能清单，也不承诺覆盖所有内部模块；需要完整命令和配置字段时，请以参考页为准。

---

## 故事一：接手新仓库，先建立上下文

> **角色**：后端工程师小张刚接手一个 Go 项目，希望先了解目录结构、入口和测试方式，再决定是否修改代码。

### 1. 配置模型并启动

小张先创建全局配置 `~/.bytemind/config.json`。如果需要在多个模型之间切换，可以使用当前的 `provider_runtime.providers` 对象格式：

```json
{
  "provider_runtime": {
    "current_provider": "deepseek",
    "default_provider": "deepseek",
    "default_model": "deepseek-v4-flash",
    "providers": {
      "deepseek": {
        "type": "openai-compatible",
        "base_url": "https://api.deepseek.com",
        "api_key_env": "DEEPSEEK_API_KEY",
        "model": "deepseek-v4-flash",
        "models": ["deepseek-v4-flash", "deepseek-v4-pro"]
      },
      "openai": {
        "type": "openai-compatible",
        "base_url": "https://api.openai.com/v1",
        "api_key_env": "OPENAI_API_KEY",
        "model": "gpt-5.4-mini",
        "models": ["gpt-5.4-mini", "gpt-5.4"]
      }
    }
  }
}
```

进入项目目录后启动交互界面：

```bash
bytemind
```

`bytemind chat` 和 `bytemind tui` 仍是兼容入口，效果等同于打开交互 TUI。

### 2. 只读探索，不急着改代码

小张先输入：

```text
请先熟悉这个仓库：梳理入口、主要包、测试命令和配置加载流程。不要修改文件。
```

ByteMind 会根据需要调用只读工具，例如 `list_files`、`read_file` 和 `search_text`。如果任务适合隔离上下文，小张也可以明确提到内置探索子代理：

```text
@explorer 帮我定位配置加载、CLI 入口和测试入口，返回关键文件路径。
```

`@explorer` 是给主 Agent 的委派提示；主 Agent 会在合适时通过 `delegate_subagent` 调用只读子代理，而不是让用户手动调用底层工具。

### 3. 使用会话和模型切换

如果小张需要换模型，可以输入：

```text
/model
```

TUI 会打开已配置 provider/model 的选择器；也可以直接输入类似 `/model openai/gpt-5.4` 的目标。

探索结束后，会话会自动持久化。下次回来时，在 TUI 中输入：

```text
/session
```

然后在最近会话选择器里用方向键选择会话，按 `Enter` 恢复。TUI 当前不使用 `/resume <id>`；这个命令只保留在 CLI/脚本化恢复路径中。

### 架构链路

TUI 输入层 → Session Store → Agent Runner → Tool Registry → Subagent Gateway → Provider Runtime → TUI 会话与工具渲染

| 层级 | 涉及能力 |
| ---- | -------- |
| 用户入口 | `bytemind`、`/model`、`/session` |
| 会话层 | 会话创建、自动保存、最近会话恢复 |
| Agent 编排 | 根据任务决定直接探索或委派 `explorer` |
| 工具层 | `list_files`、`read_file`、`search_text`、`delegate_subagent` |
| Provider 层 | `provider_runtime` 配置和模型切换 |

---

## 故事二：先规划，再执行一组多文件修改

> **角色**：小张需要把认证模块的一段校验逻辑抽到独立包里，改动会影响多个调用方。他希望先看到计划，再开始修改。

### 1. 切到 Plan 模式

在交互界面中按 `Tab` 可以在 Build 和 Plan 之间切换。切到 Plan 后，小张输入：

```text
把认证模块里的 token 校验逻辑抽到 internal/tokenval 包。先给我计划，包含会改哪些文件、风险和验证命令；在我确认前不要写文件。
```

Plan 模式下，Agent 会先探索相关代码，再通过 `update_plan` 维护结构化步骤。典型计划会包含：

- 读取现有认证中间件和测试
- 设计 `internal/tokenval` 的最小接口
- 更新调用方
- 添加或调整测试
- 运行 `go test ./...`

### 2. 批准后进入 Build 执行

小张确认计划后，使用界面中的执行选项开始实现。兼容情况下，也可以输入 `start execution` 或 `continue execution` 让当前计划进入执行。

执行阶段 ByteMind 可能会调用 `read_file`、`search_text`、`write_file`、`replace_in_file`、`apply_patch` 和 `run_shell`。默认审批模式下，写文件和执行 Shell 命令会弹出确认；读文件和搜索通常直接执行。

### 3. 用审批和回滚控制风险

如果 ByteMind 要执行：

```bash
go test ./...
```

TUI 会在需要审批时展示命令和原因。小张可以只批准这一次，也可以允许当前 TUI 会话中同类后续请求自动通过。

如果某次文件修改方向不对，当前 TUI 提供：

```text
/rollback
```

它会列出由 `write_file`、`replace_in_file` 或 `apply_patch` 记录的文件编辑快照；使用 `/rollback last` 或指定 operation id 可以回退 ByteMind 记录的文件编辑。它不是 Git 回滚，也不会替代人工检查 diff。

### 架构链路

TUI 模式状态 → Plan State → `update_plan` → Agent Runner → Tool Registry → Approval/Sandbox → 文件编辑快照 → TUI 工具调用渲染

| 层级 | 涉及能力 |
| ---- | -------- |
| 用户入口 | `Tab` 切换 Build / Plan、计划确认、`/rollback` |
| Plan 层 | 结构化步骤、风险和验证方案 |
| 工具层 | `read_file`、`search_text`、`write_file`、`replace_in_file`、`apply_patch`、`run_shell` |
| 安全层 | 高风险工具审批、`exec_allowlist`、沙箱边界 |
| 恢复层 | 文件编辑快照和回滚 |

---

## 故事三：排查一个可复现 Bug

> **角色**：线上接口偶发 500，小张已经拿到错误关键字和一段日志，想让 ByteMind 帮他定位根因并给出最小修复。

### 1. 选择 Bug 排查工作流

小张可以通过技能选择器选择内置的 Bug Investigation 技能：

```text
/skills-select
```

也可以在已加载的技能列表中查看当前可用技能：

```text
/skills
```

激活后，他输入：

```text
症状：订单创建接口偶发 500，日志里有 "nil pointer in price calculator"。请先复现路径和证据，再给最小修复方案。
```

技能会让 Agent 更偏向先收集证据，而不是直接猜测改法。

### 2. 读取代码、运行验证

ByteMind 先用 `search_text` 搜索错误关键字和相关调用链，再用 `read_file` 阅读实现和测试。需要验证时，它可以通过 `run_shell` 执行聚焦测试，例如：

```bash
go test ./internal/order -run TestCreateOrder
```

如果测试命令不在 `exec_allowlist` 中，默认会进入审批流程。小张确认后，命令才会执行。

### 3. 修改并复测

确认根因后，小张要求：

```text
只修复 price calculator 的 nil pointer 问题，并补一个能复现的单元测试。不要重构其它代码。
```

ByteMind 只在相关文件中写入补丁，随后再次运行聚焦测试。若达到 `max_iterations` 上限，Agent 会输出阶段性总结，说明已经完成的工作、阻塞点和建议下一步，而不是静默继续消耗轮次。

### 架构链路

Skill Manager → Agent Runner → Tool Registry → Approval/Sandbox → Provider Runtime → Session Store → TUI 结果展示

| 层级 | 涉及能力 |
| ---- | -------- |
| 用户入口 | `/skills-select`、`/skills`、自然语言症状描述 |
| 技能层 | Bug Investigation 技能注入排查流程 |
| Agent 编排 | 证据优先、最小修复、验证闭环 |
| 工具层 | `search_text`、`read_file`、`run_shell`、写入类工具 |
| 预算层 | `max_iterations` 上限和阶段性总结 |

---

## 故事四：审查当前分支并整理提交

> **角色**：小王要审查当前分支相对 `main` 的改动，重点看回归风险和测试覆盖。

### 1. 启动 Review 工作流

小王启动 ByteMind 后，可以选择 Review 技能，或直接描述审查目标：

```text
请 review 当前分支相对 main 的改动，优先找正确性问题、回归风险和缺失测试。先给发现，不要修改文件。
```

ByteMind 会通过 `run_shell` 获取必要的 Git 信息，通过 `read_file` 和 `search_text` 阅读相关代码。审查结果应优先列出具体问题、文件位置和风险，而不是泛泛总结。

如果希望只读审查与主上下文隔离，可以提示：

```text
@review 请只读审查当前改动，重点看并发安全和错误处理。
```

主 Agent 会在合适时通过 `delegate_subagent` 调用内置 review 子代理。review 子代理是只读的，不会修改文件。

### 2. 查看 MCP 状态

如果项目配置了 MCP 服务器，命令行可以管理它们：

```bash
bytemind mcp list
bytemind mcp add github --cmd npx --args "-y,@modelcontextprotocol/server-github"
bytemind mcp test github
```

在 TUI 中可查看当前配置和运行状态：

```text
/mcp list
/mcp show github
```

MCP 服务器提供的工具会以稳定 key 注册到工具列表中，例如 `mcp:github:search_code`。

### 3. 创建本地提交

小王确认修改已经通过测试后，在 TUI 里输入：

```text
/commit fix(order): guard nil price calculator
```

ByteMind 会执行 `git add -A` 并创建本地 commit，然后反馈 commit hash、message 和文件数量。如果刚创建完发现提交信息或内容有问题，可以使用：

```text
/undo-commit
```

该命令只回退当前会话里由 `/commit` 创建的最后一个本地 commit，并保留文件改动。

### 架构链路

TUI 输入层 → Skill / Subagent 提示 → Agent Runner → Tool Registry / MCP Runtime → Commit Command → Session Store

| 层级 | 涉及能力 |
| ---- | -------- |
| 用户入口 | Review 描述、`@review`、`/mcp list`、`/commit` |
| 审查层 | Review 技能或只读 review 子代理 |
| 扩展层 | MCP 配置、状态查看和外部工具注册 |
| 工具层 | `run_shell`、`read_file`、`search_text`、MCP 工具 |
| Git 层 | 本地提交和 `/undo-commit` |

---

## 常用能力对照

| 场景 | 当前推荐入口 |
| ---- | ------------ |
| 启动交互会话 | `bytemind`、`bytemind chat`、`bytemind tui` |
| 单次非交互任务 | `bytemind run -prompt "任务"` |
| 切换 Build / Plan | TUI 中按 `Tab` |
| 查看和恢复会话 | TUI 中 `/session` |
| 新建会话 | TUI 中 `/new` |
| 切换模型 | TUI 中 `/model` 或 `/model provider/model` |
| 查看子代理 | TUI 中 `/agents` |
| 提示使用子代理 | 在任务中提到 `@explorer` 或 `@review` |
| 查看技能 | TUI 中 `/skills` 或 `/skills-select` |
| 清除当前技能 | TUI 中 `/skill clear` |
| 查看 MCP 状态 | TUI 中 `/mcp list`、`/mcp show <id>` |
| 管理 MCP 配置 | Shell 中 `bytemind mcp <list|add|remove|enable|disable|test|reload>` |
| 压缩长会话 | TUI 中 `/compact` |
| 回退 ByteMind 文件编辑 | TUI 中 `/rollback` |
| 创建本地提交 | TUI 中 `/commit <message>` |
| 回退本会话创建的提交 | TUI 中 `/undo-commit` |
