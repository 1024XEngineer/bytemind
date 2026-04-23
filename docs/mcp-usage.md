# MCP 使用姿势（双入口）

本文档给出当前项目可用的两种 MCP 配置方式：`slash 命令` 和 `自然语言`。

## 1. 在 TUI 里用 Slash（单命令方式）

1. 启动 TUI：

```bash
go run ./cmd/bytemind chat
```

2. 输入一条命令直接执行：

```text
/mcp setup <id> [--cmd <command>] [--args a,b] [--env K=V[,K2=V2]]
```

3. 命令会自动执行：`Add -> Test -> Enable -> Reload`。

常见示例：

```text
/mcp setup github
/mcp setup filesystem --cmd npx --args -y,@modelcontextprotocol/server-filesystem
/mcp setup github --env GITHUB_PERSONAL_ACCESS_TOKEN=ghp_xxx
```

## 2. 在 TUI 里用自然语言（快捷方式）

可以直接输入自然语句触发同一个 setup 向导，不必手敲 slash：

```text
帮我配置 github mcp
请添加 filesystem mcp
configure github mcp
add filesystem mcp
```

说明：

1. 需要句子里包含 MCP 语义和配置动作（如 setup/configure/add/配置/添加/接入）。
2. 需要能识别出 server id（如 `github`、`filesystem`）。
3. 若缺少 id，系统会提示你补充（例如改成“帮我配置 github mcp”）。

## 3. 配置后怎么确认是否生效

在 TUI 输入：

```text
/mcp list
/mcp show <id>
/mcp help
```

`/mcp list` 可看当前 server 状态和 tools 数量；`/mcp show <id>` 可看该 server 的详细配置与运行态。

## 4. 非 TUI 场景（CLI）

自动化脚本或终端可直接使用：

```bash
bytemind mcp list
bytemind mcp add --id <id> --cmd <command>
bytemind mcp test --id <id>
bytemind mcp enable --id <id>
bytemind mcp disable --id <id>
bytemind mcp remove --id <id>
bytemind mcp reload
```

## 5. 常见误区

1. “看起来卡住”：setup 向导等待你在输入框继续填写下一步参数，不是死锁。
2. 输入了自然语言但没触发：通常是缺少 `mcp` 关键词或缺少 server id。
3. 配置失败：先看 `/mcp show <id>` 的 `status/message`，再按提示修正 command/env/token。
