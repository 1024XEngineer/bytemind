# 故障排查

## `bytemind: 未找到命令`

二进制已安装但不在 `PATH` 中。

**修复：** 将安装目录加入 `PATH`：

```bash
export PATH="$HOME/.local/bin:$PATH"
```

将该行写入 `~/.bashrc`、`~/.zshrc` 或 Shell 配置文件以永久生效。Windows 用户执行：

```powershell
[Environment]::SetEnvironmentVariable("Path", "$env:USERPROFILE\.local\bin;" + $env:Path, "User")
```

## Provider 鉴权失败

症状：输出中出现 `401 Unauthorized` 或 `authentication failed`。

**检查：**

1. `provider.api_key` 或 `provider.api_key_env` 指定的环境变量已设置且有效
2. `provider.base_url` 指向正确端点（无末尾斜杠，路径包含正确版本号）
3. `provider.model` 在你的 Provider 计划中存在

```bash
# 快速验证 API Key 有效性
curl -s -H "Authorization: Bearer $OPENAI_API_KEY" \
  https://api.openai.com/v1/models | head -c 200
```

如果 curl 返回模型列表，说明 Key 有效。若 ByteMind 仍失败，确认配置中的 `base_url` 与正常工作的 curl URL 完全匹配。

## Agent 过早停止

症状：Agent 输出部分结果后提示已到迎代上限。

**修复：** 提高 `max_iterations`：

```bash
bytemind chat -max-iterations 64
```

或写入配置文件永久生效：

```json
{ "max_iterations": 64 }
```

## 配置文件未被读取

症状：ByteMind 行为与配置不符，似乎使用默认值。

**检查配置查找顺序：**

1. 当前目录的 `.bytemind/config.json`
2. 当前目录的 `config.json`
3. 主目录的 `~/.bytemind/config.json`

运行 `bytemind chat -v` 可查看实际加载的配置文件路径。

## 恢复会话后找不到

症状：`/resume <id>` 提示找不到会话。

**检查：**

- 当前工作目录与创建会话时相同
- `.bytemind/sessions/` 中存在对应会话文件
- `BYTEMIND_HOME` 环境变量未指向其他目录

## 沙箱限制了写入

症状：Agent 尝试写入文件时失败，即使路径看起来合法。

**修复：** 将该路径加入 `writable_roots`：

```json
{
  "sandbox_enabled": true,
  "writable_roots": ["./src", "./docs"]
}
```

或在本地开发时禁用沙箱：

```json
{ "sandbox_enabled": false }
```

## 流式输出乱码

症状：输出内容杂乱或显示原始转义序列。

**修复：** 禁用流式输出：

```json
{ "stream": false }
```

在非 TTY 环境（如管道输出、某些 CI runner）中较常见。

## 上下文窗口超限

症状：Agent 警告上下文用量并中途停止。

**应对方法：**

- 用 `/new` 开启新会话以清空上下文
- 将任务拆分为更小的块分次完成
- 调整 `context_budget.warning_ratio` 和 `context_budget.critical_ratio` 阈值

## 相关页面

- [常见问题](/zh/faq) — 常见问题解答
- [配置](/zh/configuration) — 行为调优配置项
- [安装](/zh/installation) — PATH 和版本固定
