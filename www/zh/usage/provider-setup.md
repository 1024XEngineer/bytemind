# Provider 配置

ByteMind 支持任何兼容 OpenAI API 的服务，以及 Anthropic 和 Gemini 原生 API。

## 多 Provider 配置（模型切换）

一次性配置多个 Provider，运行时通过 `/model` 切换：

```json
{
  "provider_runtime": {
    "current_provider": "deepseek",
    "default_provider": "deepseek",
    "default_model": "deepseek-v4-flash",
    "providers": {
      "deepseek": {
        "type": "openai-compatible",
        "base_url": "https://api.deepseek.com",
        "api_key_env": "DEEPSEEK_API_KEY",
        "model": "deepseek-v4-flash",
        "models": ["deepseek-v4-flash", "deepseek-v4-pro"]
      },
      "openai": {
        "type": "openai-compatible",
        "base_url": "https://api.openai.com/v1",
        "api_key_env": "OPENAI_API_KEY",
        "model": "gpt-5.4-mini",
        "models": ["gpt-5.4-mini", "gpt-5.4"]
      },
      "anthropic": {
        "type": "anthropic",
        "base_url": "https://api.anthropic.com",
        "api_key_env": "ANTHROPIC_API_KEY",
        "model": "claude-sonnet-4-20250514",
        "models": ["claude-sonnet-4-20250514", "claude-opus-4-20250514"]
      }
    }
  }
}
```

切换模型：输入 `/model` 弹出交互式选择器，列出所有已配置 Provider 的模型，`↑↓` 移动光标、`Enter` 切换。切换后配置文件会自动更新。

:::tip 直接跳转
也可以输入 `/model <provider>/<model>` 直接切换，如 `/model openai/gpt-5.4`。
:::

完整字段参考见[配置参考](/zh/reference/config-reference#provider-runtime-多-provider)。

## 单 Provider 示例（兼容旧版）

<Tabs default-tab="OpenAI">
<Tab title="OpenAI">

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-4o",
    "api_key_env": "OPENAI_API_KEY"
  }
}
```

</Tab>

<Tab title="Anthropic">

```json
{
  "provider": {
    "type": "anthropic",
    "base_url": "https://api.anthropic.com",
    "model": "claude-sonnet-4-20250514",
    "api_key_env": "ANTHROPIC_API_KEY",
    "anthropic_version": "2023-06-01"
  }
}
```

</Tab>

<Tab title="Gemini">

```json
{
  "provider": {
    "type": "gemini",
    "base_url": "https://generativelanguage.googleapis.com/v1beta",
    "model": "gemini-2.5-flash",
    "api_key_env": "GEMINI_API_KEY"
  }
}
```

</Tab>

<Tab title="DeepSeek">

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.deepseek.com",
    "model": "deepseek-v4-flash",
    "api_key_env": "DEEPSEEK_API_KEY"
  }
}
```

</Tab>

<Tab title="Ollama（本地）">

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "http://localhost:11434/v1",
    "model": "qwen2.5-coder:7b",
    "api_key": "ollama"
  }
}
```

</Tab>
</Tabs>

新手可先看[获取 API Key](/zh/api-key)，按 DeepSeek 示例完成控制台、模型 ID 和配置文件的对应关系。

:::tip 任意 OpenAI 兼容端点均可使用
只要服务支持 `POST /v1/chat/completions` 标准格式，就可直接配置。包括 Azure OpenAI、Groq、Together AI 等云服务，以及大多数本地推理就。
:::

## 通过环境变量传入 API Key

推荐始终使用 `api_key_env` 而不是将密钥写入配置文件：

```json
{ "provider": { "api_key_env": "MY_API_KEY_VAR" } }
```

**在启动 ByteMind 之前**设置环境变量：

<Tabs default-tab="PowerShell">
<Tab title="PowerShell">

```powershell
# 临时（仅当前窗口有效）：
$env:MY_API_KEY_VAR = "sk-..."

# 永久（重启电脑后依然有效）：
[Environment]::SetEnvironmentVariable("MY_API_KEY_VAR", "sk-...", "User")
# 执行后需重启终端窗口。
```

</Tab>

<Tab title="Linux">

```bash
# 临时（仅当前窗口有效）：
export MY_API_KEY_VAR="sk-..."

# 永久：
echo 'export MY_API_KEY_VAR="sk-..."' >> ~/.bashrc
```

</Tab>

<Tab title="macOS">

```bash
# 临时（仅当前窗口有效）：
export MY_API_KEY_VAR="sk-..."

# 永久：
echo 'export MY_API_KEY_VAR="sk-..."' >> ~/.zshrc
```

</Tab>
</Tabs>

```bash
bytemind
```

:::warning `api_key` 会覆盖 `api_key_env`
如果同时写了 `api_key` 和 `api_key_env`，`api_key`（明文）优先。想用环境变量需先删掉 config 里的 `api_key` 字段。
:::

## 自定义鉴权头

对于需要非标准鉴权的网关或内部服务：

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://my-internal-gateway/v1",
    "model": "gpt-4o",
    "auth_header": "X-API-Token",
    "auth_scheme": "",
    "api_key_env": "GATEWAY_TOKEN"
  }
}
```

## 验证配置

创建配置后运行：

```bash
bytemind
```

输入一个简单任务（如 `说个 hello`）验证模型应答正常。如果失败，检查：

- `base_url` 可从你的机器访问
- `api_key` 或环境变量已设置且有效
- `model` ID 与 Provider 提供的模型匹配

常见鉴权失败解决方法见[故障排查](/zh/troubleshooting)。

## 相关页面

- [配置](/zh/configuration) — 完整配置字段
- [环境变量](/zh/reference/env-vars) — 运行时覆盖
- [故障排查](/zh/troubleshooting) — 鉴权失败与连接问题
