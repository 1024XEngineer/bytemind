# MCP 使用方式（配置文件识别）

当前版本不再支持在 TUI 中通过 `/mcp setup ...` 或自然语言触发 MCP 添加流程。

## 1. 在项目内放置 MCP 配置文件

推荐先复制示例文件，再按需修改：

```powershell
New-Item -ItemType Directory -Force .bytemind | Out-Null
Copy-Item mcp.example.json .bytemind/mcp.json
```

然后把 `.bytemind/mcp.json` 中的 `GITHUB_PERSONAL_ACCESS_TOKEN` 改成你的真实 PAT。

`.bytemind/mcp.json` 使用独立的顶层 MCP 配置结构：

```json
{
  "enabled": true,
  "sync_ttl_s": 30,
  "servers": [
    {
      "id": "github",
      "name": "GitHub MCP",
      "enabled": true,
      "transport": {
        "type": "stdio",
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-github"],
        "env": {
          "GITHUB_PERSONAL_ACCESS_TOKEN": "<YOUR_GITHUB_PAT>"
        }
      },
      "auto_start": true,
      "startup_timeout_s": 20,
      "call_timeout_s": 60,
      "max_concurrency": 4
    }
  ]
}
```

## 2. 在 TUI 中查看状态

```text
/mcp list
/mcp show <id>
/mcp help
```

## 3. CLI 仍可用于运维操作

```bash
bytemind mcp list
bytemind mcp show --id <id>
bytemind mcp test --id <id>
bytemind mcp enable --id <id>
bytemind mcp disable --id <id>
bytemind mcp remove --id <id>
bytemind mcp reload
```

## 4. 常见问题

1. `No MCP servers configured.`：确认 `.bytemind/mcp.json` 路径和 JSON 结构正确。
2. `requires transport.command`：`stdio` 模式必须提供 `transport.command`。
3. 服务加载失败：先用 `/mcp show <id>` 查看 `status/message`，再修正 command/args/env。
