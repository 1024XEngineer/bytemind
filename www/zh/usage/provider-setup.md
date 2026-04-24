# Provider 配置

ByteMind 支持任何兼容 OpenAI API 的服务，以及 Anthropic 原生 API。

## OpenAI

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

## Anthropic

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

## DeepSeek

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.deepseek.com/v1",
    "model": "deepseek-coder",
    "api_key_env": "DEEPSEEK_API_KEY"
  }
}
```

## 本地模型（Ollama）

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

:::tip 任意 OpenAI 兼容端点均可使用
只要服务支持 `POST /v1/chat/completions` 标准格式，就可直接配置。包括 Azure OpenAI、Groq、Together AI 等云服务，以及大多数本地推理就。
:::

## 通过环境变量传入 API Key

推荐始终使用 `api_key_env` 而不是将密钥写入配置文件：

```json
{ "provider": { "api_key_env": "MY_API_KEY_VAR" } }
```

```bash
export MY_API_KEY_VAR="sk-..."
bytemind chat
```

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
bytemind chat
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
