# 会话管理

ByteMind 的每次对话都存在于一个**会话**中。会话自动持久化到磁盘，可随时中断和恢复而不丢失上下文。

## 会话工作原理

- 每个会话有唯一 ID（如 `abc123def`）
- 会话数据存储在 ByteMind home 目录中，默认是用户目录下的 `.bytemind/`
- 运行 `bytemind` 时创建新会话或恢复已有会话
- 消息历史保留，Agent 具备累加的历史上下文；极长的会话会自动触发上下文压缩以避免超出模型窗口限制

## 打开会话选择器

```text
/session
```

执行后弹出交互式选择框：

```
Recent Sessions
Page 1/3 · Total 22
Up/Down move, Left/Right page, Enter resume, Delete remove, Esc close

> abc123def  2026-05-11 14:22  raw:18
   /d/code/my-project
   重构认证模块

  def456ghi  2026-05-10 09:15  raw:5
   /d/code/my-project
   修复登录 500 错误
```

键盘操作：

| 按键 | 功能 |
| ---- | ---- |
| `↑` `↓` 或 `k` `j` | 上下移动光标 |
| `←` `→` | 翻页（每页最多 8 条，最多 10 页） |
| `Enter` | 切换到选中的会话（恢复上下文） |
| `Delete` | 删除选中的会话 |
| `Esc` | 关闭选择器，保留在当前会话 |

没有单独的 `/sessions` 或 `/resume` 命令——所有会话的查看、恢复、删除都在 `/session` 选择框内完成。

## 开启新会话

```text
/new
```

在当前工作区创建全新会话。之前的会话仍然保存，随时可在 `/session` 选择器中恢复。

## 实用场景

**跨多天的大重构**

每天做一段工作，再回来继续：

```
/session → 选中昨天的会话 → Enter 恢复
```

**并行工作流**

用 `/new` 为不同功能分支分别建会话，避免上下文混乱。每个会话独立持久化。

**清理旧会话**

```
/session → 上下移动到不用的会话 → Delete 删除
```

## 存储位置

会话文件存储在 ByteMind home 目录中，默认是用户目录下的 `.bytemind/`。可通过 `BYTEMIND_HOME` 环境变量覆盖基础路径。

## 相关页面

- [交互模式 (Build)](/zh/usage/chat-mode) — 会话的使用场景
- [环境变量](/zh/reference/env-vars) — `BYTEMIND_HOME` 覆盖
- [CLI 命令](/zh/reference/cli-commands) — 完整命令参考
