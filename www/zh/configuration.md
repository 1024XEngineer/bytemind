# 配置

ByteMind 默认读取 `.bytemind/config.json`（或项目根目录 `config.json`）。

## 最小 OpenAI 兼容配置

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4-mini",
    "api_key": "YOUR_API_KEY"
  },
  "approval_policy": "on-request",
  "max_iterations": 32,
  "stream": true
}
```

## 最小 Anthropic 配置

```json
{
  "provider": {
    "type": "anthropic",
    "base_url": "https://api.anthropic.com",
    "model": "claude-sonnet-4-20250514",
    "api_key": "YOUR_API_KEY",
    "anthropic_version": "2023-06-01"
  }
}
```

## 推荐默认值

- 日常使用建议 `approval_policy: on-request`
- 复杂任务提高 `max_iterations`
- 保持 `stream: true` 便于实时观察进度

完整字段见[配置参考](/zh/reference/config-reference)。
