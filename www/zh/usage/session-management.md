# 会话管理

ByteMind 的每次对话都存在于一个**会话**中。会话自动持久化到磁盘，可随时中断和恢复而不丢失任何上下文。

## 会话工作原理

- 每个会话有唯一 ID（如 `abc123def`）
- 会话数据存储在工作目录的 `.bytemind/` 中
- 运行 `bytemind chat` 时创建新会话或恢复已有会话
- 完整消息历史均保留，Agent 具备全部历史上下文

## 列出会话

```text
/sessions
```

输出最近会话列表，包含 ID、开始时间和消息数。可传入数字限制条数：

```text
/sessions 5
```

## 查看当前会话

```text
/session
```

显示当前会话的 ID、工作目录、消息数和任务摘要。

## 恢复会话

```text
/resume abc123
```

可使用完整 ID 或唯一前缀。Agent 会加载完整对话历史，从断点继续。

## 开启新会话

```text
/new
```

在当前工作区创建全新会话。之前的会话仍然保存，随时可恢复。

## 实用场景

**工作跨多天的大重构**

每天做一段工作，第二天回来继续：

```bash
bytemind chat
/sessions
/resume <昨天的会话 ID>
```

**并行工作流**

用 `/new` 为不同功能分支分别建会话，避免上下文丢或混淆。

**回顾已完成的工作**

```text
/session
```

显示当前会话中调用过的工具和已做变更的摘要。

## 存储位置

会话文件存储在 `bytemind chat` 启动目录的 `.bytemind/sessions/` 中。可通过 `BYTEMIND_HOME` 环境变量覆盖 `.bytemind/` 基础路径。

## 相关页面

- [聊天模式](/zh/usage/chat-mode) — 会话的使用场景
- [环境变量](/zh/reference/env-vars) — `BYTEMIND_HOME` 覆盖
- [CLI 命令](/zh/reference/cli-commands) — 完整命令参考
