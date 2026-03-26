# ForgeCLI 产品需求文档

## 1. 产品定位

**一句话：** 面向开发团队的终端原生、可审批、可审计的 coding agent runtime。

**核心差异：** 可信执行 + 企业适配 + Windows 一等公民

---

## 2. 目标用户

| 角色 | 优先级 |
|------|--------|
| 后端/全栈工程师 | P0 |
| 开发平台团队 | P0 |
| DevOps/SRE | P1 |
| Tech Lead | P1 |

**非目标：** 零基础用户、纯 IDE 内工作流

---

## 3. MVP 功能

### 3.1 核心功能

| 模块 | 功能 | 优先级 |
|------|------|--------|
| 会话系统 | 启动/输入/流式输出/会话持久化 | P0 |
| 仓库读取 | list/read/glob/rg + .gitignore 过滤 | P0 |
| 文件编辑 | 写前 hash 校验、diff 预览、原子写入 | P0 |
| 命令执行 | 超时/退出码/审批/日志 | P0 |
| 安全策略 | trusted workspace、敏感文件保护 | P0 |
| 模型接入 | OpenAI + Anthropic adapter | P0 |
| 规则兼容 | AGENTS.md / CLAUDE.md | P1 |

### 3.2 用户流程

1. 用户指定仓库 + 任务
2. 系统加载规则文件 + 仓库上下文
3. Agent 输出计划 → 用户确认
4. Agent 生成 diff → 用户审批
5. Agent 执行命令 → 验证结果
6. 输出总结 + 可恢复会话

### 3.3 风险控制

- 默认不自动执行高风险操作
- 文件写入前 hash 校验
- 命令执行需审批
- 敏感文件(.env/密钥)默认保护
- 所有操作写入审计日志

---

## 4. 竞品参考

### 分类
| 类别 | 代表产品 |
|------|----------|
| 终端原生 | Claude Code、Codex CLI、Gemini CLI、Qwen Code |
| 商业AI IDE | Cursor、Sourcegraph Amp、Trae、通义灵码 |
| 开源项目 | Aider、OpenHands、Goose、SWE-agent |

### 核心竞品对比

| 产品 | 代码理解 | 命令执行 | 文件修改 | 工具/MCP | 长任务恢复 | 多agent |
|------|:--------:|:--------:|:--------:|:--------:|:----------:|:-------:|
| Claude Code | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Codex CLI | ✅ | ✅ | ✅ | ✅ | 部分 | - |
| Gemini CLI | ✅ | ✅ | ✅ | ✅ | ✅ | - |
| Qwen Code | ✅ | ✅ | ✅ | ✅ | 部分 | ✅ |
| Aider | ✅ | 部分 | ✅ | - | 部分 | - |
| OpenHands | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

### 关键参考
- **Claude Code**：rules+tools+approvals+memory+subagents 一体化设计
- **Codex CLI**：单二进制、本地优先、审批与 sandbox 内建
- **Gemini CLI**：checkpoint/resume、trusted folders、headless 输出
- **Aider**：repo map、Git 工作流、编辑体验打磨

---

## 5. 技术架构

### 分层
```
CLI
  └─ Command Layer
      └─ Session Orchestrator
          ├─ Planner / Tool Loop
          ├─ Context Assembler
          ├─ Policy Engine
          ├─ Model Adapter
          └─ Tool Runtime
              ├─ Workspace FS
              ├─ Edit / Diff
              ├─ Shell Runner
              └─ Search / Index
```

### 模块

| 模块 | 职责 |
|------|------|
| `cmd/` | CLI 入口、子命令 |
| `session` | 会话状态、event log |
| `workspace` | 路径校验、文件访问 |
| `edit` | 结构化编辑、diff 生成 |
| `execx` | 命令执行、审批 |
| `model` | provider adapter |
| `policy` | 权限、审批、危险命令 |

### 技术选型

| 类别 | 推荐 |
|------|------|
| CLI框架 | Cobra |
| 配置 | Koanf |
| 会话存储 | SQLite |
| 命令执行 | os/exec |
| 检索 | Bleve BM25 |
| 日志 | slog |
| MCP | modelcontextprotocol/go-sdk |

---

## 6. 验证指标

| 指标 | 目标 |
|------|------|
| 任务闭环成功率 | >70% |
| 用户审批通过率 | >60% |
| 首次有效改动时间 | <5 分钟 |
| 人工接管率 | <20% |

**北极星指标：** 周成功闭环任务数

---

## 7. 风险与难点

| 风险 | 缓解措施 |
|------|----------|
| Agent可靠性 | turn 预算、自检步骤、模式化策略 |
| Shell执行安全 | direct exec、命令分级审批、审计日志 |
| 文件改动可控 | 写入前 hash 校验、统一 edit service |
| 大仓库上下文 | 关键词先行、分离摘要与局部上下文 |
| 模型幻觉 | 强制读取验证、不存在路径返回硬错误 |
| 成本控制 | 预算管理器、小模型承担路由、prompt caching |

---

## 8. 本期不做

- 多智能体编排
- 全屏 TUI
- 浏览器自动化
- 向量数据库
- IDE 插件
- 插件市场
- PTY 交互式 shell
- MCP server
- AST-aware 编辑
