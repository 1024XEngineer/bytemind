# MCP 配置与使用

**MCP（Model Context Protocol）**是一种开放协议，允许 ByteMind 通过外部 MCP 服务器扩展工具能力。配置后，MCP 服务器提供的工具会自动注册到 Agent 的工具列表中，可像内置工具一样被 Agent 调用。

## 工作原理

1. 在 `.bytemind/mcp.json`（项目级）或 `~/.bytemind/mcp.json`（用户级）中配置 MCP 服务器
2. ByteMind 启动时加载并连接到配置的 MCP 服务器（通过 stdio 子进程）
3. 服务器提供的工具自动注册到工具注册表，以稳定 key 格式命名，如 `mcp:github:search_code`
4. Agent 在需要时自动调用这些工具，与其他内置工具一致

## 配置文件

MCP 服务器配置在独立的 `mcp.json` 文件中，与主 `config.json` 分开。项目级配置放在 `.bytemind/mcp.json`，用户级（跨项目）配置放在 `~/.bytemind/mcp.json`。

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
        },
        "cwd": ""
      },
      "auto_start": true,
      "startup_timeout_s": 20,
      "call_timeout_s": 60,
      "max_concurrency": 4
    }
  ]
}
```

### MCP 配置字段

**顶层：**

| 字段          | 类型    | 默认值 | 说明                           |
| ------------- | ------- | ------ | ------------------------------ |
| `enabled`     | bool    | `true` | 全局 MCP 开关                  |
| `sync_ttl_s`  | int     | `30`   | 工具列表同步间隔（秒）         |
| `servers`     | array   | —      | MCP 服务器列表                 |

**每个 Server：**

| 字段               | 类型   | 默认值 | 说明                                 |
| ------------------ | ------ | ------ | ------------------------------------ |
| `id`               | string | —（必填） | 唯一标识符，用于工具命名和命令行操作 |
| `name`             | string | `id`   | 可读名称                             |
| `enabled`          | bool   | `true` | 是否启用该服务器                     |
| `auto_start`       | bool   | `true` | 启动时自动连接服务器                 |
| `transport.type`   | string | `stdio`| 传输类型（目前仅支持 stdio）         |
| `transport.command`| string | —（必填）| 启动服务器的可执行命令             |
| `transport.args`   | array  | `[]`   | 命令参数                             |
| `transport.env`    | object | `{}`   | 环境变量（API Key 等）              |
| `transport.cwd`    | string | —      | 工作目录                             |
| `startup_timeout_s`| int    | `20`   | 服务器启动超时（秒）                 |
| `call_timeout_s`   | int    | `60`   | 工具调用超时（秒）                   |
| `max_concurrency`  | int    | `4`    | 最大并发工具调用数                   |
| `protocol_version` | string | —      | MCP 协议版本                         |

## 添加 MCP 服务器

在工作区的 `.bytemind/mcp.json` 中新建或编辑，在 `servers` 数组中新增条目：

```json
{
  "enabled": true,
  "servers": [
    {
      "id": "filesystem",
      "name": "File System MCP",
      "enabled": true,
      "transport": {
        "type": "stdio",
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/allowed/dir"],
        "env": {},
        "cwd": ""
      },
      "auto_start": true
    }
  ]
}
```

保存后重启 ByteMind，MCP 服务器提供的工具会自动出现在 Agent 的工具集中。

## 管理 MCP 服务器

ByteMind 提供完整的 MCP 服务器生命周期管理：

```bash
# 列出所有 MCP 服务器
bytemind mcp list

# 添加 MCP 服务器
bytemind mcp add

# 启用/禁用指定服务器
bytemind mcp enable <server-id>
bytemind mcp disable <server-id>

# 移除服务器
bytemind mcp remove <server-id>

# 查看服务器详情
bytemind mcp show <server-id>

# 测试服务器连接
bytemind mcp test <server-id>
```

## 工具名称

MCP 服务器提供的工具以 `mcp:<server_id>:<tool_name>` 格式注册为稳定 key。例如，`github` 服务器的 `search_code` 工具会注册为 `mcp:github:search_code`。Agent 在需要时会自动发现并调用这些工具。

## 配置示例

### GitHub MCP

使用 GitHub MCP 服务器让 Agent 直接操作 GitHub 资源：

```json
{
  "enabled": true,
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
      "auto_start": true
    }
  ]
}
```

### 自定义 Python MCP 服务器

```json
{
  "enabled": true,
  "servers": [
    {
      "id": "my-tools",
      "name": "My Custom Tools",
      "enabled": true,
      "transport": {
        "type": "stdio",
        "command": "python",
        "args": ["-m", "my_mcp_server"],
        "env": {
          "DATABASE_URL": "postgres://localhost:5432/mydb"
        },
        "cwd": "./tools"
      },
      "auto_start": false,
      "startup_timeout_s": 30,
      "call_timeout_s": 120
    }
  ]
}
```

## 相关页面

- [工具与审批](/zh/usage/tools-and-approval) — 工具分类与审批流程
- [配置参考](/zh/reference/config-reference) — 完整配置字段
- [Provider 配置](/zh/usage/provider-setup) — 多 Provider 配置
