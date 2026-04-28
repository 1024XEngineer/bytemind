# 环境变量

ByteMind 的环境变量分两类：**安装时**（安装脚本读取）和**运行时**（`bytemind` 二进制读取）。

## 安装时变量

传入安装脚本，控制二进制的下载方式。

| 变量                   | 说明                          | 默认值                   |
| ---------------------- | ----------------------------- | ------------------------ |
| `BYTEMIND_VERSION`     | 安装的发布标签（如 `v0.3.0`） | 最新版本                 |
| `BYTEMIND_INSTALL_DIR` | 安装目标目录                  | `~/.local/bin`           |
| `BYTEMIND_REPO`        | GitHub 仓库地址               | `1024XEngineer/bytemind` |

**示例：**

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh \
  | BYTEMIND_VERSION=v0.3.0 BYTEMIND_INSTALL_DIR=/usr/local/bin bash
```

## 运行时变量

`bytemind` 启动时读取，均可覆盖配置文件中的对应字段。

| 变量                       | 覆盖字段           | 说明                                           |
| -------------------------- | ------------------ | ---------------------------------------------- |
| `BYTEMIND_API_KEY`         | `provider.api_key` | 未设置 `api_key_env` 时的默认 API 密钥         |
| `BYTEMIND_APPROVAL_MODE`   | `approval_mode`    | 覆盖审批模式（`interactive` 或 `away`）        |
| `BYTEMIND_SANDBOX_ENABLED` | `sandbox_enabled`  | 设为 `true` 开启沙箱模式                       |
| `BYTEMIND_WRITABLE_ROOTS`  | `writable_roots`   | 英文冒号分隔的可写目录列表                     |
| `BYTEMIND_HOME`            | —                  | 覆盖 `.bytemind/` 基础目录路径                 |
| `BYTEMIND_MAX_ITERATIONS`  | `max_iterations`   | 覆盖最大工具调用轮次限制                       |
| `BYTEMIND_LOG_LEVEL`       | —                  | 日志详细级别：`debug`、`info`、`warn`、`error` |

**示例——纯环境变量配置（无需配置文件）：**

```bash
export OPENAI_API_KEY="sk-..."
export BYTEMIND_APPROVAL_MODE="away"
export BYTEMIND_SANDBOX_ENABLED="true"
export BYTEMIND_WRITABLE_ROOTS="./src:./docs"

bytemind run -prompt "重新生成所有 API 文档"
```

:::tip Provider 密钥变量
建议在 `provider` 配置中设置 `api_key_env` 为个人值信的变量名。`BYTEMIND_API_KEY` 只是未设置 `api_key_env` 时的回退默认值。
:::

## 相关页面

- [配置参考](/zh/reference/config-reference) — 完整配置字段
- [配置](/zh/configuration) — 配置文件格式与示例
- [安装](/zh/installation) — `BYTEMIND_INSTALL_DIR` 与 `BYTEMIND_VERSION`
