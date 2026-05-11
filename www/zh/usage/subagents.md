# 子代理

**子代理**是一种拥有独立工具集和指令的专项 Agent，由主 Agent 通过 `delegate_subagent` 工具调起，在限定的范围内完成子任务。子代理适用于需要并行分工、隔离上下文或限制工具权限的复杂场景。

## 工作原理

1. 主 Agent 识别可拆解的子任务
2. 调用 `delegate_subagent` 工具，指定代理名称和任务描述
3. 子代理在独立的会话上下文中执行任务
4. 子代理完成后，将结果（含摘要、修改文件列表、工作历史）返回主 Agent
5. 主 Agent 整合结果继续执行

子代理可配置工具白名单/黑名单、最大轮次、隔离模式等，确保执行范围可控。

## 内置子代理

ByteMind 随附三个内置子代理：

### explorer（探索器）

只读型代码探索代理。用于定位文件、符号、调用链和配置流程。不执行任何写入或 Shell 命令。

| 属性       | 说明                          |
| ---------- | ----------------------------- |
| 工具集     | `list_files` `read_file` `search_text` |
| 最大轮次   | 6                             |
| 何时使用   | 查找文件、理解代码结构、定位模式 |

```text
/explorer
```

### review（审查器）

代码审查代理。分析变更的正确性、回归风险、安全问题和测试覆盖盲点。只产出审查结果，不修改代码。

| 属性       | 说明                          |
| ---------- | ----------------------------- |
| 工具集     | `list_files` `read_file` `search_text` |
| 最大轮次   | 8                             |
| 何时使用   | 审查代码、检查 Bug、评估质量  |

```text
/review
```

### general（通用代理）

通用编码代理，支持读写文件。适用于需要修改代码的复杂多步骤任务。不能继续委派其他子代理。

| 属性       | 说明                          |
| ---------- | ----------------------------- |
| 工具集     | `list_files` `read_file` `search_text` `replace_in_file` `write_file` |
| 最大轮次   | 12                            |
| 隔离       | none                          |
| 何时使用   | 多文件编辑、重构、功能实现    |

## 查看子代理

在会话中输入以下命令查看所有可用子代理：

```text
/agents
```

查看特定子代理的详细信息：

```text
/agents explorer
```

直接查看内置代理定义：

```text
/explorer
/review
```

## 自定义子代理

在 `.agents/agents/` 目录下创建 `.md` 文件来定义项目级子代理。每个文件使用 YAML frontmatter 描述元数据，Markdown 正文作为子代理的执行指令。

### 目录结构

```
.agents/
  agents/
    frontend-developer.md
    api-tester.md
```

### Frontmatter 字段

| 字段              | 类型   | 说明                               | 默认值            |
| ----------------- | ------ | ---------------------------------- | ----------------- |
| `name`            | string | 代理名称，用于 `delegate_subagent` 调用 | 文件名（不含扩展名） |
| `description`     | string | 简短描述，显示在 `/agents` 列表中  | 正文首段自动提取  |
| `tools`           | array  | 可用工具白名单                     | —                 |
| `disallowed_tools`| array  | 禁止使用的工具黑名单               | —                 |
| `model`           | string | 指定使用的模型（如 `sonnet`）      | 跟随主会话        |
| `mode`            | string | 工作模式：`build` 或 `plan`        | `build`           |
| `max_turns`       | int    | 最大工具调用轮次                   | 0（无限制）       |
| `isolation`       | string | 隔离模式：`none` 或 `worktree`     | `none`            |
| `when_to_use`     | string | 提示主 Agent 何时委派此代理        | —                 |
| `aliases`         | array  | 别名列表                           | 文件名、名称等自动生成 |

### 示例

```markdown
---
name: api-tester
description: "专注于 API 端点的集成测试编写和调试"
tools: [list_files, read_file, search_text, write_file, replace_in_file, run_shell]
disallowed_tools: [delegate_subagent]
model: sonnet
max_turns: 10
isolation: none
when_to_use: "用于编写 API 测试、调试接口问题、添加 HTTP 测试覆盖"
---

你是一个 API 测试专家。

## 工作流程

1. 先读取相关 handler 和路由定义
2. 分析现有测试模式和覆盖盲点
3. 编写最小但完整的集成测试
4. 运行测试并修复失败用例

## 规范

- 每个测试用例独立且可重复执行
- 覆盖正常路径和常见错误路径
- 不修改业务逻辑代码
```

### 作用域优先级

子代理从三个作用域加载，同名代理按优先级覆盖：

| 作用域    | 路径                       | 优先级 |
| --------- | -------------------------- | ------ |
| `project` | `.agents/agents/`          | 最高   |
| `user`    | `~/.bytemind/agents/`      | 中     |
| `builtin` | 内置                       | 最低   |

项目级配置优先级最高：`.agents/agents/frontend-developer.md`（project）会覆盖 `~/.bytemind/agents/frontend-developer.md`（user）中的同名代理。

## 子代理隔离（Worktree）

设置 `isolation: worktree` 后，子代理会在独立的 git worktree 中执行，文件变更与主工作区隔离。完成后可选择保留或丢弃 worktree。适合需要大量试验性变更的场景。

## 在对话中委派

一般情况下你不需要手动调用 `delegate_subagent`——主 Agent 会根据任务复杂度自动判断何时委派。你也可以直接要求：

```text
用 explorer 子代理查找所有认证相关的中间件
用 review 子代理审查最近一次变更
```

## 相关页面

- [技能](/zh/usage/skills) — 斜杠命令激活的专项工作流
- [聊天模式](/zh/usage/chat-mode) — 子代理在其中运行
- [核心概念](/zh/core-concepts) — Agent 模式与工具
