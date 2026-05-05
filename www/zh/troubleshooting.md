# 故障排查

## `bytemind: 未找到命令`

二进制已安装但不在 `PATH` 中。

**修复：** 将安装目录加入 `PATH`：

```bash
export PATH="$HOME/bin:$PATH"
```

将该行写入 `~/.bashrc`、`~/.zshrc` 或 Shell 配置文件以永久生效。Windows 用户执行：

```powershell
$target = "$env:USERPROFILE\bin"
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (-not (($userPath -split ";") -contains $target)) {
  [Environment]::SetEnvironmentVariable("Path", ($target + ";" + $userPath), "User")
}
$env:Path = $target + ";" + $env:Path
```

## Windows 更新后仍显示旧版本

症状：安装脚本显示下载了最新版本，但 `bytemind --version` 仍输出旧版本。

**修复：** 先确认 PowerShell 实际命中的二进制：

```powershell
Get-Command bytemind -All | Select-Object Source
& "$env:USERPROFILE\bin\bytemind.exe" --version
```

如果第二行输出新版本，而 `Get-Command` 第一行不是 `$env:USERPROFILE\bin\bytemind.exe`，把新安装目录移动到用户 PATH 最前面：

```powershell
$target = "$env:USERPROFILE\bin"
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
$parts = $userPath -split ";" | Where-Object { $_ -and ($_ -ine $target) }
[Environment]::SetEnvironmentVariable("Path", ($target + ";" + ($parts -join ";")), "User")
$env:Path = $target + ";" + $env:Path
bytemind --version
```

## Windows 运行 bash 安装命令时报 WSL 错误

症状：在 PowerShell 或 CMD 中运行 `curl ... install.sh | bash` 后，出现 `ext4.vhdx`、`HCS`、`Bash/Service/CreateInstance` 或 WSL 挂载错误。

**修复：** 这是 WSL 环境错误，不是 ByteMind 安装包错误。在 Windows 终端中使用 PowerShell 安装脚本：

```powershell
iwr -useb https://raw.githubusercontent.com/1024XEngineer/bytemind/main/scripts/install.ps1 | iex
```

只有在已经进入正常工作的 WSL/Linux 终端时，才使用 `install.sh | bash`。WSL 里的 `~/bin/bytemind` 和 Windows 的 `%USERPROFILE%\bin\bytemind.exe` 是两个不同位置。

## Windows 卸载时报路径不存在

症状：在 PowerShell 中运行 `rm ~/bin/bytemind`，提示找不到 `C:\Users\<你>\bin\bytemind`。

**修复：** Windows 安装的文件名是 `bytemind.exe`，请删除带 `.exe` 后缀的文件：

```powershell
Remove-Item "$env:USERPROFILE\bin\bytemind.exe"
```

如果当前命令来自其他目录，先查看实际路径再删除：

```powershell
Get-Command bytemind -All | Select-Object Source
Remove-Item "<上一步显示的 bytemind.exe 路径>"
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
bytemind -max-iterations 64
```

或写入配置文件永久生效：

```json
{ "max_iterations": 64 }
```

## 配置文件未被读取

症状：ByteMind 行为与配置不符，似乎使用默认值。

**检查配置加载顺序：**

1. 用户目录的 `~/.bytemind/config.json`
2. 当前工作区的 `.bytemind/config.json`（可选，覆盖全局配置）

新用户建议先把通用配置放在用户目录，不要放到 `~/bin` 或 `%USERPROFILE%\bin`。运行 `bytemind -v` 可查看实际加载的配置文件路径。

## 工作区过大或目录不合适

症状：在用户主目录、磁盘根目录、Downloads、Desktop 或很大的文件夹中启动时，ByteMind 提示当前目录过宽，或响应明显变慢。

**修复：** 先进入具体代码仓库或项目子目录，再启动：

```powershell
Set-Location D:\code\my-project
bytemind
```

也可以从任意目录显式指定工作区：

```powershell
bytemind -workspace D:\code\my-project
```

暂不建议把包含大量无关文件的大文件夹作为工作区。安装目录 `%USERPROFILE%\bin` / `~/bin` 只用于存放二进制，也不是工作区。

## 恢复会话后找不到

症状：`/resume <id>` 提示找不到会话。

**检查：**

- 当前工作目录与创建会话时相同
- ByteMind home 目录的会话数据中存在对应会话
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
