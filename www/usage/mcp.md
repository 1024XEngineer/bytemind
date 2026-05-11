# MCP Setup and Usage

**MCP (Model Context Protocol)** is an open protocol that allows ByteMind to extend its tool capabilities through external MCP servers. Once configured, tools exposed by MCP servers are automatically registered in the agent's tool list and can be called just like built-in tools.

## How It Works

1. Configure MCP servers in `mcp.json` or `config.json`
2. ByteMind loads and connects to configured MCP servers at startup (via stdio subprocess)
3. Server-provided tools are automatically registered in the tool registry with names like `mcp.<server_id>_<tool_name>`
4. The agent invokes these tools as needed, just like built-in tools

## Configuration File

MCP servers can be configured in a project-level `mcp.json` file or inside `config.json` under the `mcp` field (both are equivalent).

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

### MCP Config Fields

**Top-level:**

| Field         | Type   | Default | Description                    |
| ------------- | ------ | ------- | ------------------------------ |
| `enabled`     | bool   | `true`  | Global MCP switch              |
| `sync_ttl_s`  | int    | `30`    | Tool list sync interval (seconds) |
| `servers`     | array  | —       | MCP server list                |

**Per Server:**

| Field              | Type   | Default    | Description                        |
| ------------------ | ------ | ---------- | ---------------------------------- |
| `id`               | string | — (required) | Unique identifier for tool naming  |
| `name`             | string | `id`       | Human-readable name                |
| `enabled`          | bool   | `true`     | Whether this server is enabled     |
| `auto_start`       | bool   | `true`     | Auto-connect on startup            |
| `transport.type`   | string | `stdio`    | Transport type (stdio only)        |
| `transport.command`| string | — (required) | Executable to start the server   |
| `transport.args`   | array  | `[]`       | Command arguments                  |
| `transport.env`    | object | `{}`       | Environment variables (API keys, etc.) |
| `transport.cwd`    | string | —          | Working directory                  |
| `startup_timeout_s`| int    | `20`       | Server startup timeout (seconds)   |
| `call_timeout_s`   | int    | `60`       | Tool call timeout (seconds)        |
| `max_concurrency`  | int    | `4`        | Max concurrent tool calls          |
| `protocol_version` | string | —          | MCP protocol version               |

## Adding an MCP Server

Edit `mcp.json` in the project root, adding a new entry to the `servers` array:

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

Save and restart ByteMind. Tools from the MCP server will automatically appear in the agent's tool set.

## Managing MCP Servers

ByteMind provides full MCP server lifecycle management:

```bash
# List all MCP servers
bytemind mcp list

# Add an MCP server
bytemind mcp add

# Enable/disable a server
bytemind mcp enable <server-id>
bytemind mcp disable <server-id>

# Remove a server
bytemind mcp remove <server-id>

# Show server details
bytemind mcp show <server-id>

# Test server connection
bytemind mcp test <server-id>
```

## Tool Naming

MCP server tools are registered with `mcp.<server_id>_<tool_name>` names. For example, the `search_code` tool from the `github` server is registered as `mcp_github__search_code`. The agent discovers and invokes these tools automatically as needed.

## Configuration Examples

### GitHub MCP

Use the GitHub MCP server to let the agent interact with GitHub resources directly:

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

### Custom Python MCP Server

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

## Health Checks

ByteMind automatically monitors MCP server health. Health check configuration is in `provider_runtime.health`:

- `fail_threshold` (default 3): Consecutive failures before marking unhealthy
- `recover_probe_sec` (default 30): Recovery probe interval
- `recover_success_threshold` (default 2): Consecutive successes before recovery

Tools from unhealthy servers are temporarily removed from the registry and automatically re-registered upon recovery.

## See Also

- [Tools and Approval](/usage/tools-and-approval) — tool categories and approval flow
- [Config Reference](/reference/config-reference) — all config fields
- [Provider Setup](/usage/provider-setup) — multi-provider configuration
