# 获取 API Key

很多模型服务都提供“OpenAI 兼容 API”。你需要从服务商那里拿到四个信息，再填进 ByteMind 配置：

| 信息 | 在 ByteMind 里的字段 | 例子 |
| ---- | -------------------- | ---- |
| 服务类型 | `provider.type` | `openai-compatible` |
| API 地址 | `provider.base_url` | `https://api.deepseek.com` |
| 模型 ID | `provider.model` | `deepseek-v4-flash` |
| API Key | `provider.api_key` | `sk-...` |

下面以 DeepSeek 为例说明。其他 OpenAI 兼容服务也基本按同样思路查找：先找 API Key，再找 Base URL 和模型 ID。

## 第一步：进入服务商控制台

打开 [DeepSeek Platform](https://platform.deepseek.com/api_keys)，登录账号后进入 API Keys 页面。

![DeepSeek API Keys 页面](./assets/api_key1.png)

如果你还没有可用额度，先在平台里确认余额或充值状态。API 调用通常按 token 用量扣费；DeepSeek 官方说明费用会从充值余额或赠送余额中扣除，价格可能调整，实际以 [模型与价格](https://api-docs.deepseek.com/zh-cn/quick_start/pricing) 页面为准。

## 第二步：创建 API Key

在 API Keys 页面创建一个新的 Key。创建后复制完整密钥，保存到本机安全位置，后面配置 ByteMind 时会用到。

![DeepSeek API Keys 页面](./assets/api_key2.png)

![DeepSeek API Keys 页面](./assets/api_key3.png)

不要把 API Key 发给别人，也不要提交到公开仓库。对新手来说，先写进本机 `~/.bytemind/config.json` 最直观；熟悉后可以改用环境变量。

## 第三步：解读 API 文档

打开 DeepSeek 的 [首次调用 API](https://api-docs.deepseek.com/zh-cn/) 文档，重点只看这几项：

![DeepSeek API Keys 页面](./assets/deepseek_api.png)

| 文档里的名字 | ByteMind 配置 | DeepSeek 示例 |
| ------------ | ------------- | ------------- |
| `base_url (OpenAI)` | `provider.base_url` | `https://api.deepseek.com` |
|  | `provider.type` | `openai-compatible` |
| `api_key` | `provider.api_key` | 你刚创建的 Key |
| `model` | `provider.model` | `deepseek-v4-flash` |

截至 2026-05-07，DeepSeek 官方文档推荐的 OpenAI 格式模型包括 `deepseek-v4-flash` 和 `deepseek-v4-pro`。旧模型名 `deepseek-chat` 与 `deepseek-reasoner` 官方标注将于 2026-07-24 弃用，因此新配置建议优先使用 `deepseek-v4-flash`(对应网页版“快速模式”) 或 `deepseek-v4-pro`(对应网页版“专家模式”)。

## 第四步：写入 ByteMind 配置

把下面的 `YOUR_DEEPSEEK_API_KEY` 替换为你刚复制的 Key。

<Tabs default-tab="PowerShell">
<Tab title="PowerShell">

```powershell
New-Item -ItemType Directory -Force "$env:USERPROFILE\.bytemind" | Out-Null
$config = @'
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.deepseek.com",
    "model": "deepseek-v4-flash",
    "api_key": "YOUR_DEEPSEEK_API_KEY"
  }
}
'@

$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText("$env:USERPROFILE\.bytemind\config.json", $config, $utf8NoBom)
```

这会把 `config.json` 写成不带 BOM 的 UTF-8，在 Windows PowerShell 5.1 和 PowerShell 7+ 中都能稳定工作。

</Tab>

<Tab title="Linux">

```bash
mkdir -p ~/.bytemind
cat > ~/.bytemind/config.json <<'JSON'
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.deepseek.com",
    "model": "deepseek-v4-flash",
    "api_key": "YOUR_DEEPSEEK_API_KEY"
  }
}
JSON
```

</Tab>

<Tab title="MacOS">

```bash
mkdir -p ~/.bytemind
cat > ~/.bytemind/config.json <<'JSON'
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.deepseek.com",
    "model": "deepseek-v4-flash",
    "api_key": "YOUR_DEEPSEEK_API_KEY"
  }
}
JSON
```

</Tab>
</Tabs>

## 第五步：验证是否可用

进入一个具体项目目录后启动 ByteMind：

```bash
bytemind
```

输入一个很小的任务，例如：

```text
用一句话介绍这个项目。
```

如果模型正常回复，说明 API Key、Base URL 和模型 ID 都配置成功。

## 常见问题

**`provider.type` 应该填什么？**

DeepSeek 使用 OpenAI 兼容格式，所以填 `openai-compatible`。

**`base_url` 要不要加 `/v1`？**

DeepSeek 官方文档给出的 OpenAI 格式 Base URL 是 `https://api.deepseek.com`。ByteMind 会在后面拼接默认接口路径 `/chat/completions`，所以这里不要再加 `/chat/completions`。

**可以用 `api_key_env` 代替 `api_key` 吗？**

可以，而且更安全。把 `"api_key"` 替换为 `"api_key_env": "DEEPSEEK_API_KEY"`，然后设置环境变量：

<Tabs default-tab="PowerShell">
<Tab title="PowerShell">

```powershell
# 临时（仅当前窗口有效）：
$env:DEEPSEEK_API_KEY = "sk-..."

# 永久（重启电脑后依然有效）：
[Environment]::SetEnvironmentVariable("DEEPSEEK_API_KEY", "sk-...", "User")
# 执行后需重启终端窗口。
```

</Tab>

<Tab title="Linux">

```bash
export DEEPSEEK_API_KEY="sk-..."
```

</Tab>

<Tab title="macOS">

```bash
export DEEPSEEK_API_KEY="sk-..."
```

</Tab>
</Tabs>

:::warning 不要同时设置 `api_key` 和 `api_key_env`
如果两个都存在，`api_key` 优先，`api_key_env` 会被忽略。二选一即可。
:::

**模型 ID 可以随便写吗？**

不可以。模型 ID 必须和服务商文档里的名字完全一致。DeepSeek 当前建议从 `deepseek-v4-flash` 开始；如果你需要更高能力，再按官方文档改成 `deepseek-v4-pro`。

**仍然报错怎么办？**

先检查三件事：Key 有没有复制完整、平台余额是否可用、`base_url` 和 `model` 有没有多写或少写字符。更多排查见[故障排查](/zh/troubleshooting)。
