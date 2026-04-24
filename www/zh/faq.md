# 常见问题

## 安装

### 使用 ByteMind 需要先安装 Go 吗？

不需要。ByteMind 提供针对 macOS、Linux 和 Windows 的预编译二进制，无需预先安装 Go。

### 安装后没有 `bytemind` 命令，怎么办？

安装脚本把二进制放入 `~/.local/bin`。确认该目录已加入 `PATH`：

```bash
export PATH="$HOME/.local/bin:$PATH"
```

将该行添加到 `~/.bashrc` 或 `~/.zshrc` 以永久生效。

### 如何安装指定版本？

```bash
curl -fsSL https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.sh \
  | BYTEMIND_VERSION=v0.3.0 bash
```

## Provider 问题

### 可以使用本地模型吗？

可以。只要本地服务器展开兼容 OpenAI 的 API（如 Ollama 的 `/v1/chat/completions` 端点）即可。将 `provider.type` 设为 `openai-compatible`，`base_url` 指向 `http://localhost:11434/v1`。

### 可以对接 DeepSeek、Groq 等第三方 Provider 吗？

可以。只要 Provider 实现了 OpenAI Chat Completions 接口，配置 `base_url` 指向对应端点即可。

### 必须显式设置 `provider.type` 吗？

不必须。设置 `"auto_detect_type": true`，ByteMind 会根据 `base_url` 自动推断 Provider 类型。

## 隐私与安全

### 我的代码会被上传或存储到任何地方吗？

不会。ByteMind 完全运行在你的本地机器上。它读取你的本地文件，并只将你在提示词中明确包含的内容发送到你配置的 LLM API。没有任何数据会发送到 ByteMind 服务器。

### 如何保存 API 密钥安全？

在配置文件中使用 `api_key_env` 而不是 `api_key`，将真实密钥存入环境变量，并将 `.bytemind/` 加入 `.gitignore`。

## 使用问题

### Agent 在完成任务前就停止了，怎么办？

提高 `max_iterations`：

```bash
bytemind chat -max-iterations 64
```

或在配置文件中永久设置：

```json
{ "max_iterations": 64 }
```

### 在 CI 中能不能不需手动审批？

可以。将 `approval_mode` 设为 `away`，`away_policy` 设为 `auto_deny_continue`。高风险操作会被自动拒绝（跳过），任务不会阻塞。

### 如何恢复上次的会话？

```text
/sessions
/resume <会话-id>
```

### 可以为团队创建自定义技能吗？

可以。将包含 `skill.json` 和 `SKILL.md` 的技能目录放入仓库的 `.bytemind/skills/` 中，团队成员即可共享使用。详见[技能](/zh/usage/skills)。

## 相关页面

- [故障排查](/zh/troubleshooting) — 具体错误解决方法
- [安装](/zh/installation) — 详细安装步骤
- [配置](/zh/configuration) — 完整配置指南
