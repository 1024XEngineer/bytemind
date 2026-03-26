# ForgeCLI 项目完整分析

## 项目是什么

**ForgeCLI** 是一个 **终端原生编程 Agent 运行时**，类似 Claude Code 或 Cursor。

---

## 实际完成的功能

### 1. 核心架构

| 模块 | 位置 | 功能 |
|------|------|------|
| 入口 | `cmd/main.go` | REPL 交互界面 |
| 会话 | `pkg/session/` | 消息历史 + 持久化 |
| 工作区 | `pkg/workspace/` | 文件读写、glob、grep |
| 编辑 | `pkg/edit/` | diff 生成、hash 校验 |
| 执行 | `pkg/execx/` | 命令执行、超时控制 |
| 策略 | `pkg/policy/` | 敏感文件保护、危险命令拦截 |
| 模型 | `pkg/model/` | DeepSeek API 调用、Tool Calling |
| 技能 | `pkg/skills/` | gstack skills 集成 |

### 2. 用户交互

**本地命令：**
```
help       - 显示帮助
config     - 查看/修改配置
status     - 会话状态
skills     - 列出可用 skills
/<skill>   - 使用 skill（如 /qa）
list       - 列出文件
glob <p>   - 搜索文件
grep <p>   - 搜索内容
read <f>   - 读取文件
edit <f>   - 编辑文件
!<cmd>     - 执行命令
exit       - 退出
```

### 3. Tool Calling（14个工具）

```
read_file, read_file_lines, list_files, glob, grep,
find_functions, write_file, edit_file, create_directory,
delete_file, execute_command, get_file_info,
get_project_structure, analyze_dependencies
```

### 4. Skills 集成（21个）

```
/office-hours, /plan-ceo-review, /plan-eng-review,
/plan-design-review, /design-consultation, /browse,
/qa, /qa-only, /review, /investigate, /design-review,
/ship, /document-release, /retro, /careful, /freeze,
/guard, /unfreeze, /gstack-upgrade, /codex, /setup-browser-cookies
```

### 5. 安全机制

- Trusted Workspace 检查（.git, package.json, go.mod 等）
- 敏感文件保护（.env, .pem, credentials.json）
- 危险命令拦截（rm -rf, del /s /q, format）
- 审计日志（.forgecli/audit.jsonl）

### 6. 配置系统

- API Key 管理（支持 DeepSeek / OpenAI）
- 配置文件：`~/.forgecli/config.json`
- 会话持久化：`.forgecli/sessions/`

---

## 构建产物

```bash
forgecli.exe  # 9.3 MB，可执行
```

---

## 当前状态总结

| 维度 | 状态 |
|------|------|
| **代码量** | ~1500 行（不含 gstack-main） |
| **模块化** | ✅ 7 个独立包 |
| **Tool Calling** | ✅ 14 个工具 |
| **Skills** | ✅ 21 个集成 |
| **可执行** | ✅ 已编译运行过 |
| **测试** | ❌ 无 |
| **文档** | ❌ 无 |

---

## 客观评价

### 优点

- ✅ 功能完整，基本的 Agent 运行时已实现
- ✅ 模块化结构清晰
- ✅ 安全机制到位
- ✅ 已有运行痕迹（audit.jsonl 有记录）

### 待改进

- ❌ 无测试文件
- ❌ 无 README/文档
- ❌ 错误处理不完整（多处忽略 err）
- ❌ 只支持 Windows（硬编码 cmd /c）
- ❌ Tool Calling 的 ToolCallID 虽已加，但未验证

---

## 结论

这是一个**可运行的 MVP**，核心功能已完成，但还需要测试、文档和错误处理改进。
